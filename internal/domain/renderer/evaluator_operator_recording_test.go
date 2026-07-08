package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvaluatorDoesNotPreallocateOperators(t *testing.T) {
	e := NewEvaluator(nil)

	assert.Len(t, e.GetOperators(), 0)
	assert.Equal(t, 0, cap(e.operators))
}

func TestParseOperatorsAllocatesOnlyWhenRecording(t *testing.T) {
	t.Run("recording disabled", func(t *testing.T) {
		e := NewEvaluator(nil)
		e.SetOperatorRecording(false)

		require.NoError(t, e.parseOperators([]byte("q Q")))
		assert.Len(t, e.GetOperators(), 0)
		assert.Equal(t, 0, cap(e.operators))
	})

	t.Run("recording enabled", func(t *testing.T) {
		e := NewEvaluator(nil)

		require.NoError(t, e.parseOperators([]byte("q Q")))
		assert.Len(t, e.GetOperators(), 2)
		assert.Greater(t, cap(e.operators), 0)
	})
}
