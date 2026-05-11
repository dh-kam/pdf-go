package renderer

import "github.com/dh-kam/pdf-go/internal/domain/entity"

// FormOperatorCache stores parsed Form XObject operators for reuse.
type FormOperatorCache interface {
	Get(xobj *entity.Stream) ([]Operator, bool)
	Set(xobj *entity.Stream, ops []Operator)
}
