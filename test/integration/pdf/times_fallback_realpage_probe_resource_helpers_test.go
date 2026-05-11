package pdf_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

func pageBaseFontsForProbe(t *testing.T, target realPageProbeTarget) []string {
	t.Helper()

	doc, err := internalpdf.Open(target.pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(target.pageNumber - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	fontsObj := resources.Get(entity.Name("Font"))
	fonts, ok := resolveDictForProbe(doc.XRef(), fontsObj)
	require.True(t, ok)
	require.NotNil(t, fonts)

	seen := make(map[string]struct{})
	for _, key := range fonts.Keys() {
		fontDict, ok := resolveDictForProbe(doc.XRef(), fonts.Get(key))
		require.True(t, ok)
		require.NotNil(t, fontDict)

		base := normalizeProbeBaseFontName(fontDict.Get(entity.Name("BaseFont")))
		if base == "" {
			continue
		}
		seen[base] = struct{}{}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func resolveDictForProbe(xref entity.XRef, obj entity.Object) (*entity.Dict, bool) {
	switch value := obj.(type) {
	case *entity.Dict:
		return value, true
	case entity.Ref:
		resolved, err := xref.Fetch(value)
		if err != nil {
			return nil, false
		}
		return resolveDictForProbe(xref, resolved)
	default:
		return nil, false
	}
}

func normalizeProbeBaseFontName(obj entity.Object) string {
	raw, ok := obj.(entity.Name)
	if !ok {
		return ""
	}
	name := strings.TrimSpace(raw.Value())
	name = strings.TrimPrefix(name, "/")
	if idx := strings.IndexByte(name, '+'); idx >= 0 && idx+1 < len(name) {
		name = name[idx+1:]
	}
	return strings.TrimSpace(name)
}
