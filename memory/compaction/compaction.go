package compaction

import (
	"github.com/bizshuk/agentsdk/core"
)

// Compactor reduces a window of messages into a smaller form.
type Compactor interface {
	Compact(msgs []core.Message) (core.Message, error)
}
