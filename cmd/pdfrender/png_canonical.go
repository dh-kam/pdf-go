// Package main: canonical PNG encoder.
//
// This file implements a libpng-compatible PNG encoder for RGB8 (no alpha) +
// IHDR + pHYs(150 DPI) + IDAT(zlib best-compression, "first-min sum" filter
// tie-break) + IEND. It exists so the splash backend's parity gate
// (test/integration/splash/parity_test.go — sha256 byte-equal) can match
// pdftoppm's output exactly. Stdlib image/png picks Paeth where libpng picks
// Sub when both produce the same sum-of-absolute-deltas, which would
// otherwise prevent byte-for-byte equality.
package main

import (
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"io"
)

// encodePNGCanonical writes img to w as RGB8 (alpha dropped). The output
// matches pdftoppm/libpng's chunk layout: IHDR + pHYs + IDAT + IEND.
func encodePNGCanonical(w io.Writer, img image.Image) error {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// PNG signature (libpng / RFC 2083 §3.1).
	if _, err := w.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}); err != nil {
		return err
	}

	// IHDR — 13 bytes: width, height, depth=8, color=2 (truecolor), compression=0, filter=0, interlace=0.
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
	ihdr[8] = 8  // bit depth
	ihdr[9] = 2  // color type RGB
	ihdr[10] = 0 // compression
	ihdr[11] = 0 // filter
	ihdr[12] = 0 // interlace
	if err := writeChunk(w, "IHDR", ihdr); err != nil {
		return err
	}

	// pHYs — pdftoppm at 150 DPI emits 5905 px/m on both axes (0x00001711),
	// unit = 1 (meter). 150 DPI * 39.3700787... = 5905.5 → libpng rounds to 5905.
	phys := make([]byte, 9)
	binary.BigEndian.PutUint32(phys[0:4], 5905)
	binary.BigEndian.PutUint32(phys[4:8], 5905)
	phys[8] = 1 // unit: meter
	if err := writeChunk(w, "pHYs", phys); err != nil {
		return err
	}

	// IDAT — filter rows libpng-style, then zlib best-compress.
	raw := buildFilteredScanlines(img, width, height)
	idat, err := zlibBestCompress(raw)
	if err != nil {
		return err
	}
	if err := writeChunk(w, "IDAT", idat); err != nil {
		return err
	}

	// IEND.
	return writeChunk(w, "IEND", nil)
}

// writeChunk emits a PNG chunk: 4-byte length (data only) + 4-byte type + data + 4-byte CRC32 of (type+data).
func writeChunk(w io.Writer, ctype string, data []byte) error {
	if len(ctype) != 4 {
		return fmt.Errorf("chunk type %q must be 4 bytes", ctype)
	}
	hdr := make([]byte, 8)
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(data)))
	copy(hdr[4:8], ctype)
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(ctype))
	_, _ = crc.Write(data)
	tail := make([]byte, 4)
	binary.BigEndian.PutUint32(tail, crc.Sum32())
	_, err := w.Write(tail)
	return err
}

// buildFilteredScanlines extracts RGB rows from img and applies libpng's
// "minimum sum of absolute deltas" filter heuristic with FIRST-MIN tie break
// (None → Sub → Up → Avg → Paeth) — Go's image/png uses LAST-MIN tie break,
// which is why fixtures show 0x04 (Paeth) where libpng picks 0x01 (Sub).
func buildFilteredScanlines(img image.Image, width, height int) []byte {
	bounds := img.Bounds()
	const bpp = 3
	rowLen := width * bpp
	prev := make([]byte, rowLen)
	curr := make([]byte, rowLen)

	out := make([]byte, 0, height*(rowLen+1))
	cands := make([][]byte, 5)
	for i := range cands {
		cands[i] = make([]byte, rowLen)
	}

	for y := 0; y < height; y++ {
		// Load current row as RGB8.
		off := 0
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			curr[off+0] = uint8(r >> 8)
			curr[off+1] = uint8(g >> 8)
			curr[off+2] = uint8(b >> 8)
			off += bpp
		}

		// Build all 5 filter candidates.
		filterRow(0, curr, prev, bpp, cands[0])
		filterRow(1, curr, prev, bpp, cands[1])
		filterRow(2, curr, prev, bpp, cands[2])
		filterRow(3, curr, prev, bpp, cands[3])
		filterRow(4, curr, prev, bpp, cands[4])

		// Pick filter with minimum sum-of-signed-bytes-as-unsigned (libpng's heuristic).
		bestF := 0
		bestSum := absSum(cands[0])
		for f := 1; f < 5; f++ {
			s := absSum(cands[f])
			if s < bestSum {
				bestSum = s
				bestF = f
			}
		}
		out = append(out, byte(bestF))
		out = append(out, cands[bestF]...)

		// Swap prev/curr.
		prev, curr = curr, prev
	}
	return out
}

// filterRow writes the result of filter type ftype applied to curr (with prev
// row context) into dst. Matches the PNG spec §9 (RFC 2083).
func filterRow(ftype int, curr, prev []byte, bpp int, dst []byte) {
	rowLen := len(curr)
	switch ftype {
	case 0: // None
		copy(dst, curr)
	case 1: // Sub: f(x) = curr[x] - curr[x-bpp]
		for i := 0; i < rowLen; i++ {
			var left byte
			if i >= bpp {
				left = curr[i-bpp]
			}
			dst[i] = curr[i] - left
		}
	case 2: // Up: f(x) = curr[x] - prev[x]
		for i := 0; i < rowLen; i++ {
			dst[i] = curr[i] - prev[i]
		}
	case 3: // Average: f(x) = curr[x] - floor((left + up)/2)
		for i := 0; i < rowLen; i++ {
			var left byte
			if i >= bpp {
				left = curr[i-bpp]
			}
			up := prev[i]
			avg := uint16(left) + uint16(up)
			dst[i] = curr[i] - byte(avg/2)
		}
	case 4: // Paeth
		for i := 0; i < rowLen; i++ {
			var left, upLeft byte
			if i >= bpp {
				left = curr[i-bpp]
				upLeft = prev[i-bpp]
			}
			up := prev[i]
			dst[i] = curr[i] - paethPredictor(left, up, upLeft)
		}
	}
}

// paethPredictor implements PNG spec §9.4.
func paethPredictor(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa := abs(p - int(a))
	pb := abs(p - int(b))
	pc := abs(p - int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	default:
		return c
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// absSum mirrors libpng's "minimum sum of absolute deltas" heuristic: each
// filtered byte is treated as signed (-128..127); the sum of |b| over the row
// is the score (libpng's pngwrite.c png_setup_filtered_row + heuristic).
func absSum(row []byte) int64 {
	var s int64
	for _, b := range row {
		v := int8(b)
		if v < 0 {
			s += int64(-int(v))
		} else {
			s += int64(v)
		}
	}
	return s
}

// zlibBestCompress wraps compress/zlib at BestCompression so the IDAT zlib
// header is 0x78 0xDA, matching libpng's default for full-quality output.
func zlibBestCompress(data []byte) ([]byte, error) {
	var buf bytesBufferRef
	zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.b, nil
}

// bytesBufferRef is a tiny io.Writer that appends to an internal slice — used
// so we can avoid pulling bytes.Buffer into this file's import set (keeps the
// import list to {compress/zlib, encoding/binary, fmt, hash/crc32, image, io}).
type bytesBufferRef struct{ b []byte }

func (w *bytesBufferRef) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
