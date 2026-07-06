// Package mcp bridges an MCP server's tool surface into agentsdk's
// action.ToolSource so the runtime can dispatch remote tools the same
// way it dispatches local TypedTools.
//
// The adapter is intentionally thin: it owns a single MCP ClientSession
// (caller-provided transport), converts MCP tool descriptors to
// core.ToolSchema, and forwards CALL_TOOL calls via CallTool. Stdio,
// HTTP, and InMemoryTransports all work — production callers typically
// wire stdio via mcp.NewCommandTransport / http via their own Server.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	mcppkg "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps an MCP ClientSession to implement action.ToolSource.
//
// Construction is two-stage:
//
//	session, err := mcppkg.ClientConnect(ctx, transport, nil)
//	if err != nil { ... }
//	c := mcp.NewClient(session)
//	schemas, err := c.Discover(ctx)
type Client struct {
	session *mcppkg.ClientSession
}

// NewClient returns a Client backed by the given session.
func NewClient(session *mcppkg.ClientSession) *Client {
	return &Client{session: session}
}

// Discover implements action.ToolSource. It calls the MCP ListTools
// RPC and converts the result into core.ToolSchema. The risk level is
// heuristic — MCP does not carry risk metadata, so we default to LOW
// for any tool whose name ends in "_read" or starts with "get_", and
// HIGH for "delete" / "exec" / "shell" prefixes. Production-grade
// detection belongs in an allowlist override (added in M4).
func (c *Client) Discover(ctx context.Context) ([]core.ToolSchema, error) {
	if c.session == nil {
		return nil, fmt.Errorf("mcp: nil session")
	}
	res, err := c.session.ListTools(ctx, &mcppkg.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("mcp: list tools: %w", err)
	}
	out := make([]core.ToolSchema, 0, len(res.Tools))
	for _, t := range res.Tools {
		schema := mcpToolToSchema(t)
		out = append(out, schema)
	}
	return out, nil
}

// Call implements action.ToolSource. It forwards the call to the MCP
// server and returns the resulting content as the ToolResult.Output.
func (c *Client) Call(ctx context.Context, name string, args json.RawMessage) (core.ToolResult, error) {
	if c.session == nil {
		return core.ToolResult{OK: false, Error: "mcp: nil session"}, nil
	}
	// Decode args into map for MCP's loose interface.
	var argMap map[string]any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &argMap); err != nil {
			return core.ToolResult{Name: name, OK: false, Error: "invalid args: " + err.Error()}, nil
		}
	}
	if argMap == nil {
		argMap = map[string]any{}
	}
	res, err := c.session.CallTool(ctx, &mcppkg.CallToolParams{
		Name:      name,
		Arguments: argMap,
	})
	if err != nil {
		return core.ToolResult{Name: name, OK: false, Error: err.Error()}, nil
	}
	if res == nil {
		return core.ToolResult{Name: name, OK: false, Error: "empty result"}, nil
	}
	if res.IsError {
		errStr := "tool error"
		if len(res.Content) > 0 {
			if txt, ok := res.Content[0].(*mcppkg.TextContent); ok {
				errStr = txt.Text
			}
		}
		return core.ToolResult{Name: name, OK: false, Error: errStr}, nil
	}
	// Encode the result content as JSON for the runtime's downstream
	// consumers. When there is exactly one TextContent, we keep its raw
	// text as the Output so message-order is preserved.
	outputs := make([]map[string]any, 0, len(res.Content))
	for _, c := range res.Content {
		if txt, ok := c.(*mcppkg.TextContent); ok {
			outputs = append(outputs, map[string]any{"type": "text", "text": txt.Text})
		} else {
			outputs = append(outputs, map[string]any{"type": "other"})
		}
	}
	encoded, err := json.Marshal(outputs)
	if err != nil {
		return core.ToolResult{Name: name, OK: false, Error: "marshal: " + err.Error()}, nil
	}
	return core.ToolResult{Name: name, OK: true, Output: json.RawMessage(encoded)}, nil
}

// mcpToolToSchema converts an MCP Tool descriptor to our ToolSchema.
func mcpToolToSchema(t *mcppkg.Tool) core.ToolSchema {
	var params any
	if t.InputSchema != nil {
		params = t.InputSchema
	} else {
		params = map[string]any{"type": "object"}
	}
	return core.ToolSchema{
		Name:        t.Name,
		Description: t.Description,
		Risk:        inferRisk(t.Name),
		Parameters:  params,
	}
}

// inferRisk classifies a tool name. Heuristic only — production
// callers should pass an explicit per-tool risk policy.
func inferRisk(name string) core.RiskLevel {
	lowPrefixes := []string{"get_", "read_", "list_", "search_", "find_"}
	highPrefixes := []string{"delete_", "exec_", "shell_", "write_", "post_"}
	for _, p := range lowPrefixes {
		if len(name) >= len(p) && name[:len(p)] == p {
			return core.RISK_LEVEL_LOW
		}
	}
	for _, p := range highPrefixes {
		if len(name) >= len(p) && name[:len(p)] == p {
			return core.RISK_LEVEL_HIGH
		}
	}
	return core.RISK_LEVEL_LOW
}

// Ensure action.ToolSource is implemented at compile time.
var _ action.ToolSource = (*Client)(nil)