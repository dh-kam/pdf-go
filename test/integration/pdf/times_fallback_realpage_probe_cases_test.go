package pdf_test

import (
	"path/filepath"
	"strings"
)

type realPageProbeTarget struct {
	name       string
	pdfPath    string
	pageNumber int
}

type realPageFontProbeCase struct {
	target        realPageProbeTarget
	skipBaseFonts string
	forcedEnv     map[string]string
}

type realPageLowercaseProbeCase struct {
	target            realPageProbeTarget
	baseFont          string
	singleCodeSpec    string
	topSetCodeSpec    string
	secondaryCodeSpec string
	tertiaryCodeSpec  string
	longTailCodeSpec  string
	nonLowerCodeSpec  string
	forcedEnv         map[string]string
	maxCoverage       float64
	minCoverage       float64
}

type realPageResidualClass string

const (
	realPageResidualClassMixedLowercase realPageResidualClass = "mixed_lowercase"
	realPageResidualClassLongTail       realPageResidualClass = "long_tail"
	realPageResidualClassNonLower       realPageResidualClass = "non_lower"
)

func (c realPageLowercaseProbeCase) combinedCodeSpec() string {
	return combineCodeSkipSpecsForProbe(c.topSetCodeSpec, c.secondaryCodeSpec)
}

func (c realPageLowercaseProbeCase) broadCodeSpec() string {
	return combineCodeSkipSpecsForProbe(c.topSetCodeSpec, c.secondaryCodeSpec, c.longTailCodeSpec)
}

func (c realPageLowercaseProbeCase) hasTertiaryCodeSpec() bool {
	return strings.TrimSpace(c.tertiaryCodeSpec) != ""
}

func (c realPageLowercaseProbeCase) hasLongTailCodeSpec() bool {
	return strings.TrimSpace(c.longTailCodeSpec) != ""
}

func (c realPageLowercaseProbeCase) hasNonLowerCodeSpec() bool {
	return strings.TrimSpace(c.nonLowerCodeSpec) != ""
}

func (c realPageLowercaseProbeCase) broadWithNonLowerCodeSpec() string {
	return combineCodeSkipSpecsForProbe(c.topSetCodeSpec, c.secondaryCodeSpec, c.longTailCodeSpec, c.nonLowerCodeSpec)
}

func (c realPageLowercaseProbeCase) expandedCodeSpec() string {
	return c.broadWithNonLowerCodeSpec()
}

func (c realPageLowercaseProbeCase) dominantResidualClass() realPageResidualClass {
	switch c.target.name {
	case "009_p95_sfrm1095_top6":
		return realPageResidualClassLongTail
	case "009_p109_sfrm1095_top5":
		return realPageResidualClassNonLower
	default:
		return realPageResidualClassMixedLowercase
	}
}

func realPageFontProbeCases() []realPageFontProbeCase {
	return []realPageFontProbeCase{
		{
			target: realPageProbeTarget{
				name:       "004_p3_cmr10",
				pdfPath:    filepath.Join(getSampleDir(), "004-pdflatex-4-pages", "pdflatex-4-pages.pdf"),
				pageNumber: 3,
			},
			skipBaseFonts: "CMR10",
			forcedEnv:     map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "CMR10=Courier"},
		},
		{
			target: realPageProbeTarget{
				name:       "009_p95_sfrm1095",
				pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
				pageNumber: 95,
			},
			skipBaseFonts: "SFRM1095",
			forcedEnv:     map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "SFRM1095=Times-Italic"},
		},
		{
			target: realPageProbeTarget{
				name:       "009_p109_sfrm1095",
				pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
				pageNumber: 109,
			},
			skipBaseFonts: "SFRM1095",
			forcedEnv:     map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "SFRM1095=Times-Italic"},
		},
	}
}

func realPageLowercaseProbeCases() []realPageLowercaseProbeCase {
	return []realPageLowercaseProbeCase{
		{
			target: realPageProbeTarget{
				name:       "004_p3_cmr10_top6",
				pdfPath:    filepath.Join(getSampleDir(), "004-pdflatex-4-pages", "pdflatex-4-pages.pdf"),
				pageNumber: 3,
			},
			baseFont:          "CMR10",
			singleCodeSpec:    "CMR10=101",
			topSetCodeSpec:    "CMR10=101,116,111,110,105,97",
			secondaryCodeSpec: "CMR10=108,104,115,114,100,117",
			tertiaryCodeSpec:  "",
			longTailCodeSpec:  "CMR10=118,99,107,112",
			nonLowerCodeSpec:  "CMR10=44,46,75,41,40,45,69,83,49,50",
			forcedEnv:         map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "CMR10=Courier"},
			maxCoverage:       0.60,
			minCoverage:       0.45,
		},
		{
			target: realPageProbeTarget{
				name:       "009_p95_sfrm1095_top6",
				pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
				pageNumber: 95,
			},
			baseFont:          "SFRM1095",
			singleCodeSpec:    "SFRM1095=101",
			topSetCodeSpec:    "SFRM1095=101,110,105,100,117,109",
			secondaryCodeSpec: "SFRM1095=103,97,115,116,98,114",
			tertiaryCodeSpec:  "",
			longTailCodeSpec:  "SFRM1095=108,111,104,118,99,107,112",
			nonLowerCodeSpec:  "SFRM1095=44,46,75,41,40,45,69,83,228,49,50",
			forcedEnv:         map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "SFRM1095=Times-Italic"},
			maxCoverage:       0.60,
			minCoverage:       0.40,
		},
		{
			target: realPageProbeTarget{
				name:       "009_p109_sfrm1095_top5",
				pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
				pageNumber: 109,
			},
			baseFont:          "SFRM1095",
			singleCodeSpec:    "SFRM1095=101",
			topSetCodeSpec:    "SFRM1095=101,98,110,97,109",
			secondaryCodeSpec: "SFRM1095=111,116,105,114,115,108",
			tertiaryCodeSpec:  "SFRM1095=46",
			longTailCodeSpec:  "SFRM1095=99,117,100,104,107,103,120,112,102",
			nonLowerCodeSpec:  "SFRM1095=65,49,58,44,47,48,50,51,84",
			forcedEnv:         map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "SFRM1095=Times-Italic"},
			maxCoverage:       0.40,
			minCoverage:       0.25,
		},
	}
}
