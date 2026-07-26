package cmd

import (
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/planning"
)

// strategies 回傳六條策略的 catalog,順序即展示順序。
//
// 每一條的 rule / seed 都是真實 SDK code(與 planning 單元測試相同的
// 進入點),fold 只腳本化「環境」的回饋。
func strategies() []strategy {
	return []strategy{
		reactStrategy(),
		planThenRunStrategy(),
		doThenReviewStrategy(),
		oneShotStrategy(),
		learnFromFailureStrategy(),
		chooseAgentStrategy(),
	}
}

// react — Reason + Act(Yao 2023)。think → act(tool) → reflect,
// 由環境的 end_turn 收尾,對應 runtime 的 short-circuit。
func reactStrategy() strategy {
	return strategy{
		id:        "react",
		title:     "ThinkThenAct (ReAct)",
		style:     core.REASON_REACT,
		blurb:     "推理→挑工具→觀察結果→再推理;由 model 的 end_turn 結束(runtime 短路)。",
		phaseKey:  planning.THINK_THEN_ACT_PHASE,
		initPhase: planning.THINK_THEN_ACT_REASON,
		prompt:    "Watch the log and tell me if there are errors.",
		rule:      planning.NewThinkThenAct(),
		fold: func(s *core.State, step int) bool {
			switch step {
			case 1: // model 回 tool_use → runtime 把 pending_call 播進 scratch
				scratchPut(s, planning.THINK_THEN_ACT_PENDING_CALL, core.ToolCall{
					ID: "c1", Name: "read_log_tail",
					Args: map[string]any{"n": 20}, Risk: core.RISK_LEVEL_LOW,
				})
			case 2: // 工具執行完 → 折回 tool result
				appendToolResult(s, "c1", `{"lines":20,"errors":2}`)
			case 3: // model 回 end_turn → 環境結束回合
				appendAssistant(s, "log has 2 ERROR lines; operator notified")
				return true
			}
			return false
		},
	}
}

// plan_then_run — 先要 blueprint,再逐一 dispatch。這裡直接以 SeedBlueprint
// 播入計畫(production 會由 plan phase 的 model 回覆解碼而來),之後全程
// 不需要 model:三個 tool call 依序執行到 blueprint 用盡即 DONE。
func planThenRunStrategy() strategy {
	st := strategy{
		id:        "plan",
		title:     "PlanThenRun",
		style:     core.REASON_PLAN_THEN_RUN,
		blurb:     "先產出 blueprint(此處 seed 三步),再依序 dispatch,用盡即 DONE。",
		phaseKey:  planning.PLAN_THEN_RUN_PHASE,
		initPhase: planning.RUN_PHASE_PTR,
		prompt:    "Triage the incident: read log, open ticket, page on-call.",
		rule:      planning.NewPlanThenRun(),
		seed: func(s *core.State) {
			planning.SeedBlueprint(s, []core.ToolCall{
				{ID: "s1", Name: "read_log_tail", Args: map[string]any{"n": 50}},
				{ID: "s2", Name: "open_ticket", Args: map[string]any{"severity": "high"}},
				{ID: "s3", Name: "page_oncall", Args: map[string]any{"team": "infra"}},
			})
		},
	}
	return st
}

// do_then_review — Self-Refine(Welleck 2023)。execute → critique,
// 評語未過就 iterate。環境先給 FAIL 一次,再給 OK 收尾。
func doThenReviewStrategy() strategy {
	return strategy{
		id:        "review",
		title:     "DoThenReview (Self-Refine)",
		style:     core.REASON_DO_THEN_REVIEW,
		blurb:     "執行→自我批判;評語非 'OK:' 就重來一輪。環境先 FAIL、再 PASS。",
		phaseKey:  planning.RUN_THEN_REVIEW_PHASE,
		initPhase: planning.RUN_PHASE,
		prompt:    "Draft a fix for the null-pointer crash.",
		rule:      planning.NewRunThenReview(),
		fold: func(s *core.State, step int) bool {
			switch step {
			case 1: // 第一次 execute 後,reviewer 給 FAIL
				scratchPut(s, planning.RUN_THEN_REVIEW_NOTE, "missing null-check on line 42")
			case 3: // 第二次 execute 後,reviewer 給 PASS
				scratchPut(s, planning.RUN_THEN_REVIEW_NOTE, "OK: null-check added, all tests green")
			}
			return false
		},
	}
}

// one_shot — 單發 CoT(Wei 2022)。think 一次即 done。
func oneShotStrategy() strategy {
	return strategy{
		id:        "oneshot",
		title:     "OneShotReasoning (Chain-of-Thought)",
		style:     core.REASON_ONE_SHOT,
		blurb:     "一次推理就結束的雙相 FSM:think → done。",
		phaseKey:  planning.ONE_SHOT_PHASE,
		initPhase: planning.ONE_SHOT_THINK,
		prompt:    "What is 17 * 23? Show your reasoning.",
		rule:      planning.NewOneShotReasoning(),
		fold: func(s *core.State, step int) bool {
			if step == 1 {
				appendAssistant(s, "17*23 = 17*20 + 17*3 = 340 + 51 = 391")
			}
			return false
		},
	}
}

// learn_from_failure — Reflexion(Shinn 2023)。act → reflect → retry;
// 批判失敗就把反思累積進 scratch 再重試,直到批判以 "OK:" 開頭。
func learnFromFailureStrategy() strategy {
	return strategy{
		id:        "reflexion",
		title:     "LearnFromFailure (Reflexion)",
		style:     core.REASON_LEARN_FROM_FAILURE,
		blurb:     "act→reflect→retry;失敗評語累積成 verbal reinforcement,直到評語 'OK:'。",
		phaseKey:  planning.LEARN_FROM_FAILURE_PHASE,
		initPhase: planning.LFF_ACT,
		prompt:    "Write a regex that matches all ERROR log lines.",
		rule:      planning.NewLearnFromFailure(),
		fold: func(s *core.State, step int) bool {
			switch step {
			case 1: // act 後 → 第一次嘗試
				appendAssistant(s, "attempt: /err/")
			case 2: // reflect 後 → 批判(FAIL,無 OK: 前綴)
				appendAssistant(s, "critique: /err/ misses uppercase ERROR")
			case 3: // retry 之後 → 批判(PASS)
				appendAssistant(s, "OK: /(?i)error/ now matches every ERROR line")
			}
			return false
		},
	}
}

// choose_agent — Router / Orchestrator。從 agent 清單選一個,delegate,done。
func chooseAgentStrategy() strategy {
	return strategy{
		id:        "router",
		title:     "ChooseAgent (Router)",
		style:     core.REASON_PICK_AGENT,
		blurb:     "從 agent 清單挑一個(此處 seed 兩個)→ 以該 agent 身分 delegate → done。",
		phaseKey:  planning.CHOOSE_AGENT_PHASE,
		initPhase: planning.CA_SELECT,
		prompt:    "The service is down — figure out why.",
		rule:      planning.NewChooseAgent(),
		seed: func(s *core.State) {
			planning.SeedAgents(s, []string{"log-analyst", "patch-writer"})
		},
		fold: func(s *core.State, step int) bool {
			if step == 2 { // delegate 後 → 被選中的 agent 回覆
				appendAssistant(s, "log-analyst: root cause is an unclosed file handle")
			}
			return false
		},
	}
}
