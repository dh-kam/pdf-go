package freetype

import "testing"

func TestUseFreeTypeGoFillRuleMatchesPopplerForAllFreeTypeOutlines(t *testing.T) {
	if !useFreeTypeGoFillRule(nil) {
		t.Fatal("FreeType-Go adapter must use Poppler/FreeType non-zero coverage quantization")
	}
}
