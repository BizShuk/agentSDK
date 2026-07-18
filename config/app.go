// Package config provides the one-stop AppConfig for CLI samples.
// It wraps gosdk/config initialization, log file setup, and filestore-backed
// StateStore + WriteAheadLog wiring in a single OpenForCLI call.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/memory/filestore"
	utils "github.com/bizshuk/auth/utils"
	gosdkconfig "github.com/bizshuk/gosdk/config"
)

// AppConfig is the result of OpenForCLI — all wiring needed by a CLI sample
// in one struct. Callers wire Store / WAL directly from the fields;
// DataDir / LogDir / RunID are also exposed for further wiring.
type AppConfig struct {
	DataDir    string             // ~/.config/<appName>/data
	LogDir     string             // ~/.config/<appName>/log
	AuthDir    string             // ~/.config/<appName>/data/auth
	RunID      string             // UnixNano, used as states/wal/log filename
	LogFile    string             // <LogDir>/<RunID>.log
	StateStore core.StateStore    // file-backed StateStore, ready to use
	WAL        core.WriteAheadLog // file-backed WriteAheadLog, ready to use
	AuthStore  *utils.FileStore   // provider credentials (0600 JSON files)
}

// OpenForCLI does everything a CLI sample needs in one call:
//  1. gosdkconfig.Default(gosdkconfig.WithAppName(appName)) — bind APP_CONFIG_DIR
//  2. sentinel check that appName is bound
//  3. mkdir <dataDir>/states, <dataDir>/wal, <logDir>
//  4. open <logDir>/<runID>.log (O_APPEND|O_CREATE)
//  5. swap slog default to JSON file handler at the given level
//  6. initialise file-backed StateStore + WriteAheadLog under dataDir
//  7. generate runID = UnixNano
//
// Returns an error if appName is empty. CLI callers that cannot recover
// should use MustOpenForCLI.
func OpenForCLI(appName string, level slog.Level) (*AppConfig, error) {
	if appName == "" {
		return nil, fmt.Errorf("config: appName must not be empty")
	}

	// 1. Bind appName. Calling WithAppName with the same appName twice is
	//    idempotent — gosdk/config writes os.UserHomeDir()/.config/<appName>
	//    into o.appConfigDir; Default() writes it back to the package global.
	gosdkconfig.Default(gosdkconfig.WithAppName(appName))

	// 2. Sentinel check. GetAppName() is the only reliable "initialised"
	//    indicator — GetAppDataDir() returns "data" when appName is empty
	//    (via filepath.Join("", "data")), not "".
	if gosdkconfig.GetAppName() == "" {
		return nil, fmt.Errorf("config: gosdk/config not initialised (call config.Default(config.WithAppName(%q)))", appName)
	}

	dataDir := gosdkconfig.GetAppDataDir()
	logDir := gosdkconfig.GetAppLogDir()
	statesDir := filepath.Join(dataDir, "states")
	walDir := filepath.Join(dataDir, "wal")
	for _, d := range []string{statesDir, walDir, logDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, fmt.Errorf("config: mkdir %s: %w", d, err)
		}
	}

	// 3. Open log file + swap slog default.
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	logFile := filepath.Join(logDir, runID+".log")
	if err := openRunLog(logFile, runID, level); err != nil {
		return nil, fmt.Errorf("config: open log: %w", err)
	}

	// 4. File-backed StateStore + WriteAheadLog.
	store, err := filestore.NewJSONFileStateStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("config: state store: %w", err)
	}
	wal, err := filestore.NewJSONLFileLog(dataDir)
	if err != nil {
		return nil, fmt.Errorf("config: wal: %w", err)
	}

	// 5. Provider 憑證 store (0700 目錄 / 0600 檔案)。
	authStore, err := utils.NewFileStore(authDir(dataDir))
	if err != nil {
		return nil, fmt.Errorf("config: auth store: %w", err)
	}

	return &AppConfig{
		DataDir:    dataDir,
		LogDir:     logDir,
		AuthDir:    authStore.Dir(),
		RunID:      runID,
		LogFile:    logFile,
		StateStore: store,
		WAL:        wal,
		AuthStore:  authStore,
	}, nil
}

func authDir(dataDir string) string {
	return filepath.Join(dataDir, "auth")
}

// MustOpenForCLI is like OpenForCLI but panics on error — for CLI entry
// points where failure is always a programmer error (e.g. empty appName).
func MustOpenForCLI(appName string, level slog.Level) *AppConfig {
	cfg, err := OpenForCLI(appName, level)
	if err != nil {
		panic(err)
	}
	return cfg
}

// openRunLog swaps slog default to a JSON file handler writing to logFile,
// and attaches runID to every record for grep-ability.
func openRunLog(logFile, runID string, level slog.Level) error {
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h).With(slog.String("runID", runID)))
	return nil
}
