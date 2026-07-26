package agent

import (
	"fmt"
	"log/slog"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/memory/filestore"
	gosdkconfig "github.com/bizshuk/gosdk/config"
)

// Host is the embeddable container agent needs to drive a run. It owns
// the process-identity paths and the file-backed persistence the engine
// reads through, and nothing else. Process-level concerns — signal
// handling, global slog default, mkdir for logs — live in agent/cli,
// so a host can be embedded in an HTTP server, a test harness, or any
// other caller that wants the agent without surrendering those knobs.
//
// AppConfig is the deprecated alias kept for one release; new code
// should write *Host.
type Host struct {
	DataDir    string             // ~/.config/<name>/data
	LogDir     string             // ~/.config/<name>/logs
	RunID      string             // caller-assigned; empty until the host chooses to seed one
	LogFile    string             // empty when no log file is wired (a Host built by Open)
	Logger     *slog.Logger       // optional; nil when no log handler has been installed
	StateStore core.StateStore    // file-backed StateStore, ready to use
	WAL        core.WriteAheadLog // file-backed WriteAheadLog, ready to use
}

// AppConfig is the historical name for Host.
//
// Deprecated: use Host. Kept for one release so existing callers and
// tests keep compiling; cli.OpenForCLI is the new home for the process
// side of AppConfig.
type AppConfig = Host

// Open is the embeddable half of the prior OpenForCLI split. It binds
// appName under gosdk/config, opens the file-backed StateStore and WAL
// under ~/.config/<Name>/, and returns the resulting Host. It does NOT
// create directories, open log files, or install a slog handler — those
// are process-side concerns that belong to agent/cli.OpenForCLI.
//
// RunID, LogFile, Logger are left for the caller (or for cli) to set.
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
