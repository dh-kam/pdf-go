// Package entity provides PDF encryption-related types.
package entity

// Encryption represents PDF encryption metadata as defined in PDF 1.7 specification, section 3.5.
type Encryption struct {
	Filter          string
	SubFilter       string
	ID              []byte
	O               []byte
	U               []byte
	OE              []byte
	UE              []byte
	Perms           []byte
	Length          int
	R               int
	V               int
	P               uint32
	EncryptMetadata bool
}

// PermissionFlags represents PDF document permission flags.
// These flags control what operations are allowed on the document.
type PermissionFlags uint32

// PDF permission flag constants as defined in PDF 1.7 specification.
const (
	// PermPrint allows printing the document.
	PermPrint PermissionFlags = 1 << 2

	// PermModify allows modifying the document contents.
	PermModify PermissionFlags = 1 << 3

	// PermCopy allows copying or extracting text and graphics.
	PermCopy PermissionFlags = 1 << 4

	// PermAnnotate allows adding or modifying text annotations.
	PermAnnotate PermissionFlags = 1 << 5

	// PermFillForms allows filling in form fields.
	PermFillForms PermissionFlags = 1 << 8

	// PermExtract allows extracting text and graphics for accessibility.
	PermExtract PermissionFlags = 1 << 9

	// PermAssemble allows assembling the document (insert/rotate/delete pages).
	PermAssemble PermissionFlags = 1 << 11

	// PermPrintHighRes allows printing the document in high quality.
	PermPrintHighRes PermissionFlags = 1 << 12

	// PermOwner is the owner permission bit (bit 32, treated as negative)
	PermOwner PermissionFlags = 1 << 31
)

// HasPermission checks if the given permission is granted.
func (p PermissionFlags) HasPermission(flag PermissionFlags) bool {
	return p&flag != 0
}

// String returns a string representation of the permission flags.
func (p PermissionFlags) String() string {
	var result string
	if p.HasPermission(PermPrint) {
		result += "Print "
	}
	if p.HasPermission(PermModify) {
		result += "Modify "
	}
	if p.HasPermission(PermCopy) {
		result += "Copy "
	}
	if p.HasPermission(PermAnnotate) {
		result += "Annotate "
	}
	if p.HasPermission(PermFillForms) {
		result += "FillForms "
	}
	if p.HasPermission(PermExtract) {
		result += "Extract "
	}
	if p.HasPermission(PermAssemble) {
		result += "Assemble "
	}
	if p.HasPermission(PermPrintHighRes) {
		result += "PrintHighRes "
	}
	if p.HasPermission(PermOwner) {
		result += "Owner "
	}
	return result
}

// Encryption algorithm constants.
const (
	// Algorithm 1: RC4 with 40-bit key (V=0, R=2)
	AlgorithmRC4_40 = 0

	// Algorithm 2: RC4 with 40-bit key (V=1, R=2) - deprecated
	AlgorithmRC4_40Deprecated = 1

	// Algorithm 3: RC4 or AES with variable key length (V=2, R=3-4)
	AlgorithmVariable = 2

	// Algorithm 5: AES-256 (V=4, R=5)
	AlgorithmAES256 = 4

	// Algorithm 6: AES-256 (V=5, R=6)
	AlgorithmAES256Rev = 5
)

// CryptoFilter represents a cryptographic filter used in PDF encryption.
type CryptoFilter struct {
	// AuthEvent indicates when the security handler is invoked.
	// Values: "DocOpen", "EFOpen"
	AuthEvent string

	// CFM is the cryptographic method name.
	// Values: "None", "V2", "AESV2", "AESV3", "Identity"
	CFM string

	// Length is the encryption key length in bits.
	Length int
}

// EncryptDict represents the full encryption dictionary from the PDF.
type EncryptDict struct {
	CF              map[string]*CryptoFilter
	SubFilter       string
	StmF            string
	StrF            string
	Filter          string
	UE              []byte
	O               []byte
	U               []byte
	Perms           []byte
	ID              []byte
	OE              []byte
	R               int
	Length          int
	V               int
	P               uint32
	EncryptMetadata bool
}

// ToEncryption converts an EncryptDict to an Encryption entity.
func (e *EncryptDict) ToEncryption() *Encryption {
	enc := &Encryption{
		Filter:          e.Filter,
		SubFilter:       e.SubFilter,
		V:               e.V,
		Length:          e.Length,
		R:               e.R,
		O:               e.O,
		U:               e.U,
		P:               e.P,
		EncryptMetadata: e.EncryptMetadata,
		ID:              e.ID,
		OE:              e.OE,
		UE:              e.UE,
		Perms:           e.Perms,
	}

	// Set default Length if not specified
	if enc.Length == 0 && enc.R >= 3 {
		enc.Length = 128 // Default for R3+
	} else if enc.Length == 0 && enc.R == 2 {
		enc.Length = 40 // Default for R2
	}

	return enc
}

// encryption errors.
var (
	// ErrEncryptionUnsupported is returned when the encryption algorithm is not supported.
	ErrEncryptionUnsupported = &PDFError{Op: "encryption", Err: ErrInvalid, Type: ErrTypeEncryption}

	// ErrInvalidPassword is returned when the provided password is incorrect.
	ErrInvalidPassword = &PDFError{Op: "encryption", Err: ErrInvalid, Type: ErrTypeEncryption}

	// ErrEncryptionKey is returned when key derivation fails.
	ErrEncryptionKey = &PDFError{Op: "encryption", Err: ErrInvalid, Type: ErrTypeEncryption}

	// ErrDecryptFailed is returned when decryption fails.
	ErrDecryptFailed = &PDFError{Op: "encryption", Err: ErrInvalid, Type: ErrTypeEncryption}
)

// IsEncrypted returns true if the document trailer contains an Encrypt entry.
func IsEncrypted(trailer *Dict) bool {
	if trailer == nil {
		return false
	}
	return trailer.Get(Name("/Encrypt")) != nil
}
