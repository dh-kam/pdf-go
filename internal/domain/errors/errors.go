// Package errors defines domain-specific error types for PDF processing.
package errors

import "fmt"

// ErrorType represents the category of an error.
type ErrorType int

const (
	// ErrTypeInvalid indicates an invalid or malformed PDF structure.
	ErrTypeInvalid ErrorType = iota
	// ErrTypeNotFound indicates a requested object was not found.
	ErrTypeNotFound
	// ErrTypeEncryption indicates an encryption-related error.
	ErrTypeEncryption
	// ErrTypeFont indicates a font-related error.
	ErrTypeFont
	// ErrTypeRendering indicates a rendering-related error.
	ErrTypeRendering
	// ErrTypeIO indicates an I/O error.
	ErrTypeIO
)

// PDFError represents a domain-specific error.
type PDFError struct {
	Err  error
	Op   string
	Type ErrorType
}

// Error returns the error message.
func (e *PDFError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("pdf: %s", e.Op)
	}
	return fmt.Sprintf("pdf: %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error.
func (e *PDFError) Unwrap() error {
	return e.Err
}

// NewError creates a new PDFError.
func NewError(op string, err error, typ ErrorType) *PDFError {
	return &PDFError{
		Op:   op,
		Err:  err,
		Type: typ,
	}
}

// Error constructors for convenience

// Invalid creates an error for invalid/malformed PDF structures.
func Invalid(op string, err error) *PDFError {
	return &PDFError{Op: op, Err: err, Type: ErrTypeInvalid}
}

// Invalidf creates an error with formatted message.
func Invalidf(op string, format string, args ...interface{}) *PDFError {
	return &PDFError{
		Op:   op,
		Err:  fmt.Errorf(format, args...),
		Type: ErrTypeInvalid,
	}
}

// NotFound creates an error for missing objects.
func NotFound(op string, err error) *PDFError {
	return &PDFError{Op: op, Err: err, Type: ErrTypeNotFound}
}

// NotFoundf creates an error with formatted message.
func NotFoundf(op string, format string, args ...interface{}) *PDFError {
	return &PDFError{
		Op:   op,
		Err:  fmt.Errorf(format, args...),
		Type: ErrTypeNotFound,
	}
}

// Missing creates an error for missing required objects.
func Missing(op string) *PDFError {
	return &PDFError{Op: op, Err: fmt.Errorf("missing required object"), Type: ErrTypeNotFound}
}

// Encryption creates an error for encryption-related failures.
func Encryption(op string, err error) *PDFError {
	return &PDFError{Op: op, Err: err, Type: ErrTypeEncryption}
}

// Font creates an error for font-related failures.
func Font(op string, err error) *PDFError {
	return &PDFError{Op: op, Err: err, Type: ErrTypeFont}
}

// Rendering creates an error for rendering-related failures.
func Rendering(op string, err error) *PDFError {
	return &PDFError{Op: op, Err: err, Type: ErrTypeRendering}
}

// IO creates an error for I/O failures.
func IO(op string, err error) *PDFError {
	return &PDFError{Op: op, Err: err, Type: ErrTypeIO}
}

// NotImplemented creates an error for features that are not yet implemented.
func NotImplemented(op string, err error) *PDFError {
	return &PDFError{Op: op, Err: err, Type: ErrTypeRendering}
}

// OutOfRangeError indicates a value is out of valid range.
type OutOfRangeError struct {
	Code uint32
}

// Error returns the error message.
func (e *OutOfRangeError) Error() string {
	return fmt.Sprintf("character code %d out of range", e.Code)
}
