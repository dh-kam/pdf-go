// Package content provides content stream evaluation interfaces for PDF rendering.
package content

import (
	"fmt"
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
)

// Operator represents a PDF graphics operator.
type Operator interface {
	Execute(state *graphics.State, operands []float64) error
}

// OperatorRegistry manages PDF operators.
type OperatorRegistry struct {
	operators map[string]Operator
}

// NewOperatorRegistry creates a new operator registry.
func NewOperatorRegistry() *OperatorRegistry {
	return &OperatorRegistry{
		operators: make(map[string]Operator),
	}
}

// Register registers an operator.
func (r *OperatorRegistry) Register(name string, op Operator) {
	r.operators[name] = op
}

// Get retrieves an operator by name.
func (r *OperatorRegistry) Get(name string) (Operator, bool) {
	op, ok := r.operators[name]
	return op, ok
}

// Evaluator evaluates content streams.
type Evaluator struct {
	registry *OperatorRegistry
	state    *graphics.State
	objects  map[entity.Ref]entity.Object
	xref     entity.XRef
}

// NewEvaluator creates a new content stream evaluator.
func NewEvaluator(xref entity.XRef) *Evaluator {
	return &Evaluator{
		registry: NewOperatorRegistry(),
		state:    graphics.NewState(),
		objects:  make(map[entity.Ref]entity.Object),
		xref:     xref,
	}
}

// SetRegistry sets the operator registry.
func (e *Evaluator) SetRegistry(registry *OperatorRegistry) {
	e.registry = registry
}

// GetRegistry returns the operator registry.
func (e *Evaluator) GetRegistry() *OperatorRegistry {
	return e.registry
}

// GetState returns the current graphics state.
func (e *Evaluator) GetState() *graphics.State {
	return e.state
}

// SetState sets the graphics state.
func (e *Evaluator) SetState(state *graphics.State) {
	e.state = state
}

// ProcessObject processes a content stream or object.
func (e *Evaluator) ProcessObject(obj entity.Object) error {
	switch o := obj.(type) {
	case *entity.Stream:
		return e.ProcessStream(o)
	case *entity.Dict:
		return e.ProcessDict(o)
	case *entity.Array:
		return e.ProcessArray(o)
	case entity.Ref:
		if e.xref == nil {
			return errors.NotFoundf("content_ref", "xref for ref %d", o.Num())
		}
		resolved, err := e.xref.Fetch(o)
		if err != nil {
			return errors.Invalid("content_ref", err)
		}
		return e.ProcessObject(resolved)
	}
	return nil
}

// ProcessStream processes a content stream.
func (e *Evaluator) ProcessStream(stream *entity.Stream) error {
	data, err := stream.Decode()
	if err != nil {
		return err
	}

	// Parse and execute the stream
	return e.ProcessBytes(data)
}

// ProcessBytes processes raw content stream bytes.
func (e *Evaluator) ProcessBytes(data []byte) error {
	lexer := NewOperatorLexer(data)

	for {
		op, operands, err := lexer.NextOperator()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Invalid("content_parse", err)
		}

		if err := e.ExecuteOperator(op, operands); err != nil {
			return err
		}
	}

	return nil
}

// ProcessDict processes a dictionary (for Form XObjects).
func (e *Evaluator) ProcessDict(dict *entity.Dict) error {
	if dict == nil {
		return nil
	}

	contents := dict.Get(entity.Name("Contents"))
	if contents == nil {
		return nil
	}

	return e.ProcessObject(contents)
}

// ProcessArray processes an array.
func (e *Evaluator) ProcessArray(arr *entity.Array) error {
	// Process each element in the array
	for i := 0; i < arr.Len(); i++ {
		obj := arr.Get(i)
		if obj == nil {
			continue
		}
		if err := e.ProcessObject(obj); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteOperator executes an operator with its operands.
func (e *Evaluator) ExecuteOperator(name string, operands []float64) error {
	op, ok := e.registry.Get(name)
	if !ok {
		// Unknown operator, ignore
		return nil
	}

	return op.Execute(e.state, operands)
}

// SetXRef sets the cross-reference table for resolving indirect objects.
func (e *Evaluator) SetXRef(xref entity.XRef) {
	e.xref = xref
}

// StoreObject stores an object for later reference.
func (e *Evaluator) StoreObject(ref entity.Ref, obj entity.Object) {
	e.objects[ref] = obj
}

// GetObject retrieves a stored object.
func (e *Evaluator) GetObject(ref entity.Ref) (entity.Object, bool) {
	obj, ok := e.objects[ref]
	return obj, ok
}

// OperatorLexer tokenizes PDF content streams.
type OperatorLexer struct {
	data   []byte
	pos    int
	line   int
	column int
}

// NewOperatorLexer creates a new operator lexer.
func NewOperatorLexer(data []byte) *OperatorLexer {
	return &OperatorLexer{
		data: data,
	}
}

// NextOperator reads the next operator and its operands.
func (l *OperatorLexer) NextOperator() (string, []float64, error) {
	for l.pos < len(l.data) {
		b := l.data[l.pos]
		l.pos++
		l.column++

		switch {
		case b == '\n':
			l.line++
			l.column = 0
		case b == '\r':
			l.line++
			l.column = 0
			if l.pos < len(l.data) && l.data[l.pos] == '\n' {
				l.pos++
			}
		case b == '%':
			// Skip comment until end of line
			for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
		case b == ' ' || b == '\t':
			// Skip whitespace
		case b == '\f' || b == '\u0000':
			// Skip null bytes
		default:
			// Found non-whitespace, parse operator
			return l.parseOperator()
		}
	}

	return "", nil, io.EOF
}

// parseOperator parses an operator and its operands.
// In PDF content streams, operands come BEFORE the operator.
// For example: "10 20 30 Td" has operands [10, 20, 30] and operator "Td"
// Note: NextOperator() increments l.pos before calling this, so we need to adjust.
func (l *OperatorLexer) parseOperator() (string, []float64, error) {
	var operands []float64
	var operator string

	// Back up one position since NextOperator already incremented
	if l.pos > 0 {
		l.pos--
	}

	// Skip any leading whitespace
	for l.pos < len(l.data) {
		b := l.data[l.pos]
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' {
			l.pos++
			l.column++
			if b == '\n' || b == '\r' {
				l.line++
				l.column = 0
			}
			continue
		}
		break
	}

	// Keep parsing until we find an operator (non-numeric token)
	for l.pos < len(l.data) {
		// Skip whitespace
		for l.pos < len(l.data) {
			b := l.data[l.pos]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' {
				l.pos++
				l.column++
				if b == '\n' || b == '\r' {
					l.line++
					l.column = 0
				}
				continue
			}
			break
		}

		if l.pos >= len(l.data) {
			break
		}

		b := l.data[l.pos]

		// Check for delimiters that end the sequence
		if b == '<' || b == '>' || b == '[' || b == ']' || b == '(' || b == ')' || b == '%' {
			break
		}

		// Check if it's a number
		if b >= '0' && b <= '9' || b == '-' || b == '.' || b == '+' {
			num, err := l.parseNumber()
			if err == nil {
				operands = append(operands, num)
			}
			// Continue parsing
		} else {
			// It's an operator - parse it
			startPos := l.pos
			for l.pos < len(l.data) {
				b := l.data[l.pos]
				if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '<' || b == '>' || b == '[' || b == ']' || b == '(' || b == ')' || b == '%' {
					break
				}
				l.pos++
				l.column++
			}
			operator = string(l.data[startPos:l.pos])
			break
		}
	}

	if operator == "" && len(operands) == 0 {
		return "", nil, io.EOF
	}

	// If we only got numbers but no operator, treat it as just numbers
	if operator == "" {
		return "", operands, nil
	}

	return operator, operands, nil
}

// parseNumber parses a number (integer or real).
func (l *OperatorLexer) parseNumber() (float64, error) {
	startPos := l.pos

	// Handle optional sign
	if l.data[l.pos] == '-' || l.data[l.pos] == '+' {
		l.pos++
	}

	// Parse integer part
parseLoop:
	for l.pos < len(l.data) {
		b := l.data[l.pos]
		switch {
		case b >= '0' && b <= '9':
			l.pos++
		case b == '.':
			l.pos++
			// Parse fractional part
			for l.pos < len(l.data) {
				b := l.data[l.pos]
				if b >= '0' && b <= '9' {
					l.pos++
				} else {
					break
				}
			}
			break parseLoop
		default:
			break parseLoop
		}
	}

	numStr := string(l.data[startPos:l.pos])

	var num float64
	_, err := fmt.Sscanf(numStr, "%f", &num)
	if err != nil {
		return 0, err
	}

	return num, nil
}
