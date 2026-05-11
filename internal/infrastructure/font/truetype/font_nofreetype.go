//go:build nofreetype

package truetype

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// RenderGlyph renders a glyph to a path using our pure-Go TrueType parser.
func (f *Font) RenderGlyph(glyph uint32, size float64) (result *entity.GlyphPath, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			resultErr = fmt.Errorf("glyph render panic: %v", r)
		}
	}()

	return f.renderGlyphGoParser(glyph, size)
}
