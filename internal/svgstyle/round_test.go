package svgstyle

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestRoundSquare(t *testing.T) {
	for _, source := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><path fill="red" d="M0 0h256v256H0z"/></svg>`,
		`<?xml version="1.0"?><svg:svg xmlns:svg="http://www.w3.org/2000/svg" viewBox="-10 -10 100 100"><svg:defs><svg:linearGradient id="icon-svg-rounded-frame"/></svg:defs><svg:path fill="url(#icon-svg-rounded-frame)" d="M0 0"/></svg:svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="256px" height="256px"><rect width="256" height="256"/></svg>`,
	} {
		rounded, changed, err := RoundSquare([]byte(source))
		if err != nil || !changed {
			t.Fatalf("changed=%v error=%v", changed, err)
		}
		decoder := xml.NewDecoder(bytes.NewReader(rounded))
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if tag, ok := token.(xml.StartElement); ok && tag.Name.Space != "http://www.w3.org/2000/svg" {
				t.Fatalf("lost namespace: %v", tag.Name)
			}
		}
		second, changed, err := RoundSquare(rounded)
		if err != nil || changed || !bytes.Equal(second, rounded) {
			t.Fatal("rounding must be idempotent")
		}
	}
	for _, source := range []string{`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><path d="M0 0"/></svg>`, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"/>`} {
		out, changed, err := RoundSquare([]byte(source))
		if err != nil || changed || string(out) != source {
			t.Fatal("wide wordmarks and empty canvases should be preserved")
		}
	}
	out, _, _ := RoundSquare([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="10 20 100 100"><path id="icon-svg-rounded-frame" d="M10 20"/></svg>`))
	if !strings.Contains(string(out), `id="icon-svg-rounded-frame-1"`) || !strings.Contains(string(out), `x="10" y="20" width="100" height="100" rx="20"`) {
		t.Fatal("must preserve viewBox origin and avoid ID collisions")
	}
}
