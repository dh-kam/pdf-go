// Package content provides content stream processing infrastructure for PDF rendering.
package content

import (
	"github.com/dh-kam/pdf-go/internal/domain/content"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// NewStandardEvaluator creates a new content stream evaluator with all standard operators registered.
func NewStandardEvaluator(xref entity.XRef) *content.Evaluator {
	ev := content.NewEvaluator(xref)

	// Get the registry and register all standard operators
	registry := ev.GetRegistry()
	registerStandardOperators(registry)

	return ev
}

// registerStandardOperators registers all PDF graphics operators.
func registerStandardOperators(registry *content.OperatorRegistry) {
	// Graphics state operators
	registry.Register("q", &SaveOperator{})
	registry.Register("Q", &RestoreOperator{})
	registry.Register("cm", &ConcatMatrixOperator{})

	// Path construction operators
	registry.Register("m", &MoveToOperator{})
	registry.Register("l", &LineToOperator{})
	registry.Register("c", &CurveToOperator{})
	registry.Register("v", &CurveToNoFirstControlOperator{})
	registry.Register("y", &CurveToNoLastControlOperator{})
	registry.Register("h", &ClosePathOperator{})
	registry.Register("re", &RectangleOperator{})

	// Path painting operators
	registry.Register("S", &StrokeOperator{})
	registry.Register("s", &CloseAndStrokeOperator{})
	registry.Register("f", &FillOperator{})
	registry.Register("F", &FillOperator{})
	registry.Register("f*", &EOFillOperator{})
	registry.Register("B", &FillAndStrokeOperator{})
	registry.Register("B*", &EOFillAndStrokeOperator{})
	registry.Register("b", &CloseFillAndStrokeOperator{})
	registry.Register("b*", &EOCloseFillAndStrokeOperator{})
	registry.Register("n", &EndPathOperator{})

	// Clipping operators
	registry.Register("W", &ClipOperator{})
	registry.Register("W*", &EOClipOperator{})

	// Text object operators
	registry.Register("BT", &BeginTextOperator{})
	registry.Register("ET", &EndTextOperator{})

	// Text state operators
	registry.Register("Tc", &SetCharSpacingOperator{})
	registry.Register("Tw", &SetWordSpacingOperator{})
	registry.Register("Tz", &SetHorizontalScalingOperator{})
	registry.Register("TL", &SetTextLeadingOperator{})
	registry.Register("Tf", &SetFontOperator{})
	registry.Register("Tr", &SetTextRenderModeOperator{})
	registry.Register("Ts", &SetTextRiseOperator{})

	// Text positioning operators
	registry.Register("Td", &MoveTextOperator{})
	registry.Register("TD", &MoveTextSetLeadingOperator{})
	registry.Register("Tm", &SetTextMatrixOperator{})
	registry.Register("T*", &MoveTextNextLineOperator{})
	registry.Register("Tj", &ShowTextOperator{})
	registry.Register("TJ", &ShowTextArrayOperator{})

	// Color operators
	registry.Register("CS", &SetStrokeColorSpaceOperator{})
	registry.Register("cs", &SetFillColorSpaceOperator{})
	registry.Register("SC", &SetStrokeColorOperator{})
	registry.Register("SCN", &SetStrokeColorNOperator{})
	registry.Register("scn", &SetFillColorNOperator{})
	registry.Register("G", &SetGrayStrokeOperator{})
	registry.Register("g", &SetGrayFillOperator{})
	registry.Register("RG", &SetRGBStrokeOperator{})
	registry.Register("rg", &SetRGBFillOperator{})
	registry.Register("K", &SetCMYKStrokeOperator{})
	registry.Register("k", &SetCMYKFillOperator{})

	// XObject operators
	registry.Register("Do", &ExecuteXObjectOperator{})

	// Shading operators
	registry.Register("sh", &ShadingOperator{})

	// Compatibility operators
	registry.Register("BX", &BeginCompatibilityOperator{})
	registry.Register("EX", &EndCompatibilityOperator{})
}
