package pdf_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	infraimage "github.com/dh-kam/pdf-go/internal/infrastructure/image"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
	pdfxref "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
	"github.com/dh-kam/pdf-go/pkg/pdf"
)

type samplePageImagePlacementInvokeProbeResult struct {
	imageName string
	imageRef  entity.Ref
	matrix    [6]float64
}

type samplePageOperatorHistogramProbeResult struct {
	operatorCounts map[string]int
	streamCount    int
}

type samplePageLayoutProbeResult struct {
	mediaBox [4]float64
	cropBox  [4]float64
}

type samplePageImageObjectProbeResult struct {
	imageName         string
	imageRef          entity.Ref
	dictKeys          []string
	filter            domainimage.ImageFilter
	colorSpace        string
	decode            []float64
	decodeParms       map[string]interface{}
	width             int
	height            int
	bitsPerComponent  int
	iccProfileLength  int
	iccComponents     int
	rawStreamLength   int
	decodedDataLength int
}

type sampleICCProfileProbeResult struct {
	dictKeys      []string
	filter        domainimage.ImageFilter
	alternate     string
	components    int
	profileLength int
}

type sampleICCProfileStreamFingerprintProbeResult struct {
	filter        domainimage.ImageFilter
	rawLength     int
	rawSHA256     string
	decodedLength int
	decodedSHA256 string
}

type samplePageImageDecodeProbeResult struct {
	width            int
	height           int
	colorSpace       string
	bitsPerComponent int
	rgbLength        int
	rgbSHA256        string
}

type samplePageImageDataFingerprintProbeResult struct {
	filter           domainimage.ImageFilter
	colorSpace       string
	bitsPerComponent int
	width            int
	height           int
	dataSHA256       string
	iccProfileSHA256 string
	iccComponents    int
	decode           []float64
	decodeParms      map[string]interface{}
}

func measureSamplePageImagePlacementOpsForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
) []samplePageImagePlacementInvokeProbeResult {
	t.Helper()

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() { _ = doc.Close() }()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	xObjectRefs := samplePageImageXObjectRefsForProbe(t, page)
	contents, err := page.Contents()
	require.NoError(t, err)

	var results []samplePageImagePlacementInvokeProbeResult
	for _, obj := range contents {
		streamObj, ok := obj.(*entity.Stream)
		if !ok || streamObj == nil {
			continue
		}
		decoded, err := streamObj.Decode()
		require.NoError(t, err)
		results = append(results, extractImagePlacementInvokesFromContentForProbe(string(decoded), xObjectRefs)...)
	}

	return results
}

func measureSamplePageOperatorHistogramForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
) samplePageOperatorHistogramProbeResult {
	t.Helper()

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() { _ = doc.Close() }()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	contents, err := page.Contents()
	require.NoError(t, err)

	result := samplePageOperatorHistogramProbeResult{
		operatorCounts: make(map[string]int),
		streamCount:    len(contents),
	}
	for _, obj := range contents {
		streamObj, ok := obj.(*entity.Stream)
		if !ok || streamObj == nil {
			continue
		}
		decoded, err := streamObj.Decode()
		require.NoError(t, err)
		accumulateOperatorHistogramForProbe(result.operatorCounts, string(decoded))
	}

	return result
}

func measureSamplePageLayoutForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
) samplePageLayoutProbeResult {
	t.Helper()

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() { _ = doc.Close() }()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	return samplePageLayoutProbeResult{
		mediaBox: page.MediaBox(),
		cropBox:  page.CropBox(),
	}
}

func measurePDFPageImageObjectForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
	imageName string,
) samplePageImageObjectProbeResult {
	t.Helper()

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() { _ = doc.Close() }()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	xObjects, ok := resources.Get("XObject").(*pdf.Dict)
	require.True(t, ok)

	imageObj := xObjects.Get(imageName)
	require.NotNil(t, imageObj)

	var imageRef entity.Ref

	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	xrefTable := pdfxref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	var obj entity.Object
	if ref, ok := imageObj.(entity.Ref); ok {
		imageRef = ref
		fetched, err := xrefTable.Fetch(ref)
		require.NoError(t, err)
		obj = fetched
	} else if streamObj, ok := imageObj.(*entity.Stream); ok {
		obj = streamObj
	} else {
		require.FailNowf(t, "resolve_xobject", "unsupported XObject type: %T", imageObj)
	}

	streamObj := requireStreamObject(t, xrefTable, obj)
	require.NotNil(t, streamObj)

	decodedLength := -1
	if decoded, err := pdfstream.NewFromEntity(streamObj).Decode(); err == nil {
		decodedLength = len(decoded)
	}

	colorSpace, iccProfile, iccComponents := requireSampleImageColorSpaceMetadata(
		t,
		xrefTable,
		streamObj.Dict().Get(entity.Name("ColorSpace")),
	)

	dictKeys := make([]string, 0, streamObj.Dict().Len())
	for _, key := range streamObj.Dict().Keys() {
		dictKeys = append(dictKeys, key.Value())
	}
	sort.Strings(dictKeys)

	return samplePageImageObjectProbeResult{
		imageName:         imageName,
		imageRef:          imageRef,
		dictKeys:          dictKeys,
		filter:            requireImageFilter(t, xrefTable, streamObj.Dict().Get(entity.Name("Filter"))),
		colorSpace:        colorSpace,
		decode:            requireObjectFloatSlice(t, xrefTable, streamObj.Dict().Get(entity.Name("Decode"))),
		decodeParms:       requireDecodeParms(t, xrefTable, streamObj.Dict().Get(entity.Name("DecodeParms"))),
		width:             requireObjectInt(t, streamObj.Dict().Get(entity.Name("Width"))),
		height:            requireObjectInt(t, streamObj.Dict().Get(entity.Name("Height"))),
		bitsPerComponent:  requireObjectInt(t, streamObj.Dict().Get(entity.Name("BitsPerComponent"))),
		iccProfileLength:  len(iccProfile),
		iccComponents:     iccComponents,
		rawStreamLength:   len(streamObj.RawBytes()),
		decodedDataLength: decodedLength,
	}
}

func measurePDFPageImageICCProfileForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
	imageName string,
) sampleICCProfileProbeResult {
	t.Helper()

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() { _ = doc.Close() }()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	xObjects, ok := resources.Get("XObject").(*pdf.Dict)
	require.True(t, ok)

	imageObj := xObjects.Get(imageName)
	require.NotNil(t, imageObj)

	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	xrefTable := pdfxref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	var obj entity.Object
	if ref, ok := imageObj.(entity.Ref); ok {
		fetched, err := xrefTable.Fetch(ref)
		require.NoError(t, err)
		obj = fetched
	} else if streamObj, ok := imageObj.(*entity.Stream); ok {
		obj = streamObj
	} else {
		require.FailNowf(t, "resolve_xobject", "unsupported XObject type: %T", imageObj)
	}

	streamObj := requireStreamObject(t, xrefTable, obj)
	require.NotNil(t, streamObj)

	colorSpaceObj := streamObj.Dict().Get(entity.Name("ColorSpace"))
	colorSpaceArray, ok := colorSpaceObj.(*entity.Array)
	require.True(t, ok)
	require.GreaterOrEqual(t, colorSpaceArray.Len(), 2)

	iccStream := requireStreamObject(t, xrefTable, colorSpaceArray.Get(1))
	require.NotNil(t, iccStream)
	profileBytes, err := pdfstream.NewFromEntity(iccStream).Decode()
	require.NoError(t, err)

	dictKeys := make([]string, 0, iccStream.Dict().Len())
	for _, key := range iccStream.Dict().Keys() {
		dictKeys = append(dictKeys, key.Value())
	}
	sort.Strings(dictKeys)

	alternate := ""
	if altObj := iccStream.Dict().Get(entity.Name("Alternate")); altObj != nil {
		alternate = requireColorSpaceName(t, xrefTable, altObj)
	}

	return sampleICCProfileProbeResult{
		dictKeys:      dictKeys,
		filter:        requireImageFilter(t, xrefTable, iccStream.Dict().Get(entity.Name("Filter"))),
		alternate:     alternate,
		components:    requireSampleICCBasedN(t, iccStream.Dict()),
		profileLength: len(profileBytes),
	}
}

func measurePDFPageImageICCProfileStreamFingerprintForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
	imageName string,
) sampleICCProfileStreamFingerprintProbeResult {
	t.Helper()

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() { _ = doc.Close() }()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	xObjects, ok := resources.Get("XObject").(*pdf.Dict)
	require.True(t, ok)

	imageObj := xObjects.Get(imageName)
	require.NotNil(t, imageObj)

	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	xrefTable := pdfxref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	var obj entity.Object
	if ref, ok := imageObj.(entity.Ref); ok {
		fetched, err := xrefTable.Fetch(ref)
		require.NoError(t, err)
		obj = fetched
	} else if streamObj, ok := imageObj.(*entity.Stream); ok {
		obj = streamObj
	} else {
		require.FailNowf(t, "resolve_xobject", "unsupported XObject type: %T", imageObj)
	}

	streamObj := requireStreamObject(t, xrefTable, obj)
	require.NotNil(t, streamObj)

	colorSpaceObj := streamObj.Dict().Get(entity.Name("ColorSpace"))
	colorSpaceArray, ok := colorSpaceObj.(*entity.Array)
	require.True(t, ok)
	require.GreaterOrEqual(t, colorSpaceArray.Len(), 2)

	iccStream := requireStreamObject(t, xrefTable, colorSpaceArray.Get(1))
	require.NotNil(t, iccStream)

	decoded, err := pdfstream.NewFromEntity(iccStream).Decode()
	require.NoError(t, err)

	return sampleICCProfileStreamFingerprintProbeResult{
		filter:        requireImageFilter(t, xrefTable, iccStream.Dict().Get(entity.Name("Filter"))),
		rawLength:     len(iccStream.RawBytes()),
		rawSHA256:     sha256HexForProbe(iccStream.RawBytes()),
		decodedLength: len(decoded),
		decodedSHA256: sha256HexForProbe(decoded),
	}
}

func loadPDFPageImageDataForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
	imageName string,
) *domainimage.ImageData {
	t.Helper()

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() { _ = doc.Close() }()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	xObjects, ok := resources.Get("XObject").(*pdf.Dict)
	require.True(t, ok)

	imageObj := xObjects.Get(imageName)
	require.NotNil(t, imageObj)

	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	xrefTable := pdfxref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	var obj entity.Object
	if ref, ok := imageObj.(entity.Ref); ok {
		fetched, err := xrefTable.Fetch(ref)
		require.NoError(t, err)
		obj = fetched
	} else if streamObj, ok := imageObj.(*entity.Stream); ok {
		obj = streamObj
	} else {
		require.FailNowf(t, "resolve_xobject", "unsupported XObject type: %T", imageObj)
	}

	streamObj := requireStreamObject(t, xrefTable, obj)
	require.NotNil(t, streamObj)

	colorSpace, iccProfile, iccComponents := requireSampleImageColorSpaceMetadata(
		t,
		xrefTable,
		streamObj.Dict().Get(entity.Name("ColorSpace")),
	)

	return &domainimage.ImageData{
		Data:             append([]byte(nil), streamObj.RawBytes()...),
		Filter:           requireImageFilter(t, xrefTable, streamObj.Dict().Get(entity.Name("Filter"))),
		ColorSpace:       domainimage.ColorSpace(colorSpace),
		ICCProfile:       iccProfile,
		ICCComponents:    iccComponents,
		Width:            requireObjectInt(t, streamObj.Dict().Get(entity.Name("Width"))),
		Height:           requireObjectInt(t, streamObj.Dict().Get(entity.Name("Height"))),
		BitsPerComponent: requireObjectInt(t, streamObj.Dict().Get(entity.Name("BitsPerComponent"))),
		DecodeParms:      requireDecodeParms(t, xrefTable, streamObj.Dict().Get(entity.Name("DecodeParms"))),
		Decode:           requireObjectFloatSlice(t, xrefTable, streamObj.Dict().Get(entity.Name("Decode"))),
	}
}

func decodePDFPageImageObjectToRGBForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
	imageName string,
) samplePageImageDecodeProbeResult {
	t.Helper()

	imgData := loadPDFPageImageDataForProbe(t, pdfPath, pageNum, imageName)
	decoder := infraimage.NewDecoder()
	decoded, err := decoder.Decode(imgData)
	require.NoError(t, err)

	return samplePageImageDecodeProbeResult{
		width:            decoded.Width(),
		height:           decoded.Height(),
		colorSpace:       string(decoded.ColorSpace()),
		bitsPerComponent: decoded.BitsPerComponent(),
		rgbLength:        len(imageToRGBBytes(decoded.Image())),
		rgbSHA256:        sha256HexForProbe(imageToRGBBytes(decoded.Image())),
	}
}

func fingerprintPDFPageImageDataForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
	imageName string,
) samplePageImageDataFingerprintProbeResult {
	t.Helper()

	imgData := loadPDFPageImageDataForProbe(t, pdfPath, pageNum, imageName)

	return samplePageImageDataFingerprintProbeResult{
		filter:           imgData.Filter,
		colorSpace:       string(imgData.ColorSpace),
		bitsPerComponent: imgData.BitsPerComponent,
		width:            imgData.Width,
		height:           imgData.Height,
		dataSHA256:       sha256HexForProbe(imgData.Data),
		iccProfileSHA256: sha256HexForProbe(imgData.ICCProfile),
		iccComponents:    imgData.ICCComponents,
		decode:           append([]float64(nil), imgData.Decode...),
		decodeParms:      imgData.DecodeParms,
	}
}

func measureSamplePageImageObjectForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
	imageName string,
) samplePageImageObjectProbeResult {
	t.Helper()

	return measurePDFPageImageObjectForProbe(t, pdfPath, pageNum, imageName)
}

func extractImagePlacementInvokesFromContentForProbe(
	content string,
	xObjectRefs map[string]entity.Ref,
) []samplePageImagePlacementInvokeProbeResult {
	tokens := strings.Fields(content)
	var (
		numberStack []float64
		currentCM   [6]float64
		currentName string
		results     []samplePageImagePlacementInvokeProbeResult
	)

	for _, token := range tokens {
		if v, err := strconv.ParseFloat(token, 64); err == nil {
			numberStack = append(numberStack, v)
			continue
		}

		switch {
		case token == "cm":
			if len(numberStack) >= 6 {
				copy(currentCM[:], numberStack[len(numberStack)-6:])
			}
			numberStack = numberStack[:0]
		case strings.HasPrefix(token, "/"):
			currentName = strings.TrimPrefix(token, "/")
			numberStack = numberStack[:0]
		case token == "Do":
			if currentName != "" {
				results = append(results, samplePageImagePlacementInvokeProbeResult{
					imageName: currentName,
					imageRef:  xObjectRefs[currentName],
					matrix:    currentCM,
				})
			}
			currentName = ""
			numberStack = numberStack[:0]
		default:
			numberStack = numberStack[:0]
		}
	}

	return results
}

func sha256HexForProbe(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func accumulateOperatorHistogramForProbe(dst map[string]int, content string) {
	for _, token := range strings.Fields(content) {
		switch token {
		case "q", "Q", "cm", "Do", "m", "l", "c", "v", "y", "h", "re", "S", "s", "f", "f*", "B", "B*", "b", "b*", "n", "BT", "ET", "Tj", "TJ", "'", "\"":
			dst[token]++
		}
	}
}

func samplePageImageXObjectRefsForProbe(
	t *testing.T,
	page *pdf.Page,
) map[string]entity.Ref {
	t.Helper()

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	xObjects, ok := resources.Get("XObject").(*pdf.Dict)
	require.True(t, ok)

	results := make(map[string]entity.Ref)
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("Im%d", i)
		if ref, ok := xObjects.Get(name).(entity.Ref); ok {
			results[name] = ref
		}
	}
	return results
}

func sampleDirDoc007MainPDFForProbe() string {
	return filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")
}
