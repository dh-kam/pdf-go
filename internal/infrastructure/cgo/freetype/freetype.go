// Package freetype provides CGo wrapper around FreeType library for glyph rendering.
// This gives pixel-identical rendering to Poppler (which also uses FreeType).
//go:build !nofreetype

package freetype

/*
#cgo pkg-config: freetype2

#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_OUTLINE_H
#include FT_SIZES_H
#include <stdlib.h>
#include <string.h>

// Callback data for outline decomposition
typedef struct {
    double *points;   // x,y pairs
    int    *commands; // 0=moveto, 1=lineto, 2=cubeto, 3=close
    int    count;
    int    capacity;
} outline_data;

static void ensure_capacity(outline_data *d, int need) {
    if (d->count + need > d->capacity) {
        int newCap = d->capacity * 2;
        if (newCap < d->count + need) newCap = d->count + need;
        d->points = (double*)realloc(d->points, newCap * 2 * sizeof(double));
        d->commands = (int*)realloc(d->commands, newCap * sizeof(int));
        d->capacity = newCap;
    }
}

static int outline_moveto(const FT_Vector *to, void *user) {
    outline_data *d = (outline_data*)user;
    ensure_capacity(d, 1);
    d->points[d->count*2]   = (double)to->x / 64.0;
    d->points[d->count*2+1] = (double)to->y / 64.0;
    d->commands[d->count] = 0;
    d->count++;
    return 0;
}

static int outline_lineto(const FT_Vector *to, void *user) {
    outline_data *d = (outline_data*)user;
    ensure_capacity(d, 1);
    d->points[d->count*2]   = (double)to->x / 64.0;
    d->points[d->count*2+1] = (double)to->y / 64.0;
    d->commands[d->count] = 1;
    d->count++;
    return 0;
}

static int outline_conicto(const FT_Vector *cp, const FT_Vector *to, void *user) {
    // Convert quadratic to cubic bezier
    outline_data *d = (outline_data*)user;
    ensure_capacity(d, 1);
    // Store control point and endpoint; Go side converts quad->cubic
    d->points[d->count*2]   = (double)cp->x / 64.0;
    d->points[d->count*2+1] = (double)cp->y / 64.0;
    d->commands[d->count] = 4; // quadratic
    d->count++;
    ensure_capacity(d, 1);
    d->points[d->count*2]   = (double)to->x / 64.0;
    d->points[d->count*2+1] = (double)to->y / 64.0;
    d->commands[d->count] = 5; // quadratic endpoint
    d->count++;
    return 0;
}

static int outline_cubicto(const FT_Vector *cp1, const FT_Vector *cp2, const FT_Vector *to, void *user) {
    outline_data *d = (outline_data*)user;
    ensure_capacity(d, 3);
    // cp1
    d->points[d->count*2]   = (double)cp1->x / 64.0;
    d->points[d->count*2+1] = (double)cp1->y / 64.0;
    d->commands[d->count] = 2;
    d->count++;
    // cp2
    d->points[d->count*2]   = (double)cp2->x / 64.0;
    d->points[d->count*2+1] = (double)cp2->y / 64.0;
    d->commands[d->count] = 2;
    d->count++;
    // endpoint
    d->points[d->count*2]   = (double)to->x / 64.0;
    d->points[d->count*2+1] = (double)to->y / 64.0;
    d->commands[d->count] = 2;
    d->count++;
    return 0;
}

// decompose_outline extracts outline points from a loaded glyph.
// Returns number of points, or negative on error.
// select_encoding_charmap selects an Adobe encoding charmap so that
// FT_Get_Char_Index uses the font's own encoding positions rather than Unicode.
// This is required for Type1 math fonts (cmmi, cmsy, etc.) where glyph codes
// are encoding positions (0-127), not Unicode code points.
static void select_encoding_charmap(FT_Face face) {
    // Prefer ADOBE_CUSTOM (fonts with non-standard encoding like OT1, cmmi),
    // then ADOBE_STANDARD, then ADOBE_EXPERT. If none available, keep default.
    FT_Encoding preferred[] = {
        FT_ENCODING_ADOBE_CUSTOM,
        FT_ENCODING_ADOBE_STANDARD,
        FT_ENCODING_ADOBE_EXPERT,
    };
    int n = sizeof(preferred) / sizeof(preferred[0]);
    for (int i = 0; i < n; i++) {
        if (FT_Select_Charmap(face, preferred[i]) == 0) {
            return;
        }
    }
}

static int decompose_outline(FT_Face face, int glyph_index, double size_pt, int dpi,
                             double **out_points, int **out_commands) {
    FT_Set_Char_Size(face, 0, (FT_F26Dot6)(size_pt * 64), dpi, dpi);

    if (FT_Load_Glyph(face, glyph_index, FT_LOAD_NO_BITMAP | FT_LOAD_NO_HINTING)) {
        return -1;
    }

    if (face->glyph->format != FT_GLYPH_FORMAT_OUTLINE) {
        return -2;
    }

    FT_Outline_Funcs funcs;
    memset(&funcs, 0, sizeof(funcs));
    funcs.move_to  = outline_moveto;
    funcs.line_to  = outline_lineto;
    funcs.conic_to = outline_conicto;
    funcs.cubic_to = outline_cubicto;

    outline_data data;
    data.count = 0;
    data.capacity = 64;
    data.points = (double*)malloc(64 * 2 * sizeof(double));
    data.commands = (int*)malloc(64 * sizeof(int));

    if (FT_Outline_Decompose(&face->glyph->outline, &funcs, &data)) {
        free(data.points);
        free(data.commands);
        return -3;
    }

    *out_points = data.points;
    *out_commands = data.commands;
    return data.count;
}

// get_glyph_name_index returns the glyph index for a glyph name (for CFF fonts without cmap).
// Returns 0 if not found.
  static FT_UInt get_glyph_name_index(FT_Face face, const char *name) {
      return FT_Get_Name_Index(face, (FT_String*)name);
  }

  static int get_glyph_name_by_index(FT_Face face, FT_UInt glyph_index, char *out, int out_len) {
      if (out == NULL || out_len <= 0) {
          return 0;
      }
      out[0] = '\0';
      if (FT_Get_Glyph_Name(face, glyph_index, out, out_len) != 0) {
          return 0;
      }
      return out[0] != '\0';
  }

  static int get_face_bbox(FT_Face face, double *x_min, double *y_min, double *x_max, double *y_max, int *units_per_em) {
      if (face == NULL || face->units_per_EM == 0) {
          return 0;
      }
      int div = face->bbox.xMax > 20000 ? 65536 : 1;
      *x_min = (double)face->bbox.xMin / div;
      *y_min = (double)face->bbox.yMin / div;
      *x_max = (double)face->bbox.xMax / div;
      *y_max = (double)face->bbox.yMax / div;
      *units_per_em = face->units_per_EM;
      return 1;
  }

  static int select_poppler_size_object_for_probe(FT_Face face) {
      const char *mode = getenv("PDF_DEBUG_FT_NEW_SIZE");
      if (mode == NULL || strcmp(mode, "1") != 0) {
          return 0;
      }
      FT_Size size_obj;
      if (FT_New_Size(face, &size_obj)) {
          return -20;
      }
      face->size = size_obj;
      return 0;
  }

  static int set_poppler_glyph_transform_pixels(FT_Face face, double pixel_size_x, double pixel_size_y,
                                                 FT_Pos phase_x26dot6, FT_Pos phase_y26dot6) {
      int size_rc = select_poppler_size_object_for_probe(face);
      if (size_rc != 0) {
          return size_rc;
      }
      int size = (int)(pixel_size_y + 0.5);
      if (size < 1) {
          size = 1;
      }
      if (FT_Set_Pixel_Sizes(face, 0, size)) {
          return -10;
      }

      FT_Matrix matrix;
      matrix.xx = (FT_Fixed)((pixel_size_x / (double)size) * 65536.0);
      matrix.xy = 0;
      matrix.yx = 0;
      matrix.yy = (FT_Fixed)((pixel_size_y / (double)size) * 65536.0);

      FT_Vector offset;
      offset.x = phase_x26dot6;
      offset.y = phase_y26dot6;
      FT_Set_Transform(face, &matrix, &offset);
      return 0;
  }

  static int set_poppler_glyph_transform(FT_Face face, double size_pt, int dpi,
                                          FT_Pos phase_x26dot6, FT_Pos phase_y26dot6) {
      double pixel_size = size_pt * (double)dpi / 72.0;
      return set_poppler_glyph_transform_pixels(face, pixel_size, pixel_size, phase_x26dot6, phase_y26dot6);
  }

  static FT_Int32 bitmap_load_flags_for_probe(void) {
      const char *mode = getenv("PDF_DEBUG_FT_LOAD_FLAGS");
      if (mode != NULL) {
          if (strcmp(mode, "default") == 0) {
              return FT_LOAD_DEFAULT | FT_LOAD_NO_BITMAP;
          }
          if (strcmp(mode, "light") == 0) {
              return FT_LOAD_DEFAULT | FT_LOAD_NO_BITMAP | FT_LOAD_TARGET_LIGHT;
          }
          if (strcmp(mode, "no_auto_hint") == 0) {
              return FT_LOAD_DEFAULT | FT_LOAD_NO_BITMAP | FT_LOAD_NO_AUTOHINT;
          }
      }
      return FT_LOAD_NO_HINTING | FT_LOAD_NO_BITMAP;
  }

  // render_glyph_bitmap renders a glyph to a grayscale bitmap using FreeType's rasterizer.
  // Returns 0 on success, negative on error.
  // out_buffer contains width*height bytes of alpha values.
  static int render_glyph_bitmap(FT_Face face, int glyph_index, double size_pt, int dpi,
                                  unsigned char **out_buffer, int *out_width, int *out_height,
                                  int *out_left, int *out_top) {
      int transform_rc = set_poppler_glyph_transform(face, size_pt, dpi, 0, 0);
      if (transform_rc != 0) {
          return transform_rc;
      }

      if (FT_Load_Glyph(face, glyph_index, bitmap_load_flags_for_probe())) {
          return -1;
      }

    if (FT_Render_Glyph(face->glyph, FT_RENDER_MODE_NORMAL)) {
        return -2;
    }

    FT_Bitmap *bmp = &face->glyph->bitmap;
    if (bmp->pixel_mode != FT_PIXEL_MODE_GRAY || bmp->width == 0 || bmp->rows == 0) {
        *out_width = 0;
        *out_height = 0;
        *out_buffer = NULL;
        return 0;
    }

    int w = bmp->width;
    int h = bmp->rows;
    int size = w * h;
    unsigned char *buf = (unsigned char*)malloc(size);
    for (int y = 0; y < h; y++) {
        memcpy(buf + y * w, bmp->buffer + y * bmp->pitch, w);
    }

    *out_buffer = buf;
    *out_width = w;
    *out_height = h;
    *out_left = face->glyph->bitmap_left;
    *out_top = face->glyph->bitmap_top;
    return 0;
}

// render_glyph_bitmap_phased renders a glyph with sub-pixel phase offset for accurate antialiasing.
// phase_x26dot6: X phase in FreeType 26.6 format (0-63 = 0 to <1 pixel rightward shift).
// phase_y26dot6: Y phase in FreeType 26.6 format (negative = downward shift in FT Y-up coords).
  static int render_glyph_bitmap_phased(FT_Face face, int glyph_index, double size_pt, int dpi,
                                        FT_Pos phase_x26dot6, FT_Pos phase_y26dot6,
                                        unsigned char **out_buffer, int *out_width, int *out_height,
                                        int *out_left, int *out_top) {
      int transform_rc = set_poppler_glyph_transform(face, size_pt, dpi, phase_x26dot6, phase_y26dot6);
      if (transform_rc != 0) {
          return transform_rc;
      }

      if (FT_Load_Glyph(face, glyph_index, bitmap_load_flags_for_probe())) {
          return -1;
      }

      if (FT_Render_Glyph(face->glyph, FT_RENDER_MODE_NORMAL)) {
          return -2;
      }

    FT_Bitmap *bmp = &face->glyph->bitmap;
    if (bmp->pixel_mode != FT_PIXEL_MODE_GRAY || bmp->width == 0 || bmp->rows == 0) {
        *out_width = 0;
        *out_height = 0;
        *out_buffer = NULL;
        return 0;
    }

    int w = bmp->width;
    int h = bmp->rows;
    int size = w * h;
    unsigned char *buf = (unsigned char*)malloc(size);
    for (int y = 0; y < h; y++) {
        memcpy(buf + y * w, bmp->buffer + y * bmp->pitch, w);
    }

    *out_buffer = buf;
    *out_width = w;
    *out_height = h;
    *out_left = face->glyph->bitmap_left;
      *out_top = face->glyph->bitmap_top;
      return 0;
  }

  static int render_glyph_bitmap_transformed(FT_Face face, int glyph_index, double size_pt,
                                             double scale_x, double scale_y,
                                             FT_Pos phase_x26dot6, FT_Pos phase_y26dot6,
                                             unsigned char **out_buffer, int *out_width, int *out_height,
                                             int *out_left, int *out_top) {
      double pixel_size_x = size_pt * scale_x;
      double pixel_size_y = size_pt * scale_y;
      int transform_rc = set_poppler_glyph_transform_pixels(face, pixel_size_x, pixel_size_y, phase_x26dot6, phase_y26dot6);
      if (transform_rc != 0) {
          return transform_rc;
      }

      if (FT_Load_Glyph(face, glyph_index, bitmap_load_flags_for_probe())) {
          return -1;
      }

      if (FT_Render_Glyph(face->glyph, FT_RENDER_MODE_NORMAL)) {
          return -2;
      }

      FT_Bitmap *bmp = &face->glyph->bitmap;
      if (bmp->pixel_mode != FT_PIXEL_MODE_GRAY || bmp->width == 0 || bmp->rows == 0) {
          *out_width = 0;
          *out_height = 0;
          *out_buffer = NULL;
          return 0;
      }

      int w = bmp->width;
      int h = bmp->rows;
      int size = w * h;
      unsigned char *buf = (unsigned char*)malloc(size);
      for (int y = 0; y < h; y++) {
          memcpy(buf + y * w, bmp->buffer + y * bmp->pitch, w);
      }

      *out_buffer = buf;
      *out_width = w;
      *out_height = h;
      *out_left = face->glyph->bitmap_left;
      *out_top = face->glyph->bitmap_top;
      return 0;
  }

  static int render_glyph_bitmap_matrix(FT_Face face, int glyph_index, int size,
                                        double mxx, double myx, double mxy, double myy,
                                        FT_Pos phase_x26dot6, FT_Pos phase_y26dot6,
                                        unsigned char **out_buffer, int *out_width, int *out_height,
                                        int *out_left, int *out_top) {
      int size_rc = select_poppler_size_object_for_probe(face);
      if (size_rc != 0) {
          return size_rc;
      }
      if (size < 1) {
          size = 1;
      }
      if (FT_Set_Pixel_Sizes(face, 0, size)) {
          return -10;
      }

      FT_Matrix matrix;
      matrix.xx = (FT_Fixed)(mxx * 65536.0);
      matrix.yx = (FT_Fixed)(myx * 65536.0);
      matrix.xy = (FT_Fixed)(mxy * 65536.0);
      matrix.yy = (FT_Fixed)(myy * 65536.0);

      FT_Vector offset;
      offset.x = phase_x26dot6;
      offset.y = phase_y26dot6;
      FT_Set_Transform(face, &matrix, &offset);

      if (FT_Load_Glyph(face, glyph_index, bitmap_load_flags_for_probe())) {
          return -1;
      }

      if (FT_Render_Glyph(face->glyph, FT_RENDER_MODE_NORMAL)) {
          return -2;
      }

      FT_Bitmap *bmp = &face->glyph->bitmap;
      if (bmp->pixel_mode != FT_PIXEL_MODE_GRAY || bmp->width == 0 || bmp->rows == 0) {
          *out_width = 0;
          *out_height = 0;
          *out_buffer = NULL;
          return 0;
      }

      int w = bmp->width;
      int h = bmp->rows;
      int alloc_size = w * h;
      unsigned char *buf = (unsigned char*)malloc(alloc_size);
      for (int y = 0; y < h; y++) {
          memcpy(buf + y * w, bmp->buffer + y * bmp->pitch, w);
      }

      *out_buffer = buf;
      *out_width = w;
      *out_height = h;
      *out_left = face->glyph->bitmap_left;
      *out_top = face->glyph->bitmap_top;
      return 0;
  }

*/
import "C"
import (
	"fmt"
	"math"
	"sync"
	"unsafe"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

var (
	ftLibrary C.FT_Library
	ftOnce    sync.Once
	ftErr     error
)

func initFreeType() {
	if C.FT_Init_FreeType(&ftLibrary) != 0 {
		ftErr = fmt.Errorf("failed to initialize FreeType")
	}
}

// IsAvailable returns true if FreeType library is available.
func IsAvailable() bool {
	ftOnce.Do(initFreeType)
	return ftErr == nil
}

// RenderGlyph renders a glyph outline using FreeType (same as Poppler).
// fontData is OTF/CFF binary, glyphCode is the character code,
// size is in points, dpi is dots per inch.
func RenderGlyph(fontData []byte, glyphCode uint32, size float64, dpi int) (*entity.GlyphPath, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, ftErr
	}

	// Load font face from memory
	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	// Select Adobe encoding charmap so that FT_Get_Char_Index uses the font's
	// own encoding positions (required for Type1 math fonts like cmmi, cmsy).
	C.select_encoding_charmap(face)

	// Get glyph index from char code
	glyphIndex := C.FT_Get_Char_Index(face, C.FT_ULong(glyphCode))
	if glyphIndex == 0 {
		return nil, fmt.Errorf("FreeType: glyph not found for code %d", glyphCode)
	}

	// Decompose outline
	var outPoints *C.double
	var outCommands *C.int
	n := C.decompose_outline(face, C.int(glyphIndex), C.double(size), C.int(dpi), &outPoints, &outCommands)
	if n < 0 {
		return nil, fmt.Errorf("FreeType: outline decomposition failed (%d)", n)
	}
	if n == 0 {
		return nil, fmt.Errorf("FreeType: empty glyph")
	}
	defer C.free(unsafe.Pointer(outPoints))
	defer C.free(unsafe.Pointer(outCommands))

	// Convert to entity.GlyphPath
	points := unsafe.Slice((*[2]C.double)(unsafe.Pointer(outPoints)), int(n))
	commands := unsafe.Slice((*C.int)(unsafe.Pointer(outCommands)), int(n))

	path := &entity.GlyphPath{
		Commands: make([]entity.PathCommand, 0, int(n)),
	}
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64

	updateBounds := func(x, y float64) {
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	i := 0
	for i < int(n) {
		x := float64(points[i][0])
		y := float64(points[i][1])
		cmd := int(commands[i])

		switch cmd {
		case 0: // moveto
			path.Commands = append(path.Commands, &entity.PathMoveTo{X: x, Y: -y})
			updateBounds(x, -y)
			i++
		case 1: // lineto
			path.Commands = append(path.Commands, &entity.PathLineTo{X: x, Y: -y})
			updateBounds(x, -y)
			i++
		case 2: // cubicto (3 entries: cp1, cp2, endpoint)
			if i+2 < int(n) {
				c1x := float64(points[i][0])
				c1y := float64(points[i][1])
				c2x := float64(points[i+1][0])
				c2y := float64(points[i+1][1])
				ex := float64(points[i+2][0])
				ey := float64(points[i+2][1])
				path.Commands = append(path.Commands, &entity.PathCurveTo{
					X1: c1x, Y1: -c1y, X2: c2x, Y2: -c2y, X3: ex, Y3: -ey,
				})
				updateBounds(c1x, -c1y)
				updateBounds(c2x, -c2y)
				updateBounds(ex, -ey)
				i += 3
			} else {
				i++
			}
		case 4: // quadratic cp
			if i+1 < int(n) {
				cpx := float64(points[i][0])
				cpy := float64(points[i][1])
				ex := float64(points[i+1][0])
				ey := float64(points[i+1][1])
				// Convert quadratic to cubic (same control point for both)
				path.Commands = append(path.Commands, &entity.PathCurveTo{
					X1: cpx, Y1: -cpy, X2: cpx, Y2: -cpy, X3: ex, Y3: -ey,
				})
				updateBounds(cpx, -cpy)
				updateBounds(ex, -ey)
				i += 2
			} else {
				i++
			}
		case 5: // quadratic endpoint (handled with case 4)
			i++
		default:
			i++
		}
	}

	if minX == math.MaxFloat64 {
		return nil, fmt.Errorf("FreeType: no outline points")
	}
	path.Bounds = [4]float64{minX, minY, maxX, maxY}
	return path, nil
}

// GetGlyphIndexByCharCode returns the FreeType glyph index for a char code in fontData.
// Selects the Adobe encoding charmap so that TeX/CM fonts use their built-in encoding.
// Returns 0, false if the glyph is not found.
func GetGlyphIndexByCharCode(fontData []byte, charCode uint32) (uint32, bool) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return 0, false
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return 0, false
	}
	defer C.FT_Done_Face(face)

	C.select_encoding_charmap(face)

	idx := C.FT_Get_Char_Index(face, C.FT_ULong(charCode))
	if idx == 0 {
		return 0, false
	}
	return uint32(idx), true
}

// GetGlyphIndexByName returns the FreeType glyph index for a named glyph in fontData.
// This is used for CFF fonts where glyph names (not char codes) identify glyphs.
// Returns 0, false if the glyph is not found.
func GetGlyphIndexByName(fontData []byte, glyphName string) (uint32, bool) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return 0, false
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return 0, false
	}
	defer C.FT_Done_Face(face)

	cName := C.CString(glyphName)
	defer C.free(unsafe.Pointer(cName))

	idx := C.get_glyph_name_index(face, cName)
	if idx == 0 {
		return 0, false
	}
	return uint32(idx), true
}

// GetGlyphNameByCharCode returns the glyph name selected by the font's own
// Adobe encoding charmap for the given character code.
func GetGlyphNameByCharCode(fontData []byte, charCode uint32) (string, bool) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return "", false
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return "", false
	}
	defer C.FT_Done_Face(face)

	C.select_encoding_charmap(face)

	idx := C.FT_Get_Char_Index(face, C.FT_ULong(charCode))
	if idx == 0 {
		return "", false
	}

	var name [128]C.char
	if C.get_glyph_name_by_index(face, idx, &name[0], C.int(len(name))) == 0 {
		return "", false
	}
	return C.GoString(&name[0]), true
}

// GetFaceBoundingBox returns FreeType's face bbox and units-per-em.
func GetFaceBoundingBox(fontData []byte) (float64, float64, float64, float64, uint16, bool) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return 0, 0, 0, 0, 0, false
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return 0, 0, 0, 0, 0, false
	}
	defer C.FT_Done_Face(face)

	var xMin, yMin, xMax, yMax C.double
	var units C.int
	if C.get_face_bbox(face, &xMin, &yMin, &xMax, &yMax, &units) == 0 {
		return 0, 0, 0, 0, 0, false
	}
	if units <= 0 {
		return 0, 0, 0, 0, 0, false
	}
	return float64(xMin), float64(yMin), float64(xMax), float64(yMax), uint16(units), true
}

// RenderGlyphByIndex renders a glyph outline using its FreeType glyph index directly.
// Unlike RenderGlyph, this skips charmap lookup — required for CFF fonts without cmap.
func RenderGlyphByIndex(fontData []byte, glyphIndex uint32, sizePt float64, dpi int) (*entity.GlyphPath, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	var outPoints *C.double
	var outCommands *C.int
	n := C.decompose_outline(face, C.int(glyphIndex), C.double(sizePt), C.int(dpi), &outPoints, &outCommands)
	if n < 0 {
		return nil, fmt.Errorf("FreeType: outline decomposition failed (%d)", n)
	}
	if n == 0 {
		return nil, fmt.Errorf("FreeType: empty glyph")
	}
	defer C.free(unsafe.Pointer(outPoints))
	defer C.free(unsafe.Pointer(outCommands))

	points := unsafe.Slice((*[2]C.double)(unsafe.Pointer(outPoints)), int(n))
	commands := unsafe.Slice((*C.int)(unsafe.Pointer(outCommands)), int(n))

	path := &entity.GlyphPath{
		Commands: make([]entity.PathCommand, 0, int(n)),
	}
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64

	updateBounds := func(x, y float64) {
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	i := 0
	for i < int(n) {
		x := float64(points[i][0])
		y := float64(points[i][1])
		cmd := int(commands[i])

		switch cmd {
		case 0:
			path.Commands = append(path.Commands, &entity.PathMoveTo{X: x, Y: -y})
			updateBounds(x, -y)
			i++
		case 1:
			path.Commands = append(path.Commands, &entity.PathLineTo{X: x, Y: -y})
			updateBounds(x, -y)
			i++
		case 2:
			if i+2 < int(n) {
				c1x := float64(points[i][0])
				c1y := float64(points[i][1])
				c2x := float64(points[i+1][0])
				c2y := float64(points[i+1][1])
				ex := float64(points[i+2][0])
				ey := float64(points[i+2][1])
				path.Commands = append(path.Commands, &entity.PathCurveTo{
					X1: c1x, Y1: -c1y, X2: c2x, Y2: -c2y, X3: ex, Y3: -ey,
				})
				updateBounds(c1x, -c1y)
				updateBounds(c2x, -c2y)
				updateBounds(ex, -ey)
				i += 3
			} else {
				i++
			}
		case 4:
			if i+1 < int(n) {
				cpx := float64(points[i][0])
				cpy := float64(points[i][1])
				ex := float64(points[i+1][0])
				ey := float64(points[i+1][1])
				path.Commands = append(path.Commands, &entity.PathCurveTo{
					X1: cpx, Y1: -cpy, X2: cpx, Y2: -cpy, X3: ex, Y3: -ey,
				})
				updateBounds(cpx, -cpy)
				updateBounds(ex, -ey)
				i += 2
			} else {
				i++
			}
		case 5:
			i++
		default:
			i++
		}
	}

	if minX == math.MaxFloat64 {
		return nil, fmt.Errorf("FreeType: no outline points")
	}
	path.Bounds = [4]float64{minX, minY, maxX, maxY}
	return path, nil
}

// RenderGlyphBitmapPhased renders a glyph bitmap with sub-pixel phase for accurate antialiasing.
// phaseX, phaseY are fractional pixel offsets (0.0 to <1.0): phaseX shifts glyph rightward,
// phaseY shifts glyph downward in canvas coordinates (converted to FT Y-up internally).
func RenderGlyphBitmapPhased(fontData []byte, glyphCode uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, 0, 0, 0, 0, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	C.select_encoding_charmap(face)

	glyphIndex := C.FT_Get_Char_Index(face, C.FT_ULong(glyphCode))
	if glyphIndex == 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: glyph not found for code %d", glyphCode)
	}

	px := C.FT_Pos(math.Round(phaseX * 64))
	py := C.FT_Pos(math.Round(-phaseY * 64)) // phaseY is canvas-down, FT is Y-up
	var outBuf *C.uchar
	var outW, outH, outLeft, outTop C.int
	rc := C.render_glyph_bitmap_phased(face, C.int(glyphIndex), C.double(sizePt), C.int(dpi),
		px, py, &outBuf, &outW, &outH, &outLeft, &outTop)
	if rc != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: phased render failed (%d)", rc)
	}

	w := int(outW)
	h := int(outH)
	if w == 0 || h == 0 || outBuf == nil {
		return nil, 0, 0, int(outLeft), int(outTop), nil
	}
	defer C.free(unsafe.Pointer(outBuf))

	buf := C.GoBytes(unsafe.Pointer(outBuf), C.int(w*h))
	return buf, w, h, int(outLeft), int(outTop), nil
}

// RenderGlyphBitmapByIndexPhased renders a glyph bitmap by FT index with sub-pixel phase.
func RenderGlyphBitmapByIndexPhased(fontData []byte, glyphIndex uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, 0, 0, 0, 0, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	px := C.FT_Pos(math.Round(phaseX * 64))
	py := C.FT_Pos(math.Round(-phaseY * 64))
	var outBuf *C.uchar
	var outW, outH, outLeft, outTop C.int
	rc := C.render_glyph_bitmap_phased(face, C.int(glyphIndex), C.double(sizePt), C.int(dpi),
		px, py, &outBuf, &outW, &outH, &outLeft, &outTop)
	if rc != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: phased render failed (%d)", rc)
	}

	w := int(outW)
	h := int(outH)
	if w == 0 || h == 0 || outBuf == nil {
		return nil, 0, 0, int(outLeft), int(outTop), nil
	}
	defer C.free(unsafe.Pointer(outBuf))

	buf := C.GoBytes(unsafe.Pointer(outBuf), C.int(w*h))
	return buf, w, h, int(outLeft), int(outTop), nil
}

// RenderGlyphBitmapTransformedPhased renders a glyph bitmap with Poppler-style
// axis-aligned FreeType transform scaling and sub-pixel phase.
func RenderGlyphBitmapTransformedPhased(fontData []byte, glyphCode uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, 0, 0, 0, 0, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	C.select_encoding_charmap(face)

	glyphIndex := C.FT_Get_Char_Index(face, C.FT_ULong(glyphCode))
	if glyphIndex == 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: glyph not found for code %d", glyphCode)
	}

	return renderGlyphBitmapTransformedByIndex(face, glyphIndex, sizePt, scaleX, scaleY, phaseX, phaseY)
}

// RenderGlyphBitmapByIndexTransformedPhased renders a glyph bitmap by FT index
// with Poppler-style axis-aligned FreeType transform scaling and phase.
func RenderGlyphBitmapByIndexTransformedPhased(fontData []byte, glyphIndex uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, 0, 0, 0, 0, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	return renderGlyphBitmapTransformedByIndex(face, C.FT_UInt(glyphIndex), sizePt, scaleX, scaleY, phaseX, phaseY)
}

func renderGlyphBitmapTransformedByIndex(face C.FT_Face, glyphIndex C.FT_UInt, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	px := C.FT_Pos(math.Floor(phaseX * 64))
	py := C.FT_Pos(math.Floor(-phaseY * 64))
	var outBuf *C.uchar
	var outW, outH, outLeft, outTop C.int
	rc := C.render_glyph_bitmap_transformed(face, C.int(glyphIndex), C.double(sizePt), C.double(scaleX), C.double(scaleY),
		px, py, &outBuf, &outW, &outH, &outLeft, &outTop)
	if rc != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: transformed phased render failed (%d)", rc)
	}

	w := int(outW)
	h := int(outH)
	if w == 0 || h == 0 || outBuf == nil {
		return nil, 0, 0, int(outLeft), int(outTop), nil
	}
	defer C.free(unsafe.Pointer(outBuf))

	buf := C.GoBytes(unsafe.Pointer(outBuf), C.int(w*h))
	return buf, w, h, int(outLeft), int(outTop), nil
}

// RenderGlyphBitmapMatrixPhased renders a glyph bitmap with Poppler's full
// SplashFTFont 2x2 transform matrix and sub-pixel phase.
func RenderGlyphBitmapMatrixPhased(fontData []byte, glyphCode uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, 0, 0, 0, 0, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	C.select_encoding_charmap(face)

	glyphIndex := C.FT_Get_Char_Index(face, C.FT_ULong(glyphCode))
	if glyphIndex == 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: glyph not found for code %d", glyphCode)
	}

	return renderGlyphBitmapMatrixByIndex(face, glyphIndex, sizePt, matrix, phaseX, phaseY)
}

// RenderGlyphBitmapByIndexMatrixPhased renders a glyph bitmap by FT index with
// Poppler's full SplashFTFont 2x2 transform matrix and sub-pixel phase.
func RenderGlyphBitmapByIndexMatrixPhased(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, 0, 0, 0, 0, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	return renderGlyphBitmapMatrixByIndex(face, C.FT_UInt(glyphIndex), sizePt, matrix, phaseX, phaseY)
}

func renderGlyphBitmapMatrixByIndex(face C.FT_Face, glyphIndex C.FT_UInt, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	mat := [4]float64{
		sizePt * matrix[0],
		sizePt * matrix[1],
		sizePt * matrix[2],
		sizePt * matrix[3],
	}
	size := int(math.Floor(math.Hypot(mat[2], mat[3]) + 0.5))
	if size < 1 {
		size = 1
	}
	sizeF := float64(size)
	px := C.FT_Pos(math.Floor(phaseX * 64))
	py := C.FT_Pos(math.Floor(-phaseY * 64))
	var outBuf *C.uchar
	var outW, outH, outLeft, outTop C.int
	rc := C.render_glyph_bitmap_matrix(face, C.int(glyphIndex), C.int(size),
		C.double(mat[0]/sizeF), C.double(mat[1]/sizeF), C.double(mat[2]/sizeF), C.double(mat[3]/sizeF),
		px, py, &outBuf, &outW, &outH, &outLeft, &outTop)
	if rc != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: matrix phased render failed (%d)", rc)
	}

	w := int(outW)
	h := int(outH)
	if w == 0 || h == 0 || outBuf == nil {
		return nil, 0, 0, int(outLeft), int(outTop), nil
	}
	defer C.free(unsafe.Pointer(outBuf))

	buf := C.GoBytes(unsafe.Pointer(outBuf), C.int(w*h))
	return buf, w, h, int(outLeft), int(outTop), nil
}

// RenderGlyphBitmapByIndex renders a glyph bitmap using its FreeType glyph index directly.
// Like RenderGlyphBitmap but skips charmap lookup — required for CFF fonts without cmap.
func RenderGlyphBitmapByIndex(fontData []byte, glyphIndex uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, 0, 0, 0, 0, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	var outBuf *C.uchar
	var outW, outH, outLeft, outTop C.int
	rc := C.render_glyph_bitmap(face, C.int(glyphIndex), C.double(sizePt), C.int(dpi),
		&outBuf, &outW, &outH, &outLeft, &outTop)
	if rc != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: render failed (%d)", rc)
	}

	w := int(outW)
	h := int(outH)
	if w == 0 || h == 0 || outBuf == nil {
		return nil, 0, 0, int(outLeft), int(outTop), nil
	}
	defer C.free(unsafe.Pointer(outBuf))

	buf := C.GoBytes(unsafe.Pointer(outBuf), C.int(w*h))
	return buf, w, h, int(outLeft), int(outTop), nil
}

// RenderGlyphBitmap renders a glyph to a grayscale bitmap using FreeType's rasterizer.
// Returns alpha buffer, width, height, bearingX (left), bearingY (top), error.
func RenderGlyphBitmap(fontData []byte, glyphCode uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	ftOnce.Do(initFreeType)
	if ftErr != nil {
		return nil, 0, 0, 0, 0, ftErr
	}

	var face C.FT_Face
	cData := C.CBytes(fontData)
	defer C.free(cData)

	if C.FT_New_Memory_Face(ftLibrary, (*C.FT_Byte)(cData), C.FT_Long(len(fontData)), 0, &face) != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: failed to load font face")
	}
	defer C.FT_Done_Face(face)

	C.select_encoding_charmap(face)

	glyphIndex := C.FT_Get_Char_Index(face, C.FT_ULong(glyphCode))
	if glyphIndex == 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: glyph not found for code %d", glyphCode)
	}

	var outBuf *C.uchar
	var outW, outH, outLeft, outTop C.int
	rc := C.render_glyph_bitmap(face, C.int(glyphIndex), C.double(sizePt), C.int(dpi),
		&outBuf, &outW, &outH, &outLeft, &outTop)
	if rc != 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType: render failed (%d)", rc)
	}

	w := int(outW)
	h := int(outH)
	if w == 0 || h == 0 || outBuf == nil {
		return nil, 0, 0, int(outLeft), int(outTop), nil // empty glyph (e.g., space)
	}
	defer C.free(unsafe.Pointer(outBuf))

	buf := C.GoBytes(unsafe.Pointer(outBuf), C.int(w*h))
	return buf, w, h, int(outLeft), int(outTop), nil
}
