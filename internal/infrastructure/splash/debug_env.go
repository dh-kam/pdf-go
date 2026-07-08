package splash

import "os"

var (
	debugSplashDisableStrokeAdjust                 = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_STROKE_ADJUST") == "1"
	splashDisableMirrorStrokeNormals               = os.Getenv("PDF_SPLASH_DISABLE_MIRROR_STROKE_NORMALS") == "1"
	debugSplashOpaquePageAlpha                     = os.Getenv("PDF_DEBUG_SPLASH_OPAQUE_PAGE_ALPHA") == "1"
	disableSplashFillSubpathYMinTie                = os.Getenv("PDF_DISABLE_SPLASH_FILL_SUBPATH_YMIN_TIE") == "1"
	debugSplashFillSubpathYMinTie                  = os.Getenv("PDF_DEBUG_SPLASH_FILL_SUBPATH_YMIN_TIE")
	debugSplashStrokeTrace                         = os.Getenv("PDF_DEBUG_SPLASH_STROKE_TRACE")
	debugSplashDisableDeviceCapGate                = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_DEVICE_CAP_GATE") == "1"
	debugSplashDeviceCapGateTrace                  = os.Getenv("PDF_DEBUG_SPLASH_DEVICE_CAP_GATE_TRACE")
	debugSplashSkipStrokeIndex                     = os.Getenv("PDF_DEBUG_SPLASH_SKIP_STROKE_INDEX")
	debugSplashSkipFillIndex                       = os.Getenv("PDF_DEBUG_SPLASH_SKIP_FILL_INDEX")
	debugSplashDisableStrokeAdjustForStrokeIndex   = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_STROKE_ADJUST_FOR_STROKE_INDEX")
	debugSplashFillTrace                           = os.Getenv("PDF_DEBUG_SPLASH_FILL_TRACE")
	debugSplashClipBBoxTrace                       = os.Getenv("PDF_DEBUG_SPLASH_CLIP_BBOX_TRACE")
	splashDebugGt                                  = os.Getenv("SPLASH_DEBUG_GT") != ""
	debugSplashTextFillPath                        = os.Getenv("PDF_DEBUG_SPLASH_TEXT_FILL_PATH") == "1"
	debugSplashTextPathDeferMatrix                 = os.Getenv("PDF_DEBUG_SPLASH_TEXT_PATH_DEFER_MATRIX")
	debugSplashTextPathTrace                       = os.Getenv("PDF_DEBUG_SPLASH_TEXT_PATH_TRACE")
	debugSplashTextPathTraceContext                = os.Getenv("PDF_DEBUG_SPLASH_TEXT_PATH_TRACE_CONTEXT")
	debugSplashTextFillYMinSubpixelBias            = os.Getenv("PDF_DEBUG_SPLASH_TEXT_FILL_YMIN_SUBPIXEL_BIAS")
	debugSplashTextPathPopplerOrder                = os.Getenv("PDF_DEBUG_SPLASH_TEXT_PATH_POPPLER_ORDER") == "1"
	debugSplashTextStrokeWidthScale                = os.Getenv("PDF_DEBUG_SPLASH_TEXT_STROKE_WIDTH_SCALE")
	splashDisableFtGlyph                           = os.Getenv("SPLASH_DISABLE_FT_GLYPH") != ""
	debugSplashGlyphSkipTransformed                = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_SKIP_TRANSFORMED") != ""
	debugSplashGlyphRowDump                        = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_ROW_DUMP")
	debugSplashGlyphRowDumpRowsVal                 = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_ROW_DUMP_ROWS")
	debugSplashGlyphHalfEps                        = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_HALF_EPS")
	debugSplashGlyphPhaseEps                       = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_PHASE_EPS")
	debugSplashGlyphPhaseForce                     = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_PHASE_FORCE")
	debugSplashGlyphPhaseBias                      = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_PHASE_BIAS")
	debugSplashGlyphYPhase                         = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_Y_PHASE")
	debugSplashGlyphSnapEps                        = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_SNAP_EPS")
	debugSplashGlyphSnapGrid                       = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_SNAP_GRID")
	debugSplashGlyphSnapGridEps                    = os.Getenv("PDF_DEBUG_SPLASH_GLYPH_SNAP_GRID_EPS")
	debugSplashDisablePopplerImageContract         = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_POPPLER_IMAGE_CONTRACT") == "1"
	debugSplashPopplerImageSourceFlip              = os.Getenv("PDF_DEBUG_SPLASH_POPPLER_IMAGE_SOURCE_FLIP") == "1"
	debugSplashPopplerImageLegacyMatrix            = os.Getenv("PDF_DEBUG_SPLASH_POPPLER_IMAGE_LEGACY_MATRIX") == "1"
	debugSplashDisableIccPostScale                 = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_ICC_POST_SCALE") == "1"
	debugSplashDisableSoftmaskPopplerImageContract = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_SOFTMASK_POPPLER_IMAGE_CONTRACT") == "1"
	debugSplashSoftmaskPopplerScaleFlip            = os.Getenv("PDF_DEBUG_SPLASH_SOFTMASK_POPPLER_SCALE_FLIP") != ""
	debugSplashDisableTopdownVFlipScale            = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_TOPDOWN_VFLIP_SCALE") == "1"
	debugSplashDisableTopdownDownscale             = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_TOPDOWN_DOWNSCALE") == "1"
	splashImageEnableAlphaUnpremultiply            = os.Getenv("PDF_SPLASH_IMAGE_ENABLE_ALPHA_UNPREMULTIPLY") == "1"
	debugSplashImageDrawTrace                      = os.Getenv("PDF_DEBUG_SPLASH_IMAGE_DRAW_TRACE") != ""
	debugSplashImageClipT2ToT1                     = os.Getenv("PDF_DEBUG_SPLASH_IMAGE_CLIP_T2_TO_T1") == "1"
	debugSplashImageClipSpanGate                   = os.Getenv("PDF_DEBUG_SPLASH_IMAGE_CLIP_SPAN_GATE") == "1"
	debugSplashImageClipFullWidth                  = os.Getenv("PDF_DEBUG_SPLASH_IMAGE_CLIP_FULLWIDTH") == "1"
	debugSplashSoftmaskImageTrace                  = os.Getenv("PDF_DEBUG_SPLASH_SOFTMASK_IMAGE_TRACE") != ""
	debugSplashDisableTextGlyphIntegerSlopeBias    = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_TEXT_GLYPH_INTEGER_SLOPE_BIAS") == "1"
	debugSplashXpathTrace                          = os.Getenv("PDF_DEBUG_SPLASH_XPATH_TRACE")
	debugSplashXpathTraceStroke                    = os.Getenv("PDF_DEBUG_SPLASH_XPATH_TRACE_STROKE")
	debugSplashXpathTraceFill                      = os.Getenv("PDF_DEBUG_SPLASH_XPATH_TRACE_FILL")
	debugSplashDisableTinyGroupYMinTie             = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_TINY_GROUP_YMIN_TIE") == "1"
	debugSplashDisableFullWidthAabuf               = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_FULL_WIDTH_AABUF") == "1"
	debugSplashAxialRightEdgeSamplePrevVal         = os.Getenv("PDF_DEBUG_SPLASH_AXIAL_RIGHT_EDGE_SAMPLE_PREV") == "1"
	debugSplashStrokeFlatness                      = os.Getenv("PDF_DEBUG_SPLASH_STROKE_FLATNESS")
	debugSplashStrokeInputTrace                    = os.Getenv("PDF_DEBUG_SPLASH_STROKE_INPUT_TRACE")
	debugSplashStrokeInputContext                  = os.Getenv("PDF_DEBUG_SPLASH_STROKE_INPUT_CONTEXT")
	debugSplashStrokeOutlineTrace                  = os.Getenv("PDF_DEBUG_SPLASH_STROKE_OUTLINE_TRACE")
	debugSplashStrokeOutlineContext                = os.Getenv("PDF_DEBUG_SPLASH_STROKE_OUTLINE_CONTEXT")
	debugSplashDisableDashedButtStrokeMirror       = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_DASHED_BUTT_STROKE_MIRROR") == "1"
	debugSplashDisablePopplerStrokeWalker          = os.Getenv("PDF_DEBUG_SPLASH_DISABLE_POPPLER_STROKE_WALKER") == "1"
	debugSplashForceMirrorStrokeNormals            = os.Getenv("PDF_DEBUG_SPLASH_FORCE_MIRROR_STROKE_NORMALS") == "1"
)
