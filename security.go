package main

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const maxSVG = 5 * 1024 * 1024

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func passwordHash(p string) string {
	salt := randomToken()
	key, err := pbkdf2.Key(sha256.New, p, []byte(salt), 600000, 32)
	if err != nil {
		panic(err)
	}
	return "pbkdf2:600000:" + salt + ":" + hex.EncodeToString(key)
}
func passwordMatches(p, encoded string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 100000 || n > 1000000 {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, p, []byte(parts[2]), n, 32)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(key)), []byte(parts[3])) == 1
}
func equalToken(a, b string) bool {
	return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var urlPattern = regexp.MustCompile(`(?is)url\s*\((.*?)\)`)
var dimensions = regexp.MustCompile(`^(\d+(?:\.\d+)?)(?:px)?$`)
var whitespace = regexp.MustCompile(`\s+`)

func safeCSS(s string) bool {
	compact := strings.ToLower(whitespace.ReplaceAllString(s, ""))
	for _, bad := range []string{"@import", "javascript:", "data:", "expression(", "\\", "/*"} {
		if strings.Contains(compact, bad) {
			return false
		}
	}
	for _, match := range urlPattern.FindAllStringSubmatch(s, -1) {
		if !strings.HasPrefix(strings.Trim(match[1], " \r\n\t\"'"), "#") {
			return false
		}
	}
	return true
}
func validateSVG(content []byte) ([]byte, error) {
	bad := func() ([]byte, error) {
		return nil, errors.New("SVG 包含不允许的内容、外部资源或无效尺寸")
	}
	if len(content) == 0 || len(content) > maxSVG {
		return nil, errors.New("SVG 必须大于 0 且不超过 5 MB")
	}
	decoder := xml.NewDecoder(bytes.NewReader(content))
	depth, count, roots := 0, 0, 0
	styleDepth := 0
	var styleText strings.Builder
	blocked := map[string]bool{}
	for _, tag := range strings.Fields("script foreignobject image iframe object embed audio video a animate animatetransform animatemotion set discard handler listener") {
		blocked[tag] = true
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return bad()
		}
		switch node := token.(type) {
		case xml.Directive:
			return bad()
		case xml.ProcInst:
			if node.Target != "xml" {
				return bad()
			}
		case xml.StartElement:
			if styleDepth > 0 {
				return bad()
			}
			depth++
			count++
			if count > 200000 || depth > 256 {
				return bad()
			}
			if node.Name.Space != "http://www.w3.org/2000/svg" || blocked[strings.ToLower(node.Name.Local)] {
				return bad()
			}
			if depth == 1 {
				roots++
				if roots != 1 || node.Name.Local != "svg" {
					return bad()
				}
				attrs := map[string]string{}
				for _, a := range node.Attr {
					attrs[a.Name.Local] = a.Value
				}
				box := strings.Fields(strings.ReplaceAll(attrs["viewBox"], ",", " "))
				if len(box) > 0 {
					if len(box) != 4 {
						return bad()
					}
					for n, v := range box {
						x, e := strconv.ParseFloat(v, 64)
						if e != nil || math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) >= 1e8 || (n >= 2 && x <= 0) {
							return bad()
						}
					}
				} else {
					for _, name := range []string{"width", "height"} {
						match := dimensions.FindStringSubmatch(attrs[name])
						if len(match) != 2 {
							return bad()
						}
						x, e := strconv.ParseFloat(match[1], 64)
						if e != nil || x <= 0 || x >= 1e8 {
							return bad()
						}
					}
				}
			}
			if node.Name.Local == "style" {
				styleDepth = depth
				styleText.Reset()
			}
			for _, attr := range node.Attr {
				local := strings.ToLower(attr.Name.Local)
				if attr.Name.Space == "xmlns" || attr.Name.Local == "xmlns" {
					continue
				}
				if strings.HasPrefix(local, "on") || local == "base" {
					return bad()
				}
				if local == "href" && !strings.HasPrefix(strings.TrimSpace(attr.Value), "#") {
					return bad()
				}
				if !safeCSS(attr.Value) {
					return bad()
				}
			}
		case xml.EndElement:
			if styleDepth == depth {
				if !safeCSS(styleText.String()) {
					return bad()
				}
				styleDepth = 0
			}
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(node)) != "" {
				return bad()
			}
			if styleDepth > 0 {
				styleText.Write(node)
			}
		}
	}
	if roots != 1 || depth != 0 {
		return bad()
	}
	return content, nil
}
func validURL(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}
func validateMeta(m GroupMeta) error {
	if strings.TrimSpace(m.Title) == "" || len([]rune(m.Title)) > 100 || len([]rune(m.Category)) > 60 || len(m.Description) > 4000 || !validURL(m.URL) {
		return errors.New("请填写有效名称、分类和 HTTP/HTTPS 网址")
	}
	for _, values := range [][]string{m.Domains, m.Tags, m.Aliases} {
		if len(values) > 40 {
			return errors.New("标签或域名数量过多")
		}
		for _, s := range values {
			if len(s) > 300 {
				return fmt.Errorf("字段过长: %.30s", s)
			}
		}
	}
	return nil
}
