package pdf

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"time"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// Signature represents one digital signature field value.
type Signature struct {
	FieldName   string
	Filter      string
	SubFilter   string
	Name        string
	Reason      string
	Location    string
	ContactInfo string
	ModifiedAt  string
	ByteRange   []int64
	Contents    []byte
}

// SignatureVerification describes validation results for one signature.
type SignatureVerification struct {
	FieldName      string
	ByteRangeError string
	HasContents    bool
	HasByteRange   bool
	ByteRangeValid bool
	VerificationOK bool
}

// SignatureFieldSpec describes a visible signature field to be materialized on save.
type SignatureFieldSpec struct {
	FieldName  string
	Name       string
	Reason     string
	Location   string
	ModifiedAt string
	Contents   []byte
	ByteRange  []int64
	Rect       [4]float64
	PageIndex  int
}

type signatureFieldSnapshot struct {
	FieldName  string
	Name       string
	Reason     string
	Location   string
	ModifiedAt string
	Contents   []byte
	ByteRange  []int64
	Rect       [4]float64
	PageIndex  int
}

// SignatureDigest contains digest input/result for detached signing workflows.
type SignatureDigest struct {
	FieldName         string
	HashAlgorithm     string
	ByteRange         []int64
	Digest            []byte
	SignedContentSize int
}

// SetVisibleSignatureField registers or replaces one visible signature field in session scope.
// The actual PDF objects are created when SaveWithNativeSessionUpdates is called.
func (d *Document) SetVisibleSignatureField(spec SignatureFieldSpec) error {
	if spec.FieldName == "" {
		return fmt.Errorf("signature field name is required")
	}

	sourceIndex, err := d.resolveSourcePageIndex(spec.PageIndex)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	modifiedAt := spec.ModifiedAt
	if modifiedAt == "" {
		modifiedAt = time.Now().UTC().Format("D:20060102150405Z")
	}

	d.signatureFields[spec.FieldName] = signatureFieldSnapshot{
		FieldName:  spec.FieldName,
		PageIndex:  sourceIndex,
		Rect:       spec.Rect,
		Name:       spec.Name,
		Reason:     spec.Reason,
		Location:   spec.Location,
		ModifiedAt: modifiedAt,
		Contents:   append([]byte(nil), spec.Contents...),
		ByteRange:  append([]int64(nil), spec.ByteRange...),
	}

	return nil
}

// ClearVisibleSignatureField removes one session signature field registration.
func (d *Document) ClearVisibleSignatureField(fieldName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.signatureFields, fieldName)
}

// VisibleSignatureFields returns a copy of session signature field registrations.
func (d *Document) VisibleSignatureFields() []SignatureFieldSpec {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.signatureFields) == 0 {
		return nil
	}

	out := make([]SignatureFieldSpec, 0, len(d.signatureFields))
	for _, item := range d.signatureFields {
		out = append(out, SignatureFieldSpec{
			FieldName:  item.FieldName,
			PageIndex:  item.PageIndex,
			Rect:       item.Rect,
			Name:       item.Name,
			Reason:     item.Reason,
			Location:   item.Location,
			ModifiedAt: item.ModifiedAt,
			Contents:   append([]byte(nil), item.Contents...),
			ByteRange:  append([]int64(nil), item.ByteRange...),
		})
	}

	return out
}

// Signatures returns signature field values from AcroForm.
func (d *Document) Signatures() ([]*Signature, error) {
	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil, nil
	}

	acroObj := catalog.Get(entity.Name("AcroForm"))
	if acroObj == nil {
		return nil, nil
	}

	acroDict, err := d.asDict(acroObj)
	if err != nil {
		return nil, err
	}

	fieldsObj := acroDict.Get(entity.Name("Fields"))
	if fieldsObj == nil {
		return nil, nil
	}

	fieldsArr, err := d.asArray(fieldsObj)
	if err != nil {
		return nil, err
	}

	out := make([]*Signature, 0)
	visited := make(map[*entity.Dict]struct{})
	for i := 0; i < fieldsArr.Len(); i++ {
		if err := d.collectSignatures(fieldsArr.Get(i), "", "", visited, &out); err != nil {
			return nil, err
		}
	}

	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// VerifySignatures performs structural signature validation checks.
// This validates ByteRange shape and range bounds. Cryptographic validation
// is not performed in this method.
func (d *Document) VerifySignatures() ([]*SignatureVerification, error) {
	signatures, err := d.Signatures()
	if err != nil {
		return nil, err
	}
	if len(signatures) == 0 {
		return nil, nil
	}

	fileLength := d.doc.FileSize()
	if provider, ok := d.doc.XRef().(interface{ RawData() []byte }); ok {
		if raw := provider.RawData(); len(raw) > 0 {
			fileLength = int64(len(raw))
		}
	}

	out := make([]*SignatureVerification, 0, len(signatures))
	for _, sig := range signatures {
		verification := verifySignatureStructure(sig, fileLength)
		out = append(out, verification)
	}

	return out, nil
}

// SignatureDigest calculates digest from the signed content range for one signature.
// Supported hash algorithms: sha256 (default), sha384, sha512.
func (d *Document) SignatureDigest(fieldName, hashAlgorithm string) (*SignatureDigest, error) {
	if fieldName == "" {
		return nil, fmt.Errorf("signature field name is required")
	}

	signatures, err := d.Signatures()
	if err != nil {
		return nil, err
	}

	var target *Signature
	for _, sig := range signatures {
		if sig != nil && sig.FieldName == fieldName {
			target = sig
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("signature field not found: %s", fieldName)
	}

	rawProvider, ok := d.doc.XRef().(interface{ RawData() []byte })
	if !ok {
		return nil, fmt.Errorf("document xref does not provide raw bytes")
	}
	raw := rawProvider.RawData()
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty raw pdf stream")
	}

	verification := verifySignatureStructure(target, int64(len(raw)))
	if !verification.ByteRangeValid {
		return nil, fmt.Errorf("invalid signature byte range: %s", verification.ByteRangeError)
	}

	signedContent, err := signedContentFromByteRange(raw, target.ByteRange)
	if err != nil {
		return nil, err
	}

	algo := normalizeHashAlgorithm(hashAlgorithm)
	digest, err := computeDigest(algo, signedContent)
	if err != nil {
		return nil, err
	}

	return &SignatureDigest{
		FieldName:         fieldName,
		HashAlgorithm:     algo,
		ByteRange:         append([]int64(nil), target.ByteRange...),
		SignedContentSize: len(signedContent),
		Digest:            digest,
	}, nil
}

func (d *Document) collectSignatures(
	obj entity.Object,
	parentName string,
	parentFieldType string,
	visited map[*entity.Dict]struct{},
	out *[]*Signature,
) error {
	dict, err := d.asDict(obj)
	if err != nil {
		return err
	}
	if _, seen := visited[dict]; seen {
		return nil
	}
	visited[dict] = struct{}{}

	partial := extractEntityString(dict.Get(entity.Name("T")))
	fullName := parentName
	if partial != "" {
		if fullName == "" {
			fullName = partial
		} else {
			fullName = fullName + "." + partial
		}
	}

	fieldType := parentFieldType
	if ftObj := dict.Get(entity.Name("FT")); ftObj != nil {
		fieldType = extractEntityNameOrString(ftObj)
	}
	if fieldType == "Sig" {
		sig, err := d.extractSignature(fullName, dict)
		if err != nil {
			return err
		}
		if sig != nil {
			*out = append(*out, sig)
		}
	}

	kidsObj := dict.Get(entity.Name("Kids"))
	if kidsObj == nil {
		return nil
	}

	kidsArr, err := d.asArray(kidsObj)
	if err != nil {
		return err
	}
	for i := 0; i < kidsArr.Len(); i++ {
		if err := d.collectSignatures(kidsArr.Get(i), fullName, fieldType, visited, out); err != nil {
			return err
		}
	}

	return nil
}

func (d *Document) extractSignature(fieldName string, fieldDict *entity.Dict) (*Signature, error) {
	if fieldDict == nil {
		return nil, nil
	}

	vObj := fieldDict.Get(entity.Name("V"))
	if vObj == nil {
		return nil, nil
	}

	sigDict, err := d.asDict(vObj)
	if err != nil {
		return nil, fmt.Errorf("signature value is not dictionary: %w", err)
	}

	sig := &Signature{
		FieldName:   fieldName,
		Filter:      extractEntityNameOrString(sigDict.Get(entity.Name("Filter"))),
		SubFilter:   extractEntityNameOrString(sigDict.Get(entity.Name("SubFilter"))),
		Name:        extractEntityString(sigDict.Get(entity.Name("Name"))),
		Reason:      extractEntityString(sigDict.Get(entity.Name("Reason"))),
		Location:    extractEntityString(sigDict.Get(entity.Name("Location"))),
		ContactInfo: extractEntityString(sigDict.Get(entity.Name("ContactInfo"))),
		ModifiedAt:  extractEntityString(sigDict.Get(entity.Name("M"))),
	}

	if contentsObj := sigDict.Get(entity.Name("Contents")); contentsObj != nil {
		if content := extractEntityString(contentsObj); content != "" {
			sig.Contents = []byte(content)
		}
	}

	if brObj := sigDict.Get(entity.Name("ByteRange")); brObj != nil {
		brArr, err := d.asArray(brObj)
		if err != nil {
			return nil, fmt.Errorf("signature ByteRange is not array: %w", err)
		}
		sig.ByteRange = make([]int64, 0, brArr.Len())
		for i := 0; i < brArr.Len(); i++ {
			switch v := brArr.Get(i).(type) {
			case *entity.Integer:
				sig.ByteRange = append(sig.ByteRange, v.Value())
			case *entity.Real:
				sig.ByteRange = append(sig.ByteRange, int64(v.Value()))
			}
		}
	}

	return sig, nil
}

func verifySignatureStructure(sig *Signature, fileLength int64) *SignatureVerification {
	result := &SignatureVerification{}
	if sig == nil {
		result.ByteRangeError = "signature is nil"
		return result
	}

	result.FieldName = sig.FieldName
	result.HasContents = len(sig.Contents) > 0
	result.HasByteRange = len(sig.ByteRange) == 4

	if !result.HasByteRange {
		result.ByteRangeError = "ByteRange must have 4 integers"
		result.VerificationOK = false
		return result
	}

	start1 := sig.ByteRange[0]
	length1 := sig.ByteRange[1]
	start2 := sig.ByteRange[2]
	length2 := sig.ByteRange[3]

	if start1 < 0 || length1 < 0 || start2 < 0 || length2 < 0 {
		result.ByteRangeError = "ByteRange contains negative value"
		return result
	}
	if start1 != 0 {
		result.ByteRangeError = "ByteRange first offset must be 0"
		return result
	}
	if start1+length1 > start2 {
		result.ByteRangeError = "ByteRange segments overlap"
		return result
	}
	if fileLength > 0 && start2+length2 > fileLength {
		result.ByteRangeError = "ByteRange exceeds file length"
		return result
	}

	result.ByteRangeValid = true
	result.VerificationOK = result.HasContents && result.ByteRangeValid
	return result
}

func cloneSignatureFieldSnapshots(input map[string]signatureFieldSnapshot) map[string]signatureFieldSnapshot {
	if len(input) == 0 {
		return map[string]signatureFieldSnapshot{}
	}

	out := make(map[string]signatureFieldSnapshot, len(input))
	for name, item := range input {
		out[name] = signatureFieldSnapshot{
			FieldName:  item.FieldName,
			PageIndex:  item.PageIndex,
			Rect:       item.Rect,
			Name:       item.Name,
			Reason:     item.Reason,
			Location:   item.Location,
			ModifiedAt: item.ModifiedAt,
			Contents:   append([]byte(nil), item.Contents...),
			ByteRange:  append([]int64(nil), item.ByteRange...),
		}
	}
	return out
}

func signedContentFromByteRange(raw []byte, byteRange []int64) ([]byte, error) {
	if len(byteRange) != 4 {
		return nil, fmt.Errorf("byte range must have 4 values")
	}

	start1, length1 := byteRange[0], byteRange[1]
	start2, length2 := byteRange[2], byteRange[3]

	if start1 < 0 || length1 < 0 || start2 < 0 || length2 < 0 {
		return nil, fmt.Errorf("byte range contains negative value")
	}
	end1 := start1 + length1
	end2 := start2 + length2

	if end1 > int64(len(raw)) || end2 > int64(len(raw)) {
		return nil, fmt.Errorf("byte range exceeds input length")
	}

	out := make([]byte, 0, int(length1+length2))
	out = append(out, raw[start1:end1]...)
	out = append(out, raw[start2:end2]...)
	return out, nil
}

func normalizeHashAlgorithm(value string) string {
	switch value {
	case "", "sha256", "SHA256", "SHA-256":
		return "sha256"
	case "sha384", "SHA384", "SHA-384":
		return "sha384"
	case "sha512", "SHA512", "SHA-512":
		return "sha512"
	default:
		return value
	}
}

func computeDigest(algorithm string, payload []byte) ([]byte, error) {
	switch algorithm {
	case "sha256":
		sum := sha256.Sum256(payload)
		return sum[:], nil
	case "sha384":
		sum := sha512.Sum384(payload)
		return sum[:], nil
	case "sha512":
		sum := sha512.Sum512(payload)
		return sum[:], nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}
}
