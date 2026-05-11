// extract_cmyk_lut extracts the raw palette indices from the CMYK indexed image in
// 023-cmyk-image/cmyk-image.pdf and writes them as a flat binary (one byte per pixel)
// to stdout.
package main

import (
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// The PDF stream is FlateDecode compressed. This program opens the PDF binary,
// finds the CMYK image stream, decompresses it, and writes raw pixel bytes to stdout.
// The image is 756 x 1008, 8bpc, Indexed, so raw = 756*1008 = 761856 bytes.

func main() {
	pdfPath := "/workspace/pdf-reader/go-pdf/test/integration/pdf/testdata/023-cmyk-image/cmyk-image.pdf"
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading PDF:", err)
		os.Exit(1)
	}

	// Find all "stream\r\n" or "stream\n" markers and try to decompress each,
	// looking for a stream that produces exactly 756*1008 bytes.
	const targetSize = 756 * 1008
	_ = binary.LittleEndian

	searchFor := [][]byte{
		[]byte("stream\r\n"),
		[]byte("stream\n"),
	}

	found := 0
	pos := 0
	for pos < len(data) {
		streamStart := -1
		headerLen := 0
		for _, marker := range searchFor {
			idx := indexAt(data, marker, pos)
			if idx >= 0 && (streamStart < 0 || idx < streamStart) {
				streamStart = idx
				headerLen = len(marker)
			}
		}
		if streamStart < 0 {
			break
		}

		streamData := data[streamStart+headerLen:]
		// Try to decompress with zlib
		zr, err := zlib.NewReader(io.LimitReader(
			byteReader(streamData), int64(len(streamData)),
		))
		if err != nil {
			pos = streamStart + 1
			continue
		}
		decompressed, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			pos = streamStart + 1
			continue
		}
		if len(decompressed) == targetSize {
			found++
			fmt.Fprintf(os.Stderr, "Found stream at offset %d, decompressed size=%d (attempt %d)\n",
				streamStart, len(decompressed), found)
			if found == 1 {
				os.Stdout.Write(decompressed)
				os.Exit(0)
			}
		}
		pos = streamStart + 1
	}

	if found == 0 {
		fmt.Fprintln(os.Stderr, "No stream found with target size", targetSize)
		os.Exit(1)
	}
}

type byteReaderT struct {
	data []byte
	pos  int
}

func byteReader(data []byte) io.Reader {
	return &byteReaderT{data: data}
}

func (r *byteReaderT) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func indexAt(data, needle []byte, start int) int {
	if len(needle) == 0 || start >= len(data) {
		return -1
	}
	for i := start; i <= len(data)-len(needle); i++ {
		if data[i] == needle[0] {
			match := true
			for j := 1; j < len(needle); j++ {
				if data[i+j] != needle[j] {
					match = false
					break
				}
			}
			if match {
				return i
			}
		}
	}
	return -1
}
