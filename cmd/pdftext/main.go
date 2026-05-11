// Package main provides the pdftext CLI tool.
// pdftext extracts text content from PDF documents.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
	infrastructuretext "github.com/dh-kam/pdf-go/internal/infrastructure/text"
	appversion "github.com/dh-kam/pdf-go/internal/version"
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
		fmt.Printf("pdftext version %s\n", appversion.Current)
		os.Exit(0)
	}

	// Parse options
	var options Options
	pdfFiles := []string{}
	pages := []int{}

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-j", "--json":
			options.JSON = true
		case "-o", "--output":
			if i+1 < len(args) {
				options.Output = args[i+1]
				i++ // Skip next arg
			}
		case "-p", "--pages":
			if i+1 < len(args) {
				pageSpec := args[i+1]
				parsedPages, err := parsePageSpec(pageSpec)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error parsing page specification: %v\n", err)
					os.Exit(1)
				}
				pages = append(pages, parsedPages...)
				i++ // Skip next arg
			}
		case "-l", "--layout":
			options.PreserveLayout = true
		case "--positions":
			options.WithPositions = true
		case "-password", "--password":
			if i+1 < len(args) {
				options.Password = args[i+1]
				i++ // Skip next arg
			}
		default:
			if len(arg) > 0 && arg[0] != '-' {
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
		if err := processPDF(pdfFile, pages, options); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", pdfFile, err)
			exitCode = 1
		}

		// Print separator between files
		if len(pdfFiles) > 1 && !options.JSON {
			fmt.Println("---")
		}
	}

	os.Exit(exitCode)
}

// Options represents the command line options
type Options struct {
	Output         string
	Password       string
	JSON           bool
	PreserveLayout bool
	WithPositions  bool
}

// TextOutput represents the text extraction output
type TextOutput struct {
	FilePath string     `json:"file_path"`
	FullText string     `json:"full_text,omitempty"`
	Pages    []PageText `json:"pages"`
}

// PageText represents text from a single page
type PageText struct {
	Text    string     `json:"text"`
	Items   []TextItem `json:"items,omitempty"`
	PageNum int        `json:"page_num"`
}

// TextItem represents a piece of text with position information
type TextItem struct {
	Text     string  `json:"text"`
	Font     string  `json:"font,omitempty"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	FontSize float64 `json:"font_size,omitempty"`
}

// processPDF processes a single PDF file and extracts text
func processPDF(filePath string, pages []int, options Options) error {
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

	// Get page count
	pageCount, err := doc.PageCount()
	if err != nil {
		return fmt.Errorf("get page count: %w", err)
	}

	// Determine which pages to extract
	var pagesToExtract []int
	if len(pages) > 0 {
		// Validate page numbers
		for _, p := range pages {
			if p < 1 || p > pageCount {
				return fmt.Errorf("page %d out of range (1-%d)", p, pageCount)
			}
			pagesToExtract = append(pagesToExtract, p-1) // Convert to 0-based
		}
	} else {
		// Extract all pages
		pagesToExtract = make([]int, pageCount)
		for i := 0; i < pageCount; i++ {
			pagesToExtract[i] = i
		}
	}

	// Create text extractor
	extractor := infrastructuretext.NewExtractor()
	extractor.SetPreserveSpacing(options.PreserveLayout)

	// Extract text from pages
	output := TextOutput{
		FilePath: filePath,
		Pages:    make([]PageText, 0, len(pagesToExtract)),
	}

	var fullText strings.Builder

	for _, pageIndex := range pagesToExtract {
		page, err := doc.GetPage(pageIndex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not get page %d: %v\n", pageIndex+1, err)
			continue
		}

		var pageText PageText
		pageText.PageNum = pageIndex + 1

		if options.WithPositions {
			// Extract text with positions
			layer, err := extractor.Extract(page)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Could not extract text from page %d: %v\n", pageIndex+1, err)
				pageText.Text = ""
			} else {
				pageText.Text = layer.Text()
				pageText.Items = make([]TextItem, len(layer.Items))
				for i, item := range layer.Items {
					pageText.Items[i] = TextItem{
						Text:     item.Text,
						X:        float64(item.BoundingBox.Min.X),
						Y:        float64(item.BoundingBox.Min.Y),
						FontSize: item.FontSize,
						Font:     item.Font,
					}
				}
			}
		} else {
			// Extract plain text
			text, err := extractor.ExtractToText(page)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Could not extract text from page %d: %v\n", pageIndex+1, err)
				pageText.Text = ""
			} else {
				pageText.Text = text
			}
		}

		output.Pages = append(output.Pages, pageText)

		// Add to full text
		if fullText.Len() > 0 {
			fullText.WriteString("\n\n")
		}
		fullText.WriteString(fmt.Sprintf("--- Page %d ---\n", pageIndex+1))
		fullText.WriteString(pageText.Text)
	}

	output.FullText = fullText.String()

	// Output the extracted text
	if options.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(output); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else {
		// Output to file or stdout
		var (
			writer     io.Writer = os.Stdout
			outputFile *os.File
		)
		if options.Output != "" {
			outputFile, err = os.Create(options.Output)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			writer = outputFile
		}

		if options.WithPositions {
			// Print with page and position information
			for _, page := range output.Pages {
				if _, writeErr := fmt.Fprintf(writer, "=== Page %d ===\n", page.PageNum); writeErr != nil {
					return fmt.Errorf("write page header: %w", writeErr)
				}
				for _, item := range page.Items {
					if _, writeErr := fmt.Fprintf(writer, "[%.1f,%.1f] %s\n", item.X, item.Y, item.Text); writeErr != nil {
						return fmt.Errorf("write positioned text: %w", writeErr)
					}
				}
				if _, writeErr := fmt.Fprintln(writer); writeErr != nil {
					return fmt.Errorf("write page separator: %w", writeErr)
				}
			}
		} else {
			// Print plain text
			if _, writeErr := fmt.Fprint(writer, output.FullText); writeErr != nil {
				return fmt.Errorf("write text output: %w", writeErr)
			}
		}

		if outputFile != nil {
			if closeErr := outputFile.Close(); closeErr != nil {
				return fmt.Errorf("close output file: %w", closeErr)
			}
		}
	}

	return nil
}

// parsePageSpec parses a page specification string
// Supports: single pages (1), ranges (1-5), and comma-separated lists (1,3,5)
func parsePageSpec(spec string) ([]int, error) {
	var pages []int

	parts := strings.Split(spec, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			// Parse range
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid page range: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start page in range %s: %w", part, err)
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end page in range %s: %w", part, err)
			}

			if start < 1 || end < 1 {
				return nil, fmt.Errorf("page numbers must be >= 1 in range %s", part)
			}

			if start > end {
				return nil, fmt.Errorf("start page must be <= end page in range %s", part)
			}

			for i := start; i <= end; i++ {
				pages = append(pages, i)
			}
		} else {
			// Parse single page
			page, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid page number: %s", part)
			}
			if page < 1 {
				return nil, fmt.Errorf("page numbers must be >= 1")
			}
			pages = append(pages, page)
		}
	}

	return pages, nil
}

// printUsage prints usage information
func printUsage() {
	fmt.Println(`Usage: pdftext [options] <pdf-file> [pdf-file ...]

Extract text content from PDF documents.

Options:
  -h, --help          Show this help message and exit
  -v, --version       Show version information and exit
  -j, --json          Output text in JSON format
  -o, --output FILE   Write output to file instead of stdout
  -p, --pages SPEC    Extract specific pages (e.g., "1", "1-5", "1,3,5")
  -l, --layout        Preserve layout (try to maintain original formatting)
  --positions         Include text position information
  -password <pwd>     Password for encrypted PDFs

Page Specification:
  - Single page: 1
  - Range: 1-5
  - Comma-separated: 1,3,5
  - Combined: 1-3,5,7-9

Examples:
  pdftext document.pdf
  pdftext -o output.txt document.pdf
  pdftext -p 1-3 document.pdf
  pdftext -j document.pdf
  pdftext --positions document.pdf
  pdftext -p 1,3,5-7 document.pdf`)
}
