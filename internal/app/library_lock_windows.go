package app

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func lockLibrary(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

func rejectReparsePoint(root *os.Root, name string) error {
	p, e := windows.UTF16PtrFromString(filepath.Join(root.Name(), filepath.FromSlash(name)))
	if e != nil {
		return e
	}
	attrs, e := windows.GetFileAttributes(p)
	if e != nil {
		return e
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("素材库不允许 Windows junction 或其他 reparse point")
	}
	return nil
}
