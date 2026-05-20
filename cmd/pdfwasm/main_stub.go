//go:build !js || !wasm

// Package main provides a native placeholder for the browser-only WASM command.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "pdfwasm is a browser WebAssembly target. Build it with GOOS=js GOARCH=wasm.")
	os.Exit(2)
}
