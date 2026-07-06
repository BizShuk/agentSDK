package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
)

// ProposeFixArgs — TypedTool argument shape.
type ProposeFixArgs struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ProposeFixOutput — TypedTool output shape.
type ProposeFixOutput struct {
	ProposalID string `json:"proposal_id"`
	CreatedAt  string `json:"created_at"`
}

// ProposeFix is a HIGH-RISK TypedTool — when invoked, the runtime's
// ApprovalGate (L2 default) will turn it into REQUEST_APPROVAL and
// pause the run until an operator approves.
type ProposeFix struct {
	Inner *action.TypedTool[ProposeFixArgs, ProposeFixOutput]
}

// NewProposeFix wires the HIGH-RISK TypedTool.
func NewProposeFix() *ProposeFix {
	t := action.NewTypedTool("propose_fix", "Propose a fix for the operator to approve",
		func(_ context.Context, a ProposeFixArgs) (ProposeFixOutput, error) {
			if a.Title == "" {
				return ProposeFixOutput{}, fmt.Errorf("propose_fix: title is required")
			}
			return ProposeFixOutput{
				ProposalID: fmt.Sprintf("prop-%d", time.Now().UnixNano()),
				CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			}, nil
		})
	t.SetRisk(sdkcore.RISK_LEVEL_HIGH)
	return &ProposeFix{Inner: t}
}

func (p *ProposeFix) Name() string              { return p.Inner.Name() }
func (p *ProposeFix) Description() string       { return p.Inner.Description() }
func (p *ProposeFix) Schema() sdkcore.ToolSchema { return p.Inner.Schema() }
func (p *ProposeFix) Risk() sdkcore.RiskLevel    { return p.Inner.Risk() }
func (p *ProposeFix) Call(ctx context.Context, args []byte) (sdkcore.ToolResult, error) {
	return p.Inner.Call(ctx, args)
}