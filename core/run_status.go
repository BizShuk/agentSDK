package core

// RunStatus tracks the lifecycle of a single run.
type RunStatus string

const (
	RUN_STATUS_RUNNING         RunStatus = "running"
	RUN_STATUS_PAUSED_APPROVAL RunStatus = "paused_for_approval"
	RUN_STATUS_COMPLETED       RunStatus = "completed"
	RUN_STATUS_FAILED          RunStatus = "failed"
	RUN_STATUS_ABORTED         RunStatus = "aborted"
)

// Terminal reports whether a status can no longer transition.
func (s RunStatus) Terminal() bool {
	switch s {
	case RUN_STATUS_COMPLETED, RUN_STATUS_FAILED, RUN_STATUS_ABORTED:
		return true
	default:
		return false
	}
}
