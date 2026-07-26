// Command demo-strategy is a catalog sample: it drives each of the six
// reasoning DecisionRule strategies (ReAct / PlanThenRun / DoThenReview /
// OneShot / LearnFromFailure / ChooseAgent) through a deterministic,
// offline FSM trace so the control flow of each is visible side by side.
//
// The reasoning logic is 100% real SDK code — the exact
// reasoning.*.NextStep functions and Seed* helpers the unit tests use.
// Only the environment (model replies, tool results, review verdicts) is
// scripted, so the demo needs no network and no API key. This is the
// "fake the non-relevant parts to show the main topic" path: the main
// topic is the decision-rule control flow, not the LLM.
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/sample/demo-strategy/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
