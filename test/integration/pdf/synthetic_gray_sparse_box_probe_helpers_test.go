package pdf_test

import "image"

func syntheticGraySparse16x16ImageForProbe() *image.Gray {
	src := image.NewGray(image.Rect(0, 0, 16, 16))
	copy(src.Pix, syntheticSparseGray16x16())
	return src
}

func buildSyntheticGraySparseBoxPDFForProbe(
	pageSize float64,
	matrix [6]float64,
	src *image.Gray,
) []byte {
	return buildSyntheticGrayImagePDFFloat(
		pageSize,
		pageSize,
		src.Bounds().Dx(),
		src.Bounds().Dy(),
		matrix,
		append([]byte(nil), src.Pix...),
	)
}
