// Package entity tests for Encryption functionality.
package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPermissionFlags tests the PermissionFlags type and constants.
func TestPermissionFlags(t *testing.T) {
	t.Run("Permission flag constants have correct values", func(t *testing.T) {
		assert.Equal(t, PermissionFlags(1<<2), PermPrint)
		assert.Equal(t, PermissionFlags(1<<3), PermModify)
		assert.Equal(t, PermissionFlags(1<<4), PermCopy)
		assert.Equal(t, PermissionFlags(1<<5), PermAnnotate)
		assert.Equal(t, PermissionFlags(1<<8), PermFillForms)
		assert.Equal(t, PermissionFlags(1<<9), PermExtract)
		assert.Equal(t, PermissionFlags(1<<11), PermAssemble)
		assert.Equal(t, PermissionFlags(1<<12), PermPrintHighRes)
		assert.Equal(t, PermissionFlags(1<<31), PermOwner)
	})

	t.Run("Permission flags are distinct", func(t *testing.T) {
		flags := []PermissionFlags{
			PermPrint, PermModify, PermCopy, PermAnnotate,
			PermFillForms, PermExtract, PermAssemble, PermPrintHighRes, PermOwner,
		}
		for i, f1 := range flags {
			for j, f2 := range flags {
				if i != j {
					assert.NotEqual(t, f1, f2)
				}
			}
		}
	})
}

// TestPermissionFlags_HasPermission tests the HasPermission method.
func TestPermissionFlags_HasPermission(t *testing.T) {
	t.Run("HasPermission returns true for granted permissions", func(t *testing.T) {
		flags := PermPrint | PermCopy | PermModify

		assert.True(t, flags.HasPermission(PermPrint))
		assert.True(t, flags.HasPermission(PermCopy))
		assert.True(t, flags.HasPermission(PermModify))
	})

	t.Run("HasPermission returns false for denied permissions", func(t *testing.T) {
		flags := PermPrint | PermCopy

		assert.False(t, flags.HasPermission(PermModify))
		assert.False(t, flags.HasPermission(PermAnnotate))
		assert.False(t, flags.HasPermission(PermFillForms))
	})

	t.Run("HasPermission with zero flags", func(t *testing.T) {
		var flags PermissionFlags
		assert.False(t, flags.HasPermission(PermPrint))
		assert.False(t, flags.HasPermission(PermModify))
	})

	t.Run("HasPermission with all flags", func(t *testing.T) {
		flags := PermissionFlags(0xFFFFFFFF)
		assert.True(t, flags.HasPermission(PermPrint))
		assert.True(t, flags.HasPermission(PermModify))
		assert.True(t, flags.HasPermission(PermCopy))
		assert.True(t, flags.HasPermission(PermAnnotate))
		assert.True(t, flags.HasPermission(PermFillForms))
		assert.True(t, flags.HasPermission(PermExtract))
		assert.True(t, flags.HasPermission(PermAssemble))
		assert.True(t, flags.HasPermission(PermPrintHighRes))
		assert.True(t, flags.HasPermission(PermOwner))
	})

	t.Run("HasPermission with combined flag", func(t *testing.T) {
		flags := PermPrint | PermModify
		// Check a combination that includes both flags
		combinedFlag := PermPrint | PermModify
		assert.True(t, flags.HasPermission(combinedFlag))
	})
}

// TestPermissionFlags_String tests the String method.
func TestPermissionFlags_String(t *testing.T) {
	t.Run("String returns empty string for no permissions", func(t *testing.T) {
		var flags PermissionFlags
		assert.Empty(t, flags.String())
	})

	t.Run("String returns single permission name", func(t *testing.T) {
		flags := PermPrint
		assert.Equal(t, "Print ", flags.String())
	})

	t.Run("String returns multiple permission names", func(t *testing.T) {
		flags := PermPrint | PermCopy | PermModify
		result := flags.String()
		assert.Contains(t, result, "Print ")
		assert.Contains(t, result, "Copy ")
		assert.Contains(t, result, "Modify ")
	})

	t.Run("String returns all permissions", func(t *testing.T) {
		flags := PermissionFlags(0xFFFFFFFF)
		result := flags.String()
		assert.Contains(t, result, "Print ")
		assert.Contains(t, result, "Modify ")
		assert.Contains(t, result, "Copy ")
		assert.Contains(t, result, "Annotate ")
		assert.Contains(t, result, "FillForms ")
		assert.Contains(t, result, "Extract ")
		assert.Contains(t, result, "Assemble ")
		assert.Contains(t, result, "PrintHighRes ")
		assert.Contains(t, result, "Owner ")
	})

	t.Run("String includes only granted permissions", func(t *testing.T) {
		flags := PermPrint | PermAssemble
		result := flags.String()
		assert.Contains(t, result, "Print ")
		assert.Contains(t, result, "Assemble ")
		assert.NotContains(t, result, "Modify ")
		assert.NotContains(t, result, "Copy ")
	})
}

// TestEncryptionStruct tests the Encryption struct.
func TestEncryptionStruct(t *testing.T) {
	t.Run("Encryption with default values", func(t *testing.T) {
		enc := &Encryption{}
		assert.Empty(t, enc.Filter)
		assert.Empty(t, enc.SubFilter)
		assert.Equal(t, 0, enc.V)
		assert.Equal(t, 0, enc.Length)
		assert.Equal(t, 0, enc.R)
		assert.Nil(t, enc.O)
		assert.Nil(t, enc.U)
		assert.Equal(t, uint32(0), enc.P)
		assert.False(t, enc.EncryptMetadata)
		assert.Nil(t, enc.ID)
		assert.Nil(t, enc.OE)
		assert.Nil(t, enc.UE)
		assert.Nil(t, enc.Perms)
	})

	t.Run("Encryption with values", func(t *testing.T) {
		enc := &Encryption{
			Filter:          "Standard",
			SubFilter:       "AESV2",
			V:               4,
			Length:          128,
			R:               4,
			O:               []byte{1, 2, 3, 4},
			U:               []byte{5, 6, 7, 8},
			P:               uint32(PermPrint | PermCopy),
			EncryptMetadata: true,
			ID:              []byte{9, 10, 11, 12},
			OE:              []byte{13, 14, 15, 16},
			UE:              []byte{17, 18, 19, 20},
			Perms:           []byte{21, 22, 23, 24},
		}
		assert.Equal(t, "Standard", enc.Filter)
		assert.Equal(t, "AESV2", enc.SubFilter)
		assert.Equal(t, 4, enc.V)
		assert.Equal(t, 128, enc.Length)
		assert.Equal(t, 4, enc.R)
		assert.Equal(t, []byte{1, 2, 3, 4}, enc.O)
		assert.Equal(t, []byte{5, 6, 7, 8}, enc.U)
		assert.Equal(t, uint32(PermPrint|PermCopy), enc.P)
		assert.True(t, enc.EncryptMetadata)
		assert.Equal(t, []byte{9, 10, 11, 12}, enc.ID)
		assert.Equal(t, []byte{13, 14, 15, 16}, enc.OE)
		assert.Equal(t, []byte{17, 18, 19, 20}, enc.UE)
		assert.Equal(t, []byte{21, 22, 23, 24}, enc.Perms)
	})
}

// TestCryptoFilter tests the CryptoFilter struct.
func TestCryptoFilter(t *testing.T) {
	t.Run("CryptoFilter with default values", func(t *testing.T) {
		cf := &CryptoFilter{}
		assert.Empty(t, cf.AuthEvent)
		assert.Empty(t, cf.CFM)
		assert.Equal(t, 0, cf.Length)
	})

	t.Run("CryptoFilter with values", func(t *testing.T) {
		cf := &CryptoFilter{
			AuthEvent: "DocOpen",
			CFM:       "AESV2",
			Length:    128,
		}
		assert.Equal(t, "DocOpen", cf.AuthEvent)
		assert.Equal(t, "AESV2", cf.CFM)
		assert.Equal(t, 128, cf.Length)
	})
}

// TestEncryptDict tests the EncryptDict struct.
func TestEncryptDict(t *testing.T) {
	t.Run("EncryptDict with default values", func(t *testing.T) {
		ed := &EncryptDict{}
		assert.Empty(t, ed.Filter)
		assert.Empty(t, ed.SubFilter)
		assert.Equal(t, 0, ed.V)
		assert.Equal(t, 0, ed.Length)
		assert.Equal(t, 0, ed.R)
		assert.Nil(t, ed.O)
		assert.Nil(t, ed.U)
		assert.Equal(t, uint32(0), ed.P)
		assert.False(t, ed.EncryptMetadata)
		assert.Nil(t, ed.ID)
		assert.Nil(t, ed.OE)
		assert.Nil(t, ed.UE)
		assert.Nil(t, ed.Perms)
		assert.Nil(t, ed.CF)
		assert.Empty(t, ed.StrF)
		assert.Empty(t, ed.StmF)
	})

	t.Run("EncryptDict with values", func(t *testing.T) {
		cf := &CryptoFilter{
			AuthEvent: "DocOpen",
			CFM:       "AESV2",
			Length:    128,
		}
		ed := &EncryptDict{
			Filter:          "Standard",
			SubFilter:       "AESV2",
			V:               4,
			Length:          128,
			R:               4,
			O:               []byte{1, 2},
			U:               []byte{3, 4},
			P:               uint32(PermPrint),
			EncryptMetadata: true,
			ID:              []byte{5, 6},
			OE:              []byte{7, 8},
			UE:              []byte{9, 10},
			Perms:           []byte{11, 12},
			CF:              map[string]*CryptoFilter{"StdCF": cf},
			StrF:            "StdCF",
			StmF:            "StdCF",
		}
		assert.Equal(t, "Standard", ed.Filter)
		assert.Equal(t, "AESV2", ed.SubFilter)
		assert.Equal(t, 4, ed.V)
		assert.Equal(t, 128, ed.Length)
		assert.Equal(t, 4, ed.R)
		assert.Equal(t, []byte{1, 2}, ed.O)
		assert.Equal(t, []byte{3, 4}, ed.U)
		assert.Equal(t, uint32(PermPrint), ed.P)
		assert.True(t, ed.EncryptMetadata)
		assert.Equal(t, []byte{5, 6}, ed.ID)
		assert.Equal(t, []byte{7, 8}, ed.OE)
		assert.Equal(t, []byte{9, 10}, ed.UE)
		assert.Equal(t, []byte{11, 12}, ed.Perms)
		assert.Equal(t, cf, ed.CF["StdCF"])
		assert.Equal(t, "StdCF", ed.StrF)
		assert.Equal(t, "StdCF", ed.StmF)
	})
}

// TestEncryptDict_ToEncryption tests the ToEncryption method.
func TestEncryptDict_ToEncryption(t *testing.T) {
	t.Run("ToEncryption with R4 and explicit Length", func(t *testing.T) {
		ed := &EncryptDict{
			Filter: "Standard",
			V:      4,
			R:      4,
			Length: 256,
		}
		enc := ed.ToEncryption()
		assert.Equal(t, "Standard", enc.Filter)
		assert.Equal(t, 4, enc.V)
		assert.Equal(t, 4, enc.R)
		assert.Equal(t, 256, enc.Length)
	})

	t.Run("ToEncryption with R3 sets default Length to 128", func(t *testing.T) {
		ed := &EncryptDict{
			Filter: "Standard",
			V:      2,
			R:      3,
			Length: 0, // Should default to 128
		}
		enc := ed.ToEncryption()
		assert.Equal(t, 128, enc.Length)
	})

	t.Run("ToEncryption with R2 sets default Length to 40", func(t *testing.T) {
		ed := &EncryptDict{
			Filter: "Standard",
			V:      0,
			R:      2,
			Length: 0, // Should default to 40
		}
		enc := ed.ToEncryption()
		assert.Equal(t, 40, enc.Length)
	})

	t.Run("ToEncryption copies all fields", func(t *testing.T) {
		ed := &EncryptDict{
			Filter:          "Standard",
			SubFilter:       "AESV2",
			V:               4,
			R:               4,
			Length:          128,
			O:               []byte{1, 2},
			U:               []byte{3, 4},
			P:               uint32(PermPrint),
			EncryptMetadata: true,
			ID:              []byte{5, 6},
			OE:              []byte{7, 8},
			UE:              []byte{9, 10},
			Perms:           []byte{11, 12},
		}
		enc := ed.ToEncryption()
		assert.Equal(t, "Standard", enc.Filter)
		assert.Equal(t, "AESV2", enc.SubFilter)
		assert.Equal(t, 4, enc.V)
		assert.Equal(t, 4, enc.R)
		assert.Equal(t, 128, enc.Length)
		assert.Equal(t, []byte{1, 2}, enc.O)
		assert.Equal(t, []byte{3, 4}, enc.U)
		assert.Equal(t, uint32(PermPrint), enc.P)
		assert.True(t, enc.EncryptMetadata)
		assert.Equal(t, []byte{5, 6}, enc.ID)
		assert.Equal(t, []byte{7, 8}, enc.OE)
		assert.Equal(t, []byte{9, 10}, enc.UE)
		assert.Equal(t, []byte{11, 12}, enc.Perms)
	})

	t.Run("ToEncryption with R5 and extended fields", func(t *testing.T) {
		ed := &EncryptDict{
			Filter: "Standard",
			V:      5,
			R:      5,
			OE:     make([]byte, 32),
			UE:     make([]byte, 32),
			Perms:  make([]byte, 16),
		}
		enc := ed.ToEncryption()
		assert.Equal(t, 5, enc.V)
		assert.Equal(t, 5, enc.R)
		assert.NotNil(t, enc.OE)
		assert.NotNil(t, enc.UE)
		assert.NotNil(t, enc.Perms)
		assert.Equal(t, 32, len(enc.OE))
		assert.Equal(t, 32, len(enc.UE))
		assert.Equal(t, 16, len(enc.Perms))
	})
}

// TestEncryptionAlgorithmConstants tests the algorithm constants.
func TestEncryptionAlgorithmConstants(t *testing.T) {
	t.Run("Algorithm constants have correct values", func(t *testing.T) {
		assert.Equal(t, 0, AlgorithmRC4_40)
		assert.Equal(t, 1, AlgorithmRC4_40Deprecated)
		assert.Equal(t, 2, AlgorithmVariable)
		assert.Equal(t, 4, AlgorithmAES256)
		assert.Equal(t, 5, AlgorithmAES256Rev)
	})

	t.Run("Algorithm constants are distinct", func(t *testing.T) {
		algorithms := []int{
			AlgorithmRC4_40, AlgorithmRC4_40Deprecated,
			AlgorithmVariable, AlgorithmAES256, AlgorithmAES256Rev,
		}
		for i, a1 := range algorithms {
			for j, a2 := range algorithms {
				if i != j {
					assert.NotEqual(t, a1, a2)
				}
			}
		}
	})
}

// TestEncryptionErrors tests the encryption error variables.
func TestEncryptionErrors(t *testing.T) {
	t.Run("ErrEncryptionUnsupported is a PDFError", func(t *testing.T) {
		err := ErrEncryptionUnsupported
		assert.NotNil(t, err)
		assert.Equal(t, "encryption", err.Op)
		assert.Equal(t, ErrTypeEncryption, err.Type)
	})

	t.Run("ErrInvalidPassword is a PDFError", func(t *testing.T) {
		err := ErrInvalidPassword
		assert.NotNil(t, err)
		assert.Equal(t, "encryption", err.Op)
		assert.Equal(t, ErrTypeEncryption, err.Type)
	})

	t.Run("ErrEncryptionKey is a PDFError", func(t *testing.T) {
		err := ErrEncryptionKey
		assert.NotNil(t, err)
		assert.Equal(t, "encryption", err.Op)
		assert.Equal(t, ErrTypeEncryption, err.Type)
	})

	t.Run("ErrDecryptFailed is a PDFError", func(t *testing.T) {
		err := ErrDecryptFailed
		assert.NotNil(t, err)
		assert.Equal(t, "encryption", err.Op)
		assert.Equal(t, ErrTypeEncryption, err.Type)
	})
}

// TestIsEncrypted tests the IsEncrypted function.
func TestIsEncrypted(t *testing.T) {
	t.Run("IsEncrypted returns false for nil trailer", func(t *testing.T) {
		result := IsEncrypted(nil)
		assert.False(t, result)
	})

	t.Run("IsEncrypted returns false when Encrypt entry is missing", func(t *testing.T) {
		trailer := NewDict()
		result := IsEncrypted(trailer)
		assert.False(t, result)
	})

	t.Run("IsEncrypted returns true when Encrypt entry is present", func(t *testing.T) {
		trailer := NewDict()
		trailer.Set(NewName("/Encrypt"), NewName("/Standard"))
		result := IsEncrypted(trailer)
		assert.True(t, result)
	})

	t.Run("IsEncrypted with Encrypt entry as reference", func(t *testing.T) {
		trailer := NewDict()
		ref := NewRef(10, 0)
		trailer.Set(NewName("/Encrypt"), ref)
		result := IsEncrypted(trailer)
		assert.True(t, result)
	})

	t.Run("IsEncrypted with Encrypt entry as integer", func(t *testing.T) {
		trailer := NewDict()
		trailer.Set(NewName("/Encrypt"), NewInteger(100))
		result := IsEncrypted(trailer)
		assert.True(t, result)
	})
}
