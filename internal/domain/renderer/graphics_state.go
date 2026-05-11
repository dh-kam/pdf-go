package renderer

import (
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
)

// NewGraphicsState creates a new graphics state.
func NewGraphicsState() *GraphicsState {
	identity := [6]float64{1, 0, 0, 1, 0, 0}
	return &GraphicsState{
		transform:       identity,
		baseTransform:   identity,
		textMatrix:      identity,
		textLine:        identity,
		textBaseMatrix:  identity,
		lineWidth:       1.0,
		fillAlpha:       1.0,
		strokeAlpha:     1.0,
		transferActive:  false,
		fillColor:       &ColorSpace{Color: &Color{Hex: "000000"}},
		strokeColor:     &ColorSpace{Color: &Color{Hex: "000000"}},
		fillPattern:     nil,
		strokePattern:   nil,
		fillCS:          "DeviceGray",
		strokeCS:        "DeviceGray",
		font:            nil,
		fontSize:        12.0,
		currentState:    graphics.NewState(),
		path:            NewPath(),
		clipMode:        ClipNonZeroWinding,
		pendingClip:     false,
		pendingClipMode: ClipNonZeroWinding,
	}
}

// GraphicsState represents the current graphics state.
type GraphicsState struct {
	font                 entity.Font
	fontDebugName        string
	fillColor            *ColorSpace
	strokeColor          *ColorSpace
	fillPattern          entity.Pattern
	strokePattern        entity.Pattern
	fillCS               string
	strokeCS             string
	fillPatternBaseCS    string
	strokePatternBaseCS  string
	currentState         *graphics.State
	path                 *Path
	pathClip             *Path
	textMatrix           [6]float64
	textLine             [6]float64
	textBaseMatrix       [6]float64
	textLineX            float64
	textLineY            float64
	textUserCurrentX     float64
	textUserCurrentY     float64
	textUserCurrentValid bool
	transform            [6]float64
	baseTransform        [6]float64
	fillAlpha            float64
	strokeAlpha          float64
	transferRed          [256]uint8
	transferGreen        [256]uint8
	transferBlue         [256]uint8
	transferGray         [256]uint8
	transferActive       bool
	lineWidth            float64
	fontSize             float64
	clipMode             ClipMode
	pendingClip          bool
	pendingClipMode      ClipMode
}

// ClipMode represents the clipping rule.
type ClipMode int

const (
	ClipNonZeroWinding ClipMode = iota
	ClipEvenOdd
)
