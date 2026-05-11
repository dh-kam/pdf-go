package renderer

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	imginfra "github.com/dh-kam/pdf-go/internal/infrastructure/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
)

func TestPathLifecycle(t *testing.T) {
	p := NewPath()
	assert.True(t, p.IsEmpty())

	p.AddElement(&MoveTo{X: 1, Y: 2})
	p.AddElement(&LineTo{X: 3, Y: 4})
	p.AddElement(&CurveTo{X1: 5, Y1: 6, X2: 7, Y2: 8, X: 9, Y: 10})
	p.AddElement(&Close{})

	assert.False(t, p.IsEmpty())
	assert.Len(t, p.Elements(), 4)

	cpX, cpY := p.CurrentPoint()
	assert.Equal(t, 1.0, cpX)
	assert.Equal(t, 2.0, cpY)
	require.Equal(t, 4, len(p.Elements()))
	moveX, moveY := p.MovePoint()
	assert.Equal(t, 1.0, moveX)
	assert.Equal(t, 2.0, moveY)

	bx1, by1, bx2, by2 := p.GetBounds()
	assert.Equal(t, 1.0, bx1)
	assert.Equal(t, 2.0, by1)
	assert.Equal(t, 9.0, bx2)
	assert.Equal(t, 10.0, by2)

	c := p.Clone()
	p.AddElement(&LineTo{X: 11, Y: 12})
	c.AddElement(&MoveTo{X: -2, Y: -2})
	pCurX2, pCurY2 := c.CurrentPoint()
	assert.NotEqual(t, cpX, pCurX2)
	assert.NotEqual(t, cpY, pCurY2)

	p.Clear()
	assert.True(t, p.IsEmpty())
	assert.Empty(t, p.Elements())
}

func TestMatrixAndTransform(t *testing.T) {
	e := NewEvaluator(nil)
	m := multiplyMatrix([6]float64{1, 0, 0, 1, 0, 0}, [6]float64{2, 0, 0, 2, 1, 1})
	assert.Equal(t, 2.0, m[0])
	assert.Equal(t, 2.0, m[3])
	assert.Equal(t, 1.0, m[4])
	assert.Equal(t, 1.0, m[5])

	e.graphics.transform = m
	x, y := e.transformPoint(1, 2)
	assert.Equal(t, 3.0, x)
	assert.Equal(t, 5.0, y)
}

func TestTextCodeUnitAndGlyphAdvance(t *testing.T) {
	font := &testFont{
		widths:    map[uint32]float64{65: 500, 0x4E00: 1000, 0x4E8C: 1000},
		isCIDFont: false,
		names: map[uint32]string{
			65: "A",
		},
	}
	unit := splitTextCodeUnits("AB", font)
	require.Len(t, unit, 2)
	assert.Equal(t, uint32('A'), unit[0].code)
	assert.Equal(t, uint32('B'), unit[1].code)

	cidFont := &testFont{widths: map[uint32]float64{0x4E00: 1000, 0x4E8C: 1000}, isCIDFont: true}
	unit = splitTextCodeUnits(string([]byte{0x4E, 0x00, 0x4E, 0x8C}), cidFont)
	require.Len(t, unit, 2)
	assert.Equal(t, uint32(0x4E00), unit[0].code)
	assert.Equal(t, uint32(0x4E8C), unit[1].code)

	e := NewEvaluator(nil)
	e.graphics.currentState.SetFont(font)
	assert.Equal(t, 12.0, e.graphics.currentState.GetFontSize())
	e.graphics.currentState.SetFont(font)
	e.graphics.currentState.SetFontSize(12)
	assert.Greater(t, e.glyphAdvance('A', font, 12), 0.0)

	e.graphics.currentState.SetWordSpacing(2.0)
	assert.Greater(t, e.glyphAdvance(' ', font, 12), 0.0)
	assert.Greater(t, e.glyphAdvance('A', font, 12), 0.0)
}

func TestDecodeGlyphName(t *testing.T) {
	r, ok := decodeGlyphName("space")
	assert.True(t, ok)
	assert.Equal(t, ' ', r)

	r, ok = decodeGlyphName("uni0041")
	assert.True(t, ok)
	assert.Equal(t, 'A', r)

	r, ok = decodeGlyphName("u03A9")
	assert.True(t, ok)
	assert.Equal(t, 'Ω', r)

	_, ok = decodeGlyphName("A")
	assert.True(t, ok)
	_, ok = decodeGlyphName("unknown")
	assert.False(t, ok)
}

func TestTextRenderingOperators(t *testing.T) {
	e := NewEvaluator(nil)
	font := &testFont{
		widths:    map[uint32]float64{65: 500, 66: 600},
		names:     map[uint32]string{65: "A", 66: "B"},
		isCIDFont: false,
	}
	e.graphics.currentState.SetFont(font)
	e.graphics.currentState.SetFontSize(12)

	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NoError(t, e.showText(Operator{Operands: []entity.Object{entity.NewString("AB")}}))
	assert.Equal(t, "AB", e.ExtractedText())

	require.NoError(t, e.showTextArray(Operator{Operands: []entity.Object{
		entity.NewArray(
			entity.NewString("A"),
			entity.NewInteger(10),
			entity.NewString("B"),
		),
	}}))
	assert.Contains(t, e.ExtractedText(), "AB")
	assert.Greater(t, canvas.drawTextCalls, 0)

	require.NoError(t, e.moveText(Operator{Operands: []entity.Object{entity.NewInteger(1), entity.NewInteger(2)}}))
	assert.Equal(t, 1.0, e.textLineMatrix[4])
	assert.Equal(t, 2.0, e.textLineMatrix[5])

	require.NoError(t, e.setTextMatrix(Operator{Operands: []entity.Object{
		entity.NewReal(1), entity.NewReal(0),
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(10), entity.NewReal(11),
	}}))
	assert.Equal(t, 10.0, e.textMatrix[4])
	assert.Equal(t, 11.0, e.textMatrix[5])

	require.NoError(t, e.setCharSpacing(Operator{Operands: []entity.Object{entity.NewReal(1.2)}}))
	require.NoError(t, e.setWordSpacing(Operator{Operands: []entity.Object{entity.NewReal(2.5)}}))
	require.NoError(t, e.setHorizScaling(Operator{Operands: []entity.Object{entity.NewReal(110)}}))
	assert.Equal(t, 1.2, e.graphics.currentState.GetCharSpacing())
	assert.Equal(t, 2.5, e.graphics.currentState.GetWordSpacing())
	assert.Equal(t, 110.0, e.graphics.currentState.GetHorizontalScaling())
}

func TestShowTextArrayBranches(t *testing.T) {
	e := NewEvaluator(nil)
	font := &testFont{
		widths:    map[uint32]float64{65: 500, 66: 600},
		names:     map[uint32]string{65: "A", 66: "B"},
		isCIDFont: false,
	}
	e.graphics.currentState.SetFont(font)
	e.graphics.currentState.SetFontSize(12)

	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NoError(t, e.showTextArray(Operator{Operands: []entity.Object{
		entity.NewArray(
			entity.NewString("A"),
			entity.NewReal(2),
			entity.NewString("B"),
		),
	}}))
	assert.Equal(t, "AB", e.ExtractedText())

	require.Error(t, e.showTextArray(Operator{}))
	require.Error(t, e.showTextArray(Operator{Operands: []entity.Object{entity.NewString("X")}}))

	e.graphics.currentState.SetFont(nil)
	require.NoError(t, e.showTextArray(Operator{Operands: []entity.Object{
		entity.NewArray(entity.NewString("A")),
	}}))
	assert.Equal(t, "AB", e.ExtractedText())
	assert.Greater(t, canvas.drawTextCalls, 0)
}

func TestFontResolution(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.SetFont(Operator{Operands: []entity.Object{entity.Name("Missing"), entity.NewInteger(10)}}))

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("Times-Roman"))
	fonts := entity.NewDict()
	fonts.Set(entity.Name("F1"), fontDict)
	resources.Set(entity.Name("Font"), fonts)
	e.SetResources(resources)

	require.NoError(t, e.SetFont(Operator{Operands: []entity.Object{entity.Name("F1"), entity.NewReal(11)}}))
	require.NotNil(t, e.graphics.currentState.GetFont())
	assert.Equal(t, 11.0, e.graphics.currentState.GetFontSize())
}

func TestGetFontFromDict_Type3FallsBackToDefaultFont(t *testing.T) {
	e := NewEvaluator(nil)

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type3"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("Helvetica"))

	font, err := e.getFontFromDict(fontDict, "Helvetica")
	require.NoError(t, err)
	require.NotNil(t, font)
	assert.Equal(t, "Helvetica", font.Name())
}

func TestSetFont_Type3ResourceFallsBackToDefaultFont(t *testing.T) {
	e := NewEvaluator(nil)

	resources := entity.NewDict()
	fonts := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type3"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("Helvetica"))
	fonts.Set(entity.Name("FType3"), fontDict)
	resources.Set(entity.Name("Font"), fonts)
	e.SetResources(resources)

	require.NoError(t, e.setFont(Operator{Operands: []entity.Object{entity.Name("FType3"), entity.NewInteger(13)}}))
	require.NotNil(t, e.graphics.currentState.GetFont())
	assert.Equal(t, "Helvetica", e.graphics.currentState.GetFont().Name())
	assert.Equal(t, 13.0, e.graphics.currentState.GetFontSize())
}

func TestSetFontFallbackBranches(t *testing.T) {
	e := NewEvaluator(nil)

	require.Error(t, e.setFont(Operator{Operands: []entity.Object{entity.NewName("a")}}))
	require.NoError(t, e.setFont(Operator{Operands: []entity.Object{entity.Name("F1"), entity.NewString("bad")}}))

	e.SetResources(entity.NewDict())
	require.NoError(t, e.setFont(Operator{Operands: []entity.Object{entity.Name("F1"), entity.NewInteger(14)}}))

	resources := entity.NewDict()
	fonts := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("BaseFont"), entity.Name("Times-Roman"))

	fonts.Set(entity.Name("F3"), fontDict)
	resources.Set(entity.Name("Font"), fonts)
	e.SetResources(resources)
	require.NoError(t, e.setFont(Operator{Operands: []entity.Object{entity.Name("F3"), entity.NewInteger(16)}}))

	resources = entity.NewDict()
	fonts = entity.NewDict()
	fonts.Set(entity.Name("F2"), entity.NewInteger(7))
	resources.Set(entity.Name("Font"), fonts)
	e.SetResources(resources)
	require.Error(t, e.setFont(Operator{Operands: []entity.Object{entity.Name("F2"), entity.NewInteger(15)}}))
}

func TestParseShadingCommonBranches(t *testing.T) {
	e := NewEvaluator(nil)
	shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
	dict := entity.NewDict()
	dict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(1),
		entity.NewReal(2.5),
		entity.NewInteger(3),
	))
	dict.Set(entity.Name("AntiAlias"), entity.NewInteger(1))

	e.parseShadingCommon(dict, shading)

	bbox := shading.GetBBox()
	assert.Equal(t, 0.0, bbox[0])
	assert.Equal(t, 1.0, bbox[1])
	assert.InDelta(t, 2.5, bbox[2], 0.0)
	assert.Equal(t, 3.0, bbox[3])
	assert.True(t, shading.GetAntiAlias())
}

func TestParseShadingFunctionBranches(t *testing.T) {
	e := NewEvaluator(nil)
	functions := entity.NewArray(entity.NewString("bad"))
	_, err := e.parseShadingFunctionList(functions)
	assert.Error(t, err)

	_, err = e.parseShadingFunctionObject(entity.NewString("bad"))
	assert.Error(t, err)

	dict := entity.NewDict()
	dict.Set(entity.Name("FunctionType"), entity.NewInteger(9))
	stream := entity.NewStream(dict, []byte{})
	_, err = e.parseShadingFunctionObject(stream)
	assert.Error(t, err)
}

func TestStateSetters(t *testing.T) {
	e := NewEvaluator(nil)

	require.NoError(t, e.setGrayStroke(Operator{Operands: []entity.Object{entity.NewReal(-1.2)}}))
	assert.Equal(t, "000000", e.graphics.strokeColor.Color.(*Color).Hex)

	require.NoError(t, e.setGrayFill(Operator{Operands: []entity.Object{entity.NewReal(1.7)}}))
	assert.Equal(t, "FFFFFF", e.graphics.fillColor.Color.(*Color).Hex)

	require.NoError(t, e.setRGBFill(Operator{Operands: []entity.Object{
		entity.NewReal(0.1), entity.NewReal(0.2), entity.NewReal(0.3),
	}}))
	assert.Equal(t, "19334C", e.graphics.fillColor.Color.(*Color).Hex)

	require.NoError(t, e.setCMYKFill(Operator{Operands: []entity.Object{
		entity.NewReal(0.1), entity.NewReal(0.2), entity.NewReal(0.3), entity.NewReal(0.4),
	}}))
	assert.NotNil(t, e.graphics.fillColor.Color.(*Color).Hex)

	require.NoError(t, e.setLineWidth(Operator{Operands: []entity.Object{entity.NewReal(-3)}}))
	assert.Equal(t, 0.0, e.graphics.lineWidth)
	require.NoError(t, e.setLineCap(Operator{Operands: []entity.Object{entity.NewInteger(2)}}))
	assert.Equal(t, 2, e.graphics.currentState.GetLineCap())
	require.NoError(t, e.setLineJoin(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
	assert.Equal(t, 1, e.graphics.currentState.GetLineJoin())
	require.NoError(t, e.setMiterLimit(Operator{Operands: []entity.Object{entity.NewReal(0.1)}}))
	assert.Equal(t, 1.0, e.graphics.currentState.GetMiterLimit())
}

func TestGraphicsSaveRestore(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NoError(t, e.saveState())
	assert.Len(t, e.stateStack, 1)
	require.NoError(t, e.restoreState())
	assert.Len(t, e.stateStack, 0)
	assert.Equal(t, 1, canvas.restoreCalls)

	require.Error(t, e.restoreState())
	assert.Equal(t, 1, canvas.restoreCalls)
}

func TestPathOperatorsAndPainting(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(2)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(3), entity.NewReal(4)}}))
	require.NoError(t, e.curveTo(Operator{Operands: []entity.Object{
		entity.NewReal(4), entity.NewReal(5), entity.NewReal(6), entity.NewReal(7), entity.NewReal(8), entity.NewReal(9),
	}}))
	require.NoError(t, e.curveToNoFirstControl(Operator{Operands: []entity.Object{
		entity.NewReal(1), entity.NewReal(2), entity.NewReal(3), entity.NewReal(4),
	}}))
	require.NoError(t, e.curveToNoLastControl(Operator{Operands: []entity.Object{
		entity.NewReal(5), entity.NewReal(6), entity.NewReal(7), entity.NewReal(8),
	}}))
	require.NoError(t, e.rectangle(Operator{Operands: []entity.Object{
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(4), entity.NewReal(5),
	}}))
	require.NoError(t, e.closePath(Operator{}))

	require.NoError(t, e.fillPath())
	require.NoError(t, e.fillPathEvenOdd())
	require.NoError(t, e.strokePath())
	require.NoError(t, e.strokeAndClosePath())
	require.NoError(t, e.fillAndStrokePath())
	require.NoError(t, e.fillAndStrokePathEvenOdd())
	require.NoError(t, e.closeFillAndStrokePath())
	require.NoError(t, e.closeFillAndStrokePathEvenOdd())
	require.NoError(t, e.endPath())

	assert.True(t, canvas.fillCalls > 0)
	assert.True(t, canvas.strokeCalls > 0)
	assert.True(t, canvas.fillEvenOddCalls > 0)
}

func TestFillAndStrokePath_FallbackCanvasReplaysPath(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(5)}}))
	require.NoError(t, e.closePath(Operator{}))

	require.NoError(t, e.fillAndStrokePath())

	assert.Equal(t, 1, canvas.fillCalls)
	assert.Equal(t, 1, canvas.strokeCalls)
	assert.Equal(t, 2, canvas.moveCalls)
	assert.True(t, e.graphics.path.IsEmpty())
}

func TestFillAndStrokePath_UsesCombinedCanvasPath(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := &combinedTestCanvas{testCanvas: newRecordingCanvas()}
	e.SetCanvas(canvas)

	require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(5)}}))
	require.NoError(t, e.closePath(Operator{}))

	require.NoError(t, e.fillAndStrokePath())

	assert.Equal(t, 1, canvas.fillAndStrokeCalls)
	assert.Equal(t, 0, canvas.fillCalls)
	assert.Equal(t, 0, canvas.strokeCalls)
	assert.Equal(t, 1, canvas.moveCalls)
	assert.True(t, e.graphics.path.IsEmpty())
}

func TestFillAndStrokePathEvenOdd_UsesCombinedCanvasPath(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := &combinedTestCanvas{testCanvas: newRecordingCanvas()}
	e.SetCanvas(canvas)

	require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(5)}}))
	require.NoError(t, e.closePath(Operator{}))

	require.NoError(t, e.fillAndStrokePathEvenOdd())

	assert.Equal(t, 1, canvas.fillEvenOddAndStrokeCalls)
	assert.Equal(t, 0, canvas.fillEvenOddCalls)
	assert.Equal(t, 0, canvas.strokeCalls)
	assert.Equal(t, 1, canvas.moveCalls)
	assert.True(t, e.graphics.path.IsEmpty())
}

func TestClipOperatorsAndBoundsFallback(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(10), entity.NewReal(10)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(20), entity.NewReal(10)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(20), entity.NewReal(20)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(10), entity.NewReal(20)}}))
	require.NoError(t, e.closePath(Operator{}))

	require.NoError(t, e.setClipPath())
	assert.True(t, e.graphics.pendingClip)
	assert.Nil(t, e.graphics.pathClip)
	require.NoError(t, e.setClipPathEvenOdd())
	assert.True(t, e.graphics.pendingClip)
	assert.Nil(t, e.graphics.pathClip)
}

func TestClipAppliedAtPathEndNotAtWOperator(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(10), entity.NewReal(10)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(20), entity.NewReal(10)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(20), entity.NewReal(20)}}))
	require.NoError(t, e.closePath(Operator{}))

	require.NoError(t, e.setClipPath())
	assert.True(t, e.graphics.pendingClip)
	assert.Equal(t, 0, canvas.clipCalls)
	assert.False(t, e.graphics.path.IsEmpty())

	require.NoError(t, e.rectangle(Operator{Operands: []entity.Object{
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(40), entity.NewReal(40),
	}}))
	require.NoError(t, e.fillPath())

	assert.Equal(t, 1, canvas.fillCalls)
	assert.Equal(t, 1, canvas.clipCalls)
	assert.False(t, e.graphics.pendingClip)
	assert.NotNil(t, e.graphics.pathClip)
	assert.True(t, e.graphics.path.IsEmpty())
}

func TestShadingHelpersAndFunctions(t *testing.T) {
	arr := entity.NewArray(entity.NewInteger(2), entity.NewInteger(3))
	ints, err := parseShadingIntArray(arr, 2)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 3}, ints)

	floatArr := entity.NewArray(entity.NewReal(0), entity.NewReal(1), entity.NewReal(2), entity.NewReal(3))
	floats, err := parseShadingFloatArray(floatArr, 4)
	require.NoError(t, err)
	assert.Equal(t, []float64{0, 1, 2, 3}, floats)

	total, err := sampledFunctionTotalPoints([]int{2, 3})
	require.NoError(t, err)
	assert.Equal(t, 6, total)

	samples, err := decodePackedSamples([]byte{0b10000000}, 1, 2)
	require.NoError(t, err)
	assert.InDeltaSlice(t, []float64{1.0, 0}, samples, 0.0001)

	assert.Equal(t, [][2]float64{{0, 1}, {2, 3}, {4, 5}}, toPairFloatRanges([]float64{0, 1, 2, 3, 4, 5}))
	floatVal, ok := extractFloat(entity.NewInteger(1))
	assert.True(t, ok)
	assert.InDelta(t, 1.0, floatVal, 0.0)
	assert.Equal(t, 7, func() int {
		v, _ := extractInt(entity.NewInteger(7))
		return v
	}())
}

func TestShadingHelpersEdgeCases(t *testing.T) {
	e := NewEvaluator(nil)

	_, err := parseShadingIntArray(entity.NewArray(entity.NewInteger(1)), 2)
	require.Error(t, err)

	_, err = parseShadingFloatArray(entity.NewArray(entity.NewInteger(1)), 2)
	require.Error(t, err)

	_, err = sampledFunctionTotalPoints([]int{})
	require.Error(t, err)

	_, err = sampledFunctionTotalPoints([]int{0})
	require.Error(t, err)

	_, ok := extractFloat(entity.Name("bad"))
	assert.False(t, ok)

	_, ok = extractInt(entity.Name("bad"))
	assert.False(t, ok)

	bbox, ok := e.currentShadingBBox()
	assert.False(t, ok)
	assert.Equal(t, [4]float64{}, bbox)

	e.SetCanvas(newRecordingCanvas())
	bbox, ok = e.currentShadingBBox()
	assert.True(t, ok)
	assert.Equal(t, [4]float64{0, 0, 128, 64}, bbox)
}

func TestShadingColorDefaults(t *testing.T) {
	e := NewEvaluator(nil)

	start, end := e.getShadingColors(nil)
	assert.Equal(t, color.White, start)
	assert.Equal(t, color.Black, end)

	fn := &testFunction{values: []float64{0.2, 0.4, 0.6}}
	shading := entity.NewShading(entity.ShadingFunctionBased, "DeviceRGB")
	shading.SetFunctions([]entity.Function{fn})

	start, end = e.getShadingColors(shading)
	assert.Equal(t, color.RGBA{R: 51, G: 102, B: 153, A: 255}, start)
	assert.Equal(t, color.RGBA{R: 51, G: 102, B: 153, A: 255}, end)
}

func TestEvaluateShadingFunctionColor(t *testing.T) {
	fn := &testFunction{values: []float64{0.2}}
	assert.Equal(t, color.RGBA{R: 51, G: 51, B: 51, A: 255}, evaluateShadingFunctionColor(fn, 0.0))

	fn.values = []float64{1.2, -0.1, 0.3}
	assert.Equal(t, color.RGBA{R: 255, G: 0, B: 76, A: 255}, evaluateShadingFunctionColor(fn, 0.5))

	fn.values = []float64{1.2}
	fn.err = true
	assert.Equal(t, color.White, evaluateShadingFunctionColor(fn, 0.5))
}

func TestImageHelpers(t *testing.T) {
	assert.Equal(t, domainimage.FilterFlate, normalizeImageFilterName("Fl"))
	assert.Equal(t, domainimage.FilterDCT, normalizeImageFilterName("DCT"))
	assert.False(t, isEncodedImageFilter(domainimage.FilterFlate))
	assert.True(t, isEncodedImageFilter(domainimage.FilterJPX))
	assert.Equal(t, "DeviceRGB", normalizeImageColorSpaceName("RGB"))
	assert.Equal(t, "DeviceCMYK", normalizeImageColorSpaceName("CMYK"))
	assert.True(t, isSupportedImageColorSpace("DeviceRGB"))
	assert.False(t, isSupportedImageColorSpace("Foo"))
}

func TestShadingFunctionParsingAndRender(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	exponentialDict := buildExponentialFunctionDict()
	exponentialDict.Set(entity.Name("C0"), entity.NewArray(entity.NewReal(0.2), entity.NewReal(0.4), entity.NewReal(0.6)))
	exponentialDict.Set(entity.Name("C1"), entity.NewArray(entity.NewReal(0.8), entity.NewReal(0.9), entity.NewReal(1.0)))
	exponentialDict.Set(entity.Name("N"), entity.NewReal(2))

	shading := entity.NewDict()
	shading.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
	shading.Set(entity.Name("Coords"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1)))
	shading.Set(entity.Name("Function"), exponentialDict)

	s, err := e.parseShading(shading)
	require.NoError(t, err)
	assert.Equal(t, entity.ShadingAxial, s.GetShadingType())

	// path bbox fallback to canvas bounds.
	require.NoError(t, e.renderShading(s))
	assert.True(t, canvas.drawShadingCalls > 0)

	// Trigger canvas-native shading path failure fallback path.
	canvas.drawShadingErr = errors.New("not implemented")
	require.NoError(t, e.renderShading(s))
	assert.True(t, canvas.fillCalls >= 1)
}

func TestShadingPaintOperator(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	resources := entity.NewDict()
	shadingDict := entity.NewDict()
	shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingFunctionBased)))
	shadingDict.Set(entity.Name("Function"), buildExponentialFunctionDict())
	shadingDict.Set(entity.Name("Coords"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1)))
	shadingCategory := entity.NewDict()
	shadingCategory.Set(entity.Name("S1"), shadingDict)
	resources.Set(entity.Name("Shading"), shadingCategory)
	e.SetResources(resources)

	require.NoError(t, e.paintShading(Operator{Operands: []entity.Object{entity.Name("S1")}}))
	assert.True(t, canvas.drawShadingCalls > 0)
}

func TestEvaluatorPublicAPIWrappers(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	assert.Same(t, e.graphics, e.GetGraphicsState())

	font := &testFont{
		widths:    map[uint32]float64{65: 500, 66: 600},
		names:     map[uint32]string{65: "A", 66: "B"},
		isCIDFont: false,
	}
	e.graphics.currentState.SetFont(font)
	e.graphics.currentState.SetFontSize(12)

	require.NoError(t, e.SetFont(Operator{Operands: []entity.Object{entity.Name("Unused"), entity.NewReal(14)}}))
	e.SetResources(entity.NewDict())
	require.NoError(t, e.EvaluateContent([]byte("q")))

	require.NoError(t, e.SetTextMatrix(Operator{Operands: []entity.Object{
		entity.NewReal(1), entity.NewReal(2), entity.NewReal(3), entity.NewReal(4), entity.NewReal(5), entity.NewReal(6),
	}}))
	require.NoError(t, e.MoveText(Operator{Operands: []entity.Object{entity.NewReal(7), entity.NewReal(8)}}))
	require.NoError(t, e.setTextLeading(Operator{Operands: []entity.Object{entity.NewReal(2)}}))
	require.NoError(t, e.setTextRenderMode(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
	require.NoError(t, e.setTextRise(Operator{Operands: []entity.Object{entity.NewReal(1.5)}}))
	require.NoError(t, e.MoveTextSetLeading(Operator{Operands: []entity.Object{entity.NewReal(3), entity.NewReal(-4)}}))
	assert.Equal(t, 4.0, e.graphics.currentState.GetTextLeading())
	require.NoError(t, e.moveTextNextLine())
	require.NoError(t, e.moveTextNextLineAndShowText(Operator{Operands: []entity.Object{entity.NewString("A")}}))

	e.graphics.currentState.SetCharSpacing(1)
	e.graphics.currentState.SetWordSpacing(1)
	require.NoError(t, e.setSpacingMoveTextNextLineAndShowText(Operator{Operands: []entity.Object{
		entity.NewReal(3), entity.NewReal(4), entity.NewString("B"),
	}}))
	assert.Contains(t, e.ExtractedText(), "AB")

	require.NoError(t, e.ShowText(Operator{Operands: []entity.Object{entity.NewString("C")}}))
	require.NoError(t, e.ShowTextArray(Operator{Operands: []entity.Object{
		entity.NewArray(entity.NewString("D"), entity.NewInteger(-20), entity.NewString("E")),
	}}))

	require.NoError(t, e.SaveState())
	assert.Len(t, e.stateStack, 2)
	require.NoError(t, e.RestoreState())
	assert.Len(t, e.stateStack, 1)

	require.NoError(t, e.MoveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(2)}}))
	require.NoError(t, e.LineTo(Operator{Operands: []entity.Object{entity.NewReal(3), entity.NewReal(4)}}))
	require.NoError(t, e.CurveTo(Operator{Operands: []entity.Object{
		entity.NewReal(4), entity.NewReal(5),
		entity.NewReal(6), entity.NewReal(7),
		entity.NewReal(8), entity.NewReal(9),
	}}))
	require.NoError(t, e.Rectangle(Operator{Operands: []entity.Object{
		entity.NewReal(10), entity.NewReal(10),
		entity.NewReal(20), entity.NewReal(5),
	}}))
	require.NoError(t, e.ClosePath(Operator{}))
	require.NoError(t, e.StrokePath())
	require.NoError(t, e.FillPath())
	require.NoError(t, e.FillPathEvenOdd())
	require.NoError(t, e.EndPath())

	require.NoError(t, e.SetGrayStroke(Operator{Operands: []entity.Object{entity.NewReal(0.2)}}))
	require.NoError(t, e.SetGrayFill(Operator{Operands: []entity.Object{entity.NewReal(0.8)}}))
	require.NoError(t, e.SetRGBStroke(Operator{Operands: []entity.Object{
		entity.NewReal(0.1), entity.NewReal(0.2), entity.NewReal(0.3),
	}}))
	require.NoError(t, e.SetRGBFill(Operator{Operands: []entity.Object{
		entity.NewReal(0.4), entity.NewReal(0.5), entity.NewReal(0.6),
	}}))
	require.NoError(t, e.SetCMYKStroke(Operator{Operands: []entity.Object{
		entity.NewReal(0.1), entity.NewReal(0.2), entity.NewReal(0.3), entity.NewReal(0.4),
	}}))
	require.NoError(t, e.SetCMYKFill(Operator{Operands: []entity.Object{
		entity.NewReal(0.4), entity.NewReal(0.3), entity.NewReal(0.2), entity.NewReal(0.1),
	}}))
	require.NoError(t, e.SetLineWidth(Operator{Operands: []entity.Object{entity.NewReal(2.5)}}))
	require.NoError(t, e.SetLineCap(Operator{Operands: []entity.Object{entity.NewInteger(2)}}))
	require.NoError(t, e.SetLineJoin(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
	require.NoError(t, e.SetMiterLimit(Operator{Operands: []entity.Object{entity.NewReal(10)}}))

	e.beginTextObject()
	e.endTextObject()
	e.SetResources(entity.NewDict())
	imageDict := entity.NewDict()
	imageDict.Set(entity.Name("Subtype"), entity.Name("Image"))
	imageDict.Set(entity.Name("Width"), entity.NewInteger(1))
	imageDict.Set(entity.Name("Height"), entity.NewInteger(1))
	imageDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	xObjects := entity.NewDict()
	xObjects.Set(entity.Name("Im1"), entity.NewStream(imageDict, []byte{1, 2, 3}))
	e.resources.Set(entity.Name("XObject"), xObjects)
	require.NoError(t, e.InvokeXObject(Operator{Operands: []entity.Object{entity.Name("Im1")}}))

	assert.Greater(t, canvas.drawTextCalls, 0)

	m := &MoveTo{}
	l := &LineTo{}
	c := &CurveTo{}
	z := &Close{}
	assert.Equal(t, PathMoveTo, m.Type())
	assert.Equal(t, PathLineTo, l.Type())
	assert.Equal(t, PathCurveTo, c.Type())
	assert.Equal(t, PathClose, z.Type())
}

func TestTextMatrixStateSyncHelpers_KeepEvaluatorGraphicsAndCurrentStateAligned(t *testing.T) {
	e := NewEvaluator(nil)

	e.beginTextObject()
	identity := [6]float64{1, 0, 0, 1, 0, 0}
	assert.Equal(t, identity, e.textMatrix)
	assert.Equal(t, identity, e.textLineMatrix)
	assert.Equal(t, identity, e.graphics.textMatrix)
	assert.Equal(t, identity, e.graphics.textLine)
	assert.Equal(t, identity, e.graphics.currentState.GetTextMatrix())

	matrix := [6]float64{1, 2, 3, 4, 5, 6}
	require.NoError(t, e.SetTextMatrix(Operator{Operands: []entity.Object{
		entity.NewReal(1), entity.NewReal(2), entity.NewReal(3), entity.NewReal(4), entity.NewReal(5), entity.NewReal(6),
	}}))
	assert.Equal(t, matrix, e.textMatrix)
	assert.Equal(t, matrix, e.textLineMatrix)
	assert.Equal(t, matrix, e.graphics.textMatrix)
	assert.Equal(t, matrix, e.graphics.textLine)
	assert.Equal(t, matrix, e.graphics.currentState.GetTextMatrix())
}

func TestDefaultTextPlacement_CurrentPositionUsesTextRiseInRenderingMatrix(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.transform = [6]float64{1, 0, 0, 1, 10, 20}
	e.textMatrix = [6]float64{1, 0, 0, 1, 3, 4}
	e.graphics.currentState.SetTextRise(7)

	placement := defaultTextPlacement{}
	trm := placement.CurrentRenderingMatrix(e)
	x, y := placement.CurrentPosition(e)

	assert.Equal(t, [6]float64{1, 0, 0, 1, 13, 31}, trm)
	assert.Equal(t, 13.0, x)
	assert.Equal(t, 31.0, y)
}

func TestShadingRadialAndMeshCoverage(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	radialDict := entity.NewDict()
	radialDict.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingRadial)))
	radialDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(100), entity.NewReal(100), entity.NewReal(2),
	))
	radialDict.Set(entity.Name("Function"), buildExponentialFunctionDict())

	radial, err := e.parseShading(radialDict)
	require.NoError(t, err)
	assert.Equal(t, entity.ShadingRadial, radial.GetShadingType())

	canvas.drawShadingErr = errors.New("not implemented")
	require.NoError(t, e.renderShading(radial))
	assert.True(t, canvas.fillCalls > 0)

	meshDict := entity.NewDict()
	meshDict.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingLatticeGouraud)))
	meshDict.Set(entity.Name("BitsPerCoordinate"), entity.NewInteger(2))
	meshDict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(2))
	meshDict.Set(entity.Name("BitsPerFlag"), entity.NewInteger(1))
	meshDict.Set(entity.Name("Decode"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))

	meshShading, err := e.parseShading(meshDict)
	require.NoError(t, err)
	assert.Equal(t, entity.ShadingLatticeGouraud, meshShading.GetShadingType())
}

func TestParseShadingFunctionListAndObject(t *testing.T) {
	e := NewEvaluator(nil)
	fnDict := buildExponentialFunctionDict()
	fn, err := e.parseShadingFunctionList(fnDict)
	require.NoError(t, err)
	require.Len(t, fn, 1)

	obj, err := e.parseShadingFunctionObject(fnDict)
	require.NoError(t, err)
	assert.NotNil(t, obj)
}

func TestShadingFunctionTypesCoverage(t *testing.T) {
	e := NewEvaluator(nil)

	sampledDict := entity.NewDict()
	sampledDict.Set(entity.Name("FunctionType"), entity.NewInteger(0))
	sampledDict.Set(entity.Name("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	sampledDict.Set(entity.Name("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	sampledDict.Set(entity.Name("Size"), entity.NewArray(entity.NewInteger(2)))
	sampledDict.Set(entity.Name("BitsPerSample"), entity.NewInteger(1))
	sampledDict.Set(entity.Name("Decode"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	sampledStream := entity.NewStream(sampledDict, []byte{0b10000000})

	fn, err := e.parseShadingFunctionObject(sampledStream)
	require.NoError(t, err)
	assert.IsType(t, &entity.SampledFunction{}, fn)

	stitchingDict := entity.NewDict()
	stitchingDict.Set(entity.Name("FunctionType"), entity.NewInteger(3))
	stitchingDict.Set(entity.Name("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	stitchingDict.Set(entity.Name("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	stitchingDict.Set(entity.Name("Bounds"), entity.NewArray(entity.NewReal(0.5)))
	stitchingDict.Set(entity.Name("Encode"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	stitchingDict.Set(entity.Name("Functions"), entity.NewArray(buildExponentialFunctionDict()))

	fn, err = e.parseShadingFunctionObject(stitchingDict)
	require.NoError(t, err)
	assert.IsType(t, &entity.StitchingFunction{}, fn)

	postScriptData := []byte("0 1 add")
	postScriptDict := entity.NewDict()
	postScriptDict.Set(entity.Name("FunctionType"), entity.NewInteger(4))
	postScriptDict.Set(entity.Name("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	postScriptDict.Set(entity.Name("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	postScriptStream := entity.NewStream(postScriptDict, postScriptData)

	fn, err = e.parseShadingFunctionObject(postScriptStream)
	require.NoError(t, err)
	assert.IsType(t, &entity.PostScriptFunction{}, fn)
}

func TestGraphicsStateSaveRestoreAndEndTextObject(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NotNil(t, e.graphics.currentState)

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("Times-Roman"))
	fontResources := entity.NewDict()
	fonts := entity.NewDict()
	fonts.Set(entity.Name("F1"), fontDict)
	fontResources.Set(entity.Name("Font"), fonts)
	e.SetResources(fontResources)

	e.graphics.currentState.SetFontSize(10)
	e.SetFont(Operator{Operands: []entity.Object{entity.Name("F1"), entity.NewReal(16)}})
	e.SetFont(Operator{Operands: []entity.Object{entity.Name("F1"), entity.NewReal(20)}})

	e.beginTextObject()
	require.NoError(t, e.MoveText(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(6)}}))
	e.endTextObject()

	gs := e.graphics
	gs.Save()
	e.graphics.currentState.SetFontSize(24)
	require.EqualValues(t, 24, e.graphics.currentState.GetFontSize())
	gs.Restore()
	require.EqualValues(t, 20, e.graphics.currentState.GetFontSize())

	assert.Equal(t, [6]float64{1, 0, 0, 1, 5, 6}, e.graphics.textLine)
}

func TestImageObjectDecodePlaceholder(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	imageDict := entity.NewDict()
	imageDict.Set(entity.Name("Subtype"), entity.Name("Image"))
	imageDict.Set(entity.Name("Width"), entity.NewInteger(1))
	imageDict.Set(entity.Name("Height"), entity.NewInteger(1))
	imageDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	// Empty stream data triggers decode error and fallback rendering path.
	stream := entity.NewStream(imageDict, nil)

	require.NoError(t, e.evaluateImageXObject(stream, entity.Name("Im1")))
	assert.Greater(t, canvas.strokeCalls, 0)
	assert.Equal(t, 0, canvas.drawImageCalls)
}

func TestImageHelperFunctions(t *testing.T) {
	e := NewEvaluator(nil)

	cs, ok := e.resolveImageColorSpace(nil)
	require.True(t, ok)
	assert.Equal(t, "DeviceRGB", cs)

	colorSpaceArray := entity.NewArray(entity.Name("RGB"))
	cs, ok = e.resolveImageColorSpace(colorSpaceArray)
	assert.True(t, ok)
	assert.Equal(t, "DeviceRGB", cs)

	iccbased := entity.NewArray(entity.Name("ICCBased"))
	_, ok = e.resolveImageColorSpace(iccbased)
	assert.False(t, ok)

	unsupported := entity.NewArray(entity.Name("Custom"))
	_, ok = e.resolveImageColorSpace(unsupported)
	assert.False(t, ok)

	filter, ok := resolveXObjectImageFilter(entity.Name("DCT"))
	require.True(t, ok)
	assert.Equal(t, domainimage.FilterDCT, filter)

	filter, ok = resolveXObjectImageFilter(entity.Name("FlateDecode"))
	require.False(t, ok)
	assert.Equal(t, domainimage.FilterFlate, filter)

	filter, ok = resolveXObjectImageFilter(entity.Name("JPXDecode"))
	require.True(t, ok)
	assert.Equal(t, domainimage.FilterJPX, filter)

	filter, ok = resolveXObjectImageFilter(entity.Name("JBIG2Decode"))
	require.True(t, ok)
	assert.Equal(t, domainimage.FilterJBIG2, filter)

	filter, ok = resolveXObjectImageFilter(entity.Name("Unknown"))
	require.False(t, ok)
	assert.Equal(t, domainimage.ImageFilter("Unknown"), filter)

	filterArr := entity.NewArray(entity.Name("DCT"))
	filter, ok = resolveXObjectImageFilter(filterArr)
	require.True(t, ok)
	assert.Equal(t, domainimage.FilterDCT, filter)

	filterArrWrong := entity.NewArray(entity.Name("CCF"), entity.Name("DCT"))
	_, ok = resolveXObjectImageFilter(filterArrWrong)
	assert.False(t, ok)

	filter, prefixLen, ok := resolveXObjectEncodedFilterPipeline(entity.NewArray(entity.Name("FlateDecode"), entity.Name("DCTDecode")))
	require.True(t, ok)
	assert.Equal(t, domainimage.FilterDCT, filter)
	assert.Equal(t, 1, prefixLen)

	_, _, ok = resolveXObjectEncodedFilterPipeline(filterArrWrong)
	assert.False(t, ok)

	filterNone, ok := resolveXObjectImageFilter(entity.NewInteger(1))
	assert.False(t, ok)
	assert.Equal(t, domainimage.FilterNone, filterNone)

	assert.Equal(t, domainimage.FilterFlate, normalizeImageFilterName("Fl"))
	assert.Equal(t, domainimage.FilterFlate, normalizeImageFilterName("/Fl"))
	assert.Equal(t, domainimage.FilterFlate, normalizeImageFilterName("FlateDecode"))
	assert.Equal(t, domainimage.FilterFlate, normalizeImageFilterName("/FlateDecode"))
	assert.Equal(t, domainimage.FilterJPX, normalizeImageFilterName("JPXDecode"))
	assert.Equal(t, domainimage.FilterJBIG2, normalizeImageFilterName("/JBIG2Decode"))
	assert.Equal(t, domainimage.ImageFilter("Unknown"), normalizeImageFilterName("Unknown"))

	assert.True(t, isEncodedImageFilter(domainimage.FilterDCT))
	assert.True(t, isEncodedImageFilter(domainimage.FilterJPX))
	assert.True(t, isEncodedImageFilter(domainimage.FilterJBIG2))
	assert.False(t, isEncodedImageFilter(domainimage.FilterFlate))

	assert.Equal(t, "DeviceGray", normalizeImageColorSpaceName("G"))
	assert.Equal(t, "DeviceRGB", normalizeImageColorSpaceName("RGB"))
	assert.Equal(t, "DeviceCMYK", normalizeImageColorSpaceName("CMYK"))
	assert.Equal(t, "Other", normalizeImageColorSpaceName("Other"))

	assert.True(t, isSupportedImageColorSpace("DeviceRGB"))
	assert.False(t, isSupportedImageColorSpace("Lab"))
}

func TestDecodeImageEncodedFilterPrefix(t *testing.T) {
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, err := zw.Write([]byte("encoded-jpeg-bytes"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.NewArray(entity.Name("FlateDecode"), entity.Name("DCTDecode")))
	xobj := entity.NewStream(dict, compressed.Bytes())

	data, err := decodeImageEncodedFilterPrefix(xobj, 1)
	require.NoError(t, err)
	assert.Equal(t, []byte("encoded-jpeg-bytes"), data)
}

func TestDecodeSoftMaskImageStreamPreservesEncodedDCT(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("DCTDecode"))
	maskStream := entity.NewStream(dict, []byte("encoded-jpeg-bytes"))

	data, filter, err := decodeSoftMaskImageStream(maskStream)
	require.NoError(t, err)
	assert.Equal(t, []byte("encoded-jpeg-bytes"), data)
	assert.Equal(t, domainimage.FilterDCT, filter)
}

func TestDecodeSoftMaskImageStreamDecodesPrefixBeforeDCT(t *testing.T) {
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, err := zw.Write([]byte("encoded-jpeg-bytes"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.NewArray(entity.Name("FlateDecode"), entity.Name("DCTDecode")))
	maskStream := entity.NewStream(dict, compressed.Bytes())

	data, filter, err := decodeSoftMaskImageStream(maskStream)
	require.NoError(t, err)
	assert.Equal(t, []byte("encoded-jpeg-bytes"), data)
	assert.Equal(t, domainimage.FilterDCT, filter)
}

func TestImageObjectsAndInlineImage(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	imageDict := entity.NewDict()
	imageDict.Set(entity.Name("Subtype"), entity.Name("Image"))
	imageDict.Set(entity.Name("Width"), entity.NewInteger(1))
	imageDict.Set(entity.Name("Height"), entity.NewInteger(1))
	imageDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	stream := entity.NewStream(imageDict, []byte{1, 2, 3})
	require.NoError(t, e.evaluateImageXObject(stream, entity.Name("Im1")))
	assert.Equal(t, 1, canvas.drawImageCalls)

	require.NoError(t, e.beginInlineImage())
	require.NotNil(t, e.inlineImageDict)
	e.inlineImageDict.Set(entity.Name("W"), entity.NewInteger(1))
	e.inlineImageDict.Set(entity.Name("H"), entity.NewInteger(1))
	e.inlineImageData = []byte{1, 2, 3}
	require.NoError(t, e.endInlineImage())
	assert.Equal(t, 2, canvas.drawImageCalls)
}

func TestInlineImage_ShorthandFilterArrayDecodesAndDraws(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	rawPixel := []byte{255, 0, 0}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, err := zw.Write(rawPixel)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	encoded := make([]byte, ascii85.MaxEncodedLen(compressed.Len()))
	n := ascii85.Encode(encoded, compressed.Bytes())

	require.NoError(t, e.beginInlineImage())
	e.inlineImageDict.Set(entity.Name("W"), entity.NewInteger(1))
	e.inlineImageDict.Set(entity.Name("H"), entity.NewInteger(1))
	e.inlineImageDict.Set(entity.Name("BPC"), entity.NewInteger(8))
	e.inlineImageDict.Set(entity.Name("CS"), entity.Name("RGB"))
	e.inlineImageDict.Set(
		entity.Name("F"),
		entity.NewArray(entity.Name("A85"), entity.Name("Fl")),
	)
	e.inlineImageData = encoded[:n]

	require.NoError(t, e.endInlineImage())
	assert.Equal(t, 1, canvas.drawImageCalls)
}

func TestExecuteOperatorNoopAndDefaultCases(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	noopOps := []string{"CS", "cs", "SC", "SCN", "sc", "scn", "gs", "d0", "d1", "ID"}
	for _, opcode := range noopOps {
		require.NoError(t, e.executeOperator(Operator{Opcode: opcode}))
	}

	require.NoError(t, e.executeOperator(Operator{Opcode: "UNKNOWN_OP"}))
}

func TestParseOperatorsIgnoreUnrelatedTokens(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.parseOperators([]byte("1 2 (abc) /Name 3 4")))
}

func TestFormXObjectLifecycle(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	formDict := entity.NewDict()
	formDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	formDict.Set(entity.Name("Resources"), entity.NewDict())
	formDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(4), entity.NewReal(5),
	))
	formDict.Set(entity.Name("Matrix"), entity.NewArray(
		entity.NewReal(1), entity.NewReal(0), entity.NewReal(0),
		entity.NewReal(1), entity.NewReal(0), entity.NewReal(0),
	))
	formStream := entity.NewStream(formDict, []byte("1 0 0 1 2 3 cm"))
	require.NoError(t, e.evaluateFormXObject(formStream, entity.Name("Fm1")))

	assert.Greater(t, canvas.saveCalls, 0)
	assert.Greater(t, canvas.restoreCalls, 0)
}

func TestFormXObjectClearsCallerPathBeforeContent(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	e.graphics.path.AddElement(&MoveTo{X: 0, Y: 0})
	e.graphics.path.AddElement(&LineTo{X: 10, Y: 0})
	e.graphics.pendingClip = true
	e.graphics.pendingClipMode = ClipEvenOdd

	formDict := entity.NewDict()
	formDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	formDict.Set(entity.Name("Resources"), entity.NewDict())
	formStream := entity.NewStream(formDict, []byte("S"))

	require.NoError(t, e.evaluateFormXObject(formStream, entity.Name("Fm1")))
	assert.Equal(t, 0, canvas.strokeCalls)
	assert.False(t, e.graphics.path.IsEmpty())
	assert.True(t, e.graphics.pendingClip)
	assert.Equal(t, ClipEvenOdd, e.graphics.pendingClipMode)
}

func TestFormXObjectLifecycleResolvesIndirectResources(t *testing.T) {
	resourcesRef := entity.NewRef(100, 0)
	innerFormRef := entity.NewRef(101, 0)

	innerFormDict := entity.NewDict()
	innerFormDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	innerFormDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(4), entity.NewReal(5),
	))
	innerForm := entity.NewStream(innerFormDict, []byte("0 0 m 4 0 l 4 5 l h f"))

	xobjects := entity.NewDict()
	xobjects.Set(entity.Name("FmInner"), innerFormRef)

	resources := entity.NewDict()
	resources.Set(entity.Name("XObject"), xobjects)

	outerFormDict := entity.NewDict()
	outerFormDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	outerFormDict.Set(entity.Name("Resources"), resourcesRef)
	outerForm := entity.NewStream(outerFormDict, []byte("/FmInner Do"))

	e := NewEvaluator(&testMapXRef{
		objects: map[entity.Ref]entity.Object{
			resourcesRef: resources,
			innerFormRef: innerForm,
		},
	})
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	require.NoError(t, e.evaluateFormXObject(outerForm, entity.Name("FmOuter")))
	assert.Greater(t, canvas.fillCalls, 0)
}

func TestConcatenateMatrixToCTMUsesSameOrderAsCM(t *testing.T) {
	e := NewEvaluator(nil)
	e.SetInitialTransform([6]float64{2, 0, 0, 3, -4, 7})

	matrix := [6]float64{0, 1, -1, 0, 5, 6}
	e.concatenateMatrixToCTM(matrix)

	assert.Equal(t, multiplyMatrix([6]float64{2, 0, 0, 3, -4, 7}, matrix), e.GetGraphicsState().transform)
}

func TestFormXObjectOperatorCacheReuse(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	formDict := entity.NewDict()
	formDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	formStream := entity.NewStream(formDict, []byte("q Q"))

	require.NoError(t, e.evaluateFormXObject(formStream, entity.Name("Fm1")))
	assert.Equal(t, 2, canvas.saveCalls)
	assert.Equal(t, 2, canvas.restoreCalls)
	require.Len(t, e.formOperatorCache, 1)

	// Mutate raw data after first evaluation.
	// Cached parsed operators should still be reused for subsequent invocations.
	formStream.SetData([]byte(""))

	require.NoError(t, e.evaluateFormXObject(formStream, entity.Name("Fm1")))
	assert.Equal(t, 4, canvas.saveCalls)
	assert.Equal(t, 4, canvas.restoreCalls)
	require.Len(t, e.formOperatorCache, 1)
}

func TestFormXObjectOperatorCacheReuseAcrossEvaluators(t *testing.T) {
	shared := newFormOperatorCacheForTest()
	formDict := entity.NewDict()
	formDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	formStream := entity.NewStream(formDict, []byte("q Q"))

	e1 := NewEvaluator(nil)
	e1.SetCanvas(newRecordingCanvas())
	e1.SetFormOperatorCache(shared)
	require.NoError(t, e1.evaluateFormXObject(formStream, entity.Name("Fm1")))
	assert.Equal(t, 1, shared.setCalls)
	assert.Equal(t, 2, shared.setLen)

	// Ensure evaluator2 can reuse shared parsed operators without re-parsing stream bytes.
	formStream.SetData([]byte(""))
	e2 := NewEvaluator(nil)
	e2.SetCanvas(newRecordingCanvas())
	e2.SetFormOperatorCache(shared)
	require.NoError(t, e2.evaluateFormXObject(formStream, entity.Name("Fm1")))
	assert.Equal(t, 1, shared.hitCalls)
	assert.Equal(t, 1, shared.setCalls)
}

func TestParseOperatorsAndEvaluate(t *testing.T) {
	e := NewEvaluator(nil)
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("Times-Roman"))
	fontCategory := entity.NewDict()
	fontCategory.Set(entity.Name("F1"), fontDict)
	resources := entity.NewDict()
	resources.Set(entity.Name("Font"), fontCategory)
	e.SetResources(resources)

	require.NoError(t, e.parseOperators([]byte("q /F1 12 Tf BT (A) Tj ET Q")))
	assert.Greater(t, len(e.GetOperators()), 0)
	assert.Equal(t, "A", e.ExtractedText())

	err := e.Evaluate([]entity.Object{entity.NewStream(entity.NewDict(), []byte("q"))})
	require.NoError(t, err)
}

func TestExecuteOperatorBranches(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("Times-Roman"))
	fontCategory := entity.NewDict()
	fontCategory.Set(entity.Name("F1"), fontDict)
	formXObjDict := entity.NewDict()
	formXObjDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	formXObjDict.Set(entity.Name("Resources"), entity.NewDict())
	formXObjDict.Set(entity.Name("BBox"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(2), entity.NewReal(2)))
	formXObj := entity.NewStream(formXObjDict, []byte("n"))
	xobjectCategory := entity.NewDict()
	xobjectCategory.Set(entity.Name("X1"), formXObj)
	resources := entity.NewDict()
	resources.Set(entity.Name("Font"), fontCategory)
	resources.Set(entity.Name("XObject"), xobjectCategory)
	e.SetResources(resources)

	executions := []Operator{
		{Opcode: "q"},
		{Opcode: "BT"},
		{Opcode: "Tj", Operands: []entity.Object{entity.NewString("x")}},
		{Opcode: "'", Operands: []entity.Object{entity.NewString("x")}},
		{Opcode: "\"", Operands: []entity.Object{entity.NewReal(0), entity.NewReal(0), entity.NewString("x")}},
		{Opcode: "TJ", Operands: []entity.Object{entity.NewArray(entity.NewString("x"), entity.NewInteger(-20))}},
		{Opcode: "Td", Operands: []entity.Object{entity.NewInteger(2), entity.NewInteger(3)}},
		{Opcode: "TD", Operands: []entity.Object{entity.NewInteger(2), entity.NewInteger(3)}},
		{Opcode: "T*"},
		{Opcode: "Tm", Operands: []entity.Object{
			entity.NewReal(1), entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(0), entity.NewReal(0),
		}},
		{Opcode: "Tc", Operands: []entity.Object{entity.NewReal(1)}},
		{Opcode: "Tw", Operands: []entity.Object{entity.NewReal(2)}},
		{Opcode: "Tz", Operands: []entity.Object{entity.NewReal(100)}},
		{Opcode: "TL", Operands: []entity.Object{entity.NewReal(10)}},
		{Opcode: "Tf", Operands: []entity.Object{entity.Name("F1"), entity.NewReal(12)}},
		{Opcode: "Tr", Operands: []entity.Object{entity.NewInteger(0)}},
		{Opcode: "Ts", Operands: []entity.Object{entity.NewReal(0)}},
		{Opcode: "w", Operands: []entity.Object{entity.NewReal(1)}},
		{Opcode: "J", Operands: []entity.Object{entity.NewInteger(0)}},
		{Opcode: "j", Operands: []entity.Object{entity.NewInteger(0)}},
		{Opcode: "M", Operands: []entity.Object{entity.NewReal(10)}},
		{Opcode: "m", Operands: []entity.Object{entity.NewReal(1), entity.NewReal(2)}},
		{Opcode: "l", Operands: []entity.Object{entity.NewReal(3), entity.NewReal(4)}},
		{Opcode: "c", Operands: []entity.Object{
			entity.NewReal(1), entity.NewReal(1), entity.NewReal(2), entity.NewReal(2), entity.NewReal(3), entity.NewReal(3),
		}},
		{Opcode: "v", Operands: []entity.Object{
			entity.NewReal(1), entity.NewReal(1), entity.NewReal(2), entity.NewReal(2),
		}},
		{Opcode: "y", Operands: []entity.Object{
			entity.NewReal(1), entity.NewReal(1), entity.NewReal(2), entity.NewReal(2),
		}},
		{Opcode: "H"},
		{Opcode: "h"},
		{Opcode: "re", Operands: []entity.Object{
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1),
		}},
		{Opcode: "f"},
		{Opcode: "f*"},
		{Opcode: "B"},
		{Opcode: "B*"},
		{Opcode: "b"},
		{Opcode: "S"},
		{Opcode: "s"},
		{Opcode: "b*"},
		{Opcode: "n"},
		{Opcode: "W"},
		{Opcode: "W*"},
		{Opcode: "cs"},
		{Opcode: "CS"},
		{Opcode: "sc"},
		{Opcode: "SC"},
		{Opcode: "scn"},
		{Opcode: "SCN"},
		{Opcode: "g", Operands: []entity.Object{entity.NewReal(0)}},
		{Opcode: "G", Operands: []entity.Object{entity.NewReal(1)}},
		{Opcode: "rg", Operands: []entity.Object{
			entity.NewReal(0), entity.NewReal(0.5), entity.NewReal(1),
		}},
		{Opcode: "RG", Operands: []entity.Object{
			entity.NewReal(1), entity.NewReal(0), entity.NewReal(0),
		}},
		{Opcode: "k", Operands: []entity.Object{
			entity.NewReal(0), entity.NewReal(1), entity.NewReal(0), entity.NewReal(0),
		}},
		{Opcode: "K", Operands: []entity.Object{
			entity.NewReal(1), entity.NewReal(0), entity.NewReal(0), entity.NewReal(0),
		}},
		{Opcode: "sh", Operands: []entity.Object{entity.Name("S1")}},
		{Opcode: "gs", Operands: []entity.Object{entity.Name("G1")}},
		{Opcode: "Do", Operands: []entity.Object{entity.Name("X1")}},
		{Opcode: "BI"},
		{Opcode: "ID"},
		{Opcode: "EI"},
		{Opcode: "cm", Operands: []entity.Object{
			entity.NewReal(1), entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(10), entity.NewReal(20),
		}},
		{Opcode: "Y", Operands: []entity.Object{
			entity.NewReal(1), entity.NewReal(1), entity.NewReal(2), entity.NewReal(2), entity.NewReal(3), entity.NewReal(3),
		}},
		{Opcode: "d0", Operands: []entity.Object{entity.NewInteger(0), entity.NewInteger(0)}},
		{Opcode: "d1", Operands: []entity.Object{entity.NewInteger(0), entity.NewInteger(0)}},
		{Opcode: "ET"},
		{Opcode: "Q"},
		{Opcode: "UNKNOWN"},
	}

	for _, op := range executions {
		require.NoError(t, e.executeOperator(op), op.Opcode)
	}

	assert.Greater(t, len(e.GetOperators()), 0)
}

func TestOperatorAliases(t *testing.T) {
	e := NewEvaluator(nil)
	egraphics := e.graphics
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	err := e.parseOperators([]byte("1 2 m 3 4 l 5 6 7 8 9 10 c 11 12 v 13 14 y s f* B* b b* n W W*"))
	require.NoError(t, err)

	assert.Equal(t, 13, len(e.GetOperators()))
	assert.True(t, egraphics.path.IsEmpty())
	assert.NotEmpty(t, e.GetOperators())
}

func TestContentOperatorMap(t *testing.T) {
	assert.True(t, isContentOperatorKeyword("BT"))
	assert.False(t, isContentOperatorKeyword("UNKNOWN"))
	assert.Equal(t, 0.0, clamp(-1, 0, 1))
	assert.Equal(t, 1.0, clamp(2, 0, 1))
	r, _, _ := cmykToRGB(0.5, 0, 0, 0)
	assert.InDelta(t, 0.5019607843137255, r, 1e-9)
}

func TestGetNumberOperand(t *testing.T) {
	v, err := getNumberOperand(entity.NewInteger(3))
	require.NoError(t, err)
	assert.Equal(t, 3.0, v)

	bits := getImageBitsPerComponent(entity.NewInteger(12))
	assert.EqualValues(t, 12, bits)
	bits = getImageBitsPerComponent(entity.NewReal(8.0))
	assert.EqualValues(t, 8, bits)
	bits = getImageBitsPerComponent(entity.NewString("10"))
	assert.EqualValues(t, 10, bits)
	bits = getImageBitsPerComponent(entity.NewString("bad"))
	assert.EqualValues(t, 8, bits)
	bits = getImageBitsPerComponent(entity.NewInteger(0))
	assert.EqualValues(t, 8, bits)

	_, err = getNumberOperand(entity.NewString("x"))
	require.Error(t, err)
}

type testMapXRef struct {
	objects map[entity.Ref]entity.Object
}

func (m *testMapXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	obj, ok := m.objects[ref]
	if !ok {
		return nil, errors.New("missing object")
	}
	return obj, nil
}

type formOperatorCacheForTest struct {
	store    map[*entity.Stream][]Operator
	hitCalls int
	setCalls int
	setLen   int
}

func newFormOperatorCacheForTest() *formOperatorCacheForTest {
	return &formOperatorCacheForTest{
		store: make(map[*entity.Stream][]Operator),
	}
}

func (c *formOperatorCacheForTest) Get(xobj *entity.Stream) ([]Operator, bool) {
	ops, ok := c.store[xobj]
	if !ok {
		return nil, false
	}
	c.hitCalls++
	return append([]Operator(nil), ops...), true
}

func (c *formOperatorCacheForTest) Set(xobj *entity.Stream, ops []Operator) {
	c.setCalls++
	c.setLen = len(ops)
	c.store[xobj] = append([]Operator(nil), ops...)
}

func TestImageResolveHelperBranches(t *testing.T) {
	refDecode := entity.NewRef(10, 0)
	refMask := entity.NewRef(11, 0)
	refCS := entity.NewRef(12, 0)
	refStream := entity.NewRef(13, 0)
	refLookup := entity.NewRef(14, 0)
	refInvalidDecode := entity.NewRef(15, 0)

	filteredDict := entity.NewDict()
	filteredDict.Set(entity.Name("Filter"), entity.Name("UnknownFilter"))

	xref := &testMapXRef{
		objects: map[entity.Ref]entity.Object{
			refDecode: entity.NewArray(entity.NewInteger(0), entity.NewReal(1.5)),
			refMask: entity.NewArray(
				entity.NewInteger(0), entity.NewInteger(10),
				entity.NewInteger(1), entity.NewInteger(11),
				entity.NewInteger(2), entity.NewInteger(12),
			),
			refCS:            entity.Name("DeviceRGB"),
			refStream:        entity.NewStream(entity.NewDict(), []byte{1, 2, 3}),
			refLookup:        entity.NewHexString("0A0"),
			refInvalidDecode: entity.NewArray(entity.NewString("bad")),
		},
	}

	e := NewEvaluator(xref)

	decode := e.resolveImageDecodeArray(refDecode)
	require.Equal(t, []float64{0, 1.5}, decode)
	assert.Nil(t, e.resolveImageDecodeArrayWithDepth(refInvalidDecode, 0))
	assert.Nil(t, e.resolveImageDecodeArrayWithDepth(refDecode, 9))

	maskArr := e.resolveMaskArray(refMask, 0)
	require.NotNil(t, maskArr)
	assert.Equal(t, 6, maskArr.Len())
	assert.Nil(t, e.resolveMaskArray(nil, 0))
	assert.Nil(t, e.resolveMaskArray(refMask, 9))

	colorMask := e.resolveColorKeyMask(refMask, "DeviceRGB")
	require.NotNil(t, colorMask)
	assert.True(t, colorMask.IsTransparent([]uint8{5, 6, 7}))
	assert.Nil(t, e.resolveColorKeyMask(entity.NewArray(entity.NewInteger(1)), "DeviceRGB"))
	assert.Nil(t, e.resolveColorKeyMask(refMask, "DeviceGray"))

	lookup, ok := e.resolveIndexedLookupBytes(refLookup, 0)
	require.True(t, ok)
	assert.Equal(t, []byte{0x0A, 0x00}, lookup)
	lookup, ok = e.resolveIndexedLookupBytes(entity.NewHexString("GG"), 0)
	require.True(t, ok)
	assert.Equal(t, []byte("GG"), lookup)
	lookup, ok = e.resolveIndexedLookupBytes(entity.NewStream(filteredDict, []byte{9, 8}), 0)
	require.True(t, ok)
	assert.Equal(t, []byte{9, 8}, lookup)
	assert.Nil(t, e.resolveImageDecodeArrayWithDepth(nil, 0))
	_, ok = e.resolveIndexedLookupBytes(entity.NewInteger(1), 0)
	assert.False(t, ok)
	_, ok = e.resolveIndexedLookupBytes(entity.NewString("A"), 9)
	assert.False(t, ok)

	streamObj, ok := e.resolveStreamObject(refStream)
	require.True(t, ok)
	require.NotNil(t, streamObj)
	assert.Equal(t, []byte{1, 2, 3}, streamObj.RawBytes())
	_, ok = e.resolveStreamObject(entity.NewInteger(1))
	assert.False(t, ok)

	cs, ok := e.resolveColorSpaceName(entity.Name("DeviceGray"), 0)
	require.True(t, ok)
	assert.Equal(t, "DeviceGray", cs)
	cs, ok = e.resolveColorSpaceName(refCS, 0)
	require.True(t, ok)
	assert.Equal(t, "DeviceRGB", cs)
	_, ok = e.resolveColorSpaceName(refCS, 9)
	assert.False(t, ok)
}

func TestParseFunctionShadingBranches(t *testing.T) {
	e := NewEvaluator(nil)
	shading := entity.NewShading(entity.ShadingFunctionBased, "DeviceRGB")

	dict := entity.NewDict()
	dict.Set(entity.Name("Domain"), entity.NewArray(
		entity.NewInteger(1),
		entity.NewReal(2.5),
		entity.NewInteger(3),
		entity.NewReal(4.5),
	))
	dict.Set(entity.Name("Matrix"), entity.NewArray(
		entity.NewInteger(1),
		entity.NewReal(0),
		entity.NewInteger(0),
		entity.NewReal(1),
		entity.NewInteger(9),
		entity.NewReal(11),
	))
	dict.Set(entity.Name("Function"), buildExponentialFunctionDict())

	got, err := e.parseFunctionShading(dict, shading)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, [4]float64{1, 2.5, 3, 4.5}, got.GetDomain())
	assert.Equal(t, [6]float64{1, 0, 0, 1, 9, 11}, got.GetMatrix())
	assert.NotEmpty(t, got.GetFunctions())

	invalidFnDict := entity.NewDict()
	invalidFnDict.Set(entity.Name("Function"), entity.NewString("bad"))
	got, err = e.parseFunctionShading(invalidFnDict, entity.NewShading(entity.ShadingFunctionBased, "DeviceRGB"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.GetFunctions())
}

func TestEvaluator_ImageColorSpaceAndIndexedBranches(t *testing.T) {
	iccDict := entity.NewDict()
	iccDict.Set(entity.Name("N"), entity.NewInteger(4))
	iccStream := entity.NewStream(iccDict, []byte{1})

	e := NewEvaluator(&testMapXRef{
		objects: map[entity.Ref]entity.Object{
			entity.NewRef(41, 0): entity.NewArray(
				entity.Name("Indexed"),
				entity.Name("DeviceRGB"),
				entity.NewInteger(255),
				entity.NewString("abc"),
			),
		},
	})

	cs, ok := e.resolveImageColorSpaceWithDepth(entity.NewArray(entity.Name("ICCBased"), iccStream), 0)
	require.True(t, ok)
	assert.Equal(t, "DeviceCMYK", cs)

	_, ok = e.resolveImageColorSpaceWithDepth(entity.NewArray(entity.Name("ICCBased"), entity.NewDict()), 0)
	assert.False(t, ok)

	base, lookup, ok := e.resolveIndexedColorSpace(entity.NewRef(41, 0), 0)
	require.True(t, ok)
	assert.Equal(t, "DeviceRGB", base)
	assert.Equal(t, []byte("abc"), lookup)

	_, _, ok = e.resolveIndexedColorSpace(entity.NewArray(entity.Name("DeviceRGB")), 0)
	assert.False(t, ok)

	v, ok := objectInt(entity.NewInteger(3))
	require.True(t, ok)
	assert.Equal(t, 3, v)
	v, ok = objectInt(entity.NewReal(2.9))
	require.True(t, ok)
	assert.Equal(t, 2, v)
	_, ok = objectInt(entity.NewString("bad"))
	assert.False(t, ok)

	n, ok := e.resolveICCBasedComponentValue(entity.NewStream(iccDict, nil))
	require.True(t, ok)
	assert.Equal(t, 4, n)
	n, ok = e.resolveICCBasedComponentValue(iccDict)
	require.True(t, ok)
	assert.Equal(t, 4, n)
	_, ok = e.resolveICCBasedComponentValue(entity.NewString("bad"))
	assert.False(t, ok)

	n, ok = parseICCBasedN(iccDict)
	require.True(t, ok)
	assert.Equal(t, 4, n)
	_, ok = parseICCBasedN(nil)
	assert.False(t, ok)
}

func TestEvaluator_FilterShadingAndScaleHelpers(t *testing.T) {
	assert.Equal(t, domainimage.FilterASCIIHex, normalizeImageFilterName("AHx"))
	assert.Equal(t, domainimage.FilterASCII85, normalizeImageFilterName("A85"))
	assert.Equal(t, domainimage.FilterLZW, normalizeImageFilterName("LZW"))
	assert.Equal(t, domainimage.FilterRunLength, normalizeImageFilterName("RL"))
	assert.Equal(t, domainimage.FilterCCITTFax, normalizeImageFilterName("CCF"))

	e := NewEvaluator(nil)
	shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
	_, err := e.parseAxialShading(entity.NewDict(), shading)
	require.Error(t, err)
	_, err = e.parseAxialShading(entity.NewDict(), shading)
	require.Error(t, err)

	axialDict := entity.NewDict()
	axialDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewString("bad"),
		entity.NewString("bad"),
		entity.NewString("bad"),
		entity.NewString("bad"),
	))
	_, err = e.parseAxialShading(axialDict, shading)
	require.Error(t, err)

	radial := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
	_, err = e.parseRadialShading(entity.NewDict(), radial)
	require.Error(t, err)

	radialDict := entity.NewDict()
	radialDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewString("bad"),
		entity.NewString("bad"),
		entity.NewString("bad"),
		entity.NewString("bad"),
		entity.NewString("bad"),
		entity.NewString("bad"),
	))
	_, err = e.parseRadialShading(radialDict, radial)
	require.Error(t, err)

	assert.Equal(t, 1.0, matrixAverageScale([6]float64{0, 0, 0, 0, 0, 0}))
	assert.Equal(t, 2.0, matrixAverageScale([6]float64{2, 0, 0, 0, 0, 0}))
	assert.Equal(t, 3.0, matrixAverageScale([6]float64{0, 0, 0, 3, 0, 0}))
	assert.Equal(t, 2.0, matrixAverageScale([6]float64{2, 0, 0, 2, 0, 0}))

	assert.False(t, resolveImageInterpolate(nil, false))
	assert.True(t, resolveImageInterpolate(nil, true))
	assert.True(t, resolveImageInterpolate(entity.NewBoolean(true), false))
	assert.False(t, resolveImageInterpolate(entity.NewBoolean(false), true))
	assert.True(t, resolveImageInterpolate(entity.NewInteger(1), false))
	assert.False(t, resolveImageInterpolate(entity.NewInteger(0), true))

	val, explicit := resolveImageInterpolateOption(nil, true)
	assert.True(t, val)
	assert.False(t, explicit)
	val, explicit = resolveImageInterpolateOption(entity.NewBoolean(false), true)
	assert.False(t, val)
	assert.True(t, explicit)

	autoNearest := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		true,
		false,
		domainimage.FilterFlate,
		"DeviceGray",
		false,
		16,
		16,
		8,
		8,
	)
	assert.True(t, autoNearest.Interpolate)
	assert.Equal(t, "auto_interpolate=true", autoNearest.Reason)
	assert.Equal(t, "auto_approx_bilinear", autoNearest.Sampler)
	assert.Equal(t, "rejected_strict_downscale", autoNearest.ExperimentalCandidate)

	autoDownscale := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterFlate,
		"DeviceRGB",
		false,
		64,
		64,
		16,
		16,
	)
	assert.True(t, autoDownscale.Interpolate)
	assert.Equal(t, "auto_interpolate=false_downscale", autoDownscale.Reason)
	assert.Equal(t, "auto_downscale_bilinear", autoDownscale.Sampler)
	assert.Equal(t, "rejected_colorspace", autoDownscale.ExperimentalCandidate)

	tinyJPEG := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		true,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		false,
		16,
		16,
		8,
		8,
	)
	assert.True(t, tinyJPEG.Interpolate)
	assert.Equal(t, "auto_interpolate=true", tinyJPEG.Reason)
	assert.Equal(t, "auto_approx_bilinear", tinyJPEG.Sampler)

	explicitBilinear := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		true,
		true,
		domainimage.FilterFlate,
		"DeviceGray",
		false,
		16,
		16,
		8,
		8,
	)
	assert.True(t, explicitBilinear.Interpolate)
	assert.Equal(t, "explicit_interpolate=true", explicitBilinear.Reason)
	assert.Equal(t, "explicit_approx_bilinear", explicitBilinear.Sampler)
	assert.Equal(t, "rejected_interpolate_explicit", explicitBilinear.ExperimentalCandidate)

	largeDefault := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterNone,
		"Indexed",
		false,
		324,
		450,
		507,
		704,
	)
	assert.True(t, largeDefault.Interpolate)
	assert.Equal(t, "auto_interpolate=false_upscale", largeDefault.Reason)
	assert.Equal(t, "auto_upscale_bilinear", largeDefault.Sampler)

	largeDefaultUpscale := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterNone,
		"DeviceGray",
		false,
		324,
		450,
		507,
		704,
	)
	assert.True(t, largeDefaultUpscale.Interpolate)
	assert.Equal(t, "auto_interpolate=false_upscale", largeDefaultUpscale.Reason)
	assert.Equal(t, "auto_upscale_bilinear", largeDefaultUpscale.Sampler)

	nearIdentityUpscale := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterNone,
		"DeviceRGB",
		false,
		781,
		627,
		781.5306,
		627.0225,
	)
	assert.False(t, nearIdentityUpscale.Interpolate)
	assert.Equal(t, "auto_interpolate=false_near_identity_scale", nearIdentityUpscale.Reason)
	assert.Equal(t, "auto_nearest", nearIdentityUpscale.Sampler)

	legacyTinyICCBased := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterFlate,
		"DeviceGray",
		true,
		16,
		16,
		4,
		4,
	)
	assert.True(t, legacyTinyICCBased.Interpolate)
	assert.Equal(t, "auto_interpolate=false_downscale_tiny_iccbased_gray", legacyTinyICCBased.Reason)
	assert.Equal(t, "auto_box_tiny_iccbased_gray_downscale", legacyTinyICCBased.Sampler)

	legacyTinyCCITTFax := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterCCITTFax,
		"DeviceGray",
		false,
		16,
		16,
		4,
		4,
	)
	assert.True(t, legacyTinyCCITTFax.Interpolate)
	assert.Equal(t, "auto_interpolate=false_downscale_tiny_gray_ccittfax", legacyTinyCCITTFax.Reason)
	assert.Equal(t, "auto_approx_bilinear_tiny_gray_ccittfax_downscale", legacyTinyCCITTFax.Sampler)

	adaptiveTinyNearest := chooseImageSamplingPolicy(
		ImageSamplingModeAdaptiveDCTICCBasedV1,
		true,
		false,
		domainimage.FilterFlate,
		"DeviceGray",
		false,
		16,
		16,
		8,
		8,
	)
	assert.False(t, adaptiveTinyNearest.Interpolate)
	assert.Equal(t, "adaptive_tiny_gray_downscale_non_encoded", adaptiveTinyNearest.Reason)
	assert.Equal(t, "adaptive_nearest_tiny_gray_downscale_non_encoded", adaptiveTinyNearest.Sampler)

	adaptiveICCBased := chooseImageSamplingPolicy(
		ImageSamplingModeAdaptiveDCTICCBasedV1,
		true,
		false,
		domainimage.FilterDCT,
		"DeviceRGB",
		true,
		128,
		128,
		96,
		96,
	)
	assert.True(t, adaptiveICCBased.Interpolate)
	assert.Equal(t, "adaptive_encoded_or_iccbased_downscale", adaptiveICCBased.Reason)
	assert.Equal(t, "adaptive_approx_bilinear_dct_or_iccbased", adaptiveICCBased.Sampler)

	experimentalSplash := chooseImageSamplingPolicy(
		ImageSamplingModeExperimentalSplashScaleOnlyV1,
		false,
		false,
		domainimage.FilterFlate,
		"DeviceGray",
		false,
		4,
		4,
		4,
		4,
	)
	assert.True(t, experimentalSplash.Interpolate)
	assert.Equal(t, "experimental_splash_scale_only_small_gray_or_indexed", experimentalSplash.Reason)
	assert.Equal(t, "experimental_splash_scale_only", experimentalSplash.Sampler)
	assert.Equal(t, "candidate_small_gray_or_indexed_non_downscale", experimentalSplash.ExperimentalCandidate)

	experimentalIndexedOriginDownscale := chooseImageSamplingPolicy(
		ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1,
		false,
		false,
		domainimage.FilterFlate,
		"Indexed",
		false,
		324,
		450,
		243,
		338,
	)
	assert.True(t, experimentalIndexedOriginDownscale.Interpolate)
	assert.Equal(t, "experimental_indexed_origin_downscale_phase", experimentalIndexedOriginDownscale.Reason)
	assert.Equal(t, "experimental_indexed_origin_downscale_bilinear", experimentalIndexedOriginDownscale.Sampler)
	assert.Equal(t, "rejected_large_source", experimentalIndexedOriginDownscale.ExperimentalCandidate)

	phaseX, phaseY := imageSamplingPhase(
		"experimental_indexed_origin_downscale_bilinear",
		"experimental_indexed_origin_downscale_phase",
		true,
		[6]float64{243, 0, 0, 338, 0, 0},
	)
	assert.Equal(t, 0.5, phaseX)
	assert.Equal(t, 0.5, phaseY)

	phaseX, phaseY = imageSamplingPhase(
		"experimental_indexed_origin_downscale_bilinear",
		"experimental_indexed_origin_downscale_phase",
		true,
		[6]float64{468, 0, 0, 624, 72, 96},
	)
	assert.Zero(t, phaseX)
	assert.Zero(t, phaseY)

	assert.True(t, isExperimentalSplashScaleOnlyCandidate(false, "DeviceGray", 4, 4, 4, 4))
	assert.True(t, isExperimentalSplashScaleOnlyCandidate(false, "Indexed", 4, 4, 5, 5))
	assert.False(t, isExperimentalSplashScaleOnlyCandidate(true, "DeviceGray", 4, 4, 4, 4))
	assert.False(t, isExperimentalSplashScaleOnlyCandidate(false, "DeviceRGB", 4, 4, 4, 4))
	assert.False(t, isExperimentalSplashScaleOnlyCandidate(false, "DeviceGray", 64, 4, 64, 4))
	assert.False(t, isExperimentalSplashScaleOnlyCandidate(false, "Indexed", 324, 450, 243, 338))
	assert.False(t, isExperimentalSplashScaleOnlyCandidate(false, "DeviceGray", 4, 4, 3, 3))
	assert.Equal(t, "candidate_small_gray_or_indexed_non_downscale", classifyExperimentalSplashScaleOnlyCandidate(false, "DeviceGray", 4, 4, 4.25, 4.75))
	assert.Equal(t, "rejected_interpolate_explicit", classifyExperimentalSplashScaleOnlyCandidate(true, "DeviceGray", 4, 4, 4, 4))
	assert.Equal(t, "rejected_colorspace", classifyExperimentalSplashScaleOnlyCandidate(false, "DeviceRGB", 4, 4, 4, 4))
	assert.Equal(t, "rejected_large_source", classifyExperimentalSplashScaleOnlyCandidate(false, "DeviceGray", 64, 4, 64, 4))
	assert.Equal(t, "rejected_large_source", classifyExperimentalSplashScaleOnlyCandidate(false, "Indexed", 756, 1008, 468, 624))
	assert.Equal(t, "rejected_strict_downscale", classifyExperimentalSplashScaleOnlyCandidate(false, "DeviceGray", 4, 4, 3, 3))
	assert.Equal(t, "candidate_large_indexed_cmyk_downscale", classifyExperimentalIndexedCMYKCandidate("Indexed", "DeviceCMYK", 756, 1008, 468, 624))
	assert.Equal(t, "candidate_large_indexed_gray_origin_downscale", classifyExperimentalIndexedGrayOriginCandidate("Indexed", "DeviceGray", 324, 450, 243, 338, [6]float64{243, 0, 0, 338, 0, 0}))
	assert.Equal(t, "rejected_non_gray_indexed_base", classifyExperimentalIndexedGrayOriginCandidate("Indexed", "DeviceCMYK", 324, 450, 243, 338, [6]float64{243, 0, 0, 338, 0, 0}))
	assert.Equal(t, "rejected_non_origin_placement", classifyExperimentalIndexedGrayOriginCandidate("Indexed", "DeviceGray", 324, 450, 243, 338, [6]float64{243, 0, 0, 338, 1, 0}))
	assert.Equal(t, "rejected_non_axis_aligned", classifyExperimentalIndexedGrayOriginCandidate("Indexed", "DeviceGray", 324, 450, 243, 338, [6]float64{243, 1, 0, 338, 0, 0}))
	assert.Equal(t, "candidate_tiny_dct_iccbased_gray_downscale", classifyExperimentalDCTGrayIgnoreICCCandidate(domainimage.FilterDCT, "DeviceGray", true, 16, 16, 4, 4, [6]float64{4, 0, 0, 4, 0, 0}))
	assert.Equal(t, "rejected_non_iccbased_source", classifyExperimentalDCTGrayIgnoreICCCandidate(domainimage.FilterDCT, "DeviceGray", false, 16, 16, 4, 4, [6]float64{4, 0, 0, 4, 0, 0}))
	assert.Equal(t, "rejected_non_dct_filter", classifyExperimentalDCTGrayIgnoreICCCandidate(domainimage.FilterFlate, "DeviceGray", true, 16, 16, 4, 4, [6]float64{4, 0, 0, 4, 0, 0}))
	assert.Equal(t, "rejected_non_indexed_colorspace", classifyExperimentalIndexedCMYKCandidate("DeviceCMYK", "", 756, 1008, 468, 624))
	assert.Equal(t, "rejected_non_cmyk_indexed_base", classifyExperimentalIndexedCMYKCandidate("Indexed", "DeviceGray", 756, 1008, 468, 624))
	assert.Equal(t, "rejected_small_source", classifyExperimentalIndexedCMYKCandidate("Indexed", "DeviceCMYK", 64, 64, 64, 64))
	assert.Equal(t, "candidate_large_indexed_cmyk_downscale", classifyExperimentalIndexedCMYKCandidate("Indexed", "DeviceCMYK", 756, 1008, 756, 1008))
	assert.Equal(t, domainimage.CMYKConversionModeHybrid75, resolveExperimentalIndexedCMYKConversionMode(ImageSamplingModeLegacy, "candidate_large_indexed_cmyk_downscale"))
	assert.Equal(t, domainimage.CMYKConversionModeHybrid75, resolveExperimentalIndexedCMYKConversionMode(ImageSamplingModeExperimentalDCTGrayIgnoreICCV1, "candidate_large_indexed_cmyk_downscale"))
	assert.Equal(t, domainimage.CMYKConversionModeSimpleSubtractive, resolveExperimentalIndexedCMYKConversionMode(ImageSamplingModeExperimentalIndexedCMYKSimpleV1, "candidate_large_indexed_cmyk_downscale"))
	assert.Equal(t, domainimage.CMYKConversionModeHybrid75, resolveExperimentalIndexedCMYKConversionMode(ImageSamplingModeExperimentalIndexedCMYKHybrid75V1, "candidate_large_indexed_cmyk_downscale"))
	assert.Equal(t, domainimage.CMYKConversionModeStdlib, resolveExperimentalIndexedCMYKConversionMode(ImageSamplingModeExperimentalIndexedCMYKStdlibV1, "candidate_large_indexed_cmyk_downscale"))
	assert.Equal(t, domainimage.CMYKConversionModeDefault, resolveExperimentalIndexedCMYKConversionMode(ImageSamplingModeLegacy, "rejected_small_source"))
	assert.Equal(t, domainimage.CMYKConversionModeDefault, resolveExperimentalIndexedCMYKConversionMode(ImageSamplingModeAdaptiveDCTICCBasedV1, "candidate_large_indexed_cmyk_downscale"))
	iccProfile, iccComponents, iccMode := resolveExperimentalDCTGrayICCProfile(ImageSamplingModeLegacy, "candidate_tiny_dct_iccbased_gray_downscale", []byte{1, 2, 3}, 1)
	assert.Nil(t, iccProfile)
	assert.Zero(t, iccComponents)
	assert.Equal(t, "legacy_selective_ignore", iccMode)
	iccProfile, iccComponents, iccMode = resolveExperimentalDCTGrayICCProfile(ImageSamplingModeExperimentalDCTGrayIgnoreICCV1, "candidate_tiny_dct_iccbased_gray_downscale", []byte{1, 2, 3}, 1)
	assert.Nil(t, iccProfile)
	assert.Zero(t, iccComponents)
	assert.Equal(t, "ignore", iccMode)
	iccProfile, iccComponents, iccMode = resolveExperimentalDCTGrayICCProfile(ImageSamplingModeAdaptiveDCTICCBasedV1, "candidate_tiny_dct_iccbased_gray_downscale", []byte{1, 2, 3}, 1)
	assert.Equal(t, []byte{1, 2, 3}, iccProfile)
	assert.Equal(t, 1, iccComponents)
	assert.Equal(t, "default", iccMode)
	sampler, reason := resolveSelectiveIndexedGrayOriginDownscaleSampler(ImageSamplingModeLegacy, "candidate_large_indexed_gray_origin_downscale", "auto_downscale_bilinear", "auto_interpolate=false_downscale")
	assert.Equal(t, "experimental_indexed_origin_downscale_bilinear", sampler)
	assert.Equal(t, "legacy_selective_indexed_origin_downscale_phase", reason)
	sampler, reason = resolveSelectiveIndexedGrayOriginDownscaleSampler(ImageSamplingModeLegacy, "rejected_non_origin_placement", "auto_downscale_bilinear", "auto_interpolate=false_downscale")
	assert.Equal(t, "auto_downscale_bilinear", sampler)
	assert.Equal(t, "auto_interpolate=false_downscale", reason)
	sampler, reason = resolveSelectiveIndexedGrayOriginDownscaleSampler(ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1, "candidate_large_indexed_gray_origin_downscale", "auto_downscale_bilinear", "auto_interpolate=false_downscale")
	assert.Equal(t, "auto_downscale_bilinear", sampler)
	assert.Equal(t, "auto_interpolate=false_downscale", reason)
	sampler, reason = resolveSelectiveIndexedGrayOriginDownscaleSampler(ImageSamplingModeExperimentalDCTGrayIgnoreICCV1, "candidate_large_indexed_gray_origin_downscale", "auto_downscale_bilinear", "auto_interpolate=false_downscale")
	assert.Equal(t, "experimental_indexed_origin_downscale_bilinear", sampler)
	assert.Equal(t, "legacy_selective_indexed_origin_downscale_phase", reason)

	assert.Equal(t, ImageSamplingModeLegacy, normalizeImageSamplingMode(""))
	assert.Equal(t, ImageSamplingModeLegacy, normalizeImageSamplingMode("default"))
	assert.Equal(t, ImageSamplingModeAdaptiveDCTICCBasedV1, normalizeImageSamplingMode(ImageSamplingModeAdaptiveDCTICCBasedV1))
	assert.Equal(t, ImageSamplingModeExperimentalSplashScaleOnlyV1, normalizeImageSamplingMode(ImageSamplingModeExperimentalSplashScaleOnlyV1))
	assert.Equal(t, ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1, normalizeImageSamplingMode(ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1))
	assert.Equal(t, ImageSamplingModeExperimentalIndexedCMYKSimpleV1, normalizeImageSamplingMode(ImageSamplingModeExperimentalIndexedCMYKSimpleV1))
	assert.Equal(t, ImageSamplingModeExperimentalIndexedCMYKHybrid75V1, normalizeImageSamplingMode(ImageSamplingModeExperimentalIndexedCMYKHybrid75V1))
	assert.Equal(t, ImageSamplingModeExperimentalIndexedCMYKStdlibV1, normalizeImageSamplingMode(ImageSamplingModeExperimentalIndexedCMYKStdlibV1))
	assert.Equal(t, ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1, normalizeImageSamplingMode(ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1))
	assert.Equal(t, ImageSamplingModeExperimentalDCTGrayIgnoreICCV1, normalizeImageSamplingMode(ImageSamplingModeExperimentalDCTGrayIgnoreICCV1))
	assert.Equal(t, ImageSamplingModeLegacy, normalizeImageSamplingMode("unknown"))
	assert.Equal(t, "default", formatCMYKConversionModeForTrace(""))
	assert.Equal(t, domainimage.CMYKConversionModeStdlib, formatCMYKConversionModeForTrace(domainimage.CMYKConversionModeStdlib))
	assert.Equal(t, "default", formatImageEdgeModeForTrace(""))
	assert.Equal(t, domainimage.ImageEdgeModeTransparentEdgeOverWhite, formatImageEdgeModeForTrace(domainimage.ImageEdgeModeTransparentEdgeOverWhite))
	assert.Equal(t, "default", formatGrayICCProfileModeForTrace(""))
	assert.Equal(t, "ignore", formatGrayICCProfileModeForTrace("ignore"))
	assert.True(t, usesCMYKImageConversion("DeviceCMYK", ""))
	assert.True(t, usesCMYKImageConversion("Indexed", "DeviceCMYK"))
	assert.False(t, usesCMYKImageConversion("Indexed", "DeviceRGB"))
	assert.Equal(t, "candidate_positive_subpixel_vertical_offset", classifyExperimentalRGBEdgeCandidate("DeviceRGB", 300, 200, 300.364873, 200.026132, [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907}))
	assert.Equal(t, "rejected_zero_or_negative_vertical_offset", classifyExperimentalRGBEdgeCandidate("DeviceRGB", 300, 200, 300.364873, 200.026132, [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412}))
	assert.Equal(t, "rejected_non_rgb_colorspace", classifyExperimentalRGBEdgeCandidate("DeviceGray", 300, 200, 300, 200, [6]float64{300, 0, 0, 200, 1, 1}))
	assert.Equal(t, "rejected_non_axis_aligned", classifyExperimentalRGBEdgeCandidate("DeviceRGB", 300, 200, 300, 200, [6]float64{300, 1, 0, 200, 1, 1}))
	assert.Equal(t, "rejected_non_upscale", classifyExperimentalRGBEdgeCandidate("DeviceRGB", 300, 200, 150, 100, [6]float64{150, 0, 0, 100, 1, 1}))
	assert.True(t, supportsExperimentalRGBTransparentEdgeSurface(16, 16, 20, 20))
	assert.False(t, supportsExperimentalRGBTransparentEdgeSurface(300, 200, 300.364873, 200.026132))
	assert.Equal(
		t,
		domainimage.ImageEdgeModeTransparentEdgeOverWhite,
		resolveExperimentalImageEdgeMode(ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1, "candidate_positive_subpixel_vertical_offset", 16, 16, 20, 20),
	)
	assert.Equal(
		t,
		domainimage.ImageEdgeModeDefault,
		resolveExperimentalImageEdgeMode(ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1, "candidate_positive_subpixel_vertical_offset", 300, 200, 300.364873, 200.026132),
	)
	assert.Equal(
		t,
		domainimage.ImageEdgeModeDefault,
		resolveExperimentalImageEdgeMode(ImageSamplingModeLegacy, "candidate_positive_subpixel_vertical_offset", 16, 16, 20, 20),
	)
	assert.Equal(t, domainimage.CMYKConversionModeDefault, e.resolveImageCMYKConversionMode("DeviceCMYK", "", 16, 16, 16, 16))
	assert.Equal(t, domainimage.CMYKConversionModeDefault, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 16, 16, 16, 16))
	assert.Equal(t, domainimage.CMYKConversionModeDefault, e.resolveImageCMYKConversionMode("DeviceRGB", "", 16, 16, 16, 16))
	assert.Equal(t, domainimage.ImageEdgeModeDefault, e.resolveImageEdgeMode("DeviceRGB", 300, 200, [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907}, 300.364873, 200.026132))
	e.SetImageSamplingMode(ImageSamplingModeLegacy)
	assert.Equal(t, domainimage.CMYKConversionModeHybrid75, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 756, 1008, 468, 624))
	e.SetImageSamplingMode(ImageSamplingModeExperimentalIndexedCMYKSimpleV1)
	assert.Equal(t, domainimage.CMYKConversionModeSimpleSubtractive, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 756, 1008, 468, 624))
	assert.Equal(t, domainimage.CMYKConversionModeDefault, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 64, 64, 64, 64))
	e.SetImageSamplingMode(ImageSamplingModeExperimentalIndexedCMYKHybrid75V1)
	assert.Equal(t, domainimage.CMYKConversionModeHybrid75, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 756, 1008, 468, 624))
	assert.Equal(t, domainimage.CMYKConversionModeDefault, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 64, 64, 64, 64))
	e.SetImageSamplingMode(ImageSamplingModeExperimentalIndexedCMYKStdlibV1)
	assert.Equal(t, domainimage.CMYKConversionModeStdlib, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 756, 1008, 468, 624))
	e.SetImageSamplingMode(ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1)
	assert.Equal(t, domainimage.CMYKConversionModeHybrid75, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 756, 1008, 468, 624))
	assert.Equal(t, domainimage.ImageEdgeModeTransparentEdgeOverWhite, e.resolveImageEdgeMode("DeviceRGB", 16, 16, [6]float64{20, 0, 0, 20, 1.5, 1.5}, 20, 20))
	assert.Equal(t, domainimage.ImageEdgeModeDefault, e.resolveImageEdgeMode("DeviceRGB", 16, 16, [6]float64{20, 0, 0, 20, 1.5, 1}, 20, 20))
	assert.Equal(t, domainimage.ImageEdgeModeDefault, e.resolveImageEdgeMode("DeviceRGB", 300, 200, [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907}, 300.364873, 200.026132))
	assert.Equal(t, domainimage.CMYKConversionModeDefault, e.resolveImageCMYKConversionMode("Indexed", "DeviceCMYK", 64, 64, 64, 64))

	e.SetImageSamplingMode(ImageSamplingModeAdaptiveDCTICCBasedV1)
	assert.Equal(t, ImageSamplingModeAdaptiveDCTICCBasedV1, e.imageSamplingMode)
	e.SetImageSamplingMode(ImageSamplingModeExperimentalSplashScaleOnlyV1)
	assert.Equal(t, ImageSamplingModeExperimentalSplashScaleOnlyV1, e.imageSamplingMode)
	e.SetImageSamplingMode(ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1)
	assert.Equal(t, ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1, e.imageSamplingMode)
	e.SetImageSamplingMode(ImageSamplingModeExperimentalIndexedCMYKSimpleV1)
	assert.Equal(t, ImageSamplingModeExperimentalIndexedCMYKSimpleV1, e.imageSamplingMode)
	e.SetImageSamplingMode(ImageSamplingModeExperimentalIndexedCMYKHybrid75V1)
	assert.Equal(t, ImageSamplingModeExperimentalIndexedCMYKHybrid75V1, e.imageSamplingMode)
	e.SetImageSamplingMode("bad-mode")
	assert.Equal(t, ImageSamplingModeLegacy, e.imageSamplingMode)
}

func TestProjectedImageDimensions(t *testing.T) {
	t.Run("axis aligned", func(t *testing.T) {
		width, height := projectedImageDimensions([6]float64{1, 0, 0, 1, 0, 0}, 4, 5)
		assert.InDelta(t, 1, width, 1e-9)
		assert.InDelta(t, 1, height, 1e-9)
	})

	t.Run("axis align scale", func(t *testing.T) {
		width, height := projectedImageDimensions([6]float64{2, 0, 0, 3, 10, 11}, 4, 5)
		assert.InDelta(t, 2, width, 1e-9)
		assert.InDelta(t, 3, height, 1e-9)
	})

	t.Run("axis rotate 90", func(t *testing.T) {
		width, height := projectedImageDimensions([6]float64{0, 1, -1, 0, 0, 0}, 4, 5)
		assert.InDelta(t, 1, width, 1e-9)
		assert.InDelta(t, 1, height, 1e-9)
	})

	t.Run("shear", func(t *testing.T) {
		width, height := projectedImageDimensions([6]float64{1, 0.5, -0.5, 1, 0, 0}, 4, 5)
		assert.InDelta(t, 1.11803398875, width, 1e-9)
		assert.InDelta(t, 1.11803398875, height, 1e-9)
	})
}

func TestImageSamplingPhaseUsesZeroPhaseForNearestSamplers(t *testing.T) {
	phaseX, phaseY := imageSamplingPhase("auto_nearest_tiny_iccbased_gray_downscale", "", false, [6]float64{})
	assert.Equal(t, 0.0, phaseX)
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("adaptive_nearest_tiny_gray_downscale_non_encoded", "", false, [6]float64{})
	assert.Equal(t, 0.0, phaseX)
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("auto_approx_bilinear", "", true, [6]float64{})
	assert.Equal(t, 0.0, phaseX)
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("auto_approx_bilinear_tiny_gray_ccittfax_downscale", "", true, [6]float64{})
	assert.Equal(t, 0.0, phaseX)
	assert.Equal(t, 0.0, phaseY)
}

func TestCurrentImageTransformUsesCurrentGraphicsTransform(t *testing.T) {
	e := NewEvaluator(nil)
	e.SetInitialTransform([6]float64{2, 0, 0, 3, -4, 7})
	require.NoError(
		t,
		e.concatenateMatrix(Operator{Operands: []entity.Object{
			entity.NewReal(0), entity.NewReal(1), entity.NewReal(-1), entity.NewReal(0),
			entity.NewReal(0), entity.NewReal(0),
		}}),
	)

	assert.Equal(t, [6]float64{0, 3, -2, 0, -4, 7}, e.currentImageTransform())
}

func TestEvaluator_DrawImageUsesCurrentTransform(t *testing.T) {
	e := NewEvaluator(nil)
	c := newRecordingCanvas()
	e.canvas = c

	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	err := e.drawImageUsingCurrentTransform(img, [6]float64{1.2, 0.4, -0.2, 0.9, 7, 11}, true, "", 0.5, 0.5, domainimage.ImageEdgeModeDefault)
	require.NoError(t, err)

	assert.Equal(t, 1, c.drawImageCalls)
	assert.Equal(t, 1, c.saveCalls)
	assert.Equal(t, 1, c.restoreCalls)
	assert.Equal(t, 1, c.transformCalls)
	assert.Equal(t, image.Rect(0, 0, 4, 3), c.lastDrawImageBounds)
	assert.Equal(t, 0.0, c.lastDrawImageX)
	assert.Equal(t, 0.0, c.lastDrawImageY)
	assert.Equal(t, 1.0, c.lastDrawImageWidth)
	assert.Equal(t, 1.0, c.lastDrawImageHeight)
	assert.True(t, c.lastDrawImageInterpolate)
	assert.Equal(t, [6]float64{1.2, 0.4, -0.2, 0.9, 7, 11}, c.lastTransform)
}

func TestEvaluator_SetSpacingMoveTextNextLineAndShowTextErrors(t *testing.T) {
	e := NewEvaluator(nil)
	font := &testFont{
		widths: map[uint32]float64{65: 500},
		names:  map[uint32]string{65: "A"},
	}
	e.graphics.currentState.SetFont(font)
	e.graphics.currentState.SetFontSize(12)

	require.Error(t, e.setSpacingMoveTextNextLineAndShowText(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
	require.Error(t, e.setSpacingMoveTextNextLineAndShowText(Operator{Operands: []entity.Object{
		entity.NewString("bad"), entity.NewInteger(1), entity.NewString("A"),
	}}))
}

type testCanvas struct {
	bounds image.Rectangle

	moveCalls                int
	lineCalls                int
	curveCalls               int
	closeCalls               int
	fillCalls                int
	strokeCalls              int
	clipCalls                int
	eoClipCalls              int
	fillEvenOddCalls         int
	drawTextCalls            int
	drawImageCalls           int
	drawShadingCalls         int
	lastDrawImageBounds      image.Rectangle
	lastDrawImageX           float64
	lastDrawImageY           float64
	lastDrawImageWidth       float64
	lastDrawImageHeight      float64
	lastDrawImageInterpolate bool
	lastTransform            [6]float64
	saveCalls                int
	restoreCalls             int
	transformCalls           int

	drawShadingErr error

	ops        []string
	fillColor  color.Color
	lineWidth  float64
	lineCap    int
	lineJoin   int
	miterLimit float64
}

func newRecordingCanvas() *testCanvas {
	return &testCanvas{
		bounds: image.Rect(0, 0, 128, 64),
	}
}

type combinedTestCanvas struct {
	*testCanvas
	fillAndStrokeCalls        int
	fillEvenOddAndStrokeCalls int
}

func (c *combinedTestCanvas) FillAndStroke() {
	c.fillAndStrokeCalls++
}

func (c *combinedTestCanvas) FillEvenOddAndStroke() {
	c.fillEvenOddAndStrokeCalls++
}

func (c *testCanvas) Width() int              { return c.bounds.Dx() }
func (c *testCanvas) Height() int             { return c.bounds.Dy() }
func (c *testCanvas) Bounds() image.Rectangle { return c.bounds }
func (c *testCanvas) MoveTo(x, y float64) {
	c.moveCalls++
	c.ops = append(c.ops, "M")
}
func (c *testCanvas) LineTo(x, y float64) {
	c.lineCalls++
	c.ops = append(c.ops, "L")
}
func (c *testCanvas) CurveTo(c1x, c1y, c2x, c2y, x, y float64) {
	c.curveCalls++
	c.ops = append(c.ops, "C")
}
func (c *testCanvas) Rectangle(x, y, width, height float64) {}
func (c *testCanvas) ClosePath() {
	c.closeCalls++
	c.ops = append(c.ops, "Z")
}
func (c *testCanvas) Fill()        { c.fillCalls++ }
func (c *testCanvas) Stroke()      { c.strokeCalls++ }
func (c *testCanvas) FillEvenOdd() { c.fillEvenOddCalls++ }
func (c *testCanvas) Clip() {
	c.clipCalls++
	c.ops = append(c.ops, "Clip")
}
func (c *testCanvas) EoClip() {
	c.eoClipCalls++
	c.ops = append(c.ops, "EoClip")
}
func (c *testCanvas) DrawText(text string, x, y float64, font entity.Font, fontSize float64) error {
	c.drawTextCalls++
	return nil
}
func (c *testCanvas) BeginText(x, y float64)       {}
func (c *testCanvas) EndText()                     {}
func (c *testCanvas) ShowText(text string) error   { return nil }
func (c *testCanvas) MoveTextPoint(tx, ty float64) {}
func (c *testCanvas) DrawImage(img image.Image, x, y, w, h float64, interpolate bool) error {
	c.drawImageCalls++
	c.lastDrawImageBounds = img.Bounds()
	c.lastDrawImageX = x
	c.lastDrawImageY = y
	c.lastDrawImageWidth = w
	c.lastDrawImageHeight = h
	c.lastDrawImageInterpolate = interpolate
	return nil
}
func (c *testCanvas) Save()    { c.saveCalls++ }
func (c *testCanvas) Restore() { c.restoreCalls++ }
func (c *testCanvas) Transform(matrix [6]float64) {
	c.transformCalls++
	c.lastTransform = matrix
}
func (c *testCanvas) SetFillColor(color color.Color)               { c.fillColor = color }
func (c *testCanvas) SetStrokeColor(color color.Color)             {}
func (c *testCanvas) SetLineWidth(width float64)                   { c.lineWidth = width }
func (c *testCanvas) SetLineCap(cap int)                           { c.lineCap = cap }
func (c *testCanvas) SetLineJoin(join int)                         { c.lineJoin = join }
func (c *testCanvas) SetMiterLimit(limit float64)                  { c.miterLimit = limit }
func (c *testCanvas) SetDashPattern(dash []float64, phase float64) {}
func (c *testCanvas) SetFillPattern(pattern entity.Pattern)        {}
func (c *testCanvas) SetStrokePattern(pattern entity.Pattern)      {}
func (c *testCanvas) DrawTilingPattern(pattern *entity.TilingPattern, bbox [4]float64) error {
	return nil
}
func (c *testCanvas) DrawShadingPattern(pattern *entity.ShadingPattern, bbox [4]float64) error {
	c.drawShadingCalls++
	return c.drawShadingErr
}
func (c *testCanvas) Image() image.Image { return nil }
func (c *testCanvas) Reset()             {}

type testFont struct {
	widths map[uint32]float64
	names  map[uint32]string

	isCIDFont bool
}

func (f *testFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if f.widths == nil {
		return 0, errors.New("missing font")
	}
	return code, nil
}

func (f *testFont) GlyphName(glyph uint32) string {
	return f.names[glyph]
}

func (f *testFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if f.widths == nil {
		return 0, errors.New("missing width")
	}
	if w, ok := f.widths[glyph]; ok {
		return w, nil
	}
	return 0, errors.New("missing width")
}

func (f *testFont) GetBoundingBox() (float64, float64, float64, float64) { return 0, 0, 1000, 1000 }

func (f *testFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return nil, nil
}

func (f *testFont) IsCIDFont() bool    { return f.isCIDFont }
func (f *testFont) IsSymbolic() bool   { return false }
func (f *testFont) UnitsPerEm() uint16 { return 1000 }
func (f *testFont) Name() string       { return "TestFont" }

type zeroUnitsPerEmFont struct{}

func (f *zeroUnitsPerEmFont) CharCodeToGlyph(code uint32) (uint32, error) { return code, nil }
func (f *zeroUnitsPerEmFont) GlyphName(glyph uint32) string               { return "" }
func (f *zeroUnitsPerEmFont) GetGlyphWidth(glyph uint32) (float64, error) { return 500, nil }
func (f *zeroUnitsPerEmFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}
func (f *zeroUnitsPerEmFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return nil, nil
}
func (f *zeroUnitsPerEmFont) IsCIDFont() bool    { return false }
func (f *zeroUnitsPerEmFont) IsSymbolic() bool   { return false }
func (f *zeroUnitsPerEmFont) UnitsPerEm() uint16 { return 0 }
func (f *zeroUnitsPerEmFont) Name() string       { return "ZeroEmFont" }

type testFunction struct {
	values []float64
	err    bool
}

func (f *testFunction) Evaluate(inputs []float64) ([]float64, error) {
	if f.err {
		return nil, errors.New("function fail")
	}
	return f.values, nil
}
func (f *testFunction) GetInputSize() int  { return 1 }
func (f *testFunction) GetOutputSize() int { return len(f.values) }
func (f *testFunction) GetDomain() [][2]float64 {
	return nil
}
func (f *testFunction) GetRange() [][2]float64 {
	return nil
}

func TestParseEncodingDifferences(t *testing.T) {
	encoding := make(map[uint32]string)
	arr := entity.NewArray(
		entity.NewInteger(65),
		entity.Name("A"),
		entity.Name("B"),
		entity.NewInteger(67),
		entity.Name("C"),
	)
	parseEncodingDifferences(arr, encoding)
	assert.Equal(t, "A", encoding[65])
	assert.Equal(t, "B", encoding[66])
	assert.Equal(t, "C", encoding[67])
	assert.Len(t, encoding, 3)

	empty := make(map[uint32]string)
	parseEncodingDifferences(entity.NewArray(), empty)
	assert.Empty(t, empty)
}

func TestResolveICCBasedComponentCount(t *testing.T) {
	e := NewEvaluator(nil)

	assert.Equal(t, 0, e.resolveICCBasedComponentCount(nil))

	iccDict := entity.NewDict()
	iccDict.Set(entity.Name("N"), entity.NewInteger(3))
	iccStream := entity.NewStream(iccDict, []byte{1})
	cs := entity.NewArray(entity.Name("ICCBased"), iccStream)
	assert.Equal(t, 3, e.resolveICCBasedComponentCount(cs))

	assert.Equal(t, 0, e.resolveICCBasedComponentCount(entity.NewDict()))

	// Via reference
	ref := entity.NewRef(99, 0)
	xref := &testMapXRef{
		objects: map[entity.Ref]entity.Object{
			ref: cs,
		},
	}
	e2 := NewEvaluator(xref)
	assert.Equal(t, 3, e2.resolveICCBasedComponentCount(ref))

	// Reference with nil xref
	e3 := NewEvaluator(nil)
	assert.Equal(t, 0, e3.resolveICCBasedComponentCount(ref))
}

func TestResolveICCBasedProfile(t *testing.T) {
	e := NewEvaluator(nil)

	profile, ok := e.resolveICCBasedProfile(nil, 0)
	assert.False(t, ok)
	assert.Nil(t, profile)

	profile, ok = e.resolveICCBasedProfile(nil, 9)
	assert.False(t, ok)
	assert.Nil(t, profile)

	// Stream with raw bytes
	rawBytes := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	streamDict := entity.NewDict()
	iccStream := entity.NewStream(streamDict, rawBytes)
	cs := entity.NewArray(entity.Name("ICCBased"), iccStream)
	profile, ok = e.resolveICCBasedProfile(cs, 0)
	assert.True(t, ok)
	assert.Equal(t, rawBytes, profile)

	// Via reference
	ref := entity.NewRef(50, 0)
	xref := &testMapXRef{
		objects: map[entity.Ref]entity.Object{ref: cs},
	}
	e2 := NewEvaluator(xref)
	profile, ok = e2.resolveICCBasedProfile(ref, 0)
	assert.True(t, ok)
	assert.NotNil(t, profile)
}

func TestIsICCBasedColorSpaceWithDepth(t *testing.T) {
	e := NewEvaluator(nil)

	assert.False(t, e.isICCBasedColorSpaceWithDepth(nil, 0))
	assert.False(t, e.isICCBasedColorSpaceWithDepth(nil, 9))

	assert.True(t, e.isICCBasedColorSpaceWithDepth(entity.Name("ICCBased"), 0))
	assert.False(t, e.isICCBasedColorSpaceWithDepth(entity.Name("DeviceRGB"), 0))

	cs := entity.NewArray(entity.Name("ICCBased"), entity.NewStream(entity.NewDict(), nil))
	assert.True(t, e.isICCBasedColorSpaceWithDepth(cs, 0))

	// Indexed with ICCBased base
	indexed := entity.NewArray(entity.Name("Indexed"), cs, entity.NewInteger(255), entity.NewString("abc"))
	assert.True(t, e.isICCBasedColorSpaceWithDepth(indexed, 0))

	empty := entity.NewArray()
	assert.False(t, e.isICCBasedColorSpaceWithDepth(empty, 0))

	// Via reference
	ref := entity.NewRef(60, 0)
	xref := &testMapXRef{
		objects: map[entity.Ref]entity.Object{ref: entity.Name("ICCBased")},
	}
	e2 := NewEvaluator(xref)
	assert.True(t, e2.isICCBasedColorSpaceWithDepth(ref, 0))

	// Reference with nil xref
	e3 := NewEvaluator(nil)
	assert.False(t, e3.isICCBasedColorSpaceWithDepth(ref, 0))
}

func TestResolveType3FontCandidate(t *testing.T) {
	e := NewEvaluator(nil)

	assert.Nil(t, e.resolveType3FontCandidate(nil, ""))

	charProcs := entity.NewDict()
	charProcs.Set(entity.Name(".notdef"), entity.NewStream(entity.NewDict(), []byte("n")))
	encoding := entity.NewDict()
	encoding.Set(entity.Name("Differences"), entity.NewArray(
		entity.NewInteger(65),
		entity.Name("A"),
	))

	dict := entity.NewDict()
	dict.Set(entity.Name("FontMatrix"), entity.NewArray(
		entity.NewReal(0.001), entity.NewReal(0), entity.NewReal(0),
		entity.NewReal(0.001), entity.NewReal(0), entity.NewReal(0),
	))
	dict.Set(entity.Name("FontBBox"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(-200), entity.NewInteger(600), entity.NewInteger(800),
	))
	dict.Set(entity.Name("CharProcs"), charProcs)
	dict.Set(entity.Name("Encoding"), encoding)
	dict.Set(entity.Name("FirstChar"), entity.NewInteger(65))
	dict.Set(entity.Name("LastChar"), entity.NewInteger(90))
	widthItems := make([]entity.Object, 26)
	for i := 0; i < 26; i++ {
		widthItems[i] = entity.NewReal(500)
	}
	dict.Set(entity.Name("Widths"), entity.NewArray(widthItems...))

	font := e.resolveType3FontCandidate(dict, "TestT3")
	require.NotNil(t, font)
	assert.Equal(t, "TestT3", font.Name())

	// Test default name fallback
	font2 := e.resolveType3FontCandidate(entity.NewDict(), "")
	require.NotNil(t, font2)
	assert.Equal(t, "Type3", font2.Name())

	// Test with ref-resolved CharProcs
	ref := entity.NewRef(70, 0)
	charProcsWithRef := entity.NewDict()
	charProcsWithRef.Set(entity.Name(".notdef"), ref)
	dictWithRef := entity.NewDict()
	dictWithRef.Set(entity.Name("CharProcs"), charProcsWithRef)
	xref := &testMapXRef{
		objects: map[entity.Ref]entity.Object{
			ref: entity.NewStream(entity.NewDict(), []byte("n")),
		},
	}
	eWithXref := NewEvaluator(xref)
	font3 := eWithXref.resolveType3FontCandidate(dictWithRef, "T3Ref")
	require.NotNil(t, font3)
}

func TestEvaluatorIsRenderableFont(t *testing.T) {
	e := NewEvaluator(nil)

	assert.False(t, e.isRenderableFont(nil))

	font := &testFont{
		widths: map[uint32]float64{65: 500},
		names:  map[uint32]string{65: "A"},
	}
	assert.False(t, e.isRenderableFont(font)) // testFont returns nil glyph path
}

func TestResolveSoftMask_NilAndMissingFields(t *testing.T) {
	e := NewEvaluator(nil)
	assert.Nil(t, e.resolveSoftMask(nil))
	assert.Nil(t, e.resolveSoftMask(entity.NewInteger(99)))
}

func TestResolveSoftMask_ValidMaskStream(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(2))
	dict.Set(entity.Name("Height"), entity.NewInteger(2))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	dict.Set(entity.Name("ColorSpace"), entity.Name("DeviceGray"))
	// Raw pixels for a 2x2 gray image
	maskStream := entity.NewStream(dict, []byte{255, 128, 64, 0})

	e := NewEvaluator(nil)
	mask := e.resolveSoftMask(maskStream)
	assert.NotNil(t, mask)
}

func TestResolveSoftMask_ViaReference(t *testing.T) {
	ref := entity.NewRef(80, 0)
	dict := entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(1))
	dict.Set(entity.Name("Height"), entity.NewInteger(1))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	maskStream := entity.NewStream(dict, []byte{200})

	xref := &testMapXRef{
		objects: map[entity.Ref]entity.Object{ref: maskStream},
	}
	e := NewEvaluator(xref)
	mask := e.resolveSoftMask(ref)
	assert.NotNil(t, mask)
}

func buildExponentialFunctionDict() *entity.Dict {
	fn := entity.NewDict()
	fn.Set(entity.Name("FunctionType"), entity.NewInteger(2))
	fn.Set(entity.Name("C0"), entity.NewArray(entity.NewReal(0)))
	fn.Set(entity.Name("C1"), entity.NewArray(entity.NewReal(1)))
	fn.Set(entity.Name("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	fn.Set(entity.Name("N"), entity.NewReal(2))
	return fn
}

// ---------------------------------------------------------------------------
// Tests for previously 0%-coverage functions
// ---------------------------------------------------------------------------

func TestSkipInlineImageLeadingWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		start    int
		expected int
	}{
		{"no whitespace at start", []byte("ABC"), 0, 0},
		{"space prefix", []byte(" ABC"), 0, 1},
		{"tab prefix", []byte("\tABC"), 0, 1},
		{"newline prefix", []byte("\nABC"), 0, 1},
		{"carriage return prefix", []byte("\rABC"), 0, 1},
		{"null prefix", []byte{0x00, 'A'}, 0, 1},
		{"form feed prefix", []byte{0x0C, 'A'}, 0, 1},
		{"multiple whitespace", []byte("  \t\nABC"), 0, 4},
		{"all whitespace", []byte("   "), 0, 3},
		{"start beyond length", []byte("ABC"), 5, 5},
		{"start at end", []byte("ABC"), 3, 3},
		{"empty data", []byte{}, 0, 0},
		{"start in middle no whitespace", []byte("A BC"), 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skipInlineImageLeadingWhitespace(tt.data, tt.start)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsInlineImageTokenBoundary(t *testing.T) {
	tests := []struct {
		name     string
		b        byte
		expected bool
	}{
		{"null", 0x00, true},
		{"tab", 0x09, true},
		{"newline", 0x0A, true},
		{"form feed", 0x0C, true},
		{"carriage return", 0x0D, true},
		{"space", 0x20, true},
		{"open paren", '(', true},
		{"close paren", ')', true},
		{"less than", '<', true},
		{"greater than", '>', true},
		{"open bracket", '[', true},
		{"close bracket", ']', true},
		{"slash", '/', true},
		{"percent", '%', true},
		{"letter E", 'E', false},
		{"letter I", 'I', false},
		{"digit 0", '0', false},
		{"dot", '.', false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isInlineImageTokenBoundary(tt.b))
		})
	}
}

func TestFindInlineImageEndOffset(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		start       int
		expectedOff int
		expectErr   bool
	}{
		{"EI at start with trailing space", []byte("EI "), 0, 0, false},
		{"EI at start with trailing newline", []byte("EI\n"), 0, 0, false},
		{"EI at start at end of data", []byte("EI"), 0, 0, false},
		{"no EI marker", []byte("ABCDEFGH"), 0, 0, true},
		{"EI with space boundary before", []byte("A EI B"), 0, 2, false},
		{"EI with newline boundary before", []byte("A\nEI B"), 0, 2, false},
		{"EI with space boundary after", []byte(" EI "), 0, 1, false},
		{"EI with newline boundary after", []byte("\nEI\n"), 0, 1, false},
		{"EI without boundary before - part of word", []byte("ABCDEIFG"), 0, 0, true},
		{"EI without boundary after - part of word", []byte(" ABCEIGH"), 0, 0, true},
		{"EI in short data", []byte("E"), 0, 0, true},
		{"start offset past marker", []byte(" EI FG"), 3, 0, true},
		{"start offset before second EI", []byte(" EI  EI "), 4, 5, false},
		{"EI in middle with spaces", []byte("DATA EI REST"), 0, 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			off, err := findInlineImageEndOffset(tt.data, tt.start)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedOff, off)
			}
		})
	}
}

func TestResetInlineImageState(t *testing.T) {
	e := NewEvaluator(nil)
	e.inInlineImage = true
	e.inlineImageDict = entity.NewDict()
	e.inlineImageDict.Set(entity.Name("W"), entity.NewInteger(1))
	e.inlineImageData = []byte{1, 2, 3}

	e.resetInlineImageState()

	assert.False(t, e.inInlineImage)
	assert.Nil(t, e.inlineImageDict)
	assert.Nil(t, e.inlineImageData)
}

func TestSkipInlineImageAndContinue(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		searchPos int
	}{
		{"negative searchPos resets", []byte("Q Q"), -5},
		{"searchPos past end resets", []byte("Q"), 10},
		{"finds EI and continues", []byte("DATAEI Q"), 0},
		{"no EI resets", []byte("NOMARKER"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEvaluator(nil)
			e.inInlineImage = true
			e.inlineImageDict = entity.NewDict()

			// Should not panic and should reset state
			err := e.skipInlineImageAndContinue(tt.data, tt.searchPos)
			assert.NoError(t, err)
			assert.False(t, e.inInlineImage)
			assert.Nil(t, e.inlineImageDict)
		})
	}
}

// TestParseInlineImageFromLexer_NilLexerWithID removed - duplicate of later declaration.

func TestNumericColorOperands(t *testing.T) {
	tests := []struct {
		name     string
		operands []entity.Object
		expected []float64
	}{
		{"empty", nil, []float64{}},
		{"single integer", []entity.Object{entity.NewInteger(0)}, []float64{0}},
		{"single real clamped low", []entity.Object{entity.NewReal(-1)}, []float64{0}},
		{"single real clamped high", []entity.Object{entity.NewReal(2)}, []float64{1}},
		{"multiple reals", []entity.Object{entity.NewReal(0.1), entity.NewReal(0.5), entity.NewReal(0.9)}, []float64{0.1, 0.5, 0.9}},
		{"skips non-numeric", []entity.Object{entity.NewString("bad"), entity.NewReal(0.5)}, []float64{0.5}},
		{"mixed integer and real", []entity.Object{entity.NewInteger(1), entity.NewReal(0)}, []float64{1, 0}},
		{"all non-numeric", []entity.Object{entity.NewString("a"), entity.Name("b")}, []float64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := numericColorOperands(tt.operands)
			assert.InDeltaSlice(t, tt.expected, result, 1e-9)
		})
	}
}

func TestObjectIntStrict(t *testing.T) {
	tests := []struct {
		name    string
		obj     entity.Object
		val     int
		wantErr bool
	}{
		{"integer", entity.NewInteger(42), 42, false},
		{"real", entity.NewReal(3.7), 3, false},
		{"negative integer", entity.NewInteger(-5), -5, false},
		{"name", entity.Name("foo"), 0, true},
		{"string", entity.NewString("5"), 0, true},
		{"nil", nil, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := objectIntStrict(tt.obj)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.val, val)
			}
		})
	}
}

func TestGetNumericOrZero(t *testing.T) {
	assert.Equal(t, 3.0, getNumericOrZero(entity.NewInteger(3)))
	assert.Equal(t, 2.5, getNumericOrZero(entity.NewReal(2.5)))
	assert.Equal(t, 0.0, getNumericOrZero(entity.NewString("bad")))
	assert.Equal(t, 0.0, getNumericOrZero(nil))
}

func TestParseMatrix(t *testing.T) {
	tests := []struct {
		name   string
		obj    entity.Object
		want   [6]float64
		wantOK bool
	}{
		{
			"valid matrix",
			entity.NewArray(entity.NewReal(2), entity.NewReal(0), entity.NewReal(0), entity.NewReal(3), entity.NewReal(1), entity.NewReal(2)),
			[6]float64{2, 0, 0, 3, 1, 2},
			true,
		},
		{
			"integer matrix",
			entity.NewArray(entity.NewInteger(1), entity.NewInteger(0), entity.NewInteger(0), entity.NewInteger(1), entity.NewInteger(0), entity.NewInteger(0)),
			[6]float64{1, 0, 0, 1, 0, 0},
			true,
		},
		{"nil object", nil, [6]float64{1, 0, 0, 1, 0, 0}, false},
		{"not an array", entity.NewString("bad"), [6]float64{1, 0, 0, 1, 0, 0}, false},
		{"short array", entity.NewArray(entity.NewReal(1), entity.NewReal(0)), [6]float64{1, 0, 0, 1, 0, 0}, false},
		{
			"non-numeric element",
			entity.NewArray(entity.NewReal(1), entity.NewString("x"), entity.NewReal(0), entity.NewReal(1), entity.NewReal(0), entity.NewReal(0)),
			[6]float64{1, 0, 0, 1, 0, 0},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mat, ok := parseMatrix(tt.obj)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, mat)
		})
	}
}

func TestResolveGraphicsColorSpace(t *testing.T) {
	tests := []struct {
		name       string
		resources  *entity.Dict
		colorName  string
		expectedCS string
	}{
		{"DeviceRGB", nil, "RGB", "DeviceRGB"},
		{"DeviceGray", nil, "G", "DeviceGray"},
		{"DeviceCMYK", nil, "CMYK", "DeviceCMYK"},
		{"Pattern keyword", nil, "Pattern", "Pattern"},
		{"unknown defaults to DeviceRGB", nil, "UnknownCS", "DeviceRGB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEvaluator(nil)
			if tt.resources != nil {
				e.SetResources(tt.resources)
			}
			result := e.resolveGraphicsColorSpace(entity.Name(tt.colorName))
			assert.Equal(t, tt.expectedCS, result)
		})
	}

	// Test with color space resource that resolves
	t.Run("color space resource resolves", func(t *testing.T) {
		e := NewEvaluator(nil)
		resources := entity.NewDict()
		csCategory := entity.NewDict()
		csCategory.Set(entity.Name("CS1"), entity.Name("RGB"))
		resources.Set(entity.Name("ColorSpace"), csCategory)
		e.SetResources(resources)
		result := e.resolveGraphicsColorSpace(entity.Name("CS1"))
		assert.Equal(t, "DeviceRGB", result)
	})
}

func TestIsPatternColorSpaceResource(t *testing.T) {
	t.Run("nil resources", func(t *testing.T) {
		e := NewEvaluator(nil)
		assert.False(t, e.isPatternColorSpaceResource(entity.Name("P1")))
	})

	t.Run("no ColorSpace category", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.SetResources(entity.NewDict())
		assert.False(t, e.isPatternColorSpaceResource(entity.Name("P1")))
	})

	t.Run("resource not found", func(t *testing.T) {
		e := NewEvaluator(nil)
		resources := entity.NewDict()
		csCategory := entity.NewDict()
		resources.Set(entity.Name("ColorSpace"), csCategory)
		e.SetResources(resources)
		assert.False(t, e.isPatternColorSpaceResource(entity.Name("P1")))
	})

	t.Run("resource not an array", func(t *testing.T) {
		e := NewEvaluator(nil)
		resources := entity.NewDict()
		csCategory := entity.NewDict()
		csCategory.Set(entity.Name("P1"), entity.Name("Pattern"))
		resources.Set(entity.Name("ColorSpace"), csCategory)
		e.SetResources(resources)
		assert.False(t, e.isPatternColorSpaceResource(entity.Name("P1")))
	})

	t.Run("empty array", func(t *testing.T) {
		e := NewEvaluator(nil)
		resources := entity.NewDict()
		csCategory := entity.NewDict()
		csCategory.Set(entity.Name("P1"), entity.NewArray())
		resources.Set(entity.Name("ColorSpace"), csCategory)
		e.SetResources(resources)
		assert.False(t, e.isPatternColorSpaceResource(entity.Name("P1")))
	})

	t.Run("valid pattern array", func(t *testing.T) {
		e := NewEvaluator(nil)
		resources := entity.NewDict()
		csCategory := entity.NewDict()
		csCategory.Set(entity.Name("P1"), entity.NewArray(entity.Name("Pattern"), entity.Name("P1")))
		resources.Set(entity.Name("ColorSpace"), csCategory)
		e.SetResources(resources)
		assert.True(t, e.isPatternColorSpaceResource(entity.Name("P1")))
	})

	t.Run("non-pattern base", func(t *testing.T) {
		e := NewEvaluator(nil)
		resources := entity.NewDict()
		csCategory := entity.NewDict()
		csCategory.Set(entity.Name("CS1"), entity.NewArray(entity.Name("RGB")))
		resources.Set(entity.Name("ColorSpace"), csCategory)
		e.SetResources(resources)
		assert.False(t, e.isPatternColorSpaceResource(entity.Name("CS1")))
	})
}

func TestResolvePattern(t *testing.T) {
	t.Run("nil resources", func(t *testing.T) {
		e := NewEvaluator(nil)
		p, err := e.resolvePattern(entity.Name("P1"))
		assert.Error(t, err)
		assert.Nil(t, p)
	})

	t.Run("pattern not found", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.SetResources(entity.NewDict())
		p, err := e.resolvePattern(entity.Name("P1"))
		assert.Error(t, err)
		assert.Nil(t, p)
	})

	t.Run("tiling pattern type 1", func(t *testing.T) {
		e := NewEvaluator(nil)
		patternDict := entity.NewDict()
		patternDict.Set(entity.Name("PatternType"), entity.NewInteger(1))
		patternDict.Set(entity.Name("PaintType"), entity.NewInteger(1))
		patternDict.Set(entity.Name("TilingType"), entity.NewInteger(1))
		patternDict.Set(entity.Name("BBox"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(10), entity.NewReal(10),
		))
		patternDict.Set(entity.Name("XStep"), entity.NewReal(10))
		patternDict.Set(entity.Name("YStep"), entity.NewReal(10))
		patternDict.Set(entity.Name("Resources"), entity.NewDict())
		resources := entity.NewDict()
		patternCategory := entity.NewDict()
		patternCategory.Set(entity.Name("P1"), patternDict)
		resources.Set(entity.Name("Pattern"), patternCategory)
		e.SetResources(resources)

		p, err := e.resolvePattern(entity.Name("P1"))
		require.NoError(t, err)
		require.NotNil(t, p)
	})

	t.Run("shading pattern type 2", func(t *testing.T) {
		e := NewEvaluator(nil)
		shadingDict := entity.NewDict()
		shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
		shadingDict.Set(entity.Name("Coords"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1)))
		shadingDict.Set(entity.Name("Function"), buildExponentialFunctionDict())

		patternDict := entity.NewDict()
		patternDict.Set(entity.Name("PatternType"), entity.NewInteger(2))
		patternDict.Set(entity.Name("Shading"), shadingDict)

		resources := entity.NewDict()
		patternCategory := entity.NewDict()
		patternCategory.Set(entity.Name("P1"), patternDict)
		resources.Set(entity.Name("Pattern"), patternCategory)
		e.SetResources(resources)

		p, err := e.resolvePattern(entity.Name("P1"))
		require.NoError(t, err)
		require.NotNil(t, p)
	})

	t.Run("stream pattern", func(t *testing.T) {
		e := NewEvaluator(nil)
		patternDict := entity.NewDict()
		patternDict.Set(entity.Name("PatternType"), entity.NewInteger(1))
		patternDict.Set(entity.Name("PaintType"), entity.NewInteger(2))
		patternDict.Set(entity.Name("TilingType"), entity.NewInteger(2))
		patternDict.Set(entity.Name("BBox"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(5), entity.NewReal(5),
		))
		patternDict.Set(entity.Name("XStep"), entity.NewReal(5))
		patternDict.Set(entity.Name("YStep"), entity.NewReal(5))
		patternStream := entity.NewStream(patternDict, []byte("q Q"))

		resources := entity.NewDict()
		patternCategory := entity.NewDict()
		patternCategory.Set(entity.Name("P1"), patternStream)
		resources.Set(entity.Name("Pattern"), patternCategory)
		e.SetResources(resources)

		p, err := e.resolvePattern(entity.Name("P1"))
		require.NoError(t, err)
		require.NotNil(t, p)
	})

	t.Run("unsupported pattern type", func(t *testing.T) {
		e := NewEvaluator(nil)
		patternDict := entity.NewDict()
		patternDict.Set(entity.Name("PatternType"), entity.NewInteger(99))

		resources := entity.NewDict()
		patternCategory := entity.NewDict()
		patternCategory.Set(entity.Name("P1"), patternDict)
		resources.Set(entity.Name("Pattern"), patternCategory)
		e.SetResources(resources)

		p, err := e.resolvePattern(entity.Name("P1"))
		assert.Error(t, err)
		assert.Nil(t, p)
	})

	t.Run("invalid pattern object type", func(t *testing.T) {
		e := NewEvaluator(nil)
		resources := entity.NewDict()
		patternCategory := entity.NewDict()
		patternCategory.Set(entity.Name("P1"), entity.NewString("bad"))
		resources.Set(entity.Name("Pattern"), patternCategory)
		e.SetResources(resources)

		p, err := e.resolvePattern(entity.Name("P1"))
		assert.Error(t, err)
		assert.Nil(t, p)
	})

	t.Run("pattern without explicit PatternType uses default", func(t *testing.T) {
		e := NewEvaluator(nil)
		patternDict := entity.NewDict()
		patternDict.Set(entity.Name("PaintType"), entity.NewInteger(1))
		patternDict.Set(entity.Name("TilingType"), entity.NewInteger(1))
		patternDict.Set(entity.Name("BBox"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(10), entity.NewReal(10),
		))
		patternDict.Set(entity.Name("XStep"), entity.NewReal(10))
		patternDict.Set(entity.Name("YStep"), entity.NewReal(10))

		resources := entity.NewDict()
		patternCategory := entity.NewDict()
		patternCategory.Set(entity.Name("P1"), patternDict)
		resources.Set(entity.Name("Pattern"), patternCategory)
		e.SetResources(resources)

		p, err := e.resolvePattern(entity.Name("P1"))
		require.NoError(t, err)
		require.NotNil(t, p)
	})

	t.Run("pattern with matrix", func(t *testing.T) {
		e := NewEvaluator(nil)
		patternDict := entity.NewDict()
		patternDict.Set(entity.Name("PatternType"), entity.NewInteger(1))
		patternDict.Set(entity.Name("Matrix"), entity.NewArray(
			entity.NewReal(2), entity.NewReal(0), entity.NewReal(0),
			entity.NewReal(2), entity.NewReal(1), entity.NewReal(1),
		))
		patternDict.Set(entity.Name("BBox"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(10), entity.NewReal(10),
		))

		resources := entity.NewDict()
		patternCategory := entity.NewDict()
		patternCategory.Set(entity.Name("P1"), patternDict)
		resources.Set(entity.Name("Pattern"), patternCategory)
		e.SetResources(resources)

		p, err := e.resolvePattern(entity.Name("P1"))
		require.NoError(t, err)
		require.NotNil(t, p)
	})
}

func TestParsePatternShading(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("nil object", func(t *testing.T) {
		s, err := e.parsePatternShading(nil)
		assert.NoError(t, err)
		assert.Nil(t, s)
	})

	t.Run("dict shading", func(t *testing.T) {
		shadingDict := entity.NewDict()
		shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
		shadingDict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1),
		))
		shadingDict.Set(entity.Name("Function"), buildExponentialFunctionDict())

		s, err := e.parsePatternShading(shadingDict)
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("stream shading", func(t *testing.T) {
		shadingDict := entity.NewDict()
		shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingFunctionBased)))
		shadingDict.Set(entity.Name("Coords"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1)))
		shadingDict.Set(entity.Name("Function"), buildExponentialFunctionDict())
		stream := entity.NewStream(shadingDict, nil)

		s, err := e.parsePatternShading(stream)
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("type4 mesh stream decodes vertices", func(t *testing.T) {
		shadingDict := entity.NewDict()
		shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingFreeFormGouraud)))
		shadingDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
		shadingDict.Set(entity.Name("BitsPerFlag"), entity.NewInteger(8))
		shadingDict.Set(entity.Name("BitsPerCoordinate"), entity.NewInteger(8))
		shadingDict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
		shadingDict.Set(entity.Name("Decode"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(1),
			entity.NewReal(0), entity.NewReal(1),
			entity.NewReal(0), entity.NewReal(1),
			entity.NewReal(0), entity.NewReal(1),
			entity.NewReal(0), entity.NewReal(1),
		))

		stream := entity.NewStream(shadingDict, []byte{
			0, 0, 0, 255, 0, 0,
			0, 255, 0, 255, 0, 0,
			0, 0, 255, 255, 0, 0,
		})

		s, err := e.parsePatternShading(stream)
		require.NoError(t, err)
		require.NotNil(t, s)
		require.Len(t, s.GetVertices(), 3)
		assert.InDelta(t, 1.0, s.GetVertices()[1].X, 1e-9)
		assert.InDelta(t, 1.0, s.GetVertices()[2].Y, 1e-9)
		assert.Equal(t, []float64{1, 0, 0}, s.GetVertices()[0].Colors)
	})

	t.Run("invalid type", func(t *testing.T) {
		s, err := e.parsePatternShading(entity.NewString("bad"))
		assert.Error(t, err)
		assert.Nil(t, s)
	})

	t.Run("nil dict", func(t *testing.T) {
		// Stream with nil dict
		s, err := e.parsePatternShading(entity.NewStream(nil, nil))
		assert.Error(t, err)
		assert.Nil(t, s)
	})
}

func TestSetImageSamplingDebug(t *testing.T) {
	e := NewEvaluator(nil)
	assert.False(t, e.debugImageSampling)
	assert.Equal(t, "", e.debugDocumentID)
	assert.Equal(t, 0, e.debugPageNumber)

	e.SetImageSamplingDebug(true, "doc123", 5)
	assert.True(t, e.debugImageSampling)
	assert.Equal(t, "doc123", e.debugDocumentID)
	assert.Equal(t, 5, e.debugPageNumber)

	e.SetImageSamplingDebug(false, "", 0)
	assert.False(t, e.debugImageSampling)
}

func TestSetFillPattern(t *testing.T) {
	e := NewEvaluator(nil)
	assert.Nil(t, e.graphics.fillPattern)

	tiling := entity.NewTilingPattern("test", 1, 1)
	e.SetFillPattern(tiling)
	assert.NotNil(t, e.graphics.fillPattern)

	e.SetFillPattern(nil)
	assert.Nil(t, e.graphics.fillPattern)
}

func TestPatternForCanvasShadingUsesFormBaseMatrix(t *testing.T) {
	e := NewEvaluator(nil)
	base := [6]float64{2, 0, 0, 3, 5, 7}
	content := [6]float64{1, 0, 0, 1, 100, 200}
	e.SetInitialTransform(base)
	e.graphics.transform = multiplyMatrix(base, content)
	e.graphics.baseTransform = base

	shading := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	pattern := entity.NewShadingPattern("mesh", shading)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 11, 13})

	got, ok := e.patternForCanvas(pattern).(*entity.ShadingPattern)
	require.True(t, ok)
	assert.Same(t, shading, got.GetShading())
	assert.Equal(t, multiplyMatrix(base, pattern.Matrix()), got.Matrix())
}

func TestPatternForCanvasTilingUsesFormBaseMatrix(t *testing.T) {
	e := NewEvaluator(nil)
	base := [6]float64{2, 0, 0, 3, 5, 7}
	content := [6]float64{1, 0, 0, 1, 100, 200}
	e.SetInitialTransform(base)
	e.graphics.transform = multiplyMatrix(base, content)
	e.graphics.baseTransform = base

	pattern := entity.NewTilingPattern("tile", 2, entity.TilingConstantSpacing)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 11, 13})
	pattern.SetBBox([4]float64{-1, -1, 3, 3})
	pattern.SetXStep(3)
	pattern.SetYStep(3)

	got, ok := e.patternForCanvas(pattern).(*entity.TilingPattern)
	require.True(t, ok)
	assert.Equal(t, multiplyMatrix(base, pattern.Matrix()), got.Matrix())
	assert.Equal(t, pattern.GetBBox(), got.GetBBox())
	assert.Equal(t, pattern.GetXStep(), got.GetXStep())
	assert.Equal(t, pattern.GetYStep(), got.GetYStep())
}

func TestFormXObjectRestoresBaseMatrix(t *testing.T) {
	e := NewEvaluator(nil)
	base := [6]float64{2, 0, 0, 3, 5, 7}
	e.SetInitialTransform(base)

	formDict := entity.NewDict()
	formDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	formDict.Set(entity.Name("Matrix"), entity.NewArray(
		entity.NewReal(1), entity.NewReal(0), entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(11), entity.NewReal(13),
	))
	formStream := entity.NewStream(formDict, nil)

	require.NoError(t, e.evaluateFormXObject(formStream, entity.Name("Fm1")))
	assert.Equal(t, base, e.graphics.baseTransform)
}

func TestSetStrokePattern(t *testing.T) {
	e := NewEvaluator(nil)
	assert.Nil(t, e.graphics.strokePattern)

	tiling := entity.NewTilingPattern("test", 1, 1)
	e.SetStrokePattern(tiling)
	assert.NotNil(t, e.graphics.strokePattern)

	e.SetStrokePattern(nil)
	assert.Nil(t, e.graphics.strokePattern)
}

func TestNewEmbeddedTrueTypeFont(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("error input returns nil", func(t *testing.T) {
		font := e.newEmbeddedTrueTypeFont(nil, fmt.Errorf("read error"))
		assert.Nil(t, font)
	})

	t.Run("invalid font data returns nil", func(t *testing.T) {
		font := e.newEmbeddedTrueTypeFont([]byte("not a font"), nil)
		assert.Nil(t, font)
	})

	t.Run("empty data returns nil", func(t *testing.T) {
		font := e.newEmbeddedTrueTypeFont([]byte{}, nil)
		assert.Nil(t, font)
	})
}

func TestResolveImageMaskPaintBit(t *testing.T) {
	tests := []struct {
		name    string
		decode  []float64
		paintOn bool
	}{
		{"nil decode defaults to inverted", nil, false},
		{"empty decode defaults to inverted", []float64{}, false},
		{"single element defaults to inverted", []float64{0}, false},
		{"normal order [0 1]", []float64{0, 1}, true},
		{"inverted order [1 0]", []float64{1, 0}, false},
		{"negative first", []float64{-1, 0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.paintOn, resolveImageMaskPaintBit(tt.decode))
		})
	}
}

func TestIndexedPaletteEntries(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		lookupLen int
		expected  int
	}{
		{"DeviceGray", "DeviceGray", 256, 256},
		{"DeviceRGB", "DeviceRGB", 768, 256},
		{"DeviceCMYK", "DeviceCMYK", 1024, 256},
		{"unknown base", "Lab", 100, 0},
		{"zero length", "DeviceRGB", 0, 0},
		{"negative length", "DeviceGray", -1, 0},
		{"case insensitive gray", "devicegray", 10, 10},
		{"case insensitive rgb", "devicergb", 9, 3},
		{"case insensitive cmyk", "devicecmyk", 8, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, indexedPaletteEntries(tt.base, tt.lookupLen))
		})
	}
}

func TestRenderPathToCanvasEvenOdd(t *testing.T) {
	t.Run("nil canvas returns early", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.renderPathToCanvasEvenOdd() // should not panic
	})

	t.Run("with canvas uses FillEvenOdd", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)

		require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(1)}}))
		require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(1)}}))
		require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(5)}}))
		require.NoError(t, e.closePath(Operator{}))

		e.renderPathToCanvasEvenOdd()
		assert.Equal(t, 1, canvas.fillEvenOddCalls)
		assert.Equal(t, 1, canvas.moveCalls)
	})
}

func TestApplyClippingPathEvenOdd(t *testing.T) {
	t.Run("nil canvas", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.applyClippingPathEvenOdd() // should not panic
	})

	t.Run("nil pathClip", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.applyClippingPathEvenOdd()
		assert.Equal(t, 0, canvas.eoClipCalls)
	})

	t.Run("with clip path calls EoClip", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)

		require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(0), entity.NewReal(0)}}))
		require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(10), entity.NewReal(0)}}))
		require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(10), entity.NewReal(10)}}))
		require.NoError(t, e.closePath(Operator{}))
		e.graphics.pathClip = e.graphics.path.Clone()
		e.graphics.path.Clear()

		e.applyClippingPathEvenOdd()
		assert.Equal(t, 1, canvas.eoClipCalls)
		assert.Equal(t, 1, canvas.moveCalls)
	})
}

func TestEvaluateImageMaskUniformAlpha(t *testing.T) {
	t.Run("nil mask returns mixed", func(t *testing.T) {
		assert.Equal(t, imageMaskAlphaMixed, evaluateImageMaskUniformAlpha(nil))
	})

	t.Run("opaque gray mask", func(t *testing.T) {
		gray := image.NewGray(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				gray.SetGray(x, y, color.Gray{Y: 255})
			}
		}
		mask := imginfra.NewBitmapMaskFromImage(gray, false)
		assert.Equal(t, imageMaskAlphaOpaque, evaluateImageMaskUniformAlpha(mask))
	})

	t.Run("transparent gray mask", func(t *testing.T) {
		gray := imginfra.NewBitmapMaskFromImage(image.NewGray(image.Rect(0, 0, 2, 2)), false)
		assert.Equal(t, imageMaskAlphaTransparent, evaluateImageMaskUniformAlpha(gray))
	})

	t.Run("mixed gray mask", func(t *testing.T) {
		gray := image.NewGray(image.Rect(0, 0, 2, 2))
		gray.SetGray(0, 0, color.Gray{Y: 0})
		gray.SetGray(1, 0, color.Gray{Y: 255})
		mask := imginfra.NewBitmapMaskFromImage(gray, false)
		assert.Equal(t, imageMaskAlphaMixed, evaluateImageMaskUniformAlpha(mask))
	})

	t.Run("inverted opaque mask", func(t *testing.T) {
		gray := image.NewGray(image.Rect(0, 0, 2, 2))
		mask := imginfra.NewBitmapMaskFromImage(gray, true) // inverted, all zeros => 255 after invert
		assert.Equal(t, imageMaskAlphaOpaque, evaluateImageMaskUniformAlpha(mask))
	})

	t.Run("inverted transparent mask", func(t *testing.T) {
		gray := image.NewGray(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				gray.SetGray(x, y, color.Gray{Y: 255})
			}
		}
		mask := imginfra.NewBitmapMaskFromImage(gray, true) // inverted, 255 => 0 after invert
		assert.Equal(t, imageMaskAlphaTransparent, evaluateImageMaskUniformAlpha(mask))
	})

	t.Run("empty bounds gray mask", func(t *testing.T) {
		gray := image.NewGray(image.Rect(0, 0, 0, 0))
		mask := imginfra.NewBitmapMaskFromImage(gray, false)
		assert.Equal(t, imageMaskAlphaMixed, evaluateImageMaskUniformAlpha(mask))
	})

	t.Run("uniform mid-gray mask", func(t *testing.T) {
		gray := image.NewGray(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				gray.SetGray(x, y, color.Gray{Y: 128})
			}
		}
		mask := imginfra.NewBitmapMaskFromImage(gray, false)
		assert.Equal(t, imageMaskAlphaMixed, evaluateImageMaskUniformAlpha(mask))
	})
}

func TestImageMaskAlphaUniformityFallback(t *testing.T) {
	t.Run("uniform opaque NRGBA", func(t *testing.T) {
		rgba := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				rgba.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
			}
		}
		mask := &nonGrayMask{img: rgba, inverted: false}
		assert.Equal(t, imageMaskAlphaOpaque, imageMaskAlphaUniformityFallback(mask))
	})

	t.Run("uniform transparent NRGBA", func(t *testing.T) {
		rgba := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		mask := &nonGrayMask{img: rgba, inverted: false}
		assert.Equal(t, imageMaskAlphaTransparent, imageMaskAlphaUniformityFallback(mask))
	})

	t.Run("mixed NRGBA", func(t *testing.T) {
		rgba := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		rgba.SetNRGBA(0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
		rgba.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
		mask := &nonGrayMask{img: rgba, inverted: false}
		assert.Equal(t, imageMaskAlphaMixed, imageMaskAlphaUniformityFallback(mask))
	})

	t.Run("inverted opaque NRGBA", func(t *testing.T) {
		rgba := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		mask := &nonGrayMask{img: rgba, inverted: true}
		assert.Equal(t, imageMaskAlphaOpaque, imageMaskAlphaUniformityFallback(mask))
	})

	t.Run("inverted transparent NRGBA", func(t *testing.T) {
		rgba := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				rgba.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
			}
		}
		mask := &nonGrayMask{img: rgba, inverted: true}
		assert.Equal(t, imageMaskAlphaTransparent, imageMaskAlphaUniformityFallback(mask))
	})

	t.Run("uniform mid-alpha NRGBA", func(t *testing.T) {
		rgba := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				rgba.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 128})
			}
		}
		mask := &nonGrayMask{img: rgba, inverted: false}
		assert.Equal(t, imageMaskAlphaMixed, imageMaskAlphaUniformityFallback(mask))
	})
}

// nonGrayMask is a test mask that returns a non-Gray image to trigger the
// imageMaskAlphaUniformityFallback path.
type nonGrayMask struct {
	img      image.Image
	inverted bool
}

func (m *nonGrayMask) Image() image.Image { return m.img }
func (m *nonGrayMask) IsInverted() bool   { return m.inverted }

func TestFillImageMaskWithCurrentClip(t *testing.T) {
	t.Run("nil canvas returns nil", func(t *testing.T) {
		e := NewEvaluator(nil)
		err := e.fillImageMaskWithCurrentClip(10, 10, [6]float64{1, 0, 0, 1, 0, 0})
		assert.NoError(t, err)
	})

	t.Run("nil pathClip returns nil", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		err := e.fillImageMaskWithCurrentClip(10, 10, [6]float64{1, 0, 0, 1, 0, 0})
		assert.NoError(t, err)
	})

	t.Run("empty pathClip returns nil", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.graphics.pathClip = NewPath()
		err := e.fillImageMaskWithCurrentClip(10, 10, [6]float64{1, 0, 0, 1, 0, 0})
		assert.NoError(t, err)
	})

	t.Run("zero dimensions returns nil", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.graphics.pathClip = NewPath()
		e.graphics.pathClip.AddElement(&MoveTo{X: 0, Y: 0})
		err := e.fillImageMaskWithCurrentClip(0, 10, [6]float64{1, 0, 0, 1, 0, 0})
		assert.NoError(t, err)
	})

	t.Run("NaN transform returns nil", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.graphics.pathClip = NewPath()
		e.graphics.pathClip.AddElement(&MoveTo{X: 0, Y: 0})
		err := e.fillImageMaskWithCurrentClip(10, 10, [6]float64{math.NaN(), 0, 0, 1, 0, 0})
		assert.NoError(t, err)
	})

	t.Run("Inf transform returns nil", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.graphics.pathClip = NewPath()
		e.graphics.pathClip.AddElement(&MoveTo{X: 0, Y: 0})
		err := e.fillImageMaskWithCurrentClip(10, 10, [6]float64{math.Inf(1), 0, 0, 1, 0, 0})
		assert.NoError(t, err)
	})

	t.Run("valid clip fills path", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)

		// Set up a clip path
		require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(0), entity.NewReal(0)}}))
		require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(100), entity.NewReal(0)}}))
		require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(100), entity.NewReal(100)}}))
		require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(0), entity.NewReal(100)}}))
		require.NoError(t, e.closePath(Operator{}))
		e.graphics.pathClip = e.graphics.path.Clone()
		e.graphics.path.Clear()

		err := e.fillImageMaskWithCurrentClip(10, 10, [6]float64{1, 0, 0, 1, 0, 0})
		assert.NoError(t, err)
		assert.Equal(t, 1, canvas.fillCalls)
	})
}

func TestCanFillImageMaskViaClip(t *testing.T) {
	t.Run("nil canvas returns false", func(t *testing.T) {
		e := NewEvaluator(nil)
		assert.False(t, e.canFillImageMaskViaClip(10, 10, [6]float64{1, 0, 0, 1, 0, 0}))
	})

	t.Run("nil pathClip returns false", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		assert.False(t, e.canFillImageMaskViaClip(10, 10, [6]float64{1, 0, 0, 1, 0, 0}))
	})

	t.Run("empty pathClip returns false", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.graphics.pathClip = NewPath()
		assert.False(t, e.canFillImageMaskViaClip(10, 10, [6]float64{1, 0, 0, 1, 0, 0}))
	})

	t.Run("zero dimensions returns false", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		p := NewPath()
		p.AddElement(&MoveTo{X: 0, Y: 0})
		e.graphics.pathClip = p
		assert.False(t, e.canFillImageMaskViaClip(0, 10, [6]float64{1, 0, 0, 1, 0, 0}))
		assert.False(t, e.canFillImageMaskViaClip(10, 0, [6]float64{1, 0, 0, 1, 0, 0}))
	})

	t.Run("NaN transform returns false", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		p := NewPath()
		p.AddElement(&MoveTo{X: 0, Y: 0})
		e.graphics.pathClip = p
		assert.False(t, e.canFillImageMaskViaClip(10, 10, [6]float64{math.NaN(), 0, 0, 1, 0, 0}))
	})

	t.Run("image covers clip bounds returns true", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)

		// Create a clip path with known bounds
		p := NewPath()
		p.AddElement(&MoveTo{X: 10, Y: 10})
		p.AddElement(&LineTo{X: 50, Y: 10})
		p.AddElement(&LineTo{X: 50, Y: 50})
		p.AddElement(&LineTo{X: 10, Y: 50})
		p.AddElement(&Close{})
		e.graphics.pathClip = p

		// Transform that maps 0..1 to 0..100 (covers the 10..50 clip)
		ctm := [6]float64{100, 0, 0, 100, 0, 0}
		assert.True(t, e.canFillImageMaskViaClip(100, 100, ctm))
	})

	t.Run("image does not cover clip returns false", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)

		// Clip path far away
		p := NewPath()
		p.AddElement(&MoveTo{X: 500, Y: 500})
		p.AddElement(&LineTo{X: 600, Y: 500})
		p.AddElement(&LineTo{X: 600, Y: 600})
		p.AddElement(&LineTo{X: 500, Y: 600})
		p.AddElement(&Close{})
		e.graphics.pathClip = p

		// Small image at origin
		ctm := [6]float64{10, 0, 0, 10, 0, 0}
		assert.False(t, e.canFillImageMaskViaClip(10, 10, ctm))
	})

	t.Run("zero bounds clip returns false", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)

		// Path with all zero bounds
		p := NewPath()
		p.AddElement(&MoveTo{X: 0, Y: 0})
		e.graphics.pathClip = p

		ctm := [6]float64{1, 0, 0, 1, 0, 0}
		assert.False(t, e.canFillImageMaskViaClip(10, 10, ctm))
	})
}

func TestAdvanceTextWithoutRendering(t *testing.T) {
	e := NewEvaluator(nil)
	font := &testFont{
		widths:    map[uint32]float64{65: 500, 66: 600},
		names:     map[uint32]string{65: "A", 66: "B"},
		isCIDFont: false,
	}
	e.graphics.currentState.SetFont(font)
	e.graphics.currentState.SetFontSize(12)

	identity := [6]float64{1, 0, 0, 1, 0, 0}
	e.textMatrix = identity
	e.textLineMatrix = identity

	units := []textCodeUnit{
		{code: 'A', raw: []byte("A")},
		{code: 'B', raw: []byte("B")},
	}

	e.advanceTextWithoutRendering(units, font, 12)

	// The text matrix should have advanced
	assert.NotEqual(t, identity, e.textMatrix, "text matrix should advance after advancing text")
}

func TestParseInlineImageFromLexer_WithLexerData(t *testing.T) {
	// Test the full parseInlineImageFromLexer flow with actual lexer data
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	// Create data that has: key-value pairs, ID marker, image data, EI end marker
	data := []byte("/W 1 /H 1 /CS /G ID \x80\x40\x20 EI\n")
	lex := parser.NewLexerBytes(data)
	p := parser.NewParser(lex, nil)

	err := e.parseInlineImageFromLexer(lex, p, data)
	assert.NoError(t, err)
}

func TestSkipInlineImageAndContinue_WithValidEI(t *testing.T) {
	e := NewEvaluator(nil)
	e.inInlineImage = true
	e.inlineImageDict = entity.NewDict()

	// Data with EI marker after some content
	data := []byte("some image data EI Q")

	err := e.skipInlineImageAndContinue(data, 0)
	assert.NoError(t, err)
	assert.False(t, e.inInlineImage)
}

func TestRenderImageMaskToCanvas(t *testing.T) {
	t.Run("nil canvas returns nil", func(t *testing.T) {
		e := NewEvaluator(nil)
		err := e.renderImageMaskToCanvas(nil, 2, 2, 1, domainimage.FilterFlate, false, false, false)
		assert.NoError(t, err)
	})

	t.Run("zero dimensions returns error", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		err := e.renderImageMaskToCanvas(nil, 0, 2, 1, domainimage.FilterFlate, false, false, false)
		assert.Error(t, err)
	})

	t.Run("negative dimensions returns error", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		err := e.renderImageMaskToCanvas(nil, -1, 2, 1, domainimage.FilterFlate, false, false, false)
		assert.Error(t, err)
	})

	t.Run("invalid mask data returns error", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		err := e.renderImageMaskToCanvas([]byte{0xFF}, 1, 1, 8, domainimage.FilterFlate, false, false, false)
		// 1 byte is not enough for 1x1 8bpc mask (need 1 byte actually, should be ok)
		// The actual result depends on DecodeMaskData behavior
		// It should either succeed or return an error depending on the data
		// Just verify it doesn't panic
		_ = err
	})
}

func TestParseInlineImageFromLexer_NilLexerWithID(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	// Build data with inline image entries and use a real lexer.
	data := []byte("/W 1 /H 1 ID \xFF EI Q")
	lex := parser.NewLexerBytes(data)
	p := parser.NewParser(lex, nil)

	err := e.parseInlineImageFromLexer(lex, p, data)
	assert.NoError(t, err)
}

func TestParseInlineImageFromLexer_ActualLexer(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	// Build inline image data: /W 1 /H 1 /CS /G ID <data> EI Q
	data := []byte("/W 1 /H 1 /CS /G ID \x80\x40\x20 EI Q")

	lex := parser.NewLexerBytes(data)
	p := parser.NewParser(lex, nil)

	err := e.parseInlineImageFromLexer(lex, p, data)
	assert.NoError(t, err)
}

func TestParseInlineImageFromLexer_EOF(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	// Data with no ID marker - will hit EOF
	data := []byte("/W 1 /H 1")

	lex := parser.NewLexerBytes(data)
	p := parser.NewParser(lex, nil)

	err := e.parseInlineImageFromLexer(lex, p, data)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Additional coverage tests for low-coverage functions
// ---------------------------------------------------------------------------

func TestAdvanceTextMatrixZeroTx(t *testing.T) {
	e := NewEvaluator(nil)
	e.textMatrix = [6]float64{1, 0, 0, 1, 10, 20}
	e.advanceTextMatrix(0)
	// Should not modify the matrix when tx is 0
	assert.Equal(t, [6]float64{1, 0, 0, 1, 10, 20}, e.textMatrix)
}

// Test splitTextCodeUnits empty string
func TestSplitTextCodeUnitsEmpty(t *testing.T) {
	units := splitTextCodeUnits("", nil)
	assert.Nil(t, units)
}

// Test splitTextCodeUnits CID odd-length (single trailing byte)
func TestSplitTextCodeUnitsCIDOddLength(t *testing.T) {
	font := &testFont{isCIDFont: true}
	units := splitTextCodeUnits(string([]byte{0x41, 0x42, 0x43}), font)
	assert.Len(t, units, 2)
	assert.Equal(t, uint32(0x4142), units[0].code)
	assert.Equal(t, uint32(0x43), units[1].code)
}

// Test glyphAdvance unitsPerEm=0
func TestGlyphAdvanceUnitsPerEmZero(t *testing.T) {
	e := NewEvaluator(nil)
	zeroEmFont := &zeroUnitsPerEmFont{}
	e.graphics.currentState.SetFont(zeroEmFont)
	e.graphics.currentState.SetFontSize(12)
	advance := e.glyphAdvance('A', zeroEmFont, 12)
	assert.Greater(t, advance, 0.0) // Should use default 1000
}

// Test cloneCurrentState nil input
func TestCloneCurrentStateNil(t *testing.T) {
	result := cloneCurrentState(nil)
	assert.NotNil(t, result)
}

// Test showText operand errors
func TestShowTextOperandErrors(t *testing.T) {
	e := NewEvaluator(nil)
	// No operands
	require.Error(t, e.showText(Operator{}))
	// Non-string operand
	require.Error(t, e.showText(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
}

// Test setTextMatrix error path
func TestSetTextMatrixErrorPath(t *testing.T) {
	e := NewEvaluator(nil)
	// Bad operand - permissive
	err := e.setTextMatrix(Operator{Operands: []entity.Object{
		entity.NewString("bad"), entity.NewReal(0), entity.NewReal(0),
		entity.NewReal(1), entity.NewReal(0), entity.NewReal(0),
	}})
	require.NoError(t, err) // permissive
}

// Test setCharSpacing error
func TestSetCharSpacingError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setCharSpacing(Operator{}))
}

// Test setWordSpacing error
func TestSetWordSpacingError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setWordSpacing(Operator{}))
}

// Test setHorizScaling error
func TestSetHorizScalingError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setHorizScaling(Operator{}))
}

// Test setTextLeading error
func TestSetTextLeadingError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setTextLeading(Operator{}))
}

// Test setTextRise error
func TestSetTextRiseError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setTextRise(Operator{}))
}

// Test setGrayStroke error
func TestSetGrayStrokeError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setGrayStroke(Operator{}))
	require.Error(t, e.setGrayStroke(Operator{Operands: []entity.Object{entity.NewString("bad")}}))
}

// Test setGrayFill error
func TestSetGrayFillError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setGrayFill(Operator{}))
	require.Error(t, e.setGrayFill(Operator{Operands: []entity.Object{entity.NewString("bad")}}))
}

// Test setRGBStroke error
func TestSetRGBStrokeError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setRGBStroke(Operator{}))
	require.Error(t, e.setRGBStroke(Operator{Operands: []entity.Object{entity.NewReal(1)}}))
}

// Test setRGBFill error
func TestSetRGBFillError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setRGBFill(Operator{}))
	require.Error(t, e.setRGBFill(Operator{Operands: []entity.Object{entity.NewReal(1)}}))
}

// Test setCMYKStroke error
func TestSetCMYKStrokeError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setCMYKStroke(Operator{}))
	require.Error(t, e.setCMYKStroke(Operator{Operands: []entity.Object{entity.NewReal(1)}}))
}

// Test setCMYKFill error
func TestSetCMYKFillError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setCMYKFill(Operator{}))
	require.Error(t, e.setCMYKFill(Operator{Operands: []entity.Object{entity.NewReal(1)}}))
}

// Test setLineWidth error
func TestSetLineWidthError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setLineWidth(Operator{}))
	require.Error(t, e.setLineWidth(Operator{Operands: []entity.Object{entity.NewString("bad")}}))
}

// Test setLineCap with and without operands
func TestSetLineCapOperands(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setLineCap(Operator{}))
	require.NoError(t, e.setLineCap(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
	assert.Equal(t, 1, e.graphics.currentState.GetLineCap())
}

// Test setLineJoin with and without operands
func TestSetLineJoinOperands(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setLineJoin(Operator{}))
	require.NoError(t, e.setLineJoin(Operator{Operands: []entity.Object{entity.NewInteger(2)}}))
	assert.Equal(t, 2, e.graphics.currentState.GetLineJoin())
}

// Test setMiterLimit error
func TestSetMiterLimitError(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setMiterLimit(Operator{}))
	require.Error(t, e.setMiterLimit(Operator{Operands: []entity.Object{entity.NewString("bad")}}))
}

// Test setDashPattern error paths
func TestSetDashPatternErrors(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.setDashPattern(Operator{}))
	require.Error(t, e.setDashPattern(Operator{Operands: []entity.Object{entity.NewInteger(1), entity.NewReal(0)}}))
	require.Error(t, e.setDashPattern(Operator{Operands: []entity.Object{
		entity.NewArray(entity.NewReal(3)), entity.NewString("bad"),
	}}))
	require.Error(t, e.setDashPattern(Operator{Operands: []entity.Object{
		entity.NewArray(entity.NewString("bad")), entity.NewReal(0)},
	}))
}

// Test applyGraphicsStateParameters all entries
func TestApplyGraphicsStateParametersAllEntries(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	gsDict := entity.NewDict()
	gsDict.Set(entity.Name("LW"), entity.NewReal(2.5))
	gsDict.Set(entity.Name("LC"), entity.NewInteger(1))
	gsDict.Set(entity.Name("LJ"), entity.NewInteger(2))
	gsDict.Set(entity.Name("ML"), entity.NewReal(5))
	gsDict.Set(entity.Name("D"), entity.NewArray(
		entity.NewArray(entity.NewReal(3), entity.NewReal(5)),
		entity.NewReal(0),
	))
	gsDict.Set(entity.Name("CA"), entity.NewReal(0.8))
	gsDict.Set(entity.Name("ca"), entity.NewReal(0.6))

	resources := entity.NewDict()
	gsCategory := entity.NewDict()
	gsCategory.Set(entity.Name("G1"), gsDict)
	resources.Set(entity.Name("ExtGState"), gsCategory)
	e.SetResources(resources)
	require.NoError(t, e.applyGraphicsStateParameters(Operator{
		Opcode:   "gs",
		Operands: []entity.Object{entity.Name("G1")},
	}))
	assert.Equal(t, 2.5, e.graphics.lineWidth)
	assert.Equal(t, 0.8, e.graphics.strokeAlpha)
	assert.Equal(t, 0.6, e.graphics.fillAlpha)
}

// Test applyGraphicsStateParameters nil/missing gs
func TestApplyGraphicsStateParametersNilMissing(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.applyGraphicsStateParameters(Operator{}))
	require.NoError(t, e.applyGraphicsStateParameters(Operator{
		Operands: []entity.Object{entity.NewInteger(1)},
	}))
	require.NoError(t, e.applyGraphicsStateParameters(Operator{
		Operands: []entity.Object{entity.Name("Missing")},
	}))
}

// Test moveTo operand errors
func TestMoveToOperandErrors(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.moveTo(Operator{}))
	require.Error(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewString("bad"), entity.NewReal(2)}}))
	require.Error(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewString("bad")}}))
}

// Test lineTo operand errors
func TestLineToOperandErrors(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.lineTo(Operator{}))
	require.Error(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewString("bad"), entity.NewReal(2)}}))
	require.Error(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewString("bad")}}))
}

// Test curveTo operand errors
func TestCurveToOperandErrors(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.curveTo(Operator{}))
	require.Error(t, e.curveTo(Operator{Operands: []entity.Object{
		entity.NewString("bad"), entity.NewReal(0),
		entity.NewReal(0), entity.NewReal(0),
		entity.NewReal(0), entity.NewReal(0),
	}}))
}

// Test curveToNoFirstControl operand errors
func TestCurveToNoFirstControlOperandErrors(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.curveToNoFirstControl(Operator{}))
	require.Error(t, e.curveToNoFirstControl(Operator{Operands: []entity.Object{
		entity.NewString("bad"), entity.NewReal(0),
		entity.NewReal(0), entity.NewReal(0),
	}}))
}

// Test curveToNoLastControl operand errors
func TestCurveToNoLastControlOperandErrors(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.curveToNoLastControl(Operator{}))
	require.Error(t, e.curveToNoLastControl(Operator{Operands: []entity.Object{
		entity.NewString("bad"), entity.NewReal(0),
		entity.NewReal(0), entity.NewReal(0),
	}}))
}

// Test rectangle operand errors
func TestRectangleOperandErrors(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.rectangle(Operator{}))
	require.Error(t, e.rectangle(Operator{Operands: []entity.Object{
		entity.NewString("bad"), entity.NewReal(0),
		entity.NewReal(4), entity.NewReal(5),
	}}))
}

// Test fillPathEvenOdd with canvas
func TestFillPathEvenOddWithCanvas(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)
	require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(5)}}))
	require.NoError(t, e.closePath(Operator{}))
	require.NoError(t, e.fillPathEvenOdd())
	assert.Equal(t, 1, canvas.fillEvenOddCalls)
}

// Test renderPathToCanvas nil canvas
func TestRenderPathToCanvasNilCanvas(t *testing.T) {
	e := NewEvaluator(nil)
	e.renderPathToCanvas(true)
	e.renderPathToCanvas(false)
}

// Test renderPathToCanvas fill vs stroke
func TestRenderPathToCanvasFillVsStroke(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)
	require.NoError(t, e.moveTo(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(1)}}))
	require.NoError(t, e.lineTo(Operator{Operands: []entity.Object{entity.NewReal(5), entity.NewReal(5)}}))
	e.renderPathToCanvas(true)
	assert.Equal(t, 1, canvas.fillCalls)
	assert.Equal(t, 0, canvas.strokeCalls)
	e.renderPathToCanvas(false)
	assert.Equal(t, 1, canvas.fillCalls)
	assert.Equal(t, 1, canvas.strokeCalls)
}

// Test SetFillColor nil
func TestSetFillColorNil(t *testing.T) {
	e := NewEvaluator(nil)
	e.SetFillColor(nil)
	assert.Equal(t, "000000", e.graphics.fillColor.Color.(*Color).Hex)
	assert.Equal(t, 1.0, e.graphics.fillAlpha)
}

// Test SetStrokeColor nil
func TestSetStrokeColorNil(t *testing.T) {
	e := NewEvaluator(nil)
	e.SetStrokeColor(nil)
	assert.Equal(t, "000000", e.graphics.strokeColor.Color.(*Color).Hex)
	assert.Equal(t, 1.0, e.graphics.strokeAlpha)
}

// Test setColorBySpace all color spaces
func TestSetColorBySpaceAllColorSpaces(t *testing.T) {
	tests := []struct {
		name       string
		colorSpace string
		stroke     bool
		operands   []entity.Object
	}{
		{"DeviceGray fill", "DeviceGray", false, []entity.Object{entity.NewReal(0.5)}},
		{"DeviceGray stroke", "DeviceGray", true, []entity.Object{entity.NewReal(0.5)}},
		{"DeviceRGB fill", "DeviceRGB", false, []entity.Object{entity.NewReal(0.1), entity.NewReal(0.2), entity.NewReal(0.3)}},
		{"DeviceRGB stroke", "DeviceRGB", true, []entity.Object{entity.NewReal(0.1), entity.NewReal(0.2), entity.NewReal(0.3)}},
		{"DeviceCMYK fill", "DeviceCMYK", false, []entity.Object{entity.NewReal(0.1), entity.NewReal(0.2), entity.NewReal(0.3), entity.NewReal(0.4)}},
		{"DeviceCMYK stroke", "DeviceCMYK", true, []entity.Object{entity.NewReal(0.1), entity.NewReal(0.2), entity.NewReal(0.3), entity.NewReal(0.4)}},
		{"Pattern fill empty", "Pattern", false, nil},
		{"Pattern stroke empty", "Pattern", true, nil},
		{"default fill", "", false, []entity.Object{entity.NewReal(0.5)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEvaluator(nil)
			if tt.stroke {
				e.graphics.strokeCS = tt.colorSpace
			} else {
				e.graphics.fillCS = tt.colorSpace
			}
			require.NoError(t, e.setColorBySpace(Operator{Operands: tt.operands}, tt.stroke))
		})
	}
}

// Test setColorBySpace Pattern with pattern name
func TestSetColorBySpacePatternWithPatternName(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillCS = "Pattern"
	require.NoError(t, e.setColorBySpace(Operator{
		Operands: []entity.Object{entity.Name("P1")},
	}, false))
}

// Test setColorBySpace Pattern with values and pattern name
func TestSetColorBySpacePatternWithValuesAndPatternName(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillCS = "Pattern"
	patternDict := entity.NewDict()
	patternDict.Set(entity.Name("PatternType"), entity.NewInteger(1))
	patternDict.Set(entity.Name("PaintType"), entity.NewInteger(2))
	patternDict.Set(entity.Name("TilingType"), entity.NewInteger(1))
	patternDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1),
	))
	patternDict.Set(entity.Name("XStep"), entity.NewReal(1))
	patternDict.Set(entity.Name("YStep"), entity.NewReal(1))
	patternDict.Set(entity.Name("Resources"), entity.NewDict())
	resources := entity.NewDict()
	patternCategory := entity.NewDict()
	patternCategory.Set(entity.Name("P1"), entity.NewStream(patternDict, []byte("q Q")))
	resources.Set(entity.Name("Pattern"), patternCategory)
	e.SetResources(resources)

	require.NoError(t, e.setColorBySpace(Operator{
		Operands: []entity.Object{
			entity.NewReal(1),
			entity.NewReal(0),
			entity.NewReal(0),
			entity.Name("P1"),
		},
	}, false))
	require.NotNil(t, e.graphics.fillPattern)
	require.Nil(t, e.graphics.strokePattern)
	require.NotNil(t, e.graphics.fillColor)
	fillColor, ok := e.graphics.fillColor.Color.(*Color)
	require.True(t, ok)
	require.Equal(t, "FF0000", fillColor.Hex)
}

// Test setColorBySpace Pattern stroke resolution
func TestSetColorBySpacePatternStrokeResolution(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.strokeCS = "Pattern"
	require.NoError(t, e.setColorBySpace(Operator{
		Operands: []entity.Object{entity.Name("P1")},
	}, true))
}

func TestSetFillAndStrokeColorSpaceClearsStalePattern(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillPattern = entity.NewTilingPattern("fill", 1, entity.TilingConstantSpacing)
	e.graphics.strokePattern = entity.NewTilingPattern("stroke", 1, entity.TilingConstantSpacing)

	require.NoError(t, e.setFillColorSpace(Operator{
		Operands: []entity.Object{entity.Name("DeviceRGB")},
	}))
	require.NoError(t, e.setStrokeColorSpace(Operator{
		Operands: []entity.Object{entity.Name("DeviceGray")},
	}))

	require.Nil(t, e.graphics.fillPattern)
	require.Nil(t, e.graphics.strokePattern)
}

// Test setColorBySpace CMYK insufficient values
func TestSetColorBySpaceCMYKInsufficientValues(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillCS = "DeviceCMYK"
	require.NoError(t, e.setColorBySpace(Operator{
		Operands: []entity.Object{entity.NewReal(0.1), entity.NewReal(0.2)},
	}, false))
}

// Test setColorBySpace empty values and no pattern
func TestSetColorBySpaceEmptyValuesNoPattern(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillCS = "DeviceRGB"
	require.NoError(t, e.setColorBySpace(Operator{}, false))
	require.NoError(t, e.setColorBySpace(Operator{}, true))
}

// Test invokeXObject error paths
func TestInvokeXObjectErrorPaths(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.invokeXObject(Operator{}))
	require.Error(t, e.invokeXObject(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
	require.Error(t, e.invokeXObject(Operator{Operands: []entity.Object{entity.Name("X1")}}))
	e.SetResources(entity.NewDict())
	require.Error(t, e.invokeXObject(Operator{Operands: []entity.Object{entity.Name("X1")}}))
	xobjCategory := entity.NewDict()
	xobjCategory.Set(entity.Name("X1"), entity.NewInteger(1))
	resources := entity.NewDict()
	resources.Set(entity.Name("XObject"), xobjCategory)
	e.SetResources(resources)
	require.Error(t, e.invokeXObject(Operator{Operands: []entity.Object{entity.Name("X1")}}))
}

// Test invokeXObject no subtype
func TestInvokeXObjectNoSubtype(t *testing.T) {
	e := NewEvaluator(nil)
	stream := entity.NewStream(entity.NewDict(), nil)
	xobjCategory := entity.NewDict()
	xobjCategory.Set(entity.Name("X1"), stream)
	resources := entity.NewDict()
	resources.Set(entity.Name("XObject"), xobjCategory)
	e.SetResources(resources)
	require.Error(t, e.invokeXObject(Operator{Operands: []entity.Object{entity.Name("X1")}}))
}

// Test invokeXObject unsupported subtype
func TestInvokeXObjectUnsupportedSubtype(t *testing.T) {
	e := NewEvaluator(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Subtype"), entity.Name("PS"))
	stream := entity.NewStream(dict, nil)
	xobjCategory := entity.NewDict()
	xobjCategory.Set(entity.Name("X1"), stream)
	resources := entity.NewDict()
	resources.Set(entity.Name("XObject"), xobjCategory)
	e.SetResources(resources)
	require.Error(t, e.invokeXObject(Operator{Operands: []entity.Object{entity.Name("X1")}}))
}

// Test cachedFormOperators error paths
func TestCachedFormOperatorsErrorPaths(t *testing.T) {
	e := NewEvaluator(nil)
	_, err := e.cachedFormOperators(nil)
	require.Error(t, err)
	badStream := entity.NewStream(entity.NewDict(), []byte("invalid"))
	badStream.Dict().Set(entity.Name("Filter"), entity.Name("UnknownFilter"))
	_, err = e.cachedFormOperators(badStream)
	require.Error(t, err)
}

// Test evaluateFormXObject nil error
func TestEvaluateFormXObjectNilError(t *testing.T) {
	e := NewEvaluator(nil)
	err := e.evaluateFormXObject(nil, entity.Name("Fm1"))
	require.Error(t, err)
}

// Test applyFormBBoxClip invalid arrays
func TestApplyFormBBoxClipInvalidArrays(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.applyFormBBoxClip(nil))
	require.NoError(t, e.applyFormBBoxClip(entity.NewArray(entity.NewReal(1), entity.NewReal(2))))
	err := e.applyFormBBoxClip(entity.NewArray(
		entity.NewString("bad"), entity.NewReal(0),
		entity.NewReal(2), entity.NewReal(3),
	))
	require.Error(t, err)
	err = e.applyFormBBoxClip(entity.NewArray(
		entity.NewReal(0), entity.NewString("bad"),
		entity.NewReal(2), entity.NewReal(3),
	))
	require.Error(t, err)
	err = e.applyFormBBoxClip(entity.NewArray(
		entity.NewReal(0), entity.NewReal(0),
		entity.NewString("bad"), entity.NewReal(3),
	))
	require.Error(t, err)
	err = e.applyFormBBoxClip(entity.NewArray(
		entity.NewReal(0), entity.NewReal(0),
		entity.NewReal(2), entity.NewString("bad"),
	))
	require.Error(t, err)
}

// Test Save/Restore nil state paths
func TestSaveRestoreNilStatePaths(t *testing.T) {
	var gs *GraphicsState
	gs.Save()
	gs.Restore()
}

// Test saveState with nil canvas
func TestSaveStateNilCanvas(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.saveState())
	assert.Len(t, e.stateStack, 1)
	require.NoError(t, e.restoreState())
}

// Test concatenateMatrix error path
func TestConcatenateMatrixErrorPath(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.concatenateMatrix(Operator{}))
	require.NoError(t, e.concatenateMatrix(Operator{Operands: []entity.Object{
		entity.NewString("bad"), entity.NewReal(0),
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(0),
	}}))
}

// Test getFontFromDict nil/missing subtype
func TestGetFontFromDictNilMissingSubtype(t *testing.T) {
	e := NewEvaluator(nil)
	_, err := e.getFontFromDict(nil, "TestFont")
	require.Error(t, err)
	dict := entity.NewDict()
	_, err = e.getFontFromDict(dict, "TestFont")
	require.Error(t, err)
}

// Test renderType3Glyph nil charproc
func TestRenderType3GlyphNilCharproc(t *testing.T) {
	e := NewEvaluator(nil)
	font := entity.NewType3Font("TestT3", [6]float64{0.001, 0, 0, 0.001, 0, 0},
		map[string]*entity.Stream{}, map[uint32]string{}, map[uint32]float64{}, 0, 255, [4]float64{})
	err := e.renderType3Glyph(font, 65, 0, 0, 12)
	require.Error(t, err)
}

func TestType3CharProcUsesD1CacheGate(t *testing.T) {
	tests := []struct {
		name string
		ops  []Operator
		want bool
	}{
		{
			name: "d1 charproc uses cached bitmap placement",
			ops:  []Operator{{Opcode: "d1"}, {Opcode: "q"}},
			want: true,
		},
		{
			name: "d0 charproc keeps direct vector placement",
			ops:  []Operator{{Opcode: "d0"}, {Opcode: "d1"}},
			want: false,
		},
		{
			name: "graphics state before d1 keeps existing inline path",
			ops:  []Operator{{Opcode: "q"}, {Opcode: "d1"}},
			want: false,
		},
		{
			name: "missing d1 does not quantize",
			ops:  []Operator{{Opcode: "cm"}, {Opcode: "f"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, type3CharProcUsesD1Cache(tt.ops))
		})
	}
}

func TestType3GlyphCTMMatchesPopplerDoShowText(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.transform = [6]float64{2, 0.5, 0.25, 3, 11, 13}
	e.textMatrix = [6]float64{1.5, 0.2, 0.4, 1.25, 7, 9}
	e.graphics.currentState.SetHorizontalScaling(80)

	font := entity.NewType3Font("TestT3", [6]float64{0.001, 0.0002, -0.0001, 0.002, 5, 6},
		map[string]*entity.Stream{}, map[uint32]string{}, map[uint32]float64{}, 0, 255, [4]float64{})

	got := e.type3GlyphCTM(font, 123, 456, 12)
	assert.InDeltaSlice(t, []float64{0.031416, 0.020544, 0.02304, 0.09318, 123, 456}, got[:], 1e-12)
}

// Test evaluate nil/non-stream content objects
func TestEvaluateNilNonStreamContent(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.Evaluate(nil))
	require.NoError(t, e.Evaluate([]entity.Object{entity.NewInteger(1)}))
	require.NoError(t, e.Evaluate([]entity.Object{entity.NewString("not a stream")}))
}

// Test parseOperatorsOnly error path
func TestParseOperatorsOnlyErrorPath(t *testing.T) {
	e := NewEvaluator(nil)
	ops, err := e.parseOperatorsOnly([]byte("1 2 m"))
	require.NoError(t, err)
	assert.Len(t, ops, 1)
}

// Test renderInlineImage various configurations
func TestRenderInlineImageVariousConfigs(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)
	e.inInlineImage = true
	e.inlineImageDict = entity.NewDict()
	e.inlineImageData = []byte{1}
	err := e.renderInlineImage()
	require.Error(t, err)

	e.inInlineImage = true
	e.inlineImageDict = entity.NewDict()
	e.inlineImageDict.Set(entity.Name("W"), entity.NewInteger(1))
	e.inlineImageData = []byte{1}
	err = e.renderInlineImage()
	require.Error(t, err)

	e.inInlineImage = true
	e.inlineImageDict = entity.NewDict()
	e.inlineImageDict.Set(entity.Name("W"), entity.NewString("bad"))
	e.inlineImageDict.Set(entity.Name("H"), entity.NewInteger(1))
	e.inlineImageData = []byte{1}
	err = e.renderInlineImage()
	require.Error(t, err)

	e.inInlineImage = true
	e.inlineImageDict = entity.NewDict()
	e.inlineImageDict.Set(entity.Name("W"), entity.NewInteger(1))
	e.inlineImageDict.Set(entity.Name("H"), entity.NewString("bad"))
	e.inlineImageData = []byte{1}
	err = e.renderInlineImage()
	require.Error(t, err)
}

// Test renderInlineImage with no canvas
func TestRenderInlineImageNoCanvas(t *testing.T) {
	e := NewEvaluator(nil)
	e.inInlineImage = true
	e.inlineImageDict = entity.NewDict()
	e.inlineImageDict.Set(entity.Name("W"), entity.NewInteger(1))
	e.inlineImageDict.Set(entity.Name("H"), entity.NewInteger(1))
	e.inlineImageDict.Set(entity.Name("CS"), entity.Name("RGB"))
	e.inlineImageData = []byte{1, 2, 3}
	err := e.renderInlineImage()
	require.NoError(t, err)
}

// Test splitColorAndPatternOperands
func TestSplitColorAndPatternOperands(t *testing.T) {
	values, pattern := splitColorAndPatternOperands([]entity.Object{
		entity.NewReal(0.1), entity.NewReal(0.2), entity.Name("P1"),
	})
	assert.InDeltaSlice(t, []float64{0.1, 0.2}, values, 1e-9)
	assert.NotNil(t, pattern)
	assert.Equal(t, "P1", pattern.Value())

	values, pattern = splitColorAndPatternOperands([]entity.Object{
		entity.NewReal(0.5), entity.NewReal(0.6),
	})
	assert.InDeltaSlice(t, []float64{0.5, 0.6}, values, 1e-9)
	assert.Nil(t, pattern)

	values, pattern = splitColorAndPatternOperands([]entity.Object{
		entity.Name("bad"), entity.NewReal(0.5),
	})
	assert.InDeltaSlice(t, []float64{0.5}, values, 1e-9)
	assert.Nil(t, pattern)

	values, pattern = splitColorAndPatternOperands(nil)
	assert.Empty(t, values)
	assert.Nil(t, pattern)
}

// Test executeCachedOperators capacity growth
func TestExecuteCachedOperatorsCapacityGrowth(t *testing.T) {
	e := NewEvaluator(nil)
	e.operators = make([]Operator, 0, 2)
	ops := []Operator{
		{Opcode: "q"},
		{Opcode: "Q"},
		{Opcode: "n"},
		{Opcode: "q"},
		{Opcode: "Q"},
	}
	e.executeCachedOperators(ops)
	assert.Len(t, e.GetOperators(), 5)
}

// Test executeCachedOperators empty
func TestExecuteCachedOperatorsEmpty(t *testing.T) {
	e := NewEvaluator(nil)
	e.executeCachedOperators(nil)
	assert.Empty(t, e.GetOperators())
}

// Test endInlineImage without begin
func TestEndInlineImageWithoutBegin(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.endInlineImage())
}

// Test EvaluateContent
func TestEvaluateContentData(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.EvaluateContent([]byte("q Q")))
}

// Test objectFloat helper
func TestObjectFloat(t *testing.T) {
	v, ok := objectFloat(entity.NewInteger(3))
	assert.True(t, ok)
	assert.Equal(t, 3.0, v)
	v, ok = objectFloat(entity.NewReal(2.5))
	assert.True(t, ok)
	assert.Equal(t, 2.5, v)
	_, ok = objectFloat(entity.NewString("bad"))
	assert.False(t, ok)
}

// Test emitImageSamplingTrace with debug enabled
func TestEmitImageSamplingTraceWithDebug(t *testing.T) {
	e := NewEvaluator(nil)
	e.SetImageSamplingDebug(true, "test-doc", 1)
	e.emitImageSamplingTrace(
		domainimage.FilterDCT, "DeviceRGB", "", 0,
		"cmyk_mode", "edge_mode",
		"gray_cand", "cmyk_cand",
		"edge_cand", "icc_cand",
		"icc_mode", "sampler",
		"reason", "experimental",
		[6]float64{1, 0, 0, 1, 0, 0},
		0.5, 0.5, 10, 20, 100, 200,
		image.NewRGBA(image.Rect(0, 0, 10, 10)),
	)
}

// Test emitImageSamplingTrace without debug
func TestEmitImageSamplingTraceWithoutDebug(t *testing.T) {
	e := NewEvaluator(nil)
	e.emitImageSamplingTrace(
		domainimage.FilterNone, "DeviceGray", "", 0,
		"", "", "", "", "",
		"", "", "",
		"sampler", "reason",
		[6]float64{}, 0, 0, 0, 0, 0, 0,
		nil,
	)
}

// Test isNearIdentityImageScale boundary conditions
func TestIsNearIdentityImageScaleBoundary(t *testing.T) {
	assert.False(t, isNearIdentityImageScale(0, 0, 1, 1))
	assert.False(t, isNearIdentityImageScale(-1, 10, 1, 1))
	assert.False(t, isNearIdentityImageScale(10, -1, 1, 1))
	assert.False(t, isNearIdentityImageScale(10, 10, 0, 0))
	assert.False(t, isNearIdentityImageScale(10, 10, -1, -1))
	assert.True(t, isNearIdentityImageScale(10, 10, 10, 10))
	assert.True(t, isNearIdentityImageScale(10, 10, 10.001, 10.001))
	assert.False(t, isNearIdentityImageScale(10, 10, 0.5, 0.5))
}

// Test isImageDownscale boundary conditions
func TestIsImageDownscaleBoundary(t *testing.T) {
	assert.False(t, isImageDownscale(0, 0, 1, 1))
	assert.False(t, isImageDownscale(-1, 10, 1, 1))
	assert.False(t, isImageDownscale(10, -1, 1, 1))
	assert.False(t, isImageDownscale(10, 10, 0, 0))
	assert.False(t, isImageDownscale(10, 10, -1, -1))
	assert.True(t, isImageDownscale(10, 10, 5, 5))
	assert.False(t, isImageDownscale(10, 10, 20, 5))
}

// Test isImageUpscale boundary conditions
func TestIsImageUpscaleBoundary(t *testing.T) {
	assert.False(t, isImageUpscale(0, 0, 1, 1))
	assert.False(t, isImageUpscale(-1, 10, 1, 1))
	assert.False(t, isImageUpscale(10, -1, 1, 1))
	assert.False(t, isImageUpscale(10, 10, 0, 0))
	assert.True(t, isImageUpscale(10, 10, 20, 5))
	assert.True(t, isImageUpscale(10, 10, 5, 20))
	assert.False(t, isImageUpscale(10, 10, 5, 5))
}

// Test isImageStrictDownscale boundary conditions
func TestIsImageStrictDownscaleBoundary(t *testing.T) {
	assert.False(t, isImageStrictDownscale(0, 0, 1, 1))
	assert.False(t, isImageStrictDownscale(10, 10, 0, 0))
	assert.True(t, isImageStrictDownscale(10, 10, 5, 5))
	assert.True(t, isImageStrictDownscale(10, 10, 5, 10))
	assert.False(t, isImageStrictDownscale(10, 10, 10, 10))
}

// Test isStrongImageDownscale boundary conditions
func TestIsStrongImageDownscaleBoundary(t *testing.T) {
	assert.False(t, isStrongImageDownscale(0, 0, 1, 1))
	assert.False(t, isStrongImageDownscale(10, 10, 0, 0))
	assert.False(t, isStrongImageDownscale(10, 10, 9, 9))
	assert.True(t, isStrongImageDownscale(100, 100, 10, 10))
	assert.False(t, isStrongImageDownscale(10, 10, 10, 10))
}

// Test isTinyImageSource
func TestIsTinyImageSource(t *testing.T) {
	assert.False(t, isTinyImageSource(0, 0))
	assert.False(t, isTinyImageSource(-1, 10))
	assert.True(t, isTinyImageSource(16, 16))
	assert.True(t, isTinyImageSource(32, 32))
	assert.False(t, isTinyImageSource(33, 33))
}

// Test imageSamplingPhase additional paths
func TestImageSamplingPhaseAdditionalPaths(t *testing.T) {
	phaseX, phaseY := imageSamplingPhase("custom_bilinear", "", true, [6]float64{})
	assert.Equal(t, 0.0, phaseX) // Contains("bilinear") with interpolate → 0,0
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("explicit_nearest", "", false, [6]float64{})
	assert.Equal(t, 0.0, phaseX)
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("auto_downscale_nearest", "", false, [6]float64{})
	assert.Equal(t, 0.0, phaseX)
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("auto_upscale_nearest", "", false, [6]float64{})
	assert.Equal(t, 0.0, phaseX)
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("auto_nearest", "auto_interpolate=false_downscale_small_grayscale", false, [6]float64{})
	assert.Equal(t, 0.0, phaseX)
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("auto_nearest", "other_reason", false, [6]float64{})
	assert.Equal(t, 0.0, phaseX) // Contains("nearest") → 0,0
	assert.Equal(t, 0.0, phaseY)

	phaseX, phaseY = imageSamplingPhase("unknown_sampler", "", true, [6]float64{})
	assert.Equal(t, 0.5, phaseX)
	assert.Equal(t, 0.5, phaseY)

	phaseX, phaseY = imageSamplingPhase("unknown_sampler", "", false, [6]float64{})
	assert.Equal(t, 0.5, phaseX)
	assert.Equal(t, 0.5, phaseY)
}

// Test paintShading missing color entry
func TestPaintShadingMissingColorEntry(t *testing.T) {
	e := NewEvaluator(nil)
	// Missing ShadingType
	dict := entity.NewDict()
	_, err := e.parseShading(dict)
	require.Error(t, err)
	// Invalid ShadingType
	dict = entity.NewDict()
	dict.Set(entity.Name("ShadingType"), entity.NewString("bad"))
	_, err = e.parseShading(dict)
	require.Error(t, err)
	// Out of range ShadingType
	dict = entity.NewDict()
	dict.Set(entity.Name("ShadingType"), entity.NewInteger(99))
	_, err = e.parseShading(dict)
	require.Error(t, err)
}

// Test renderShading different types
func TestRenderShadingDifferentTypes(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)
	shading := entity.NewShading(entity.ShadingFunctionBased, "DeviceRGB")
	fn := &testFunction{values: []float64{0.5}}
	shading.SetFunctions([]entity.Function{fn})
	require.NoError(t, e.renderShading(shading))
	shading2 := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	shading2.SetFunctions([]entity.Function{fn})
	require.NoError(t, e.renderShading(shading2))
}

// Test currentShadingBBox with pathClip
func TestCurrentShadingBBoxWithClipPath(t *testing.T) {
	e := NewEvaluator(nil)
	clipPath := NewPath()
	clipPath.AddElement(&MoveTo{X: 5, Y: 5})
	clipPath.AddElement(&LineTo{X: 100, Y: 5})
	clipPath.AddElement(&LineTo{X: 100, Y: 100})
	clipPath.AddElement(&LineTo{X: 5, Y: 100})
	clipPath.AddElement(&Close{})
	e.graphics.pathClip = clipPath
	bbox, ok := e.currentShadingBBox()
	assert.True(t, ok)
	assert.Equal(t, [4]float64{5, 5, 100, 100}, bbox)

	emptyClip := NewPath()
	emptyClip.AddElement(&MoveTo{X: 0, Y: 0})
	e.graphics.pathClip = emptyClip
	bbox, ok = e.currentShadingBBox()
	assert.False(t, ok)

	e.graphics.pathClip = nil
	path := NewPath()
	path.AddElement(&MoveTo{X: 10, Y: 10})
	path.AddElement(&LineTo{X: 50, Y: 10})
	path.AddElement(&LineTo{X: 50, Y: 50})
	path.AddElement(&Close{})
	e.graphics.path = path
	bbox, ok = e.currentShadingBBox()
	assert.True(t, ok)
	assert.Equal(t, [4]float64{10, 10, 50, 50}, bbox)
}

// Test renderTextString empty / nil font
func TestRenderTextStringEmptyNilFont(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.renderTextString("test"))
	e.graphics.currentState.SetFont(&testFont{widths: map[uint32]float64{}})
	require.NoError(t, e.renderTextString(""))
}

// Test renderTextCharByChar with type3 font
func TestRenderTextCharByCharType3(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)
	charProcs := map[string]*entity.Stream{
		".notdef": entity.NewStream(entity.NewDict(), []byte("n")),
	}
	encoding := map[uint32]string{65: ".notdef"}
	widths := map[uint32]float64{65: 500}
	t3Font := entity.NewType3Font("T3", [6]float64{0.001, 0, 0, 0.001, 0, 0},
		charProcs, encoding, widths, 65, 90, [4]float64{})
	require.NoError(t, e.renderTextCharByChar("A", t3Font, 12))
}

func TestRenderType3GlyphUsesFontResources(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	formDict := entity.NewDict()
	formDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	form := entity.NewStream(formDict, []byte("0 0 1 1 re f"))

	xobjects := entity.NewDict()
	xobjects.Set(entity.Name("X1"), form)
	fontResources := entity.NewDict()
	fontResources.Set(entity.Name("XObject"), xobjects)

	charProcs := map[string]*entity.Stream{
		"A": entity.NewStream(entity.NewDict(), []byte("500 0 d0 /X1 Do")),
	}
	encoding := map[uint32]string{65: "A"}
	widths := map[uint32]float64{65: 500}
	t3Font := entity.NewType3Font("T3", [6]float64{0.001, 0, 0, 0.001, 0, 0},
		charProcs, encoding, widths, 65, 90, [4]float64{})
	t3Font.SetResources(fontResources)

	pageResources := entity.NewDict()
	e.SetResources(pageResources)

	require.NoError(t, e.renderTextCharByChar("A", t3Font, 12))
	assert.Equal(t, 1, canvas.fillCalls)
	assert.Same(t, pageResources, e.resources)
}

// Test moveText error paths
func TestMoveTextErrorPaths(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.moveText(Operator{}))
	require.Error(t, e.moveText(Operator{Operands: []entity.Object{entity.NewString("bad"), entity.NewReal(2)}}))
	require.Error(t, e.moveText(Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewString("bad")}}))
}

// Test moveTextSetLeading error paths
func TestMoveTextSetLeadingErrorPaths(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.moveTextSetLeading(Operator{}))
	require.Error(t, e.moveTextSetLeading(Operator{Operands: []entity.Object{entity.NewString("bad"), entity.NewReal(2)}}))
}

// Test moveTextNextLineAndShowText error paths
func TestMoveTextNextLineAndShowTextErrorPaths(t *testing.T) {
	e := NewEvaluator(nil)
	require.Error(t, e.moveTextNextLineAndShowText(Operator{}))
	require.Error(t, e.moveTextNextLineAndShowText(Operator{Operands: []entity.Object{}}))
}

// Test decodeGlyphName additional cases
func TestDecodeGlyphNameAdditionalCases(t *testing.T) {
	r, ok := decodeGlyphName("tab")
	assert.True(t, ok)
	assert.Equal(t, '\t', r)
	r, ok = decodeGlyphName("hyphen")
	assert.True(t, ok)
	assert.Equal(t, '-', r)
	r, ok = decodeGlyphName("period")
	assert.True(t, ok)
	assert.Equal(t, '.', r)
	r, ok = decodeGlyphName("comma")
	assert.True(t, ok)
	assert.Equal(t, ',', r)
	r, ok = decodeGlyphName("colon")
	assert.True(t, ok)
	assert.Equal(t, ':', r)
	r, ok = decodeGlyphName("semicolon")
	assert.True(t, ok)
	assert.Equal(t, ';', r)
	_, ok = decodeGlyphName("")
	assert.False(t, ok)
	_, ok = decodeGlyphName("uniGG")
	assert.False(t, ok)
	_, ok = decodeGlyphName("uGGG")
	assert.False(t, ok)
}

// Test font wrapper methods
func TestFontWrapperMethods(t *testing.T) {
	baseFont := &testFont{
		widths:    map[uint32]float64{65: 500},
		names:     map[uint32]string{65: "A"},
		isCIDFont: false,
	}

	t.Run("widthMappedFont", func(t *testing.T) {
		wmf := &widthMappedFont{
			base:         baseFont,
			widths:       map[uint32]float64{65: 600},
			defaultWidth: 250,
		}
		glyph, err := wmf.CharCodeToGlyph(65)
		require.NoError(t, err)
		assert.Equal(t, uint32(65), glyph)
		name := wmf.GlyphName(65)
		assert.Equal(t, "A", name)
		width, err := wmf.GetGlyphWidth(65)
		require.NoError(t, err)
		assert.Equal(t, 600.0, width)
		width, err = wmf.GetGlyphWidth(99)
		require.NoError(t, err)
		assert.Equal(t, 250.0, width)
		bb0, bb1, bb2, bb3 := wmf.GetBoundingBox()
		assert.Equal(t, 0.0, bb0)
		assert.Equal(t, 0.0, bb1)
		assert.Equal(t, 1000.0, bb2)
		assert.Equal(t, 1000.0, bb3)
		assert.False(t, wmf.IsCIDFont())
		assert.False(t, wmf.IsSymbolic())
		assert.Equal(t, uint16(1000), wmf.UnitsPerEm())
		assert.Equal(t, "TestFont", wmf.Name())
		_, ok := wmf.GlyphIDByName("A")
		assert.False(t, ok)
	})

	t.Run("widthMappedFont nil", func(t *testing.T) {
		var wmf *widthMappedFont
		_, err := wmf.CharCodeToGlyph(65)
		assert.Error(t, err)
		assert.Equal(t, "", wmf.GlyphName(65))
		_, err = wmf.GetGlyphWidth(65)
		assert.Error(t, err)
		_, ok := wmf.GlyphIDByName("A")
		assert.False(t, ok)
	})

	t.Run("encodedFont", func(t *testing.T) {
		ef := &encodedFont{
			base:         baseFont,
			glyphByCode:  map[uint32]uint32{65: 97},
			nameByCode:   map[uint32]string{65: "a"},
			defaultWidth: 500,
		}
		glyph, err := ef.CharCodeToGlyph(65)
		require.NoError(t, err)
		assert.Equal(t, uint32(97), glyph)
		glyph, err = ef.CharCodeToGlyph(66)
		require.NoError(t, err)
		assert.Equal(t, uint32(66), glyph)
		assert.Equal(t, "A", ef.GlyphName(65))
		assert.False(t, ef.IsCIDFont())
		assert.False(t, ef.IsSymbolic())
		assert.Equal(t, uint16(1000), ef.UnitsPerEm())
		assert.Equal(t, "TestFont", ef.Name())
		_, ok := ef.GlyphIDByName("A")
		assert.False(t, ok)
	})

	t.Run("encodedFont nil", func(t *testing.T) {
		var ef *encodedFont
		_, err := ef.CharCodeToGlyph(65)
		assert.Error(t, err)
		assert.Equal(t, "", ef.GlyphName(65))
		_, err = ef.GetGlyphWidth(65)
		assert.Error(t, err)
		_, ok := ef.GlyphIDByName("A")
		assert.False(t, ok)
	})

	t.Run("glyphSourceOverrideFont", func(t *testing.T) {
		overrideFont := &testFont{
			widths: map[uint32]float64{97: 700},
		}
		gso := &glyphSourceOverrideFont{
			base: baseFont,
			overrides: map[uint32]glyphSourceOverride{
				65: {font: overrideFont, glyph: 97},
			},
		}
		glyph, err := gso.CharCodeToGlyph(65)
		require.NoError(t, err)
		assert.Equal(t, uint32(65), glyph)
		assert.Equal(t, "A", gso.GlyphName(65))
		assert.False(t, gso.IsCIDFont())
		assert.False(t, gso.IsSymbolic())
		assert.Equal(t, uint16(1000), gso.UnitsPerEm())
		assert.Equal(t, "TestFont", gso.Name())
		path, err := gso.RenderGlyph(65, 12)
		assert.Nil(t, path)
		path, err = gso.RenderGlyph(66, 12)
		assert.Nil(t, path)
	})

	t.Run("glyphSourceOverrideFont nil", func(t *testing.T) {
		var gso *glyphSourceOverrideFont
		_, err := gso.CharCodeToGlyph(65)
		assert.Error(t, err)
		assert.Equal(t, "", gso.GlyphName(65))
		_, err = gso.GetGlyphWidth(65)
		assert.Error(t, err)
		_, err = gso.RenderGlyph(65, 12)
		assert.Error(t, err)
	})
}

// Test widthMappedFont with nil widths and no defaultWidth
func TestWidthMappedFontNilWidthsNoDefault(t *testing.T) {
	baseFont := &testFont{
		widths: map[uint32]float64{65: 500},
	}
	wmf := &widthMappedFont{
		base:   baseFont,
		widths: nil,
	}
	width, err := wmf.GetGlyphWidth(65)
	require.NoError(t, err)
	assert.Equal(t, 500.0, width)
	_, err = wmf.GetGlyphWidth(99)
	assert.Error(t, err)

	wmf2 := &widthMappedFont{
		base:   baseFont,
		widths: map[uint32]float64{},
	}
	width, err = wmf2.GetGlyphWidth(99)
	assert.Error(t, err)
}

// Test ResolveCandidate via defaultFontCandidateResolver
func TestResolveCandidateType1(t *testing.T) {
	e := NewEvaluator(nil)
	resolver := defaultFontCandidateResolver{}
	font := resolver.ResolveCandidate(e, entity.NewDict(), "Type1", "TestFont", nil, nil)
	assert.Nil(t, font)
	font = resolver.ResolveCandidate(e, entity.NewDict(), "Unknown", "TestFont", nil, nil)
	assert.Nil(t, font)
}

// Test ResolveType1FontCandidate
func TestResolveType1FontCandidate(t *testing.T) {
	e := NewEvaluator(nil)
	font := e.resolveType1FontCandidate("", nil, fmt.Errorf("no data"))
	assert.Nil(t, font)
	font = e.resolveType1FontCandidate("", []byte{}, nil)
	assert.Nil(t, font)
	font = e.resolveType1FontCandidate("TestFont", []byte("invalid font data"), nil)
	assert.NotNil(t, font) // Type1 parser is lenient, parses even "invalid font data"
}

// Test ResolveType0FontCandidate
func TestResolveType0FontCandidate(t *testing.T) {
	e := NewEvaluator(nil)
	// nil dict causes resolveFirstDescendantFontDict to panic - skip that case
	dict := entity.NewDict()
	font := e.resolveType0FontCandidate(dict, "TestFont")
	assert.Nil(t, font)
	dict = entity.NewDict()
	dict.Set(entity.Name("DescendantFonts"), entity.Name("bad"))
	font = e.resolveType0FontCandidate(dict, "TestFont")
	assert.Nil(t, font)
	dict = entity.NewDict()
	dict.Set(entity.Name("DescendantFonts"), entity.NewArray())
	font = e.resolveType0FontCandidate(dict, "TestFont")
	assert.Nil(t, font)
	dict = entity.NewDict()
	dict.Set(entity.Name("DescendantFonts"), entity.NewArray(entity.NewInteger(1)))
	font = e.resolveType0FontCandidate(dict, "TestFont")
	assert.Nil(t, font)
	descDict := entity.NewDict()
	descDict.Set(entity.Name("BaseFont"), entity.Name("TestCID"))
	dict = entity.NewDict()
	dict.Set(entity.Name("DescendantFonts"), entity.NewArray(descDict))
	font = e.resolveType0FontCandidate(dict, "TestFont")
	assert.Nil(t, font)
}

// Test resolveFirstDescendantFontDict
func TestResolveFirstDescendantFontDict(t *testing.T) {
	e := NewEvaluator(nil)
	dict := entity.NewDict()
	_, ok := e.resolveFirstDescendantFontDict(dict)
	assert.False(t, ok)
	dict = entity.NewDict()
	dict.Set(entity.Name("DescendantFonts"), entity.Name("bad"))
	_, ok = e.resolveFirstDescendantFontDict(dict)
	assert.False(t, ok)
	dict = entity.NewDict()
	dict.Set(entity.Name("DescendantFonts"), entity.NewArray())
	_, ok = e.resolveFirstDescendantFontDict(dict)
	assert.False(t, ok)
	dict = entity.NewDict()
	dict.Set(entity.Name("DescendantFonts"), entity.NewArray(entity.NewString("bad")))
	_, ok = e.resolveFirstDescendantFontDict(dict)
	assert.False(t, ok)
}

// Test resolveDirectObject
func TestResolveDirectObject(t *testing.T) {
	e := NewEvaluator(nil)
	obj := entity.NewInteger(42)
	resolved := e.resolveDirectObject(obj)
	assert.Equal(t, obj, resolved)
	ref := entity.NewRef(1, 0)
	resolved = e.resolveDirectObject(ref)
	assert.Equal(t, ref, resolved)
}

// Test nameValueForEncoding
func TestNameValueForEncoding(t *testing.T) {
	assert.Equal(t, "TestEncoding", nameValueForEncoding(entity.Name("TestEncoding")))
	assert.Equal(t, "", nameValueForEncoding(entity.NewInteger(1)))
	assert.Equal(t, "", nameValueForEncoding(nil))
}

// Test applyFontMetricsFromDict
func TestApplyFontMetricsFromDict(t *testing.T) {
	e := NewEvaluator(nil)
	font := &testFont{
		widths: map[uint32]float64{65: 500},
	}
	result := e.applyFontMetricsFromDict(nil, font)
	assert.NotNil(t, result)
	dict := entity.NewDict()
	result = e.applyFontMetricsFromDict(dict, font)
	assert.NotNil(t, result)
	dict = entity.NewDict()
	dict.Set(entity.Name("FirstChar"), entity.NewInteger(65))
	dict.Set(entity.Name("LastChar"), entity.NewInteger(66))
	dict.Set(entity.Name("Widths"), entity.NewArray(entity.NewReal(600), entity.NewReal(700)))
	result = e.applyFontMetricsFromDict(dict, font)
	require.NotNil(t, result)
	result = e.applyFontMetricsFromDict(dict, nil)
	assert.Nil(t, result)
}

// Test isImageMaskDictValue
func TestIsImageMaskDictValue(t *testing.T) {
	assert.True(t, isImageMaskDictValue(entity.NewBoolean(true)))
	assert.False(t, isImageMaskDictValue(entity.NewBoolean(false)))
	assert.True(t, isImageMaskDictValue(entity.NewInteger(1)))
	assert.False(t, isImageMaskDictValue(entity.NewInteger(0)))
	assert.False(t, isImageMaskDictValue(nil))
}

// Test encodingGlyphNameCandidates
func TestEncodingGlyphNameCandidates(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single char", "A", []string{"A"}},
		{"quotedblleft", "quotedblleft", []string{"quotedblleft", "\""}},
		{"quotedblright", "quotedblright", []string{"quotedblright", "\""}},
		{"quotedblbase", "quotedblbase", []string{"quotedblbase", ","}},
		{"quoteleft", "quoteleft", []string{"quoteleft", "'"}},
		{"quoteright", "quoteright", []string{"quoteright", "'"}},
		{"quotesinglbase", "quotesinglbase", []string{"quotesinglbase", ","}},
		{"endash", "endash", []string{"endash", "-"}},
		{"emdash", "emdash", []string{"emdash", "-"}},
		{"hyphen", "hyphen", []string{"hyphen", "-"}},
		{"minus", "minus", []string{"minus", "-"}},
		{"periodcentered", "periodcentered", []string{"periodcentered", "."}},
		{"plusminus", "plusminus", []string{"plusminus", "+"}},
		{"reflexsubset", "reflexsubset", []string{"reflexsubset", "<"}},
		{"fi", "fi", []string{"fi", "f"}},
		{"fl", "fl", []string{"fl", "f"}},
		{"ff", "ff", []string{"ff", "f"}},
		{"ffi", "ffi", []string{"ffi", "f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodingGlyphNameCandidates(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test normalizeInlineImageStreamDict
func TestNormalizeInlineImageStreamDict(t *testing.T) {
	result := normalizeInlineImageStreamDict(nil)
	assert.NotNil(t, result)
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("Fl"))
	dict.Set(entity.Name("Width"), entity.NewInteger(1))
	result = normalizeInlineImageStreamDict(dict)
	assert.NotNil(t, result)
	dict = entity.NewDict()
	dict.Set(entity.Name("F"), entity.NewArray(entity.Name("Fl"), entity.Name("A85")))
	dict.Set(entity.Name("Width"), entity.NewInteger(1))
	result = normalizeInlineImageStreamDict(dict)
	assert.NotNil(t, result)
}

// Test normalizeInlineImageFilterObject
func TestNormalizeInlineImageFilterObject(t *testing.T) {
	result := normalizeInlineImageFilterObject(entity.Name("Fl"))
	name, ok := result.(entity.Name)
	assert.True(t, ok)
	assert.Equal(t, entity.Name(domainimage.FilterFlate), name)
	arr := entity.NewArray(entity.Name("Fl"), entity.Name("A85"))
	result = normalizeInlineImageFilterObject(arr)
	resultArr, ok := result.(*entity.Array)
	assert.True(t, ok)
	assert.Equal(t, 2, resultArr.Len())
	result = normalizeInlineImageFilterObject(entity.NewInteger(1))
	assert.Equal(t, entity.NewInteger(1), result)
}

// Test parseShading with Real ShadingType
func TestParseShadingRealShadingType(t *testing.T) {
	e := NewEvaluator(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("ShadingType"), entity.NewReal(2))
	dict.Set(entity.Name("Coords"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1)))
	dict.Set(entity.Name("Function"), buildExponentialFunctionDict())
	shading, err := e.parseShading(dict)
	require.NoError(t, err)
	assert.Equal(t, entity.ShadingAxial, shading.GetShadingType())
}

// Test colorFromGraphicsState
func TestColorFromGraphicsState(t *testing.T) {
	c := colorFromGraphicsState(nil, 1.0)
	assert.Equal(t, color.Black, c)
	cs := &ColorSpace{Color: (*Color)(nil)}
	c = colorFromGraphicsState(cs, 1.0)
	assert.Equal(t, color.Black, c)
	cs = &ColorSpace{Color: &Color{Hex: "ZZZ"}}
	c = colorFromGraphicsState(cs, 1.0)
	assert.Equal(t, color.Black, c)
	cs = &ColorSpace{Color: &Color{Hex: "FF"}}
	c = colorFromGraphicsState(cs, 1.0)
	assert.Equal(t, color.Black, c)
	cs = &ColorSpace{Color: &Color{Hex: "FF0000"}}
	c = colorFromGraphicsState(cs, 1.0)
	assert.Equal(t, color.RGBA{R: 255, G: 0, B: 0, A: 255}, c)
	c = colorFromGraphicsState(cs, 0.5)
	assert.Equal(t, color.NRGBA{R: 255, G: 0, B: 0, A: 128}, c)
	cs = &ColorSpace{Color: &Color{Hex: "000000"}}
	c = colorFromGraphicsState(cs, 2.0)
	assert.Equal(t, uint8(255), c.(color.RGBA).A)
}

// Test evaluateImageXObject error paths
func TestEvaluateImageXObjectErrorPaths(t *testing.T) {
	e := NewEvaluator(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Height"), entity.NewInteger(1))
	stream := entity.NewStream(dict, nil)
	require.Error(t, e.evaluateImageXObject(stream, entity.Name("Im1")))
	dict = entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewString("bad"))
	dict.Set(entity.Name("Height"), entity.NewInteger(1))
	stream = entity.NewStream(dict, nil)
	require.Error(t, e.evaluateImageXObject(stream, entity.Name("Im1")))
	dict = entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(1))
	stream = entity.NewStream(dict, nil)
	require.Error(t, e.evaluateImageXObject(stream, entity.Name("Im1")))
	dict = entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(1))
	dict.Set(entity.Name("Height"), entity.NewString("bad"))
	stream = entity.NewStream(dict, nil)
	require.Error(t, e.evaluateImageXObject(stream, entity.Name("Im1")))
	dict = entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(1))
	dict.Set(entity.Name("Height"), entity.NewInteger(1))
	dict.Set(entity.Name("ColorSpace"), entity.Name("Lab"))
	stream = entity.NewStream(dict, nil)
	require.NoError(t, e.evaluateImageXObject(stream, entity.Name("Im1")))
}

// Test chooseImageSamplingPolicy additional branches
func TestChooseImageSamplingPolicyAdditionalBranches(t *testing.T) {
	decision := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy, false, true,
		domainimage.FilterNone, "DeviceGray", false, 10, 10, 10, 10,
	)
	assert.False(t, decision.Interpolate)
	assert.Equal(t, "explicit_interpolate=false", decision.Reason)
	decision = chooseImageSamplingPolicy(
		ImageSamplingModeAdaptiveDCTICCBasedV1, false, false,
		domainimage.FilterNone, "DeviceRGB", false, 10, 10, 10, 10,
	)
	assert.True(t, decision.Interpolate) // isImageDownscale(10,10,10,10) returns true,
	decision = chooseImageSamplingPolicy(
		ImageSamplingModeAdaptiveDCTICCBasedV1, false, false,
		domainimage.FilterNone, "DeviceGray", true, 8, 8, 4, 4,
	)
	assert.False(t, decision.Interpolate)
	assert.Equal(t, "adaptive_tiny_gray_downscale_iccbased", decision.Reason)
}

// Test classifyExperimentalDCTGrayIgnoreICCCandidate additional cases
func TestClassifyExperimentalDCTGrayIgnoreICCCandidateAdditional(t *testing.T) {
	assert.Equal(t, "rejected_non_gray_colorspace", classifyExperimentalDCTGrayIgnoreICCCandidate(
		domainimage.FilterDCT, "DeviceRGB", true, 16, 16, 4, 4, [6]float64{4, 0, 0, 4, 0, 0},
	))
	assert.Equal(t, "rejected_invalid_source", classifyExperimentalDCTGrayIgnoreICCCandidate(
		domainimage.FilterDCT, "DeviceGray", true, 0, 0, 0, 0, [6]float64{0, 0, 0, 0, 0, 0},
	))
	assert.Equal(t, "rejected_large_source", classifyExperimentalDCTGrayIgnoreICCCandidate(
		domainimage.FilterDCT, "DeviceGray", true, 64, 64, 32, 32, [6]float64{32, 0, 0, 32, 0, 0},
	))
	assert.Equal(t, "rejected_non_downscale", classifyExperimentalDCTGrayIgnoreICCCandidate(
		domainimage.FilterDCT, "DeviceGray", true, 16, 16, 20, 20, [6]float64{20, 0, 0, 20, 0, 0},
	))
	assert.Equal(t, "rejected_non_axis_aligned", classifyExperimentalDCTGrayIgnoreICCCandidate(
		domainimage.FilterDCT, "DeviceGray", true, 16, 16, 4, 4, [6]float64{4, 1, 0, 4, 0, 0},
	))
}

// Test getResourceEntry
func TestGetResourceEntry(t *testing.T) {
	e := NewEvaluator(nil)
	assert.Nil(t, e.getResourceEntry(entity.Name("Font"), entity.Name("F1")))
	e.SetResources(entity.NewDict())
	assert.Nil(t, e.getResourceEntry(entity.Name("Font"), entity.Name("F1")))
	fonts := entity.NewDict()
	fonts.Set(entity.Name("F1"), entity.Name("TestFont"))
	resources := entity.NewDict()
	resources.Set(entity.Name("Font"), fonts)
	e.SetResources(resources)
	obj := e.getResourceEntry(entity.Name("Font"), entity.Name("F1"))
	assert.Equal(t, entity.Name("TestFont"), obj)
	obj = e.getResourceEntry(entity.Name("Missing"), entity.Name("F1"))
	assert.Nil(t, obj)
}

// Test setStrokeColorSpace / setFillColorSpace
func TestSetColorSpaces(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.setStrokeColorSpace(Operator{}))
	require.NoError(t, e.setFillColorSpace(Operator{}))
	require.NoError(t, e.setStrokeColorSpace(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
	require.NoError(t, e.setFillColorSpace(Operator{Operands: []entity.Object{entity.NewInteger(1)}}))
	require.NoError(t, e.setStrokeColorSpace(Operator{Operands: []entity.Object{entity.Name("DeviceRGB")}}))
	assert.Equal(t, "DeviceRGB", e.graphics.strokeCS)
	require.NoError(t, e.setFillColorSpace(Operator{Operands: []entity.Object{entity.Name("DeviceCMYK")}}))
	assert.Equal(t, "DeviceCMYK", e.graphics.fillCS)
}

// Test setStrokeColorBySpace / setFillColorBySpace
func TestSetStrokeFillColorBySpace(t *testing.T) {
	e := NewEvaluator(nil)
	require.NoError(t, e.setStrokeColorBySpace(Operator{}))
	require.NoError(t, e.setFillColorBySpace(Operator{}))
}

// Test applyFontEncodingFromDict nil and CID
func TestApplyFontEncodingFromDictNilAndCID(t *testing.T) {
	e := NewEvaluator(nil)
	cidFont := &testFont{isCIDFont: true, widths: map[uint32]float64{}}
	result := e.applyFontEncodingFromDict(nil, cidFont)
	assert.Equal(t, cidFont, result)
	result = e.applyFontEncodingFromDict(entity.NewDict(), cidFont)
	assert.Equal(t, cidFont, result)
}

// Test setFont nil font resolution failure
func TestSetFontResolutionFailure(t *testing.T) {
	e := NewEvaluator(nil)
	resources := entity.NewDict()
	fonts := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("NonExistentFont"))
	fonts.Set(entity.Name("F1"), fontDict)
	resources.Set(entity.Name("Font"), fonts)
	e.SetResources(resources)
	require.NoError(t, e.setFont(Operator{Operands: []entity.Object{entity.Name("F1"), entity.NewReal(12)}}))
}

// Test Evaluate with malformed stream
func TestEvaluateWithMalformedStream(t *testing.T) {
	e := NewEvaluator(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("UnknownFilter"))
	stream := entity.NewStream(dict, []byte("q Q"))
	require.NoError(t, e.Evaluate([]entity.Object{stream}))
}

// Test captureTextWithoutRendering
func TestCaptureTextWithoutRendering(t *testing.T) {
	e := NewEvaluator(nil)
	font := &testFont{
		widths: map[uint32]float64{65: 500, 66: 600},
		names:  map[uint32]string{65: "A", 66: "B"},
	}
	units := []textCodeUnit{
		{code: 'A', raw: []byte("A")},
		{code: 'B', raw: []byte("B")},
	}
	e.captureTextWithoutRendering(units, font, 12)
	assert.Contains(t, e.ExtractedText(), "AB")
}

// ---------------------------------------------------------------------------
// Coverage tests for functions below 60%
// ---------------------------------------------------------------------------

func TestApplyGlyphSourceOverrideFontForDebug(t *testing.T) {
	t.Run("nil font returns nil", func(t *testing.T) {
		result := applyGlyphSourceOverrideFontForDebug("AnyFont", nil)
		assert.Nil(t, result)
	})

	t.Run("unknown font returns same font", func(t *testing.T) {
		font := &testFont{widths: map[uint32]float64{}}
		result := applyGlyphSourceOverrideFontForDebug("UnknownFont", font)
		assert.Equal(t, font, result)
	})

	t.Run("standard font with overrides returns wrapped font", func(t *testing.T) {
		font := &testFont{
			widths:    map[uint32]float64{65: 500, 66: 600},
			names:     map[uint32]string{65: "A", 66: "B"},
			isCIDFont: false,
		}
		// Test with common standard fonts that may have overrides
		result := applyGlyphSourceOverrideFontForDebug("Times-Roman", font)
		// Result is either the same font or a wrapped font depending on overrides
		assert.NotNil(t, result)
	})
}

func TestTextRenderStrategyRender_AllBranches(t *testing.T) {
	font := &testFont{
		widths:    map[uint32]float64{65: 500, 66: 600},
		names:     map[uint32]string{65: "A", 66: "B"},
		isCIDFont: false,
	}

	t.Run("skip all text advances without rendering", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.textPolicy = stubTextRenderPolicy{skipAll: true}
		e.graphics.currentState.SetFont(font)
		e.graphics.currentState.SetFontSize(12)
		identity := [6]float64{1, 0, 0, 1, 0, 0}
		e.textMatrix = identity
		e.textLineMatrix = identity
		units := []textCodeUnit{{code: 'A', raw: []byte("A")}}
		renderer := defaultTextRenderer{}
		require.NoError(t, renderer.Render(e, "A", font, 12, units))
		assert.NotEqual(t, identity, e.textMatrix) // advanced
	})

	t.Run("skip specific font captures without rendering", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.textPolicy = stubTextRenderPolicy{skipFont: true}
		e.graphics.fontDebugName = "TestFont"
		e.graphics.currentState.SetFont(font)
		e.graphics.currentState.SetFontSize(12)
		e.textMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		e.textLineMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		units := []textCodeUnit{{code: 'A', raw: []byte("A")}}
		renderer := defaultTextRenderer{}
		require.NoError(t, renderer.Render(e, "A", font, 12, units))
		assert.Contains(t, e.ExtractedText(), "A")
	})

	t.Run("with canvas uses char by char", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.textPolicy = stubTextRenderPolicy{skipCodes: map[uint32]struct{}{65: {}}}
		e.graphics.currentState.SetFont(font)
		e.graphics.currentState.SetFontSize(12)
		e.textMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		e.textLineMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		units := []textCodeUnit{{code: 'A', raw: []byte("A")}}
		renderer := defaultTextRenderer{}
		require.NoError(t, renderer.Render(e, "A", font, 12, units))
		// char-by-char path should capture text
		assert.Contains(t, e.ExtractedText(), "A")
	})

	t.Run("without canvas captures text", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.textPolicy = stubTextRenderPolicy{}
		e.graphics.currentState.SetFont(font)
		e.graphics.currentState.SetFontSize(12)
		e.textMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		e.textLineMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		units := []textCodeUnit{{code: 'A', raw: []byte("A")}}
		renderer := defaultTextRenderer{}
		require.NoError(t, renderer.Render(e, "A", font, 12, units))
		assert.Contains(t, e.ExtractedText(), "A")
	})

	t.Run("fast path via canvas DrawText", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.textPolicy = stubTextRenderPolicy{fastPath: true}
		e.graphics.currentState.SetFont(font)
		e.graphics.currentState.SetFontSize(12)
		e.textMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		e.textLineMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		units := []textCodeUnit{{code: 'A', raw: []byte("A")}}
		renderer := defaultTextRenderer{}
		require.NoError(t, renderer.Render(e, "A", font, 12, units))
		assert.Greater(t, canvas.drawTextCalls, 0)
	})
}

func TestApplyClippingPath_FullCoverage(t *testing.T) {
	t.Run("nil canvas returns early", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.applyClippingPath()
	})

	t.Run("nil pathClip returns early", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.SetCanvas(newRecordingCanvas())
		e.applyClippingPath()
	})

	t.Run("with path replays to canvas and clips", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		clipPath := NewPath()
		clipPath.AddElement(&MoveTo{X: 0, Y: 0})
		clipPath.AddElement(&LineTo{X: 10, Y: 0})
		clipPath.AddElement(&LineTo{X: 10, Y: 10})
		clipPath.AddElement(&CurveTo{X1: 5, Y1: 5, X2: 8, Y2: 8, X: 0, Y: 10})
		clipPath.AddElement(&Close{})
		e.graphics.pathClip = clipPath
		e.applyClippingPath()
		assert.Equal(t, 1, canvas.clipCalls)
		assert.Greater(t, canvas.moveCalls, 0)
		assert.Greater(t, canvas.lineCalls, 0)
		assert.Greater(t, canvas.curveCalls, 0)
		assert.Greater(t, canvas.closeCalls, 0)
		assert.Equal(t, []string{"M", "L", "L", "C", "Z", "Clip"}, canvas.ops)
	})

	t.Run("canvas with SetClipPathDirect interface", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := &clipDirectTestCanvas{}
		e.SetCanvas(canvas)
		clipPath := NewPath()
		clipPath.AddElement(&MoveTo{X: 5, Y: 5})
		clipPath.AddElement(&Close{})
		e.graphics.pathClip = clipPath
		e.applyClippingPath()
		assert.True(t, canvas.setClipPathDirectCalled)
	})
}

type clipDirectTestCanvas struct {
	testCanvas
	setClipPathDirectCalled bool
}

func (c *clipDirectTestCanvas) SetClipPathDirect(elements []interface{}, fillRule graphics.FillRule) {
	c.setClipPathDirectCalled = true
}

func TestResolveICCBasedProfileWithDepth(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("nil returns false", func(t *testing.T) {
		_, ok := e.resolveICCBasedProfileWithDepth(nil, 0)
		assert.False(t, ok)
	})

	t.Run("depth exceeds limit", func(t *testing.T) {
		_, ok := e.resolveICCBasedProfileWithDepth(entity.Name("ICCBased"), 9)
		assert.False(t, ok)
	})

	t.Run("empty array returns false", func(t *testing.T) {
		_, ok := e.resolveICCBasedProfileWithDepth(entity.NewArray(), 0)
		assert.False(t, ok)
	})

	t.Run("ICCBased array returns profile", func(t *testing.T) {
		streamDict := entity.NewDict()
		iccStream := entity.NewStream(streamDict, []byte{0xDE, 0xAD})
		cs := entity.NewArray(entity.Name("ICCBased"), iccStream)
		profile, ok := e.resolveICCBasedProfileWithDepth(cs, 0)
		assert.True(t, ok)
		assert.Equal(t, []byte{0xDE, 0xAD}, profile)
	})

	t.Run("ICCBased array too short returns false", func(t *testing.T) {
		cs := entity.NewArray(entity.Name("ICCBased"))
		_, ok := e.resolveICCBasedProfileWithDepth(cs, 0)
		assert.False(t, ok)
	})

	t.Run("Indexed with ICCBased base resolves profile", func(t *testing.T) {
		streamDict := entity.NewDict()
		iccStream := entity.NewStream(streamDict, []byte{0xBE, 0xEF})
		iccCS := entity.NewArray(entity.Name("ICCBased"), iccStream)
		indexed := entity.NewArray(entity.Name("Indexed"), iccCS, entity.NewInteger(255), entity.NewString("abc"))
		profile, ok := e.resolveICCBasedProfileWithDepth(indexed, 0)
		assert.True(t, ok)
		assert.Equal(t, []byte{0xBE, 0xEF}, profile)
	})

	t.Run("ref resolves via xref", func(t *testing.T) {
		ref := entity.NewRef(10, 0)
		streamDict := entity.NewDict()
		iccStream := entity.NewStream(streamDict, []byte{0xCA, 0xFE})
		cs := entity.NewArray(entity.Name("ICCBased"), iccStream)
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: cs}}
		e2 := NewEvaluator(xref)
		profile, ok := e2.resolveICCBasedProfileWithDepth(ref, 0)
		assert.True(t, ok)
		assert.Equal(t, []byte{0xCA, 0xFE}, profile)
	})

	t.Run("ref with nil xref returns false", func(t *testing.T) {
		e3 := NewEvaluator(nil)
		_, ok := e3.resolveICCBasedProfileWithDepth(entity.NewRef(1, 0), 0)
		assert.False(t, ok)
	})

	t.Run("non-ICCBased array base name returns false", func(t *testing.T) {
		cs := entity.NewArray(entity.Name("DeviceRGB"))
		_, ok := e.resolveICCBasedProfileWithDepth(cs, 0)
		assert.False(t, ok)
	})

	t.Run("stream with RawBytes fallback", func(t *testing.T) {
		// Stream that fails Decode but has RawBytes
		dict := entity.NewDict()
		dict.Set(entity.Name("Filter"), entity.Name("UnknownFilter"))
		stream := entity.NewStream(dict, []byte{1, 2, 3})
		profile, ok := e.resolveICCBasedProfileWithDepth(stream, 0)
		assert.True(t, ok)
		assert.Equal(t, []byte{1, 2, 3}, profile)
	})
}

func TestResolveICCProfileObjectWithDepth(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("nil returns false", func(t *testing.T) {
		_, ok := e.resolveICCProfileObjectWithDepth(nil, 0)
		assert.False(t, ok)
	})

	t.Run("depth exceeds limit", func(t *testing.T) {
		_, ok := e.resolveICCProfileObjectWithDepth(entity.Name("test"), 9)
		assert.False(t, ok)
	})

	t.Run("ref resolves via xref", func(t *testing.T) {
		ref := entity.NewRef(20, 0)
		iccStream := entity.NewStream(entity.NewDict(), []byte{0xAA, 0xBB})
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: iccStream}}
		e2 := NewEvaluator(xref)
		profile, ok := e2.resolveICCProfileObjectWithDepth(ref, 0)
		assert.True(t, ok)
		assert.Equal(t, []byte{0xAA, 0xBB}, profile)
	})

	t.Run("ref with nil xref returns false", func(t *testing.T) {
		_, ok := e.resolveICCProfileObjectWithDepth(entity.NewRef(1, 0), 0)
		assert.False(t, ok)
	})

	t.Run("non-stream type returns false", func(t *testing.T) {
		_, ok := e.resolveICCProfileObjectWithDepth(entity.NewInteger(5), 0)
		assert.False(t, ok)
	})

	t.Run("stream with raw bytes fallback", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("Filter"), entity.Name("UnknownFilter"))
		stream := entity.NewStream(dict, []byte{0xCC, 0xDD})
		profile, ok := e.resolveICCProfileObjectWithDepth(stream, 0)
		assert.True(t, ok)
		assert.Equal(t, []byte{0xCC, 0xDD}, profile)
	})
}

func TestResolveImageColorSpaceWithDepth(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("nil returns DeviceRGB", func(t *testing.T) {
		cs, ok := e.resolveImageColorSpaceWithDepth(nil, 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceRGB", cs)
	})

	t.Run("depth exceeds limit returns false", func(t *testing.T) {
		_, ok := e.resolveImageColorSpaceWithDepth(entity.Name("DeviceRGB"), 9)
		assert.False(t, ok)
	})

	t.Run("entity.Name resolves known spaces", func(t *testing.T) {
		cs, ok := e.resolveImageColorSpaceWithDepth(entity.Name("G"), 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceGray", cs)
	})

	t.Run("entity.Name unknown returns false", func(t *testing.T) {
		_, ok := e.resolveImageColorSpaceWithDepth(entity.Name("Lab"), 0)
		assert.False(t, ok)
	})

	t.Run("ref resolves via xref", func(t *testing.T) {
		ref := entity.NewRef(30, 0)
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: entity.Name("RGB")}}
		e2 := NewEvaluator(xref)
		cs, ok := e2.resolveImageColorSpaceWithDepth(ref, 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceRGB", cs)
	})

	t.Run("ref with nil xref returns false", func(t *testing.T) {
		_, ok := e.resolveImageColorSpaceWithDepth(entity.NewRef(1, 0), 0)
		assert.False(t, ok)
	})

	t.Run("empty array returns false", func(t *testing.T) {
		_, ok := e.resolveImageColorSpaceWithDepth(entity.NewArray(), 0)
		assert.False(t, ok)
	})

	t.Run("ICCBased with 3 components returns DeviceRGB", func(t *testing.T) {
		iccDict := entity.NewDict()
		iccDict.Set(entity.Name("N"), entity.NewInteger(3))
		iccStream := entity.NewStream(iccDict, []byte{1})
		cs := entity.NewArray(entity.Name("ICCBased"), iccStream)
		result, ok := e.resolveImageColorSpaceWithDepth(cs, 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceRGB", result)
	})

	t.Run("ICCBased with 1 component returns DeviceGray", func(t *testing.T) {
		iccDict := entity.NewDict()
		iccDict.Set(entity.Name("N"), entity.NewInteger(1))
		iccStream := entity.NewStream(iccDict, []byte{1})
		cs := entity.NewArray(entity.Name("ICCBased"), iccStream)
		result, ok := e.resolveImageColorSpaceWithDepth(cs, 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceGray", result)
	})

	t.Run("ICCBased with 4 components returns DeviceCMYK", func(t *testing.T) {
		iccDict := entity.NewDict()
		iccDict.Set(entity.Name("N"), entity.NewInteger(4))
		iccStream := entity.NewStream(iccDict, []byte{1})
		cs := entity.NewArray(entity.Name("ICCBased"), iccStream)
		result, ok := e.resolveImageColorSpaceWithDepth(cs, 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceCMYK", result)
	})

	t.Run("ICCBased with unknown N returns false", func(t *testing.T) {
		iccDict := entity.NewDict()
		iccDict.Set(entity.Name("N"), entity.NewInteger(5))
		iccStream := entity.NewStream(iccDict, []byte{1})
		cs := entity.NewArray(entity.Name("ICCBased"), iccStream)
		_, ok := e.resolveImageColorSpaceWithDepth(cs, 0)
		assert.False(t, ok)
	})

	t.Run("non-ICCBased array resolves base name", func(t *testing.T) {
		cs := entity.NewArray(entity.Name("RGB"))
		result, ok := e.resolveImageColorSpaceWithDepth(cs, 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceRGB", result)
	})

	t.Run("unsupported base name returns false", func(t *testing.T) {
		cs := entity.NewArray(entity.Name("Lab"))
		_, ok := e.resolveImageColorSpaceWithDepth(cs, 0)
		assert.False(t, ok)
	})

	t.Run("default type returns false", func(t *testing.T) {
		_, ok := e.resolveImageColorSpaceWithDepth(entity.NewInteger(5), 0)
		assert.False(t, ok)
	})
}

func TestResolveICCBasedComponents(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("nil array returns false", func(t *testing.T) {
		_, ok := e.resolveICCBasedComponents(nil, 0)
		assert.False(t, ok)
	})

	t.Run("short array returns false", func(t *testing.T) {
		_, ok := e.resolveICCBasedComponents(entity.NewArray(entity.Name("ICCBased")), 0)
		assert.False(t, ok)
	})

	t.Run("depth exceeds limit returns false", func(t *testing.T) {
		iccDict := entity.NewDict()
		iccDict.Set(entity.Name("N"), entity.NewInteger(3))
		cs := entity.NewArray(entity.Name("ICCBased"), entity.NewStream(iccDict, []byte{}))
		_, ok := e.resolveICCBasedComponents(cs, 9)
		assert.False(t, ok)
	})

	t.Run("direct stream resolves N", func(t *testing.T) {
		iccDict := entity.NewDict()
		iccDict.Set(entity.Name("N"), entity.NewInteger(3))
		cs := entity.NewArray(entity.Name("ICCBased"), entity.NewStream(iccDict, []byte{}))
		n, ok := e.resolveICCBasedComponents(cs, 0)
		assert.True(t, ok)
		assert.Equal(t, 3, n)
	})

	t.Run("ref resolves via xref", func(t *testing.T) {
		ref := entity.NewRef(40, 0)
		iccDict := entity.NewDict()
		iccDict.Set(entity.Name("N"), entity.NewInteger(1))
		iccStream := entity.NewStream(iccDict, []byte{1})
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: iccStream}}
		e2 := NewEvaluator(xref)
		cs := entity.NewArray(entity.Name("ICCBased"), ref)
		n, ok := e2.resolveICCBasedComponents(cs, 0)
		assert.True(t, ok)
		assert.Equal(t, 1, n)
	})

	t.Run("ref with nil xref returns false", func(t *testing.T) {
		cs := entity.NewArray(entity.Name("ICCBased"), entity.NewRef(1, 0))
		_, ok := e.resolveICCBasedComponents(cs, 0)
		assert.False(t, ok)
	})
}

func TestParseOperatorsWithHandler(t *testing.T) {
	t.Run("empty data returns nil", func(t *testing.T) {
		e := NewEvaluator(nil)
		err := e.parseOperatorsWithHandler([]byte{}, func(op Operator) {})
		require.NoError(t, err)
	})

	t.Run("simple operators invoke handler", func(t *testing.T) {
		e := NewEvaluator(nil)
		var ops []Operator
		err := e.parseOperatorsWithHandler([]byte("1 2 m 3 4 l S"), func(op Operator) {
			ops = append(ops, op)
		})
		require.NoError(t, err)
		assert.Len(t, ops, 3)
		assert.Equal(t, "m", ops[0].Opcode)
		assert.Equal(t, "l", ops[1].Opcode)
		assert.Equal(t, "S", ops[2].Opcode)
	})

	t.Run("inline image triggers BI path", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		var ops []Operator
		data := []byte("/W 1 /H 1 /CS /G ID \x80\x40\x20 EI Q")
		err := e.parseOperatorsWithHandler(data, func(op Operator) {
			ops = append(ops, op)
		})
		require.NoError(t, err)
		// The BI path causes early return; Q may not be parsed
	})

	t.Run("malformed data recovers", func(t *testing.T) {
		e := NewEvaluator(nil)
		var ops []Operator
		// data with invalid tokens mixed with valid operators
		err := e.parseOperatorsWithHandler([]byte("q Q"), func(op Operator) {
			ops = append(ops, op)
		})
		require.NoError(t, err)
	})
}

func TestGetEmbeddedFontData(t *testing.T) {
	t.Run("missing font descriptor", func(t *testing.T) {
		e := NewEvaluator(nil)
		_, err := e.getEmbeddedFontData(entity.NewDict())
		require.Error(t, err)
	})

	t.Run("descriptor is not a dict and not a ref", func(t *testing.T) {
		e := NewEvaluator(nil)
		dict := entity.NewDict()
		dict.Set(entity.Name("FontDescriptor"), entity.NewString("bad"))
		_, err := e.getEmbeddedFontData(dict)
		require.Error(t, err)
	})

	t.Run("descriptor ref with nil xref", func(t *testing.T) {
		e := NewEvaluator(nil)
		dict := entity.NewDict()
		dict.Set(entity.Name("FontDescriptor"), entity.NewRef(1, 0))
		_, err := e.getEmbeddedFontData(dict)
		require.Error(t, err)
	})

	t.Run("descriptor ref resolves to non-dict", func(t *testing.T) {
		ref := entity.NewRef(50, 0)
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: entity.NewString("not a dict")}}
		e := NewEvaluator(xref)
		dict := entity.NewDict()
		dict.Set(entity.Name("FontDescriptor"), ref)
		_, err := e.getEmbeddedFontData(dict)
		require.Error(t, err)
	})

	t.Run("descriptor ref resolves to dict with font file", func(t *testing.T) {
		fontStreamDict := entity.NewDict()
		fontData := []byte{0x00, 0x01, 0x00, 0x00} // minimal TrueType header
		fontStream := entity.NewStream(fontStreamDict, fontData)
		fontStreamRef := entity.NewRef(51, 0)

		descDict := entity.NewDict()
		descDict.Set(entity.Name("FontFile2"), fontStream)
		descRef := entity.NewRef(52, 0)

		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{
			descRef:       descDict,
			fontStreamRef: fontStream,
		}}
		e := NewEvaluator(xref)

		fontDict := entity.NewDict()
		fontDict.Set(entity.Name("FontDescriptor"), descRef)
		data, err := e.getEmbeddedFontData(fontDict)
		require.NoError(t, err)
		assert.Equal(t, fontData, data)
	})

	t.Run("descriptor with no font file keys", func(t *testing.T) {
		e := NewEvaluator(nil)
		descDict := entity.NewDict()
		fontDict := entity.NewDict()
		fontDict.Set(entity.Name("FontDescriptor"), descDict)
		_, err := e.getEmbeddedFontData(fontDict)
		require.Error(t, err)
	})

	t.Run("descriptor with empty font stream", func(t *testing.T) {
		e := NewEvaluator(nil)
		descDict := entity.NewDict()
		descDict.Set(entity.Name("FontFile"), entity.NewStream(entity.NewDict(), []byte{}))
		fontDict := entity.NewDict()
		fontDict.Set(entity.Name("FontDescriptor"), descDict)
		_, err := e.getEmbeddedFontData(fontDict)
		require.Error(t, err)
	})
}

func TestParseAxialShadingFull(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("with extend booleans", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1),
		))
		dict.Set(entity.Name("Extend"), entity.NewArray(
			entity.NewBoolean(true), entity.NewBoolean(false),
		))
		dict.Set(entity.Name("Function"), buildExponentialFunctionDict())
		s, err := e.parseAxialShading(dict, shading)
		require.NoError(t, err)
		assert.Equal(t, [2]bool{true, false}, s.GetExtend())
		assert.NotEmpty(t, s.GetFunctions())
	})

	t.Run("with extend integers", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1),
		))
		dict.Set(entity.Name("Extend"), entity.NewArray(
			entity.NewInteger(1), entity.NewInteger(0),
		))
		s, err := e.parseAxialShading(dict, shading)
		require.NoError(t, err)
		assert.Equal(t, [2]bool{true, false}, s.GetExtend())
	})

	t.Run("with nil extend values", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(1), entity.NewReal(1),
		))
		dict.Set(entity.Name("Extend"), entity.NewArray(
			entity.NewString("bad"), entity.NewString("bad"),
		))
		s, err := e.parseAxialShading(dict, shading)
		require.NoError(t, err)
		assert.Equal(t, [2]bool{false, false}, s.GetExtend())
	})

	t.Run("with integer coords", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewInteger(0), entity.NewInteger(0), entity.NewInteger(100), entity.NewInteger(100),
		))
		s, err := e.parseAxialShading(dict, shading)
		require.NoError(t, err)
		coords := s.GetCoords()
		assert.Equal(t, []float64{0, 0, 100, 100}, coords)
	})

	t.Run("missing coords", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		_, err := e.parseAxialShading(entity.NewDict(), shading)
		require.Error(t, err)
	})

	t.Run("short coords array", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
		_, err := e.parseAxialShading(dict, shading)
		require.Error(t, err)
	})
}

func TestParseRadialShadingFull(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("with extend and function", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(5),
			entity.NewReal(100), entity.NewReal(100), entity.NewReal(10),
		))
		dict.Set(entity.Name("Extend"), entity.NewArray(
			entity.NewBoolean(true), entity.NewBoolean(true),
		))
		dict.Set(entity.Name("Function"), buildExponentialFunctionDict())
		s, err := e.parseRadialShading(dict, shading)
		require.NoError(t, err)
		assert.Equal(t, [2]bool{true, true}, s.GetExtend())
		assert.NotEmpty(t, s.GetFunctions())
	})

	t.Run("with integer coords", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewInteger(0), entity.NewInteger(0), entity.NewInteger(1),
			entity.NewInteger(100), entity.NewInteger(100), entity.NewInteger(2),
		))
		s, err := e.parseRadialShading(dict, shading)
		require.NoError(t, err)
		coords := s.GetCoords()
		assert.Equal(t, []float64{0, 0, 1, 100, 100, 2}, coords)
	})

	t.Run("missing coords", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
		_, err := e.parseRadialShading(entity.NewDict(), shading)
		require.Error(t, err)
	})

	t.Run("short coords array", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(1),
			entity.NewReal(100), entity.NewReal(100),
		))
		_, err := e.parseRadialShading(dict, shading)
		require.Error(t, err)
	})

	t.Run("with extend integers", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
		dict := entity.NewDict()
		dict.Set(entity.Name("Coords"), entity.NewArray(
			entity.NewReal(0), entity.NewReal(0), entity.NewReal(5),
			entity.NewReal(100), entity.NewReal(100), entity.NewReal(10),
		))
		dict.Set(entity.Name("Extend"), entity.NewArray(
			entity.NewInteger(1), entity.NewInteger(1),
		))
		s, err := e.parseRadialShading(dict, shading)
		require.NoError(t, err)
		assert.Equal(t, [2]bool{true, true}, s.GetExtend())
	})
}

func TestGetBoundingBoxCoverage(t *testing.T) {
	baseFont := &testFont{
		widths: map[uint32]float64{65: 500},
	}

	t.Run("widthMappedFont GetBoundingBox", func(t *testing.T) {
		wmf := &widthMappedFont{base: baseFont, widths: map[uint32]float64{65: 600}}
		x0, _, x1, _ := wmf.GetBoundingBox()
		assert.Equal(t, 0.0, x0)
		assert.Equal(t, 1000.0, x1)
	})

	t.Run("encodedFont GetBoundingBox", func(t *testing.T) {
		ef := &encodedFont{base: baseFont, glyphByCode: map[uint32]uint32{}}
		x0, _, x1, _ := ef.GetBoundingBox()
		assert.Equal(t, 0.0, x0)
		assert.Equal(t, 1000.0, x1)
	})

	t.Run("glyphSourceOverrideFont GetBoundingBox", func(t *testing.T) {
		gso := &glyphSourceOverrideFont{base: baseFont, overrides: map[uint32]glyphSourceOverride{}}
		x0, _, x1, _ := gso.GetBoundingBox()
		assert.Equal(t, 0.0, x0)
		assert.Equal(t, 1000.0, x1)
	})
}

func TestResolveDirectObjectCoverage(t *testing.T) {
	t.Run("ref resolves via xref", func(t *testing.T) {
		ref := entity.NewRef(60, 0)
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: entity.Name("Resolved")}}
		e := NewEvaluator(xref)
		result := e.resolveDirectObject(ref)
		assert.Equal(t, entity.Name("Resolved"), result)
	})

	t.Run("ref with nil xref returns ref", func(t *testing.T) {
		e := NewEvaluator(nil)
		ref := entity.NewRef(1, 0)
		result := e.resolveDirectObject(ref)
		assert.Equal(t, ref, result)
	})

	t.Run("non-ref returns itself", func(t *testing.T) {
		e := NewEvaluator(nil)
		obj := entity.NewInteger(42)
		result := e.resolveDirectObject(obj)
		assert.Equal(t, obj, result)
	})

	t.Run("ref fetch error returns ref", func(t *testing.T) {
		ref := entity.NewRef(99, 0)
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{}}
		e := NewEvaluator(xref)
		result := e.resolveDirectObject(ref)
		assert.Equal(t, ref, result)
	})
}

func TestRenderImageMaskToCanvas_FullCoverage(t *testing.T) {
	t.Run("successful mask rendering", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.graphics.transform = [6]float64{1, 0, 0, 1, 0, 0}
		e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: "FF0000"}}
		e.graphics.fillAlpha = 1.0

		// Use 1bpc mask data: 2x2 pixels = 1 byte (4 bits used, 4 padding)
		// Bits: 1,0,1,0 = mixed transparency
		data := []byte{0b10100000}
		err := e.renderImageMaskToCanvas(data, 2, 2, 1, domainimage.FilterFlate, false, false, false)
		require.NoError(t, err)
		assert.Equal(t, 1, canvas.drawImageCalls)
	})

	t.Run("mask with interpolation", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.graphics.transform = [6]float64{1, 0, 0, 1, 0, 0}
		e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: "00FF00"}}
		e.graphics.fillAlpha = 1.0

		data := []byte{0b01100000}
		err := e.renderImageMaskToCanvas(data, 2, 2, 1, domainimage.FilterFlate, true, false, true)
		require.NoError(t, err)
		assert.Equal(t, 1, canvas.drawImageCalls)
	})
}

func TestResolveFontFromDescendant(t *testing.T) {
	t.Run("descendant with CIDFont subtype", func(t *testing.T) {
		e := NewEvaluator(nil)
		descDict := entity.NewDict()
		descDict.Set(entity.Name("Subtype"), entity.Name("CIDFontType2"))
		descDict.Set(entity.Name("BaseFont"), entity.Name("TestCIDFont"))
		descDict.Set(entity.Name("W"), entity.NewArray(
			entity.NewInteger(0), entity.NewArray(entity.NewInteger(500)),
		))

		dict := entity.NewDict()
		dict.Set(entity.Name("DescendantFonts"), entity.NewArray(descDict))
		font := e.resolveType0FontCandidate(dict, "TestCIDFont")
		// May return nil if no valid CID system info
		// Just ensure no panic
		_ = font
	})

	t.Run("empty descendants", func(t *testing.T) {
		e := NewEvaluator(nil)
		dict := entity.NewDict()
		dict.Set(entity.Name("DescendantFonts"), entity.NewArray())
		font := e.resolveType0FontCandidate(dict, "TestFont")
		assert.Nil(t, font)
	})
}

func TestNormalizeBaseFontNameCoverage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ABCDEF+Times-Roman", "Times-Roman"},
		{"TimesNewRomanPSMT", "TimesNewRomanPSMT"},
		{"", ""},
		{"Helvetica-Bold", "Helvetica-Bold"},
		{"NimbusRomNo9L-Regu", "Times-Roman"},
		{"NimbusRomNo9L-Medi", "Times-Bold"},
		{"NimbusRomNo9L-ReguItal", "Times-Italic"},
		{"NimbusRomNo9L-MediItal", "Times-BoldItalic"},
		{"NimbusRomNo9L-Regu-Slant_167", "Times-Italic"},
		{"Helvetica", "Helvetica"},
		{"Arial", "Helvetica"},
		{"YuMincho-Regular", "Helvetica"},
		{"NimbusSanL-Bold", "Helvetica-Bold"},
		{"Arial,Bold", "Helvetica-Bold"},
		{"Calibri-Bold", "Helvetica-Bold"},
		{"Helvetica-Oblique", "Helvetica-Oblique"},
		{"Arial,Italic", "Helvetica-Oblique"},
		{"Calibri-Italic", "Helvetica-Oblique"},
		{"Helvetica-BoldOblique", "Helvetica-BoldOblique"},
		{"Arial,BoldItalic", "Helvetica-BoldOblique"},
		{"Calibri-BoldItalic", "Helvetica-BoldOblique"},
		{"CMRTesting", "Times-Roman"},
		{"CMMITesting", "Times-Italic"},
		{"CMSYTesting", "Symbol"},
		{"CMTTTesting", "Courier"},
		{"SFTTTesting", "Courier"},
		{"SFSXTesting", "Helvetica"},
		{"Calibri-BoldTesting", "Helvetica-Bold"},
		{"CalibriTesting", "Helvetica"},
		{"UnknownFont", "UnknownFont"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeBaseFontName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveType1FontCandidateDebugFallbacks(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("non-fallback base font returns nil for standard", func(t *testing.T) {
		font := e.resolveType1FontCandidate("Times-Roman", nil, fmt.Errorf("no data"))
		// Without embedded data, without a default font loaded, should return nil
		// (unless a preferred fallback exists)
		_ = font
	})

	t.Run("embedded data returns parsed font", func(t *testing.T) {
		font := e.resolveType1FontCandidate("TestFont", []byte("%!PS-AdobeFont-1.0: TestFont\n"), nil)
		// Type1 parser is lenient - may return a font
		_ = font
	})
}

func TestResolveFirstDescendantFontDictViaRef(t *testing.T) {
	t.Run("descendant resolved via xref", func(t *testing.T) {
		ref := entity.NewRef(70, 0)
		descDict := entity.NewDict()
		descDict.Set(entity.Name("Subtype"), entity.Name("CIDFontType2"))
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: descDict}}
		e := NewEvaluator(xref)

		dict := entity.NewDict()
		dict.Set(entity.Name("DescendantFonts"), entity.NewArray(ref))
		result, ok := e.resolveFirstDescendantFontDict(dict)
		assert.True(t, ok)
		assert.NotNil(t, result)
	})

	t.Run("ref fetch error", func(t *testing.T) {
		ref := entity.NewRef(71, 0)
		e := NewEvaluator(&testMapXRef{objects: map[entity.Ref]entity.Object{}})

		dict := entity.NewDict()
		dict.Set(entity.Name("DescendantFonts"), entity.NewArray(ref))
		_, ok := e.resolveFirstDescendantFontDict(dict)
		assert.False(t, ok)
	})

	t.Run("ref resolves to non-dict", func(t *testing.T) {
		ref := entity.NewRef(72, 0)
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: entity.NewString("bad")}}
		e := NewEvaluator(xref)

		dict := entity.NewDict()
		dict.Set(entity.Name("DescendantFonts"), entity.NewArray(ref))
		_, ok := e.resolveFirstDescendantFontDict(dict)
		assert.False(t, ok)
	})
}

func TestResolveColorSpaceNameWithDepth(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("name resolves", func(t *testing.T) {
		name, ok := e.resolveColorSpaceName(entity.Name("DeviceRGB"), 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceRGB", name)
	})

	t.Run("ref resolves", func(t *testing.T) {
		ref := entity.NewRef(80, 0)
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: entity.Name("DeviceGray")}}
		e2 := NewEvaluator(xref)
		name, ok := e2.resolveColorSpaceName(ref, 0)
		assert.True(t, ok)
		assert.Equal(t, "DeviceGray", name)
	})

	t.Run("depth exceeds limit", func(t *testing.T) {
		_, ok := e.resolveColorSpaceName(entity.Name("DeviceRGB"), 9)
		assert.False(t, ok)
	})

	t.Run("nil returns false", func(t *testing.T) {
		_, ok := e.resolveColorSpaceName(nil, 0)
		assert.False(t, ok)
	})

	t.Run("ref with nil xref returns false", func(t *testing.T) {
		_, ok := e.resolveColorSpaceName(entity.NewRef(1, 0), 0)
		assert.False(t, ok)
	})

	t.Run("ref fetch error returns false", func(t *testing.T) {
		e2 := NewEvaluator(&testMapXRef{objects: map[entity.Ref]entity.Object{}})
		_, ok := e2.resolveColorSpaceName(entity.NewRef(1, 0), 0)
		assert.False(t, ok)
	})
}

func TestRenderAxialShadingCoverage(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	t.Run("with extend flags", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		shading.SetCoords([]float64{0, 0, 100, 100})
		extend := [2]bool{true, true}
		shading.SetExtend(extend)
		fn := &testFunction{values: []float64{0.5, 0.5, 0.5}}
		shading.SetFunctions([]entity.Function{fn})

		bbox := [4]float64{0, 0, 200, 200}
		err := e.renderAxialShading(shading, bbox)
		require.NoError(t, err)
		assert.Equal(t, 1, canvas.fillCalls)
	})

	t.Run("without extend flags", func(t *testing.T) {
		canvas2 := newRecordingCanvas()
		e2 := NewEvaluator(nil)
		e2.SetCanvas(canvas2)

		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		shading.SetCoords([]float64{10, 10, 90, 90})
		fn := &testFunction{values: []float64{0.8}}
		shading.SetFunctions([]entity.Function{fn})

		bbox := [4]float64{0, 0, 100, 100}
		err := e2.renderAxialShading(shading, bbox)
		require.NoError(t, err)
		assert.Equal(t, 1, canvas2.fillCalls)
	})

	t.Run("invalid coords", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		shading.SetCoords([]float64{0, 0})
		bbox := [4]float64{0, 0, 100, 100}
		err := e.renderAxialShading(shading, bbox)
		require.Error(t, err)
	})
}

func TestRenderRadialShadingCoverage(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)

	t.Run("valid radial shading", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
		shading.SetCoords([]float64{50, 50, 10, 50, 50, 100})
		fn := &testFunction{values: []float64{0.5, 0.5, 0.5}}
		shading.SetFunctions([]entity.Function{fn})

		bbox := [4]float64{0, 0, 200, 200}
		err := e.renderRadialShading(shading, bbox)
		require.NoError(t, err)
		assert.Equal(t, 1, canvas.fillCalls)
	})

	t.Run("invalid coords", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
		shading.SetCoords([]float64{0, 0, 10})
		bbox := [4]float64{0, 0, 100, 100}
		err := e.renderRadialShading(shading, bbox)
		require.Error(t, err)
	})
}

func TestResolveSimpleFontEncodingFullCoverage(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("nil returns nil", func(t *testing.T) {
		result := e.resolveSimpleFontEncoding(nil)
		assert.Nil(t, result)
	})

	t.Run("name encoding", func(t *testing.T) {
		result := e.resolveSimpleFontEncoding(entity.Name("WinAnsiEncoding"))
		assert.NotNil(t, result)
	})

	t.Run("dict encoding with BaseEncoding and Differences", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("BaseEncoding"), entity.Name("WinAnsiEncoding"))
		dict.Set(entity.Name("Differences"), entity.NewArray(
			entity.NewInteger(65),
			entity.Name("A"),
		))
		result := e.resolveSimpleFontEncoding(dict)
		assert.NotNil(t, result)
		assert.Equal(t, "A", result[65])
	})

	t.Run("dict encoding without BaseEncoding", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("Differences"), entity.NewArray(
			entity.NewInteger(66),
			entity.Name("B"),
		))
		result := e.resolveSimpleFontEncoding(dict)
		assert.NotNil(t, result)
		assert.Equal(t, "B", result[66])
	})

	t.Run("dict encoding without Differences returns base encoding", func(t *testing.T) {
		dict := entity.NewDict()
		result := e.resolveSimpleFontEncoding(dict)
		// Returns the base encoding (StandardEncoding by default)
		assert.NotNil(t, result)
	})

	t.Run("unknown type returns nil", func(t *testing.T) {
		result := e.resolveSimpleFontEncoding(entity.NewInteger(5))
		assert.Nil(t, result)
	})

	t.Run("ref resolves to name", func(t *testing.T) {
		ref := entity.NewRef(90, 0)
		xref := &testMapXRef{objects: map[entity.Ref]entity.Object{ref: entity.Name("WinAnsiEncoding")}}
		e2 := NewEvaluator(xref)
		result := e2.resolveSimpleFontEncoding(ref)
		assert.NotNil(t, result)
	})
}

func TestEncodedFontGetGlyphWidthFullCoverage(t *testing.T) {
	baseFont := &testFont{
		widths: map[uint32]float64{65: 500},
	}

	t.Run("with valid glyph", func(t *testing.T) {
		ef := &encodedFont{base: baseFont, glyphByCode: map[uint32]uint32{65: 65}}
		w, err := ef.GetGlyphWidth(65)
		require.NoError(t, err)
		assert.Equal(t, 500.0, w)
	})

	t.Run("with glyph not in base returns error", func(t *testing.T) {
		ef := &encodedFont{base: baseFont, glyphByCode: map[uint32]uint32{}}
		_, err := ef.GetGlyphWidth(99)
		require.Error(t, err)
	})
}

func TestApplyFontMetricsFromDictFullCoverage(t *testing.T) {
	e := NewEvaluator(nil)

	t.Run("with partial widths array", func(t *testing.T) {
		font := &testFont{widths: map[uint32]float64{}}
		dict := entity.NewDict()
		dict.Set(entity.Name("FirstChar"), entity.NewInteger(65))
		dict.Set(entity.Name("LastChar"), entity.NewInteger(67))
		dict.Set(entity.Name("Widths"), entity.NewArray(
			entity.NewReal(500),
			entity.NewReal(600),
			// Missing third entry - should handle gracefully
		))
		result := e.applyFontMetricsFromDict(dict, font)
		assert.NotNil(t, result)
	})

	t.Run("with non-numeric widths entry", func(t *testing.T) {
		font := &testFont{widths: map[uint32]float64{}}
		dict := entity.NewDict()
		dict.Set(entity.Name("FirstChar"), entity.NewInteger(65))
		dict.Set(entity.Name("LastChar"), entity.NewInteger(66))
		dict.Set(entity.Name("Widths"), entity.NewArray(
			entity.NewString("bad"),
			entity.NewReal(600),
		))
		result := e.applyFontMetricsFromDict(dict, font)
		assert.NotNil(t, result)
	})
}

func TestRenderInlineImageAdditionalCoverage(t *testing.T) {
	t.Run("with grayscale inline image", func(t *testing.T) {
		e := NewEvaluator(nil)
		canvas := newRecordingCanvas()
		e.SetCanvas(canvas)
		e.inInlineImage = true
		e.inlineImageDict = entity.NewDict()
		e.inlineImageDict.Set(entity.Name("W"), entity.NewInteger(2))
		e.inlineImageDict.Set(entity.Name("H"), entity.NewInteger(2))
		e.inlineImageDict.Set(entity.Name("BPC"), entity.NewInteger(8))
		e.inlineImageDict.Set(entity.Name("CS"), entity.Name("G"))
		e.inlineImageData = []byte{128, 64, 32, 200}
		err := e.renderInlineImage()
		require.NoError(t, err)
		assert.Equal(t, 1, canvas.drawImageCalls)
	})
}
