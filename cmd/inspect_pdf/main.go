package main

import (
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: inspect_pdf <pdf_file>")
		return
	}

	pdfPath := os.Args[1]

	doc, err := pdf.Open(pdfPath)
	if err != nil {
		fmt.Printf("Error opening PDF: %v\n", err)
		return
	}
	defer doc.Close()

	page, err := doc.Page(0)
	if err != nil {
		fmt.Printf("Error getting page: %v\n", err)
		return
	}

	resources, _ := page.Resources()
	if resources == nil {
		fmt.Printf("No resources\n")
		return
	}

	// Check for ColorSpace
	colorSpace := resources.Get("ColorSpace")
	if colorSpace != nil {
		fmt.Printf("ColorSpace type: %T\n", colorSpace)
		fmt.Printf("ColorSpace: %+v\n", colorSpace)
	}

	// Check for ICC profiles
	fmt.Printf("\nSearching for ICC profiles...\n")
	fmt.Printf("ColorSpace details: %+v\n", colorSpace)

	fmt.Printf("\nResources:\n")
	fmt.Printf("Resources details: %+v\n", resources)
}
