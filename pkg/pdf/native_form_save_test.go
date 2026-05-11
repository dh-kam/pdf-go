package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type nativeFormSaveTestXRef struct {
	objects    map[entity.Ref]entity.Object
	trailer    *entity.Dict
	raw        []byte
	startXRef  uint64
	startXErr  error
	trailerErr error
}

func (x *nativeFormSaveTestXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	if obj, ok := x.objects[ref]; ok {
		return obj, nil
	}
	return nil, assert.AnError
}

func (x *nativeFormSaveTestXRef) GetTrailer() (*entity.Dict, error) {
	if x.trailerErr != nil {
		return nil, x.trailerErr
	}
	return x.trailer, nil
}

func (x *nativeFormSaveTestXRef) RawData() []byte {
	return append([]byte(nil), x.raw...)
}

func (x *nativeFormSaveTestXRef) StartXRefOffset() (uint64, error) {
	if x.startXErr != nil {
		return 0, x.startXErr
	}
	return x.startXRef, nil
}

func (x *nativeFormSaveTestXRef) GetNumObjects() int {
	return len(x.objects) + 1
}

type fetchOnlyXRef struct{}

func (f *fetchOnlyXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	return nil, assert.AnError
}

func TestNativeFormSave_AllocatorAndNameHelpers(t *testing.T) {
	allocator := newNativeObjectAllocator(0)
	first := allocator.Next()
	assert.Equal(t, uint32(1), first.Num())

	allocator.ObserveRef(entity.NewRef(8, 0))
	next := allocator.Next()
	assert.Equal(t, uint32(9), next.Num())

	assert.Nil(t, nativeNamesObject(nil))

	single := nativeNamesObject([]string{"name"})
	singleString, ok := single.(*entity.String)
	require.True(t, ok)
	assert.Equal(t, "name", singleString.Value())

	multiple := nativeNamesObject([]string{"a", "b"})
	multipleArray, ok := multiple.(*entity.Array)
	require.True(t, ok)
	assert.Equal(t, 2, multipleArray.Len())
}

func TestNativeFormSave_PublicObjectAndDestinationHelpers(t *testing.T) {
	obj, ok := nativePublicObjectToEntity("text")
	require.True(t, ok)
	strObj, ok := obj.(*entity.String)
	require.True(t, ok)
	assert.Equal(t, "text", strObj.Value())

	obj, ok = nativePublicObjectToEntity(true)
	require.True(t, ok)
	boolObj, ok := obj.(*entity.Boolean)
	require.True(t, ok)
	assert.True(t, boolObj.Value())

	obj, ok = nativePublicObjectToEntity(int64(42))
	require.True(t, ok)
	intObj, ok := obj.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(42), intObj.Value())

	obj, ok = nativePublicObjectToEntity(float64(1.5))
	require.True(t, ok)
	realObj, ok := obj.(*entity.Real)
	require.True(t, ok)
	assert.Equal(t, 1.5, realObj.Value())

	obj, ok = nativePublicObjectToEntity(&Dict{dict: entity.NewDict()})
	require.True(t, ok)
	_, isDict := obj.(*entity.Dict)
	assert.True(t, isDict)

	obj, ok = nativePublicObjectToEntity(&Array{array: entity.NewArray(entity.NewInteger(1))})
	require.True(t, ok)
	_, isArray := obj.(*entity.Array)
	assert.True(t, isArray)

	_, ok = nativePublicObjectToEntity(struct{}{})
	assert.False(t, ok)

	_, ok = nativePublicObjectToEntity((*Dict)(nil))
	assert.False(t, ok)

	pageRefs := []entity.Ref{entity.NewRef(20, 0)}
	dest := nativeDestinationObject(nil, 0, pageRefs)
	destArr, ok := dest.(*entity.Array)
	require.True(t, ok)
	require.Equal(t, 2, destArr.Len())

	overridden := nativeDestinationObject("namedDest", -1, pageRefs)
	overriddenString, ok := overridden.(*entity.String)
	require.True(t, ok)
	assert.Equal(t, "namedDest", overriddenString.Value())
}

func TestNativeFormSave_BuildNativeOutlineActionObjectBranches(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	pageRefs := []entity.Ref{entity.NewRef(30, 0)}

	testCases := []struct {
		name     string
		action   *OutlineAction
		required []string
	}{
		{
			name:     "URI",
			action:   &OutlineAction{Type: "URI", URI: "https://example.com"},
			required: []string{"S", "URI"},
		},
		{
			name:     "GoTo",
			action:   &OutlineAction{Type: "GoTo", PageIndex: 0},
			required: []string{"S", "D"},
		},
		{
			name: "GoToR",
			action: &OutlineAction{
				Type:         "GoToR",
				File:         "remote.pdf",
				PageIndex:    0,
				HasNewWindow: true,
				NewWindow:    true,
			},
			required: []string{"S", "F", "D", "NewWindow"},
		},
		{
			name:     "Named",
			action:   &OutlineAction{Type: "Named", Named: "NextPage"},
			required: []string{"S", "N"},
		},
		{
			name: "Launch",
			action: &OutlineAction{
				Type:      "Launch",
				File:      "doc.txt",
				Command:   "open",
				Directory: "/tmp",
				Operation: "print",
			},
			required: []string{"S", "F", "P", "D", "O"},
		},
		{
			name:     "JavaScript",
			action:   &OutlineAction{Type: "JavaScript", JavaScript: "app.alert('x')"},
			required: []string{"S", "JS"},
		},
		{
			name: "SubmitForm",
			action: &OutlineAction{
				Type:       "SubmitForm",
				File:       "submit.fdf",
				FieldNames: []string{"A", "B"},
				Flags:      3,
			},
			required: []string{"S", "F", "Fields", "Flags"},
		},
		{
			name: "ResetForm",
			action: &OutlineAction{
				Type:       "ResetForm",
				FieldNames: []string{"A"},
				Flags:      2,
			},
			required: []string{"S", "Fields", "Flags"},
		},
		{
			name: "Hide",
			action: &OutlineAction{
				Type:        "Hide",
				HasHide:     true,
				Hide:        false,
				HideTargets: []string{"A", "B"},
			},
			required: []string{"S", "H", "T"},
		},
		{
			name: "Rendition",
			action: &OutlineAction{
				Type:               "Rendition",
				RenditionOperation: 1,
				RenditionName:      "clip",
				RenditionFile:      "media.mp4",
				RenditionMIMEType:  "video/mp4",
			},
			required: []string{"S", "OP", "R"},
		},
		{
			name: "Default",
			action: &OutlineAction{
				Type:       "Custom",
				URI:        "u",
				Named:      "n",
				File:       "f",
				JavaScript: "js",
				PageIndex:  0,
			},
			required: []string{"S", "URI", "N", "F", "JS", "D"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dict, err := doc.buildNativeOutlineActionObject(tc.action, pageRefs, 0)
			require.NoError(t, err)
			require.NotNil(t, dict)
			for _, key := range tc.required {
				assert.NotNil(t, dict.GetRaw(entity.Name(key)))
			}
		})
	}

	none, err := doc.buildNativeOutlineActionObject(nil, pageRefs, 0)
	require.NoError(t, err)
	assert.Nil(t, none)

	singleNext, err := doc.buildNativeOutlineActionObject(&OutlineAction{
		Type:        "URI",
		URI:         "root",
		NextActions: []*OutlineAction{{Type: "Named", Named: "One"}},
	}, pageRefs, 0)
	require.NoError(t, err)
	require.NotNil(t, singleNext)
	_, ok := singleNext.GetRaw(entity.Name("Next")).(*entity.Dict)
	assert.True(t, ok)

	multiNext, err := doc.buildNativeOutlineActionObject(&OutlineAction{
		Type: "URI",
		URI:  "root",
		NextActions: []*OutlineAction{
			{Type: "Named", Named: "One"},
			{Type: "Named", Named: "Two"},
		},
	}, pageRefs, 0)
	require.NoError(t, err)
	require.NotNil(t, multiNext)
	_, ok = multiNext.GetRaw(entity.Name("Next")).(*entity.Array)
	assert.True(t, ok)

	_, err = doc.buildNativeOutlineActionObject(&OutlineAction{Type: "URI"}, pageRefs, 17)
	require.Error(t, err)
	assert.ErrorContains(t, err, "depth exceeded")
}

func TestNativeFormSave_ResolveWritableAndTrailerHelpers(t *testing.T) {
	pageRef := entity.NewRef(40, 0)
	acroRef := entity.NewRef(41, 0)

	pageDict := entity.NewDict()
	pageDict.Set(entity.Name("Type"), entity.NewName("Page"))
	acroDict := entity.NewDict()
	acroDict.Set(entity.Name("Fields"), entity.NewArray())

	xref := &nativeFormSaveTestXRef{
		objects: map[entity.Ref]entity.Object{
			pageRef: pageDict,
			acroRef: acroDict,
		},
	}
	entityDoc := entity.NewDocument(xref)
	entityDoc.SetCatalog(entity.NewDict())
	doc := newDocument(entityDoc)

	pageUpdates := map[entity.Ref]entity.Object{}
	clonedPage, err := doc.resolveWritablePageDict(pageRef, pageUpdates)
	require.NoError(t, err)
	require.NotNil(t, clonedPage)
	require.Contains(t, pageUpdates, pageRef)

	_, err = doc.resolveWritablePageDict(pageRef, map[entity.Ref]entity.Object{
		pageRef: entity.NewArray(),
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "page update is not dictionary")

	xref.objects[pageRef] = entity.NewArray()
	_, err = doc.resolveWritablePageDict(pageRef, map[entity.Ref]entity.Object{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "page is not dictionary")
	xref.objects[pageRef] = pageDict

	rootWithoutAcro := entity.NewDict()
	alloc := newNativeObjectAllocator(100)
	createdRef, createdAcro, err := doc.resolveWritableAcroForm(rootWithoutAcro, alloc, map[entity.Ref]entity.Object{})
	require.NoError(t, err)
	assert.Equal(t, uint32(100), createdRef.Num())
	assert.NotNil(t, createdAcro.Get(entity.Name("Fields")))

	rootWithRef := entity.NewDict()
	rootWithRef.Set(entity.Name("AcroForm"), acroRef)
	existing := entity.NewDict()
	ref, resolvedAcro, err := doc.resolveWritableAcroForm(rootWithRef, alloc, map[entity.Ref]entity.Object{
		acroRef: existing,
	})
	require.NoError(t, err)
	assert.Equal(t, acroRef, ref)
	assert.Same(t, existing, resolvedAcro)

	_, _, err = doc.resolveWritableAcroForm(rootWithRef, alloc, map[entity.Ref]entity.Object{
		acroRef: entity.NewArray(),
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "acroform update is not dictionary")

	xref.objects[acroRef] = entity.NewArray()
	_, _, err = doc.resolveWritableAcroForm(rootWithRef, alloc, map[entity.Ref]entity.Object{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "acroform is not dictionary")
	xref.objects[acroRef] = acroDict

	inlineRoot := entity.NewDict()
	inlineRoot.Set(entity.Name("AcroForm"), acroDict)
	inlineRef, inlineAcro, err := doc.resolveWritableAcroForm(inlineRoot, alloc, map[entity.Ref]entity.Object{})
	require.NoError(t, err)
	assert.NotZero(t, inlineRef.Num())
	assert.NotNil(t, inlineAcro)

	_, err = extractTrailerRootRef(nil)
	require.Error(t, err)

	rootTrailer := entity.NewDict()
	rootTrailer.Set(entity.Name("/Root"), entity.NewRef(55, 0))
	rootRef, err := extractTrailerRootRef(rootTrailer)
	require.NoError(t, err)
	assert.Equal(t, uint32(55), rootRef.Num())

	rootTrailer = entity.NewDict()
	rootTrailer.Set(entity.Name("Root"), entity.NewRef(56, 0))
	rootRef, err = extractTrailerRootRef(rootTrailer)
	require.NoError(t, err)
	assert.Equal(t, uint32(56), rootRef.Num())

	badTrailer := entity.NewDict()
	badTrailer.Set(entity.Name("Root"), entity.NewString("not-ref"))
	_, err = extractTrailerRootRef(badTrailer)
	require.Error(t, err)
}

func TestNativeFormSave_FormAndEncodingHelpers(t *testing.T) {
	btnObj := formValuesToNativeObject("Btn", []string{""})
	btnName, ok := btnObj.(entity.Name)
	require.True(t, ok)
	assert.Equal(t, "Off", btnName.Value())

	textObj := formValuesToNativeObject("Tx", []string{"A", "B"})
	textArray, ok := textObj.(*entity.Array)
	require.True(t, ok)
	assert.Equal(t, 2, textArray.Len())

	emptyObj := formValuesToNativeObject("Tx", nil)
	emptyString, ok := emptyObj.(*entity.String)
	require.True(t, ok)
	assert.Equal(t, "", emptyString.Value())

	assert.True(t, isChoiceFieldType("Ch"))
	assert.True(t, isChoiceFieldType("/Ch"))
	assert.False(t, isChoiceFieldType("Tx"))

	assert.Equal(t, "/A#20#23#2F", encodePDFName("A #/"))
	assert.Equal(t, "(a\\(b\\)\\\\\\r\\n\\t\\b\\f)", encodePDFLiteralString("a(b)\\\r\n\t\b\f"))
}

func TestNativeFormSave_ResolveNativeSaveContext(t *testing.T) {
	noPersistenceDoc := newDocument(entity.NewDocument(&fetchOnlyXRef{}))
	_, err := noPersistenceDoc.resolveNativeSaveContext()
	require.Error(t, err)
	assert.ErrorContains(t, err, "does not support persistence")

	xref := &nativeFormSaveTestXRef{
		objects:   map[entity.Ref]entity.Object{},
		raw:       []byte("%PDF-1.7\n"),
		startXRef: 123,
		trailer:   entity.NewDict(),
	}
	xref.trailer.Set(entity.Name("Root"), entity.NewRef(77, 0))

	entityDoc := entity.NewDocument(xref)
	entityDoc.SetCatalog(entity.NewDict())
	doc := newDocument(entityDoc)

	ctx, err := doc.resolveNativeSaveContext()
	require.NoError(t, err)
	assert.Equal(t, uint64(123), ctx.prevXRef)
	assert.Equal(t, uint32(77), ctx.rootRef.Num())
	assert.Equal(t, len(xref.raw), len(ctx.raw))

	xref.raw = nil
	_, err = doc.resolveNativeSaveContext()
	require.Error(t, err)
	assert.ErrorContains(t, err, "empty PDF stream")
}
