package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
)

type svgStream interface {
	fs.File
	io.Seeker
}

type bufferedSVG struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *bufferedSVG) Close() error               { return nil }
func (f *bufferedSVG) Stat() (fs.FileInfo, error) { return f.info, nil }

type cancelReader struct {
	ctx context.Context
	r   io.Reader
}

func (r cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func iconPath(i Icon) (string, error) {
	name := i.Brand + "/" + i.Meta.File
	if !validSlug(i.Brand) || !validSVGName(i.Meta.File) || !libraryTarget(name) {
		return "", errors.New("无效素材路径")
	}
	return name, nil
}

// Open the authoritative file, verify its indexed digest with a bounded buffer,
// then rewind. No SVG body is retained in SQLite or a process-wide cache.
// App writes replace files atomically; an already-open descriptor remains valid.
func (s *Store) openSVG(ctx context.Context, i Icon) (svgStream, error) {
	name, err := iconPath(i)
	if err != nil {
		return nil, err
	}
	if s.library != nil {
		if err = s.library.check(name); err != nil {
			return nil, err
		}
	}
	f, err := s.source.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSVG {
		f.Close()
		return nil, fmt.Errorf("无效或过大的 SVG 文件: %s", name)
	}
	stream, ok := f.(svgStream)
	if !ok {
		raw, readErr := io.ReadAll(io.LimitReader(f, maxSVG+1))
		f.Close()
		if readErr != nil || int64(len(raw)) > maxSVG {
			return nil, errors.New("无法读取 SVG 文件")
		}
		stream = &bufferedSVG{Reader: bytes.NewReader(raw), info: info}
	}
	h := sha256.New()
	n, err := io.Copy(h, cancelReader{ctx: ctx, r: io.LimitReader(stream, maxSVG+1)})
	after, statErr := stream.Stat()
	if err == nil && (statErr != nil || n != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || hex.EncodeToString(h.Sum(nil)) != i.Meta.SHA) {
		err = errors.New("素材文件已变化，请扫描素材库后重试")
	}
	if err == nil {
		_, err = stream.Seek(0, io.SeekStart)
	}
	if err != nil {
		stream.Close()
		return nil, err
	}
	return stream, nil
}

func (s *Store) stageSVG(brand, file string, raw []byte) {
	s.pendingSVG[brand+"/"+file] = raw
}
