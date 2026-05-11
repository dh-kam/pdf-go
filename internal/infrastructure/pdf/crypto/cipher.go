// Package crypto implements PDF encryption ciphers (RC4 and AES).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"io"
)

// RC4Cipher implements RC4 encryption/decryption as used in PDF.
type RC4Cipher struct {
	key []byte
}

// NewRC4Cipher creates a new RC4 cipher with the given key.
func NewRC4Cipher(key []byte) *RC4Cipher {
	return &RC4Cipher{
		key: makeCopy(key),
	}
}

// Encrypt encrypts data using RC4.
func (r *RC4Cipher) Encrypt(data []byte) []byte {
	return r.crypt(data)
}

// Decrypt decrypts data using RC4.
// RC4 is a symmetric cipher, so encryption and decryption are the same.
func (r *RC4Cipher) Decrypt(data []byte) []byte {
	return r.crypt(data)
}

// crypt performs RC4 encryption/decryption.
func (r *RC4Cipher) crypt(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	// Initialize RC4 state
	state := make([]byte, 256)
	for i := range state {
		state[i] = byte(i)
	}

	// Key scheduling algorithm (KSA)
	var j byte
	for i := range state {
		j = (j + state[i] + r.key[i%len(r.key)]) & 0xFF
		state[i], state[j] = state[j], state[i]
	}

	// Pseudo-random generation algorithm (PRGA)
	result := make([]byte, len(data))
	var i byte
	var j2 byte
	for n := range data {
		i = (i + 1) & 0xFF
		j2 = (j2 + state[i]) & 0xFF
		state[i], state[j2] = state[j2], state[i]
		k := state[(state[i]+state[j2])&0xFF]
		result[n] = data[n] ^ k
	}

	return result
}

// RC4Encrypt performs RC4 encryption with a key.
func RC4Encrypt(key, data []byte) []byte {
	ciph := NewRC4Cipher(key)
	return ciph.Encrypt(data)
}

// RC4Decrypt performs RC4 decryption with a key.
func RC4Decrypt(key, data []byte) []byte {
	ciph := NewRC4Cipher(key)
	return ciph.Decrypt(data)
}

// AESCipher implements AES encryption/decryption for PDF.
type AESCipher struct {
	key     []byte
	keySize int // 16 for AES-128, 32 for AES-256
}

// NewAESCipher creates a new AES cipher with the given key.
func NewAESCipher(key []byte) *AESCipher {
	keySize := len(key)
	if keySize != 16 && keySize != 32 {
		// Default to 16 bytes (128-bit)
		keySize = 16
	}

	return &AESCipher{
		key:     makeCopy(key),
		keySize: keySize,
	}
}

// EncryptECB encrypts data using AES in ECB mode.
// Note: ECB is not recommended for general use but is used in PDF.
func (a *AESCipher) EncryptECB(data []byte) []byte {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return data
	}

	// Pad data to block size
	padded := pkcs7Pad(data, aes.BlockSize)

	// Encrypt each block
	result := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(result[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}

	return result
}

// DecryptECB decrypts data using AES in ECB mode.
func (a *AESCipher) DecryptECB(data []byte) []byte {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return data
	}

	if len(data)%aes.BlockSize != 0 {
		// Invalid ciphertext length
		return data
	}

	// Decrypt each block
	result := make([]byte, len(data))
	for i := 0; i < len(data); i += aes.BlockSize {
		block.Decrypt(result[i:i+aes.BlockSize], data[i:i+aes.BlockSize])
	}

	// Remove padding
	unpadded, err := pkcs7Unpad(result, aes.BlockSize)
	if err != nil {
		return result
	}

	return unpadded
}

// EncryptCBC encrypts data using AES in CBC mode with the given IV.
func (a *AESCipher) EncryptCBC(data, iv []byte) []byte {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return data
	}

	// Pad data to block size
	padded := pkcs7Pad(data, aes.BlockSize)

	// Create CBC mode
	mode := cipher.NewCBCEncrypter(block, iv)

	// Encrypt
	result := make([]byte, len(padded))
	mode.CryptBlocks(result, padded)

	return result
}

// DecryptCBC decrypts data using AES in CBC mode with the given IV.
func (a *AESCipher) DecryptCBC(data, iv []byte) []byte {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return data
	}

	if len(data)%aes.BlockSize != 0 {
		// Invalid ciphertext length
		return data
	}

	// Create CBC mode
	mode := cipher.NewCBCDecrypter(block, iv)

	// Decrypt
	result := make([]byte, len(data))
	mode.CryptBlocks(result, data)

	// Remove padding
	unpadded, err := pkcs7Unpad(result, aes.BlockSize)
	if err != nil {
		return result
	}

	return unpadded
}

// EncryptCTR encrypts data using AES in CTR mode with the given IV.
func (a *AESCipher) EncryptCTR(data, iv []byte) []byte {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return data
	}

	// Create CTR mode
	stream := cipher.NewCTR(block, iv)

	// Encrypt (CTR mode uses XOR, so encrypt and decrypt are the same)
	result := make([]byte, len(data))
	stream.XORKeyStream(result, data)

	return result
}

// DecryptCTR decrypts data using AES in CTR mode with the given IV.
func (a *AESCipher) DecryptCTR(data, iv []byte) []byte {
	// CTR mode is symmetric
	return a.EncryptCTR(data, iv)
}

// AESEncryptECB performs AES-ECB encryption with a key.
func AESEncryptECB(key, data []byte) []byte {
	ciph := NewAESCipher(key)
	return ciph.EncryptECB(data)
}

// AESDecryptECB performs AES-ECB decryption with a key.
func AESDecryptECB(key, data []byte) []byte {
	ciph := NewAESCipher(key)
	return ciph.DecryptECB(data)
}

// AESEncryptCBC performs AES-CBC encryption with a key and IV.
func AESEncryptCBC(key, iv, data []byte) []byte {
	ciph := NewAESCipher(key)
	return ciph.EncryptCBC(data, iv)
}

// AESDecryptCBC performs AES-CBC decryption with a key and IV.
func AESDecryptCBC(key, iv, data []byte) []byte {
	ciph := NewAESCipher(key)
	return ciph.DecryptCBC(data, iv)
}

// pkcs7Pad applies PKCS#7 padding to data.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padded := make([]byte, len(data)+padding)
	copy(padded, data)

	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}

	return padded
}

// pkcs7Unpad removes PKCS#7 padding from data.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidPadding
	}

	padding := int(data[len(data)-1])

	if padding < 1 || padding > blockSize {
		return nil, ErrInvalidPadding
	}

	if padding > len(data) {
		return nil, ErrInvalidPadding
	}

	// Verify padding
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, ErrInvalidPadding
		}
	}

	return data[:len(data)-padding], nil
}

// EncryptString encrypts a PDF string using the appropriate cipher.
func EncryptString(key, data []byte, algorithm int) []byte {
	if len(data) == 0 {
		return data
	}

	switch algorithm {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated, AlgorithmVariable:
		// Use RC4
		return RC4Encrypt(key, data)
	case AlgorithmAES256, AlgorithmAES256Rev:
		// Use AES-256 in ECB mode for strings
		return AESEncryptECB(key, data)
	default:
		// Default to RC4
		return RC4Encrypt(key, data)
	}
}

// DecryptString decrypts a PDF string using the appropriate cipher.
func DecryptString(key, data []byte, algorithm int) []byte {
	if len(data) == 0 {
		return data
	}

	switch algorithm {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated, AlgorithmVariable:
		// Use RC4
		return RC4Decrypt(key, data)
	case AlgorithmAES256, AlgorithmAES256Rev:
		// Use AES-256 in ECB mode for strings
		return AESDecryptECB(key, data)
	default:
		// Default to RC4
		return RC4Decrypt(key, data)
	}
}

// EncryptStream encrypts a PDF stream using the appropriate cipher.
func EncryptStream(key, data []byte, algorithm int) []byte {
	if len(data) == 0 {
		return data
	}

	switch algorithm {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated, AlgorithmVariable:
		// Use RC4
		return RC4Encrypt(key, data)
	case AlgorithmAES256, AlgorithmAES256Rev:
		// Use AES in CBC mode for streams
		// Generate or extract IV from the beginning of data
		iv := make([]byte, aes.BlockSize)
		if len(data) > aes.BlockSize {
			// For streams, IV might be prepended
			copy(iv, data[:aes.BlockSize])
			encrypted := AESEncryptCBC(key, iv, data[aes.BlockSize:])
			// Prepend IV to result
			result := make([]byte, len(iv)+len(encrypted))
			copy(result, iv)
			copy(result[len(iv):], encrypted)
			return result
		}
		// No IV, generate one
		if _, err := io.ReadFull(rand.Reader, iv); err == nil {
			encrypted := AESEncryptCBC(key, iv, data)
			result := make([]byte, len(iv)+len(encrypted))
			copy(result, iv)
			copy(result[len(iv):], encrypted)
			return result
		}
		// Fallback to ECB
		return AESEncryptECB(key, data)
	default:
		// Default to RC4
		return RC4Encrypt(key, data)
	}
}

// DecryptStream decrypts a PDF stream using the appropriate cipher.
func DecryptStream(key, data []byte, algorithm int) []byte {
	if len(data) == 0 {
		return data
	}

	switch algorithm {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated, AlgorithmVariable:
		// Use RC4
		return RC4Decrypt(key, data)
	case AlgorithmAES256, AlgorithmAES256Rev:
		// Use AES in CBC mode for streams
		// Extract IV from the beginning of data
		if len(data) > aes.BlockSize {
			iv := make([]byte, aes.BlockSize)
			copy(iv, data[:aes.BlockSize])
			return AESDecryptCBC(key, iv, data[aes.BlockSize:])
		}
		// No IV, try ECB
		return AESDecryptECB(key, data)
	default:
		// Default to RC4
		return RC4Decrypt(key, data)
	}
}

// ComputeObjectEncryptionKey computes the encryption key for a specific object.
func ComputeObjectEncryptionKey(baseKey []byte, objNum uint32, genNum uint16) []byte {
	// For PDF, the object encryption key is computed as:
	// MD5(baseKey + objNum (3 bytes LE) + genNum (2 bytes LE))

	data := make([]byte, len(baseKey)+5)
	copy(data, baseKey)

	// Add object number (little-endian, 3 bytes)
	binary.LittleEndian.PutUint32(data[len(baseKey):], objNum)
	// Add generation number (little-endian, 2 bytes)
	binary.LittleEndian.PutUint16(data[len(baseKey)+3:], genNum)

	// Compute MD5
	h := md5.New()
	h.Write(data)
	hash := h.Sum(nil)

	// Truncate to base key length
	if len(hash) > len(baseKey) {
		hash = hash[:len(baseKey)]
	}

	return hash
}

// DecryptObject decrypts an encrypted PDF object.
func DecryptObject(baseKey, data []byte, objNum uint32, genNum uint16, algorithm int) []byte {
	// Compute object-specific key
	objKey := ComputeObjectEncryptionKey(baseKey, objNum, genNum)

	// Decrypt based on algorithm
	switch algorithm {
	case AlgorithmRC4_40, AlgorithmRC4_40Deprecated, AlgorithmVariable:
		// For RC4, need to apply multiple iterations for revision >= 3
		if len(data) == 0 {
			return data
		}

		// Apply RC4 decryption
		result := RC4Decrypt(objKey, data)

		return result
	case AlgorithmAES256, AlgorithmAES256Rev:
		// For AES-256, streams use CBC, strings use ECB
		// Determine based on context - assume stream for objects
		return DecryptStream(objKey, data, algorithm)
	default:
		// Default to RC4
		return RC4Decrypt(objKey, data)
	}
}

// DecryptMetadata decrypts metadata if it's not encrypted.
func DecryptMetadata(data []byte) []byte {
	// If EncryptMetadata is false, metadata is stored as-is
	return data
}

// Algorithm constants.
const (
	// AlgorithmRC4_40 is RC4 with 40-bit key.
	AlgorithmRC4_40 = 0

	// AlgorithmRC4_40Deprecated is deprecated RC4 with 40-bit key.
	AlgorithmRC4_40Deprecated = 1

	// AlgorithmVariable is RC4 or AES with variable key length.
	AlgorithmVariable = 2

	// AlgorithmAES256 is AES-256 (R5).
	AlgorithmAES256 = 4

	// AlgorithmAES256Rev is AES-256 revised (R6).
	AlgorithmAES256Rev = 5
)

// Error definitions.
var (
	// ErrInvalidPadding is returned when PKCS#7 padding is invalid.
	ErrInvalidPadding = &CipherError{"invalid PKCS#7 padding"}

	// ErrInvalidKeyLength is returned when the key length is invalid.
	ErrInvalidKeyLength = &CipherError{"invalid key length"}

	// ErrInvalidBlockSize is returned when the block size is invalid.
	ErrInvalidBlockSize = &CipherError{"invalid block size"}
)

// CipherError represents a cipher operation error.
type CipherError struct {
	Message string
}

// Error returns the error message.
func (e *CipherError) Error() string {
	return e.Message
}

// makeCopy creates a copy of a byte slice.
func makeCopy(data []byte) []byte {
	if data == nil {
		return nil
	}
	result := make([]byte, len(data))
	copy(result, data)
	return result
}
