// Package cmd hosts the demo-strategy CLI and the FSM tracer.
package cmd

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/reasoning"
)

// MAX_TRACE_STEPS 是任一 strategy 的步數上限,做為 FSM 卡死的護欄
// (真正的 rule 都在 4 步內收斂到 DONE)。
const MAX_TRACE_STEPS = 12

// strategy 描述一條可被追蹤的 DecisionRule。
//
// rule / seed 全部走真實 SDK code;fold 才是被腳本化的「環境」——
// 它模擬 runtime 對 emitted instruction 的回饋(model 回 tool_use、
// tool 回結果、reviewer 給評語…),讓純函式的 NextStep 能推進。
type strategy struct {
	id        string                 // CLI 識別字,如 "react"
	title     string                 // 人類可讀標題
	style     string                 // 對應 State.ReasoningStyle 的字串值
	blurb     string                 // 一行說明
	phaseKey  string                 // 存 phase 的 scratch key(顯示用)
	initPhase string                 // phase scratch 未設定時的預設顯示值
	prompt    string                 // 開場 user message
	rule      reasoning.DecisionRule // 被測的規則
	seed      func(*core.State)      // 初始 scratch 播種(可為 nil)
	// fold 模擬環境對第 step 步 instruction 的回應,回傳 true 代表環境
	// 結束了這一回合(等同 runtime 收到 end_turn 的 COMPLETED 短路)。
	fold func(s *core.State, step int) bool
}

// messages 產生開場 user 訊息。
func (st strategy) messages() []core.Message {
	return []core.Message{{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: st.prompt}},
		Ts:    time.Now().UTC(),
	}}
}

// traceStrategy 逐步驟驅動 st.rule.NextStep,印出 phase 轉移與 emitted
// instruction。全程離線、決定性,不打任何網路。
func traceStrategy(w io.Writer, st strategy) {
	fmt.Fprintf(w, "\n=== %s  [%s] ===\n", st.title, st.style)
	fmt.Fprintf(w, "%s\n", st.blurb)
	fmt.Fprintf(w, "prompt: %q\n\n", st.prompt)

	s := core.State{ReasoningStyle: st.style, Messages: st.messages()}
	if st.seed != nil {
		st.seed(&s)
	}

	for step := 1; step <= MAX_TRACE_STEPS; step++ {
		phase := scratchStr(s, st.phaseKey, st.initPhase)
		next, instrs := st.rule.NextStep(s)
		s = next

		done := false
		for _, inst := range instrs {
			fmt.Fprintf(w, "  step %d  [%-8s] -> %-16s %s\n",
				step, phase, inst.Kind, summarize(inst))
			if inst.Kind == core.INSTRUCTION_DONE {
				done = true
			}
		}
		if done {
			fmt.Fprintf(w, "  DONE — rule reached terminal state in %d step(s)\n", step)
			return
		}
		if st.fold != nil && st.fold(&s, step) {
			fmt.Fprintf(w, "  DONE — environment ended the turn after step %d (end_turn short-circuit)\n", step)
			return
		}
	}
	fmt.Fprintf(w, "  ! did not reach DONE within %d steps\n", MAX_TRACE_STEPS)
}

// summarize 把一個 instruction 濃縮成一行可讀摘要。
func summarize(inst core.Instruction) string {
	switch inst.Kind {
	case core.INSTRUCTION_CALL_MODEL:
		if inst.CallModel == nil {
			return ""
		}
		msgs := inst.CallModel.Messages
		note := fmt.Sprintf("%d msg(s)", len(msgs))
		if len(msgs) > 0 && msgs[0].Role == core.ROLE_SYSTEM {
			note += " (system-prefixed)"
		}
		return note
	case core.INSTRUCTION_CALL_TOOL:
		if inst.CallTool == nil {
			return ""
		}
		c := inst.CallTool.Call
		return fmt.Sprintf("%s(%s)", c.Name, argsString(c.Args))
	case core.INSTRUCTION_NOTIFY:
		if inst.Notify == nil {
			return ""
		}
		return fmt.Sprintf("[%s] %s", inst.Notify.Level, inst.Notify.Message)
	default:
		return ""
	}
}

// argsString 把 tool args map 攤平成 k=v 逗號串(排序無關,demo 用)。
func argsString(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, ",")
}

// --- demo-local scratch / message helpers -----------------------------------
// 這些是 demo 端模擬「環境」的最小工具,不觸碰 reasoning 的 unexported helper。

func scratchStr(s core.State, key, def string) string {
	if key == "" || s.WorkingMemory == nil {
		return def
	}
	if v, ok := s.WorkingMemory[key].(string); ok {
		return v
	}
	return def
}

func scratchPut(s *core.State, key string, val any) {
	if s.WorkingMemory == nil {
		s.WorkingMemory = make(map[string]any, 4)
	}
	s.WorkingMemory[key] = val
}

func appendAssistant(s *core.State, text string) {
	s.Messages = append(s.Messages, core.Message{
		Role:  core.ROLE_ASSISTANT,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}},
		Ts:    time.Now().UTC(),
	})
}

func appendToolResult(s *core.State, callID, output string) {
	s.Messages = append(s.Messages, core.Message{
		Role: core.ROLE_TOOL,
		Parts: []core.Part{{
			Kind:       core.PART_KIND_TOOL_RESULT,
			ToolResult: &core.ToolResult{CallID: callID, OK: true, Output: output},
		}},
		Ts: time.Now().UTC(),
	})
}
