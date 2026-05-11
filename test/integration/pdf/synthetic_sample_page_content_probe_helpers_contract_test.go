package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestExtractImagePlacementInvokesFromContentForProbe_TracksCMBeforeDo(t *testing.T) {
	content := "q 4 0 0 4 12.25 410.000732421875 cm /Im0 Do Q"

	results := extractImagePlacementInvokesFromContentForProbe(content, map[string]entity.Ref{
		"Im0": entity.NewRef(56, 0),
	})

	require.Len(t, results, 1)
	require.Equal(t, "Im0", results[0].imageName)
	require.Equal(t, entity.NewRef(56, 0), results[0].imageRef)
	require.Equal(t, [6]float64{4, 0, 0, 4, 12.25, 410.000732421875}, results[0].matrix)
}
