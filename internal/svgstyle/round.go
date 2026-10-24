package svgstyle

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// RoundSquare adds a vector clip to square canvases, preserving the original
// geometry, gradients, namespaces and source bytes inside the new group.
func RoundSquare(content []byte) ([]byte, bool, error) {
	d := xml.NewDecoder(bytes.NewReader(content))
	depth := 0
	var root xml.StartElement
	var openingEnd, closingStart int64
	ids := map[string]bool{}
	prefix := ""
	for {
		start := d.InputOffset()
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, err
		}
		switch n := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				root = n
				openingEnd = d.InputOffset()
				opening := strings.TrimSpace(string(content[start:openingEnd]))
				if strings.HasSuffix(opening, "/>") {
					return content, false, nil
				}
				name := strings.FieldsFunc(strings.TrimPrefix(opening, "<"), func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '>' })[0]
				if pos := strings.IndexByte(name, ':'); pos >= 0 {
					prefix = name[:pos+1]
				}
			}
			for _, attr := range n.Attr {
				if attr.Name.Local == "id" {
					ids[attr.Value] = true
				}
			}
		case xml.EndElement:
			if depth == 1 {
				closingStart = start
			}
			depth--
		}
	}
	if root.Name.Local != "svg" || openingEnd == 0 || closingStart == 0 {
		return nil, false, fmt.Errorf("missing SVG root")
	}
	attrs := map[string]string{}
	for _, attr := range root.Attr {
		attrs[attr.Name.Local] = attr.Value
	}
	if attrs["data-icon-svg-rounded"] == "v1" {
		return content, false, nil
	}
	box := strings.Fields(strings.ReplaceAll(attrs["viewBox"], ",", " "))
	var x, y, width, height float64
	parse := func(s string) float64 {
		n, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "px"), 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0
		}
		return n
	}
	if len(box) == 4 {
		x, y, width, height = parse(box[0]), parse(box[1]), parse(box[2]), parse(box[3])
	} else if len(box) == 0 {
		width, height = parse(attrs["width"]), parse(attrs["height"])
	}
	if width <= 0 || height <= 0 || math.Abs(width-height) > width*1e-9 {
		return content, false, nil
	}
	id := "icon-svg-rounded-frame"
	for n := 1; ids[id]; n++ {
		id = fmt.Sprintf("icon-svg-rounded-frame-%d", n)
	}
	clip := fmt.Sprintf(`<%sdefs><%sclipPath id="%s" clipPathUnits="userSpaceOnUse"><%srect x="%g" y="%g" width="%g" height="%g" rx="%g"/></%sclipPath></%sdefs><%sg clip-path="url(#%s)">`, prefix, prefix, id, prefix, x, y, width, height, width*.2, prefix, prefix, prefix, id)
	var out bytes.Buffer
	out.Write(content[:openingEnd-1])
	out.WriteString(` data-icon-svg-rounded="v1">`)
	out.WriteString(clip)
	out.Write(content[openingEnd:closingStart])
	fmt.Fprintf(&out, "</%sg>", prefix)
	out.Write(content[closingStart:])
	return out.Bytes(), true, nil
}
