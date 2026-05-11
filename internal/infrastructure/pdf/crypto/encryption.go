// Package crypto implements PDF encryption handler for decrypting encrypted PDFs.
package crypto

import (
	"fmt"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// EncryptionHandler handles PDF encryption and decryption operations.
type EncryptionHandler struct {
	encryption        *entity.Encryption
	keyDeriv          *KeyDerivation
	key               []byte
	authenticated     bool
	usedOwnerPassword bool
}

// NewEncryptionHandler creates a new encryption handler.
// It attempts to authenticate with the provided password.
// If the password is empty, it will try to open with no password (user password).
func NewEncryptionHandler(encryption *entity.Encryption, password string) (*EncryptionHandler, error) {
	if encryption == nil {
		return nil, fmt.Errorf("encryption metadata is nil")
	}

	handler := &EncryptionHandler{
		encryption:        encryption,
		keyDeriv:          NewKeyDerivation(encryption),
		authenticated:     false,
		usedOwnerPassword: false,
	}

	// Try to authenticate with the provided password
	if password != "" {
		// First try as user password
		if handler.keyDeriv.AuthenticateUserPassword(password) {
			key, err := handler.keyDeriv.ComputeEncryptionKey(password)
			if err == nil {
				handler.key = key
				handler.authenticated = true
				return handler, nil
			}
		}

		// Then try as owner password
		if handler.keyDeriv.AuthenticateOwnerPassword(password) {
			key, err := handler.keyDeriv.ComputeEncryptionKey(password)
			if err == nil {
				handler.key = key
				handler.authenticated = true
				handler.usedOwnerPassword = true
				return handler, nil
			}
		}

		// Password authentication failed
		return nil, entity.ErrInvalidPassword
	}

	// Try empty password (default user password)
	if handler.keyDeriv.AuthenticateUserPassword("") {
		key, err := handler.keyDeriv.ComputeEncryptionKey("")
		if err == nil {
			handler.key = key
			handler.authenticated = true
			return handler, nil
		}
	}

	// No valid password provided
	return nil, entity.ErrInvalidPassword
}

// Decrypt decrypts data for a specific PDF object.
// The object number and generation number are used to compute
// the object-specific encryption key.
func (h *EncryptionHandler) Decrypt(data []byte, objNum uint32, gen uint16) ([]byte, error) {
	if !h.authenticated {
		return nil, fmt.Errorf("not authenticated")
	}

	if h.key == nil {
		return nil, fmt.Errorf("encryption key not set")
	}

	// Compute object-specific key
	objKey := h.keyDeriv.ComputeObjectKeyWithBaseKey(h.key, objNum, gen)

	// Decrypt based on algorithm
	switch h.encryption.V {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated:
		// RC4 encryption
		rc4 := NewRC4Cipher(objKey)
		return rc4.Decrypt(data), nil

	case AlgorithmVariable:
		// Variable key length (RC4 or AES-128)
		if h.encryption.R >= 4 {
			// AES-128
			aes := NewAESCipher(objKey)
			return aes.DecryptCBC(data, make([]byte, 16)), nil
		}
		// RC4
		rc4 := NewRC4Cipher(objKey)
		return rc4.Decrypt(data), nil

	case AlgorithmAES256, AlgorithmAES256Rev:
		// AES-256
		aes := NewAESCipher(objKey)
		return aes.DecryptCBC(data, make([]byte, 16)), nil

	default:
		return nil, fmt.Errorf("unsupported encryption algorithm: V=%d", h.encryption.V)
	}
}

// DecryptString decrypts an encrypted PDF string.
func (h *EncryptionHandler) DecryptString(data []byte) ([]byte, error) {
	if !h.authenticated {
		return nil, fmt.Errorf("not authenticated")
	}

	if h.key == nil {
		return nil, fmt.Errorf("encryption key not set")
	}

	// For strings, use the base key directly
	switch h.encryption.V {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated, AlgorithmVariable:
		if h.encryption.R < 4 {
			// RC4
			rc4 := NewRC4Cipher(h.key)
			return rc4.Decrypt(data), nil
		}
		// AES-128 ECB for strings
		aes := NewAESCipher(h.key)
		return aes.DecryptECB(data), nil

	case AlgorithmAES256, AlgorithmAES256Rev:
		// AES-256 ECB for strings
		aes := NewAESCipher(h.key)
		return aes.DecryptECB(data), nil

	default:
		return nil, fmt.Errorf("unsupported encryption algorithm: V=%d", h.encryption.V)
	}
}

// DecryptStringForObject decrypts an encrypted PDF string for a specific object.
func (h *EncryptionHandler) DecryptStringForObject(data []byte, objNum uint32, gen uint16) ([]byte, error) {
	if !h.authenticated {
		return nil, fmt.Errorf("not authenticated")
	}

	if h.key == nil {
		return nil, fmt.Errorf("encryption key not set")
	}

	objKey := h.keyDeriv.ComputeObjectKeyWithBaseKey(h.key, objNum, gen)

	switch h.encryption.V {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated:
		return NewRC4Cipher(objKey).Decrypt(data), nil
	case AlgorithmVariable:
		if h.encryption.R < 4 {
			return NewRC4Cipher(objKey).Decrypt(data), nil
		}
		aes := NewAESCipher(objKey)
		if len(data) > 16 {
			iv := data[:16]
			return aes.DecryptCBC(data[16:], iv), nil
		}
		return aes.DecryptECB(data), nil
	case AlgorithmAES256, AlgorithmAES256Rev:
		aes := NewAESCipher(objKey)
		if len(data) > 16 {
			iv := data[:16]
			return aes.DecryptCBC(data[16:], iv), nil
		}
		return aes.DecryptECB(data), nil
	default:
		return nil, fmt.Errorf("unsupported encryption algorithm: V=%d", h.encryption.V)
	}
}

// DecryptStream decrypts an encrypted PDF stream.
func (h *EncryptionHandler) DecryptStream(data []byte, objNum uint32, gen uint16) ([]byte, error) {
	if !h.authenticated {
		return nil, fmt.Errorf("not authenticated")
	}

	if h.key == nil {
		return nil, fmt.Errorf("encryption key not set")
	}

	// Compute object-specific key
	objKey := h.keyDeriv.ComputeObjectKeyWithBaseKey(h.key, objNum, gen)

	// Decrypt based on algorithm
	switch h.encryption.V {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated, AlgorithmVariable:
		if h.encryption.R < 4 {
			// RC4
			rc4 := NewRC4Cipher(objKey)
			return rc4.Decrypt(data), nil
		}
		// AES-128 CBC for streams
		aes := NewAESCipher(objKey)
		if len(data) > 16 {
			// Extract IV from beginning
			iv := data[:16]
			return aes.DecryptCBC(data[16:], iv), nil
		}
		return aes.DecryptCBC(data, make([]byte, 16)), nil

	case AlgorithmAES256, AlgorithmAES256Rev:
		// AES-256 CBC for streams
		aes := NewAESCipher(objKey)
		if len(data) > 16 {
			// Extract IV from beginning
			iv := data[:16]
			return aes.DecryptCBC(data[16:], iv), nil
		}
		return aes.DecryptCBC(data, make([]byte, 16)), nil

	default:
		return nil, fmt.Errorf("unsupported encryption algorithm: V=%d", h.encryption.V)
	}
}

// Authenticate checks if a password is valid.
func (h *EncryptionHandler) Authenticate(password string) bool {
	// Try user password first
	if h.keyDeriv.AuthenticateUserPassword(password) {
		key, err := h.keyDeriv.ComputeEncryptionKey(password)
		if err == nil {
			h.key = key
			h.authenticated = true
			h.usedOwnerPassword = false
			return true
		}
	}

	// Try owner password
	if h.keyDeriv.AuthenticateOwnerPassword(password) {
		key, err := h.keyDeriv.ComputeEncryptionKey(password)
		if err == nil {
			h.key = key
			h.authenticated = true
			h.usedOwnerPassword = true
			return true
		}
	}

	return false
}

// IsAuthenticated returns true if the handler is authenticated.
func (h *EncryptionHandler) IsAuthenticated() bool {
	return h.authenticated
}

// UsedOwnerPassword returns true if owner password was used for authentication.
func (h *EncryptionHandler) UsedOwnerPassword() bool {
	return h.usedOwnerPassword
}

// Permissions returns the permission flags for the document.
func (h *EncryptionHandler) Permissions() entity.PermissionFlags {
	return entity.PermissionFlags(h.encryption.P)
}

// HasPermission checks if a specific permission is granted.
func (h *EncryptionHandler) HasPermission(flag entity.PermissionFlags) bool {
	// Owner password grants all permissions
	if h.usedOwnerPassword {
		return true
	}

	return h.Permissions().HasPermission(flag)
}

// Key returns the encryption key.
func (h *EncryptionHandler) Key() []byte {
	return h.key
}

// Encryption returns the encryption metadata.
func (h *EncryptionHandler) Encryption() *entity.Encryption {
	return h.encryption
}

// Algorithm returns the encryption algorithm version.
func (h *EncryptionHandler) Algorithm() int {
	return h.encryption.V
}

// Revision returns the encryption algorithm revision.
func (h *EncryptionHandler) Revision() int {
	return h.encryption.R
}

// KeyLength returns the encryption key length in bits.
func (h *EncryptionHandler) KeyLength() int {
	if h.encryption.Length > 0 {
		return h.encryption.Length
	}

	// Default based on revision
	switch h.encryption.R {
	case 2:
		return 40
	case 3:
		return 128
	case 4:
		return 128
	case 5, 6:
		return 256
	default:
		return 40
	}
}

// EncryptMetadata returns true if metadata is encrypted.
func (h *EncryptionHandler) EncryptMetadata() bool {
	return h.encryption.EncryptMetadata
}

// CanPrint returns true if printing is allowed.
func (h *EncryptionHandler) CanPrint() bool {
	return h.HasPermission(entity.PermPrint) || h.HasPermission(entity.PermPrintHighRes)
}

// CanModify returns true if document modification is allowed.
func (h *EncryptionHandler) CanModify() bool {
	return h.HasPermission(entity.PermModify)
}

// CanCopy returns true if copying text/graphics is allowed.
func (h *EncryptionHandler) CanCopy() bool {
	return h.HasPermission(entity.PermCopy)
}

// CanAnnotate returns true if adding annotations is allowed.
func (h *EncryptionHandler) CanAnnotate() bool {
	return h.HasPermission(entity.PermAnnotate)
}

// CanFillForms returns true if filling form fields is allowed.
func (h *EncryptionHandler) CanFillForms() bool {
	return h.HasPermission(entity.PermFillForms)
}

// CanExtract returns true if extracting content is allowed.
func (h *EncryptionHandler) CanExtract() bool {
	return h.HasPermission(entity.PermExtract)
}

// CanAssemble returns true if document assembly is allowed.
func (h *EncryptionHandler) CanAssemble() bool {
	return h.HasPermission(entity.PermAssemble)
}

// CanPrintHighRes returns true if high-resolution printing is allowed.
func (h *EncryptionHandler) CanPrintHighRes() bool {
	return h.HasPermission(entity.PermPrintHighRes)
}

// ParseEncryptionDict parses an encryption dictionary from a PDF dict object.
func ParseEncryptionDict(dict *entity.Dict, id []byte) (*entity.Encryption, error) {
	if dict == nil {
		return nil, fmt.Errorf("encryption dictionary is nil")
	}

	enc := &entity.Encryption{}

	// Get Filter (security handler)
	if filter := dict.Get(entity.Name("/Filter")); filter != nil {
		if name, ok := filter.(entity.Name); ok {
			enc.Filter = string(name)
		}
	}

	// Get SubFilter
	if subFilter := dict.Get(entity.Name("/SubFilter")); subFilter != nil {
		if name, ok := subFilter.(entity.Name); ok {
			enc.SubFilter = string(name)
		}
	}

	// Get V (encryption version)
	if v := dict.Get(entity.Name("/V")); v != nil {
		if num, ok := v.(*entity.Integer); ok {
			enc.V = int(num.Value())
		}
	}

	// Get Length (key length in bits)
	if length := dict.Get(entity.Name("/Length")); length != nil {
		if num, ok := length.(*entity.Integer); ok {
			enc.Length = int(num.Value())
		}
	}

	// Get R (revision)
	if r := dict.Get(entity.Name("/R")); r != nil {
		if num, ok := r.(*entity.Integer); ok {
			enc.R = int(num.Value())
		}
	}

	// Get O (owner password hash)
	if o := dict.Get(entity.Name("/O")); o != nil {
		if str, ok := o.(*entity.String); ok {
			enc.O = encryptionBytesFromString(str)
		} else if ary, ok := o.(*entity.Array); ok {
			// O might be stored as array of bytes
			enc.O = make([]byte, ary.Len())
			for i := 0; i < ary.Len(); i++ {
				if val := ary.Get(i); val != nil {
					if num, ok := val.(*entity.Integer); ok {
						enc.O[i] = byte(num.Value())
					}
				}
			}
		}
	}

	// Get U (user password hash)
	if u := dict.Get(entity.Name("/U")); u != nil {
		if str, ok := u.(*entity.String); ok {
			enc.U = encryptionBytesFromString(str)
		} else if ary, ok := u.(*entity.Array); ok {
			// U might be stored as array of bytes
			enc.U = make([]byte, ary.Len())
			for i := 0; i < ary.Len(); i++ {
				if val := ary.Get(i); val != nil {
					if num, ok := val.(*entity.Integer); ok {
						enc.U[i] = byte(num.Value())
					}
				}
			}
		}
	}

	// Get P (permissions)
	if p := dict.Get(entity.Name("/P")); p != nil {
		if num, ok := p.(*entity.Integer); ok {
			enc.P = uint32(num.Value())
		}
	}

	// Get EncryptMetadata
	if encryptMeta := dict.Get(entity.Name("/EncryptMetadata")); encryptMeta != nil {
		if b, ok := encryptMeta.(*entity.Boolean); ok {
			enc.EncryptMetadata = b.Value()
		}
	}

	// Set ID
	enc.ID = id

	// Get OE (owner encryption key, R5+)
	if oe := dict.Get(entity.Name("/OE")); oe != nil {
		if str, ok := oe.(*entity.String); ok {
			enc.OE = encryptionBytesFromString(str)
		}
	}

	// Get UE (user encryption key, R5+)
	if ue := dict.Get(entity.Name("/UE")); ue != nil {
		if str, ok := ue.(*entity.String); ok {
			enc.UE = encryptionBytesFromString(str)
		}
	}

	// Get Perms (permissions validation, R5+)
	if perms := dict.Get(entity.Name("/Perms")); perms != nil {
		if str, ok := perms.(*entity.String); ok {
			enc.Perms = encryptionBytesFromString(str)
		}
	}

	// Set defaults
	if enc.Filter == "" {
		enc.Filter = "Standard"
	}

	if enc.V == 0 {
		if enc.R >= 4 {
			enc.V = AlgorithmVariable
		} else {
			enc.V = AlgorithmRC4_40
		}
	}

	if enc.Length == 0 {
		if enc.R >= 3 {
			enc.Length = 128
		} else {
			enc.Length = 40
		}
	}

	return enc, nil
}

func encryptionBytesFromString(str *entity.String) []byte {
	if str == nil {
		return nil
	}

	if str.IsHex() {
		return []byte(str.Value())
	}

	value := str.Value()
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return decodeHexString(value)
	}

	return []byte(value)
}

// decodeHexString decodes a hex string to bytes.
func decodeHexString(s string) []byte {
	// Remove any whitespace
	s = strings.TrimSpace(s)

	// Handle hex strings that may or may not have a prefix
	if strings.HasPrefix(s, "<") {
		s = strings.TrimPrefix(s, "<")
		s = strings.TrimSuffix(s, ">")
	}

	// Decode hex
	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		if i+1 < len(s) {
			b, err := parseHexByte(s[i], s[i+1])
			if err == nil {
				result[i/2] = b
			}
		}
	}
	return result
}

// parseHexByte parses two hex characters to a byte.
func parseHexByte(c1, c2 byte) (byte, error) {
	var b byte
	var err error

	b, err = parseHexChar(c1)
	if err != nil {
		return 0, err
	}

	b <<= 4
	var b2 byte
	b2, err = parseHexChar(c2)
	if err != nil {
		return 0, err
	}

	return b | b2, nil
}

// parseHexChar parses a single hex character to its 4-bit value.
func parseHexChar(c byte) (byte, error) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', nil
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, nil
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("invalid hex character: %c", c)
	}
}

// CreateEncryptionHandlerFromDict creates an encryption handler from an encryption dictionary.
func CreateEncryptionHandlerFromDict(dict *entity.Dict, id []byte, password string) (*EncryptionHandler, error) {
	enc, err := ParseEncryptionDict(dict, id)
	if err != nil {
		return nil, err
	}

	return NewEncryptionHandler(enc, password)
}
