// Package truetype provides TrueType/OpenType font implementation.
package truetype

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

// Font represents a TrueType/OpenType font.
type Font struct {
	file *FontFile
	name string
	data []byte
}

// NewFont creates a new TrueType font from a file path.
func NewFont(path string) (*Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return NewFontFromBytes(data)
}

// NewFontFromBytes creates a new TrueType font from byte data.
func NewFontFromBytes(data []byte) (*Font, error) {
	r := bytes.NewReader(data)
	file, err := ParseFontFile(r)
	if err != nil {
		return nil, err
	}

	return &Font{
		file: file,
		data: data,
		name: file.GetFontName(),
	}, nil
}

// NewFontFromReader creates a new TrueType font from a reader.
func NewFontFromReader(r io.ReadSeeker) (*Font, error) {
	// Read all data into memory for random access
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return NewFontFromBytes(data)
}

// CharCodeToGlyph maps a character code to a glyph ID.
func (f *Font) CharCodeToGlyph(code uint32) (uint32, error) {
	glyphID, ok := f.file.CharCodeToGlyph(uint16(code))
	if ok {
		return uint32(glyphID), nil
	}

	// Fallback: use charCode as direct glyph index.
	// Per PDF spec 9.6.6.4, for TrueType subset fonts where cmap lookup fails,
	// the character code is used directly as the glyph index.
	numGlyphs := uint16(0)
	if f.file.Maxp != nil {
		numGlyphs = f.file.Maxp.NumGlyphs
	}
	if numGlyphs > 0 && uint16(code) < numGlyphs {
		return uint32(code), nil
	}

	return 0, errors.NotFoundf("glyph", "character code %d", code)
}

// GlyphName returns the name for a glyph ID.
func (f *Font) GlyphName(glyph uint32) string {
	// For now, return a placeholder name
	// A full implementation would use the post table
	return fmt.Sprintf("glyph%d", glyph)
}

// GetGlyphWidth returns the width of a glyph in font units.
func (f *Font) GetGlyphWidth(glyph uint32) (float64, error) {
	width, err := f.file.GetGlyphWidth(uint16(glyph))
	if err != nil {
		return 0, err
	}
	return float64(width), nil
}

// GetBoundingBox returns the bounding box of the font.
func (f *Font) GetBoundingBox() (float64, float64, float64, float64) {
	if f.file == nil || f.file.Head == nil {
		return 0, 0, 0, 0
	}

	// Return font-wide bounding box.
	return float64(f.file.Head.MinX), float64(f.file.Head.MinY),
		float64(f.file.Head.MaxX), float64(f.file.Head.MaxY)
}

// GetGlyphBoundingBox returns the bounding box for a specific glyph.
func (f *Font) GetGlyphBoundingBox(glyph uint32) (float64, float64, float64, float64, error) {
	xMin, yMin, xMax, yMax, err := f.file.GetGlyphBoundingBox(uint16(glyph))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return float64(xMin), float64(yMin), float64(xMax), float64(yMax), nil
}

// renderGlyphGoParser renders a glyph using the pure-Go TrueType outline parser.
func (f *Font) renderGlyphGoParser(glyph uint32, size float64) (*entity.GlyphPath, error) {
	glyphData, err := f.file.GetGlyphData(uint16(glyph))
	if err != nil {
		return nil, err
	}

	var path *GlyphPath
	if glyphData.NumberOfContours < 0 {
		path, err = f.parseCompositeGlyph(glyphData.Instructions)
		if err != nil {
			return nil, fmt.Errorf("failed to parse composite glyph: %w", err)
		}
	} else {
		path, err = f.parseGlyphOutline(glyphData.Instructions, glyphData.NumberOfContours)
		if err != nil {
			return nil, fmt.Errorf("failed to parse glyph outline: %w", err)
		}
	}

	scale := size / float64(f.file.UnitsPerEm())
	scaledPath := f.scalePath(path, scale)
	return f.convertToEntityPath(scaledPath), nil
}

// scalePath scales all coordinates in the path by the given scale factor.
func (f *Font) scalePath(path *GlyphPath, scale float64) *GlyphPath {
	scaled := NewGlyphPath()

	for _, elem := range path.Elements() {
		switch elem.Op {
		case opMoveTo:
			scaled.MoveTo(elem.X*scale, elem.Y*scale)
		case opLineTo:
			scaled.LineTo(elem.X*scale, elem.Y*scale)
		case opQuadTo:
			scaled.QuadTo(elem.CX*scale, elem.CY*scale, elem.X*scale, elem.Y*scale)
		case opClosePath:
			scaled.ClosePath()
		}
	}

	return scaled
}

// convertToEntityPath converts a truetype.GlyphPath to entity.GlyphPath.
func (f *Font) convertToEntityPath(path *GlyphPath) *entity.GlyphPath {
	if path == nil {
		return nil
	}

	entityPath := &entity.GlyphPath{
		Commands: make([]entity.PathCommand, 0, len(path.Elements())),
		Bounds:   [4]float64{0, 0, 500, 0}, // Default bounds
	}

	// Find actual bounds
	var minX, minY, maxX, maxY float64 = 1e100, 1e100, -1e100, -1e100

	for _, elem := range path.Elements() {
		switch elem.Op {
		case opMoveTo:
			// Negate Y: TrueType uses y-up, entity.GlyphPath uses y-down (like Type1 generatePath).
			cmd := &entity.PathMoveTo{X: elem.X, Y: -elem.Y}
			entityPath.Commands = append(entityPath.Commands, cmd)
			if elem.X < minX {
				minX = elem.X
			}
			if elem.X > maxX {
				maxX = elem.X
			}
			if -elem.Y < minY {
				minY = -elem.Y
			}
			if -elem.Y > maxY {
				maxY = -elem.Y
			}

		case opLineTo:
			cmd := &entity.PathLineTo{X: elem.X, Y: -elem.Y}
			entityPath.Commands = append(entityPath.Commands, cmd)
			if elem.X < minX {
				minX = elem.X
			}
			if elem.X > maxX {
				maxX = elem.X
			}
			if -elem.Y < minY {
				minY = -elem.Y
			}
			if -elem.Y > maxY {
				maxY = -elem.Y
			}

		case opQuadTo:
			cmd := &entity.PathCurveTo{X1: elem.CX, Y1: -elem.CY, X2: elem.CX, Y2: -elem.CY, X3: elem.X, Y3: -elem.Y}
			entityPath.Commands = append(entityPath.Commands, cmd)
			// Update bounds for all points
			for _, pt := range []struct{ X, Y float64 }{{elem.CX, -elem.CY}, {elem.X, -elem.Y}} {
				if pt.X < minX {
					minX = pt.X
				}
				if pt.X > maxX {
					maxX = pt.X
				}
				if pt.Y < minY {
					minY = pt.Y
				}
				if pt.Y > maxY {
					maxY = pt.Y
				}
			}

		case opClosePath:
			cmd := &entity.PathClose{}
			entityPath.Commands = append(entityPath.Commands, cmd)
		}
	}

	// Set actual bounds if we found any points
	if minX < 1e100 {
		entityPath.Bounds = [4]float64{minX, minY, maxX, maxY}
	}

	return entityPath
}

// IsCIDFont returns false for TrueType fonts.
func (f *Font) IsCIDFont() bool {
	return false
}

// IsSymbolic returns whether this is a symbolic font.
func (f *Font) IsSymbolic() bool {
	if f.file.OS2 == nil {
		return false
	}
	// Check fsType bit 0 (symbolic)
	return (f.file.OS2.FsType & 1) != 0
}

// UnitsPerEm returns the units per em value.
func (f *Font) UnitsPerEm() uint16 {
	return f.file.UnitsPerEm()
}

// Name returns the font name.
func (f *Font) Name() string {
	return f.name
}

// FontData returns the raw TrueType/OpenType font bytes.
func (f *Font) FontData() []byte {
	return append([]byte(nil), f.data...)
}

// GetAdvanceWidth returns the advance width for a character.
func (f *Font) GetAdvanceWidth(charCode uint32, size float64) (float64, error) {
	glyph, err := f.CharCodeToGlyph(charCode)
	if err != nil {
		// Return default width for missing characters
		return 0, nil
	}

	width, err := f.file.GetGlyphWidth(uint16(glyph))
	if err != nil {
		return 0, err
	}

	// Scale to requested size
	upem := float64(f.file.UnitsPerEm())
	if upem == 0 {
		upem = 1000
	}
	return (float64(width) * size) / upem, nil
}

// HasGlyph returns true if the font contains a glyph for the character.
func (f *Font) HasGlyph(charCode uint32) bool {
	_, ok := f.file.CharCodeToGlyph(uint16(charCode))
	return ok
}

// GetWeight returns the font weight class.
func (f *Font) GetWeight() uint16 {
	if f.file.OS2 == nil {
		return 400 // Normal/Regular
	}
	return f.file.OS2.WeightClass
}

// IsBold returns true if the font is bold.
func (f *Font) IsBold() bool {
	weight := f.GetWeight()
	return weight >= 600 && weight <= 900
}

// IsItalic returns true if the font is italic.
func (f *Font) IsItalic() bool {
	if f.file.Post == nil {
		return false
	}
	return f.file.Post.ItalicAngle != 0
}
