// Package main provides the pdfinfo CLI tool.
// pdfinfo displays PDF document information including metadata, page count, dimensions, and more.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
	appversion "github.com/dh-kam/pdf-go/internal/version"
	publicpdf "github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	// Parse command line flags
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Check for help flag
	if os.Args[1] == "-h" || os.Args[1] == "--help" {
		printUsage()
		os.Exit(0)
	}

	// Check for version flag
	if os.Args[1] == "-v" || os.Args[1] == "--version" {
		fmt.Printf("pdfinfo version %s\n", appversion.Current)
		os.Exit(0)
	}

	// Parse options
	var options Options
	var pdfFiles []string

	for _, arg := range os.Args[1:] {
		switch arg {
		case "-j", "--json":
			options.JSON = true
		case "-v", "--verbose":
			options.Verbose = true
		case "-p", "--page-details":
			options.PageDetails = true
		case "-m", "--metadata":
			options.Metadata = true
		case "-o", "--outlines":
			options.Outlines = true
		case "-f", "--form-fields":
			options.FormFields = true
		case "-password", "--password":
			// Next arg is password
			idx := indexOf(os.Args[1:], arg)
			if idx+1 < len(os.Args[1:]) {
				options.Password = os.Args[1:][idx+1]
			}
		default:
			if arg[0] != '-' {
				pdfFiles = append(pdfFiles, arg)
			}
		}
	}

	if len(pdfFiles) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No PDF files specified")
		printUsage()
		os.Exit(1)
	}

	// Process each PDF file
	exitCode := 0
	for _, pdfFile := range pdfFiles {
		if err := processPDF(pdfFile, options); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", pdfFile, err)
			exitCode = 1
		}

		// Print separator between files if not JSON
		if !options.JSON && len(pdfFiles) > 1 {
			fmt.Println()
		}
	}

	os.Exit(exitCode)
}

// Options represents the command line options
type Options struct {
	Password    string
	JSON        bool
	Verbose     bool
	PageDetails bool
	Metadata    bool
	Outlines    bool
	FormFields  bool
}

// PDFInfo represents information about a PDF document
type PDFInfo struct {
	Metadata    map[string]string `json:"metadata,omitempty"`
	Info        DocumentInfo      `json:"info,omitempty"`
	FilePath    string            `json:"file_path"`
	PDFVersion  string            `json:"pdf_version"`
	PageDetails []PageDetailInfo  `json:"page_details,omitempty"`
	Outlines    []OutlineInfo     `json:"outlines,omitempty"`
	FormFields  []FormFieldInfo   `json:"form_fields,omitempty"`
	FileSize    int64             `json:"file_size"`
	PageCount   int               `json:"page_count"`
	Encrypted   bool              `json:"encrypted"`
}

// DocumentInfo represents the document information dictionary
type DocumentInfo struct {
	Title    string `json:"title,omitempty"`
	Author   string `json:"author,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Keywords string `json:"keywords,omitempty"`
	Creator  string `json:"creator,omitempty"`
	Producer string `json:"producer,omitempty"`
}

// PageDetailInfo represents detailed information about a page
type PageDetailInfo struct {
	PageNum  int        `json:"page_num"`
	MediaBox [4]float64 `json:"media_box"`
	CropBox  [4]float64 `json:"crop_box"`
	Rotate   int        `json:"rotate"`
	Width    float64    `json:"width"`
	Height   float64    `json:"height"`
}

// OutlineInfo represents one flattened outline entry.
type OutlineInfo struct {
	ActionHide        *bool    `json:"action_hide,omitempty"`
	ActionNewWindow   *bool    `json:"action_new_window,omitempty"`
	ActionURI         string   `json:"action_uri,omitempty"`
	Title             string   `json:"title"`
	ActionFile        string   `json:"action_file,omitempty"`
	ActionCommand     string   `json:"action_command,omitempty"`
	ActionDirectory   string   `json:"action_directory,omitempty"`
	ActionOperation   string   `json:"action_operation,omitempty"`
	ActionType        string   `json:"action_type,omitempty"`
	ActionRendition   string   `json:"action_rendition,omitempty"`
	ActionHideTargets []string `json:"action_hide_targets,omitempty"`
	ActionFields      []string `json:"action_fields,omitempty"`
	ActionFlags       int      `json:"action_flags,omitempty"`
	PageIndex         int      `json:"page_index,omitempty"`
	ActionRenditionOp int      `json:"action_rendition_operation,omitempty"`
	Depth             int      `json:"depth"`
	ActionExclude     bool     `json:"action_exclude_fields,omitempty"`
}

// FormFieldInfo represents one flattened form field entry.
type FormFieldInfo struct {
	Name      string   `json:"name,omitempty"`
	Type      string   `json:"type,omitempty"`
	Value     string   `json:"value,omitempty"`
	Options   []string `json:"options,omitempty"`
	PageIndex int      `json:"page_index,omitempty"`
}

// processPDF processes a single PDF file and prints its information
func processPDF(filePath string, options Options) error {
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("get file info: %w", err)
	}

	// Read PDF file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Parse PDF
	xrefTable := xref.NewTable(data)
	if err := xrefTable.Parse(); err != nil {
		return fmt.Errorf("parse PDF: %w", err)
	}

	// Handle encryption
	if xrefTable.IsEncrypted() {
		if err := xrefTable.ParseEncryptionDict(options.Password); err != nil {
			return fmt.Errorf("parse encryption (try using -password flag): %w", err)
		}
		if !xrefTable.IsAuthenticated() {
			return fmt.Errorf("invalid password or unsupported encryption")
		}
	}

	// Create document
	doc := entity.NewDocument(xrefTable)

	// Get catalog
	catalog, err := xrefTable.GetCatalog()
	if err == nil {
		doc.SetCatalog(catalog)
	}

	// Resolve trailer and /Info dictionary.
	trailer, trailerErr := xrefTable.GetTrailer()
	if trailerErr != nil {
		trailer = nil
	}
	if trailer != nil {
		if infoObj := trailer.Get(entity.Name("/Info")); infoObj != nil {
			if infoDict, ok := infoObj.(*entity.Dict); ok {
				doc.SetInfo(infoDict)
			}
		}
	}

	// Set file size
	doc.SetFileSize(int64(len(data)))

	// Extract PDF version
	pdfVersion := detectPDFVersion(data)

	// Get page count
	pageCount, err := doc.PageCount()
	if err != nil {
		return fmt.Errorf("get page count: %w", err)
	}

	// Build PDFInfo structure
	pdfInfo := PDFInfo{
		FilePath:   filePath,
		FileSize:   fileInfo.Size(),
		PDFVersion: pdfVersion,
		PageCount:  pageCount,
		Encrypted:  isEncrypted(trailer),
	}

	// Extract document info
	if infoDict := doc.Info(); infoDict != nil {
		pdfInfo.Info = extractDocumentInfo(infoDict)
	}

	// Extract page details if requested
	if options.PageDetails {
		pdfInfo.PageDetails = extractPageDetails(doc, pageCount)
	}

	// Extract metadata if requested
	if options.Metadata {
		pdfInfo.Metadata = extractMetadata(doc)
	}

	// Extract outlines if requested.
	if options.Outlines {
		outlines, err := extractOutlines(filePath, options.Password)
		if err != nil {
			return fmt.Errorf("extract outlines: %w", err)
		}
		pdfInfo.Outlines = outlines
	}

	// Extract form fields if requested.
	if options.FormFields {
		formFields, err := extractFormFields(filePath, options.Password)
		if err != nil {
			return fmt.Errorf("extract form fields: %w", err)
		}
		pdfInfo.FormFields = formFields
	}

	// Output the information
	if options.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(pdfInfo); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else {
		printPDFInfo(pdfInfo, options)
	}

	return nil
}

// printPDFInfo prints PDF information in human-readable format
func printPDFInfo(info PDFInfo, options Options) {
	fmt.Printf("File: %s\n", info.FilePath)
	fmt.Printf("File Size: %d bytes\n", info.FileSize)
	fmt.Printf("PDF Version: %s\n", info.PDFVersion)
	fmt.Printf("Page Count: %d\n", info.PageCount)

	if info.Encrypted {
		fmt.Println("Encrypted: Yes")
	}

	if info.Info.Title != "" || info.Info.Author != "" ||
		info.Info.Subject != "" || info.Info.Keywords != "" ||
		info.Info.Creator != "" || info.Info.Producer != "" {
		fmt.Println("\nDocument Info:")
		if info.Info.Title != "" {
			fmt.Printf("  Title: %s\n", info.Info.Title)
		}
		if info.Info.Author != "" {
			fmt.Printf("  Author: %s\n", info.Info.Author)
		}
		if info.Info.Subject != "" {
			fmt.Printf("  Subject: %s\n", info.Info.Subject)
		}
		if info.Info.Keywords != "" {
			fmt.Printf("  Keywords: %s\n", info.Info.Keywords)
		}
		if info.Info.Creator != "" {
			fmt.Printf("  Creator: %s\n", info.Info.Creator)
		}
		if info.Info.Producer != "" {
			fmt.Printf("  Producer: %s\n", info.Info.Producer)
		}
	}

	if options.PageDetails && len(info.PageDetails) > 0 {
		fmt.Println("\nPage Details:")
		for _, page := range info.PageDetails {
			fmt.Printf("  Page %d:\n", page.PageNum)
			fmt.Printf("    Media Box: [%.2f, %.2f, %.2f, %.2f]\n",
				page.MediaBox[0], page.MediaBox[1], page.MediaBox[2], page.MediaBox[3])
			fmt.Printf("    Crop Box: [%.2f, %.2f, %.2f, %.2f]\n",
				page.CropBox[0], page.CropBox[1], page.CropBox[2], page.CropBox[3])
			fmt.Printf("    Size: %.2f x %.2f points\n", page.Width, page.Height)
			if page.Rotate != 0 {
				fmt.Printf("    Rotation: %d degrees\n", page.Rotate)
			}
		}
	}

	if options.Metadata && len(info.Metadata) > 0 {
		fmt.Println("\nMetadata:")
		for key, value := range info.Metadata {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}

	if options.Outlines && len(info.Outlines) > 0 {
		fmt.Println("\nOutlines:")
		for _, outline := range info.Outlines {
			indent := ""
			for i := 0; i < outline.Depth; i++ {
				indent += "  "
			}
			line := fmt.Sprintf("  %s- %s", indent, outline.Title)
			if outline.PageIndex >= 0 {
				line += fmt.Sprintf(" (page %d)", outline.PageIndex+1)
			}
			if outline.ActionType != "" {
				line += fmt.Sprintf(" [%s]", outline.ActionType)
			}
			if outline.ActionFile != "" {
				line += fmt.Sprintf(" file=%q", outline.ActionFile)
			}
			if outline.ActionURI != "" {
				line += fmt.Sprintf(" uri=%q", outline.ActionURI)
			}
			if outline.ActionCommand != "" {
				line += fmt.Sprintf(" cmd=%q", outline.ActionCommand)
			}
			if outline.ActionDirectory != "" {
				line += fmt.Sprintf(" dir=%q", outline.ActionDirectory)
			}
			if outline.ActionOperation != "" {
				line += fmt.Sprintf(" op=%q", outline.ActionOperation)
			}
			if len(outline.ActionFields) > 0 {
				line += fmt.Sprintf(" fields=%v", outline.ActionFields)
				if outline.ActionExclude {
					line += " (exclude)"
				}
			}
			if outline.ActionHide != nil {
				line += fmt.Sprintf(" hide=%t", *outline.ActionHide)
			}
			if len(outline.ActionHideTargets) > 0 {
				line += fmt.Sprintf(" targets=%v", outline.ActionHideTargets)
			}
			if outline.ActionRendition != "" {
				line += fmt.Sprintf(" rendition=%q", outline.ActionRendition)
			}
			if outline.ActionRenditionOp != 0 {
				line += fmt.Sprintf(" rendition_op=%d", outline.ActionRenditionOp)
			}
			if outline.ActionNewWindow != nil {
				line += fmt.Sprintf(" new_window=%t", *outline.ActionNewWindow)
			}
			fmt.Println(line)
		}
	}

	if options.FormFields && len(info.FormFields) > 0 {
		fmt.Println("\nForm Fields:")
		for _, field := range info.FormFields {
			line := fmt.Sprintf("  - %s", field.Name)
			if field.Name == "" {
				line = "  - (unnamed)"
			}
			if field.Type != "" {
				line += fmt.Sprintf(" [%s]", field.Type)
			}
			if field.PageIndex >= 0 {
				line += fmt.Sprintf(" (page %d)", field.PageIndex+1)
			}
			fmt.Println(line)

			if field.Value != "" {
				fmt.Printf("      Value: %s\n", field.Value)
			}
			if len(field.Options) > 0 {
				fmt.Printf("      Options: %v\n", field.Options)
			}
		}
	}
}

// extractDocumentInfo extracts document information from the info dictionary
func extractDocumentInfo(info *entity.Dict) DocumentInfo {
	docInfo := DocumentInfo{}

	if val := info.Get(entity.Name("/Title")); val != nil {
		if str, ok := val.(*entity.String); ok {
			docInfo.Title = str.Value()
		}
	}

	if val := info.Get(entity.Name("/Author")); val != nil {
		if str, ok := val.(*entity.String); ok {
			docInfo.Author = str.Value()
		}
	}

	if val := info.Get(entity.Name("/Subject")); val != nil {
		if str, ok := val.(*entity.String); ok {
			docInfo.Subject = str.Value()
		}
	}

	if val := info.Get(entity.Name("/Keywords")); val != nil {
		if str, ok := val.(*entity.String); ok {
			docInfo.Keywords = str.Value()
		}
	}

	if val := info.Get(entity.Name("/Creator")); val != nil {
		if str, ok := val.(*entity.String); ok {
			docInfo.Creator = str.Value()
		}
	}

	if val := info.Get(entity.Name("/Producer")); val != nil {
		if str, ok := val.(*entity.String); ok {
			docInfo.Producer = str.Value()
		}
	}

	return docInfo
}

// extractPageDetails extracts detailed information about all pages
func extractPageDetails(doc *entity.Document, pageCount int) []PageDetailInfo {
	details := make([]PageDetailInfo, 0, pageCount)

	for i := 0; i < pageCount; i++ {
		page, err := doc.GetPage(i)
		if err != nil {
			continue
		}

		detail := PageDetailInfo{
			PageNum:  i + 1,
			MediaBox: page.MediaBox(),
			CropBox:  page.CropBox(),
			Rotate:   page.Rotate(),
			Width:    page.Width(),
			Height:   page.Height(),
		}

		details = append(details, detail)
	}

	return details
}

// extractMetadata extracts XMP metadata from the document
func extractMetadata(doc *entity.Document) map[string]string {
	metadata := make(map[string]string)

	// Get parsed metadata
	parsedMeta := doc.ParsedMetadata()
	if parsedMeta == nil {
		return metadata
	}

	// Add title
	if titles := parsedMeta.Title(); len(titles) > 0 {
		metadata["Title"] = titles[0] // Use first title
	}

	// Add creators
	if creators := parsedMeta.Creator(); len(creators) > 0 {
		metadata["Creator"] = joinStrings(creators, ", ")
	}

	// Add subjects
	if subjects := parsedMeta.Subject(); len(subjects) > 0 {
		metadata["Subject"] = joinStrings(subjects, ", ")
	}

	// Add description
	if desc := parsedMeta.Description(); desc != "" {
		metadata["Description"] = desc
	}

	// Add producer
	if producer := parsedMeta.Producer(); producer != "" {
		metadata["Producer"] = producer
	}

	// Add creator tool
	if tool := parsedMeta.CreatorTool(); tool != "" {
		metadata["CreatorTool"] = tool
	}

	// Add keywords
	if keywords := parsedMeta.Keywords(); len(keywords) > 0 {
		metadata["Keywords"] = joinStrings(keywords, ", ")
	}

	// Add dates if not zero
	if !parsedMeta.CreateDate().IsZero() {
		metadata["CreateDate"] = parsedMeta.CreateDate().Format("2006-01-02 15:04:05")
	}
	if !parsedMeta.ModifyDate().IsZero() {
		metadata["ModifyDate"] = parsedMeta.ModifyDate().Format("2006-01-02 15:04:05")
	}

	return metadata
}

// extractOutlines extracts and flattens outline entries from a PDF file.
func extractOutlines(filePath, password string) ([]OutlineInfo, error) {
	doc, err := publicpdf.OpenWithPassword(filePath, password)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := doc.Close(); closeErr != nil {
			return
		}
	}()

	items, err := doc.Outlines()
	if err != nil {
		return nil, err
	}

	out := make([]OutlineInfo, 0)
	var walk func(nodes []*publicpdf.Outline, depth int)
	walk = func(nodes []*publicpdf.Outline, depth int) {
		for _, node := range nodes {
			entry := OutlineInfo{
				Depth:     depth,
				Title:     node.Title,
				PageIndex: node.PageIndex,
			}
			if node.Action != nil {
				entry.ActionType = node.Action.Type
				entry.ActionURI = node.Action.URI
				entry.ActionFile = node.Action.File
				entry.ActionCommand = node.Action.Command
				entry.ActionDirectory = node.Action.Directory
				entry.ActionOperation = node.Action.Operation
				entry.ActionFlags = node.Action.Flags
				entry.ActionFields = append([]string(nil), node.Action.FieldNames...)
				entry.ActionExclude = node.Action.ExcludeFields
				if node.Action.Type == "Hide" || node.Action.HasHide {
					hide := node.Action.Hide
					entry.ActionHide = &hide
				}
				entry.ActionHideTargets = append([]string(nil), node.Action.HideTargets...)
				entry.ActionRendition = node.Action.RenditionName
				entry.ActionRenditionOp = node.Action.RenditionOperation
				if node.Action.HasNewWindow {
					newWindow := node.Action.NewWindow
					entry.ActionNewWindow = &newWindow
				}
			}
			out = append(out, entry)
			if len(node.Children) > 0 {
				walk(node.Children, depth+1)
			}
		}
	}

	walk(items, 0)
	return out, nil
}

// extractFormFields extracts flattened form fields from a PDF file.
func extractFormFields(filePath, password string) ([]FormFieldInfo, error) {
	doc, err := publicpdf.OpenWithPassword(filePath, password)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := doc.Close(); closeErr != nil {
			return
		}
	}()

	fields, err := doc.FormFields()
	if err != nil {
		return nil, err
	}

	out := make([]FormFieldInfo, 0, len(fields))
	for _, field := range fields {
		out = append(out, FormFieldInfo{
			Name:      field.Name,
			Type:      field.Type,
			Value:     stringifyPDFValue(field.Value),
			PageIndex: field.PageIndex,
			Options:   append([]string(nil), field.Options...),
		})
	}

	return out, nil
}

func stringifyPDFValue(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// detectPDFVersion detects the PDF version from the file header
func detectPDFVersion(data []byte) string {
	// PDF header is: %PDF-1.x
	if len(data) < 8 {
		return "Unknown"
	}

	header := string(data[:8])
	if len(header) >= 8 && header[:5] == "%PDF-" {
		return header[5:8]
	}

	return "Unknown"
}

// isEncrypted checks if the PDF is encrypted
func isEncrypted(trailer *entity.Dict) bool {
	if trailer == nil {
		return false
	}

	encryptVal := trailer.Get(entity.Name("/Encrypt"))
	return encryptVal != nil
}

// printUsage prints usage information
func printUsage() {
	fmt.Println(`Usage: pdfinfo [options] <pdf-file> [pdf-file ...]

Display PDF document information.

Options:
  -h, --help          Show this help message and exit
  -v, --version       Show version information and exit
  -j, --json          Output information in JSON format
  -v, --verbose       Enable verbose output
  -p, --page-details  Show detailed information about each page
  -m, --metadata      Show XMP metadata
  -o, --outlines      Show document outlines/bookmarks
  -f, --form-fields   Show flattened AcroForm fields
  -password <pwd>     Password for encrypted PDFs

Examples:
  pdfinfo document.pdf
  pdfinfo -j document.pdf
  pdfinfo -p document.pdf
  pdfinfo -p -m document.pdf
  pdfinfo -o document.pdf
  pdfinfo -f document.pdf
  pdfinfo -password secret document.pdf
  pdfinfo *.pdf`)
}

// indexOf finds the index of a string in a slice.
func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
