// Package errors tests
package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPDFError_Error tests the Error() method.
func TestPDFError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *PDFError
		expected string
	}{
		{
			name:     "error with operation and underlying error",
			err:      &PDFError{Op: "parse_xref", Err: fmt.Errorf("unexpected EOF"), Type: ErrTypeInvalid},
			expected: "pdf: parse_xref: unexpected EOF",
		},
		{
			name:     "error with operation only",
			err:      &PDFError{Op: "parse_trailer", Err: nil, Type: ErrTypeInvalid},
			expected: "pdf: parse_trailer",
		},
		{
			name:     "error with empty operation",
			err:      &PDFError{Op: "", Err: fmt.Errorf("test"), Type: ErrTypeInvalid},
			expected: "pdf: : test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPDFError_Unwrap tests the Unwrap() method for error unwrapping.
func TestPDFError_Unwrap(t *testing.T) {
	underlyingErr := fmt.Errorf("underlying error")
	pdfErr := &PDFError{Op: "test", Err: underlyingErr, Type: ErrTypeInvalid}

	assert.Equal(t, underlyingErr, pdfErr.Unwrap(), "Unwrap should return the underlying error")
}

// TestPDFError_Unwrap_Nil tests Unwrap() when underlying error is nil.
func TestPDFError_Unwrap_Nil(t *testing.T) {
	pdfErr := &PDFError{Op: "test", Err: nil, Type: ErrTypeInvalid}

	assert.Nil(t, pdfErr.Unwrap(), "Unwrap should return nil when underlying error is nil")
}

// TestNewError tests the NewError constructor.
func TestNewError(t *testing.T) {
	underlyingErr := fmt.Errorf("test error")
	err := NewError("parse_test", underlyingErr, ErrTypeInvalid)

	assert.Equal(t, "parse_test", err.Op)
	assert.Equal(t, underlyingErr, err.Err)
	assert.Equal(t, ErrTypeInvalid, err.Type)
}

// TestErrorType_Constants tests that ErrorType constants have unique values.
func TestErrorType_Constants(t *testing.T) {
	types := []ErrorType{
		ErrTypeInvalid,
		ErrTypeNotFound,
		ErrTypeEncryption,
		ErrTypeFont,
		ErrTypeRendering,
		ErrTypeIO,
	}

	seen := make(map[ErrorType]bool)
	for _, typ := range types {
		assert.False(t, seen[typ], "ErrorType %d should be unique", typ)
		seen[typ] = true
	}
}

// TestInvalid tests the Invalid() constructor.
func TestInvalid(t *testing.T) {
	underlyingErr := fmt.Errorf("invalid data")
	err := Invalid("parse_dict", underlyingErr)

	assert.Equal(t, "parse_dict", err.Op)
	assert.Equal(t, underlyingErr, err.Err)
	assert.Equal(t, ErrTypeInvalid, err.Type)
	assert.Contains(t, err.Error(), "parse_dict")
	assert.Contains(t, err.Error(), "invalid data")
}

// TestInvalidf tests the Invalidf() constructor with formatted message.
func TestInvalidf(t *testing.T) {
	err := Invalidf("parse_glyph", "character code %d out of range", 123)

	assert.Equal(t, "parse_glyph", err.Op)
	assert.Equal(t, ErrTypeInvalid, err.Type)
	assert.Contains(t, err.Error(), "character code 123 out of range")
}

// TestInvalidf_MultipleArgs tests Invalidf() with multiple format arguments.
func TestInvalidf_MultipleArgs(t *testing.T) {
	err := Invalidf("parse", "expected %d, got %d at offset %d", 10, 20, 100)

	assert.Contains(t, err.Error(), "expected 10, got 20 at offset 100")
}

// TestNotFound tests the NotFound() constructor.
func TestNotFound(t *testing.T) {
	underlyingErr := fmt.Errorf("object not found")
	err := NotFound("fetch_object", underlyingErr)

	assert.Equal(t, "fetch_object", err.Op)
	assert.Equal(t, underlyingErr, err.Err)
	assert.Equal(t, ErrTypeNotFound, err.Type)
}

// TestNotFoundf tests the NotFoundf() constructor with formatted message.
func TestNotFoundf(t *testing.T) {
	err := NotFoundf("glyph", "character code %d", 65)

	assert.Equal(t, "glyph", err.Op)
	assert.Equal(t, ErrTypeNotFound, err.Type)
	assert.Contains(t, err.Error(), "character code 65")
}

// TestMissing tests the Missing() constructor.
func TestMissing(t *testing.T) {
	err := Missing("trailer")

	assert.Equal(t, "trailer", err.Op)
	assert.Equal(t, ErrTypeNotFound, err.Type)
	assert.Contains(t, err.Error(), "missing required object")
}

// TestEncryption tests the Encryption() constructor.
func TestEncryption(t *testing.T) {
	underlyingErr := fmt.Errorf("invalid password")
	err := Encryption("decrypt", underlyingErr)

	assert.Equal(t, "decrypt", err.Op)
	assert.Equal(t, underlyingErr, err.Err)
	assert.Equal(t, ErrTypeEncryption, err.Type)
}

// TestFont tests the Font() constructor.
func TestFont(t *testing.T) {
	underlyingErr := fmt.Errorf("unsupported font type")
	err := Font("parse_font", underlyingErr)

	assert.Equal(t, "parse_font", err.Op)
	assert.Equal(t, underlyingErr, err.Err)
	assert.Equal(t, ErrTypeFont, err.Type)
}

// TestRendering tests the Rendering() constructor.
func TestRendering(t *testing.T) {
	underlyingErr := fmt.Errorf("canvas error")
	err := Rendering("render_page", underlyingErr)

	assert.Equal(t, "render_page", err.Op)
	assert.Equal(t, underlyingErr, err.Err)
	assert.Equal(t, ErrTypeRendering, err.Type)
}

// TestIO tests the IO() constructor.
func TestIO(t *testing.T) {
	underlyingErr := fmt.Errorf("file not found")
	err := IO("read_file", underlyingErr)

	assert.Equal(t, "read_file", err.Op)
	assert.Equal(t, underlyingErr, err.Err)
	assert.Equal(t, ErrTypeIO, err.Type)
}

// TestNotImplemented tests the NotImplemented() constructor.
func TestNotImplemented(t *testing.T) {
	underlyingErr := fmt.Errorf("feature not supported")
	err := NotImplemented("parse_jbig2", underlyingErr)

	assert.Equal(t, "parse_jbig2", err.Op)
	assert.Equal(t, underlyingErr, err.Err)
	assert.Equal(t, ErrTypeRendering, err.Type)
}

// TestOutOfRangeError tests the OutOfRangeError type.
func TestOutOfRangeError(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		code     uint32
	}{
		{
			name:     "normal code",
			code:     123,
			expected: "character code 123 out of range",
		},
		{
			name:     "zero code",
			code:     0,
			expected: "character code 0 out of range",
		},
		{
			name:     "large code",
			code:     0x10FFFF,
			expected: "character code 1114111 out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &OutOfRangeError{Code: tt.code}
			assert.Equal(t, tt.expected, err.Error())
		})
	}
}

// TestErrorUnwrapping tests errors.Is and errors.As compatibility.
func TestErrorUnwrapping(t *testing.T) {
	t.Run("errors.Is with PDFError", func(t *testing.T) {
		underlyingErr := fmt.Errorf("base error")
		pdfErr := Invalid("test_op", underlyingErr)

		assert.True(t, errors.Is(pdfErr, underlyingErr), "errors.Is should find underlying error")
		assert.False(t, errors.Is(pdfErr, fmt.Errorf("other error")), "errors.Is should not match different error")
	})

	t.Run("errors.As with PDFError", func(t *testing.T) {
		err := Invalid("test_op", fmt.Errorf("test"))

		var pdfErr *PDFError
		assert.True(t, errors.As(err, &pdfErr), "errors.As should extract PDFError")
		assert.Equal(t, "test_op", pdfErr.Op)

		var notPDFErr *OutOfRangeError
		assert.False(t, errors.As(err, &notPDFErr), "errors.As should not extract different error type")
	})
}

// TestErrorWrapping_Chain tests wrapping errors in a chain.
func TestErrorWrapping_Chain(t *testing.T) {
	baseErr := fmt.Errorf("base error")
	middleErr := Invalid("middle", baseErr)
	topErr := NotFound("top", middleErr)

	// The chain should work through Unwrap()
	assert.Equal(t, middleErr, topErr.Unwrap())
	assert.Equal(t, baseErr, middleErr.Unwrap())

	// errors.Is should find errors in the chain
	assert.True(t, errors.Is(topErr, middleErr))
	assert.True(t, errors.Is(topErr, baseErr))
	assert.False(t, errors.Is(topErr, fmt.Errorf("other")))
}

// TestPDFError_Type tests the Type field.
func TestPDFError_Type(t *testing.T) {
	tests := []struct {
		constructor  func(string, error) *PDFError
		name         string
		expectedType ErrorType
	}{
		{
			name:         "Invalid returns ErrTypeInvalid",
			constructor:  Invalid,
			expectedType: ErrTypeInvalid,
		},
		{
			name:         "NotFound returns ErrTypeNotFound",
			constructor:  NotFound,
			expectedType: ErrTypeNotFound,
		},
		{
			name:         "Encryption returns ErrTypeEncryption",
			constructor:  Encryption,
			expectedType: ErrTypeEncryption,
		},
		{
			name:         "Font returns ErrTypeFont",
			constructor:  Font,
			expectedType: ErrTypeFont,
		},
		{
			name:         "Rendering returns ErrTypeRendering",
			constructor:  Rendering,
			expectedType: ErrTypeRendering,
		},
		{
			name:         "IO returns ErrTypeIO",
			constructor:  IO,
			expectedType: ErrTypeIO,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.constructor("test_op", fmt.Errorf("test"))
			assert.Equal(t, tt.expectedType, err.Type)
		})
	}
}

// TestErrorFormatting tests formatting of error messages.
func TestErrorFormatting(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains []string
	}{
		{
			name:     "Invalid error includes operation",
			err:      Invalid("parse_xref", fmt.Errorf("unexpected EOF")),
			contains: []string{"pdf", "parse_xref", "unexpected EOF"},
		},
		{
			name:     "NotFound error includes operation",
			err:      NotFound("fetch_object", fmt.Errorf("object not found")),
			contains: []string{"pdf", "fetch_object", "object not found"},
		},
		{
			name:     "Invalidf error includes formatted message",
			err:      Invalidf("glyph", "code %d invalid", 42),
			contains: []string{"pdf", "glyph", "code 42 invalid"},
		},
		{
			name:     "Missing error includes default message",
			err:      Missing("trailer"),
			contains: []string{"pdf", "trailer", "missing required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			for _, substr := range tt.contains {
				assert.Contains(t, errMsg, substr, "error message should contain %q", substr)
			}
		})
	}
}

// TestNilUnderlyingError tests behavior with nil underlying error.
func TestNilUnderlyingError(t *testing.T) {
	err := Invalid("test", nil)

	assert.Equal(t, "pdf: test", err.Error())
	assert.Nil(t, err.Unwrap())
}

// TestEmptyOperation tests behavior with empty operation string.
func TestEmptyOperation(t *testing.T) {
	underlyingErr := fmt.Errorf("test")
	err := Invalid("", underlyingErr)

	assert.Equal(t, "pdf: : test", err.Error())
	assert.Equal(t, underlyingErr, err.Unwrap())
}

// TestConvenienceFunctions tests all convenience functions work correctly.
func TestConvenienceFunctions(t *testing.T) {
	testErr := fmt.Errorf("test error")

	tests := []struct {
		fn    func() *PDFError
		check func(*testing.T, *PDFError)
		name  string
	}{
		{
			name: "Invalid",
			fn:   func() *PDFError { return Invalid("op", testErr) },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Equal(t, testErr, e.Err)
				assert.Equal(t, ErrTypeInvalid, e.Type)
			},
		},
		{
			name: "Invalidf",
			fn:   func() *PDFError { return Invalidf("op", "msg %d", 1) },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Contains(t, e.Err.Error(), "msg 1")
				assert.Equal(t, ErrTypeInvalid, e.Type)
			},
		},
		{
			name: "NotFound",
			fn:   func() *PDFError { return NotFound("op", testErr) },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Equal(t, testErr, e.Err)
				assert.Equal(t, ErrTypeNotFound, e.Type)
			},
		},
		{
			name: "NotFoundf",
			fn:   func() *PDFError { return NotFoundf("op", "msg %s", "test") },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Contains(t, e.Err.Error(), "msg test")
				assert.Equal(t, ErrTypeNotFound, e.Type)
			},
		},
		{
			name: "Missing",
			fn:   func() *PDFError { return Missing("op") },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Contains(t, e.Err.Error(), "missing required")
				assert.Equal(t, ErrTypeNotFound, e.Type)
			},
		},
		{
			name: "Encryption",
			fn:   func() *PDFError { return Encryption("op", testErr) },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Equal(t, testErr, e.Err)
				assert.Equal(t, ErrTypeEncryption, e.Type)
			},
		},
		{
			name: "Font",
			fn:   func() *PDFError { return Font("op", testErr) },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Equal(t, testErr, e.Err)
				assert.Equal(t, ErrTypeFont, e.Type)
			},
		},
		{
			name: "Rendering",
			fn:   func() *PDFError { return Rendering("op", testErr) },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Equal(t, testErr, e.Err)
				assert.Equal(t, ErrTypeRendering, e.Type)
			},
		},
		{
			name: "IO",
			fn:   func() *PDFError { return IO("op", testErr) },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Equal(t, testErr, e.Err)
				assert.Equal(t, ErrTypeIO, e.Type)
			},
		},
		{
			name: "NotImplemented",
			fn:   func() *PDFError { return NotImplemented("op", testErr) },
			check: func(t *testing.T, e *PDFError) {
				assert.Equal(t, "op", e.Op)
				assert.Equal(t, testErr, e.Err)
				assert.Equal(t, ErrTypeRendering, e.Type)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.NotNil(t, err)
			tt.check(t, err)
		})
	}
}

// BenchmarkErrorCreation benchmarks error creation.
func BenchmarkErrorCreation(b *testing.B) {
	testErr := fmt.Errorf("test error")

	b.Run("Invalid", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Invalid("test_op", testErr)
		}
	})

	b.Run("Invalidf", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Invalidf("test_op", "code %d", i)
		}
	})

	b.Run("NewError", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewError("test_op", testErr, ErrTypeInvalid)
		}
	})
}

// BenchmarkErrorString benchmarks error string conversion.
func BenchmarkErrorString(b *testing.B) {
	err := Invalid("parse_xref", fmt.Errorf("unexpected EOF"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

// BenchmarkErrorUnwrap benchmarks error unwrapping.
func BenchmarkErrorUnwrap(b *testing.B) {
	err := Invalid("parse_xref", fmt.Errorf("unexpected EOF"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Unwrap()
	}
}
