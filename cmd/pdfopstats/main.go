package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/renderer"
	pdfusecase "github.com/dh-kam/pdf-go/internal/usecase/pdf"

	_ "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

type reportRow struct {
	PDF         string
	Page        int
	Exact       float64
	Exact100    bool
	Error       string
	PopplerPNG  string
	OursPNG     string
	DiffPNG     string
	Matched     string
	Total       string
	Width       string
	Height      string
	ResourceErr string
	ParseErr    string
}

type pageStats struct {
	Row              reportRow
	OpTotal          int
	Ops              map[string]int
	TextOps          int
	PathOps          int
	ColorOps         int
	ImageDoOps       int
	InlineImageOps   int
	ShadingOps       int
	ResourceFont     int
	ResourceXObject  int
	ResourceImage    int
	ResourceForm     int
	ResourceExtG     int
	ResourcePattern  int
	ResourceShading  int
	ColorSpaces      []string
	ImageFilters     []string
	ImageColorSpaces []string
	ImageMasks       []string
	ActiveImageMasks []string
	ExtGStateSignals []string
	ResourceUses     map[string]int
	OpcodePaths      map[string]int
	Subsystem        string
	Signals          []string
	PopplerPath      string
}

func (row reportRow) badPixels() int64 {
	total, err1 := strconv.ParseInt(strings.TrimSpace(row.Total), 10, 64)
	matched, err2 := strconv.ParseInt(strings.TrimSpace(row.Matched), 10, 64)
	if err1 != nil || err2 != nil || total < matched {
		return 0
	}
	return total - matched
}

func main() {
	reportPath := flag.String("report", "", "pdfcompare report.csv")
	top := flag.Int("top", 40, "number of worst non-exact pages to inspect; use 0 for all")
	format := flag.String("format", "markdown", "output format: markdown or csv")
	tracePDF := flag.String("trace-pdf", "", "PDF path to trace by page")
	tracePage := flag.Int("trace-page", 0, "1-based page number to trace")
	traceMax := flag.Int("trace-max", 400, "maximum trace lines to print")
	traceMode := flag.String("trace-mode", "resources", "trace mode: resources, ops, or all")
	popplerRoot := flag.String("poppler-root", defaultPopplerRoot(), "Poppler source root used to qualify poppler_path output")
	flag.Parse()
	popplerSourceRoot = strings.TrimRight(*popplerRoot, "/")

	if *tracePDF != "" {
		if *tracePage <= 0 {
			fmt.Fprintln(os.Stderr, "-trace-page must be positive")
			os.Exit(2)
		}
		if err := writePageTrace(os.Stdout, *tracePDF, *tracePage, *traceMax, *traceMode); err != nil {
			fmt.Fprintf(os.Stderr, "trace page: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *reportPath == "" {
		fmt.Fprintln(os.Stderr, "usage: pdfopstats -report tmp/.../report.csv [-top 40] [-format markdown|csv]")
		fmt.Fprintln(os.Stderr, "   or: pdfopstats -trace-pdf file.pdf -trace-page N [-trace-mode resources|all]")
		os.Exit(2)
	}

	rows, err := readReport(*reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read report: %v\n", err)
		os.Exit(1)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Exact < rows[j].Exact
	})
	if *top > 0 && len(rows) > *top {
		rows = rows[:*top]
	}

	stats := make([]pageStats, 0, len(rows))
	docs := map[string]*entity.Document{}
	defer func() {
		for _, doc := range docs {
			_ = doc.Close()
		}
	}()

	for _, row := range rows {
		doc := docs[row.PDF]
		if doc == nil {
			var err error
			doc, err = pdfusecase.Open(row.PDF)
			if err != nil {
				stats = append(stats, pageStats{Row: row, Subsystem: "open-error", Signals: []string{err.Error()}})
				continue
			}
			docs[row.PDF] = doc
		}

		stat := inspectPage(doc, row)
		stats = append(stats, stat)
	}

	switch *format {
	case "csv":
		if err := writeStatsCSV(os.Stdout, stats); err != nil {
			fmt.Fprintf(os.Stderr, "write csv: %v\n", err)
			os.Exit(1)
		}
	case "markdown":
		writeMarkdown(os.Stdout, stats)
	default:
		fmt.Fprintf(os.Stderr, "unknown format: %s\n", *format)
		os.Exit(2)
	}
}

func readReport(path string) ([]reportRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	header := map[string]int{}
	for i, name := range rows[0] {
		header[name] = i
	}

	get := func(row []string, key string) string {
		idx, ok := header[key]
		if !ok || idx >= len(row) {
			return ""
		}
		return row[idx]
	}

	out := make([]reportRow, 0, len(rows)-1)
	for _, raw := range rows[1:] {
		if strings.TrimSpace(get(raw, "error")) != "" {
			continue
		}
		exact100 := strings.EqualFold(get(raw, "exact100"), "true")
		if exact100 {
			continue
		}
		page, err := strconv.Atoi(get(raw, "page"))
		if err != nil {
			continue
		}
		exact, err := strconv.ParseFloat(get(raw, "exact_percent"), 64)
		if err != nil {
			continue
		}
		out = append(out, reportRow{
			PDF:        get(raw, "pdf"),
			Page:       page,
			Exact:      exact,
			Exact100:   exact100,
			Error:      get(raw, "error"),
			PopplerPNG: get(raw, "poppler_png"),
			OursPNG:    get(raw, "ours_png"),
			DiffPNG:    get(raw, "diff_png"),
			Matched:    get(raw, "matched_pixels"),
			Total:      get(raw, "total_pixels"),
			Width:      get(raw, "width"),
			Height:     get(raw, "height"),
		})
	}
	return out, nil
}

func inspectPage(doc *entity.Document, row reportRow) pageStats {
	stat := pageStats{Row: row, Ops: map[string]int{}, ResourceUses: map[string]int{}, OpcodePaths: map[string]int{}}
	page, err := doc.GetPage(row.Page - 1)
	if err != nil {
		stat.Subsystem = "page-error"
		stat.Signals = append(stat.Signals, err.Error())
		return stat
	}

	var pageResources *entity.Dict
	seenForms := map[*entity.Stream]bool{}
	seenResources := map[*entity.Dict]bool{}
	if resources, err := page.Resources(); err == nil {
		pageResources = resources
		inspectResources(resources, doc.XRef(), &stat, seenForms, seenResources)
	} else {
		stat.Row.ResourceErr = err.Error()
	}

	contents, err := page.Contents()
	if err != nil {
		stat.Row.ParseErr = err.Error()
		stat.Subsystem = inferSubsystem(&stat)
		return stat
	}

	activeSeenForms := map[*entity.Stream]bool{}
	activeSeenOpcodeForms := map[*entity.Stream]bool{}
	activeSeenOpcodePatterns := map[*entity.Stream]bool{}
	for i, content := range contents {
		stream, ok := content.(*entity.Stream)
		if !ok {
			continue
		}
		parseContentStream(doc.XRef(), stream, &stat)
		if pageResources != nil {
			inspectOpcodePaths(doc.XRef(), stream, pageResources, fmt.Sprintf("page.contents[%d]", i), &stat, activeSeenOpcodeForms, activeSeenOpcodePatterns)
		}
		if pageResources != nil {
			inspectActiveImageMasks(doc.XRef(), stream, pageResources, &stat, activeSeenForms)
		}
	}
	stat.ActiveImageMasks = sortedUnique(stat.ActiveImageMasks)

	stat.Subsystem = inferSubsystem(&stat)
	stat.PopplerPath = inferPopplerPath(&stat)
	return stat
}

func inspectOpcodePaths(xref entity.XRef, stream *entity.Stream, resources *entity.Dict, context string, stat *pageStats, seenForms map[*entity.Stream]bool, seenPatterns map[*entity.Stream]bool) {
	data, err := stream.Decode()
	if err != nil {
		return
	}
	eval := renderer.NewEvaluator(xref)
	ops, err := eval.ParseContentOperators(data)
	if err != nil {
		return
	}
	for _, op := range ops {
		if key := opcodePathKey(op, resources, xref); key != "" {
			stat.OpcodePaths[context+"/"+key]++
		}
		switch op.Opcode {
		case "Do":
			traceOpcodePathXObject(op, resources, xref, context, stat, seenForms, seenPatterns)
		case "SCN", "scn":
			traceOpcodePathPattern(op, resources, xref, context, stat, seenForms, seenPatterns)
		}
	}
}

func traceOpcodePathXObject(op renderer.Operator, resources *entity.Dict, xref entity.XRef, context string, stat *pageStats, seenForms map[*entity.Stream]bool, seenPatterns map[*entity.Stream]bool) {
	if len(op.Operands) == 0 {
		return
	}
	name, ok := op.Operands[0].(entity.Name)
	if !ok {
		return
	}
	stream, _ := resolve(lookupResourceObject(resources, xref, "XObject", name), xref).(*entity.Stream)
	if stream == nil || nameValue(stream.Dict().Get(entity.Name("Subtype"))) != "Form" {
		return
	}
	if seenForms[stream] {
		return
	}
	seenForms[stream] = true
	formResources := asDict(resolve(stream.Dict().Get(entity.Name("Resources")), xref), xref)
	if formResources == nil {
		formResources = resources
	}
	inspectOpcodePaths(xref, stream, formResources, context+"/form:"+name.Value(), stat, seenForms, seenPatterns)
}

func traceOpcodePathPattern(op renderer.Operator, resources *entity.Dict, xref entity.XRef, context string, stat *pageStats, seenForms map[*entity.Stream]bool, seenPatterns map[*entity.Stream]bool) {
	name, ok := lastNameOperand(op.Operands)
	if !ok {
		return
	}
	stream, _ := resolve(lookupResourceObject(resources, xref, "Pattern", entity.Name(name)), xref).(*entity.Stream)
	if stream == nil {
		return
	}
	if shortObject(resolve(stream.Dict().Get(entity.Name("PatternType")), xref), xref) != "1" {
		return
	}
	if seenPatterns[stream] {
		return
	}
	seenPatterns[stream] = true
	patternResources := asDict(resolve(stream.Dict().Get(entity.Name("Resources")), xref), xref)
	if patternResources == nil {
		patternResources = resources
	}
	inspectOpcodePaths(xref, stream, patternResources, context+"/pattern:"+name, stat, seenForms, seenPatterns)
}

func parseContentStream(xref entity.XRef, stream *entity.Stream, stat *pageStats) {
	data, err := stream.Decode()
	if err != nil {
		stat.Row.ParseErr = err.Error()
		return
	}
	eval := renderer.NewEvaluator(xref)
	ops, err := eval.ParseContentOperators(data)
	if err != nil {
		stat.Row.ParseErr = err.Error()
		return
	}
	for _, op := range ops {
		stat.Ops[op.Opcode]++
		stat.OpTotal++
		classifyOperator(op.Opcode, stat)
		trackResourceUse(op, stat)
	}
}

func inspectActiveImageMasks(xref entity.XRef, stream *entity.Stream, resources *entity.Dict, stat *pageStats, seenForms map[*entity.Stream]bool) {
	data, err := stream.Decode()
	if err != nil {
		return
	}
	eval := renderer.NewEvaluator(xref)
	ops, err := eval.ParseContentOperators(data)
	if err != nil {
		return
	}
	for _, op := range ops {
		if op.Opcode != "Do" {
			continue
		}
		resourceName, ok := lastNameOperand(op.Operands)
		if !ok {
			continue
		}
		name := entity.Name(resourceName)
		xobject, _ := resolve(lookupResourceObject(resources, xref, "XObject", name), xref).(*entity.Stream)
		if xobject == nil {
			continue
		}
		dict := xobject.Dict()
		switch nameValue(dict.Get(entity.Name("Subtype"))) {
		case "Image":
			stat.ActiveImageMasks = append(stat.ActiveImageMasks, classifyImageMaskSignals(name, dict, xref)...)
		case "Form":
			if seenForms[xobject] {
				continue
			}
			seenForms[xobject] = true
			formResources := asDict(resolve(dict.Get(entity.Name("Resources")), xref), xref)
			if formResources == nil {
				formResources = resources
			}
			inspectActiveImageMasks(xref, xobject, formResources, stat, seenForms)
		}
	}
}

type traceContext struct {
	doc          *entity.Document
	xref         entity.XRef
	w            io.Writer
	max          int
	printed      int
	mode         string
	seenForms    map[*entity.Stream]bool
	seenPatterns map[*entity.Stream]bool
}

func writePageTrace(w io.Writer, pdfPath string, pageNumber, maxLines int, mode string) error {
	doc, err := pdfusecase.Open(pdfPath)
	if err != nil {
		return err
	}
	defer doc.Close()

	page, err := doc.GetPage(pageNumber - 1)
	if err != nil {
		return err
	}
	resources, err := page.Resources()
	if err != nil {
		return err
	}
	contents, err := page.Contents()
	if err != nil {
		return err
	}

	ctx := &traceContext{
		doc:          doc,
		xref:         doc.XRef(),
		w:            w,
		max:          maxLines,
		mode:         mode,
		seenForms:    map[*entity.Stream]bool{},
		seenPatterns: map[*entity.Stream]bool{},
	}
	fmt.Fprintf(w, "# opcode trace\n\n- pdf: `%s`\n- page: %d\n- mode: %s\n\n", pdfPath, pageNumber, mode)
	for i, content := range contents {
		stream, ok := content.(*entity.Stream)
		if !ok {
			continue
		}
		if err := ctx.traceStream(fmt.Sprintf("page.contents[%d]", i), stream, resources, 0); err != nil {
			return err
		}
		if ctx.limitReached() {
			break
		}
	}
	if ctx.limitReached() {
		fmt.Fprintf(w, "\n(trace truncated at %d lines)\n", ctx.max)
	}
	return nil
}

func (ctx *traceContext) traceStream(label string, stream *entity.Stream, resources *entity.Dict, depth int) error {
	data, err := stream.Decode()
	if err != nil {
		ctx.printTrace(depth, -1, "decode-error", err.Error())
		return nil
	}
	eval := renderer.NewEvaluator(ctx.xref)
	ops, err := eval.ParseContentOperators(data)
	if err != nil {
		ctx.printTrace(depth, -1, "parse-error", err.Error())
		return nil
	}
	ctx.printTrace(depth, -1, "stream", label)
	for index, op := range ops {
		if ctx.limitReached() {
			return nil
		}
		if ctx.shouldPrintOperator(op) {
			ctx.printTrace(depth, index, op.Opcode, ctx.describeOperator(op, resources))
		}
		if op.Opcode == "Do" {
			ctx.traceXObject(op, resources, depth)
		}
		if op.Opcode == "SCN" || op.Opcode == "scn" {
			ctx.tracePattern(op, resources, depth)
		}
	}
	return nil
}

func (ctx *traceContext) traceXObject(op renderer.Operator, resources *entity.Dict, depth int) {
	if len(op.Operands) == 0 || ctx.limitReached() {
		return
	}
	name, ok := op.Operands[0].(entity.Name)
	if !ok {
		return
	}
	stream := ctx.lookupXObject(resources, name)
	if stream == nil {
		return
	}
	dict := stream.Dict()
	if nameValue(dict.Get(entity.Name("Subtype"))) != "Form" {
		return
	}
	if ctx.seenForms[stream] {
		ctx.printTrace(depth+1, -1, "form-skip", fmt.Sprintf("/%s already traced", name.Value()))
		return
	}
	ctx.seenForms[stream] = true
	formResources := asDict(resolve(dict.Get(entity.Name("Resources")), ctx.xref), ctx.xref)
	if formResources == nil {
		formResources = resources
	}
	_ = ctx.traceStream("form /"+name.Value(), stream, formResources, depth+1)
}

func (ctx *traceContext) tracePattern(op renderer.Operator, resources *entity.Dict, depth int) {
	if len(op.Operands) == 0 || ctx.limitReached() {
		return
	}
	name, ok := op.Operands[len(op.Operands)-1].(entity.Name)
	if !ok {
		return
	}
	obj := resolve(ctx.lookupResourceObject(resources, "Pattern", name), ctx.xref)
	stream, ok := obj.(*entity.Stream)
	if !ok {
		return
	}
	dict := stream.Dict()
	if shortObject(resolve(dict.Get(entity.Name("PatternType")), ctx.xref), ctx.xref) != "1" {
		return
	}
	if ctx.seenPatterns[stream] {
		ctx.printTrace(depth+1, -1, "pattern-skip", fmt.Sprintf("/%s already traced", name.Value()))
		return
	}
	ctx.seenPatterns[stream] = true
	patternResources := asDict(resolve(dict.Get(entity.Name("Resources")), ctx.xref), ctx.xref)
	if patternResources == nil {
		patternResources = resources
	}
	label := fmt.Sprintf(
		"pattern /%s bbox=%s matrix=%s xstep=%s ystep=%s",
		name.Value(),
		shortObject(resolve(dict.Get(entity.Name("BBox")), ctx.xref), ctx.xref),
		shortObject(resolve(dict.Get(entity.Name("Matrix")), ctx.xref), ctx.xref),
		shortObject(resolve(dict.Get(entity.Name("XStep")), ctx.xref), ctx.xref),
		shortObject(resolve(dict.Get(entity.Name("YStep")), ctx.xref), ctx.xref),
	)
	_ = ctx.traceStream(label, stream, patternResources, depth+1)
}

func (ctx *traceContext) shouldPrintOperator(op renderer.Operator) bool {
	if ctx.mode == "all" {
		return true
	}
	if ctx.mode == "ops" {
		switch op.Opcode {
		case "q", "Q", "cm", "gs", "Do", "BI", "ID", "EI", "CS", "cs", "SC", "SCN", "sc", "scn", "Tf", "Tr", "Tj", "TJ", "'", "\"", "sh", "W", "W*", "S", "s", "f", "f*", "B", "B*", "b", "b*", "n":
			return true
		default:
			return false
		}
	}
	switch op.Opcode {
	case "gs", "Do", "BI", "ID", "EI", "CS", "cs", "SCN", "scn", "Tf", "Tr", "Tj", "TJ", "'", "\"", "sh":
		return true
	default:
		return false
	}
}

func (ctx *traceContext) printTrace(depth, index int, opcode, detail string) {
	if ctx.limitReached() {
		return
	}
	indent := strings.Repeat("  ", depth)
	if index >= 0 {
		fmt.Fprintf(ctx.w, "%s- #%05d `%s` %s\n", indent, index, opcode, detail)
	} else {
		fmt.Fprintf(ctx.w, "%s- `%s` %s\n", indent, opcode, detail)
	}
	ctx.printed++
}

func (ctx *traceContext) limitReached() bool {
	return ctx.max > 0 && ctx.printed >= ctx.max
}

func (ctx *traceContext) describeOperator(op renderer.Operator, resources *entity.Dict) string {
	operands := formatOperands(op.Operands, 6)
	switch op.Opcode {
	case "gs":
		if len(op.Operands) == 0 {
			return operands
		}
		name, ok := op.Operands[0].(entity.Name)
		if !ok {
			return operands
		}
		return fmt.Sprintf("%s => %s", operands, ctx.describeExtGState(resources, name))
	case "Do":
		if len(op.Operands) == 0 {
			return operands
		}
		name, ok := op.Operands[0].(entity.Name)
		if !ok {
			return operands
		}
		return fmt.Sprintf("%s => %s", operands, ctx.describeXObject(resources, name))
	case "CS", "cs":
		if len(op.Operands) == 0 {
			return operands
		}
		name, ok := op.Operands[0].(entity.Name)
		if !ok {
			return operands
		}
		return fmt.Sprintf("%s => %s", operands, ctx.describeNamedColorSpace(resources, name))
	case "SCN", "scn":
		pattern := ctx.describePatternOperand(resources, op.Operands)
		if pattern != "" {
			return operands + " => " + pattern
		}
		return operands
	case "Tf":
		if len(op.Operands) == 0 {
			return operands
		}
		name, ok := op.Operands[0].(entity.Name)
		if !ok {
			return operands
		}
		return fmt.Sprintf("%s => %s", operands, ctx.describeFont(resources, name))
	case "sh":
		if len(op.Operands) == 0 {
			return operands
		}
		name, ok := op.Operands[0].(entity.Name)
		if !ok {
			return operands
		}
		return fmt.Sprintf("%s => %s", operands, ctx.describeShading(resources, name))
	default:
		return operands
	}
}

func (ctx *traceContext) describeExtGState(resources *entity.Dict, name entity.Name) string {
	dict := ctx.lookupResourceDict(resources, "ExtGState", name)
	if dict == nil {
		return "ExtGState missing"
	}
	parts := make([]string, 0, 8)
	for _, key := range []string{"ca", "CA", "BM", "SMask", "AIS", "SA", "TR", "TR2"} {
		if obj := dict.Get(entity.Name(key)); obj != nil {
			parts = append(parts, key+"="+shortObject(resolve(obj, ctx.xref), ctx.xref))
		}
	}
	if len(parts) == 0 {
		return "ExtGState empty"
	}
	return "ExtGState " + strings.Join(parts, " ")
}

func (ctx *traceContext) describeXObject(resources *entity.Dict, name entity.Name) string {
	stream := ctx.lookupXObject(resources, name)
	if stream == nil {
		return "XObject missing"
	}
	dict := stream.Dict()
	subtype := nameValue(dict.Get(entity.Name("Subtype")))
	switch subtype {
	case "Image":
		parts := []string{
			"Image",
			"w=" + shortObject(dict.Get(entity.Name("Width")), ctx.xref),
			"h=" + shortObject(dict.Get(entity.Name("Height")), ctx.xref),
			"bpc=" + shortObject(dict.Get(entity.Name("BitsPerComponent")), ctx.xref),
			"filter=" + describeFilter(dict.Get(entity.Name("Filter"))),
			"cs=" + describeColorSpace(resolve(dict.Get(entity.Name("ColorSpace")), ctx.xref), ctx.xref),
		}
		for _, key := range []string{"Interpolate", "Decode", "DecodeParms", "Mask", "SMask"} {
			if obj := dict.Get(entity.Name(key)); obj != nil {
				parts = append(parts, strings.ToLower(key)+"="+shortObject(resolve(obj, ctx.xref), ctx.xref))
			}
		}
		for _, signal := range classifyImageMaskSignals(name, dict, ctx.xref) {
			parts = append(parts, strings.TrimPrefix(signal, name.Value()+":"))
		}
		return strings.Join(parts, " ")
	case "Form":
		parts := []string{
			"Form",
			"bbox=" + shortObject(resolve(dict.Get(entity.Name("BBox")), ctx.xref), ctx.xref),
			"matrix=" + shortObject(resolve(dict.Get(entity.Name("Matrix")), ctx.xref), ctx.xref),
		}
		if group := resolve(dict.Get(entity.Name("Group")), ctx.xref); group != nil {
			parts = append(parts, "group="+shortObject(group, ctx.xref))
		}
		return strings.Join(parts, " ")
	default:
		return "XObject subtype=" + subtype
	}
}

func (ctx *traceContext) describeNamedColorSpace(resources *entity.Dict, name entity.Name) string {
	switch name.Value() {
	case "DeviceGray", "DeviceRGB", "DeviceCMYK", "Pattern":
		return name.Value()
	}
	dict := asDict(resolve(resourceCategory(resources, ctx.xref, "ColorSpace"), ctx.xref), ctx.xref)
	if dict == nil {
		return "ColorSpace missing"
	}
	return describeColorSpace(resolve(dict.Get(name), ctx.xref), ctx.xref)
}

func (ctx *traceContext) describePatternOperand(resources *entity.Dict, operands []entity.Object) string {
	if len(operands) == 0 {
		return ""
	}
	name, ok := operands[len(operands)-1].(entity.Name)
	if !ok {
		return ""
	}
	dict := asDict(resolve(resourceCategory(resources, ctx.xref, "Pattern"), ctx.xref), ctx.xref)
	if dict == nil || dict.Get(name) == nil {
		return ""
	}
	return "Pattern " + name.Value() + "=" + shortObject(resolve(dict.Get(name), ctx.xref), ctx.xref)
}

func (ctx *traceContext) describeFont(resources *entity.Dict, name entity.Name) string {
	dict := ctx.lookupResourceDict(resources, "Font", name)
	if dict == nil {
		return "Font missing"
	}
	parts := []string{
		"Font",
		"Subtype=" + nameValue(dict.Get(entity.Name("Subtype"))),
		"BaseFont=" + nameValue(dict.Get(entity.Name("BaseFont"))),
	}
	if descendants, ok := resolve(dict.Get(entity.Name("DescendantFonts")), ctx.xref).(*entity.Array); ok && descendants.Len() > 0 {
		if descendant := asDict(descendants.Get(0), ctx.xref); descendant != nil {
			parts = append(parts,
				"DescSubtype="+nameValue(descendant.Get(entity.Name("Subtype"))),
				"DescBase="+nameValue(descendant.Get(entity.Name("BaseFont"))),
			)
			if fd := asDict(resolve(descendant.Get(entity.Name("FontDescriptor")), ctx.xref), ctx.xref); fd != nil {
				parts = append(parts, describeFontFiles(fd)...)
			}
		}
	}
	if fd := asDict(resolve(dict.Get(entity.Name("FontDescriptor")), ctx.xref), ctx.xref); fd != nil {
		parts = append(parts, describeFontFiles(fd)...)
	}
	return strings.Join(parts, " ")
}

func describeFontFiles(fd *entity.Dict) []string {
	parts := make([]string, 0, 3)
	for _, key := range []string{"FontFile", "FontFile2", "FontFile3"} {
		if obj := fd.Get(entity.Name(key)); obj != nil {
			desc := key
			if stream, ok := obj.(*entity.Stream); ok {
				if subtype := nameValue(stream.Dict().Get(entity.Name("Subtype"))); subtype != "" {
					desc += "/" + subtype
				}
			}
			parts = append(parts, desc)
		}
	}
	return parts
}

func (ctx *traceContext) describeShading(resources *entity.Dict, name entity.Name) string {
	dict := ctx.lookupResourceDict(resources, "Shading", name)
	if dict == nil {
		return "Shading missing"
	}
	return "Shading type=" + shortObject(resolve(dict.Get(entity.Name("ShadingType")), ctx.xref), ctx.xref) +
		" cs=" + describeColorSpace(resolve(dict.Get(entity.Name("ColorSpace")), ctx.xref), ctx.xref)
}

func (ctx *traceContext) lookupResourceDict(resources *entity.Dict, category string, name entity.Name) *entity.Dict {
	return asDict(resolve(ctx.lookupResourceObject(resources, category, name), ctx.xref), ctx.xref)
}

func (ctx *traceContext) lookupXObject(resources *entity.Dict, name entity.Name) *entity.Stream {
	obj := resolve(ctx.lookupResourceObject(resources, "XObject", name), ctx.xref)
	stream, _ := obj.(*entity.Stream)
	return stream
}

func (ctx *traceContext) lookupResourceObject(resources *entity.Dict, category string, name entity.Name) entity.Object {
	dict := asDict(resolve(resourceCategory(resources, ctx.xref, category), ctx.xref), ctx.xref)
	if dict == nil {
		return nil
	}
	return dict.Get(name)
}

func resourceCategory(resources *entity.Dict, xref entity.XRef, category string) entity.Object {
	if resources == nil {
		return nil
	}
	return resolve(resources.Get(entity.Name(category)), xref)
}

func lookupResourceObject(resources *entity.Dict, xref entity.XRef, category string, name entity.Name) entity.Object {
	dict := asDict(resolve(resourceCategory(resources, xref, category), xref), xref)
	if dict == nil {
		return nil
	}
	return dict.Get(name)
}

func formatOperands(operands []entity.Object, limit int) string {
	if len(operands) == 0 {
		return ""
	}
	if limit <= 0 || len(operands) < limit {
		limit = len(operands)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, shortObject(operands[i], nil))
	}
	if len(operands) > limit {
		parts = append(parts, fmt.Sprintf("...(+%d)", len(operands)-limit))
	}
	return strings.Join(parts, " ")
}

func shortObject(obj entity.Object, xref entity.XRef) string {
	obj = resolve(obj, xref)
	switch typed := obj.(type) {
	case nil:
		return "null"
	case entity.Name:
		return "/" + typed.Value()
	case entity.Ref:
		return typed.String()
	case *entity.Integer:
		return typed.String()
	case *entity.Real:
		return typed.String()
	case *entity.Boolean:
		return typed.String()
	case *entity.String:
		value := typed.Value()
		if len(value) > 24 {
			value = value[:24] + "..."
		}
		return "(" + value + ")"
	case *entity.Array:
		if typed.Len() == 0 {
			return "[]"
		}
		limit := typed.Len()
		if limit > 4 {
			limit = 4
		}
		parts := make([]string, 0, limit+1)
		for i := 0; i < limit; i++ {
			parts = append(parts, shortObject(typed.Get(i), xref))
		}
		if typed.Len() > limit {
			parts = append(parts, fmt.Sprintf("...(+%d)", typed.Len()-limit))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case *entity.Dict:
		keys := sortedKeys(typed)
		if len(keys) == 0 {
			return "<<>>"
		}
		limit := len(keys)
		if limit > 6 {
			limit = 6
		}
		parts := make([]string, 0, limit+1)
		for i := 0; i < limit; i++ {
			key := keys[i]
			parts = append(parts, "/"+key.Value()+" "+shortObject(typed.Get(key), xref))
		}
		if len(keys) > limit {
			parts = append(parts, fmt.Sprintf("...(+%d)", len(keys)-limit))
		}
		return "<<" + strings.Join(parts, " ") + ">>"
	case *entity.Stream:
		return "Stream " + shortObject(typed.Dict(), xref)
	default:
		return typed.String()
	}
}

func trackResourceUse(op renderer.Operator, stat *pageStats) {
	switch op.Opcode {
	case "Do":
		if name, ok := firstNameOperand(op.Operands); ok {
			stat.ResourceUses["XObject:"+name]++
		}
	case "gs":
		if name, ok := firstNameOperand(op.Operands); ok {
			stat.ResourceUses["ExtGState:"+name]++
		}
	case "Tf":
		if fontName, ok := firstNameOperand(op.Operands); ok {
			stat.ResourceUses["Font:"+fontName]++
		}
	case "CS", "cs":
		if name, ok := firstNameOperand(op.Operands); ok {
			stat.ResourceUses["ColorSpace:"+name]++
		}
	case "sh":
		if name, ok := firstNameOperand(op.Operands); ok {
			stat.ResourceUses["Shading:"+name]++
		}
	case "SCN", "scn":
		if name, ok := lastNameOperand(op.Operands); ok {
			stat.ResourceUses["PatternOrColorant:"+name]++
		}
	}
}

func opcodePathKey(op renderer.Operator, resources *entity.Dict, xref entity.XRef) string {
	switch op.Opcode {
	case "Do":
		if len(op.Operands) == 0 {
			return "Do:XObject:?"
		}
		name, ok := op.Operands[0].(entity.Name)
		if !ok {
			return "Do:XObject:?"
		}
		stream, _ := resolve(lookupResourceObject(resources, xref, "XObject", name), xref).(*entity.Stream)
		if stream == nil {
			return "Do:XObject:" + name.Value() + ":missing"
		}
		subtype := nameValue(stream.Dict().Get(entity.Name("Subtype")))
		switch subtype {
		case "Image":
			return "Do:XObject:" + name.Value() + ":Image:" + describeFilter(stream.Dict().Get(entity.Name("Filter")))
		case "Form":
			return "Do:XObject:" + name.Value() + ":Form"
		default:
			return "Do:XObject:" + name.Value() + ":" + subtype
		}
	case "gs":
		name, ok := lastNameOperand(op.Operands)
		if !ok {
			return "gs:ExtGState:?"
		}
		return "gs:ExtGState:" + name
	case "Tf":
		name, ok := firstNameOperand(op.Operands)
		if !ok {
			return "Tf:Font:?"
		}
		return "Tf:Font:" + name
	case "CS", "cs":
		name, ok := lastNameOperand(op.Operands)
		if !ok {
			return op.Opcode + ":ColorSpace:?"
		}
		return op.Opcode + ":ColorSpace:" + name
	case "SCN", "scn":
		name, ok := lastNameOperand(op.Operands)
		if !ok {
			return op.Opcode + ":color"
		}
		if resolve(lookupResourceObject(resources, xref, "Pattern", entity.Name(name)), xref) != nil {
			return op.Opcode + ":Pattern:" + name
		}
		return op.Opcode + ":Colorant:" + name
	case "sh":
		name, ok := lastNameOperand(op.Operands)
		if !ok {
			return "sh:Shading:?"
		}
		return "sh:Shading:" + name
	case "Tj", "TJ", "'", "\"":
		return "text:" + op.Opcode
	case "BI":
		return "image:inline"
	case "S", "s":
		return "path:stroke:" + op.Opcode
	case "f", "F", "f*":
		return "path:fill:" + op.Opcode
	case "B", "B*", "b", "b*":
		return "path:fill-stroke:" + op.Opcode
	case "W", "W*":
		return "path:clip:" + op.Opcode
	default:
		return ""
	}
}

func lastNameOperand(operands []entity.Object) (string, bool) {
	if len(operands) == 0 {
		return "", false
	}
	if name, ok := operands[len(operands)-1].(entity.Name); ok {
		return name.Value(), true
	}
	return "", false
}

func firstNameOperand(operands []entity.Object) (string, bool) {
	if len(operands) == 0 {
		return "", false
	}
	if name, ok := operands[0].(entity.Name); ok {
		return name.Value(), true
	}
	return "", false
}

func classifyOperator(op string, stat *pageStats) {
	switch op {
	case "BT", "ET", "Tc", "Tw", "Tz", "TL", "Tf", "Tr", "Ts", "Td", "TD", "Tm", "T*", "Tj", "TJ", "'", "\"":
		stat.TextOps++
	case "m", "l", "c", "v", "y", "h", "re", "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n", "W", "W*":
		stat.PathOps++
	case "CS", "cs", "SC", "SCN", "sc", "scn", "G", "g", "RG", "rg", "K", "k":
		stat.ColorOps++
	case "Do":
		stat.ImageDoOps++
	case "BI":
		stat.InlineImageOps++
	case "sh":
		stat.ShadingOps++
	}
}

func inspectResources(resources *entity.Dict, xref entity.XRef, stat *pageStats, seenForms map[*entity.Stream]bool, seenResources map[*entity.Dict]bool) {
	if resources == nil {
		return
	}
	if seenResources[resources] {
		return
	}
	seenResources[resources] = true

	if fontDict := asDict(resolve(resources.Get(entity.Name("Font")), xref), xref); fontDict != nil {
		stat.ResourceFont += fontDict.Len()
	}
	if patternDict := asDict(resolve(resources.Get(entity.Name("Pattern")), xref), xref); patternDict != nil {
		stat.ResourcePattern += patternDict.Len()
	}
	if shadingDict := asDict(resolve(resources.Get(entity.Name("Shading")), xref), xref); shadingDict != nil {
		stat.ResourceShading += shadingDict.Len()
	}
	if extDict := asDict(resolve(resources.Get(entity.Name("ExtGState")), xref), xref); extDict != nil {
		stat.ResourceExtG += extDict.Len()
		inspectExtGStates(extDict, xref, stat)
	}
	if csDict := asDict(resolve(resources.Get(entity.Name("ColorSpace")), xref), xref); csDict != nil {
		for _, key := range sortedKeys(csDict) {
			desc := describeColorSpace(resolve(csDict.Get(key), xref), xref)
			stat.ColorSpaces = append(stat.ColorSpaces, fmt.Sprintf("%s=%s", key.Value(), desc))
		}
	}
	if xobjectDict := asDict(resolve(resources.Get(entity.Name("XObject")), xref), xref); xobjectDict != nil {
		stat.ResourceXObject += xobjectDict.Len()
		for _, key := range sortedKeys(xobjectDict) {
			obj := resolve(xobjectDict.Get(key), xref)
			stream, ok := obj.(*entity.Stream)
			if !ok {
				continue
			}
			dict := stream.Dict()
			subtype := nameValue(dict.Get(entity.Name("Subtype")))
			switch subtype {
			case "Image":
				stat.ResourceImage++
				filters := describeFilter(dict.Get(entity.Name("Filter")))
				cs := describeColorSpace(resolve(dict.Get(entity.Name("ColorSpace")), xref), xref)
				stat.ImageFilters = append(stat.ImageFilters, fmt.Sprintf("%s:%s", key.Value(), filters))
				stat.ImageColorSpaces = append(stat.ImageColorSpaces, fmt.Sprintf("%s:%s", key.Value(), cs))
				stat.ImageMasks = append(stat.ImageMasks, classifyImageMaskSignals(key, dict, xref)...)
			case "Form":
				stat.ResourceForm++
				if !seenForms[stream] {
					seenForms[stream] = true
					parseContentStream(xref, stream, stat)
					formResources := asDict(resolve(dict.Get(entity.Name("Resources")), xref), xref)
					if formResources != nil {
						inspectResources(formResources, xref, stat, seenForms, seenResources)
					}
				}
			}
		}
	}

	stat.ColorSpaces = sortedUnique(stat.ColorSpaces)
	stat.ImageFilters = sortedUnique(stat.ImageFilters)
	stat.ImageColorSpaces = sortedUnique(stat.ImageColorSpaces)
	stat.ImageMasks = sortedUnique(stat.ImageMasks)
	stat.ExtGStateSignals = sortedUnique(stat.ExtGStateSignals)
}

func inspectExtGStates(extDict *entity.Dict, xref entity.XRef, stat *pageStats) {
	for _, key := range sortedKeys(extDict) {
		dict := asDict(resolve(extDict.Get(key), xref), xref)
		if dict == nil {
			continue
		}
		signals := make([]string, 0, 4)
		if isNonDefaultAlpha(dict.Get(entity.Name("ca")), xref) {
			signals = append(signals, "ca")
		}
		if isNonDefaultAlpha(dict.Get(entity.Name("CA")), xref) {
			signals = append(signals, "CA")
		}
		if isNonDefaultBlendMode(dict.Get(entity.Name("BM")), xref) {
			signals = append(signals, "BM")
		}
		if isActiveSoftMask(dict.Get(entity.Name("SMask")), xref) {
			signals = append(signals, "SMask")
		}
		if boolValue(resolve(dict.Get(entity.Name("AIS")), xref)) {
			signals = append(signals, "AIS")
		}
		if len(signals) > 0 {
			stat.ExtGStateSignals = append(stat.ExtGStateSignals, fmt.Sprintf("%s:%s", key.Value(), strings.Join(signals, "+")))
		}
	}
}

func isNonDefaultAlpha(obj entity.Object, xref entity.XRef) bool {
	if obj == nil {
		return false
	}
	value, ok := numberValue(resolve(obj, xref))
	if !ok {
		return true
	}
	return value != 1
}

func isNonDefaultBlendMode(obj entity.Object, xref entity.XRef) bool {
	if obj == nil {
		return false
	}
	obj = resolve(obj, xref)
	if nameValue(obj) == "Normal" {
		return false
	}
	if arr, ok := obj.(*entity.Array); ok && arr.Len() > 0 && nameValue(resolve(arr.Get(0), xref)) == "Normal" {
		return false
	}
	return true
}

func isActiveSoftMask(obj entity.Object, xref entity.XRef) bool {
	if obj == nil {
		return false
	}
	return nameValue(resolve(obj, xref)) != "None"
}

func classifyImageMaskSignals(key entity.Name, dict *entity.Dict, xref entity.XRef) []string {
	signals := make([]string, 0, 3)
	prefix := key.Value() + ":"
	if mask := dict.Get(entity.Name("SMask")); mask != nil && nameValue(mask) != "None" {
		signals = append(signals, prefix+"SMask")
	}
	if mask := dict.Get(entity.Name("Mask")); mask != nil {
		signals = append(signals, prefix+classifyMaskKind(mask, xref))
	}
	if boolValue(dict.Get(entity.Name("ImageMask"))) || boolValue(dict.Get(entity.Name("IM"))) {
		signals = append(signals, prefix+"ImageMask")
	}
	return signals
}

func classifyMaskKind(mask entity.Object, xref entity.XRef) string {
	switch typed := resolve(mask, xref).(type) {
	case *entity.Array:
		return "ColorKeyMask"
	case *entity.Stream:
		if boolValue(typed.Dict().Get(entity.Name("ImageMask"))) || boolValue(typed.Dict().Get(entity.Name("IM"))) {
			return "Mask"
		}
		return "MaskStream"
	default:
		return "Mask"
	}
}

func inferSubsystem(stat *pageStats) string {
	signals := make([]string, 0, 8)
	has := func(values []string, needles ...string) bool {
		for _, value := range values {
			for _, needle := range needles {
				if strings.Contains(value, needle) {
					return true
				}
			}
		}
		return false
	}

	if has(stat.ColorSpaces, "Separation", "DeviceN") {
		signals = append(signals, "Separation/DeviceN colorspace")
	}
	if has(stat.ColorSpaces, "ICCBased") || has(stat.ImageColorSpaces, "ICCBased") {
		signals = append(signals, "ICCBased colorspace")
	}
	if has(stat.ColorSpaces, "Indexed") || has(stat.ImageColorSpaces, "Indexed") {
		signals = append(signals, "Indexed image/color")
	}
	if has(stat.ImageFilters, "JPXDecode", "JBIG2Decode", "DCTDecode") {
		signals = append(signals, "compressed image decoder")
	}
	if len(stat.ImageMasks) > 0 || has(stat.ExtGStateSignals, "SMask", "ca", "CA") {
		signals = append(signals, "mask/transparency")
	}
	if stat.ResourceShading > 0 || stat.ShadingOps > 0 {
		signals = append(signals, "shading")
	}
	if stat.ResourcePattern > 0 {
		signals = append(signals, "pattern")
	}
	if stat.ResourceForm > 0 {
		signals = append(signals, "form xobject")
	}
	if stat.ResourceImage > 0 || stat.ImageDoOps > 0 {
		signals = append(signals, "image placement")
	}
	if stat.TextOps > stat.PathOps && stat.TextOps > stat.ImageDoOps {
		signals = append(signals, "font/text")
	}
	if stat.PathOps > stat.TextOps && stat.PathOps > stat.ImageDoOps {
		signals = append(signals, "path/vector")
	}
	stat.Signals = signals

	switch {
	case has(stat.ColorSpaces, "Separation", "DeviceN"):
		return "colorspace-separation-devicen"
	case has(stat.ColorSpaces, "ICCBased") || has(stat.ImageColorSpaces, "ICCBased"):
		return "colorspace-icc"
	case len(stat.ImageMasks) > 0 || has(stat.ExtGStateSignals, "SMask"):
		return "mask-transparency"
	case has(stat.ExtGStateSignals, "ca", "CA"):
		return "alpha-extgstate"
	case has(stat.ImageFilters, "JPXDecode"):
		return "image-jpx"
	case has(stat.ImageFilters, "JBIG2Decode"):
		return "image-jbig2"
	case has(stat.ImageFilters, "DCTDecode"):
		return "image-dct"
	case stat.ResourceShading > 0 || stat.ShadingOps > 0:
		return "shading"
	case stat.ResourcePattern > 0:
		return "pattern"
	case stat.ResourceForm > 0:
		return "form-xobject"
	case stat.ResourceImage > 0 || stat.ImageDoOps > 0:
		return "image-placement"
	case stat.TextOps > stat.PathOps:
		return "font-text"
	case stat.PathOps > 0:
		return "path-vector"
	default:
		return "unknown"
	}
}

var popplerSourceRoot string

func defaultPopplerRoot() string {
	for _, path := range []string{
		"/workspace/pdf-reader/tmp/poppler-24.02.0",
		"tmp/poppler-24.02.0",
	} {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func inferPopplerPath(stat *pageStats) string {
	subsystem := stat.Subsystem
	signals := stat.Signals
	var path string
	switch subsystem {
	case "colorspace-separation-devicen":
		path = "poppler/Gfx.cc:1435,1515,1561; poppler/GfxState.cc:2641,2704,2911,3006; poppler/SplashOutputDev.cc:1710"
	case "colorspace-icc":
		path = "poppler/GfxState.cc:1692,1797,1878,1977; poppler/SplashOutputDev.cc:2767"
	case "mask-transparency":
		path = inferMaskPopplerPath(stat)
	case "alpha-extgstate":
		path = "poppler/Gfx.cc:931,1013,1019; poppler/SplashOutputDev.cc alpha/update state"
	case "image-dct", "image-jpx", "image-jbig2", "image-placement":
		path = "poppler/Gfx.cc:4161,4590; poppler/SplashOutputDev.cc:3251; splash/Splash.cc:3489"
	case "shading":
		path = "poppler/Gfx.cc shading op path; poppler/SplashOutputDev.cc shading/fill paths"
	case "pattern":
		path = "poppler/Gfx.cc pattern color ops; poppler/SplashOutputDev.cc tiling pattern paths"
	case "font-text":
		path = "poppler/Gfx.cc text ops; poppler/SplashOutputDev.cc text/glyph paths"
	default:
		for _, signal := range signals {
			if strings.Contains(signal, "mask") {
				path = "poppler/Gfx.cc:4161; poppler/SplashOutputDev.cc:3251"
				return qualifyPopplerPathList(path)
			}
		}
		path = "poppler/Gfx.cc operator dispatch"
	}
	return qualifyPopplerPathList(path)
}

func inferMaskPopplerPath(stat *pageStats) string {
	masks := stat.ActiveImageMasks
	if len(masks) == 0 {
		masks = stat.ImageMasks
	}
	parts := make([]string, 0, 8)
	if hasMaskKind(masks, "SMask") {
		parts = appendUnique(parts,
			"poppler/Gfx.cc:4430,4495,4585,4586",
			"poppler/SplashOutputDev.cc:3674,3743,3750,3845",
			"splash/Splash.cc:3489",
		)
	}
	if hasMaskKind(masks, "Mask") || hasMaskKind(masks, "MaskStream") {
		parts = appendUnique(parts,
			"poppler/Gfx.cc:4511,4571,4587,4588",
			"poppler/SplashOutputDev.cc:3514,3535,3541,3666",
			"splash/Splash.cc:2740,3489",
		)
	}
	if hasMaskKind(masks, "ImageMask") {
		parts = appendUnique(parts,
			"poppler/Gfx.cc:4262,4300",
			"poppler/SplashOutputDev.cc:2630,2660",
			"splash/Splash.cc:2740",
		)
	}
	if hasMaskKind(masks, "ColorKeyMask") {
		parts = appendUnique(parts,
			"poppler/Gfx.cc:4496,4510,4589,4590",
			"poppler/SplashOutputDev.cc:3251,3385",
			"splash/Splash.cc:3489",
		)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "; ")
	}
	for _, signal := range stat.ExtGStateSignals {
		if strings.Contains(signal, "SMask") {
			return "poppler/Gfx.cc ExtGState SMask; poppler/SplashOutputDev.cc setSoftMask"
		}
	}
	return "poppler/Gfx.cc:4161,4590; poppler/SplashOutputDev.cc:3251; splash/Splash.cc:3489"
}

func hasMaskKind(masks []string, kind string) bool {
	suffix := ":" + kind
	for _, mask := range masks {
		if strings.HasSuffix(mask, suffix) {
			return true
		}
	}
	return false
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	for _, value := range values {
		seen[value] = true
	}
	for _, addition := range additions {
		if seen[addition] {
			continue
		}
		values = append(values, addition)
		seen[addition] = true
	}
	return values
}

func qualifyPopplerPathList(list string) string {
	if popplerSourceRoot == "" {
		return list
	}
	parts := strings.Split(list, "; ")
	for i, part := range parts {
		parts[i] = qualifyPopplerPath(part)
	}
	return strings.Join(parts, "; ")
}

func qualifyPopplerPath(part string) string {
	fields := strings.Fields(part)
	if len(fields) == 0 {
		return part
	}
	head := fields[0]
	if strings.HasPrefix(head, "poppler/") || strings.HasPrefix(head, "splash/") {
		fields[0] = popplerSourceRoot + "/" + head
		return strings.Join(fields, " ")
	}
	return part
}

func writeStatsCSV(w io.Writer, stats []pageStats) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write([]string{
		"rank", "exact_percent", "bad_pixels", "subsystem", "pdf", "page", "ops", "text_ops", "path_ops", "color_ops", "do_ops",
		"fonts", "xobjects", "images", "forms", "extgstates", "patterns", "shadings",
		"colorspaces", "image_filters", "image_colorspaces", "image_masks", "active_image_masks", "extgstate_signals", "signals",
		"resource_uses", "opcode_paths", "poppler_path",
		"poppler_png", "ours_png", "diff_png", "parse_error", "resource_error",
	}); err != nil {
		return err
	}

	for i, stat := range stats {
		if err := writer.Write([]string{
			strconv.Itoa(i + 1),
			fmt.Sprintf("%.8f", stat.Row.Exact),
			strconv.FormatInt(stat.Row.badPixels(), 10),
			stat.Subsystem,
			stat.Row.PDF,
			strconv.Itoa(stat.Row.Page),
			strconv.Itoa(stat.OpTotal),
			strconv.Itoa(stat.TextOps),
			strconv.Itoa(stat.PathOps),
			strconv.Itoa(stat.ColorOps),
			strconv.Itoa(stat.ImageDoOps),
			strconv.Itoa(stat.ResourceFont),
			strconv.Itoa(stat.ResourceXObject),
			strconv.Itoa(stat.ResourceImage),
			strconv.Itoa(stat.ResourceForm),
			strconv.Itoa(stat.ResourceExtG),
			strconv.Itoa(stat.ResourcePattern),
			strconv.Itoa(stat.ResourceShading),
			strings.Join(stat.ColorSpaces, ";"),
			strings.Join(stat.ImageFilters, ";"),
			strings.Join(stat.ImageColorSpaces, ";"),
			strings.Join(stat.ImageMasks, ";"),
			strings.Join(stat.ActiveImageMasks, ";"),
			strings.Join(stat.ExtGStateSignals, ";"),
			strings.Join(stat.Signals, ";"),
			dominantResourceUses(stat.ResourceUses, 12),
			dominantOpcodePaths(stat.OpcodePaths, 16),
			stat.PopplerPath,
			stat.Row.PopplerPNG,
			stat.Row.OursPNG,
			stat.Row.DiffPNG,
			stat.Row.ParseErr,
			stat.Row.ResourceErr,
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeMarkdown(w io.Writer, stats []pageStats) {
	clusters := map[string]int{}
	for _, stat := range stats {
		clusters[stat.Subsystem]++
	}
	keys := make([]string, 0, len(clusters))
	for key := range clusters {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if clusters[keys[i]] == clusters[keys[j]] {
			return keys[i] < keys[j]
		}
		return clusters[keys[i]] > clusters[keys[j]]
	})

	fmt.Fprintln(w, "## clusters")
	for _, key := range keys {
		fmt.Fprintf(w, "- %s: %d\n", key, clusters[key])
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## pages")
	fmt.Fprintln(w, "| rank | exact | bad px | subsystem | page | dominant ops | opcode paths | dominant uses | resources | poppler path |")
	fmt.Fprintln(w, "| ---: | ---: | ---: | --- | --- | --- | --- | --- | --- | --- |")
	for i, stat := range stats {
		fmt.Fprintf(
			w,
			"| %d | %.5f | %d | %s | `%s#p%d` | %s | %s | %s | %s | %s |\n",
			i+1,
			stat.Row.Exact,
			stat.Row.badPixels(),
			stat.Subsystem,
			stat.Row.PDF,
			stat.Row.Page,
			markdownCell(dominantOps(stat.Ops, 6)),
			markdownCell(dominantOpcodePaths(stat.OpcodePaths, 6)),
			markdownCell(dominantResourceUses(stat.ResourceUses, 6)),
			markdownCell(resourceSummary(stat)),
			markdownCell(stat.PopplerPath),
		)
	}
}

func dominantOps(ops map[string]int, limit int) string {
	return dominantCounts(ops, limit, "=", " ")
}

func dominantResourceUses(uses map[string]int, limit int) string {
	return dominantCounts(uses, limit, "=", ";")
}

func dominantOpcodePaths(paths map[string]int, limit int) string {
	return dominantCounts(paths, limit, "=", ";")
}

func dominantCounts(counts map[string]int, limit int, sep string, join string) string {
	type pair struct {
		name  string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for name, count := range counts {
		pairs = append(pairs, pair{name: name, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].count > pairs[j].count
	})
	if len(pairs) > limit {
		pairs = pairs[:limit]
	}
	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts = append(parts, fmt.Sprintf("%s%s%d", pair.name, sep, pair.count))
	}
	return strings.Join(parts, join)
}

func resourceSummary(stat pageStats) string {
	parts := []string{
		fmt.Sprintf("font=%d", stat.ResourceFont),
		fmt.Sprintf("xobj=%d", stat.ResourceXObject),
		fmt.Sprintf("img=%d", stat.ResourceImage),
		fmt.Sprintf("form=%d", stat.ResourceForm),
		fmt.Sprintf("extg=%d", stat.ResourceExtG),
		fmt.Sprintf("pat=%d", stat.ResourcePattern),
		fmt.Sprintf("sh=%d", stat.ResourceShading),
	}
	if len(stat.ColorSpaces) > 0 {
		parts = append(parts, "cs="+strings.Join(stat.ColorSpaces, ";"))
	}
	if len(stat.ImageFilters) > 0 {
		parts = append(parts, "filters="+strings.Join(stat.ImageFilters, ";"))
	}
	if len(stat.ImageMasks) > 0 {
		parts = append(parts, "masks="+strings.Join(stat.ImageMasks, ";"))
	}
	if len(stat.ActiveImageMasks) > 0 {
		parts = append(parts, "active-masks="+strings.Join(stat.ActiveImageMasks, ";"))
	}
	if len(stat.ExtGStateSignals) > 0 {
		parts = append(parts, "gs="+strings.Join(stat.ExtGStateSignals, ";"))
	}
	return strings.Join(parts, " ")
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	if value == "" {
		return "-"
	}
	return value
}

func resolve(obj entity.Object, xref entity.XRef) entity.Object {
	if ref, ok := obj.(entity.Ref); ok && xref != nil {
		if resolved, err := xref.Fetch(ref); err == nil {
			return resolved
		}
	}
	return obj
}

func asDict(obj entity.Object, xref entity.XRef) *entity.Dict {
	obj = resolve(obj, xref)
	if dict, ok := obj.(*entity.Dict); ok {
		return dict
	}
	if stream, ok := obj.(*entity.Stream); ok {
		return stream.Dict()
	}
	return nil
}

func sortedKeys(dict *entity.Dict) []entity.Name {
	keys := dict.Keys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].Value() < keys[j].Value() })
	return keys
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	out := values[:0]
	var last string
	for i, value := range values {
		if i == 0 || value != last {
			out = append(out, value)
			last = value
		}
	}
	return out
}

func nameValue(obj entity.Object) string {
	if name, ok := obj.(entity.Name); ok {
		return name.Value()
	}
	return ""
}

func boolValue(obj entity.Object) bool {
	if value, ok := obj.(*entity.Boolean); ok {
		return value.Value()
	}
	return false
}

func numberValue(obj entity.Object) (float64, bool) {
	switch value := obj.(type) {
	case *entity.Integer:
		return float64(value.Value()), true
	case *entity.Real:
		return value.Value(), true
	default:
		return 0, false
	}
}

func describeFilter(obj entity.Object) string {
	switch v := obj.(type) {
	case nil:
		return "none"
	case entity.Name:
		return v.Value()
	case *entity.Array:
		parts := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			parts = append(parts, describeFilter(v.Get(i)))
		}
		return strings.Join(parts, "+")
	default:
		return v.String()
	}
}

func describeColorSpace(obj entity.Object, xref entity.XRef) string {
	obj = resolve(obj, xref)
	switch v := obj.(type) {
	case nil:
		return "none"
	case entity.Name:
		return v.Value()
	case *entity.Array:
		if v.Len() == 0 {
			return "[]"
		}
		head := resolve(v.Get(0), xref)
		headName := nameValue(head)
		if headName == "" {
			headName = head.String()
		}
		if headName == "ICCBased" && v.Len() > 1 {
			if stream, ok := resolve(v.Get(1), xref).(*entity.Stream); ok {
				if n := stream.Dict().Get(entity.Name("N")); n != nil {
					return fmt.Sprintf("ICCBased/N=%s", n.String())
				}
			}
		}
		if (headName == "Separation" || headName == "DeviceN") && v.Len() > 1 {
			return fmt.Sprintf("%s/%s", headName, shortObject(v.Get(1), xref))
		}
		if headName == "Indexed" && v.Len() > 1 {
			return "Indexed/" + describeColorSpace(v.Get(1), xref)
		}
		return headName
	case *entity.Stream:
		return "stream"
	default:
		return v.String()
	}
}
