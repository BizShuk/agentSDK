package runtime

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/bizshuk/gosdk/config"
)

// AppDirs 是一次性 wiring 的結果。sample 把 DataDir 餵給
// filestore.NewFileStateStore / NewFileWAL,LogDir 給開 log 檔的程式碼。
type AppDirs struct {
	DataDir string // ~/.config/<appName>/data
	LogDir  string // ~/.config/<appName>/log
	RunID   string // UnixNano,當 states / wal / log 的檔名
	LogFile string // <LogDir>/<RunID>.log
}

// OpenForCLI 為 CLI 樣本一次做完六件事:
//  1. config.Default(config.WithAppName(appName)) — 決定 APP_CONFIG_DIR
//  2. 確認 appName 已綁定(sentinel 檢查)
//  3. mkdir <dataDir>/states、<dataDir>/wal、<logDir>
//  4. 開 <logDir>/<runID>.log(O_APPEND|O_CREATE)
//  5. 把 slog default 換成 JSON file handler,level 由 caller 決定
//  6. 產生 runID = UnixNano
//
// 若 appName 為空字串,回傳錯誤而非 panic — 給 library 嵌入使用。
// CLI 場景請用 MustOpenForCLI,跟 regexp/template 的 Must 慣例一致。
func OpenForCLI(appName string, level slog.Level) (*AppDirs, error) {
	if appName == "" {
		return nil, fmt.Errorf("runtime: appName 不可為空")
	}

	// 1. 綁定 appName。第二次呼叫 WithAppName 同 appName 是冪等的 —
	//    gosdk/config 的 applyOptions 會把 os.UserHomeDir()/.config/<appName>
	//    寫進 o.appConfigDir,Default() 再寫回套件全域 appName。
	config.Default(config.WithAppName(appName))

	// 2. sentinel 檢查。GetAppName() 是唯一可靠的「有沒有初始化」指標 —
	//    GetAppDataDir() 在 appName 為空時回 "data"(經 filepath.Join("","data")),
	//    不是空字串,無法當 nil-check 用。
	if config.GetAppName() == "" {
		return nil, fmt.Errorf("runtime: gosdk/config 未初始化 (請先呼叫 config.Default(config.WithAppName(%q)))", appName)
	}

	dataDir := config.GetAppDataDir()
	logDir := config.GetAppLogDir()
	statesDir := filepath.Join(dataDir, "states")
	walDir := filepath.Join(dataDir, "wal")
	for _, d := range []string{statesDir, walDir, logDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, fmt.Errorf("runtime: mkdir %s: %w", d, err)
		}
	}

	// 3. 開 log 檔 + 切 slog default。
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	logFile := filepath.Join(logDir, runID+".log")
	if err := openRunLog(logFile, runID, level); err != nil {
		return nil, fmt.Errorf("runtime: open log: %w", err)
	}

	return &AppDirs{
		DataDir: dataDir,
		LogDir:  logDir,
		RunID:   runID,
		LogFile: logFile,
	}, nil
}

// MustOpenForCLI 等同 OpenForCLI,但失敗時 panic — 給 CLI 啟動入口用。
// 失敗必為 programmer error(忘了傳 appName),panic 比 silent error 更早暴露。
func MustOpenForCLI(appName string, level slog.Level) *AppDirs {
	dirs, err := OpenForCLI(appName, level)
	if err != nil {
		panic(err)
	}
	return dirs
}

// openRunLog 把 slog default 換成 <logFile> 的 JSON handler,並把 runID 綁在
// 每筆 record 上方便事後 grep。
func openRunLog(logFile, runID string, level slog.Level) error {
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h).With(slog.String("runID", runID)))
	return nil
}
