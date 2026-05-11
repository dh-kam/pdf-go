package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func makeR3EncryptionForPassword() *entity.Encryption {
	return &entity.Encryption{
		R:      3,
		V:      AlgorithmRC4_40,
		Length: 128,
		O:      []byte("owner-key-material-abcdefghijklmnop"),
		U:      make([]byte, 32),
		P:      0xFFFFFF00,
		ID:     make([]byte, 16),
	}
}

func TestNewEncryptionHandlerNilMetadata(t *testing.T) {
	_, err := NewEncryptionHandler(nil, "")
	assert.Error(t, err)
}

func TestNewEncryptionHandlerDefaultUserPassword(t *testing.T) {
	encryption := makeR3EncryptionForPassword()
	kd := NewKeyDerivation(encryption)
	key, err := kd.ComputeEncryptionKey("")
	require.NoError(t, err)

	encryption.U = kd.computeUserPasswordR3(key)
	handler, err := NewEncryptionHandler(encryption, "")
	require.NoError(t, err)
	require.NotNil(t, handler)
	require.True(t, handler.IsAuthenticated())
	assert.NotEmpty(t, handler.Key())
}

func TestNewEncryptionHandlerWrongPassword(t *testing.T) {
	encryption := makeR3EncryptionForPassword()
	kd := NewKeyDerivation(encryption)
	key, err := kd.ComputeEncryptionKey("")
	require.NoError(t, err)

	encryption.U = kd.computeUserPasswordR3(key)

	_, err = NewEncryptionHandler(encryption, "wrong-password")
	assert.ErrorIs(t, err, entity.ErrInvalidPassword)
}

func TestEncryptionHandlerAuthenticateState(t *testing.T) {
	encryption := makeR3EncryptionForPassword()
	kd := NewKeyDerivation(encryption)
	key, err := kd.ComputeEncryptionKey("")
	require.NoError(t, err)

	encryption.U = kd.computeUserPasswordR3(key)

	handler, err := NewEncryptionHandler(encryption, "")
	require.NoError(t, err)
	require.True(t, handler.IsAuthenticated())
	assert.True(t, handler.Authenticate(""))
	assert.False(t, handler.Authenticate("wrong-password"))
}

func TestEncryptionHandlerDecryptRoundtrip(t *testing.T) {
	password := "secure-password"
	encryption := makeR3EncryptionForPassword()
	kd := NewKeyDerivation(encryption)
	key, err := kd.ComputeEncryptionKey(password)
	require.NoError(t, err)

	encryption.U = kd.computeUserPasswordR3(key)

	handler, err := NewEncryptionHandler(encryption, password)
	require.NoError(t, err)

	plaintext := []byte("renderer roundtrip test data")
	objKey := handler.keyDeriv.ComputeObjectKeyWithBaseKey(handler.key, 1, 0)
	encrypted := NewRC4Cipher(objKey).Encrypt(plaintext)

	decrypted, err := handler.Decrypt(encrypted, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptionHandlerUnsupportedAlgorithm(t *testing.T) {
	password := ""
	encryption := makeR3EncryptionForPassword()
	kd := NewKeyDerivation(encryption)
	key, err := kd.ComputeEncryptionKey(password)
	require.NoError(t, err)

	encryption.U = kd.computeUserPasswordR3(key)
	encryption.V = 999

	handler, err := NewEncryptionHandler(encryption, "")
	require.NoError(t, err)

	_, err = handler.Decrypt([]byte("abc"), 1, 0)
	assert.Error(t, err)
}

func TestEncryptionStreamHelpers(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	data := []byte("plain-stream-data")

	streamData := append(make([]byte, 16), data...)
	copy(streamData[:16], []byte("stream-iv-16-byte"))
	enc := EncryptStream(key, streamData, AlgorithmAES256Rev)
	dec := DecryptStream(key, enc, AlgorithmAES256Rev)
	assert.NotNil(t, dec)
	assert.Equal(t, data, dec)

	plain := AESEncryptECB(key, data)
	dec = AESDecryptECB(key, plain)
	assert.Equal(t, data, dec)

	baseKey := make([]byte, 16)
	copy(baseKey, "base-key-16-bytes")
	objNum := uint32(42)
	genNum := uint16(2)
	objData := []byte("object")
	objKey := ComputeObjectEncryptionKey(baseKey, objNum, genNum)
	encryptedObject := NewRC4Cipher(objKey).Encrypt(objData)
	decryptedObject := DecryptObject(baseKey, encryptedObject, objNum, genNum, AlgorithmRC4_40)
	assert.Equal(t, objData, decryptedObject)
}
