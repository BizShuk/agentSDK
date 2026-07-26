package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
)

// Tool is the canonical executable tool contract from core.
//
// The alias keeps tool implementations readable without introducing a second,
// structurally similar interface that can drift from core.Tool.
type Tool = core.Tool

// CallWithRawMessage converts raw JSON arguments to TArgs, invokes call, and
// converts its typed output into a core.ToolResult.
func CallWithRawMessage[TArgs any, TOut any](
	ctx context.Context,
	name string,
	raw json.RawMessage,
	call func(context.Context, TArgs) (TOut, error),
) (core.ToolResult, error) {
	if ok, err := ValidateArgs[TArgs](name, raw); !ok {
		return core.ToolResult{Name: name, OK: false, Error: err.Error()}, nil
	}

	var args TArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return core.ToolResult{
				Name:  name,
				OK:    false,
				Error: "invalid args: " + err.Error(),
			}, nil
		}
	}

	out, err := call(ctx, args)
	if err != nil {
		return core.ToolResult{Name: name, OK: false, Error: err.Error()}, nil
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return core.ToolResult{Name: name, OK: true, Output: fmt.Sprintf("%v", out)}, nil
	}
	return core.ToolResult{Name: name, OK: true, Output: json.RawMessage(encoded)}, nil
}
