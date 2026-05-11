package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func realPageSFRMNonLowerProbeCaseForLowercase(t *testing.T) realPageLowercaseProbeCase {
	t.Helper()
	cases := realPageSFRMNonLowerLowercaseProbeCases()
	require.Len(t, cases, 1)
	return cases[0]
}

func realPageSFRMLongTailProbeCaseForLowercase(t *testing.T) realPageLowercaseProbeCase {
	t.Helper()
	cases := realPageSFRMLongTailLowercaseProbeCases()
	require.Len(t, cases, 1)
	return cases[0]
}

