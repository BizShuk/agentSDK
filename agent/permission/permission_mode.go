package permission

// Mode is the claude-code style permission mode.
type Mode string

const (
	// MODE_DEFAULT applies rules first, then the injected
	// Fallback policy (typically the L0–L4 autonomy grid).
	MODE_DEFAULT Mode = "default"

	// MODE_ACCEPT_EDITS auto-allows low-risk calls; high-risk calls still ask.
	MODE_ACCEPT_EDITS Mode = "acceptEdits"

	// MODE_PLAN is read-only: low-risk calls are allowed and high-risk calls
	// are denied without surfacing as a PendingApproval.
	MODE_PLAN Mode = "plan"

	// MODE_BYPASS allows every call. Container / CI
	// only; an operator who pastes this into a config on a laptop is
	// inviting catastrophe, and that is the point — the spelling is
	// memorable on purpose.
	MODE_BYPASS Mode = "bypassPermissions"
)
