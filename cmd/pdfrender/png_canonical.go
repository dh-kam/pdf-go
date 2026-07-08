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
	"hash/adler32"
	"hash/crc32"
	"image"
	"io"
)

// encodePNGCanonical writes img to w as RGB8 (alpha dropped). The output
// matches pdftoppm/libpng's chunk layout: IHDR + pHYs + IDAT + IEND.
func encodePNGCanonical(w io.Writer, img image.Image) error {
	return encodePNGCanonicalWithCompression(w, img, zlib.BestCompression)
}

func encodePNGCanonicalWithCompression(w io.Writer, img image.Image, compressionLevel int) error {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if err := writePNGCanonicalPrefix(w, width, height); err != nil {
		return err
	}
	return writePNGCanonicalIDATAndEnd(w, img, width, height, compressionLevel)
}

func encodePNGCanonicalRGB8FastWithCompression(w io.Writer, src rgb8ScanlineSource, compressionLevel int) error {
	return encodePNGCanonicalRGB8WithCompression(w, src, compressionLevel)
}

func encodePNGCanonicalRGB8WithCompression(w io.Writer, src rgb8ScanlineSource, compressionLevel int) error {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if err := writePNGCanonicalPrefix(w, width, height); err != nil {
		return err
	}

	var idat []byte
	var err error
	if ws, ok := w.(writeSeeker); ok {
		if compressionLevel == zlib.BestCompression {
			err = writeIDATChunkStreamingFilteredRGB8(ws, src, width, height, compressionLevel)
		} else {
			err = writeIDATChunkStreamingRGB8(ws, src, width, height, compressionLevel)
		}
		if err != nil {
			return err
		}
		return writeChunk(w, "IEND", nil)
	}
	if compressionLevel == zlib.BestCompression {
		idat, err = zlibCompressFilteredRGB8Scanlines(src, width, height, compressionLevel)
	} else {
		idat, err = zlibCompressUnfilteredRGB8Scanlines(src, width, height, compressionLevel)
	}
	if err != nil {
		return err
	}
	if err := writeChunk(w, "IDAT", idat); err != nil {
		return err
	}
	return writeChunk(w, "IEND", nil)
}

type rgb8ScanlineSource interface {
	Bounds() image.Rectangle
	RGB8Scanline(y int, dst []byte) bool
}

func writePNGCanonicalPrefix(w io.Writer, width, height int) error {
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
	return nil
}

func writePNGCanonicalIDATAndEnd(w io.Writer, img image.Image, width, height int, compressionLevel int) error {
	// IDAT — filter rows libpng-style, then zlib-compress.
	//
	// Keep the default best-compression path byte-for-byte stable for the
	// Poppler/libpng parity gate. Fast/no-compression outputs are used for
	// profiling and comparison runs, so stream unfiltered rows directly into
	// zlib there. This avoids both the second full-page filtered-scanline
	// allocation and the five-filter libpng heuristic on every row.
	var idat []byte
	var err error
	if compressionLevel == zlib.BestCompression {
		if ws, ok := w.(writeSeeker); ok {
			if err := writeIDATChunkStreamingFiltered(ws, img, width, height, compressionLevel); err != nil {
				return err
			}
			return writeChunk(w, "IEND", nil)
		}
		idat, err = zlibCompressFilteredScanlines(img, width, height, compressionLevel)
	} else if ws, ok := w.(writeSeeker); ok {
		err = writeIDATChunkStreaming(ws, img, width, height, compressionLevel)
		if err != nil {
			return err
		}
		return writeChunk(w, "IEND", nil)
	} else {
		idat, err = zlibCompressUnfilteredScanlines(img, width, height, compressionLevel)
	}
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
	const bpp = 3
	rowLen := width * bpp
	out := filteredScanlineBuffer{b: make([]byte, 0, height*(rowLen+1))}
	_ = writeFilteredScanlines(&out, img, width, height)
	return out.b
}

func writeFilteredScanlines(w io.Writer, img image.Image, width, height int) error {
	bounds := img.Bounds()
	const bpp = 3
	rowLen := width * bpp
	prev := make([]byte, rowLen)
	curr := make([]byte, rowLen)

	out := make([]byte, rowLen+1)
	cands := make([][]byte, 5)
	for i := range cands {
		cands[i] = make([]byte, rowLen)
	}

	for y := 0; y < height; y++ {
		// Load current row as RGB8.
		off := 0
		switch im := img.(type) {
		case *image.RGBA:
			srcOff := im.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			for x := 0; x < width; x++ {
				curr[off+0] = im.Pix[srcOff]
				curr[off+1] = im.Pix[srcOff+1]
				curr[off+2] = im.Pix[srcOff+2]
				off += bpp
				srcOff += 4
			}
		case *image.Gray:
			srcOff := im.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			for x := 0; x < width; x++ {
				g := im.Pix[srcOff]
				curr[off+0] = g
				curr[off+1] = g
				curr[off+2] = g
				off += bpp
				srcOff++
			}
		default:
			for x := 0; x < width; x++ {
				r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				curr[off+0] = uint8(r >> 8)
				curr[off+1] = uint8(g >> 8)
				curr[off+2] = uint8(b >> 8)
				off += bpp
			}
		}

		// Pick filter with minimum sum-of-signed-bytes-as-unsigned
		// (libpng's heuristic), preserving FIRST-MIN tie break.
		bestF := 0
		bestSum := filterRowAndAbsSum(0, curr, prev, bpp, cands[0])
		for f := 1; f < 5; f++ {
			s := filterRowAndAbsSum(f, curr, prev, bpp, cands[f])
			if s < bestSum {
				bestSum = s
				bestF = f
			}
		}
		out[0] = byte(bestF)
		copy(out[1:], cands[bestF])
		if _, err := w.Write(out); err != nil {
			return err
		}

		// Swap prev/curr.
		prev, curr = curr, prev
	}
	return nil
}

func writeFilteredRGB8Scanlines(w io.Writer, src rgb8ScanlineSource, width, height int) error {
	bounds := src.Bounds()
	const bpp = 3
	rowLen := width * bpp
	prev := make([]byte, rowLen)
	curr := make([]byte, rowLen)

	out := make([]byte, rowLen+1)
	cands := make([][]byte, 5)
	for i := range cands {
		cands[i] = make([]byte, rowLen)
	}

	for y := 0; y < height; y++ {
		if !src.RGB8Scanline(bounds.Min.Y+y, curr) {
			return fmt.Errorf("read RGB8 scanline %d", bounds.Min.Y+y)
		}

		bestF := 0
		bestSum := filterRowAndAbsSum(0, curr, prev, bpp, cands[0])
		for f := 1; f < 5; f++ {
			s := filterRowAndAbsSum(f, curr, prev, bpp, cands[f])
			if s < bestSum {
				bestSum = s
				bestF = f
			}
		}
		out[0] = byte(bestF)
		copy(out[1:], cands[bestF])
		if _, err := w.Write(out); err != nil {
			return err
		}

		prev, curr = curr, prev
	}
	return nil
}

func writeUnfilteredScanlines(w io.Writer, img image.Image, width, height int) error {
	bounds := img.Bounds()
	const bpp = 3
	rowLen := width * bpp
	out := make([]byte, rowLen+1)
	out[0] = 0

	for y := 0; y < height; y++ {
		off := 1
		switch im := img.(type) {
		case *image.RGBA:
			srcOff := im.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			for x := 0; x < width; x++ {
				out[off+0] = im.Pix[srcOff]
				out[off+1] = im.Pix[srcOff+1]
				out[off+2] = im.Pix[srcOff+2]
				off += bpp
				srcOff += 4
			}
		case *image.Gray:
			srcOff := im.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			for x := 0; x < width; x++ {
				g := im.Pix[srcOff]
				out[off+0] = g
				out[off+1] = g
				out[off+2] = g
				off += bpp
				srcOff++
			}
		default:
			for x := 0; x < width; x++ {
				r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				out[off+0] = uint8(r >> 8)
				out[off+1] = uint8(g >> 8)
				out[off+2] = uint8(b >> 8)
				off += bpp
			}
		}
		if _, err := w.Write(out); err != nil {
			return err
		}
	}
	return nil
}

func writeUnfilteredRGB8Scanlines(w io.Writer, src rgb8ScanlineSource, width, height int) error {
	bounds := src.Bounds()
	rowLen := width * 3
	out := make([]byte, rowLen+1)
	out[0] = 0
	for y := 0; y < height; y++ {
		if !src.RGB8Scanline(bounds.Min.Y+y, out[1:]) {
			return fmt.Errorf("read RGB8 scanline %d", bounds.Min.Y+y)
		}
		if _, err := w.Write(out); err != nil {
			return err
		}
	}
	return nil
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

func filterRowAndAbsSum(ftype int, curr, prev []byte, bpp int, dst []byte) int64 {
	rowLen := len(curr)
	var sum int64
	switch ftype {
	case 0: // None
		for i := 0; i < rowLen; i++ {
			v := curr[i]
			dst[i] = v
			sum += absSignedByte(v)
		}
	case 1: // Sub: f(x) = curr[x] - curr[x-bpp]
		for i := 0; i < rowLen; i++ {
			var left byte
			if i >= bpp {
				left = curr[i-bpp]
			}
			v := curr[i] - left
			dst[i] = v
			sum += absSignedByte(v)
		}
	case 2: // Up: f(x) = curr[x] - prev[x]
		for i := 0; i < rowLen; i++ {
			v := curr[i] - prev[i]
			dst[i] = v
			sum += absSignedByte(v)
		}
	case 3: // Average: f(x) = curr[x] - floor((left + up)/2)
		for i := 0; i < rowLen; i++ {
			var left byte
			if i >= bpp {
				left = curr[i-bpp]
			}
			up := prev[i]
			avg := uint16(left) + uint16(up)
			v := curr[i] - byte(avg/2)
			dst[i] = v
			sum += absSignedByte(v)
		}
	case 4: // Paeth
		for i := 0; i < rowLen; i++ {
			var left, upLeft byte
			if i >= bpp {
				left = curr[i-bpp]
				upLeft = prev[i-bpp]
			}
			up := prev[i]
			v := curr[i] - paethPredictor(left, up, upLeft)
			dst[i] = v
			sum += absSignedByte(v)
		}
	}
	return sum
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
		s += absSignedByte(b)
	}
	return s
}

func absSignedByte(b byte) int64 {
	v := int8(b)
	if v < 0 {
		return int64(-int(v))
	}
	return int64(v)
}

// zlibCompress wraps compress/zlib at the requested compression level.
func zlibCompress(data []byte, level int) ([]byte, error) {
	var buf bytesBufferRef
	zw, err := zlib.NewWriterLevel(&buf, level)
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

func zlibCompressFilteredScanlines(img image.Image, width, height int, level int) ([]byte, error) {
	var buf bytesBufferRef
	zw, err := zlib.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}
	if err := writeFilteredScanlines(zw, img, width, height); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.b, nil
}

// zlibCompressUnfilteredScanlines streams filter-0 PNG rows into zlib without
// first materializing the full image payload. This path is intentionally only
// used for fast/no-compression output, not for byte-stable best compression.
func zlibCompressUnfilteredScanlines(img image.Image, width, height int, level int) ([]byte, error) {
	buf := bytesBufferRef{b: make([]byte, 0, initialIDATCapacity(width, height, level))}
	zw, err := zlib.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}
	if err := writeUnfilteredScanlines(zw, img, width, height); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.b, nil
}

func zlibCompressFilteredRGB8Scanlines(src rgb8ScanlineSource, width, height int, level int) ([]byte, error) {
	var buf bytesBufferRef
	zw, err := zlib.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}
	if err := writeFilteredRGB8Scanlines(zw, src, width, height); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.b, nil
}

func zlibCompressUnfilteredRGB8Scanlines(src rgb8ScanlineSource, width, height int, level int) ([]byte, error) {
	buf := bytesBufferRef{b: make([]byte, 0, initialIDATCapacity(width, height, level))}
	zw, err := zlib.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}
	if err := writeUnfilteredRGB8Scanlines(zw, src, width, height); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.b, nil
}

type writeSeeker interface {
	io.Writer
	io.Seeker
}

func writeIDATChunkStreaming(w writeSeeker, img image.Image, width, height int, level int) error {
	start, err := w.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	var hdr [8]byte
	copy(hdr[4:8], "IDAT")
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}

	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte("IDAT"))
	cw := &idatChunkDataWriter{w: w, crc: crc}
	zw, err := zlib.NewWriterLevel(cw, level)
	if err != nil {
		return err
	}
	if err := writeUnfilteredScanlines(zw, img, width, height); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	var tail [4]byte
	binary.BigEndian.PutUint32(tail[:], crc.Sum32())
	if _, err := w.Write(tail[:]); err != nil {
		return err
	}

	end, err := w.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	const maxPNGChunkLen = int64(1<<32 - 1)
	if cw.n > maxPNGChunkLen {
		return fmt.Errorf("IDAT chunk too large: %d bytes", cw.n)
	}
	binary.BigEndian.PutUint32(hdr[0:4], uint32(cw.n))
	if _, err := w.Seek(start, io.SeekStart); err != nil {
		return err
	}
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Seek(end, io.SeekStart)
	return err
}

func writeIDATChunkStreamingFiltered(w writeSeeker, img image.Image, width, height int, level int) error {
	return writeIDATChunkStreamingWith(w, level, func(zw io.Writer) error {
		return writeFilteredScanlines(zw, img, width, height)
	})
}

func writeIDATChunkStreamingRGB8(w writeSeeker, src rgb8ScanlineSource, width, height int, level int) error {
	return writeIDATChunkStreamingWith(w, level, func(zw io.Writer) error {
		return writeUnfilteredRGB8Scanlines(zw, src, width, height)
	})
}

func writeIDATChunkStreamingFilteredRGB8(w writeSeeker, src rgb8ScanlineSource, width, height int, level int) error {
	return writeIDATChunkStreamingWith(w, level, func(zw io.Writer) error {
		return writeFilteredRGB8Scanlines(zw, src, width, height)
	})
}

func writeIDATChunkStreamingWith(w writeSeeker, level int, writePayload func(io.Writer) error) error {
	start, err := w.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	var hdr [8]byte
	copy(hdr[4:8], "IDAT")
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}

	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte("IDAT"))
	cw := &idatChunkDataWriter{w: w, crc: crc}
	var zw io.WriteCloser
	if level == zlib.NoCompression {
		zw = newZlibStoredWriter(cw)
	} else {
		var err error
		zw, err = zlib.NewWriterLevel(cw, level)
		if err != nil {
			return err
		}
	}
	if err := writePayload(zw); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	var tail [4]byte
	binary.BigEndian.PutUint32(tail[:], crc.Sum32())
	if _, err := w.Write(tail[:]); err != nil {
		return err
	}

	end, err := w.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	const maxPNGChunkLen = int64(1<<32 - 1)
	if cw.n > maxPNGChunkLen {
		return fmt.Errorf("IDAT chunk too large: %d bytes", cw.n)
	}
	binary.BigEndian.PutUint32(hdr[0:4], uint32(cw.n))
	if _, err := w.Seek(start, io.SeekStart); err != nil {
		return err
	}
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Seek(end, io.SeekStart)
	return err
}

type idatChunkDataWriter struct {
	w   io.Writer
	crc interface {
		Write([]byte) (int, error)
		Sum32() uint32
	}
	n int64
}

func (w *idatChunkDataWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		_, _ = w.crc.Write(p[:n])
		w.n += int64(n)
	}
	return n, err
}

const maxStoredDeflateBlock = 65535

type zlibStoredWriter struct {
	w     io.Writer
	adler interface {
		Write([]byte) (int, error)
		Sum32() uint32
	}
	pending []byte
	started bool
	closed  bool
}

func newZlibStoredWriter(w io.Writer) *zlibStoredWriter {
	return &zlibStoredWriter{
		w:       w,
		adler:   adler32.New(),
		pending: make([]byte, 0, maxStoredDeflateBlock),
	}
}

func (w *zlibStoredWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("zlib stored writer is closed")
	}
	total := len(p)
	if len(p) > 0 {
		_, _ = w.adler.Write(p)
	}
	for len(p) > 0 {
		n := maxStoredDeflateBlock - len(w.pending)
		if n > len(p) {
			n = len(p)
		}
		w.pending = append(w.pending, p[:n]...)
		p = p[n:]
		if len(w.pending) == maxStoredDeflateBlock {
			if err := w.ensureStarted(); err != nil {
				return total - len(p), err
			}
			if err := w.writeBlock(false, w.pending); err != nil {
				return total - len(p), err
			}
			w.pending = w.pending[:0]
		}
	}
	return total, nil
}

func (w *zlibStoredWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if err := w.ensureStarted(); err != nil {
		return err
	}
	if err := w.writeBlock(true, w.pending); err != nil {
		return err
	}
	var checksum [4]byte
	binary.BigEndian.PutUint32(checksum[:], w.adler.Sum32())
	_, err := w.w.Write(checksum[:])
	return err
}

func (w *zlibStoredWriter) ensureStarted() error {
	if w.started {
		return nil
	}
	w.started = true
	_, err := w.w.Write([]byte{0x78, 0x01})
	return err
}

func (w *zlibStoredWriter) writeBlock(final bool, data []byte) error {
	if len(data) > maxStoredDeflateBlock {
		return fmt.Errorf("stored deflate block too large: %d", len(data))
	}
	var hdr [5]byte
	if final {
		hdr[0] = 0x01
	}
	binary.LittleEndian.PutUint16(hdr[1:3], uint16(len(data)))
	binary.LittleEndian.PutUint16(hdr[3:5], ^uint16(len(data)))
	if _, err := w.w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.w.Write(data)
	return err
}

func initialIDATCapacity(width, height int, level int) int {
	if width <= 0 || height <= 0 {
		return 0
	}
	rawLen := height * (width*3 + 1)
	if rawLen <= 0 {
		return 0
	}
	if level == zlib.NoCompression {
		blocks := rawLen/65535 + 1
		return rawLen + blocks*5 + 16
	}
	if level == zlib.BestSpeed {
		capacity := rawLen / 8
		const minCapacity = 256 << 10
		const maxCapacity = 4 << 20
		if capacity < minCapacity {
			return minCapacity
		}
		if capacity > maxCapacity {
			return maxCapacity
		}
		return capacity
	}
	return 0
}

// zlibBestCompress wraps compress/zlib at BestCompression so the IDAT zlib
// header is 0x78 0xDA, matching libpng's default for full-quality output.
func zlibBestCompress(data []byte) ([]byte, error) {
	return zlibCompress(data, zlib.BestCompression)
}

type filteredScanlineBuffer struct{ b []byte }

func (w *filteredScanlineBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

// bytesBufferRef is a tiny io.Writer that appends to an internal slice — used
// so we can avoid pulling bytes.Buffer into this file's import set (keeps the
// import list to {compress/zlib, encoding/binary, fmt, hash/crc32, image, io}).
type bytesBufferRef struct{ b []byte }

func (w *bytesBufferRef) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
