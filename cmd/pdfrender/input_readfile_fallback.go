//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris)

package main

import "os"

func readPDFInput(path string) ([]byte, func(), error) {
	data, err := os.ReadFile(path)
	return data, func() {}, err
}
