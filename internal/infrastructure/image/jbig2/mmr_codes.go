package jbig2

type mmrRunCode struct {
	bits  uint16
	width uint8
	run   int
}

var whiteTerminatingRunCodes = []mmrRunCode{
	{bits: 0b00110101, width: 8, run: 0},
	{bits: 0b000111, width: 6, run: 1},
	{bits: 0b0111, width: 4, run: 2},
	{bits: 0b1000, width: 4, run: 3},
	{bits: 0b1011, width: 4, run: 4},
	{bits: 0b10011, width: 5, run: 8},
}

var blackTerminatingRunCodes = []mmrRunCode{
	{bits: 0b0000110111, width: 10, run: 0},
	{bits: 0b011, width: 3, run: 4},
	{bits: 0b0011, width: 4, run: 5},
	{bits: 0b0010, width: 4, run: 6},
	{bits: 0b00011, width: 5, run: 7},
	{bits: 0b000101, width: 6, run: 8},
}
