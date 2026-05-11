package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestSignatures_WithoutSignatureDocument(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "001-trivial", "minimal-document.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	signatures, err := doc.Signatures()
	require.NoError(t, err)
	require.Len(t, signatures, 0)

	verifications, err := doc.VerifySignatures()
	require.NoError(t, err)
	require.Len(t, verifications, 0)
}
