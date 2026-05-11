package cmap

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

func TestParseStringPredefined(t *testing.T) {
	cm, err := ParseString(rksjH)
	require.NoError(t, err)

	require.NotNil(t, cm)
	assert.Equal(t, "RKSJ-H", cm.Name())
	assert.True(t, cm.IsCIDBased())
	assert.False(t, cm.IsUnicode())

	cid, ok := cm.LookupCID(1)
	assert.True(t, ok)
	assert.Equal(t, uint32(2), cid)

	base, ok := cm.(*BaseCMap)
	require.True(t, ok)

	base.SetUnicodeMapping(0x20, "!")
	uni, ok := cm.LookupUnicode(0x20)
	assert.True(t, ok)
	assert.Equal(t, "!", uni)
}

func TestParseBytesEmptyInput(t *testing.T) {
	cm, err := ParseBytes([]byte{})
	require.NoError(t, err)
	assert.Equal(t, "Unknown", cm.Name())
	assert.False(t, cm.IsCIDBased())
}

func TestPredefinedCMapNotFound(t *testing.T) {
	cm, err := PredefinedCMap("Missing-CMap")
	assert.Nil(t, cm)
	assert.Error(t, err)
	assert.IsType(t, &errors.PDFError{}, err)
}

func TestBinaryParserInvalidData(t *testing.T) {
	invalid := make([]byte, 4)
	binary.BigEndian.PutUint32(invalid, 0x00000000)

	p := NewBinaryParser(invalid)
	_, err := p.Parse()
	assert.Error(t, err)
}
