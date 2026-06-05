package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type extGStateAlphaCanvas struct {
	*testCanvas
	fillAlpha   float64
	strokeAlpha float64
}

func (c *extGStateAlphaCanvas) SetFillAlpha(alpha float64) {
	c.fillAlpha = alpha
}

func (c *extGStateAlphaCanvas) SetStrokeAlpha(alpha float64) {
	c.strokeAlpha = alpha
}

type extGStateSoftMaskCanvas struct {
	*extGStateAlphaCanvas
	clearCalls   int
	beginCalls   int
	endCalls     int
	installCalls int
	lastBBox     [4]float64
	lastIsolated bool
	lastKnockout bool
	lastAlpha    bool
	lastOptions  SoftMaskOptions
}

func (c *extGStateSoftMaskCanvas) ClearSoftMask() {
	c.clearCalls++
}

func (c *extGStateSoftMaskCanvas) BeginSoftMaskGroup(bbox [4]float64, isolated, knockout bool) error {
	c.beginCalls++
	c.lastBBox = bbox
	c.lastIsolated = isolated
	c.lastKnockout = knockout
	return nil
}

func (c *extGStateSoftMaskCanvas) EndSoftMaskGroup(alpha bool) error {
	c.endCalls++
	c.lastAlpha = alpha
	return nil
}

func (c *extGStateSoftMaskCanvas) EndSoftMaskGroupWithOptions(options SoftMaskOptions) error {
	c.endCalls++
	c.lastAlpha = options.Alpha
	c.lastOptions = options
	return nil
}

func (c *extGStateSoftMaskCanvas) InstallPendingSoftMask() error {
	c.installCalls++
	return nil
}

type formTransparencyGroupCanvas struct {
	*extGStateAlphaCanvas
	clearCalls   int
	beginCalls   int
	deviceCalls  int
	paintCalls   int
	discardCalls int
	lastBBox     [4]float64
	lastIsolated bool
	lastKnockout bool
	softMask     bool
}

func (c *formTransparencyGroupCanvas) ClearSoftMask() {
	c.clearCalls++
	c.softMask = false
}

func (c *formTransparencyGroupCanvas) HasSoftMask() bool {
	return c.softMask
}

func (c *formTransparencyGroupCanvas) BeginTransparencyGroup(bbox [4]float64, isolated, knockout bool) error {
	c.beginCalls++
	c.lastBBox = bbox
	c.lastIsolated = isolated
	c.lastKnockout = knockout
	return nil
}

func (c *formTransparencyGroupCanvas) BeginTransparencyGroupDeviceBBox(bbox [4]float64, isolated, knockout bool) error {
	c.deviceCalls++
	return c.BeginTransparencyGroup(bbox, isolated, knockout)
}

func (c *formTransparencyGroupCanvas) PaintTransparencyGroup() error {
	c.paintCalls++
	return nil
}

func (c *formTransparencyGroupCanvas) DiscardTransparencyGroup() error {
	c.discardCalls++
	return nil
}

func TestApplyGraphicsStateParametersSyncsCanvasAlphaImmediately(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()}
	e.SetCanvas(canvas)

	gsDict := entity.NewDict()
	gsDict.Set(entity.Name("CA"), entity.NewReal(0.8))
	gsDict.Set(entity.Name("ca"), entity.NewReal(0.35))

	resources := entity.NewDict()
	gsCategory := entity.NewDict()
	gsCategory.Set(entity.Name("GAlpha"), gsDict)
	resources.Set(entity.Name("ExtGState"), gsCategory)
	e.SetResources(resources)

	require.NoError(t, e.applyGraphicsStateParameters(Operator{
		Opcode:   "gs",
		Operands: []entity.Object{entity.Name("GAlpha")},
	}))

	assert.InDelta(t, 0.8, e.graphics.strokeAlpha, 1e-9)
	assert.InDelta(t, 0.35, e.graphics.fillAlpha, 1e-9)
	assert.InDelta(t, 0.8, canvas.strokeAlpha, 1e-9)
	assert.InDelta(t, 0.35, canvas.fillAlpha, 1e-9)
}

func TestApplyGraphicsStateParametersTracksBlendModeAndAIS(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()}
	e.SetCanvas(canvas)

	gsDict := entity.NewDict()
	gsDict.Set(entity.Name("BM"), entity.Name("Multiply"))
	gsDict.Set(entity.Name("AIS"), entity.NewBoolean(true))

	resources := entity.NewDict()
	gsCategory := entity.NewDict()
	gsCategory.Set(entity.Name("GBlend"), gsDict)
	resources.Set(entity.Name("ExtGState"), gsCategory)
	e.SetResources(resources)

	require.NoError(t, e.applyGraphicsStateParameters(Operator{
		Opcode:   "gs",
		Operands: []entity.Object{entity.Name("GBlend")},
	}))

	assert.Equal(t, "Multiply", e.graphics.blendMode)
	assert.True(t, e.graphics.alphaIsShape)
	assert.Equal(t, "Multiply", canvas.blendMode)
}

func TestEnableFormTransparencyGroupDefaultsOnWithOptOut(t *testing.T) {
	t.Setenv("GO_PDF_ENABLE_FORM_TRANSPARENCY_GROUP", "")
	assert.True(t, enableFormTransparencyGroup())

	t.Setenv("GO_PDF_ENABLE_FORM_TRANSPARENCY_GROUP", "0")
	assert.False(t, enableFormTransparencyGroup())

	t.Setenv("GO_PDF_ENABLE_FORM_TRANSPARENCY_GROUP", "false")
	assert.False(t, enableFormTransparencyGroup())
}

func TestApplyGraphicsStateParametersClearsSoftMaskNone(t *testing.T) {
	t.Setenv("GO_PDF_ENABLE_EXTGSTATE_SMASK", "1")

	e := NewEvaluator(nil)
	canvas := &extGStateSoftMaskCanvas{
		extGStateAlphaCanvas: &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()},
	}
	e.SetCanvas(canvas)

	gsDict := entity.NewDict()
	gsDict.Set(entity.Name("SMask"), entity.Name("None"))
	resources := entity.NewDict()
	gsCategory := entity.NewDict()
	gsCategory.Set(entity.Name("GNone"), gsDict)
	resources.Set(entity.Name("ExtGState"), gsCategory)
	e.SetResources(resources)

	require.NoError(t, e.applyGraphicsStateParameters(Operator{
		Opcode:   "gs",
		Operands: []entity.Object{entity.Name("GNone")},
	}))

	assert.Equal(t, 1, canvas.clearCalls)
	assert.Zero(t, canvas.beginCalls)
	assert.Zero(t, canvas.installCalls)
}

func TestApplyGraphicsStateParametersRendersSoftMaskForm(t *testing.T) {
	t.Setenv("GO_PDF_ENABLE_EXTGSTATE_SMASK", "1")

	e := NewEvaluator(nil)
	canvas := &extGStateSoftMaskCanvas{
		extGStateAlphaCanvas: &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()},
	}
	e.SetCanvas(canvas)

	groupDict := entity.NewDict()
	groupDict.Set(entity.Name("S"), entity.Name("Transparency"))
	groupDict.Set(entity.Name("I"), entity.NewBoolean(true))
	groupDict.Set(entity.Name("K"), entity.NewBoolean(false))

	formDict := entity.NewDict()
	formDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0), entity.NewInteger(10), entity.NewInteger(20),
	))
	formDict.Set(entity.Name("Group"), groupDict)
	formDict.Set(entity.Name("Resources"), entity.NewDict())
	form := entity.NewStream(formDict, []byte("0 0 m 10 20 l S"))

	maskDict := entity.NewDict()
	maskDict.Set(entity.Name("S"), entity.Name("Alpha"))
	maskDict.Set(entity.Name("G"), form)
	gsDict := entity.NewDict()
	gsDict.Set(entity.Name("SMask"), maskDict)

	resources := entity.NewDict()
	gsCategory := entity.NewDict()
	gsCategory.Set(entity.Name("GMask"), gsDict)
	resources.Set(entity.Name("ExtGState"), gsCategory)
	e.SetResources(resources)

	require.NoError(t, e.applyGraphicsStateParameters(Operator{
		Opcode:   "gs",
		Operands: []entity.Object{entity.Name("GMask")},
	}))

	assert.Equal(t, 1, canvas.saveCalls)
	assert.Equal(t, 1, canvas.restoreCalls)
	assert.Equal(t, 1, canvas.clearCalls)
	assert.Equal(t, 1, canvas.beginCalls)
	assert.Equal(t, 1, canvas.endCalls)
	assert.Equal(t, 1, canvas.installCalls)
	assert.Equal(t, [4]float64{0, 0, 10, 20}, canvas.lastBBox)
	assert.True(t, canvas.lastIsolated)
	assert.False(t, canvas.lastKnockout)
	assert.True(t, canvas.lastAlpha)
	assert.Equal(t, "Normal", canvas.blendMode)
	assert.InDelta(t, 1.0, canvas.fillAlpha, 1e-9)
	assert.InDelta(t, 1.0, canvas.strokeAlpha, 1e-9)
}

func TestApplyGraphicsStateParametersPassesSoftMaskBackdropAndTransfer(t *testing.T) {
	t.Setenv("GO_PDF_ENABLE_EXTGSTATE_SMASK", "1")

	e := NewEvaluator(nil)
	canvas := &extGStateSoftMaskCanvas{
		extGStateAlphaCanvas: &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()},
	}
	e.SetCanvas(canvas)

	groupDict := entity.NewDict()
	groupDict.Set(entity.Name("S"), entity.Name("Transparency"))
	groupDict.Set(entity.Name("CS"), entity.Name("DeviceRGB"))

	formDict := entity.NewDict()
	formDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0), entity.NewInteger(10), entity.NewInteger(20),
	))
	formDict.Set(entity.Name("Group"), groupDict)
	formDict.Set(entity.Name("Resources"), entity.NewDict())
	form := entity.NewStream(formDict, []byte("0 0 m 10 20 l S"))

	transfer := entity.NewDict()
	transfer.Set(entity.Name("FunctionType"), entity.NewInteger(2))
	transfer.Set(entity.Name("Domain"), entity.NewArray(entity.NewInteger(0), entity.NewInteger(1)))
	transfer.Set(entity.Name("C0"), entity.NewArray(entity.NewInteger(0)))
	transfer.Set(entity.Name("C1"), entity.NewArray(entity.NewReal(0.5)))
	transfer.Set(entity.Name("N"), entity.NewInteger(1))

	maskDict := entity.NewDict()
	maskDict.Set(entity.Name("S"), entity.Name("Luminosity"))
	maskDict.Set(entity.Name("G"), form)
	maskDict.Set(entity.Name("BC"), entity.NewArray(
		entity.NewInteger(1), entity.NewInteger(0), entity.NewInteger(0),
	))
	maskDict.Set(entity.Name("TR"), transfer)

	gsDict := entity.NewDict()
	gsDict.Set(entity.Name("SMask"), maskDict)

	resources := entity.NewDict()
	gsCategory := entity.NewDict()
	gsCategory.Set(entity.Name("GMask"), gsDict)
	resources.Set(entity.Name("ExtGState"), gsCategory)
	e.SetResources(resources)

	require.NoError(t, e.applyGraphicsStateParameters(Operator{
		Opcode:   "gs",
		Operands: []entity.Object{entity.Name("GMask")},
	}))

	assert.False(t, canvas.lastOptions.Alpha)
	assert.True(t, canvas.lastOptions.HasBackdrop)
	assert.Equal(t, [3]uint8{255, 0, 0}, canvas.lastOptions.BackdropRGB)
	assert.Equal(t, uint8(77), canvas.lastOptions.BackdropLum)
	assert.True(t, canvas.lastOptions.TransferActive)
	assert.Equal(t, uint8(0), canvas.lastOptions.Transfer[0])
	assert.Equal(t, uint8(128), canvas.lastOptions.Transfer[255])
}

func TestFormTransparencyGroupUsesPopplerActivationGate(t *testing.T) {
	t.Setenv("GO_PDF_ENABLE_FORM_TRANSPARENCY_GROUP", "1")

	makeForm := func(group *entity.Dict, resources *entity.Dict) *entity.Stream {
		formDict := entity.NewDict()
		formDict.Set(entity.Name("BBox"), entity.NewArray(
			entity.NewInteger(0), entity.NewInteger(0), entity.NewInteger(10), entity.NewInteger(20),
		))
		formDict.Set(entity.Name("Group"), group)
		if resources != nil {
			formDict.Set(entity.Name("Resources"), resources)
		}
		return entity.NewStream(formDict, []byte("0 0 m 10 20 l S"))
	}
	group := func(isolated bool) *entity.Dict {
		groupDict := entity.NewDict()
		groupDict.Set(entity.Name("S"), entity.Name("Transparency"))
		groupDict.Set(entity.Name("I"), entity.NewBoolean(isolated))
		return groupDict
	}

	t.Run("plain non-isolated group is skipped", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := &formTransparencyGroupCanvas{
			extGStateAlphaCanvas: &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()},
		}
		e.SetCanvas(canvas)

		require.NoError(t, e.evaluateFormXObject(makeForm(group(false), entity.NewDict()), entity.Name("Fm")))

		assert.Zero(t, canvas.beginCalls)
		assert.Zero(t, canvas.paintCalls)
		assert.Zero(t, canvas.clearCalls)
	})

	t.Run("isolated group starts", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := &formTransparencyGroupCanvas{
			extGStateAlphaCanvas: &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()},
		}
		e.SetCanvas(canvas)

		require.NoError(t, e.evaluateFormXObject(makeForm(group(true), entity.NewDict()), entity.Name("Fm")))

		assert.Equal(t, 1, canvas.beginCalls)
		assert.Equal(t, 1, canvas.paintCalls)
		assert.Equal(t, [4]float64{0, 0, 10, 20}, canvas.lastBBox)
		assert.True(t, canvas.lastIsolated)
	})

	t.Run("resource transparency starts non-isolated group", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := &formTransparencyGroupCanvas{
			extGStateAlphaCanvas: &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()},
		}
		e.SetCanvas(canvas)

		gsDict := entity.NewDict()
		gsDict.Set(entity.Name("ca"), entity.NewReal(0.5))
		gsCategory := entity.NewDict()
		gsCategory.Set(entity.Name("GAlpha"), gsDict)
		resources := entity.NewDict()
		resources.Set(entity.Name("ExtGState"), gsCategory)

		require.NoError(t, e.evaluateFormXObject(makeForm(group(false), resources), entity.Name("Fm")))

		assert.Equal(t, 1, canvas.beginCalls)
		assert.Equal(t, 1, canvas.paintCalls)
		assert.False(t, canvas.lastIsolated)
	})

	t.Run("current transparency starts non-isolated group", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := &formTransparencyGroupCanvas{
			extGStateAlphaCanvas: &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()},
		}
		e.SetCanvas(canvas)
		e.graphics.fillAlpha = 0.5

		require.NoError(t, e.evaluateFormXObject(makeForm(group(false), entity.NewDict()), entity.Name("Fm")))

		assert.Equal(t, 1, canvas.beginCalls)
		assert.Equal(t, 1, canvas.paintCalls)
		assert.Equal(t, "Normal", canvas.blendMode)
		assert.InDelta(t, 1.0, canvas.fillAlpha, 1e-9)
	})

	t.Run("splash path receives transformed device bbox", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := &formTransparencyGroupCanvas{
			extGStateAlphaCanvas: &extGStateAlphaCanvas{testCanvas: newRecordingCanvas()},
		}
		e.SetCanvas(canvas)
		e.graphics.transform = [6]float64{2, 0, 0, 3, 5, 7}

		require.NoError(t, e.evaluateFormXObject(makeForm(group(true), entity.NewDict()), entity.Name("Fm")))

		assert.Equal(t, 1, canvas.beginCalls)
		assert.Equal(t, 1, canvas.deviceCalls)
		assert.Equal(t, [4]float64{5, 7, 25, 67}, canvas.lastBBox)
	})
}
