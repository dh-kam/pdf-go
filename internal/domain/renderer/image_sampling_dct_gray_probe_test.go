package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
)

func TestChooseImageSamplingPolicy_TinyEncodedICCBasedDCTGrayDiffersAcrossLegacyAndAdaptive(t *testing.T) {
	legacy := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		true,
		16,
		16,
		8,
		8,
	)
	adaptive := chooseImageSamplingPolicy(
		ImageSamplingModeAdaptiveDCTICCBasedV1,
		false,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		true,
		16,
		16,
		8,
		8,
	)

	assert.True(t, legacy.Interpolate)
	assert.Equal(t, "auto_box_tiny_iccbased_gray_downscale", legacy.Sampler)
	assert.Equal(t, "auto_interpolate=false_downscale_tiny_iccbased_gray", legacy.Reason)

	assert.True(t, adaptive.Interpolate)
	assert.Equal(t, "adaptive_downscale_bilinear_tiny_encoded_gray", adaptive.Sampler)
	assert.Equal(t, "adaptive_tiny_gray_downscale_encoded", adaptive.Reason)
}

func TestChooseImageSamplingPolicy_TinyEncodedDeviceGrayDCTStillDiffersAcrossLegacyAndAdaptive(t *testing.T) {
	legacy := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		false,
		16,
		16,
		8,
		8,
	)
	adaptive := chooseImageSamplingPolicy(
		ImageSamplingModeAdaptiveDCTICCBasedV1,
		false,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		false,
		16,
		16,
		8,
		8,
	)

	assert.True(t, legacy.Interpolate)
	assert.Equal(t, "auto_approx_bilinear", legacy.Sampler)
	assert.Equal(t, "auto_interpolate=false_downscale_small_grayscale", legacy.Reason)

	assert.True(t, adaptive.Interpolate)
	assert.Equal(t, "adaptive_downscale_bilinear_tiny_encoded_gray", adaptive.Sampler)
	assert.Equal(t, "adaptive_tiny_gray_downscale_encoded", adaptive.Reason)
}

func TestResolveExperimentalDCTGrayICCProfile_TinyCandidateKeepsAdaptiveButDropsLegacyAndIgnoreICC(t *testing.T) {
	legacyProfile, legacyComponents, legacyMode := resolveExperimentalDCTGrayICCProfile(
		ImageSamplingModeLegacy,
		"candidate_tiny_dct_iccbased_gray_downscale",
		[]byte{1, 2, 3},
		1,
	)
	ignoreProfile, ignoreComponents, ignoreMode := resolveExperimentalDCTGrayICCProfile(
		ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
		"candidate_tiny_dct_iccbased_gray_downscale",
		[]byte{1, 2, 3},
		1,
	)
	adaptiveProfile, adaptiveComponents, adaptiveMode := resolveExperimentalDCTGrayICCProfile(
		ImageSamplingModeAdaptiveDCTICCBasedV1,
		"candidate_tiny_dct_iccbased_gray_downscale",
		[]byte{1, 2, 3},
		1,
	)

	assert.Nil(t, legacyProfile)
	assert.Zero(t, legacyComponents)
	assert.Equal(t, "legacy_selective_ignore", legacyMode)

	assert.Nil(t, ignoreProfile)
	assert.Zero(t, ignoreComponents)
	assert.Equal(t, "ignore", ignoreMode)

	assert.Equal(t, []byte{1, 2, 3}, adaptiveProfile)
	assert.Equal(t, 1, adaptiveComponents)
	assert.Equal(t, "default", adaptiveMode)
}

func TestImageSamplingPhase_TinyEncodedDCTGrayBranchesSplitAdaptiveAndLegacy(t *testing.T) {
	legacy := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		true,
		16,
		16,
		8,
		8,
	)
	adaptive := chooseImageSamplingPolicy(
		ImageSamplingModeAdaptiveDCTICCBasedV1,
		false,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		true,
		16,
		16,
		8,
		8,
	)

	legacyPhaseX, legacyPhaseY := imageSamplingPhase(legacy.Sampler, legacy.Reason, legacy.Interpolate, [6]float64{})
	adaptivePhaseX, adaptivePhaseY := imageSamplingPhase(adaptive.Sampler, adaptive.Reason, adaptive.Interpolate, [6]float64{})

	assert.Equal(t, 0.0, legacyPhaseX)
	assert.Equal(t, 0.0, legacyPhaseY)
	assert.Equal(t, 0.5, adaptivePhaseX)
	assert.Equal(t, 0.5, adaptivePhaseY)
}

func TestImageSamplingPhase_TinyEncodedDeviceGrayDCTBranchesUseDifferentPhaseFamilies(t *testing.T) {
	legacy := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		false,
		16,
		16,
		8,
		8,
	)
	adaptive := chooseImageSamplingPolicy(
		ImageSamplingModeAdaptiveDCTICCBasedV1,
		false,
		false,
		domainimage.FilterDCT,
		"DeviceGray",
		false,
		16,
		16,
		8,
		8,
	)

	legacyPhaseX, legacyPhaseY := imageSamplingPhase(legacy.Sampler, legacy.Reason, legacy.Interpolate, [6]float64{})
	adaptivePhaseX, adaptivePhaseY := imageSamplingPhase(adaptive.Sampler, adaptive.Reason, adaptive.Interpolate, [6]float64{})

	assert.Equal(t, 0.0, legacyPhaseX)
	assert.Equal(t, 0.0, legacyPhaseY)
	assert.Equal(t, 0.5, adaptivePhaseX)
	assert.Equal(t, 0.5, adaptivePhaseY)
}

func TestChooseImageSamplingPolicy_PopplerHighMagnificationUpscaleDisablesInterpolation(t *testing.T) {
	decision := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterNone,
		"DeviceRGB",
		false,
		16,
		16,
		208.333333,
		208.333333,
	)

	assert.False(t, decision.Interpolate)
	assert.Equal(t, "auto_nearest", decision.Sampler)
	assert.Equal(t, "auto_interpolate=false_upscale_400pct_poppler", decision.Reason)
}

func TestChooseImageSamplingPolicy_PopplerSub400PctUpscaleStillInterpolates(t *testing.T) {
	decision := chooseImageSamplingPolicy(
		ImageSamplingModeLegacy,
		false,
		false,
		domainimage.FilterNone,
		"DeviceRGB",
		false,
		128,
		128,
		200,
		200,
	)

	assert.True(t, decision.Interpolate)
	assert.Equal(t, "auto_upscale_bilinear", decision.Sampler)
	assert.Equal(t, "auto_interpolate=false_upscale", decision.Reason)
}
