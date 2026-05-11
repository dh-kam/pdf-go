// Package type1 provides Type1 font eexec and charstring functionality.
package type1

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

const eexecR = uint16(55665)
const charStringR = uint16(4330)
const (
	eexecDiscard = 4
)

func DecryptEexec(data []byte) ([]byte, error) {
	if len(data) <= eexecDiscard {
		return nil, errors.Invalid("eexec_decrypt", fmt.Errorf("data too short"))
	}

	return decryptType1(data, eexecR, eexecDiscard)
}

func EncryptEexec(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	// Eexec is a stream cipher; encrypt and decrypt are symmetric.
	result := make([]byte, len(data))
	c1, c2 := uint32(52845), uint32(22719)
	r := uint32(eexecR)

	for i := 0; i < len(data); i++ {
		enc := byte((uint32(data[i]) ^ (r >> 8)) & 0xff)
		result[i] = enc
		r = ((uint32(enc)+r)*c1 + c2) & 0xffff
	}

	return result, nil
}

func DecryptCharString(data []byte) ([]byte, error) {
	return DecryptCharStringWithLenIV(data, 4)
}

func DecryptCharStringWithLenIV(data []byte, lenIV int) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}
	if lenIV == -1 {
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	}
	if lenIV < 0 {
		return nil, errors.Invalid("charstring", fmt.Errorf("invalid lenIV: %d", lenIV))
	}
	if lenIV > len(data) {
		return nil, errors.Invalid("charstring", fmt.Errorf("lenIV larger than data length"))
	}

	return decryptType1(data, charStringR, lenIV)
}

func decryptType1(data []byte, key uint16, discard int) ([]byte, error) {
	if len(data) <= discard {
		if len(data) == 0 {
			return nil, errors.Invalid("type1_decrypt", fmt.Errorf("data too short"))
		}
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	}

	if discard < 0 {
		discard = 0
	}

	result := make([]byte, 0, len(data)-discard)
	c1, c2 := uint32(52845), uint32(22719)
	r := uint32(key)

	for i := 0; i < len(data); i++ {
		if i < discard {
			r = ((uint32(data[i])+r)*c1 + c2) & 0xffff
			continue
		}

		value := data[i]
		result = append(result, value^byte(r>>8))
		r = ((uint32(value)+r)*c1 + c2) & 0xffff
	}

	return result, nil
}

// CharStringDecoder handles Type1 CharString decoding.
type CharStringDecoder struct {
	data          []byte
	stack         []float64
	pos           int
	subrs         [][]byte
	commands      []Command
	width         float64
	lsb           float64
	depth         int
	othersubrArgs []float64 // args returned by callothersubr for pop
	flexActive    bool      // true when in flex sequence (between othersubr 1 and 0)
	flexCoords    []float64 // accumulated flex coordinates from rmoveto during flex
}

// NewCharStringDecoder creates a CharString decoder.
func NewCharStringDecoder(data []byte) *CharStringDecoder {
	return &CharStringDecoder{
		data:  data,
		pos:   0,
		subrs: make([][]byte, 0),
	}
}

func NewCharStringDecoderWithSubrs(data []byte, subrs [][]byte) *CharStringDecoder {
	return &CharStringDecoder{
		data:  data,
		pos:   0,
		subrs: subrs,
	}
}

// Decode decodes a Type1 CharString into commands and tracks width/LSB.
func (d *CharStringDecoder) Decode() ([]Command, error) {
	for d.pos < len(d.data) {
		cmd, err := d.nextCommand()
		if err != nil {
			return nil, err
		}
		if cmd == nil {
			continue
		}
		d.commands = append(d.commands, *cmd)
		if cmd.Type == CmdEndChar || cmd.Type == CmdReturn {
			break
		}
	}

	if d.width == 0 {
		d.width = 500
	}

	return d.commands, nil
}

// Width returns the glyph advance width discovered while decoding.
func (d *CharStringDecoder) Width() float64 {
	return d.width
}

// LSB returns the left side bearing discovered while decoding.
func (d *CharStringDecoder) LSB() float64 {
	return d.lsb
}

func (d *CharStringDecoder) nextCommand() (*Command, error) {
	if d.pos >= len(d.data) {
		return nil, nil
	}

	b0 := d.data[d.pos]
	d.pos++

	if b0 == 12 {
		if d.pos >= len(d.data) {
			return nil, fmt.Errorf("incomplete escape sequence")
		}
		b1 := d.data[d.pos]
		d.pos++
		return d.readCommand(CommandType(int(b1) + 32))
	}

	if b0 < 32 {
		if b0 == 0 {
			return nil, nil
		}
		return d.readCommand(CommandType(b0))
	}

	if b0 == 255 {
		if d.pos+4 > len(d.data) {
			return nil, nil
		}
		val := int32(d.data[d.pos])<<24 | int32(d.data[d.pos+1])<<16 |
			int32(d.data[d.pos+2])<<8 | int32(d.data[d.pos+3])
		d.pos += 4
		d.stack = append(d.stack, float64(val)/65536.0)
		return nil, nil
	}

	if b0 >= 247 && b0 <= 250 {
		if d.pos >= len(d.data) {
			return nil, fmt.Errorf("incomplete integer encoding")
		}
		b1 := d.data[d.pos]
		d.pos++
		val := float64((int32(b0)-247)*256 + int32(b1) + 108)
		d.stack = append(d.stack, val)
		return nil, nil
	}

	if b0 >= 251 && b0 <= 254 {
		if d.pos >= len(d.data) {
			return nil, nil
		}
		b1 := d.data[d.pos]
		d.pos++
		val := float64(-((int32(b0) - 251) * 256) - int32(b1) - 108)
		d.stack = append(d.stack, val)
		return nil, nil
	}

	if b0 >= 32 {
		d.stack = append(d.stack, float64(int32(b0)-139))
	}

	return nil, nil
}

func (d *CharStringDecoder) readCommand(cmd CommandType) (*Command, error) {
	switch cmd {
	case CmdHStem, CmdVStem, CmdHStemHM, CmdVStemHM, CmdHintmask, CmdCntrmask:
		d.stack = nil
		return nil, nil

	case CmdClosePath:
		d.stack = nil
		return &Command{Type: cmd}, nil

	case CmdReturn:
		// return: resume calling charstring. Stack is preserved (shared).
		// Return a sentinel command so Decode() in subroutines can stop here.
		return &Command{Type: CmdReturn}, nil

	case CmdEndChar:
		// Ignore trailing operators and clear the temporary stack.
		d.stack = nil
		return &Command{Type: cmd}, nil

	case CmdDiv:
		if len(d.stack) < 2 {
			return nil, nil
		}
		b := d.pop(2)
		d.stack = append(d.stack, b[0]/b[1])
		return nil, nil

	case CmdCallSubr:
		if len(d.stack) < 1 {
			return nil, nil
		}
		idx := int(d.pop(1)[0])
		if idx >= 0 && idx < len(d.subrs) && d.subrs[idx] != nil {
			if d.depth >= 8 {
				return nil, nil
			}
			d.depth++
			sub := NewCharStringDecoderWithSubrs(d.subrs[idx], d.subrs)
			sub.depth = d.depth
			sub.stack = d.stack // share operand stack
			sub.width = d.width
			sub.lsb = d.lsb
			sub.othersubrArgs = d.othersubrArgs
			sub.flexActive = d.flexActive
			sub.flexCoords = d.flexCoords
			subCommands, err := sub.Decode()
			if err != nil {
				d.depth--
				return nil, nil
			}
			// Filter out CmdReturn from subroutine commands
			for _, sc := range subCommands {
				if sc.Type == CmdReturn {
					continue
				}
				d.commands = append(d.commands, sc)
			}
			d.stack = sub.stack // recover stack state
			d.width = sub.width
			d.lsb = sub.lsb
			d.othersubrArgs = sub.othersubrArgs
			d.flexActive = sub.flexActive
			d.flexCoords = sub.flexCoords
			d.depth--
		}
		return nil, nil

	case CmdRMoveto:
		args := d.pop(2)
		if len(args) == 0 {
			return nil, nil
		}
		if d.flexActive {
			// During flex, rmoveto coordinates are accumulated, not emitted
			d.flexCoords = append(d.flexCoords, args[0], args[1])
			return nil, nil
		}
		return &Command{Type: CmdRMoveto, Args: args}, nil

	case CmdHMoveto:
		args := d.pop(1)
		if len(args) == 0 {
			return nil, nil
		}
		if d.flexActive {
			d.flexCoords = append(d.flexCoords, args[0], 0)
			return nil, nil
		}
		return &Command{Type: CmdRMoveto, Args: []float64{args[0], 0}}, nil

	case CmdVMoveto:
		args := d.pop(1)
		if len(args) == 0 {
			return nil, nil
		}
		if d.flexActive {
			d.flexCoords = append(d.flexCoords, 0, args[0])
			return nil, nil
		}
		return &Command{Type: CmdRMoveto, Args: []float64{0, args[0]}}, nil

	case CmdRLineto:
		args := d.popAll()
		if len(args) == 0 {
			return nil, nil
		}
		for i := 0; i+1 < len(args); i += 2 {
			d.commands = append(d.commands, Command{Type: CmdRLineto, Args: args[i : i+2]})
		}
		return nil, nil

	case CmdHlineto:
		args := d.consumeAlternatingHL()
		if len(args) == 0 {
			return nil, nil
		}
		for i := 0; i < len(args); i += 2 {
			d.commands = append(d.commands, Command{Type: CmdRLineto, Args: args[i : i+2]})
		}
		return nil, nil

	case CmdVlineto:
		args := d.consumeAlternatingVL()
		if len(args) == 0 {
			return nil, nil
		}
		for i := 0; i < len(args); i += 2 {
			d.commands = append(d.commands, Command{Type: CmdRLineto, Args: args[i : i+2]})
		}
		return nil, nil

	case CmdRRCurveto:
		args := d.popAll()
		if len(args) == 0 {
			return nil, nil
		}
		for i := 0; i+5 < len(args); i += 6 {
			cmd := Command{Type: CmdRRCurveto, Args: args[i : i+6]}
			d.commands = append(d.commands, cmd)
		}
		return nil, nil

	case CmdRCurveline:
		args := d.popAll()
		if len(args) < 8 {
			return fallbackLineCommands(d, args), nil
		}
		d.commands = append(d.commands, Command{
			Type: CmdRRCurveto,
			Args: args[:6],
		})
		d.commands = append(d.commands, Command{
			Type: CmdRLineto,
			Args: args[6:8],
		})
		return nil, nil

	case CmdRLinecurve:
		args := d.popAll()
		if len(args) < 8 {
			return fallbackLineCommands(d, args), nil
		}
		if len(args) >= 10 {
			d.commands = append(d.commands, Command{
				Type: CmdRLineto,
				Args: args[:2],
			})
			d.commands = append(d.commands, Command{
				Type: CmdRLineto,
				Args: args[2:4],
			})
			d.commands = append(d.commands, Command{
				Type: CmdRRCurveto,
				Args: args[4:],
			})
		} else {
			return fallbackLineCommands(d, args), nil
		}
		return nil, nil

	case CmdVVCurveto:
		args := d.popAll()
		if len(args) == 0 {
			return nil, nil
		}
		// vvcurveto: [dx1] dya dxb dyb dyc ...
		i := 0
		dx1Extra := 0.0
		if len(args)%4 == 1 {
			dx1Extra = args[0]
			i = 1
		}
		for i+3 < len(args) {
			dx1 := dx1Extra
			dy1 := args[i]
			dx2 := args[i+1]
			dy2 := args[i+2]
			dy3 := args[i+3]
			d.commands = append(d.commands, Command{
				Type: CmdRRCurveto,
				Args: []float64{dx1, dy1, dx2, dy2, 0, dy3},
			})
			dx1Extra = 0
			i += 4
		}
		return nil, nil

	case CmdHHCurveto:
		args := d.popAll()
		if len(args) == 0 {
			return nil, nil
		}
		// hhcurveto: [dy1] dxa dxb dyb dxc ...
		i := 0
		dy1Extra := 0.0
		if len(args)%4 == 1 {
			dy1Extra = args[0]
			i = 1
		}
		for i+3 < len(args) {
			dx1 := args[i]
			dy1 := dy1Extra
			dx2 := args[i+1]
			dy2 := args[i+2]
			dx3 := args[i+3]
			d.commands = append(d.commands, Command{
				Type: CmdRRCurveto,
				Args: []float64{dx1, dy1, dx2, dy2, dx3, 0},
			})
			dy1Extra = 0
			i += 4
		}
		return nil, nil

	case CmdSbw:
		// sbw: sbx sby wx wy sbw
		vals := d.popAll()
		if len(vals) < 4 {
			return nil, nil
		}
		sbx := vals[0]
		sby := vals[1]
		wx := vals[2]
		d.lsb = sbx
		d.width = wx
		d.stack = nil
		return &Command{Type: CmdRMoveto, Args: []float64{sbx, sby}}, nil

	case CmdHsbw:
		vals := d.pop(2)
		if len(vals) < 2 {
			return nil, nil
		}
		d.lsb = vals[0]
		d.width = vals[1]
		d.stack = nil
		return &Command{Type: CmdRMoveto, Args: []float64{vals[0], 0}}, nil

	case CmdCallothersubr:
		// callothersubr: arg1 ... argn n othersubr# callothersubr
		// Stack: ... args n subrIdx
		if len(d.stack) < 2 {
			d.stack = nil
			return nil, nil
		}
		othersubrIdx := int(d.stack[len(d.stack)-1])
		nArgs := int(d.stack[len(d.stack)-2])
		d.stack = d.stack[:len(d.stack)-2]

		if nArgs > len(d.stack) {
			nArgs = len(d.stack)
		}
		args := make([]float64, nArgs)
		if nArgs > 0 {
			copy(args, d.stack[len(d.stack)-nArgs:])
			d.stack = d.stack[:len(d.stack)-nArgs]
		}

		switch othersubrIdx {
		case 0:
			// Flex end: args=[flexDepth, finalX, finalY]
			// The 7 flex coordinate pairs were collected in flexCoords via rmoveto calls.
			// Emit 2 curveto commands from the collected coordinates.
			d.flexActive = false
			fc := d.flexCoords
			if len(fc) >= 14 {
				// fc[0:2] = reference point displacement (skip)
				// fc[2:4] through fc[12:14] = 6 relative displacements for 2 curves
				d.commands = append(d.commands, Command{
					Type: CmdRRCurveto,
					Args: []float64{fc[2], fc[3], fc[4], fc[5], fc[6], fc[7]},
				})
				d.commands = append(d.commands, Command{
					Type: CmdRRCurveto,
					Args: []float64{fc[8], fc[9], fc[10], fc[11], fc[12], fc[13]},
				})
			}
			d.flexCoords = nil
			// Push finalX, finalY for subsequent pop calls
			if len(args) >= 3 {
				d.othersubrArgs = []float64{args[1], args[2]}
			} else {
				d.othersubrArgs = []float64{0, 0}
			}
		case 1:
			// Flex start: begin accumulating flex coordinates
			d.flexActive = true
			d.flexCoords = make([]float64, 0, 16)
			d.othersubrArgs = nil
		case 2:
			// Flex midpoint: push 0 for subsequent pop (setcurrentpoint)
			d.othersubrArgs = []float64{0}
		case 3:
			// Hint replacement: push 1 for pop
			if nArgs > 0 {
				d.othersubrArgs = []float64{args[0]}
			} else {
				d.othersubrArgs = []float64{1}
			}
		default:
			// Unknown othersubr: push args for subsequent pops
			d.othersubrArgs = make([]float64, nArgs)
			copy(d.othersubrArgs, args)
		}
		return nil, nil

	case CmdPop:
		// Pop a value placed by callothersubr
		if len(d.othersubrArgs) > 0 {
			val := d.othersubrArgs[0]
			d.othersubrArgs = d.othersubrArgs[1:]
			d.stack = append(d.stack, val)
		} else {
			d.stack = append(d.stack, 0)
		}
		return nil, nil

	case CmdSeac:
		// seac: asb adx ady bchar achar seac (composite character)
		// For now, just handle width and ignore the composition.
		d.stack = nil
		return &Command{Type: CmdEndChar}, nil

	case CmdDotSection, CmdVStem3, CmdHStem3, CmdHStem4, CmdVStem4:
		// Hint operators - ignore for rendering
		d.stack = nil
		return nil, nil

	case CmdDup:
		if len(d.stack) > 0 {
			d.stack = append(d.stack, d.stack[len(d.stack)-1])
		}
		return nil, nil

	case CmdExch:
		if len(d.stack) >= 2 {
			n := len(d.stack)
			d.stack[n-1], d.stack[n-2] = d.stack[n-2], d.stack[n-1]
		}
		return nil, nil

	case CmdDrop:
		if len(d.stack) > 0 {
			d.stack = d.stack[:len(d.stack)-1]
		}
		return nil, nil

	case CmdAdd:
		if len(d.stack) >= 2 {
			b := d.pop(2)
			d.stack = append(d.stack, b[0]+b[1])
		}
		return nil, nil

	case CmdSub:
		if len(d.stack) >= 2 {
			b := d.pop(2)
			d.stack = append(d.stack, b[0]-b[1])
		}
		return nil, nil

	case CmdMul:
		if len(d.stack) >= 2 {
			b := d.pop(2)
			d.stack = append(d.stack, b[0]*b[1])
		}
		return nil, nil

	case CmdNeg:
		if len(d.stack) > 0 {
			d.stack[len(d.stack)-1] = -d.stack[len(d.stack)-1]
		}
		return nil, nil

	case CmdAbs:
		if len(d.stack) > 0 {
			v := d.stack[len(d.stack)-1]
			if v < 0 {
				d.stack[len(d.stack)-1] = -v
			}
		}
		return nil, nil

	default:
		// Unknown operators: clear the stack to avoid garbage accumulation
		d.stack = nil
		return nil, nil
	}
}

func fallbackLineCommands(d *CharStringDecoder, args []float64) *Command {
	if len(args) < 2 {
		return nil
	}

	for i := 0; i+1 < len(args); i += 2 {
		d.commands = append(d.commands, Command{
			Type: CmdRLineto,
			Args: []float64{args[i], args[i+1]},
		})
	}
	return nil
}

func (d *CharStringDecoder) pop(n int) []float64 {
	if n <= 0 || len(d.stack) < n {
		return nil
	}
	start := len(d.stack) - n
	args := append([]float64(nil), d.stack[start:]...)
	d.stack = d.stack[:start]
	return args
}

func (d *CharStringDecoder) popAll() []float64 {
	args := d.stack
	d.stack = nil
	return args
}

func (d *CharStringDecoder) consumeAlternatingHL() []float64 {
	vals := d.stack
	d.stack = nil
	if len(vals) == 0 {
		return nil
	}
	args := make([]float64, 0, len(vals)*2)
	for i := 0; i < len(vals); i++ {
		if i%2 == 0 {
			args = append(args, vals[i], 0)
			continue
		}
		args = append(args, 0, vals[i])
	}
	return args
}

func (d *CharStringDecoder) consumeAlternatingVL() []float64 {
	vals := d.stack
	d.stack = nil
	if len(vals) == 0 {
		return nil
	}
	args := make([]float64, 0, len(vals)*2)
	for i := 0; i < len(vals); i++ {
		if i%2 == 0 {
			args = append(args, 0, vals[i])
		} else {
			args = append(args, vals[i], 0)
		}
	}
	return args
}

// Command represents a Type1 CharString command.
type Command struct {
	Args []float64
	Type CommandType
}

// CommandType represents a Type1 CharString command type.
type CommandType int

// CommandType constants define Type1 CharString operators.
const (
	CmdHStem      CommandType = 1  // hstem
	CmdVStem      CommandType = 3  // vstem
	CmdVMoveto    CommandType = 4  // vmoveto
	CmdRLineto    CommandType = 5  // rlineto
	CmdHlineto    CommandType = 6  // hlineto
	CmdVlineto    CommandType = 7  // vlineto
	CmdRRCurveto  CommandType = 8  // rrcurveto
	CmdClosePath  CommandType = 9  // closepath
	CmdEndChar    CommandType = 14 // endchar
	CmdHStemHM    CommandType = 18 // hstemhm
	CmdHintmask   CommandType = 19 // hintmask
	CmdCntrmask   CommandType = 20 // cntrmask
	CmdRMoveto    CommandType = 21 // rmoveto
	CmdHMoveto    CommandType = 22 // hmoveto
	CmdVStemHM    CommandType = 23 // vstemhm
	CmdRCurveline CommandType = 24 // rcurveline
	CmdRLinecurve CommandType = 25 // rlinecurve
	CmdVVCurveto  CommandType = 26 // vvcurveto
	CmdHHCurveto  CommandType = 27 // hhcurveto
	CmdCallSubr   CommandType = 10 // callsubr
	CmdReturn     CommandType = 11 // return
	CmdEscape     CommandType = 12 // escape: two-byte command follows

	// Escape commands are reported as 32 + sub-code.
	// Type1 escape sub-codes: 0=dotsection, 1=vstem3, 2=hstem3, 6=seac, 7=sbw,
	// 9=abs, 10=add, 11=sub, 12=div, 14=neg, 15=eq, 16=callothersubr, 17=pop,
	// 27=dup, 28=exch, 33=setcurrentpoint
	CmdDotSection    CommandType = 32 // escape 0
	CmdVStem3        CommandType = 33 // escape 1
	CmdHStem3        CommandType = 34 // escape 2
	CmdAnd           CommandType = 35 // escape 3 (Type2/CFF extension)
	CmdOr            CommandType = 36 // escape 4 (Type2/CFF extension)
	CmdNot           CommandType = 37 // escape 5 (Type2/CFF extension)
	CmdSeac          CommandType = 38 // escape 6
	CmdSbw           CommandType = 39 // escape 7
	CmdAbs           CommandType = 41 // escape 9
	CmdAdd           CommandType = 42 // escape 10
	CmdSub           CommandType = 43 // escape 11
	CmdDiv           CommandType = 44 // escape 12
	CmdNeg           CommandType = 46 // escape 14
	CmdEq            CommandType = 47 // escape 15
	CmdCallothersubr CommandType = 48 // escape 16
	CmdPop           CommandType = 49 // escape 17
	CmdDrop          CommandType = 50 // escape 18
	CmdPut           CommandType = 52 // escape 20 (Type2/CFF extension)
	CmdGet           CommandType = 53 // escape 21 (Type2/CFF extension)
	CmdIfelse        CommandType = 54 // escape 22 (Type2/CFF extension)
	CmdRandom        CommandType = 55 // escape 23 (Type2/CFF extension)
	CmdMul           CommandType = 56 // escape 24 (Type2/CFF extension)
	CmdDiv2          CommandType = 57 // escape 25 (Type2/CFF extension)
	CmdDup           CommandType = 59 // escape 27
	CmdExch          CommandType = 60 // escape 28
	CmdHStem4        CommandType = 61 // escape 29
	CmdVStem4        CommandType = 62 // escape 30
	CmdRRoll         CommandType = 63 // escape 31
	CmdHFlex         CommandType = 64 // escape 32
	CmdFlex          CommandType = 65 // escape 33/setcurrentpoint
	CmdHFlex1        CommandType = 66 // escape 34 (Type2/CFF extension)
	CmdFlex1         CommandType = 67 // escape 35 (Type2/CFF extension)
	CmdHsbw          CommandType = 13
)

var CommandTypeNames = map[CommandType]string{
	CmdHStem:      "hstem",
	CmdVStem:      "vstem",
	CmdVMoveto:    "vmoveto",
	CmdRLineto:    "rlineto",
	CmdHlineto:    "hlineto",
	CmdVlineto:    "vlineto",
	CmdRRCurveto:  "rrcurveto",
	CmdClosePath:  "closepath",
	CmdEndChar:    "endchar",
	CmdHStemHM:    "hstemhm",
	CmdHintmask:   "hintmask",
	CmdCntrmask:   "cntrmask",
	CmdRMoveto:    "rmoveto",
	CmdHMoveto:    "hmoveto",
	CmdVStemHM:    "vstemhm",
	CmdRCurveline: "rcurveline",
	CmdRLinecurve: "rlinecurve",
	CmdVVCurveto:  "vvcurveto",
	CmdHHCurveto:  "hhcurveto",
	CmdCallSubr:   "callsubr",
	CmdReturn:     "return",
	CmdSbw:        "sbw",
	CmdHsbw:       "hsbw",
}

func (t CommandType) String() string {
	if name, ok := CommandTypeNames[t]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", t)
}
