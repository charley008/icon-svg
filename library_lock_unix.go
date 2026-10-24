//go:build linux || darwin || freebsd

package main

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockLibrary(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

func rejectReparsePoint(root *os.Root, name string) error { return nil }
