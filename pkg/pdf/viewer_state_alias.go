package pdf

import (
	"math"
	"strings"
)

// SetBannerView stores one banner view marker in session state.
func (d *Document) SetBannerView(view interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bannerView = view
}

// GetBannerView returns stored banner view marker.
func (d *Document) GetBannerView() interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.bannerView
}

// SetBookReadDirection sets book read direction.
// 1 means left-to-right and 2 means right-to-left.
func (d *Document) SetBookReadDirection(direction int) {
	if direction != 2 {
		direction = 1
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bookDirection = direction
}

// GetBookDirection returns current book read direction.
func (d *Document) GetBookDirection() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.bookDirection
}

// GetBookDirectionSetting returns configured book read direction.
func (d *Document) GetBookDirectionSetting() int {
	return d.GetBookDirection()
}

// IsBookReadDirectionR2L reports whether read direction is right-to-left.
func (d *Document) IsBookReadDirectionR2L() bool {
	return d.GetBookDirection() == 2
}

// GetZoom returns current session zoom value.
func (d *Document) GetZoom() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.zoom
}

// IntZoom returns percent zoom value (x100).
func (d *Document) IntZoom(zoom float64) float64 {
	return zoom * 100.0
}

// GetViewer returns nil because viewer object is not maintained in headless library mode.
func (d *Document) GetViewer() *ViewerSession {
	return nil
}

// NightModeGetMode returns whether night mode is enabled.
func (d *Document) NightModeGetMode() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.nightMode
}

// NightModeSet sets night mode and returns 1 when enabled and 0 when disabled.
func (d *Document) NightModeSet(enabled bool) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nightMode = enabled
	if enabled {
		return 1
	}
	return 0
}

// IsWidthFit returns width-fit mode state.
func (d *Document) IsWidthFit() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.widthFit
}

// IsHeightFit returns height-fit mode state.
func (d *Document) IsHeightFit() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.heightFit
}

// IsUseBannerView reports whether banner view can be used.
func (d *Document) IsUseBannerView(pageIndex int) bool {
	d.mu.RLock()
	banner := d.bannerView
	d.mu.RUnlock()
	if banner == nil {
		return false
	}

	count, err := d.PageCount()
	if err != nil {
		return false
	}
	return pageIndex <= 0 || pageIndex == count
}

// CalcurateZoomForHeightFit computes zoom that fits current page height to target pixels.
func (d *Document) CalcurateZoomForHeightFit(targetHeight float64) float64 {
	pageNumber := d.currentPageNumber()
	if pageNumber <= 0 {
		return 0
	}
	return d.calcurateZoomForHeightFit(pageNumber, targetHeight)
}

// CalcurateZoomForWidthFit computes zoom that fits current page width to target pixels.
func (d *Document) CalcurateZoomForWidthFit(targetWidth float64) float64 {
	pageNumber := d.currentPageNumber()
	if pageNumber <= 0 {
		return 0
	}
	return d.calcurateZoomForWidthFit(pageNumber, targetWidth)
}

// CheckPrivatePieceInfo reports whether page-piece info exists in session state.
func (d *Document) CheckPrivatePieceInfo() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.pagePieceInfo) > 0
}

// GetOtherPageInDoublePageView returns adjacent page in simple double-page layout.
// It accepts optional page number and falls back to current page when omitted.
func (d *Document) GetOtherPageInDoublePageView(pageNumbers ...int) int {
	pageNumber := d.currentPageNumber()
	if len(pageNumbers) > 0 {
		pageNumber = pageNumbers[0]
	}
	if !d.IsValidPage(pageNumber) {
		return -1
	}

	switch {
	case d.IsLeftInDoublePageView(pageNumber) && d.IsBookReadDirectionR2L():
		pageNumber--
	case d.IsLeftInDoublePageView(pageNumber):
		pageNumber++
	case d.IsBookReadDirectionR2L():
		pageNumber++
	default:
		pageNumber--
	}

	if !d.IsValidPage(pageNumber) {
		return -1
	}
	return pageNumber
}

// IsNearPage reports whether pages are near.
// With one argument, it compares against current page; with two, it compares the given pair.
func (d *Document) IsNearPage(pageNumbers ...int) bool {
	switch len(pageNumbers) {
	case 1:
		page := pageNumbers[0]
		if !d.IsValidPage(page) {
			return false
		}
		current := d.currentPageNumber()
		if current <= 0 {
			return false
		}
		return math.Abs(float64(page-current)) <= 1
	case 2:
		return math.Abs(float64(pageNumbers[0]-pageNumbers[1])) <= 1
	default:
		return false
	}
}

// IsLeadInDoublePageView reports whether page number is a lead page in simple double-page layout.
func (d *Document) IsLeadInDoublePageView(pageNumber int) bool {
	// Java has configuration for cover-page offset. In headless mode use no-cover default.
	return pageNumber%2 == 1
}

// IsLeftInDoublePageView reports whether page number is on left side in simple double-page layout.
func (d *Document) IsLeftInDoublePageView(pageNumber int) bool {
	lead := d.IsLeadInDoublePageView(pageNumber)
	if d.IsBookReadDirectionR2L() {
		return !lead
	}
	return lead
}

// UpdatePage sets current page and optional zoom.
func (d *Document) UpdatePage(pageNumber int, zoomValues ...float64) {
	if !d.IsValidPage(pageNumber) {
		return
	}

	zoom := d.GetZoom()
	if len(zoomValues) > 0 {
		if zoomValues[0] <= 0 {
			return
		}
		zoom = zoomValues[0]
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.currentPage = pageNumber
	if len(zoomValues) > 0 && zoom != d.zoom {
		d.widthFit = false
		d.heightFit = false
	}
	d.zoom = zoom
}

// UpdatePageNext moves to the next page when available.
func (d *Document) UpdatePageNext() {
	next := d.currentPageNumber() + 1
	if d.IsValidPage(next) {
		d.UpdatePage(next)
	}
}

// UpdatePagePrev moves to the previous page when available.
func (d *Document) UpdatePagePrev() {
	prev := d.currentPageNumber() - 1
	if d.IsValidPage(prev) {
		d.UpdatePage(prev)
	}
}

// UpdatePageWidthFit updates page with width-fit zoom and mode flag.
func (d *Document) UpdatePageWidthFit(pageNumber int, availableWidth int) {
	if availableWidth <= 0 {
		return
	}
	zoom := d.calcurateZoomForWidthFit(pageNumber, float64(availableWidth))
	if zoom <= 0 {
		return
	}
	d.UpdatePage(pageNumber, zoom)
	d.mu.Lock()
	d.widthFit = true
	d.heightFit = false
	d.mu.Unlock()
}

// UpdatePageHeightFit updates page with height-fit zoom and mode flag.
func (d *Document) UpdatePageHeightFit(pageNumber int, availableHeight int) {
	if availableHeight <= 0 {
		return
	}
	zoom := d.calcurateZoomForHeightFit(pageNumber, float64(availableHeight))
	if zoom <= 0 {
		return
	}
	d.UpdatePage(pageNumber, zoom)
	d.mu.Lock()
	d.widthFit = false
	d.heightFit = true
	d.mu.Unlock()
}

// SetPaperColor sets session paper color.
func (d *Document) SetPaperColor(color int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.paperColor = color
}

// SetScreenWatermarks stores session screen watermark labels.
func (d *Document) SetScreenWatermarks(values []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.screenWatermarks = append([]string(nil), values...)
}

// ClearScreenWatermarks clears all screen watermark entries.
func (d *Document) ClearScreenWatermarks() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.screenWatermarks = nil
}

// GetScreenWatermarks returns configured screen watermark labels.
func (d *Document) GetScreenWatermarks() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.screenWatermarks) == 0 {
		return nil
	}
	return append([]string(nil), d.screenWatermarks...)
}

// SetInstantWatermarks stores session instant watermark labels.
func (d *Document) SetInstantWatermarks(values []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.instantWatermarks = append([]string(nil), values...)
}

// ClearInstantWatermarks clears all instant watermark entries.
func (d *Document) ClearInstantWatermarks() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.instantWatermarks = nil
}

// SetScreenWatermarksHidden sets screen watermark visibility flag.
func (d *Document) SetScreenWatermarksHidden(hidden bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.screenWatermarkHide = hidden
}

// GetScreenWatermarksHidden reports current screen watermark hidden state.
func (d *Document) GetScreenWatermarksHidden() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.screenWatermarkHide
}

// GetDocinfoPageBackgroundColor is a Java-parity alias of GetPageBackgroundColor.
func (d *Document) GetDocinfoPageBackgroundColor() int {
	return d.GetPageBackgroundColor()
}

// PagePieceInfoGetBooleanValue returns one boolean page-piece info value.
func (d *Document) PagePieceInfoGetBooleanValue(pageIndex int, infoName, key string, defaultValue bool) bool {
	value, ok := d.pagePieceInfoValue(pageIndex, infoName, key)
	if !ok {
		return defaultValue
	}
	parsed, ok := value.(bool)
	if !ok {
		return defaultValue
	}
	return parsed
}

// PagePieceInfoGetIntArrayValue returns one int-array page-piece info value.
func (d *Document) PagePieceInfoGetIntArrayValue(pageIndex int, infoName, key string) []int {
	value, ok := d.pagePieceInfoValue(pageIndex, infoName, key)
	if !ok {
		return nil
	}
	if parsed, ok := value.([]int); ok {
		return append([]int(nil), parsed...)
	}
	return nil
}

// PagePieceInfoGetIntValue returns one int page-piece info value.
func (d *Document) PagePieceInfoGetIntValue(pageIndex int, infoName, key string, defaultValue int) int {
	value, ok := d.pagePieceInfoValue(pageIndex, infoName, key)
	if !ok {
		return defaultValue
	}
	if parsed, ok := value.(int); ok {
		return parsed
	}
	return defaultValue
}

// PagePieceInfoGetRefArrayValue returns one ref-array page-piece info value as integer refs.
func (d *Document) PagePieceInfoGetRefArrayValue(pageIndex int, infoName, key string) []int {
	return d.PagePieceInfoGetIntArrayValue(pageIndex, infoName, key)
}

// PagePieceInfoGetStringValue returns one string page-piece info value.
func (d *Document) PagePieceInfoGetStringValue(pageIndex int, infoName, key string) string {
	value, ok := d.pagePieceInfoValue(pageIndex, infoName, key)
	if !ok {
		return ""
	}
	if parsed, ok := value.(string); ok {
		return parsed
	}
	return ""
}

// PagePieceInfoSetBooleanValue stores one boolean page-piece info value.
func (d *Document) PagePieceInfoSetBooleanValue(pageIndex int, infoName, key string, value bool) bool {
	return d.setPagePieceInfoValue(pageIndex, infoName, key, value)
}

// PagePieceInfoSetIntValue stores one int page-piece info value.
func (d *Document) PagePieceInfoSetIntValue(pageIndex int, infoName, key string, value int) bool {
	return d.setPagePieceInfoValue(pageIndex, infoName, key, value)
}

// PagePieceInfoSetStringValue stores one string page-piece info value.
func (d *Document) PagePieceInfoSetStringValue(pageIndex int, infoName, key, value string) bool {
	return d.setPagePieceInfoValue(pageIndex, infoName, key, value)
}

func (d *Document) setPagePieceInfoValue(pageIndex int, infoName, key string, value interface{}) bool {
	if strings.TrimSpace(infoName) == "" || strings.TrimSpace(key) == "" {
		return false
	}

	if _, err := d.resolveSourcePageIndex(pageIndex); err != nil {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	infoMap, ok := d.pagePieceInfo[pageIndex]
	if !ok {
		infoMap = make(map[string]map[string]interface{})
		d.pagePieceInfo[pageIndex] = infoMap
	}
	keyMap, ok := infoMap[infoName]
	if !ok {
		keyMap = make(map[string]interface{})
		infoMap[infoName] = keyMap
	}
	keyMap[key] = value
	return true
}

func (d *Document) pagePieceInfoValue(pageIndex int, infoName, key string) (interface{}, bool) {
	if strings.TrimSpace(infoName) == "" || strings.TrimSpace(key) == "" {
		return nil, false
	}

	d.mu.RLock()
	defer d.mu.RUnlock()
	infoMap, ok := d.pagePieceInfo[pageIndex]
	if !ok {
		return nil, false
	}
	keyMap, ok := infoMap[infoName]
	if !ok {
		return nil, false
	}
	value, ok := keyMap[key]
	return value, ok
}

func (d *Document) currentPageNumber() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentPage
}

func (d *Document) calcurateZoomForWidthFit(pageNumber int, targetWidth float64) float64 {
	if targetWidth <= 0 || !d.IsValidPage(pageNumber) {
		return 0
	}
	width100, err := d.GetPageWidth100(pageNumber - 1)
	if err != nil || width100 <= 0 {
		return 0
	}
	return targetWidth / float64(width100)
}

func (d *Document) calcurateZoomForHeightFit(pageNumber int, targetHeight float64) float64 {
	if targetHeight <= 0 || !d.IsValidPage(pageNumber) {
		return 0
	}
	height100, err := d.GetPageHeight100(pageNumber - 1)
	if err != nil || height100 <= 0 {
		return 0
	}
	return targetHeight / float64(height100)
}
