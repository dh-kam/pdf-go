package renderer

import "github.com/dh-kam/pdf-go/internal/domain/entity"

type fontCandidateResolver interface {
	ResolveCandidate(evaluator *Evaluator, dict *entity.Dict, subtype, baseFont string, embeddedFontData []byte, embeddedErr error) entity.Font
}
