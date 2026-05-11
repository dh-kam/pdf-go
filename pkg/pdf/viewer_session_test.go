package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestViewerSession_Lifecycle(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0, 1, 2}

	session, err := NewViewerSession(doc, ViewerSessionOptions{})
	require.NoError(t, err)
	assert.Equal(t, ViewerLifecycleCreated, session.LifecycleState())
	require.NoError(t, session.OnCreate())
	assert.Equal(t, ViewerLifecycleCreated, session.LifecycleState())

	require.NoError(t, session.OnResume())
	assert.Equal(t, ViewerLifecycleResumed, session.LifecycleState())

	require.NoError(t, session.OnPause())
	assert.Equal(t, ViewerLifecyclePaused, session.LifecycleState())

	session.OnDestroy()
	assert.Equal(t, ViewerLifecycleDestroyed, session.LifecycleState())
	require.Error(t, session.OnResume())
	require.Error(t, session.OnCreate())
}

func TestViewerSession_PageNavigation(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0, 1, 2}

	session, err := NewViewerSession(doc, ViewerSessionOptions{InitialPage: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, session.CurrentPage())

	next, err := session.NextPage()
	require.NoError(t, err)
	assert.Equal(t, 2, next)

	_, err = session.NextPage()
	require.Error(t, err)

	prev, err := session.PreviousPage()
	require.NoError(t, err)
	assert.Equal(t, 1, prev)

	require.NoError(t, session.SetCurrentPage(0))
	assert.Equal(t, 0, session.CurrentPage())
	require.Error(t, session.SetCurrentPage(5))
}

func TestViewerSession_ZoomAndUIState(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0}

	session, err := NewViewerSession(doc, ViewerSessionOptions{
		MinZoom:         1.0,
		MaxZoom:         2.0,
		ToolbarVisible:  true,
		MenuVisible:     false,
		PageCurlEnabled: true,
	})
	require.NoError(t, err)

	require.NoError(t, session.SetZoom(1.5))
	assert.Equal(t, 1.5, session.Zoom())

	require.NoError(t, session.SetZoom(0.1))
	assert.Equal(t, 1.0, session.Zoom())

	require.NoError(t, session.SetZoom(3.0))
	assert.Equal(t, 2.0, session.Zoom())

	assert.True(t, session.ToolbarVisible())
	require.NoError(t, session.SetToolbarVisible(false))
	assert.False(t, session.ToolbarVisible())

	assert.False(t, session.MenuVisible())
	require.NoError(t, session.SetMenuVisible(true))
	assert.True(t, session.MenuVisible())

	assert.True(t, session.PageCurlEnabled())
	require.NoError(t, session.SetPageCurlEnabled(false))
	assert.False(t, session.PageCurlEnabled())
}

func TestViewerSession_SnapshotRestore(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0, 1}

	session, err := NewViewerSession(doc, ViewerSessionOptions{
		InitialPage:    0,
		InitialZoom:    1.2,
		ToolbarVisible: true,
	})
	require.NoError(t, err)
	require.NoError(t, session.OnResume())
	require.NoError(t, session.SetCurrentPage(1))
	require.NoError(t, session.SetZoom(2.5))
	require.NoError(t, session.SetMenuVisible(true))
	require.NoError(t, session.SetPageCurlEnabled(true))

	snapshot := session.Snapshot()

	other, err := NewViewerSession(doc, ViewerSessionOptions{})
	require.NoError(t, err)
	require.NoError(t, other.Restore(snapshot))

	assert.Equal(t, ViewerLifecycleResumed, other.LifecycleState())
	assert.Equal(t, 1, other.CurrentPage())
	assert.Equal(t, 2.5, other.Zoom())
	assert.True(t, other.ToolbarVisible())
	assert.True(t, other.MenuVisible())
	assert.True(t, other.PageCurlEnabled())
}
