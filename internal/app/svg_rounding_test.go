package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestUploadRounding(t *testing.T) {
	rounded, e := prepareUploadedSVG(testSVG)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(rounded), `data-icon-svg-rounded="v1"`) {
		t.Fatal("square icon was not rounded")
	}
	again, e := prepareUploadedSVG(rounded)
	if e != nil || !bytes.Equal(rounded, again) {
		t.Fatal("rounding must be idempotent", e)
	}
	wide := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><path d="M0 0h200v100H0z"/></svg>`)
	prepared, e := prepareUploadedSVG(wide)
	if e != nil || !bytes.Equal(wide, prepared) {
		t.Fatal("wide wordmarks must stay unchanged", e)
	}
	a, h := testApp(t)
	cookie, token := authenticate(t, a, h)
	assertCode(t, uploadTest(t, h, cookie, token, 1, testSVG), 303)
	var uploaded []byte
	var uploadedID int64
	a.store.db.QueryRow(`SELECT id FROM icons WHERE filename='google-1.svg'`).Scan(&uploadedID)
	indexed, err := iconByID(a.store.db, uploadedID)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := a.store.openSVG(context.Background(), indexed)
	if err != nil {
		t.Fatal(err)
	}
	uploaded, err = io.ReadAll(stream)
	stream.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(uploaded, rounded) {
		t.Fatal("upload must store the prepared SVG")
	}
}
