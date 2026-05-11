package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestExecuteOutlineAction_HideTargets(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	result, err := doc.ExecuteOutlineAction(&OutlineAction{
		Type:        "Hide",
		Hide:        true,
		HideTargets: []string{"A", "B"},
	}, ActionExecutionOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, []string{"A", "B"}, result.HiddenTargets)
	assert.True(t, doc.IsActionTargetHidden("A"))
	assert.True(t, doc.IsActionTargetHidden("B"))
}

func TestExecuteOutlineAction_HideHandler(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	called := 0
	result, err := doc.ExecuteOutlineAction(&OutlineAction{
		Type:        "Hide",
		Hide:        false,
		HideTargets: []string{"A", "A", "B"},
	}, ActionExecutionOptions{
		OnHide: func(action *OutlineAction, hiddenTargets []string) error {
			called++
			assert.Equal(t, []string{"A", "B"}, hiddenTargets)
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, called)
	assert.True(t, result.HandlerInvoked)
}

func TestExecuteOutlineAction_ImportDataRequiresPayload(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	_, err := doc.ExecuteOutlineAction(&OutlineAction{
		Type: "ImportData",
		File: "form.xfdf",
	}, ActionExecutionOptions{})
	require.Error(t, err)
}

func TestParseActionImportData_XFDF(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/">
  <fields>
    <field name="Name"><value>Alice</value></field>
  </fields>
</xfdf>`)

	parsed, err := parseActionImportData(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"Alice"}, parsed.Fields["Name"])
}

func TestExecuteOutlineActionByPath_InvalidPath(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	_, err := doc.ExecuteOutlineActionByPath([]int{0}, ActionExecutionOptions{})
	require.Error(t, err)
}

func TestExecuteOutlineAction_RenditionHandler(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	called := false
	multimediaCalled := false
	played := ""
	result, err := doc.ExecuteOutlineAction(&OutlineAction{
		Type:               "Rendition",
		RenditionName:      "video",
		RenditionOperation: 1,
		RenditionFile:      "sample.mp4",
	}, ActionExecutionOptions{
		OnRendition: func(action *OutlineAction) error {
			called = true
			return nil
		},
		OnMultimedia: func(action *OutlineAction) error {
			multimediaCalled = true
			return nil
		},
		OnRenditionPlayback: func(action *OutlineAction, mediaFile string) error {
			played = mediaFile
			return nil
		},
	})
	require.NoError(t, err)
	require.True(t, called)
	require.True(t, multimediaCalled)
	require.NotNil(t, result)
	assert.True(t, result.HandlerInvoked)
	assert.Equal(t, "video", result.RenditionName)
	assert.Equal(t, 1, result.RenditionOperation)
	assert.Equal(t, "sample.mp4", played)
	assert.Equal(t, "sample.mp4", result.RenditionFile)
}

func TestExecuteOutlineAction_RenditionNativePlayback(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	gotCmd := ""
	gotArgs := []string(nil)
	_, err := doc.ExecuteOutlineAction(&OutlineAction{
		Type:          "Rendition",
		RenditionFile: "media/test.mp4",
	}, ActionExecutionOptions{
		EnableNativeRenditionPlayback: true,
		NativeRenditionCommandRunner: func(command string, args ...string) error {
			gotCmd = command
			gotArgs = append([]string(nil), args...)
			return nil
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, gotCmd)
	require.NotEmpty(t, gotArgs)
	assert.Equal(t, "media/test.mp4", gotArgs[len(gotArgs)-1])
}

func TestExecuteOutlineAction_NextChain(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	uriCalled := 0
	jsCalled := 0

	result, err := doc.ExecuteOutlineAction(&OutlineAction{
		Type: "URI",
		URI:  "https://example.com",
		NextActions: []*OutlineAction{
			{
				Type:       "JavaScript",
				JavaScript: "app.alert('next')",
			},
			{
				Type:        "Hide",
				Hide:        true,
				HideTargets: []string{"ChainTarget"},
			},
		},
	}, ActionExecutionOptions{
		OnURI: func(action *OutlineAction) error {
			uriCalled++
			return nil
		},
		OnJavaScript: func(action *OutlineAction) error {
			jsCalled++
			return nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.NextResults, 2)
	assert.Equal(t, 1, uriCalled)
	assert.Equal(t, 1, jsCalled)
	assert.Equal(t, "JavaScript", result.NextResults[0].Type)
	assert.Equal(t, "Hide", result.NextResults[1].Type)
	assert.Equal(t, []string{"ChainTarget"}, result.NextResults[1].HiddenTargets)
	assert.True(t, doc.IsActionTargetHidden("ChainTarget"))
}

func TestExecuteOutlineAction_NavigationHandlers(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	gotGoTo := 0
	gotGoToR := 0
	gotNamed := 0

	_, err := doc.ExecuteOutlineAction(&OutlineAction{Type: "GoTo"}, ActionExecutionOptions{
		OnGoTo: func(action *OutlineAction) error {
			gotGoTo++
			return nil
		},
	})
	require.NoError(t, err)

	_, err = doc.ExecuteOutlineAction(&OutlineAction{Type: "GoToR"}, ActionExecutionOptions{
		OnGoToR: func(action *OutlineAction) error {
			gotGoToR++
			return nil
		},
	})
	require.NoError(t, err)

	result, err := doc.ExecuteOutlineAction(&OutlineAction{Type: "Named", Named: "NextPage"}, ActionExecutionOptions{
		OnNamed: func(action *OutlineAction) error {
			gotNamed++
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HandlerInvoked)
	assert.Equal(t, 1, gotGoTo)
	assert.Equal(t, 1, gotGoToR)
	assert.Equal(t, 1, gotNamed)
}

func TestExecuteOutlineAction_ExtendedHandlers(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	fileAttachmentCalled := 0
	scrollLockCalled := 0
	richMediaCalled := 0
	multimediaCalled := 0

	result, err := doc.ExecuteOutlineAction(&OutlineAction{
		Type: "FileAttachment",
	}, ActionExecutionOptions{
		OnFileAttachment: func(action *OutlineAction) error {
			fileAttachmentCalled++
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HandlerInvoked)

	result, err = doc.ExecuteOutlineAction(&OutlineAction{
		Type: "ScrollLock",
	}, ActionExecutionOptions{
		OnScrollLock: func(action *OutlineAction) error {
			scrollLockCalled++
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HandlerInvoked)

	result, err = doc.ExecuteOutlineAction(&OutlineAction{
		Type: "RichMedia",
	}, ActionExecutionOptions{
		OnRichMedia: func(action *OutlineAction) error {
			richMediaCalled++
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HandlerInvoked)

	result, err = doc.ExecuteOutlineAction(&OutlineAction{
		Type:          "Multimedia",
		RenditionFile: "movie.mp4",
	}, ActionExecutionOptions{
		OnMultimedia: func(action *OutlineAction) error {
			multimediaCalled++
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HandlerInvoked)

	assert.Equal(t, 1, fileAttachmentCalled)
	assert.Equal(t, 1, scrollLockCalled)
	assert.Equal(t, 1, richMediaCalled)
	assert.Equal(t, 1, multimediaCalled)
}
