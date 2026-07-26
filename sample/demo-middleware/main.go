// Command demo-middleware is a catalog sample for the agentsdk M2
// middleware chain: retry, budget guard, per-effect timeout, and the
// loopguard that rewrites a stuck CALL_TOOL into REQUEST_APPROVAL.
//
// Each demo wraps a tiny scripted base dispatcher (the terminal Next) so
// the middleware behaviour is visible in isolation — offline, deterministic,
// no provider, no real sleeping.
package main

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/sample/demo-middleware/cmd"
)

func main() {
	if err := cmd.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}