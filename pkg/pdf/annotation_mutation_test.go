package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestCloneAnnotationSnapshots_DeepCopy(t *testing.T) {
	input := []annotationSnapshot{
		{
			Type:       "Text",
			Rect:       [4]float64{1, 2, 3, 4},
			Contents:   "note",
			PgPoints:   []float64{1, 2, 3, 4},
			HeadPoints: []float64{4, 3, 2, 1},
			PathList:   [][]float64{{1, 2, 3, 4}},
			UserData: map[string]string{
				"k": "v",
			},
		},
	}

	cloned := cloneAnnotationSnapshots(input)
	require.Len(t, cloned, 1)

	cloned[0].PgPoints[0] = 99
	cloned[0].HeadPoints[0] = 55
	cloned[0].PathList[0][0] = 77
	cloned[0].UserData["k"] = "changed"

	assert.Equal(t, []float64{1, 2, 3, 4}, input[0].PgPoints)
	assert.Equal(t, []float64{4, 3, 2, 1}, input[0].HeadPoints)
	assert.Equal(t, [][]float64{{1, 2, 3, 4}}, input[0].PathList)
	assert.Equal(t, "v", input[0].UserData["k"])
}

func TestAnnotationPgPointsFromDict(t *testing.T) {
	t.Run("uses_pgpts_when_present", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("PgPts"), entity.NewArray(
			entity.NewReal(1),
			entity.NewReal(2),
			entity.NewReal(3),
			entity.NewReal(4),
		))
		dict.Set(entity.Name("QuadPoints"), entity.NewArray(
			entity.NewReal(9),
			entity.NewReal(9),
			entity.NewReal(9),
			entity.NewReal(9),
		))

		got := annotationPgPointsFromDict(dict)
		assert.Equal(t, []float64{1, 2, 3, 4}, got)
	})

	t.Run("flattens_ink_list", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("InkList"), entity.NewArray(
			entity.NewArray(
				entity.NewReal(1),
				entity.NewReal(2),
				entity.NewReal(3),
				entity.NewReal(4),
			),
			entity.NewArray(
				entity.NewReal(5),
				entity.NewReal(6),
			),
		))

		got := annotationPgPointsFromDict(dict)
		assert.Equal(t, []float64{1, 2, 3, 4, 5, 6}, got)
	})
}

func TestAnnotationHeadPointsFromDict(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("HeadPts"), entity.NewArray(
		entity.NewReal(7),
		entity.NewReal(8),
		entity.NewReal(9),
		entity.NewReal(10),
	))
	got := annotationHeadPointsFromDict(dict)
	assert.Equal(t, []float64{7, 8, 9, 10}, got)
}

func TestAnnotationUserDataFromDict(t *testing.T) {
	dict := entity.NewDict()
	ud := entity.NewDict()
	ud.Set(entity.Name("str"), entity.NewString("hello"))
	ud.Set(entity.Name("num"), entity.NewInteger(12))
	ud.Set(entity.Name("flag"), entity.NewBoolean(true))
	dict.Set(entity.Name("UD"), ud)

	got := annotationUserDataFromDict(dict)
	require.NotNil(t, got)
	assert.Equal(t, "hello", got["str"])
	assert.Equal(t, "12", got["num"])
	assert.Equal(t, "true", got["flag"])
}

func TestAnnotationPathListFromDict(t *testing.T) {
	t.Run("ink_list", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("InkList"), entity.NewArray(
			entity.NewArray(entity.NewReal(1), entity.NewReal(2), entity.NewReal(3), entity.NewReal(4)),
			entity.NewArray(entity.NewReal(5), entity.NewReal(6)),
		))

		got := annotationPathListFromDict(dict)
		assert.Equal(t, [][]float64{{1, 2, 3, 4}, {5, 6}}, got)
	})

	t.Run("vertices_fallback", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("Vertices"), entity.NewArray(
			entity.NewReal(10), entity.NewReal(20), entity.NewReal(30), entity.NewReal(40),
		))

		got := annotationPathListFromDict(dict)
		assert.Equal(t, [][]float64{{10, 20, 30, 40}}, got)
	})
}

func TestAnnotation_PgPointsHeadPointsAndUserData(t *testing.T) {
	t.Run("from_snapshot_returns_copies", func(t *testing.T) {
		annot := &Annotation{
			snapshot: &annotationSnapshot{
				PgPoints:   []float64{1, 2, 3, 4},
				HeadPoints: []float64{4, 3, 2, 1},
				PathList:   [][]float64{{1, 2, 3, 4}},
				UserData: map[string]string{
					"a": "b",
				},
			},
		}

		points := annot.PgPoints()
		headPoints := annot.HeadPoints()
		pathList := annot.PathList()
		data := annot.UserDataList()
		require.Equal(t, []float64{1, 2, 3, 4}, points)
		require.Equal(t, []float64{4, 3, 2, 1}, headPoints)
		require.Equal(t, [][]float64{{1, 2, 3, 4}}, pathList)
		require.Equal(t, "b", data["a"])

		points[0] = 99
		headPoints[0] = 77
		pathList[0][0] = 77
		data["a"] = "changed"

		assert.Equal(t, []float64{1, 2, 3, 4}, annot.PgPoints())
		assert.Equal(t, []float64{4, 3, 2, 1}, annot.HeadPoints())
		assert.Equal(t, [][]float64{{1, 2, 3, 4}}, annot.PathList())
		value, ok := annot.UserData("a")
		require.True(t, ok)
		assert.Equal(t, "b", value)
	})

	t.Run("from_entity_dict", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("Subtype"), entity.NewName("Text"))
		dict.Set(entity.Name("PgPts"), entity.NewArray(
			entity.NewReal(1),
			entity.NewReal(2),
			entity.NewReal(3),
			entity.NewReal(4),
		))
		dict.Set(entity.Name("HeadPts"), entity.NewArray(
			entity.NewReal(4),
			entity.NewReal(3),
			entity.NewReal(2),
			entity.NewReal(1),
		))
		dict.Set(entity.Name("Vertices"), entity.NewArray(
			entity.NewReal(11),
			entity.NewReal(12),
			entity.NewReal(13),
			entity.NewReal(14),
		))
		ud := entity.NewDict()
		ud.Set(entity.Name("meta"), entity.NewString("ok"))
		dict.Set(entity.Name("UD"), ud)

		annot := &Annotation{annotation: entity.NewAnnotation(dict)}

		assert.Equal(t, []float64{1, 2, 3, 4}, annot.PgPoints())
		assert.Equal(t, []float64{4, 3, 2, 1}, annot.HeadPoints())
		assert.Equal(t, [][]float64{{11, 12, 13, 14}}, annot.PathList())
		value, ok := annot.UserData("meta")
		require.True(t, ok)
		assert.Equal(t, "ok", value)
	})
}
