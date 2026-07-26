package tool

import (
	"context"
	"fmt"
	"time"

	sdkcore "github.com/bizshuk/agentsdk/core"
	sdktool "github.com/bizshuk/agentsdk/tool"
)

// ProposeFixArgs argument shape.
type ProposeFixArgs struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ProposeFixOutput output shape.
type ProposeFixOutput struct {
	ProposalID string `json:"proposal_id"`
	CreatedAt  string `json:"created_at"`
}

// ProposeFix is a HIGH-RISK tool.
type ProposeFix struct{}

// NewProposeFix constructs a ProposeFix tool.
func NewProposeFix() *ProposeFix {
	return &ProposeFix{}
}

// Register registers ProposeFix into the given sdktool.Registry.
func (p *ProposeFix) Register(reg *sdktool.Registry) {
	sdktool.RegisterFunc(reg, "propose_fix", "Propose a fix for the operator to approve", sdkcore.RISK_LEVEL_HIGH, p.Handle)
}

// Handle is pure business logic.
func (p *ProposeFix) Handle(_ context.Context, args ProposeFixArgs) (ProposeFixOutput, error) {
	if args.Title == "" {
		return ProposeFixOutput{}, fmt.Errorf("propose_fix: title is required")
	}
	return ProposeFixOutput{
		ProposalID: fmt.Sprintf("prop-%d", time.Now().UnixNano()),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}
