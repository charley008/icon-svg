package main

import (
	"path/filepath"
	"testing"
)

func TestApplicationBase(t *testing.T) {
	root := t.TempDir()
	workingDir := filepath.Join(root, "another-working-directory")
	for _, tt := range []struct {
		name, executable, workingDir, want string
	}{
		{"bin uses parent", filepath.Join(root, "bin", "icon-svg.exe"), workingDir, root},
		{"bin case insensitive", filepath.Join(root, "BIN", "icon-svg.exe"), workingDir, root},
		{"standalone uses executable directory", filepath.Join(root, "icon-svg.exe"), workingDir, root},
		{"go run uses working directory", filepath.Join(root, "go-build123", "exe", "icon-svg.exe"), workingDir, workingDir},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := applicationBase(tt.executable, tt.workingDir); got != tt.want {
				t.Fatalf("applicationBase() = %q, want %q", got, tt.want)
			}
		})
	}
}
