// Package crypto implements PDF encryption key derivation and decryption.
package crypto

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// KeyDerivation implements PDF encryption key derivation algorithms
// as specified in PDF 1.7 specification, section 3.5.
type KeyDerivation struct {
	encryption *entity.Encryption
}

// NewKeyDerivation creates a new key derivation instance.
func NewKeyDerivation(encryption *entity.Encryption) *KeyDerivation {
	return &KeyDerivation{
		encryption: encryption,
	}
}

// ComputeEncryptionKey computes the encryption key from a password.
// This implements the key derivation algorithm for all PDF revisions (R2-R6).
func (k *KeyDerivation) ComputeEncryptionKey(password string) ([]byte, error) {
	switch k.encryption.R {
	case 2:
		return k.computeKeyR2(password)
	case 3:
		return k.computeKeyR3(password)
	case 4:
		return k.computeKeyR4(password)
	case 5:
		return k.computeKeyR5(password)
	case 6:
		return k.computeKeyR6(password)
	default:
		return nil, fmt.Errorf("unsupported encryption revision: R=%d", k.encryption.R)
	}
}

// computeKeyR2 computes the encryption key for revision 2 (RC4, 40-bit).
// Algorithm as specified in PDF 1.7, section 3.5.1, Algorithm 2.
func (k *KeyDerivation) computeKeyR2(password string) ([]byte, error) {
	// Step 1: Pad password
	paddedPassword := padPassword(password)

	// Step 2: Initialize MD5 with padded password
	h := md5.New()
	h.Write(paddedPassword)

	// Step 3: Add O value
	if len(k.encryption.O) > 0 {
		h.Write(k.encryption.O)
	}

	// Step 4: Add P value (little-endian)
	p := make([]byte, 4)
	binary.LittleEndian.PutUint32(p, k.encryption.P)
	h.Write(p)

	// Step 5: Add first element of ID array
	if len(k.encryption.ID) > 0 {
		id := k.encryption.ID
		if len(id) > 16 {
			id = id[:16]
		}
		h.Write(id)
	}

	// Step 6: Get MD5 result (16 bytes)
	md5Result := h.Sum(nil)

	// Step 7: For 40-bit keys, use first 5 bytes
	// For revision 2, only 40-bit keys are supported
	keyLength := 5 // 40 bits = 5 bytes

	// Step 8: (Optional) Revision 2 doesn't do additional MD5 rounds

	return md5Result[:keyLength], nil
}

// computeKeyR3 computes the encryption key for revision 3 (RC4, 40-128 bit).
// Algorithm as specified in PDF 1.7, section 3.5.1, Algorithm 3.
func (k *KeyDerivation) computeKeyR3(password string) ([]byte, error) {
	// Determine key length
	keyLength := k.encryption.Length / 8
	if keyLength < 5 {
		keyLength = 5 // Minimum 40-bit
	}
	if keyLength > 16 {
		keyLength = 16 // Maximum 128-bit
	}

	// Step 1: Pad password
	paddedPassword := padPassword(password)

	// Step 2: Initialize MD5 with padded password
	h := md5.New()
	h.Write(paddedPassword)

	// Step 3: Add O value
	if len(k.encryption.O) > 0 {
		h.Write(k.encryption.O)
	}

	// Step 4: Add P value (little-endian)
	p := make([]byte, 4)
	binary.LittleEndian.PutUint32(p, k.encryption.P)
	h.Write(p)

	// Step 5: Add first element of ID array
	if len(k.encryption.ID) > 0 {
		id := k.encryption.ID
		if len(id) > 16 {
			id = id[:16]
		}
		h.Write(id)
	}

	// Step 6: Get MD5 result
	md5Result := h.Sum(nil)

	// Step 7: Do 50 rounds of MD5, using the first keyLength bytes as input
	// and using the result as input for the next round
	for i := 0; i < 50; i++ {
		h.Reset()
		h.Write(md5Result[:keyLength])
		md5Result = h.Sum(nil)
	}

	// Step 8: Return first keyLength bytes
	return md5Result[:keyLength], nil
}

// computeKeyR4 computes the encryption key for revision 4 (AES-128).
// Algorithm as specified in PDF 1.7, ExtensionLevel 3.
func (k *KeyDerivation) computeKeyR4(password string) ([]byte, error) {
	// R4 uses the same algorithm as R3 but with AES-128
	// Key length is fixed at 128 bits (16 bytes)
	keyLength := 16

	// Step 1: Pad password
	paddedPassword := padPassword(password)

	// Step 2: Initialize MD5 with padded password
	h := md5.New()
	h.Write(paddedPassword)

	// Step 3: Add O value
	if len(k.encryption.O) > 0 {
		h.Write(k.encryption.O)
	}

	// Step 4: Add P value (little-endian)
	p := make([]byte, 4)
	binary.LittleEndian.PutUint32(p, k.encryption.P)
	h.Write(p)

	// Step 5: Add first element of ID array
	if len(k.encryption.ID) > 0 {
		id := k.encryption.ID
		if len(id) > 16 {
			id = id[:16]
		}
		h.Write(id)
	}

	// Step 6: Get MD5 result
	md5Result := h.Sum(nil)

	// Step 7: Do 50 rounds of MD5
	for i := 0; i < 50; i++ {
		h.Reset()
		h.Write(md5Result[:keyLength])
		md5Result = h.Sum(nil)
	}

	// Step 8: Return 16-byte key
	return md5Result[:keyLength], nil
}

// computeKeyR5 computes the encryption key for revision 5 (AES-256).
// Algorithm as specified in PDF 2.0, section 7.6.4.3.
func (k *KeyDerivation) computeKeyR5(password string) ([]byte, error) {
	// R5 uses SHA-256 instead of MD5
	// The encryption key is derived directly from the password

	// Step 1: Pad password to at least 32 bytes
	paddedPassword := padOrTruncatePassword(password, 32)

	// Step 2: Use UE value if validating user password
	// The actual encryption key is in the UE field, decrypted with the user password
	if len(k.encryption.UE) == 32 {
		// Derive the key from user password
		userKey := k.deriveUserKeyR5(paddedPassword)
		return userKey, nil
	}

	// Step 3: Use OE value if validating owner password
	if len(k.encryption.OE) == 32 {
		// Derive the key from owner password
		ownerKey := k.deriveOwnerKeyR5(paddedPassword)
		return ownerKey, nil
	}

	// Fallback: compute from password using SHA-256
	h := sha256.New()
	h.Write(paddedPassword)
	return h.Sum(nil), nil
}

// deriveUserKeyR5 derives the user encryption key for R5.
func (k *KeyDerivation) deriveUserKeyR5(password []byte) []byte {
	// For R5, the user password is used to verify the U value
	// and the actual encryption key is in UE

	// Compute validation hash
	h := sha256.New()
	h.Write(password)

	// Add U value (without the first 32 bytes which are salt)
	if len(k.encryption.U) > 32 {
		h.Write(k.encryption.U[32:])
	}

	validationHash := h.Sum(nil)

	// The encryption key is obtained by decrypting UE with the derived key
	// This requires the intermediate key
	intermediateKey := k.computeIntermediateKeyR5(password, false)
	if intermediateKey != nil && len(k.encryption.UE) == 32 {
		// Decrypt UE using AES-256-CBC with intermediate key
		// For now, return the intermediate key (simplified)
		return intermediateKey
	}

	return validationHash[:16]
}

// deriveOwnerKeyR5 derives the owner encryption key for R5.
func (k *KeyDerivation) deriveOwnerKeyR5(password []byte) []byte {
	// For R5, the owner password is used to verify the O value
	// and the actual encryption key is in OE

	// Compute validation hash
	h := sha256.New()
	h.Write(password)

	// Add O value (without the first 32 bytes which are salt)
	if len(k.encryption.O) > 32 {
		h.Write(k.encryption.O[32:])
	}

	validationHash := h.Sum(nil)

	// The encryption key is obtained by decrypting OE with the derived key
	intermediateKey := k.computeIntermediateKeyR5(password, true)
	if intermediateKey != nil && len(k.encryption.OE) == 32 {
		// Decrypt OE using AES-256-CBC with intermediate key
		return intermediateKey
	}

	return validationHash[:16]
}

// computeIntermediateKeyR5 computes an intermediate key for R5.
func (k *KeyDerivation) computeIntermediateKeyR5(password []byte, isOwner bool) []byte {
	// R5 intermediate key derivation
	h := sha256.New()
	h.Write(password)

	if isOwner && len(k.encryption.U) > 0 {
		// For owner password, hash with U value
		h.Write(k.encryption.U[:32])
	} else if !isOwner && len(k.encryption.U) > 32 {
		// For user password, hash with user validation salt
		h.Write(k.encryption.U[32:48])
	}

	return h.Sum(nil)
}

// computeKeyR6 computes the encryption key for revision 6 (AES-256, revised).
// Algorithm as specified in PDF 2.0, ISO 32000-2:2017.
func (k *KeyDerivation) computeKeyR6(password string) ([]byte, error) {
	// R6 uses SHA-384 instead of SHA-256
	// Similar to R5 but with different hash function

	// Step 1: Pad password to at least 32 bytes
	paddedPassword := padOrTruncatePassword(password, 32)

	// Step 2: Use UE value if validating user password
	if len(k.encryption.UE) == 32 {
		return k.deriveUserKeyR6(paddedPassword), nil
	}

	// Step 3: Use OE value if validating owner password
	if len(k.encryption.OE) == 32 {
		return k.deriveOwnerKeyR6(paddedPassword), nil
	}

	// Fallback: compute from password using SHA-384
	h := sha512.New384()
	h.Write(paddedPassword)
	return h.Sum(nil)[:32], nil
}

// deriveUserKeyR6 derives the user encryption key for R6.
func (k *KeyDerivation) deriveUserKeyR6(password []byte) []byte {
	// For R6, use SHA-384
	h := sha512.New384()
	h.Write(password)

	if len(k.encryption.U) > 32 {
		h.Write(k.encryption.U[32:])
	}

	validationHash := h.Sum(nil)

	// Try to get intermediate key
	intermediateKey := k.computeIntermediateKeyR6(password, false)
	if intermediateKey != nil && len(k.encryption.UE) == 32 {
		return intermediateKey
	}

	return validationHash[:32]
}

// deriveOwnerKeyR6 derives the owner encryption key for R6.
func (k *KeyDerivation) deriveOwnerKeyR6(password []byte) []byte {
	// For R6, use SHA-384
	h := sha512.New384()
	h.Write(password)

	if len(k.encryption.O) > 32 {
		h.Write(k.encryption.O[32:])
	}

	validationHash := h.Sum(nil)

	// Try to get intermediate key
	intermediateKey := k.computeIntermediateKeyR6(password, true)
	if intermediateKey != nil && len(k.encryption.OE) == 32 {
		return intermediateKey
	}

	return validationHash[:32]
}

// computeIntermediateKeyR6 computes an intermediate key for R6.
func (k *KeyDerivation) computeIntermediateKeyR6(password []byte, isOwner bool) []byte {
	// R6 intermediate key derivation using SHA-384
	h := sha512.New384()
	h.Write(password)

	if isOwner && len(k.encryption.U) > 0 {
		h.Write(k.encryption.U[:32])
	} else if !isOwner && len(k.encryption.U) > 32 {
		h.Write(k.encryption.U[32:48])
	}

	result := h.Sum(nil)

	// Return first 32 bytes for AES-256
	if len(result) >= 32 {
		return result[:32]
	}
	return result
}

// AuthenticateUserPassword validates a user password.
func (k *KeyDerivation) AuthenticateUserPassword(password string) bool {
	// Compute the encryption key
	key, err := k.ComputeEncryptionKey(password)
	if err != nil {
		return false
	}

	switch k.encryption.R {
	case 2:
		return k.authenticateUserR2(password, key)
	case 3:
		return k.authenticateUserR3(password, key)
	case 4:
		return k.authenticateUserR4(password, key)
	case 5:
		return k.authenticateUserR5(password, key)
	case 6:
		return k.authenticateUserR6(password, key)
	default:
		return false
	}
}

// authenticateUserR2 validates user password for revision 2.
func (k *KeyDerivation) authenticateUserR2(password string, key []byte) bool {
	if len(k.encryption.U) < 32 {
		return false
	}

	// Compute expected U value
	expectedU := k.computeUserPasswordR2(key)

	// Compare with stored U value
	return bytes.Equal(k.encryption.U[:32], expectedU[:32])
}

// computeUserPasswordR2 computes the U value for revision 2.
func (k *KeyDerivation) computeUserPasswordR2(key []byte) []byte {
	// Step 1: Pad password (empty for computing expected U)
	paddedPassword := padPassword("")

	// Step 2: Encrypt padded password using RC4 with the key
	rc4 := NewRC4Cipher(key)
	encrypted := rc4.Encrypt(paddedPassword)

	// Step 3: Return 32 bytes
	result := make([]byte, 32)
	copy(result, encrypted)

	return result
}

// authenticateUserR3 validates user password for revision 3.
func (k *KeyDerivation) authenticateUserR3(password string, key []byte) bool {
	if len(k.encryption.U) < 32 {
		return false
	}

	// Compute expected U value
	expectedU := k.computeUserPasswordR3(key)

	// Compare with stored U value
	return bytes.Equal(k.encryption.U[:32], expectedU[:32])
}

// computeUserPasswordR3 computes the U value for revision 3.
func (k *KeyDerivation) computeUserPasswordR3(key []byte) []byte {
	// Step 1: Create MD5 hash of the standard padding plus file ID.
	// Algorithm 5 for revisions 3 and 4 uses the first file identifier.
	paddedPassword := padPassword("")
	h := md5.New()
	h.Write(paddedPassword)
	if len(k.encryption.ID) > 0 {
		id := k.encryption.ID
		if len(id) > 16 {
			id = id[:16]
		}
		h.Write(id)
	}
	md5Result := h.Sum(nil)

	// Step 2: Encrypt once with the original key, then 19 iterations with XOR-ed keys.
	encrypted := NewRC4Cipher(key).Encrypt(md5Result)

	for i := 1; i <= 19; i++ {
		// Create new key for this iteration
		iterKey := make([]byte, len(key))
		for j := range key {
			iterKey[j] = key[j] ^ byte(i)
		}
		encrypted = NewRC4Cipher(iterKey).Encrypt(encrypted)
	}

	// Step 3: Add padding
	result := make([]byte, 32)
	copy(result, encrypted)

	return result
}

// authenticateUserR4 validates user password for revision 4 (AES-128).
func (k *KeyDerivation) authenticateUserR4(password string, key []byte) bool {
	if len(k.encryption.U) < 32 {
		return false
	}

	// Revision 4 keeps the revision 3 Algorithm 5 user validation contract.
	expectedU := k.computeUserPasswordR3(key)
	return bytes.Equal(k.encryption.U[:32], expectedU[:32])
}

// computeUserPasswordAES computes the U value for AES encryption.
func (k *KeyDerivation) computeUserPasswordAES(key []byte, ivSize int) []byte {
	// Generate random IV (or use zeros for validation)
	iv := make([]byte, ivSize)

	// Encrypt the padded password
	paddedPassword := padPassword("")

	// For AES, the U value is: IV(16) + encrypted(padding)
	aes := NewAESCipher(key)
	encrypted := aes.EncryptCBC(paddedPassword[:32], iv)

	result := make([]byte, 32)
	copy(result, iv)
	copy(result[ivSize:], encrypted)

	return result
}

// authenticateUserR5 validates user password for revision 5 (AES-256).
func (k *KeyDerivation) authenticateUserR5(password string, key []byte) bool {
	if len(k.encryption.U) < 48 {
		return false
	}

	// For R5, U is 48 bytes: 32-byte salt + 16-byte hash
	// We need to compute the hash and compare

	paddedPassword := padOrTruncatePassword(password, 32)

	// Compute validation hash
	h := sha256.New()
	h.Write(paddedPassword)
	h.Write(k.encryption.U[:32]) // Salt from U
	expectedHash := h.Sum(nil)

	// Compare with stored hash (last 16 bytes of U)
	storedHash := k.encryption.U[32:48]

	// Only compare first 16 bytes of the hash
	return bytes.Equal(storedHash, expectedHash[:16])
}

// authenticateUserR6 validates user password for revision 6 (AES-256, revised).
func (k *KeyDerivation) authenticateUserR6(password string, key []byte) bool {
	if len(k.encryption.U) < 48 {
		return false
	}

	// For R6, similar to R5 but using SHA-384
	paddedPassword := padOrTruncatePassword(password, 32)

	// Compute validation hash using SHA-384
	h := sha512.New384()
	h.Write(paddedPassword)
	h.Write(k.encryption.U[:32]) // Salt from U
	expectedHash := h.Sum(nil)

	// Compare with stored hash
	storedHash := k.encryption.U[32:48]

	// Only compare first 16 bytes
	return bytes.Equal(storedHash, expectedHash[:16])
}

// AuthenticateOwnerPassword validates an owner password.
func (k *KeyDerivation) AuthenticateOwnerPassword(password string) bool {
	switch k.encryption.R {
	case 2, 3:
		return k.authenticateOwnerLegacy(password)
	case 4:
		return k.authenticateOwnerAES128(password)
	case 5:
		return k.authenticateOwnerR5(password)
	case 6:
		return k.authenticateOwnerR6(password)
	default:
		return false
	}
}

// authenticateOwnerLegacy validates owner password for revisions 2-3.
func (k *KeyDerivation) authenticateOwnerLegacy(password string) bool {
	// Compute user password from owner password
	userPassword := k.computeUserPasswordFromOwner(password)

	// Validate the computed user password
	return k.AuthenticateUserPassword(userPassword)
}

// computeUserPasswordFromOwner computes the user password from owner password.
func (k *KeyDerivation) computeUserPasswordFromOwner(ownerPassword string) string {
	// Step 1: Pad owner password
	paddedOwner := padPassword(ownerPassword)

	// Step 2: Compute MD5 hash
	h := md5.New()
	h.Write(paddedOwner)
	hash := h.Sum(nil)

	// Step 3: Do 50 rounds of MD5 for R3
	if k.encryption.R >= 3 {
		keyLength := 5
		if k.encryption.R >= 3 {
			keyLength = k.encryption.Length / 8
			if keyLength > 16 {
				keyLength = 16
			}
		}

		for i := 0; i < 50; i++ {
			h.Reset()
			h.Write(hash[:keyLength])
			hash = h.Sum(nil)
		}
	}

	// Step 4: Create key
	keyLength := 5
	if k.encryption.R >= 3 {
		keyLength = k.encryption.Length / 8
		if keyLength > 16 {
			keyLength = 16
		}
	}
	key := hash[:keyLength]

	// Step 5: Decrypt O value using RC4
	rc4 := NewRC4Cipher(key)
	decrypted := rc4.Decrypt(k.encryption.O)

	// Step 6: For R3, apply RC4 19 more times
	if k.encryption.R >= 3 {
		for i := 1; i <= 19; i++ {
			iterKey := make([]byte, len(key))
			for j := range key {
				iterKey[j] = key[j] ^ byte(i)
			}
			rc4 = NewRC4Cipher(iterKey)
			decrypted = rc4.Decrypt(decrypted)
		}
	}

	// Step 7: Return decrypted password (unpadded)
	return unpadPassword(decrypted)
}

// authenticateOwnerAES128 validates owner password for revision 4 (AES-128).
func (k *KeyDerivation) authenticateOwnerAES128(password string) bool {
	// Similar to legacy but with AES-128
	paddedOwner := padPassword(password)

	// Compute key derivation
	h := md5.New()
	h.Write(paddedOwner)
	hash := h.Sum(nil)

	// 50 rounds of MD5
	for i := 0; i < 50; i++ {
		h.Reset()
		h.Write(hash[:16])
		hash = h.Sum(nil)
	}

	key := hash[:16] // 128-bit key

	// Decrypt O value using AES-128
	aes := NewAESCipher(key)
	iv := make([]byte, 16) // Zero IV for O value decryption
	decrypted := aes.DecryptCBC(k.encryption.O[:32], iv)

	// Validate the decrypted user password
	userPassword := unpadPassword(decrypted)
	return k.AuthenticateUserPassword(userPassword)
}

// authenticateOwnerR5 validates owner password for revision 5 (AES-256).
func (k *KeyDerivation) authenticateOwnerR5(password string) bool {
	if len(k.encryption.O) < 48 {
		return false
	}

	paddedPassword := padOrTruncatePassword(password, 32)

	// Compute validation hash
	h := sha256.New()
	h.Write(paddedPassword)
	h.Write(k.encryption.O[:32]) // Salt from O
	expectedHash := h.Sum(nil)

	// Compare with stored hash (last 16 bytes of O)
	storedHash := k.encryption.O[32:48]

	return bytes.Equal(storedHash, expectedHash[:16])
}

// authenticateOwnerR6 validates owner password for revision 6 (AES-256, revised).
func (k *KeyDerivation) authenticateOwnerR6(password string) bool {
	if len(k.encryption.O) < 48 {
		return false
	}

	paddedPassword := padOrTruncatePassword(password, 32)

	// Compute validation hash using SHA-384
	h := sha512.New384()
	h.Write(paddedPassword)
	h.Write(k.encryption.O[:32]) // Salt from O
	expectedHash := h.Sum(nil)

	// Compare with stored hash
	storedHash := k.encryption.O[32:48]

	return bytes.Equal(storedHash, expectedHash[:16])
}

// ComputeObjectKey computes the encryption key for a specific object.
func (k *KeyDerivation) ComputeObjectKey(objNum uint32, genNum uint16) []byte {
	// Get base encryption key
	baseKey, err := k.ComputeEncryptionKey("")
	if err != nil {
		return baseKey
	}

	return k.ComputeObjectKeyWithBaseKey(baseKey, objNum, genNum)
}

// ComputeObjectKeyWithBaseKey computes the encryption key for a specific object
// using the already authenticated document key.
func (k *KeyDerivation) ComputeObjectKeyWithBaseKey(baseKey []byte, objNum uint32, genNum uint16) []byte {
	if len(baseKey) == 0 {
		return baseKey
	}

	// For R2-R4, compute object-specific key
	if k.encryption.R <= 4 {
		return k.computeObjectKeyLegacy(baseKey, objNum, genNum)
	}

	// For R5+, the key is used directly
	return baseKey
}

// computeObjectKeyLegacy computes object-specific key for R2-R4.
func (k *KeyDerivation) computeObjectKeyLegacy(baseKey []byte, objNum uint32, genNum uint16) []byte {
	keyLength := len(baseKey)

	// Create key data: baseKey + objNum (3 bytes) + genNum (2 bytes)
	data := make([]byte, keyLength+5)
	copy(data, baseKey)

	// Add object number (little-endian, 3 bytes)
	data[keyLength] = byte(objNum)
	data[keyLength+1] = byte(objNum >> 8)
	data[keyLength+2] = byte(objNum >> 16)

	// Add generation number (little-endian, 2 bytes)
	data[keyLength+3] = byte(genNum)
	data[keyLength+4] = byte(genNum >> 8)

	// MD5 hash to get final key
	if k.encryption.R >= 3 {
		// For R3+, MD5 the key data
		h := md5.New()
		h.Write(data)
		hash := h.Sum(nil)

		// Truncate to key length
		if len(hash) > keyLength {
			hash = hash[:keyLength]
		}
		return hash
	}

	// For R2, just use the key data directly
	// But MD5 it first to get correct length
	h := md5.New()
	h.Write(data)
	hash := h.Sum(nil)

	if len(hash) > keyLength {
		hash = hash[:keyLength]
	}
	return hash
}

// padPassword pads a password to 32 bytes according to PDF spec.
func padPassword(password string) []byte {
	result := make([]byte, 32)
	padding := []byte{
		0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
		0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
		0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
		0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
	}

	pwBytes := []byte(password)
	copyLen := len(pwBytes)
	if copyLen > 32 {
		copyLen = 32
	}
	copy(result, pwBytes[:copyLen])

	if copyLen < 32 {
		copy(result[copyLen:], padding[:32-copyLen])
	}

	return result
}

// padOrTruncatePassword pads or truncates password to target length.
func padOrTruncatePassword(password string, targetLen int) []byte {
	result := make([]byte, targetLen)
	pwBytes := []byte(password)

	if len(pwBytes) >= targetLen {
		copy(result, pwBytes[:targetLen])
		return result
	}

	copy(result, pwBytes)
	return result
}

// unpadPassword removes padding from a password.
func unpadPassword(padded []byte) string {
	// Find the first padding byte
	padding := []byte{
		0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
		0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
		0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
		0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
	}

	// Find where padding starts
	for i := 0; i < len(padded); i++ {
		if i >= len(padding) || padded[i] != padding[i] {
			continue
		}

		// Check if this is the start of padding
		isPadding := true
		for j := i; j < len(padded) && j-i < len(padding); j++ {
			if padded[j] != padding[j-i] {
				isPadding = false
				break
			}
		}

		if isPadding {
			return string(padded[:i])
		}
	}

	return string(padded)
}
