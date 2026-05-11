package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePageSpec(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{name: "empty", input: "", want: nil},
		{name: "single", input: "2", want: []int{2}},
		{name: "mixed", input: "1,3,5-7", want: []int{1, 3, 5, 6, 7}},
		{name: "spaces", input: " 2-4, 6 ", want: []int{2, 3, 4, 6}},
		{name: "invalid token", input: "x", wantErr: true},
		{name: "reverse", input: "7-1", wantErr: true},
		{name: "zero", input: "0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePageSpec(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProcessPDF_OutputToFile(t *testing.T) {
	pdfPath := testPDFPath(t)
	outputPath := filepath.Join(t.TempDir(), "text.txt")

	err := processPDF(pdfPath, nil, Options{Output: outputPath})
	require.NoError(t, err)

	b, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Greater(t, len(b), 0)
}

func TestProcessPDF_PageOutOfRange(t *testing.T) {
	pdfPath := testPDFPath(t)
	err := processPDF(pdfPath, []int{99999}, Options{})
	require.Error(t, err)
	require.ErrorContains(t, err, "out of range")
}

func TestProcessPDF_PositionOutput(t *testing.T) {
	pdfPath := testPDFPath(t)
	outputPath := filepath.Join(t.TempDir(), "text.json")

	err := processPDF(pdfPath, nil, Options{Output: outputPath, WithPositions: true})
	require.NoError(t, err)

	_, err = os.Stat(outputPath)
	require.NoError(t, err)
}

func testPDFPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(filename), "..", "..", "test", "testdata", "sample-files", "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")
}
