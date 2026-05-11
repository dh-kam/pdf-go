package pdf

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestExtractSignature(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	sigDict := entity.NewDict()
	sigDict.Set(entity.Name("Filter"), entity.NewName("Adobe.PPKLite"))
	sigDict.Set(entity.Name("SubFilter"), entity.NewName("adbe.pkcs7.detached"))
	sigDict.Set(entity.Name("ByteRange"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(100),
		entity.NewInteger(200),
		entity.NewInteger(300),
	))
	sigDict.Set(entity.Name("Contents"), entity.NewString("SIGNATURE"))
	sigDict.Set(entity.Name("Reason"), entity.NewString("Approved"))

	fieldDict := entity.NewDict()
	fieldDict.Set(entity.Name("V"), sigDict)

	sig, err := doc.extractSignature("Sig1", fieldDict)
	require.NoError(t, err)
	require.NotNil(t, sig)

	assert.Equal(t, "Sig1", sig.FieldName)
	assert.Equal(t, "Adobe.PPKLite", sig.Filter)
	assert.Equal(t, "adbe.pkcs7.detached", sig.SubFilter)
	assert.Equal(t, []int64{0, 100, 200, 300}, sig.ByteRange)
	assert.Equal(t, []byte("SIGNATURE"), sig.Contents)
	assert.Equal(t, "Approved", sig.Reason)
}

func TestSignatures_FromAcroForm(t *testing.T) {
	entityDoc := entity.NewDocument(nil)

	sigVal := entity.NewDict()
	sigVal.Set(entity.Name("Filter"), entity.NewName("Adobe.PPKLite"))
	sigVal.Set(entity.Name("SubFilter"), entity.NewName("adbe.pkcs7.detached"))
	sigVal.Set(entity.Name("Contents"), entity.NewString("X"))

	field := entity.NewDict()
	field.Set(entity.Name("T"), entity.NewString("Signature1"))
	field.Set(entity.Name("FT"), entity.NewName("Sig"))
	field.Set(entity.Name("V"), sigVal)

	acro := entity.NewDict()
	acro.Set(entity.Name("Fields"), entity.NewArray(field))

	catalog := entity.NewDict()
	catalog.Set(entity.Name("AcroForm"), acro)
	entityDoc.SetCatalog(catalog)

	doc := newDocument(entityDoc)
	sigs, err := doc.Signatures()
	require.NoError(t, err)
	require.Len(t, sigs, 1)
	assert.Equal(t, "Signature1", sigs[0].FieldName)
	assert.Equal(t, "Adobe.PPKLite", sigs[0].Filter)
}

func TestVerifySignatureStructure(t *testing.T) {
	valid := verifySignatureStructure(&Signature{
		FieldName: "Sig1",
		Contents:  []byte("signed"),
		ByteRange: []int64{0, 100, 200, 300},
	}, 600)
	require.NotNil(t, valid)
	assert.True(t, valid.VerificationOK)
	assert.True(t, valid.ByteRangeValid)
	assert.Equal(t, "Sig1", valid.FieldName)

	invalid := verifySignatureStructure(&Signature{
		FieldName: "Sig2",
		Contents:  []byte("signed"),
		ByteRange: []int64{0, 100, 150, 500},
	}, 600)
	require.NotNil(t, invalid)
	assert.False(t, invalid.VerificationOK)
	assert.False(t, invalid.ByteRangeValid)
	assert.NotEmpty(t, invalid.ByteRangeError)
}

func TestVisibleSignatureFieldSession(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0}

	require.NoError(t, doc.SetVisibleSignatureField(SignatureFieldSpec{
		FieldName: "SigA",
		PageIndex: 0,
		Rect:      [4]float64{10, 10, 20, 20},
		Name:      "Alice",
		Contents:  []byte("X"),
		ByteRange: []int64{0, 1, 2, 3},
	}))

	fields := doc.VisibleSignatureFields()
	require.Len(t, fields, 1)
	assert.Equal(t, "SigA", fields[0].FieldName)
	assert.Equal(t, 0, fields[0].PageIndex)
	assert.Equal(t, []byte("X"), fields[0].Contents)

	doc.ClearVisibleSignatureField("SigA")
	assert.Len(t, doc.VisibleSignatureFields(), 0)
}

func TestSignedContentFromByteRange(t *testing.T) {
	raw := []byte("0123456789ABCDEFGHIJ")
	content, err := signedContentFromByteRange(raw, []int64{0, 5, 10, 5})
	require.NoError(t, err)
	assert.Equal(t, []byte("01234ABCDE"), content)
}

func TestComputeDigest_SHA256(t *testing.T) {
	payload := []byte("hello")
	digest, err := computeDigest("sha256", payload)
	require.NoError(t, err)
	sum := sha256.Sum256(payload)
	assert.Equal(t, sum[:], digest)
}
