package pdf

import (
	"fmt"
	"sync"
)

// ViewerLifecycleState represents one viewer lifecycle state.
type ViewerLifecycleState string

const (
	// ViewerLifecycleCreated means the viewer has been initialized.
	ViewerLifecycleCreated ViewerLifecycleState = "created"
	// ViewerLifecycleResumed means the viewer is active.
	ViewerLifecycleResumed ViewerLifecycleState = "resumed"
	// ViewerLifecyclePaused means the viewer is paused.
	ViewerLifecyclePaused ViewerLifecycleState = "paused"
	// ViewerLifecycleDestroyed means the viewer has been destroyed.
	ViewerLifecycleDestroyed ViewerLifecycleState = "destroyed"
)

// ViewerSessionOptions configures initial viewer session state.
type ViewerSessionOptions struct {
	InitialPage     int
	InitialZoom     float64
	MinZoom         float64
	MaxZoom         float64
	ToolbarVisible  bool
	MenuVisible     bool
	PageCurlEnabled bool
}

// ViewerSessionSnapshot captures current viewer state.
type ViewerSessionSnapshot struct {
	LifecycleState  ViewerLifecycleState
	CurrentPage     int
	Zoom            float64
	ToolbarVisible  bool
	MenuVisible     bool
	PageCurlEnabled bool
}

// ViewerSession models Android-style PDF viewer lifecycle and UI state in headless mode.
type ViewerSession struct {
	doc             *Document
	state           ViewerLifecycleState
	currentPage     int
	zoom            float64
	minZoom         float64
	maxZoom         float64
	mu              sync.RWMutex
	toolbarVisible  bool
	menuVisible     bool
	pageCurlEnabled bool
}

// NewViewerSession creates a new headless viewer session for one document.
func NewViewerSession(doc *Document, options ViewerSessionOptions) (*ViewerSession, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}

	pageCount, err := doc.PageCount()
	if err != nil {
		return nil, err
	}
	if pageCount <= 0 {
		return nil, fmt.Errorf("document has no pages")
	}

	minZoom := options.MinZoom
	if minZoom <= 0 {
		minZoom = 0.5
	}
	maxZoom := options.MaxZoom
	if maxZoom <= 0 {
		maxZoom = 5.0
	}
	if minZoom > maxZoom {
		return nil, fmt.Errorf("invalid zoom bounds: min %.2f > max %.2f", minZoom, maxZoom)
	}

	currentPage := options.InitialPage
	if currentPage < 0 || currentPage >= pageCount {
		currentPage = 0
	}

	zoom := options.InitialZoom
	if zoom <= 0 {
		zoom = 1.0
	}
	zoom = clampViewerZoom(zoom, minZoom, maxZoom)

	return &ViewerSession{
		doc:             doc,
		state:           ViewerLifecycleCreated,
		currentPage:     currentPage,
		zoom:            zoom,
		minZoom:         minZoom,
		maxZoom:         maxZoom,
		toolbarVisible:  options.ToolbarVisible,
		menuVisible:     options.MenuVisible,
		pageCurlEnabled: options.PageCurlEnabled,
	}, nil
}

// OnCreate transitions session to created state.
func (s *ViewerSession) OnCreate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}
	s.state = ViewerLifecycleCreated
	return nil
}

// OnResume transitions session to resumed state.
func (s *ViewerSession) OnResume() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}
	s.state = ViewerLifecycleResumed
	return nil
}

// OnPause transitions session to paused state.
func (s *ViewerSession) OnPause() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}
	s.state = ViewerLifecyclePaused
	return nil
}

// OnDestroy transitions session to destroyed state.
func (s *ViewerSession) OnDestroy() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = ViewerLifecycleDestroyed
}

// LifecycleState returns current lifecycle state.
func (s *ViewerSession) LifecycleState() ViewerLifecycleState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// CurrentPage returns current page index.
func (s *ViewerSession) CurrentPage() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentPage
}

// SetCurrentPage sets current page index.
func (s *ViewerSession) SetCurrentPage(page int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}

	pageCount, err := s.doc.PageCount()
	if err != nil {
		return err
	}
	if page < 0 || page >= pageCount {
		return fmt.Errorf("page index out of range: %d", page)
	}

	s.currentPage = page
	return nil
}

// NextPage advances one page.
func (s *ViewerSession) NextPage() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return 0, fmt.Errorf("viewer session is destroyed")
	}

	pageCount, err := s.doc.PageCount()
	if err != nil {
		return 0, err
	}
	if s.currentPage+1 >= pageCount {
		return s.currentPage, fmt.Errorf("already at last page")
	}

	s.currentPage++
	return s.currentPage, nil
}

// PreviousPage moves back one page.
func (s *ViewerSession) PreviousPage() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return 0, fmt.Errorf("viewer session is destroyed")
	}
	if s.currentPage <= 0 {
		return s.currentPage, fmt.Errorf("already at first page")
	}

	s.currentPage--
	return s.currentPage, nil
}

// Zoom returns current zoom scale.
func (s *ViewerSession) Zoom() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.zoom
}

// SetZoom sets current zoom scale within [MinZoom, MaxZoom].
func (s *ViewerSession) SetZoom(value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}
	if value <= 0 {
		return fmt.Errorf("zoom must be positive")
	}

	s.zoom = clampViewerZoom(value, s.minZoom, s.maxZoom)
	return nil
}

// ToolbarVisible returns toolbar visibility state.
func (s *ViewerSession) ToolbarVisible() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toolbarVisible
}

// SetToolbarVisible sets toolbar visibility state.
func (s *ViewerSession) SetToolbarVisible(visible bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}
	s.toolbarVisible = visible
	return nil
}

// MenuVisible returns menu visibility state.
func (s *ViewerSession) MenuVisible() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.menuVisible
}

// SetMenuVisible sets menu visibility state.
func (s *ViewerSession) SetMenuVisible(visible bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}
	s.menuVisible = visible
	return nil
}

// PageCurlEnabled returns whether page-curl mode is enabled.
func (s *ViewerSession) PageCurlEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pageCurlEnabled
}

// SetPageCurlEnabled sets page-curl mode.
func (s *ViewerSession) SetPageCurlEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}
	s.pageCurlEnabled = enabled
	return nil
}

// Snapshot captures current viewer state.
func (s *ViewerSession) Snapshot() ViewerSessionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ViewerSessionSnapshot{
		LifecycleState:  s.state,
		CurrentPage:     s.currentPage,
		Zoom:            s.zoom,
		ToolbarVisible:  s.toolbarVisible,
		MenuVisible:     s.menuVisible,
		PageCurlEnabled: s.pageCurlEnabled,
	}
}

// Restore restores viewer state from a snapshot.
func (s *ViewerSession) Restore(snapshot ViewerSessionSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == ViewerLifecycleDestroyed {
		return fmt.Errorf("viewer session is destroyed")
	}

	pageCount, err := s.doc.PageCount()
	if err != nil {
		return err
	}
	if snapshot.CurrentPage < 0 || snapshot.CurrentPage >= pageCount {
		return fmt.Errorf("snapshot page out of range: %d", snapshot.CurrentPage)
	}
	if snapshot.Zoom <= 0 {
		return fmt.Errorf("snapshot zoom must be positive")
	}

	s.state = snapshot.LifecycleState
	s.currentPage = snapshot.CurrentPage
	s.zoom = clampViewerZoom(snapshot.Zoom, s.minZoom, s.maxZoom)
	s.toolbarVisible = snapshot.ToolbarVisible
	s.menuVisible = snapshot.MenuVisible
	s.pageCurlEnabled = snapshot.PageCurlEnabled
	return nil
}

func clampViewerZoom(value, minZoom, maxZoom float64) float64 {
	if value < minZoom {
		return minZoom
	}
	if value > maxZoom {
		return maxZoom
	}
	return value
}
