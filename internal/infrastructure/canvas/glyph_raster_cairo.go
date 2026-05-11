//go:build !nocairo

package canvas

/*
#cgo pkg-config: cairo

#include <cairo.h>
#include <stdlib.h>
#include <string.h>

// rasterize_glyph_path renders glyph draw commands onto a Cairo surface.
// cmds format: type(int), followed by coords depending on type:
//   0=moveto: x,y  (2 doubles)
//   1=lineto: x,y  (2 doubles)
//   2=curveto: c1x,c1y,c2x,c2y,x,y (6 doubles)
//   3=close: (0 doubles)
// Returns alpha buffer (width*height bytes), caller must free.
static unsigned char* cairo_rasterize(
    int *cmd_types, double *cmd_coords, int cmd_count,
    int width, int height,
    double offset_x, double offset_y
) {
    if (width <= 0 || height <= 0) return NULL;

    cairo_surface_t *surface = cairo_image_surface_create(CAIRO_FORMAT_A8, width, height);
    if (cairo_surface_status(surface) != CAIRO_STATUS_SUCCESS) {
        cairo_surface_destroy(surface);
        return NULL;
    }

    cairo_t *cr = cairo_create(surface);
    cairo_set_antialias(cr, CAIRO_ANTIALIAS_DEFAULT);
    cairo_set_source_rgba(cr, 1.0, 1.0, 1.0, 1.0); // opaque white on A8 = full alpha

    int coord_idx = 0;
    for (int i = 0; i < cmd_count; i++) {
        switch (cmd_types[i]) {
        case 0: // moveto
            cairo_move_to(cr,
                cmd_coords[coord_idx]   - offset_x,
                cmd_coords[coord_idx+1] - offset_y);
            coord_idx += 2;
            break;
        case 1: // lineto
            cairo_line_to(cr,
                cmd_coords[coord_idx]   - offset_x,
                cmd_coords[coord_idx+1] - offset_y);
            coord_idx += 2;
            break;
        case 2: // curveto
            cairo_curve_to(cr,
                cmd_coords[coord_idx]   - offset_x,
                cmd_coords[coord_idx+1] - offset_y,
                cmd_coords[coord_idx+2] - offset_x,
                cmd_coords[coord_idx+3] - offset_y,
                cmd_coords[coord_idx+4] - offset_x,
                cmd_coords[coord_idx+5] - offset_y);
            coord_idx += 6;
            break;
        case 3: // close
            cairo_close_path(cr);
            break;
        }
    }

    cairo_fill(cr);

    cairo_surface_flush(surface);

    int stride = cairo_image_surface_get_stride(surface);
    unsigned char *src = cairo_image_surface_get_data(surface);
    unsigned char *dst = (unsigned char*)malloc(width * height);

    for (int y = 0; y < height; y++) {
        memcpy(dst + y * width, src + y * stride, width);
    }

    cairo_destroy(cr);
    cairo_surface_destroy(surface);
    return dst;
}
*/
import "C"
import (
	"image"
	"image/color"
	"unsafe"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// CairoRasterStrategy uses Cairo for glyph rasterization.
// This produces pixel-identical output to Poppler (which also uses Cairo).
type CairoRasterStrategy struct{}

func (s *CairoRasterStrategy) Name() string { return "cairo" }

// IsCairoAvailable returns true if Cairo is available.
func IsCairoAvailable() bool { return true }

// defaultCairoStrategyIfAvailable returns a Cairo strategy when available.
func defaultCairoStrategyIfAvailable() GlyphRasterStrategy {
	return &CairoRasterStrategy{}
}

func (s *CairoRasterStrategy) RasterizeGlyphMask(
	drawCmds []glyphDrawCommand,
	dstRect image.Rectangle,
	originCanvasX, originCanvasY float64,
	supersample int,
) *image.Alpha {
	if len(drawCmds) == 0 {
		return image.NewAlpha(image.Rect(0, 0, 1, 1))
	}

	w := dstRect.Dx()
	h := dstRect.Dy()
	if w <= 0 || h <= 0 {
		return image.NewAlpha(image.Rect(0, 0, 1, 1))
	}

	// Build command arrays for C
	types := make([]C.int, len(drawCmds))
	// Max coords: 6 per curveto command
	coords := make([]C.double, 0, len(drawCmds)*6)

	for i, cmd := range drawCmds {
		switch cmd.kind {
		case entity.CmdMoveTo:
			types[i] = 0
			coords = append(coords, C.double(cmd.x), C.double(cmd.y))
		case entity.CmdLineTo:
			types[i] = 1
			coords = append(coords, C.double(cmd.x), C.double(cmd.y))
		case entity.CmdCurveTo:
			types[i] = 2
			coords = append(coords, C.double(cmd.c1x), C.double(cmd.c1y),
				C.double(cmd.c2x), C.double(cmd.c2y),
				C.double(cmd.x), C.double(cmd.y))
		case entity.CmdClose:
			types[i] = 3
		}
	}

	var coordsPtr *C.double
	if len(coords) > 0 {
		coordsPtr = &coords[0]
	}

	result := C.cairo_rasterize(
		&types[0], coordsPtr, C.int(len(drawCmds)),
		C.int(w), C.int(h),
		C.double(float64(dstRect.Min.X)), C.double(float64(dstRect.Min.Y)),
	)
	if result == nil {
		return image.NewAlpha(image.Rect(0, 0, w, h))
	}
	defer C.free(unsafe.Pointer(result))

	// Copy to Go image
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	src := C.GoBytes(unsafe.Pointer(result), C.int(w*h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			mask.SetAlpha(x, y, color.Alpha{A: src[y*w+x]})
		}
	}

	return mask
}
