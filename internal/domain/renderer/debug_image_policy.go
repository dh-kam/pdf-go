package renderer

import (
	"os"
	"strings"
)

func shouldSkipAllImagesForDebug() bool {
	return strings.TrimSpace(os.Getenv("PDF_DEBUG_SKIP_IMAGES")) == "1"
}

func shouldSkipAllXObjectsForDebug() bool {
	return strings.TrimSpace(os.Getenv("PDF_DEBUG_SKIP_XOBJECTS")) == "1"
}
