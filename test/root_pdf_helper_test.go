package test

import (
	"os"
	"testing"
)

const rootTestPDF = "/workspace/pdf-reader/go-pdf/test.pdf"

func rootTestPDFPath(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(rootTestPDF); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("root debug fixture is absent: %s", rootTestPDF)
		}
		t.Fatalf("stat root debug fixture: %v", err)
	}
	return rootTestPDF
}
