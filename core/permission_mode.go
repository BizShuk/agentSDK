package core

// PermissionMode is the claude-code style permission mode. Both the
// config-file vocabulary (spec.Safety.Mode) and the runtime engine
// type (agent/permission.Mode) reduce to this single string type, so
// a typo in one place fails to compile, and `var x Mode = "DEAFULT"`
// (or any other non-canonical case) is a checkable type error.
//
// The constant VALUES are written into user YAML and JSON config
// files; renaming any of them is a breaking change for downstream
// operators. The constants are declared as untyped string literals
// (no PermissionMode type) so existing string fields can read them
// without a cast — the type lives below for any consumer that wants
// compile-time checking, but the literals do not force it.
type PermissionMode string

const (
	// PERMISSION_MODE_DEFAULT — rules first, then the injected
	// Fallback policy (typically the L0–L4 autonomy grid).
	PERMISSION_MODE_DEFAULT = "default"

	// PERMISSION_MODE_ACCEPT_EDITS — low-risk calls are auto-allowed,
	// high-risk still asks.
	PERMISSION_MODE_ACCEPT_EDITS = "acceptEdits"

	// PERMISSION_MODE_PLAN — read-only: low-risk allowed, high-risk
	// denied without surfacing as a PendingApproval.
	PERMISSION_MODE_PLAN = "plan"

	// PERMISSION_MODE_BYPASS — every call allowed. Container / CI
	// only; an operator who pastes this into a config on a laptop is
	// inviting catastrophe, and that is the point — the spelling is
	// memorable on purpose.
	PERMISSION_MODE_BYPASS = "bypassPermissions"
)
