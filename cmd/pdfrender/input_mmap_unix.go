//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package main

import (
	"fmt"
	"os"
	"syscall"
)

func readPDFInput(path string) ([]byte, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	size := info.Size()
	if size <= 0 {
		data, readErr := os.ReadFile(path)
		return data, func() {}, readErr
	}
	if int64(int(size)) != size {
		return nil, nil, fmt.Errorf("file too large to map: %d bytes", size)
	}

	data, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		data, readErr := os.ReadFile(path)
		return data, func() {}, readErr
	}

	cleanup := func() {
		_ = syscall.Munmap(data)
	}
	return data, cleanup, nil
}
