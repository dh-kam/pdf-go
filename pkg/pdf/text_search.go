package pdf

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	domaintext "github.com/dh-kam/pdf-go/internal/domain/text"
)

const (
	searchContextRunes = 20
)

// TextSearchMatch represents one text search hit in one page.
type TextSearchMatch struct {
	Text      string
	Context   string
	PgPoints  []float64
	Bounds    [4]float64
	PageIndex int
	Start     int
	End       int
}

// TextSearchPageResult groups matches found in one page.
type TextSearchPageResult struct {
	Matches   []TextSearchMatch
	PageIndex int
}

// TextSearchResultSet stores grouped search results.
// It mirrors Java-style text search service result access patterns.
type TextSearchResultSet struct {
	Query string
	Pages []TextSearchPageResult
}

// TextSearchOptions configures text search behavior.
type TextSearchOptions struct {
	OnStarted        func(totalPages int)
	OnSearchedInPage func(pageIndex int, matches []TextSearchMatch)
	OnFinished       func(totalMatches int)
	OnDisposed       func()
	OnProgress       func(donePages, totalPages int)
	MaxResults       int
	PageStart        int
	PageEnd          int
	CaseSensitive    bool
	WholeWord        bool
}

type textSearchRange struct {
	start int
	end   int
}

type textSearchItemSpan struct {
	start int
	end   int
	xMin  float64
	yMin  float64
	xMax  float64
	yMax  float64
}

// SearchText searches query text across pages and returns all matches.
func (d *Document) SearchText(query string, options TextSearchOptions) ([]TextSearchMatch, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("search query is required")
	}

	pageCount, err := d.PageCount()
	if err != nil {
		return nil, err
	}

	start, end, err := resolveSearchPageRange(pageCount, options.PageStart, options.PageEnd)
	if err != nil {
		return nil, err
	}

	totalPages := end - start + 1
	if options.OnStarted != nil {
		options.OnStarted(totalPages)
	}
	if options.OnDisposed != nil {
		defer options.OnDisposed()
	}

	results := make([]TextSearchMatch, 0)
	donePages := 0

	for pageIndex := start; pageIndex <= end; pageIndex++ {
		pageMatches, pageErr := d.SearchTextInPage(pageIndex, trimmed, options)
		if pageErr != nil {
			return nil, pageErr
		}

		if len(pageMatches) > 0 {
			results = append(results, pageMatches...)
		}
		if options.OnSearchedInPage != nil {
			options.OnSearchedInPage(pageIndex, append([]TextSearchMatch(nil), pageMatches...))
		}

		donePages++
		if options.OnProgress != nil {
			options.OnProgress(donePages, totalPages)
		}

		if options.MaxResults > 0 && len(results) >= options.MaxResults {
			results = results[:options.MaxResults]
			break
		}
	}

	if options.OnFinished != nil {
		options.OnFinished(len(results))
	}

	return results, nil
}

// SearchTextResultSet searches text and returns grouped page results.
func (d *Document) SearchTextResultSet(query string, options TextSearchOptions) (*TextSearchResultSet, error) {
	matches, err := d.SearchText(query, options)
	if err != nil {
		return nil, err
	}

	grouped := groupMatchesByPage(matches)
	return &TextSearchResultSet{
		Query: strings.TrimSpace(query),
		Pages: grouped,
	}, nil
}

// SearchTextInPage searches query text in one page and returns page matches.
func (d *Document) SearchTextInPage(pageIndex int, query string, options TextSearchOptions) ([]TextSearchMatch, error) {
	if pageIndex < 0 {
		return nil, fmt.Errorf("invalid page index: %d", pageIndex)
	}

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("search query is required")
	}

	layer, err := d.extractTextLayer(pageIndex)
	if err != nil {
		return nil, err
	}
	pageText := layer.Text()

	ranges := findTextSearchRanges(pageText, trimmed, options.CaseSensitive, options.WholeWord, options.MaxResults)
	if len(ranges) == 0 {
		return []TextSearchMatch{}, nil
	}

	pageRunes := []rune(pageText)
	itemSpans := buildTextSearchItemSpans(layer.GetItems())
	matches := make([]TextSearchMatch, 0, len(ranges))
	for _, r := range ranges {
		if r.start < 0 || r.end > len(pageRunes) || r.start >= r.end {
			continue
		}

		bounds, pgPoints, hasGeometry := computeSearchMatchGeometry(r.start, r.end, itemSpans)
		if !hasGeometry {
			bounds = [4]float64{}
			pgPoints = nil
		}

		matches = append(matches, TextSearchMatch{
			PageIndex: pageIndex,
			Start:     r.start,
			End:       r.end,
			Text:      string(pageRunes[r.start:r.end]),
			Context:   buildTextSearchContext(pageRunes, r.start, r.end),
			Bounds:    bounds,
			PgPoints:  pgPoints,
		})
	}

	return matches, nil
}

// SearchedPageCount returns count of pages containing at least one match.
func (r *TextSearchResultSet) SearchedPageCount() int {
	if r == nil {
		return 0
	}
	return len(r.Pages)
}

// SearchedForPosition returns one page's matches by result-set position.
func (r *TextSearchResultSet) SearchedForPosition(position int) []TextSearchMatch {
	if r == nil || position < 0 || position >= len(r.Pages) {
		return nil
	}
	return append([]TextSearchMatch(nil), r.Pages[position].Matches...)
}

// FlattenedMatches returns all matches in grouped page order.
func (r *TextSearchResultSet) FlattenedMatches() []TextSearchMatch {
	if r == nil || len(r.Pages) == 0 {
		return nil
	}

	total := 0
	for i := range r.Pages {
		total += len(r.Pages[i].Matches)
	}
	if total == 0 {
		return nil
	}

	out := make([]TextSearchMatch, 0, total)
	for i := range r.Pages {
		out = append(out, r.Pages[i].Matches...)
	}
	return out
}

func resolveSearchPageRange(pageCount, start, end int) (int, int, error) {
	if pageCount <= 0 {
		return 0, -1, nil
	}

	resolvedStart := start
	if resolvedStart < 0 {
		resolvedStart = 0
	}
	resolvedEnd := end
	if resolvedEnd < 0 {
		resolvedEnd = pageCount - 1
	}

	if resolvedStart < 0 || resolvedStart >= pageCount {
		return 0, 0, fmt.Errorf("invalid search start page: %d", start)
	}
	if resolvedEnd < 0 || resolvedEnd >= pageCount {
		return 0, 0, fmt.Errorf("invalid search end page: %d", end)
	}
	if resolvedStart > resolvedEnd {
		return 0, 0, fmt.Errorf("invalid page range: %d > %d", resolvedStart, resolvedEnd)
	}

	return resolvedStart, resolvedEnd, nil
}

func findTextSearchRanges(text, query string, caseSensitive, wholeWord bool, maxResults int) []textSearchRange {
	textRunes := []rune(text)
	queryRunes := []rune(query)
	if len(textRunes) == 0 || len(queryRunes) == 0 || len(queryRunes) > len(textRunes) {
		return nil
	}

	candidateText := textRunes
	candidateQuery := queryRunes
	if !caseSensitive {
		candidateText = foldToLowerRunes(textRunes)
		candidateQuery = foldToLowerRunes(queryRunes)
	}

	limit := maxResults
	if limit <= 0 {
		limit = len(textRunes)
	}

	out := make([]textSearchRange, 0)
	lastStart := len(candidateText) - len(candidateQuery)
	for i := 0; i <= lastStart; i++ {
		if !matchRuneSlice(candidateText[i:i+len(candidateQuery)], candidateQuery) {
			continue
		}
		if wholeWord && !isWholeWordBoundary(textRunes, i, i+len(candidateQuery)) {
			continue
		}

		out = append(out, textSearchRange{
			start: i,
			end:   i + len(candidateQuery),
		})
		if len(out) >= limit {
			break
		}
	}

	return out
}

func foldToLowerRunes(input []rune) []rune {
	out := make([]rune, len(input))
	for i, r := range input {
		out[i] = unicode.ToLower(r)
	}
	return out
}

func matchRuneSlice(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isWholeWordBoundary(text []rune, start, end int) bool {
	if start > 0 && isWordRune(text[start-1]) {
		return false
	}
	if end < len(text) && isWordRune(text[end]) {
		return false
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

func buildTextSearchContext(text []rune, start, end int) string {
	if start < 0 || end > len(text) || start >= end {
		return ""
	}

	from := start - searchContextRunes
	if from < 0 {
		from = 0
	}
	to := end + searchContextRunes
	if to > len(text) {
		to = len(text)
	}

	var sb strings.Builder
	if from > 0 {
		sb.WriteString("...")
	}
	sb.WriteString(string(text[from:to]))
	if to < len(text) {
		sb.WriteString("...")
	}
	return strings.TrimSpace(normalizeWhitespace(sb.String()))
}

func groupMatchesByPage(matches []TextSearchMatch) []TextSearchPageResult {
	if len(matches) == 0 {
		return nil
	}

	out := make([]TextSearchPageResult, 0)
	pagePos := make(map[int]int)

	for _, m := range matches {
		pos, ok := pagePos[m.PageIndex]
		if !ok {
			pagePos[m.PageIndex] = len(out)
			out = append(out, TextSearchPageResult{
				PageIndex: m.PageIndex,
				Matches:   []TextSearchMatch{m},
			})
			continue
		}
		out[pos].Matches = append(out[pos].Matches, m)
	}

	return out
}

func buildTextSearchItemSpans(items []domaintext.TextItem) []textSearchItemSpan {
	if len(items) == 0 {
		return nil
	}

	out := make([]textSearchItemSpan, 0, len(items))
	cursor := 0

	for _, item := range items {
		length := len([]rune(item.Text))
		if length == 0 {
			continue
		}

		out = append(out, textSearchItemSpan{
			start: cursor,
			end:   cursor + length,
			xMin:  float64(item.BoundingBox.Min.X),
			yMin:  float64(item.BoundingBox.Min.Y),
			xMax:  float64(item.BoundingBox.Max.X),
			yMax:  float64(item.BoundingBox.Max.Y),
		})
		cursor += length
	}

	return out
}

func computeSearchMatchGeometry(start, end int, spans []textSearchItemSpan) ([4]float64, []float64, bool) {
	if start >= end || len(spans) == 0 {
		return [4]float64{}, nil, false
	}

	xMin := math.MaxFloat64
	yMin := math.MaxFloat64
	xMax := -math.MaxFloat64
	yMax := -math.MaxFloat64
	found := false

	for _, span := range spans {
		if span.end <= start || span.start >= end {
			continue
		}
		if span.xMin < xMin {
			xMin = span.xMin
		}
		if span.yMin < yMin {
			yMin = span.yMin
		}
		if span.xMax > xMax {
			xMax = span.xMax
		}
		if span.yMax > yMax {
			yMax = span.yMax
		}
		found = true
	}

	if !found {
		return [4]float64{}, nil, false
	}

	bounds := [4]float64{xMin, yMin, xMax, yMax}
	pgPoints := []float64{
		xMin, yMin,
		xMax, yMin,
		xMax, yMax,
		xMin, yMax,
	}

	return bounds, pgPoints, true
}
