package pdf_test

import (
	"flag"
	"os"
	"strings"
	"testing"
)

const rootTestPDF = "/workspace/pdf-reader/go-pdf/test.pdf"

func TestMain(m *testing.M) {
	if os.Getenv("PDF_RUN_POPPLER_PROBES") != "1" && !hasTestSkipArg(os.Args[1:]) {
		_ = flag.Set("test.skip", "Poppler|Probe|SurfaceAcrossSampleCorpusAllPages|ZeroVsPositiveSubpixelYOffsetContract")
	}
	os.Exit(m.Run())
}

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

func requirePopplerProbeOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv("PDF_RUN_POPPLER_PROBES") != "1" {
		t.Skip("set PDF_RUN_POPPLER_PROBES=1 to run Poppler-backed probe tests")
	}
}

func requireFullSampleCorpusOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv("PDF_RUN_FULL_SAMPLE_CORPUS") != "1" {
		t.Skip("set PDF_RUN_FULL_SAMPLE_CORPUS=1 to run full sample-corpus all-pages tests")
	}
}

func hasTestSkipArg(args []string) bool {
	for _, arg := range args {
		if arg == "-skip" || arg == "-test.skip" || strings.HasPrefix(arg, "-skip=") || strings.HasPrefix(arg, "-test.skip=") {
			return true
		}
	}
	return false
}
