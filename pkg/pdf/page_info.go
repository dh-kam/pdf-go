package pdf

import "math"

// GetPageCount is a Java-parity alias of PageCount.
func (d *Document) GetPageCount() (int, error) {
	return d.PageCount()
}

// GetPageWidth returns a page width in points for a 0-based page index.
func (d *Document) GetPageWidth(pageIndex int) (float64, error) {
	page, err := d.Page(pageIndex)
	if err != nil {
		return 0, err
	}
	return page.Width(), nil
}

// GetPageHeight returns a page height in points for a 0-based page index.
func (d *Document) GetPageHeight(pageIndex int) (float64, error) {
	page, err := d.Page(pageIndex)
	if err != nil {
		return 0, err
	}
	return page.Height(), nil
}

// GetPageRotate returns normalized page rotation (0, 90, 180, 270) for a 0-based page index.
func (d *Document) GetPageRotate(pageIndex int) (int, error) {
	page, err := d.Page(pageIndex)
	if err != nil {
		return 0, err
	}
	return page.Rotate(), nil
}

// GetPageMediaBoxSL returns a page media box rectangle for a 0-based page index.
func (d *Document) GetPageMediaBoxSL(pageIndex int) ([4]float64, error) {
	page, err := d.Page(pageIndex)
	if err != nil {
		return [4]float64{}, err
	}
	return page.MediaBox(), nil
}

// GetPageCropBoxSL returns a page crop box rectangle for a 0-based page index.
func (d *Document) GetPageCropBoxSL(pageIndex int) ([4]float64, error) {
	page, err := d.Page(pageIndex)
	if err != nil {
		return [4]float64{}, err
	}
	return page.CropBox(), nil
}

// GetPageWidth100 returns rounded page width at 100% scale.
func (d *Document) GetPageWidth100(pageIndex int) (int, error) {
	width, err := d.GetPageWidth(pageIndex)
	if err != nil {
		return 0, err
	}
	return int(math.Round(width)), nil
}

// GetPageHeight100 returns rounded page height at 100% scale.
func (d *Document) GetPageHeight100(pageIndex int) (int, error) {
	height, err := d.GetPageHeight(pageIndex)
	if err != nil {
		return 0, err
	}
	return int(math.Round(height)), nil
}

// GetSinglePageWidth returns rounded page width at the given zoom scale.
func (d *Document) GetSinglePageWidth(pageIndex int, zoom float64) (int, error) {
	if zoom <= 0 {
		return 0, ErrInvalidZoom
	}

	width, err := d.GetPageWidth(pageIndex)
	if err != nil {
		return 0, err
	}
	return int(math.Round(width * zoom)), nil
}

// GetSinglePageHeight returns rounded page height at the given zoom scale.
func (d *Document) GetSinglePageHeight(pageIndex int, zoom float64) (int, error) {
	if zoom <= 0 {
		return 0, ErrInvalidZoom
	}

	height, err := d.GetPageHeight(pageIndex)
	if err != nil {
		return 0, err
	}
	return int(math.Round(height * zoom)), nil
}

// GetSinglePageWidth100 returns rounded page width at 100% scale.
func (d *Document) GetSinglePageWidth100(pageIndex int) (int, error) {
	return d.GetPageWidth100(pageIndex)
}

// ErrInvalidZoom indicates an invalid zoom value.
var ErrInvalidZoom = &invalidZoomError{}

type invalidZoomError struct{}

// Error returns the error message.
func (e *invalidZoomError) Error() string {
	return "zoom must be positive"
}
