package pdf

import (
	"fmt"
	"os"
	"strings"
)

// ExportToFDF exports all page annotations to an FDF file.
func (d *Document) ExportToFDF(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path is empty")
	}

	count, err := d.PageCount()
	if err != nil {
		return err
	}
	if count <= 0 {
		return fmt.Errorf("document has no pages")
	}

	pageIndexes := make([]int, count)
	for i := 0; i < count; i++ {
		pageIndexes[i] = i
	}

	return d.ExportPageAnnotationsToFDF(pageIndexes, path)
}

// ImportFromFDF reads one FDF file and applies form values in session scope.
// It returns the number of fields updated.
func (d *Document) ImportFromFDF(path string) (int, error) {
	if strings.TrimSpace(path) == "" {
		return 0, fmt.Errorf("import path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read fdf file: %w", err)
	}

	return d.ImportFormDataFDF(data)
}
