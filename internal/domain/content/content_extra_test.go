package content

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
)

type mockContentOperator struct {
	err error
}

func (m *mockContentOperator) Execute(state *graphics.State, operands []float64) error {
	return m.err
}

func TestEvaluator_AccessorsAndObjectStore(t *testing.T) {
	evaluator := NewEvaluator(nil)

	registry := NewOperatorRegistry()
	evaluator.SetRegistry(registry)
	assert.Same(t, registry, evaluator.GetRegistry())

	state := graphics.NewState()
	evaluator.SetState(state)
	assert.Same(t, state, evaluator.GetState())

	ref := entity.NewRef(7, 0)
	stream := entity.NewStream(entity.NewDict(), []byte("q"))
	evaluator.StoreObject(ref, stream)

	got, ok := evaluator.GetObject(ref)
	require.True(t, ok)
	assert.Equal(t, stream, got)

	_, ok = evaluator.GetObject(entity.NewRef(99, 0))
	assert.False(t, ok)
}

func TestEvaluator_SetXRefAndProcessArray(t *testing.T) {
	ref := entity.NewRef(11, 0)
	stream := entity.NewStream(entity.NewDict(), []byte("noop"))

	evaluator := NewEvaluator(nil)
	evaluator.SetXRef(&mockEvaluatorXRef{
		objects: map[entity.Ref]entity.Object{
			ref: stream,
		},
	})

	err := evaluator.ProcessObject(ref)
	require.NoError(t, err)

	arr := entity.NewArray(nil, stream)
	err = evaluator.ProcessArray(arr)
	require.NoError(t, err)
}

func TestEvaluator_ProcessArray_PropagatesOperatorError(t *testing.T) {
	evaluator := NewEvaluator(nil)
	registry := NewOperatorRegistry()
	registry.Register("ERR", &mockContentOperator{err: assert.AnError})
	evaluator.SetRegistry(registry)

	arr := entity.NewArray(entity.NewStream(entity.NewDict(), []byte("ERR")))
	err := evaluator.ProcessArray(arr)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}
