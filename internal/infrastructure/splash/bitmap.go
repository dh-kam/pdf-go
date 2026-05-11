package splash

// Bitmap is the raster target buffer (SplashBitmap.h:49).
type Bitmap struct {
	width   int
	height  int
	rowSize int
	mode    ColorMode
	data    []byte
	alpha   []byte
}

// NewBitmap constructs a Bitmap (SplashBitmap.h:56, SplashBitmap.cc ctor).
//
// The C++ ctor allocates `data` (rowSize * height) and, when alpha=true,
// `alpha` (width * height). The Phase-0 stub left both planes nil — pipeInit
// would then read/write a zero-length slice and panic. We now allocate both
// so the production path (NewBackend → New → Fill/Stroke → pipeRunAARGB8)
// has live row buffers to address.
func NewBitmap(width, height int, mode ColorMode, alpha bool) *Bitmap {
	bpp := bytesPerPixel(mode)
	rowSize := width * bpp
	b := &Bitmap{
		width:   width,
		height:  height,
		rowSize: rowSize,
		mode:    mode,
	}
	if width > 0 && height > 0 {
		b.data = make([]byte, rowSize*height)
		if alpha {
			b.alpha = make([]byte, width*height)
		}
	}
	return b
}

// Width returns the pixel width (SplashBitmap.h:64).
func (b *Bitmap) Width() int { return b.width }

// Height returns the pixel height (SplashBitmap.h:65).
func (b *Bitmap) Height() int { return b.height }

// RowSize returns the byte stride of one row (SplashBitmap.h:66).
func (b *Bitmap) RowSize() int { return b.rowSize }

// Mode returns the pixel format (SplashBitmap.h).
func (b *Bitmap) Mode() ColorMode { return b.mode }

// Pixel returns the color at (x, y) (SplashBitmap.h:102).
func (b *Bitmap) Pixel(x, y int) Color { return Color{} }

// Clear fills the data plane with c and sets the alpha plane (if present)
// to 0xFF (opaque). Mirrors Splash::clear with an opaque alpha for the
// modes Phase 2 cares about (Mono8, RGB8, BGR8, XBGR8, CMYK8, DeviceN8).
//
// Hotfix #2: NewBackend needs paper-white as the bitmap default so that
// fills and strokes paint visible glyphs/rects on a real white page rather
// than blending into a transparent-black initial buffer.
func (b *Bitmap) Clear(c Color) {
	b.ClearWithAlpha(c, 0xFF)
}

// ClearWithAlpha fills the data plane with c and the alpha plane with alpha.
// Poppler's SplashOutputDev::startPage calls Splash::clear(paperColor, 0),
// then endPage composites the alpha plane over the paper color.
func (b *Bitmap) ClearWithAlpha(c Color, alpha byte) {
	if b == nil || len(b.data) == 0 {
		return
	}
	bpp := bytesPerPixel(b.mode)
	if bpp <= 0 {
		return
	}
	for y := 0; y < b.height; y++ {
		off := y * b.rowSize
		for x := 0; x < b.width; x++ {
			for i := 0; i < bpp; i++ {
				b.data[off+i] = c[i]
			}
			off += bpp
		}
	}
	if b.alpha != nil {
		for i := range b.alpha {
			b.alpha[i] = alpha
		}
	}
}

// Data returns the raw pixel slice (SplashBitmap.h).
func (b *Bitmap) Data() []byte { return b.data }

// Alpha returns the alpha plane slice if present (SplashBitmap.h).
func (b *Bitmap) Alpha() []byte { return b.alpha }

// TakeData returns the data slice and zeros it (SplashBitmap.h:111).
func (b *Bitmap) TakeData() []byte {
	d := b.data
	b.data = nil
	return d
}
