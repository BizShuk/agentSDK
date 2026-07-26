package tool

import (
	"context"
	"encoding/json"
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

var _ sdktool.Tool = (*ProposeFix)(nil)

// Register registers ProposeFix into the given sdktool.Registry.
func (p *ProposeFix) Register(reg *sdktool.Registry) {
	reg.Register(p)
}

// Name returns the registry name.
func (p *ProposeFix) Name() string { return "propose_fix" }

// Spec returns metadata and the reflected argument schema.
func (p *ProposeFix) Spec() sdkcore.ToolSpec {
	return sdktool.MustSchemaForTool[ProposeFixArgs](
		p.Name(),
		"Propose a fix for the operator to approve",
		sdkcore.RISK_LEVEL_HIGH,
	)
}

// Call converts raw JSON arguments and executes the proposal operation.
func (p *ProposeFix) Call(
	ctx context.Context,
	raw json.RawMessage,
) (sdkcore.ToolResult, error) {
	return sdktool.CallWithRawMessage(ctx, p.Name(), raw, p.execute)
}

func (p *ProposeFix) execute(
	_ context.Context,
	args ProposeFixArgs,
) (ProposeFixOutput, error) {
	if args.Title == "" {
		return ProposeFixOutput{}, fmt.Errorf("propose_fix: title is required")
	}
	return ProposeFixOutput{
		ProposalID: fmt.Sprintf("prop-%d", time.Now().UnixNano()),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}
