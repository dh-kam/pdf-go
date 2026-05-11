package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func newAuthenticatedR3Handler(t *testing.T, password string) *EncryptionHandler {
	t.Helper()

	encryption := makeR3EncryptionForPassword()
	kd := NewKeyDerivation(encryption)
	key, err := kd.ComputeEncryptionKey(password)
	require.NoError(t, err)
	encryption.U = kd.computeUserPasswordR3(key)

	handler, err := NewEncryptionHandler(encryption, password)
	require.NoError(t, err)
	require.NotNil(t, handler)
	return handler
}

func TestEncryptionHandlerDecryptErrors(t *testing.T) {
	encryption := makeR3EncryptionForPassword()
	handler := &EncryptionHandler{
		encryption: encryption,
		keyDeriv:   NewKeyDerivation(encryption),
	}

	_, err := handler.Decrypt([]byte("abc"), 1, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")

	handler.authenticated = true
	_, err = handler.Decrypt([]byte("abc"), 1, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key not set")
}

func TestEncryptionHandlerDecryptStringAndStream(t *testing.T) {
	handler := newAuthenticatedR3Handler(t, "")
	plain := []byte("string and stream plaintext")

	// RC4 string path.
	encString := NewRC4Cipher(handler.key).Encrypt(plain)
	decString, err := handler.DecryptString(encString)
	require.NoError(t, err)
	assert.Equal(t, plain, decString)

	// RC4 stream path.
	objNum := uint32(7)
	gen := uint16(1)
	objKey := handler.keyDeriv.ComputeObjectKeyWithBaseKey(handler.key, objNum, gen)
	encStream := NewRC4Cipher(objKey).Encrypt(plain)
	decStream, err := handler.DecryptStream(encStream, objNum, gen)
	require.NoError(t, err)
	assert.Equal(t, plain, decStream)
}

func TestEncryptionHandlerAESPaths(t *testing.T) {
	handler := newAuthenticatedR3Handler(t, "")
	handler.encryption.V = AlgorithmVariable
	handler.encryption.R = 4
	handler.encryption.Length = 128

	plain := []byte("aes path plaintext")
	objNum := uint32(9)
	gen := uint16(0)
	objKey := handler.keyDeriv.ComputeObjectKeyWithBaseKey(handler.key, objNum, gen)

	// Decrypt (object) uses zero IV.
	encObj := AESEncryptCBC(objKey, make([]byte, 16), plain)
	decObj, err := handler.Decrypt(encObj, objNum, gen)
	require.NoError(t, err)
	assert.Equal(t, plain, decObj)

	// DecryptString uses ECB.
	encString := AESEncryptECB(handler.key, plain)
	decString, err := handler.DecryptString(encString)
	require.NoError(t, err)
	assert.Equal(t, plain, decString)

	// DecryptStream extracts IV from prefix.
	iv := []byte("1234567890abcdef")
	encBody := AESEncryptCBC(objKey, iv, plain)
	streamData := make([]byte, 0, len(iv)+len(encBody))
	streamData = append(streamData, iv...)
	streamData = append(streamData, encBody...)
	decStream, err := handler.DecryptStream(streamData, objNum, gen)
	require.NoError(t, err)
	assert.Equal(t, plain, decStream)

	// AES-256 branch.
	handler.encryption.V = AlgorithmAES256
	encObj256 := AESEncryptCBC(objKey, make([]byte, 16), plain)
	decObj256, err := handler.Decrypt(encObj256, objNum, gen)
	require.NoError(t, err)
	assert.Equal(t, plain, decObj256)
}

func TestEncryptionHandlerUnsupportedAlgorithms(t *testing.T) {
	handler := newAuthenticatedR3Handler(t, "")
	handler.encryption.V = 999

	_, err := handler.DecryptString([]byte("abc"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported encryption algorithm")

	_, err = handler.DecryptStream([]byte("abc"), 1, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported encryption algorithm")
}

func TestEncryptionHandlerPermissionAndMetadataHelpers(t *testing.T) {
	handler := newAuthenticatedR3Handler(t, "")
	handler.encryption.P = uint32(entity.PermCopy | entity.PermPrintHighRes | entity.PermAnnotate)
	handler.encryption.EncryptMetadata = true

	assert.True(t, handler.HasPermission(entity.PermCopy))
	assert.False(t, handler.HasPermission(entity.PermModify))
	assert.True(t, handler.CanPrint())
	assert.False(t, handler.CanModify())
	assert.True(t, handler.CanCopy())
	assert.True(t, handler.CanAnnotate())
	assert.False(t, handler.CanFillForms())
	assert.False(t, handler.CanExtract())
	assert.False(t, handler.CanAssemble())
	assert.True(t, handler.CanPrintHighRes())
	assert.True(t, handler.EncryptMetadata())

	handler.usedOwnerPassword = true
	assert.True(t, handler.HasPermission(entity.PermModify))
}

func TestEncryptionHandlerKeyLengthAndAccessors(t *testing.T) {
	handler := newAuthenticatedR3Handler(t, "")
	require.NotNil(t, handler.Encryption())
	assert.Equal(t, handler.encryption, handler.Encryption())
	assert.Equal(t, handler.encryption.V, handler.Algorithm())
	assert.Equal(t, handler.encryption.R, handler.Revision())
	assert.Equal(t, entity.PermissionFlags(handler.encryption.P), handler.Permissions())

	handler.encryption.Length = 96
	assert.Equal(t, 96, handler.KeyLength())

	handler.encryption.Length = 0
	handler.encryption.R = 2
	assert.Equal(t, 40, handler.KeyLength())
	handler.encryption.R = 3
	assert.Equal(t, 128, handler.KeyLength())
	handler.encryption.R = 5
	assert.Equal(t, 256, handler.KeyLength())
}

func TestParseEncryptionDictAndFactory(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("/Filter"), entity.Name("/Standard"))
	dict.Set(entity.Name("/SubFilter"), entity.Name("/adbe.pkcs7.s5"))
	dict.Set(entity.Name("/V"), entity.NewInteger(4))
	dict.Set(entity.Name("/Length"), entity.NewInteger(128))
	dict.Set(entity.Name("/R"), entity.NewInteger(4))
	dict.Set(entity.Name("/O"), entity.NewString("<01020A0B>"))
	dict.Set(entity.Name("/U"), entity.NewArray(entity.NewInteger(1), entity.NewInteger(2), entity.NewInteger(3)))
	dict.Set(entity.Name("/P"), entity.NewInteger(int64(entity.PermCopy|entity.PermPrint)))
	dict.Set(entity.Name("/EncryptMetadata"), entity.NewBoolean(true))
	dict.Set(entity.Name("/OE"), entity.NewString("<0A0B0C0D>"))
	dict.Set(entity.Name("/UE"), entity.NewString("<01020304>"))
	dict.Set(entity.Name("/Perms"), entity.NewString("<AABBCCDD>"))

	id := []byte("document-id")
	enc, err := ParseEncryptionDict(dict, id)
	require.NoError(t, err)
	require.NotNil(t, enc)
	assert.Equal(t, "/Standard", enc.Filter)
	assert.Equal(t, "/adbe.pkcs7.s5", enc.SubFilter)
	assert.Equal(t, 4, enc.V)
	assert.Equal(t, 128, enc.Length)
	assert.Equal(t, 4, enc.R)
	assert.Equal(t, []byte{0x01, 0x02, 0x0A, 0x0B}, enc.O)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, enc.U)
	assert.Equal(t, uint32(entity.PermCopy|entity.PermPrint), enc.P)
	assert.True(t, enc.EncryptMetadata)
	assert.Equal(t, id, enc.ID)
	assert.Equal(t, []byte{0x0A, 0x0B, 0x0C, 0x0D}, enc.OE)
	assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, enc.UE)
	assert.Equal(t, []byte{0xAA, 0xBB, 0xCC, 0xDD}, enc.Perms)

	// Defaults when fields are omitted.
	defaultDict := entity.NewDict()
	defaultDict.Set(entity.Name("/R"), entity.NewInteger(4))
	encDefault, err := ParseEncryptionDict(defaultDict, id)
	require.NoError(t, err)
	assert.Equal(t, "Standard", encDefault.Filter)
	assert.Equal(t, AlgorithmVariable, encDefault.V)
	assert.Equal(t, 128, encDefault.Length)

	_, err = ParseEncryptionDict(nil, id)
	require.Error(t, err)

	_, err = CreateEncryptionHandlerFromDict(nil, id, "")
	require.Error(t, err)
}

func TestParseEncryptionDict_UsesLiteralStringBytes(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("/R"), entity.NewInteger(3))
	dict.Set(entity.Name("/O"), entity.NewString(string([]byte{0x01, 0xAB, 0x02, 0xCD})))
	dict.Set(entity.Name("/U"), entity.NewString(string([]byte{0x10, 0x20, 0x30, 0x40})))

	enc, err := ParseEncryptionDict(dict, []byte("id"))
	require.NoError(t, err)
	require.NotNil(t, enc)
	assert.Equal(t, []byte{0x01, 0xAB, 0x02, 0xCD}, enc.O)
	assert.Equal(t, []byte{0x10, 0x20, 0x30, 0x40}, enc.U)
}

func TestDecodeHexHelpers(t *testing.T) {
	assert.Equal(t, []byte{0x01, 0xAB}, decodeHexString("<01ab>"))
	assert.Equal(t, []byte{0x0A}, decodeHexString("0a"))

	b, err := parseHexByte('A', 'f')
	require.NoError(t, err)
	assert.Equal(t, byte(0xAF), b)

	n, err := parseHexChar('9')
	require.NoError(t, err)
	assert.Equal(t, byte(9), n)

	_, err = parseHexChar('z')
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex character")
}
