package renderer

import (
	"os"
	"strings"
)

func shouldSkipStrokePathsForDebug() bool {
	return strings.TrimSpace(os.Getenv("PDF_DEBUG_SKIP_STROKE_PATHS")) == "1"
}

func shouldSkipFillPathsForDebug() bool {
	return strings.TrimSpace(os.Getenv("PDF_DEBUG_SKIP_FILL_PATHS")) == "1"
}
