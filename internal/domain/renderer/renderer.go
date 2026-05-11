// Package renderer provides page rendering interfaces.
package renderer

import (
	"context"
	"image"
	"image/color"

	"github.com/dh-kam/pdf-go/internal/domain/canvas"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

const (
	// ImageSamplingModeLegacy keeps baseline auto-sampling behavior plus selective CMYK defaults.
	ImageSamplingModeLegacy = "legacy"
	// ImageSamplingModeAdaptiveDCTICCBasedV1 enables adaptive image sampling experiments.
	ImageSamplingModeAdaptiveDCTICCBasedV1 = "adaptive-dct-iccbased-v1"
	// ImageSamplingModeExperimentalSplashScaleOnlyV1 enables Splash-like scale-only experiments.
	ImageSamplingModeExperimentalSplashScaleOnlyV1 = "experimental-splash-scale-only-v1"
	// ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1 enables Indexed origin-downscale bilinear phase experiments.
	ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1 = "experimental-indexed-origin-downscale-phase-v1"
	// ImageSamplingModeExperimentalIndexedCMYKSimpleV1 enables Indexed DeviceCMYK simple-subtractive conversion experiments.
	ImageSamplingModeExperimentalIndexedCMYKSimpleV1 = "experimental-indexed-cmyk-simple-v1"
	// ImageSamplingModeExperimentalIndexedCMYKHybrid75V1 enables Indexed DeviceCMYK hybrid-75 conversion experiments.
	ImageSamplingModeExperimentalIndexedCMYKHybrid75V1 = "experimental-indexed-cmyk-hybrid75-v1"
	// ImageSamplingModeExperimentalIndexedCMYKStdlibV1 enables Indexed DeviceCMYK stdlib conversion experiments.
	ImageSamplingModeExperimentalIndexedCMYKStdlibV1 = "experimental-indexed-cmyk-stdlib-v1"
	// ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1 enables tiny RGB transparent-edge upscale experiments.
	ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1 = "experimental-rgb-transparent-edge-upscale-v1"
	// ImageSamplingModeExperimentalDCTGrayIgnoreICCV1 enables tiny DCT ICC gray decode experiments by ignoring ICC tone curves.
	ImageSamplingModeExperimentalDCTGrayIgnoreICCV1 = "experimental-dct-gray-ignore-icc-v1"
	// ImageSamplingModeExperimentalIndexedCMYKLUTV1 enables palette-specific LUT-based CMYK conversion for exact pixel match.
	ImageSamplingModeExperimentalIndexedCMYKLUTV1 = "experimental-indexed-cmyk-lut-v1"
)

// RenderResult represents a rendered page result.
type RenderResult struct {
	Image   image.Image
	Error   error
	PageNum int
}

// RenderOptions represents options for page rendering.
type RenderOptions struct {
	BackgroundColor    color.Color
	DPI                float64
	Scale              float64
	EnableCache        bool
	DebugImageSampling bool
	ImageSamplingMode  string
}

// DefaultRenderOptions returns default render options.
func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		DPI:               72.0,
		Scale:             1.0,
		EnableCache:       true,
		BackgroundColor:   nil,
		ImageSamplingMode: ImageSamplingModeLegacy,
	}
}

// Renderer represents a page renderer.
type Renderer interface {
	// RenderPage renders a single page to an image.
	RenderPage(ctx context.Context, page *entity.Page, options RenderOptions) (image.Image, error)

	// RenderPages renders multiple pages concurrently.
	RenderPages(ctx context.Context, doc *entity.Document, pageNumbers []int, options RenderOptions) <-chan RenderResult

	// RenderAllPages renders all pages in the document concurrently.
	RenderAllPages(ctx context.Context, doc *entity.Document, options RenderOptions) <-chan RenderResult
}

// CanvasRenderer renders to a canvas.
type CanvasRenderer interface {
	// RenderToCanvas renders a page to a canvas.
	RenderToCanvas(ctx context.Context, page *entity.Page, canvas canvas.Canvas) error
}
