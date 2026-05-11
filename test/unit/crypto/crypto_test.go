// Package crypto_test provides tests for PDF encryption/decryption.
package crypto_test

import (
	"crypto/md5"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/crypto"
)

func computeR3UserValidationValueLikeImplementation(key, fileID []byte) []byte {
	padding := []byte{
		0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
		0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
		0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
		0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
	}

	hash := md5.Sum(padding)
	if len(fileID) > 0 {
		h := md5.New()
		h.Write(padding)
		id := fileID
		if len(id) > 16 {
			id = id[:16]
		}
		h.Write(id)
		copy(hash[:], h.Sum(nil))
	}
	encrypted := crypto.NewRC4Cipher(key).Encrypt(hash[:])

	for i := 1; i <= 19; i++ {
		iterKey := make([]byte, len(key))
		for j := range key {
			iterKey[j] = key[j] ^ byte(i)
		}
		encrypted = crypto.NewRC4Cipher(iterKey).Encrypt(encrypted)
	}

	result := make([]byte, 32)
	copy(result, encrypted)
	return result
}

// TestRC4Encryption tests RC4 encryption and decryption.
func TestRC4Encryption(t *testing.T) {
	tests := []struct {
		name      string
		key       []byte
		plaintext []byte
	}{
		{
			name:      "Basic RC4",
			key:       []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			plaintext: []byte("Hello, World!"),
		},
		{
			name:      "16-byte key",
			key:       []byte("testkey12345678"),
			plaintext: []byte("This is a test message for RC4 encryption."),
		},
		{
			name:      "Empty data",
			key:       []byte{0x01, 0x02, 0x03},
			plaintext: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cipher := crypto.NewRC4Cipher(tt.key)

			// Encrypt
			encrypted := cipher.Encrypt(tt.plaintext)

			// Decrypt
			decrypted := cipher.Decrypt(encrypted)

			// Verify
			assert.Equal(t, tt.plaintext, decrypted)
		})
	}
}

// TestRC4EncryptDecrypt tests the helper functions.
func TestRC4EncryptDecrypt(t *testing.T) {
	key := []byte("testkey12345678")
	plaintext := []byte("Test data for RC4 encryption")

	encrypted := crypto.RC4Encrypt(key, plaintext)
	decrypted := crypto.RC4Decrypt(key, encrypted)

	assert.Equal(t, plaintext, decrypted)
}

// TestAESECBEncryption tests AES-ECB encryption and decryption.
func TestAESECBEncryption(t *testing.T) {
	tests := []struct {
		name      string
		key       []byte
		plaintext []byte
	}{
		{
			name:      "AES-128",
			key:       make([]byte, 16),
			plaintext: []byte("Hello, World! This is a test."),
		},
		{
			name:      "AES-256",
			key:       make([]byte, 32),
			plaintext: []byte("Test data for AES-256 encryption"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cipher := crypto.NewAESCipher(tt.key)

			// Encrypt
			encrypted := cipher.EncryptECB(tt.plaintext)

			// Decrypt
			decrypted := cipher.DecryptECB(encrypted)

			// Verify
			assert.Equal(t, tt.plaintext, decrypted)
		})
	}
}

// TestAESCBCEncryption tests AES-CBC encryption and decryption.
func TestAESCBCEncryption(t *testing.T) {
	tests := []struct {
		name      string
		key       []byte
		iv        []byte
		plaintext []byte
	}{
		{
			name:      "AES-128 CBC",
			key:       make([]byte, 16),
			iv:        make([]byte, 16),
			plaintext: []byte("Hello, World! This is a test for AES-CBC."),
		},
		{
			name:      "AES-256 CBC",
			key:       make([]byte, 32),
			iv:        make([]byte, 16),
			plaintext: []byte("Test data for AES-256 CBC encryption"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cipher := crypto.NewAESCipher(tt.key)

			// Encrypt
			encrypted := cipher.EncryptCBC(tt.plaintext, tt.iv)

			// Decrypt
			decrypted := cipher.DecryptCBC(encrypted, tt.iv)

			// Verify
			assert.Equal(t, tt.plaintext, decrypted)
		})
	}
}

// TestPasswordPadding tests password padding according to PDF spec.
func TestPasswordPadding(t *testing.T) {
	tests := []struct {
		name           string
		password       string
		expectedLength int
	}{
		{
			name:           "Short password",
			password:       "test",
			expectedLength: 32,
		},
		{
			name:           "Exact length",
			password:       "12345678901234567890123456789012",
			expectedLength: 32,
		},
		{
			name:           "Long password",
			password:       "this_is_a_very_long_password_that_exceeds_32_bytes",
			expectedLength: 32,
		},
		{
			name:           "Empty password",
			password:       "",
			expectedLength: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a basic test - the actual padding function is not exported
			// We just verify that the key derivation doesn't panic
			enc := &entity.Encryption{
				R:  2,
				O:  make([]byte, 32),
				U:  make([]byte, 32),
				P:  0xFFFFFFFF,
				ID: make([]byte, 16),
			}

			kd := crypto.NewKeyDerivation(enc)
			_, err := kd.ComputeEncryptionKey(tt.password)
			assert.NoError(t, err)
		})
	}
}

// TestKeyDerivationR2 tests key derivation for revision 2.
func TestKeyDerivationR2(t *testing.T) {
	enc := &entity.Encryption{
		R:      2,
		V:      0,
		Length: 40,
		O:      make([]byte, 32),
		U:      make([]byte, 32),
		P:      0xFFFFFFFF,
		ID:     make([]byte, 16),
	}

	// Set known O and U values for a test case
	// These are example values - in real testing, use known test vectors
	copy(enc.O, []byte{
		0x7E, 0x9A, 0x7C, 0x6A, 0x4F, 0x9E, 0x3D, 0x2B,
		0x8C, 0x1F, 0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C,
		0x1F, 0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C, 0x1F,
		0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C, 0x1F, 0x5E,
	})

	kd := crypto.NewKeyDerivation(enc)

	// Test with empty password (default user password)
	key, err := kd.ComputeEncryptionKey("")
	require.NoError(t, err)

	// Key should be 5 bytes (40 bits)
	assert.Equal(t, 5, len(key))
}

// TestKeyDerivationR3 tests key derivation for revision 3.
func TestKeyDerivationR3(t *testing.T) {
	enc := &entity.Encryption{
		R:      3,
		V:      2,
		Length: 128,
		O:      make([]byte, 32),
		U:      make([]byte, 32),
		P:      0xFFFFFFFF,
		ID:     make([]byte, 16),
	}

	kd := crypto.NewKeyDerivation(enc)

	// Test with empty password
	key, err := kd.ComputeEncryptionKey("")
	require.NoError(t, err)

	// Key should be 16 bytes (128 bits)
	assert.Equal(t, 16, len(key))
}

func TestAuthenticateUserPasswordR3UsesInitialRC4Round(t *testing.T) {
	enc := &entity.Encryption{
		R:      3,
		V:      2,
		Length: 128,
		O: []byte{
			0x7E, 0x9A, 0x7C, 0x6A, 0x4F, 0x9E, 0x3D, 0x2B,
			0x8C, 0x1F, 0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C,
			0x1F, 0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C, 0x1F,
			0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C, 0x1F, 0x5E,
		},
		U: make([]byte, 32),
		P: 0xFFFFFFFC,
		ID: []byte{
			0x01, 0x02, 0x03, 0x04, 0xAA, 0xBB, 0xCC, 0xDD,
			0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		},
	}

	kd := crypto.NewKeyDerivation(enc)
	key, err := kd.ComputeEncryptionKey("user-password")
	require.NoError(t, err)

	copy(enc.U, computeR3UserValidationValueLikeImplementation(key, enc.ID))

	assert.True(t, kd.AuthenticateUserPassword("user-password"))
	assert.False(t, kd.AuthenticateUserPassword("wrong-password"))
}

// TestKeyDerivationR4 tests key derivation for revision 4 (AES-128).
func TestKeyDerivationR4(t *testing.T) {
	enc := &entity.Encryption{
		R:      4,
		V:      4,
		Length: 128,
		O:      make([]byte, 32),
		U:      make([]byte, 32),
		P:      0xFFFFFFFF,
		ID:     make([]byte, 16),
	}

	kd := crypto.NewKeyDerivation(enc)

	// Test with empty password
	key, err := kd.ComputeEncryptionKey("")
	require.NoError(t, err)

	// Key should be 16 bytes (128 bits)
	assert.Equal(t, 16, len(key))
}

// TestKeyDerivationR5 tests key derivation for revision 5 (AES-256).
func TestKeyDerivationR5(t *testing.T) {
	enc := &entity.Encryption{
		R:      5,
		V:      5,
		Length: 256,
		O:      make([]byte, 48),
		U:      make([]byte, 48),
		P:      0xFFFFFFFF,
		ID:     make([]byte, 16),
		OE:     make([]byte, 32),
		UE:     make([]byte, 32),
		Perms:  make([]byte, 16),
	}

	kd := crypto.NewKeyDerivation(enc)

	// Test with a password
	key, err := kd.ComputeEncryptionKey("testpassword")
	require.NoError(t, err)

	// Key should be 32 bytes (256 bits)
	assert.Equal(t, 32, len(key))
}

// TestKeyDerivationR6 tests key derivation for revision 6 (AES-256 revised).
func TestKeyDerivationR6(t *testing.T) {
	enc := &entity.Encryption{
		R:      6,
		V:      5,
		Length: 256,
		O:      make([]byte, 48),
		U:      make([]byte, 48),
		P:      0xFFFFFFFF,
		ID:     make([]byte, 16),
		OE:     make([]byte, 32),
		UE:     make([]byte, 32),
		Perms:  make([]byte, 16),
	}

	kd := crypto.NewKeyDerivation(enc)

	// Test with a password
	key, err := kd.ComputeEncryptionKey("testpassword")
	require.NoError(t, err)

	// Key should be 32 bytes (256 bits)
	assert.Equal(t, 32, len(key))
}

// TestObjectKeyDerivation tests per-object key derivation.
func TestObjectKeyDerivation(t *testing.T) {
	baseKey := make([]byte, 16) // 128-bit key

	tests := []struct {
		name   string
		objNum uint32
		genNum uint16
	}{
		{"Object 1", 1, 0},
		{"Object 10", 10, 0},
		{"Object 100", 100, 0},
		{"Object with generation", 1, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objKey := crypto.ComputeObjectEncryptionKey(baseKey, tt.objNum, tt.genNum)

			// Object key should be same length as base key
			assert.Equal(t, len(baseKey), len(objKey))

			// Different objects should have different keys
			if tt.objNum > 1 || tt.genNum > 0 {
				otherKey := crypto.ComputeObjectEncryptionKey(baseKey, 1, 0)
				assert.NotEqual(t, objKey, otherKey)
			}
		})
	}
}

// TestEncryptString tests string encryption.
func TestEncryptString(t *testing.T) {
	key := make([]byte, 16)
	plaintext := []byte("Test string")

	tests := []struct {
		name      string
		algorithm int
	}{
		{"RC4", crypto.AlgorithmRC4_40},
		{"Variable", crypto.AlgorithmVariable},
		{"AES-256", crypto.AlgorithmAES256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted := crypto.EncryptString(key, plaintext, tt.algorithm)
			decrypted := crypto.DecryptString(key, encrypted, tt.algorithm)

			assert.Equal(t, plaintext, decrypted)
		})
	}
}

// TestEncryptStream tests stream encryption.
func TestEncryptStream(t *testing.T) {
	key := make([]byte, 16)
	plaintext := []byte("Test stream data with more content for encryption")

	tests := []struct {
		name      string
		algorithm int
	}{
		{"RC4", crypto.AlgorithmRC4_40},
		{"Variable", crypto.AlgorithmVariable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted := crypto.EncryptStream(key, plaintext, tt.algorithm)
			decrypted := crypto.DecryptStream(key, encrypted, tt.algorithm)

			assert.Equal(t, plaintext, decrypted)
		})
	}
}

// TestDecryptObject tests object decryption.
func TestDecryptObject(t *testing.T) {
	baseKey := make([]byte, 16)
	plaintext := []byte("Test object data")
	objNum := uint32(1)
	genNum := uint16(0)

	tests := []struct {
		name      string
		algorithm int
	}{
		{"RC4", crypto.AlgorithmRC4_40},
		{"Variable", crypto.AlgorithmVariable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt using EncryptString (which uses the base key directly)
			encrypted := crypto.EncryptString(baseKey, plaintext, tt.algorithm)

			// For object encryption, the key is modified with objNum/genNum
			// So we need to use DecryptString to reverse EncryptString
			// This test verifies string encryption/decryption symmetry
			decryptedStr := crypto.DecryptString(baseKey, encrypted, tt.algorithm)
			assert.Equal(t, plaintext, decryptedStr)

			// Test object decryption separately
			objDecrypted := crypto.DecryptObject(baseKey, encrypted, objNum, genNum, tt.algorithm)

			// Verify object decryption produces some result
			assert.NotNil(t, objDecrypted)
		})
	}
}

// TestEncryptObject is a helper that wraps object encryption.
func TestEncryptObject(t *testing.T) {
	baseKey := make([]byte, 16)
	plaintext := []byte("Test object data")
	objNum := uint32(1)
	genNum := uint16(0)

	// Use DecryptObject to encrypt (they use the same algorithm)
	encrypted := crypto.DecryptObject(baseKey, plaintext, objNum, genNum, crypto.AlgorithmRC4_40)

	// Verify it's different from plaintext
	assert.NotEqual(t, plaintext, encrypted)

	// Verify we can decrypt it back
	decrypted := crypto.DecryptObject(baseKey, encrypted, objNum, genNum, crypto.AlgorithmRC4_40)
	assert.Equal(t, plaintext, decrypted)
}

// TestPermissionFlags tests permission flag operations.
func TestPermissionFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    entity.PermissionFlags
		check    entity.PermissionFlags
		expected bool
	}{
		{
			name:     "Print permission",
			flags:    entity.PermPrint,
			check:    entity.PermPrint,
			expected: true,
		},
		{
			name:     "No permission",
			flags:    0,
			check:    entity.PermPrint,
			expected: false,
		},
		{
			name:     "All permissions",
			flags:    entity.PermPrint | entity.PermModify | entity.PermCopy,
			check:    entity.PermModify,
			expected: true,
		},
		{
			name:     "Partial permissions",
			flags:    entity.PermPrint | entity.PermCopy,
			check:    entity.PermModify,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.flags.HasPermission(tt.check)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseEncryptionDict tests parsing encryption dictionaries.
func TestParseEncryptionDict(t *testing.T) {
	dict := entity.NewDict()

	// Set basic encryption values
	dict.Set(entity.Name("/Filter"), entity.Name("Standard"))
	dict.Set(entity.Name("/V"), entity.NewInteger(2))
	dict.Set(entity.Name("/Length"), entity.NewInteger(128))
	dict.Set(entity.Name("/R"), entity.NewInteger(3))
	dict.Set(entity.Name("/P"), entity.NewInteger(0xFFFFFFFF))

	// Set O and U as strings
	dict.Set(entity.Name("/O"), entity.NewString("0000000000000000000000000000000000000000000000000000000000000000"))
	dict.Set(entity.Name("/U"), entity.NewString("0000000000000000000000000000000000000000000000000000000000000000"))

	id := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	enc, err := crypto.ParseEncryptionDict(dict, id)
	require.NoError(t, err)

	assert.Equal(t, "Standard", enc.Filter)
	assert.Equal(t, 2, enc.V)
	assert.Equal(t, 128, enc.Length)
	assert.Equal(t, 3, enc.R)
	assert.Equal(t, uint32(0xFFFFFFFF), enc.P)
}

// TestEncryptionHandler tests the encryption handler.
func TestEncryptionHandler(t *testing.T) {
	enc := &entity.Encryption{
		Filter: "Standard",
		R:      3,
		V:      2,
		Length: 128,
		O: []byte{
			0x7E, 0x9A, 0x7C, 0x6A, 0x4F, 0x9E, 0x3D, 0x2B,
			0x8C, 0x1F, 0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C,
			0x1F, 0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C, 0x1F,
			0x5E, 0x7A, 0x9B, 0x3D, 0x2F, 0x8C, 0x1F, 0x5E,
		},
		U: make([]byte, 32),
		P: 0xFFFFFFFC,
		ID: []byte{
			0x01, 0x02, 0x03, 0x04, 0xAA, 0xBB, 0xCC, 0xDD,
			0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		},
	}

	kd := crypto.NewKeyDerivation(enc)
	key, err := kd.ComputeEncryptionKey("user-password")
	require.NoError(t, err)
	copy(enc.U, computeR3UserValidationValueLikeImplementation(key, enc.ID))

	// Create encryption handler with valid user password.
	handler, err := crypto.NewEncryptionHandler(enc, "user-password")
	require.NoError(t, err)

	assert.True(t, handler.IsAuthenticated())
	assert.NotNil(t, handler.Key())
	assert.Equal(t, 3, handler.Revision())
}

// TestString tests String() method for PermissionFlags.
func TestPermissionFlagsString(t *testing.T) {
	tests := []struct {
		name     string
		contains []string
		flags    entity.PermissionFlags
	}{
		{
			name:     "Print only",
			flags:    entity.PermPrint,
			contains: []string{"Print"},
		},
		{
			name:     "Multiple permissions",
			flags:    entity.PermPrint | entity.PermModify | entity.PermCopy,
			contains: []string{"Print", "Modify", "Copy"},
		},
		{
			name:     "No permissions",
			flags:    0,
			contains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.flags.String()
			for _, substr := range tt.contains {
				assert.Contains(t, str, substr)
			}
		})
	}
}

// BenchmarkRC4Encryption benchmarks RC4 encryption.
func BenchmarkRC4Encryption(b *testing.B) {
	key := make([]byte, 16)
	data := make([]byte, 1024)
	cipher := crypto.NewRC4Cipher(key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cipher.Encrypt(data)
	}
}

// BenchmarkAESCBCEncryption benchmarks AES-CBC encryption.
func BenchmarkAESCBCEncryption(b *testing.B) {
	key := make([]byte, 16)
	iv := make([]byte, 16)
	data := make([]byte, 1024)
	cipher := crypto.NewAESCipher(key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cipher.EncryptCBC(data, iv)
	}
}

// TestEncryptObject is an alias for DecryptObject (symmetric).
var EncryptObject = crypto.DecryptObject
