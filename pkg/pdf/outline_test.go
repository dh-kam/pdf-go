package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestParseOutlineAction_GoToR(t *testing.T) {
	doc := &Document{doc: entity.NewDocument(nil)}
	pageRef := entity.NewRef(10, 0)

	actionDict := entity.NewDict()
	actionDict.Set(entity.Name("S"), entity.NewName("GoToR"))
	actionDict.Set(entity.Name("F"), entity.NewString("external.pdf"))
	actionDict.Set(entity.Name("D"), entity.NewArray(pageRef, entity.NewName("Fit")))
	actionDict.Set(entity.Name("NewWindow"), entity.NewBoolean(true))

	action, err := doc.parseOutlineAction(actionDict, map[entity.Ref]int{pageRef: 2})
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, "GoToR", action.Type)
	assert.Equal(t, "external.pdf", action.File)
	assert.Equal(t, 2, action.PageIndex)
	assert.NotNil(t, action.Dest)
	assert.True(t, action.HasNewWindow)
	assert.True(t, action.NewWindow)
}

func TestParseOutlineAction_LaunchPlatformFallback(t *testing.T) {
	doc := &Document{doc: entity.NewDocument(nil)}

	winDict := entity.NewDict()
	winDict.Set(entity.Name("F"), entity.NewString("viewer.exe"))
	winDict.Set(entity.Name("P"), entity.NewString("/open"))
	winDict.Set(entity.Name("D"), entity.NewString("C:/Program Files/Viewer"))
	winDict.Set(entity.Name("O"), entity.NewString("open"))

	actionDict := entity.NewDict()
	actionDict.Set(entity.Name("S"), entity.NewName("Launch"))
	actionDict.Set(entity.Name("Win"), winDict)

	action, err := doc.parseOutlineAction(actionDict, nil)
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, "Launch", action.Type)
	assert.Equal(t, "viewer.exe", action.File)
	assert.Equal(t, "/open", action.Command)
	assert.Equal(t, "C:/Program Files/Viewer", action.Directory)
	assert.Equal(t, "open", action.Operation)
}

func TestParseOutlineAction_JavaScriptStream(t *testing.T) {
	doc := &Document{doc: entity.NewDocument(nil)}

	actionDict := entity.NewDict()
	actionDict.Set(entity.Name("S"), entity.NewName("JavaScript"))
	actionDict.Set(entity.Name("JS"), entity.NewStream(entity.NewDict(), []byte("app.alert('hello');")))

	action, err := doc.parseOutlineAction(actionDict, nil)
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, "JavaScript", action.Type)
	assert.Equal(t, "app.alert('hello');", action.JavaScript)
}

func TestParseOutlineAction_SubmitFormFields(t *testing.T) {
	doc := &Document{doc: entity.NewDocument(nil)}

	actionDict := entity.NewDict()
	actionDict.Set(entity.Name("S"), entity.NewName("SubmitForm"))
	actionDict.Set(entity.Name("F"), entity.NewString("https://example.com/submit"))
	actionDict.Set(entity.Name("Fields"), entity.NewArray(
		entity.NewString("Name"),
		entity.NewString("Check"),
		entity.NewString("Name"),
	))
	actionDict.Set(entity.Name("Flags"), entity.NewInteger(1))

	action, err := doc.parseOutlineAction(actionDict, nil)
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, "SubmitForm", action.Type)
	assert.Equal(t, "https://example.com/submit", action.File)
	assert.Equal(t, []string{"Name", "Check"}, action.FieldNames)
	assert.Equal(t, 1, action.Flags)
	assert.True(t, action.ExcludeFields)
}

func TestParseOutlineAction_Hide(t *testing.T) {
	doc := &Document{doc: entity.NewDocument(nil)}

	actionDict := entity.NewDict()
	actionDict.Set(entity.Name("S"), entity.NewName("Hide"))
	actionDict.Set(entity.Name("H"), entity.NewBoolean(false))
	actionDict.Set(entity.Name("T"), entity.NewArray(
		entity.NewString("FieldA"),
		entity.NewString("FieldB"),
	))

	action, err := doc.parseOutlineAction(actionDict, nil)
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, "Hide", action.Type)
	assert.True(t, action.HasHide)
	assert.False(t, action.Hide)
	assert.Equal(t, []string{"FieldA", "FieldB"}, action.HideTargets)
}

func TestParseOutlineAction_NextChain(t *testing.T) {
	doc := &Document{doc: entity.NewDocument(nil)}

	nextOne := entity.NewDict()
	nextOne.Set(entity.Name("S"), entity.NewName("JavaScript"))
	nextOne.Set(entity.Name("JS"), entity.NewString("app.alert('next1')"))

	nextTwo := entity.NewDict()
	nextTwo.Set(entity.Name("S"), entity.NewName("Hide"))
	nextTwo.Set(entity.Name("T"), entity.NewString("FieldX"))

	actionDict := entity.NewDict()
	actionDict.Set(entity.Name("S"), entity.NewName("URI"))
	actionDict.Set(entity.Name("URI"), entity.NewString("https://example.com"))
	actionDict.Set(entity.Name("Next"), entity.NewArray(nextOne, nextTwo))

	action, err := doc.parseOutlineAction(actionDict, nil)
	require.NoError(t, err)
	require.NotNil(t, action)
	require.Len(t, action.NextActions, 2)

	assert.Equal(t, "JavaScript", action.NextActions[0].Type)
	assert.Equal(t, "app.alert('next1')", action.NextActions[0].JavaScript)
	assert.Equal(t, "Hide", action.NextActions[1].Type)
	assert.Equal(t, []string{"FieldX"}, action.NextActions[1].HideTargets)
}

func TestParseOutlineAction_RenditionMedia(t *testing.T) {
	doc := &Document{doc: entity.NewDocument(nil)}

	fileSpec := entity.NewDict()
	fileSpec.Set(entity.Name("F"), entity.NewString("media/video.mp4"))

	clip := entity.NewDict()
	clip.Set(entity.Name("CT"), entity.NewString("video/mp4"))
	clip.Set(entity.Name("D"), fileSpec)

	rendition := entity.NewDict()
	rendition.Set(entity.Name("N"), entity.NewString("Intro"))
	rendition.Set(entity.Name("C"), clip)

	actionDict := entity.NewDict()
	actionDict.Set(entity.Name("S"), entity.NewName("Rendition"))
	actionDict.Set(entity.Name("OP"), entity.NewInteger(0))
	actionDict.Set(entity.Name("R"), rendition)

	action, err := doc.parseOutlineAction(actionDict, nil)
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, "Rendition", action.Type)
	assert.Equal(t, "Intro", action.RenditionName)
	assert.Equal(t, "media/video.mp4", action.RenditionFile)
	assert.Equal(t, "video/mp4", action.RenditionMIMEType)
}

func TestParseOutlineNode_Color(t *testing.T) {
	doc := &Document{doc: entity.NewDocument(nil)}

	node := entity.NewDict()
	node.Set(entity.Name("Title"), entity.NewString("Colored"))
	node.Set(entity.Name("C"), entity.NewArray(
		entity.NewReal(1.0),
		entity.NewReal(0.5),
		entity.NewReal(0.0),
	))

	outline, err := doc.parseOutlineNode(node, map[*entity.Dict]struct{}{}, nil)
	require.NoError(t, err)
	require.NotNil(t, outline)
	assert.Equal(t, 0xFFFF7F00, outline.Color)
}
