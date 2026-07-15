// Package cmd hosts the memory-demo CLI and its three demos.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/memory"
	"github.com/bizshuk/agentsdk/memory/checkpoint"
	"github.com/bizshuk/agentsdk/memory/filestore"
)

// demo 描述一個可執行的記憶體子元件示範。
type demo struct {
	id    string
	title string
	blurb string
	run   func(w io.Writer) error
}

// demos 回傳 catalog,順序即展示順序。
func demos() []demo {
	return []demo{
		{
			id:    "window",
			title: "Window — 有界上下文 (bounded context)",
			blurb: "Window.Trim 以 MaxMessages / MaxTokens 保留最近的訊息;token 用 CharHeuristicCounter (chars/4)。",
			run:   demoWindow,
		},
		{
			id:    "compact",
			title: "HeadlineCompactor — 無 LLM 壓縮",
			blurb: "把一段 window 濃縮成單一 assistant 摘要(每則取首行),決定性、不打網路。",
			run:   demoCompact,
		},
		{
			id:    "checkpoint",
			title: "Recoverer — 快照 + WAL 重放",
			blurb: "StateStore 存快照、WriteAheadLog 逐事件 append;Recover 載回 State 並只回放 Seq > LastInputSeq 的事件(不重呼叫 LLM)。",
			run:   demoCheckpoint,
		},
	}
}

// --- window --------------------------------------------------------------

func demoWindow(w io.Writer) error {
	msgs := sampleTranscript()
	counter := memory.CharHeuristicCounter{}

	fmt.Fprintf(w, "原始 transcript(%d 則,%d tokens):\n", len(msgs), counter.Count(msgs))
	printMessages(w, msgs)

	byCount := memory.Window{MaxMessages: 3}
	trimmed := byCount.Trim(msgs)
	fmt.Fprintf(w, "\nWindow{MaxMessages:3} → 保留最近 %d 則(%d tokens):\n", len(trimmed), counter.Count(trimmed))
	printMessages(w, trimmed)

	byToken := memory.Window{MaxTokens: 30, Counter: counter}
	trimmed2 := byToken.Trim(msgs)
	fmt.Fprintf(w, "\nWindow{MaxTokens:30} → 從最舊丟到塞得下(%d 則,%d tokens):\n", len(trimmed2), counter.Count(trimmed2))
	printMessages(w, trimmed2)
	return nil
}

// --- compact -------------------------------------------------------------

func demoCompact(w io.Writer) error {
	msgs := sampleTranscript()
	fmt.Fprintf(w, "壓縮前(%d 則):\n", len(msgs))
	printMessages(w, msgs)

	summary, err := memory.HeadlineCompactor{}.Compact(msgs)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\n壓縮後(1 則,role=%s):\n", summary.Role)
	printMessages(w, []core.Message{summary})
	return nil
}

// --- checkpoint ----------------------------------------------------------

func demoCheckpoint(w io.Writer) error {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "memory-demo-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	fmt.Fprintf(w, "臨時儲存目錄:%s\n", dir)

	store, err := filestore.NewJSONFileStateStore(dir)
	if err != nil {
		return err
	}
	log, err := filestore.NewJSONLFileLog(dir)
	if err != nil {
		return err
	}
	rec := checkpoint.NewRecoverer(store, log)

	const runID = "run-42"
	// 模擬:跑到第 1 個事件已被折進 State,快照落地。
	snap := core.State{
		RunID:        runID,
		Status:       core.RUN_STATUS_RUNNING,
		LastInputSeq: 1,
		Messages: []core.Message{
			userMsg("Tail the log and summarize errors."),
			assistantMsg("Reading the last 20 lines…"),
		},
		WorkingMemory: map[string]any{"think_then_act.phase": "reflect"},
	}
	if err := rec.Save(ctx, snap); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n1) Save 快照:runID=%s LastInputSeq=%d 訊息=%d phase=%v\n",
		snap.RunID, snap.LastInputSeq, len(snap.Messages), snap.WorkingMemory["think_then_act.phase"])

	// WAL 事件:seq=1 已在快照內(不該被重放),seq=2/3 是崩潰前尚未折入 State 的事件。
	events := []core.Event{
		{Kind: core.EVENT_TOOL_RESULT, Seq: 1, ToolResult: &core.ToolResult{CallID: "c1", Name: "read_log_tail", OK: true}},
		{Kind: core.EVENT_MODEL_REPLY, Seq: 2, ModelResult: &core.ModelResult{StopReason: "tool_use", ToolCalls: []core.ToolCall{{ID: "c2", Name: "notify"}}}},
		{Kind: core.EVENT_TOOL_RESULT, Seq: 3, ToolResult: &core.ToolResult{CallID: "c2", Name: "notify", OK: true}},
	}
	for _, ev := range events {
		if err := log.Append(ctx, runID, ev.Seq, ev); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "2) WAL append 3 個事件(seq 1..3);seq=1 已在快照內。\n")

	// 崩潰 → 重開機 → Recover。
	run, err := rec.Recover(ctx, runID)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\n3) Recover:\n")
	fmt.Fprintf(w, "   載回 State:runID=%s LastInputSeq=%d 訊息=%d phase=%v\n",
		run.State.RunID, run.State.LastInputSeq, len(run.State.Messages), run.State.WorkingMemory["think_then_act.phase"])
	fmt.Fprintf(w, "   只回放 Seq > %d 的事件(%d 個,LLM 不被重呼叫):\n", run.State.LastInputSeq, len(run.Events))
	for _, ev := range run.Events {
		fmt.Fprintf(w, "     - seq=%d kind=%s\n", ev.Seq, ev.Kind)
	}
	fmt.Fprintf(w, "   → 重放結果已含在 WAL(ModelResult/ToolResult),caller 只需重新 fold,不再打 model。\n")
	return nil
}

// --- shared helpers ------------------------------------------------------

// sampleTranscript 是所有 demo 共用的假 transcript。
func sampleTranscript() []core.Message {
	return []core.Message{
		userMsg("Please watch the application log and alert me on errors."),
		assistantMsg("Sure — I'll tail the log now.\n(internal: planning to call read_log_tail)"),
		userMsg("Focus on the payment service."),
		assistantMsg("Tailing payment.log; found 2 ERROR lines about a timeout."),
		userMsg("What caused the timeout?"),
		assistantMsg("Root cause: the DB connection pool was exhausted under load."),
	}
}

func userMsg(text string) core.Message {
	return core.Message{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}}, Ts: time.Now().UTC()}
}

func assistantMsg(text string) core.Message {
	return core.Message{Role: core.ROLE_ASSISTANT, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}}, Ts: time.Now().UTC()}
}

// printMessages 印出每則 role + 首行預覽(截斷)。
func printMessages(w io.Writer, msgs []core.Message) {
	for i, m := range msgs {
		fmt.Fprintf(w, "  [%d] %-9s %s\n", i, m.Role, preview(text(m)))
	}
}

func text(m core.Message) string {
	for _, p := range m.Parts {
		if p.Kind == core.PART_KIND_PLAIN_TEXT {
			return p.Text
		}
	}
	return ""
}

func preview(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx] + " …"
	}
	const max = 60
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}