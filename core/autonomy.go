package core

// AutonomyLevel is the graduated trust level for tool execution and mutation.
// L0 — fully manual (every action requires human approval)
// L1 — low-risk automatic, higher-risk gated (enterprise floor)
// L2 — most automatic, high-risk gated (default cap)
// L3 — minimal gating
// L4 — fully autonomous
type AutonomyLevel int

const (
	AUTONOMY_L0 AutonomyLevel = iota
	AUTONOMY_L1
	AUTONOMY_L2
	AUTONOMY_L3
	AUTONOMY_L4
)

// AUTONOMY_DEFAULT is the typed runtime default. AUTONOMY_DEFAULT_STRING is
// the matching config-file vocabulary; the cross-package contract test keeps
// them aligned.
const (
	AUTONOMY_DEFAULT        = AUTONOMY_L2
	AUTONOMY_DEFAULT_STRING = "L2"
)

// String renders the level in its canonical config form.
func (a AutonomyLevel) String() string {
	switch a {
	case AUTONOMY_L0:
		return "L0"
	case AUTONOMY_L1:
		return "L1"
	case AUTONOMY_L2:
		return "L2"
	case AUTONOMY_L3:
		return "L3"
	case AUTONOMY_L4:
		return "L4"
	default:
		return "L?"
	}
}
