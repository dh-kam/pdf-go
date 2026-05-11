package renderer

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

func TestType4MeshShadingPatternProbe(t *testing.T) {
	doc, err := internalpdf.Open("../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, doc.Close())
	}()

	page, err := doc.GetPage(94)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	xObjects := mustResolveMeshProbeDict(t, doc.XRef(), resources.Get(entity.Name("XObject")))
	im17 := mustResolveMeshProbeStream(t, doc.XRef(), xObjects.Get(entity.Name("Im17")))

	im17Resources := mustResolveMeshProbeDict(t, doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Children := mustResolveMeshProbeDict(t, doc.XRef(), im17Resources.Get(entity.Name("XObject")))
	fm184 := mustResolveMeshProbeStream(t, doc.XRef(), im17Children.Get(entity.Name("Fm184")))

	fmResources := mustResolveMeshProbeDict(t, doc.XRef(), fm184.Dict().Get(entity.Name("Resources")))
	patterns := mustResolveMeshProbeDict(t, doc.XRef(), fmResources.Get(entity.Name("Pattern")))
	patternObj := mustResolveMeshProbeObject(t, doc.XRef(), patterns.Get(entity.Name("pgfpatPlotsurface183")))
	patternDict, ok := patternObj.(*entity.Dict)
	require.True(t, ok)

	e := NewEvaluator(doc.XRef())
	shading, err := e.parsePatternShading(patternDict.Get(entity.Name("Shading")))
	require.NoError(t, err)
	require.NotNil(t, shading)
	shadingStream := mustResolveMeshProbeStream(t, doc.XRef(), patternDict.Get(entity.Name("Shading")))

	matrix := mustMeshProbeMatrix(patternDict.Get(entity.Name("Matrix")))
	vertices := shading.GetVertices()
	require.NotEmpty(t, vertices)

	rawPreview := meshProbeVertexPreview(vertices, [6]float64{1, 0, 0, 1, 0, 0}, 6)
	transformedPreview := meshProbeVertexPreview(vertices, matrix, 6)
	rawBounds := meshProbeVertexBounds(vertices, [6]float64{1, 0, 0, 1, 0, 0})
	transformedBounds := meshProbeVertexBounds(vertices, matrix)
	colorPreview := meshProbeFunctionPreview(t, shading.GetFunctions(), vertices, 6)
	recordPreview := meshProbeRecordPreview(t, shadingStream, shading)

	functionTypes := make([]string, 0, len(shading.GetFunctions()))
	for _, fn := range shading.GetFunctions() {
		functionTypes = append(functionTypes, fmt.Sprintf("%T", fn))
	}
	functionDetails := meshProbeFunctionDetails(shading.GetFunctions())

	t.Logf(
		"fm184_bbox=%v pattern_matrix=%v shading_type=%d color_space=%s functions=%s function_details=%s decode=%v vertices=%d raw_bounds=%v transformed_bounds=%v raw_preview=%s transformed_preview=%s color_preview=%s record_preview=%s",
		mustMeshProbeBBox(fm184.Dict().Get(entity.Name("BBox"))),
		matrix,
		shading.GetShadingType(),
		shading.GetColorSpace(),
		strings.Join(functionTypes, ","),
		functionDetails,
		shading.GetDecode(),
		len(vertices),
		rawBounds,
		transformedBounds,
		rawPreview,
		transformedPreview,
		colorPreview,
		recordPreview,
	)
}

func mustResolveMeshProbeObject(t *testing.T, xref entity.XRef, obj entity.Object) entity.Object {
	t.Helper()

	ref, ok := obj.(entity.Ref)
	if !ok {
		return obj
	}
	resolved, err := xref.Fetch(ref)
	require.NoError(t, err)
	return resolved
}

func mustResolveMeshProbeDict(t *testing.T, xref entity.XRef, obj entity.Object) *entity.Dict {
	t.Helper()

	resolved := mustResolveMeshProbeObject(t, xref, obj)
	switch typed := resolved.(type) {
	case *entity.Dict:
		return typed
	case *entity.Stream:
		return typed.Dict()
	default:
		t.Fatalf("expected dict, got %T", resolved)
		return nil
	}
}

func mustResolveMeshProbeStream(t *testing.T, xref entity.XRef, obj entity.Object) *entity.Stream {
	t.Helper()

	resolved := mustResolveMeshProbeObject(t, xref, obj)
	streamObj, ok := resolved.(*entity.Stream)
	require.True(t, ok)
	return streamObj
}

func mustMeshProbeMatrix(obj entity.Object) [6]float64 {
	arr, ok := obj.(*entity.Array)
	if !ok || arr.Len() < 6 {
		return [6]float64{1, 0, 0, 1, 0, 0}
	}

	var out [6]float64
	for i := 0; i < 6; i++ {
		switch typed := arr.Get(i).(type) {
		case *entity.Integer:
			out[i] = float64(typed.Value())
		case *entity.Real:
			out[i] = typed.Value()
		}
	}
	return out
}

func mustMeshProbeBBox(obj entity.Object) [4]float64 {
	arr, ok := obj.(*entity.Array)
	if !ok || arr.Len() < 4 {
		return [4]float64{}
	}

	var out [4]float64
	for i := 0; i < 4; i++ {
		switch typed := arr.Get(i).(type) {
		case *entity.Integer:
			out[i] = float64(typed.Value())
		case *entity.Real:
			out[i] = typed.Value()
		}
	}
	return out
}

func meshProbeVertexPreview(vertices []entity.Vertex, matrix [6]float64, limit int) string {
	capacity := limit
	if len(vertices) < capacity {
		capacity = len(vertices)
	}
	parts := make([]string, 0, capacity)
	for i := 0; i < len(vertices) && i < limit; i++ {
		tx, ty := meshProbeTransformPoint(matrix, vertices[i].X, vertices[i].Y)
		parts = append(parts, fmt.Sprintf("%d:(%.4f,%.4f)%v", i, tx, ty, vertices[i].Colors))
	}
	return strings.Join(parts, " | ")
}

func meshProbeVertexBounds(vertices []entity.Vertex, matrix [6]float64) [4]float64 {
	if len(vertices) == 0 {
		return [4]float64{}
	}

	x0, y0 := meshProbeTransformPoint(matrix, vertices[0].X, vertices[0].Y)
	minX, minY, maxX, maxY := x0, y0, x0, y0
	for _, vertex := range vertices[1:] {
		tx, ty := meshProbeTransformPoint(matrix, vertex.X, vertex.Y)
		if tx < minX {
			minX = tx
		}
		if tx > maxX {
			maxX = tx
		}
		if ty < minY {
			minY = ty
		}
		if ty > maxY {
			maxY = ty
		}
	}
	return [4]float64{minX, minY, maxX, maxY}
}

func meshProbeTransformPoint(matrix [6]float64, x, y float64) (float64, float64) {
	return matrix[0]*x + matrix[2]*y + matrix[4], matrix[1]*x + matrix[3]*y + matrix[5]
}

func meshProbeFunctionPreview(t *testing.T, functions []entity.Function, vertices []entity.Vertex, limit int) string {
	t.Helper()

	if len(functions) == 0 || len(vertices) == 0 {
		return "-"
	}

	capacity := limit
	if len(vertices) < capacity {
		capacity = len(vertices)
	}
	parts := make([]string, 0, capacity)
	for i := 0; i < len(vertices) && i < limit; i++ {
		outputs, err := functions[0].Evaluate(vertices[i].Colors)
		require.NoError(t, err)
		parts = append(parts, fmt.Sprintf("%d:%v->%v", i, vertices[i].Colors, outputs))
	}
	return strings.Join(parts, " | ")
}

func meshProbeFunctionDetails(functions []entity.Function) string {
	if len(functions) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(functions))
	for idx, fn := range functions {
		switch typed := fn.(type) {
		case *entity.StitchingFunction:
			subTypes := make([]string, 0, len(typed.Functions))
			for _, subFn := range typed.Functions {
				subTypes = append(subTypes, fmt.Sprintf("%T", subFn))
			}
			parts = append(parts, fmt.Sprintf(
				"%d:domain=%v bounds=%v encode=%v range=%v sub=%s",
				idx,
				typed.Domain,
				typed.Bounds,
				typed.Encode,
				typed.RangeVal,
				strings.Join(subTypes, ","),
			))
		default:
			parts = append(parts, fmt.Sprintf("%d:%T", idx, fn))
		}
	}
	return strings.Join(parts, " | ")
}

func meshProbeRecordPreview(t *testing.T, shadingStream *entity.Stream, shading *entity.Shading) string {
	t.Helper()

	require.NotNil(t, shadingStream)
	require.NotNil(t, shading)

	colorComponents, err := meshShadingColorComponentCount(shading)
	require.NoError(t, err)

	data, err := stream.NewFromEntity(shadingStream).Decode()
	require.NoError(t, err)

	reader := &shadingBitReader{data: data}
	parts := make([]string, 0, 8)
	for idx := 0; idx < 8; idx++ {
		flag, vertex, readErr := readMeshVertexRecord(reader, shading, colorComponents)
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			require.NoError(t, readErr)
		}
		parts = append(parts, fmt.Sprintf("%d:f=%d (%.4f,%.4f)%v", idx, flag, vertex.X, vertex.Y, vertex.Colors))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " | ")
}
