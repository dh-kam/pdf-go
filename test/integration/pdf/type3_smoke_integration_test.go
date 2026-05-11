package pdf_test

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

type type3SmokeObject struct {
	number int
	body   []byte
}

func TestType3FontSmokeFallbackRendering(t *testing.T) {
	t.Run("d0 charproc does not break rendering", func(t *testing.T) {
		pdfPath := filepath.Join(t.TempDir(), "type3_d0_smoke.pdf")
		require.NoError(t, os.WriteFile(pdfPath, buildType3SmokePDF(false), 0o644))

		img := renderType3SmokePage(t, pdfPath)
		require.Greater(t, countNonWhitePixels(img), 0)
	})

	t.Run("d1 charproc does not break rendering", func(t *testing.T) {
		pdfPath := filepath.Join(t.TempDir(), "type3_d1_smoke.pdf")
		require.NoError(t, os.WriteFile(pdfPath, buildType3SmokePDF(true), 0o644))

		img := renderType3SmokePage(t, pdfPath)
		require.Greater(t, countNonWhitePixels(img), 0)
	})
}

func renderType3SmokePage(t *testing.T, pdfPath string) image.Image {
	t.Helper()

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err)
	require.Equal(t, 1, pageCount)

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NotNil(t, img)
	require.Greater(t, img.Bounds().Dx(), 0)
	require.Greater(t, img.Bounds().Dy(), 0)

	return img
}

func countNonWhitePixels(img image.Image) int {
	count := 0
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if r != 0xffff || g != 0xffff || b != 0xffff {
				count++
			}
		}
	}
	return count
}

func buildType3SmokePDF(useD1 bool) []byte {
	charProc := []byte("1000 0 d0\n0 0 1000 1000 re\nf\n")
	if useD1 {
		charProc = []byte("1000 0 0 0 1000 1000 d1\n0 0 1000 1000 re\nf\n")
	}

	objects := []type3SmokeObject{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>\n")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>\n")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>\n")},
		{4, type3SmokeStreamObject(4, []byte("BT\n/F1 48 Tf\n72 144 Td\n(A) Tj\nET\n"), nil).body},
		{5, []byte("<< /Type /Font /Subtype /Type3 /Name /F1 /FontBBox [0 0 1000 1000] /FontMatrix [0.001 0 0 0.001 0 0] /CharProcs << /A 6 0 R >> /Encoding << /Type /Encoding /Differences [65 /A] >> /FirstChar 65 /LastChar 65 /Widths [1000] >>\n")},
		{6, type3SmokeStreamObject(6, charProc, nil).body},
	}

	return buildType3SmokeDocument(objects)
}

func type3SmokeStreamObject(number int, data []byte, extraDict []byte) type3SmokeObject {
	dictionary := []byte(fmt.Sprintf("<< /Length %d", len(data)))
	if len(extraDict) > 0 {
		dictionary = append(dictionary, ' ')
		dictionary = append(dictionary, extraDict...)
	}
	dictionary = append(dictionary, []byte(" >>\nstream\n")...)
	dictionary = append(dictionary, data...)
	dictionary = append(dictionary, []byte("endstream\n")...)
	return type3SmokeObject{
		number: number,
		body:   dictionary,
	}
}

func buildType3SmokeDocument(objects []type3SmokeObject) []byte {
	out := []byte("%PDF-1.4\n")
	offsets := []int{0}

	for _, obj := range objects {
		offsets = append(offsets, len(out))
		out = append(out, []byte(fmt.Sprintf("%d 0 obj\n", obj.number))...)
		out = append(out, obj.body...)
		if len(obj.body) == 0 || obj.body[len(obj.body)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, []byte("endobj\n")...)
	}

	xrefOffset := len(out)
	out = append(out, []byte(fmt.Sprintf("xref\n0 %d\n", len(objects)+1))...)
	out = append(out, []byte("0000000000 65535 f \n")...)
	for _, offset := range offsets[1:] {
		out = append(out, []byte(fmt.Sprintf("%010d 00000 n \n", offset))...)
	}
	out = append(out, []byte(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset))...)

	return out
}
