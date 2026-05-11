package pdf_test

import (
	"strconv"
	"strings"
)

func combineCodeSkipSpecsForProbe(specs ...string) string {
	baseFont := ""
	codes := make([]int, 0)
	seen := make(map[int]struct{})

	for _, spec := range specs {
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if baseFont == "" {
			baseFont = parts[0]
		}
		if parts[0] != baseFont {
			continue
		}

		for _, codeText := range strings.Split(parts[1], ",") {
			codeText = strings.TrimSpace(codeText)
			if codeText == "" {
				continue
			}
			code, err := strconv.Atoi(codeText)
			if err != nil {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
	}

	if baseFont == "" || len(codes) == 0 {
		return ""
	}

	items := make([]string, 0, len(codes))
	for _, code := range codes {
		items = append(items, strconv.Itoa(code))
	}
	return baseFont + "=" + strings.Join(items, ",")
}
