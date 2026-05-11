package pdf

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type pageLabelStyle struct {
	prefix string
	style  string
	start  int
}

// WordCount returns number of word tokens in a page (0-based index).
func (d *Document) WordCount(pageIndex int) (int, error) {
	text, err := d.Text(pageIndex)
	if err != nil {
		return 0, err
	}
	return countWordTokens(text), nil
}

// GetWordCount is a Java-parity alias of WordCount that accepts a 1-based page number.
func (d *Document) GetWordCount(pageNumber int) (int, error) {
	if pageNumber <= 0 {
		return 0, fmt.Errorf("invalid page number: %d", pageNumber)
	}
	return d.WordCount(pageNumber - 1)
}

// LabelToPage resolves a page label to a 1-based page number.
// Returns 0 when label is not found or document has no PageLabels.
func (d *Document) LabelToPage(label string) int {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return 0
	}

	lookup, ok := d.pageLabelLookup()
	if !ok {
		return 0
	}
	if page, found := lookup[trimmed]; found {
		return page
	}
	return 0
}

func (d *Document) pageLabelLookup() (map[string]int, bool) {
	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil, false
	}

	pageLabelsObj := catalog.Get(entity.Name("PageLabels"))
	if pageLabelsObj == nil {
		pageLabelsObj = catalog.Get(entity.Name("/PageLabels"))
	}
	if pageLabelsObj == nil {
		return nil, false
	}

	pageLabelsDict, err := d.asDict(pageLabelsObj)
	if err != nil {
		return nil, false
	}

	ranges := make(map[int]pageLabelStyle)
	d.collectPageLabelRanges(pageLabelsDict, 0, ranges)
	if len(ranges) == 0 {
		return nil, false
	}

	pageCount, err := d.doc.PageCount()
	if err != nil || pageCount <= 0 {
		return nil, false
	}

	starts := make([]int, 0, len(ranges))
	for start := range ranges {
		if start >= 0 && start < pageCount {
			starts = append(starts, start)
		}
	}
	if len(starts) == 0 {
		return nil, false
	}
	sort.Ints(starts)

	lookup := make(map[string]int, pageCount)
	for i, start := range starts {
		style := ranges[start]
		end := pageCount
		if i+1 < len(starts) && starts[i+1] < end {
			end = starts[i+1]
		}

		for pageIndex := start; pageIndex < end; pageIndex++ {
			labelText := formatPageLabel(style, pageIndex-start)
			if strings.TrimSpace(labelText) == "" {
				continue
			}
			if _, exists := lookup[labelText]; !exists {
				lookup[labelText] = pageIndex + 1 // Java parity: 1-based page number.
			}
		}
	}

	if len(lookup) == 0 {
		return nil, false
	}
	return lookup, true
}

func (d *Document) collectPageLabelRanges(node *entity.Dict, depth int, out map[int]pageLabelStyle) {
	if node == nil || depth > 32 {
		return
	}

	if numsObj := node.Get(entity.Name("Nums")); numsObj != nil {
		numsArr, err := d.asArray(numsObj)
		if err == nil {
			for i := 0; i+1 < numsArr.Len(); i += 2 {
				start, ok := extractEntityPositiveInt(numsArr.Get(i))
				if !ok {
					continue
				}
				styleDict, dictErr := d.asDict(numsArr.Get(i + 1))
				if dictErr != nil {
					continue
				}
				out[start] = parsePageLabelStyle(styleDict)
			}
		}
	}

	if kidsObj := node.Get(entity.Name("Kids")); kidsObj != nil {
		kids, err := d.asArray(kidsObj)
		if err != nil {
			return
		}
		for i := 0; i < kids.Len(); i++ {
			kidDict, kidErr := d.asDict(kids.Get(i))
			if kidErr != nil {
				continue
			}
			d.collectPageLabelRanges(kidDict, depth+1, out)
		}
	}
}

func parsePageLabelStyle(dict *entity.Dict) pageLabelStyle {
	style := ""
	prefix := ""
	start := 1
	if dict == nil {
		return pageLabelStyle{start: start}
	}

	if s := extractEntityNameOrString(dict.Get(entity.Name("S"))); s != "" {
		style = strings.TrimPrefix(s, "/")
	}
	prefix = extractEntityString(dict.Get(entity.Name("P")))
	if stObj := dict.Get(entity.Name("St")); stObj != nil {
		if st, ok := extractEntityPositiveInt(stObj); ok && st > 0 {
			start = st
		}
	}

	return pageLabelStyle{
		prefix: prefix,
		style:  style,
		start:  start,
	}
}

func extractEntityPositiveInt(obj entity.Object) (int, bool) {
	integer, ok := obj.(*entity.Integer)
	if !ok {
		return 0, false
	}
	value := int(integer.Value())
	if value < 0 {
		return 0, false
	}
	return value, true
}

func formatPageLabel(style pageLabelStyle, offset int) string {
	value := style.start + offset
	if value < 1 {
		value = 1
	}

	text := style.prefix
	if strings.TrimSpace(style.style) != "" {
		text += formatPageLabelOrdinal(style.style, value)
	}
	return text
}

func formatPageLabelOrdinal(style string, value int) string {
	switch style {
	case "D":
		return strconv.Itoa(value)
	case "R":
		return toRoman(value)
	case "r":
		return strings.ToLower(toRoman(value))
	case "A":
		return toAlphabetic(value, true)
	case "a":
		return toAlphabetic(value, false)
	default:
		return strconv.Itoa(value)
	}
}

func toAlphabetic(value int, upper bool) string {
	if value <= 0 {
		return strconv.Itoa(value)
	}

	letters := make([]rune, 0, 4)
	for value > 0 {
		value--
		ch := rune('A' + (value % 26))
		letters = append(letters, ch)
		value /= 26
	}

	for i := 0; i < len(letters)/2; i++ {
		letters[i], letters[len(letters)-1-i] = letters[len(letters)-1-i], letters[i]
	}

	text := string(letters)
	if !upper {
		return strings.ToLower(text)
	}
	return text
}

func toRoman(value int) string {
	if value <= 0 {
		return strconv.Itoa(value)
	}
	pairs := []struct {
		v int
		s string
	}{
		{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"},
		{100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"},
		{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
	}

	var b strings.Builder
	for _, pair := range pairs {
		for value >= pair.v {
			b.WriteString(pair.s)
			value -= pair.v
		}
	}
	return b.String()
}

func countWordTokens(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if isWordTokenRune(r) {
			if !inWord {
				count++
				inWord = true
			}
			continue
		}
		inWord = false
	}
	return count
}

func isWordTokenRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	if r == '_' || r == '\'' {
		return true
	}
	return false
}
