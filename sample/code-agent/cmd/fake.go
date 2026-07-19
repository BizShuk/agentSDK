package cmd

import (
	"context"
	"errors"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/internal/testutil"
)

// fakeProvider wraps testutil.ScriptedProvider with an echo fallback so
// interactive --fake sessions stay usable after the script runs dry.
type fakeProvider struct {
	*testutil.ScriptedProvider
}

// newFakeProvider scripts one glob tool-call turn plus a closing answer —
// enough to exercise loop → hook gate → permission → tool → sink end to end.
func newFakeProvider() *fakeProvider {
	p := &fakeProvider{testutil.NewScriptedProvider()}
	p.EnqueueToolCall("fake-1", "glob", map[string]any{"pattern": "*.go"})
	p.EnqueueEndTurn("（fake）glob 看過工作目錄了 — 管線 end-to-end 正常。之後的輸入我會用 echo 回覆。")
	return p
}

// Generate pops the script; when dry, echo the latest user text.
func (f *fakeProvider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	res, err := f.ScriptedProvider.Generate(ctx, req)
	if err == nil {
		return res, nil
	}
	if errors.Is(err, testutil.ErrQueueEmpty) {
		return core.ModelResult{
			StopReason: "end_turn",
			Text:       "fake 回覆：" + lastUserText(req.Messages),
		}, nil
	}
	return res, err
}

func lastUserText(msgs []core.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != core.ROLE_USER {
			continue
		}
		var sb strings.Builder
		for _, p := range msgs[i].Parts {
			if p.Kind == core.PART_KIND_PLAIN_TEXT {
				sb.WriteString(p.Text)
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}
	return "(no user message)"
}
