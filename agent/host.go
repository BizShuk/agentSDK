package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/memory/filestore"
	gosdkconfig "github.com/bizshuk/gosdk/config"
)

var (
	errNilEngine = errors.New("agent: engine must not be nil")
	errNoHost    = errors.New("agent: host is required")
)

// Host contains run identity, paths, logging, and persistence ports.
type Host struct {
	DataDir    string             // ~/.config/<name>/data
	LogDir     string             // ~/.config/<name>/logs
	RunID      string             // caller-assigned; empty until the host chooses to seed one
	LogFile    string             // empty when no log file is wired (a Host built by Open)
	Logger     *slog.Logger       // optional; nil when no log handler has been installed
	StateStore core.StateStore    // file-backed StateStore, ready to use
	WAL        core.WriteAheadLog // file-backed WriteAheadLog, ready to use
}

// Open binds the conventional app directories and file-backed persistence.
// Process logging and signal handling remain in agent/cli.
func Open(appName string) (*Host, error) {
	if appName == "" {
		return nil, fmt.Errorf("agent: appName must not be empty")
	}
	gosdkconfig.Default(gosdkconfig.WithAppName(appName))
	if gosdkconfig.GetAppName() == "" {
		return nil, fmt.Errorf("agent: gosdk/config not initialised (call config.Default(config.WithAppName(%q)))", appName)
	}
	dataDir := gosdkconfig.GetAppDataDir()

	store, err := filestore.NewJSONFileStateStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("agent: state store: %w", err)
	}
	wal, err := filestore.NewJSONLFileLog(dataDir)
	if err != nil {
		return nil, fmt.Errorf("agent: wal: %w", err)
	}
	return &Host{
		DataDir:    dataDir,
		LogDir:     gosdkconfig.GetAppLogsDir(),
		StateStore: store,
		WAL:        wal,
	}, nil
}

// Perform runs an engine after filling unset persistence and run identity
// from host.
func Perform(ctx context.Context, host *Host, engine *Engine, state core.State) (core.State, error) {
	if engine == nil {
		return core.State{}, errNilEngine
	}
	if host != nil {
		if engine.Store == nil {
			engine.Store = host.StateStore
		}
		if engine.Log == nil {
			engine.Log = host.WAL
		}
		if state.RunID == "" {
			state.RunID = host.RunID
		}
	}
	return engine.Run(ctx, state)
}

// ResumeRun resumes a persisted run after filling unset persistence from
// host.
func ResumeRun(ctx context.Context, host *Host, engine *Engine, runID string) (core.State, error) {
	if engine == nil {
		return core.State{}, errNilEngine
	}
	if host != nil {
		if engine.Store == nil {
			engine.Store = host.StateStore
		}
		if engine.Log == nil {
			engine.Log = host.WAL
		}
	}
	return engine.Resume(ctx, runID)
}

// ListRuns lists run IDs in host's state store.
func ListRuns(ctx context.Context, host *Host) ([]string, error) {
	if host == nil || host.StateStore == nil {
		return nil, nil
	}
	return host.StateStore.List(ctx)
}

// Approve records a decision on every undecided approval in a persisted run.
func Approve(ctx context.Context, host *Host, runID string, decision core.ApprovalDecision, by string) error {
	if host == nil || host.StateStore == nil {
		return errNoHost
	}
	state, err := host.StateStore.Load(ctx, runID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	changed := false
	for i := range state.PendingApprovals {
		pending := &state.PendingApprovals[i]
		if pending.DecidedAt != nil {
			continue
		}
		pending.Decision = decision
		pending.DecidedAt = &now
		pending.DecidedBy = by
		changed = true
	}
	if !changed {
		return nil
	}
	return host.StateStore.Save(ctx, state)
}
