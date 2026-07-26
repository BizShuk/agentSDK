package tool

import (
	"context"

	"github.com/bizshuk/agentsdk/core"
)

// Tool represents a strongly-typed tool with metadata and a generic Handle method.
type Tool[TArgs any, TOut any] interface {
	Name() string
	Risk() core.RiskLevel
	Desc() string
	Handle(ctx context.Context, args TArgs) (TOut, error)
}
