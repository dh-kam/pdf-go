// Package splash is a Go port of the Poppler 24.02 Splash rasterizer
// (/tmp/poppler-24.02.0/splash/) targeting pixel-exact parity with
// poppler's pdftoppm output; the package is internal to go-pdf and is
// consumed via internal/infrastructure/canvas as an alternative backend
// to ImageCanvas while the migration to a Splash-faithful renderer
// proceeds (see tmp/splash_port_design/04_phase_plan.md, Phase 0).
package splash
