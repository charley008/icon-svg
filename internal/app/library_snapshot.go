package app

import (
	"bytes"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

type memoryFiles map[string][]byte

func snapshotFS(files map[string][]byte) fs.FS { return memoryFiles(files) }
func (m memoryFiles) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	if raw, ok := m[name]; ok {
		return &memoryFile{Reader: bytes.NewReader(raw), info: memoryInfo{name: path.Base(name), size: int64(len(raw))}}, nil
	}
	return nil, fs.ErrNotExist
}
func (m memoryFiles) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	entries := map[string]memoryInfo{}
	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	for file, raw := range m {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		rest := strings.TrimPrefix(file, prefix)
		first, _, dir := strings.Cut(rest, "/")
		info := memoryInfo{name: first, size: int64(len(raw)), dir: dir}
		if previous, ok := entries[first]; ok && previous.dir {
			continue
		}
		entries[first] = info
	}
	names := []string{}
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := []fs.DirEntry{}
	for _, name := range names {
		out = append(out, entries[name])
	}
	return out, nil
}

type memoryFile struct {
	*bytes.Reader
	info memoryInfo
}

func (f *memoryFile) Close() error               { return nil }
func (f *memoryFile) Stat() (fs.FileInfo, error) { return f.info, nil }

type memoryInfo struct {
	name string
	size int64
	dir  bool
}

func (i memoryInfo) Name() string { return i.name }
func (i memoryInfo) Size() int64  { return i.size }
func (i memoryInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0555
	}
	return 0444
}
func (i memoryInfo) ModTime() time.Time         { return time.Time{} }
func (i memoryInfo) IsDir() bool                { return i.dir }
func (i memoryInfo) Sys() any                   { return nil }
func (i memoryInfo) Type() fs.FileMode          { return i.Mode().Type() }
func (i memoryInfo) Info() (fs.FileInfo, error) { return i, nil }
