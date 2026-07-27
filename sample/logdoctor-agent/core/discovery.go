package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type logFile struct {
	source string
	path   string
	info   os.FileInfo
}

func discoverLogFiles(ctx context.Context, root string) ([]logFile, []error, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("scan log root: %w", err)
	}

	var files []logFile
	var warnings []error
	for _, appEntry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, warnings, err
		}
		if appEntry.Name() == "logdoctor" {
			continue
		}

		appPath := filepath.Join(root, appEntry.Name())
		appInfo, err := os.Lstat(appPath)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("inspect app %s: %w", appEntry.Name(), err))
			continue
		}
		if appInfo.Mode()&os.ModeSymlink != 0 || !appInfo.IsDir() {
			continue
		}

		appLogs, appWarnings, err := discoverAppLogs(ctx, appEntry.Name(), appPath)
		files = append(files, appLogs...)
		warnings = append(warnings, appWarnings...)
		if err != nil {
			return nil, warnings, err
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].source < files[j].source
	})
	return files, warnings, nil
}

func discoverAppLogs(ctx context.Context, appName, appPath string) ([]logFile, []error, error) {
	logsPath := filepath.Join(appPath, "logs")
	logsInfo, err := os.Lstat(logsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, []error{fmt.Errorf("inspect %s/logs: %w", appName, err)}, nil
	}
	if logsInfo.Mode()&os.ModeSymlink != 0 || !logsInfo.IsDir() {
		return nil, nil, nil
	}

	entries, err := os.ReadDir(logsPath)
	if err != nil {
		return nil, []error{fmt.Errorf("scan %s/logs: %w", appName, err)}, nil
	}

	var files []logFile
	var warnings []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, warnings, err
		}
		logPath := filepath.Join(logsPath, entry.Name())
		info, err := os.Lstat(logPath)
		if err != nil {
			warnings = append(warnings, fmt.Errorf(
				"inspect %s/logs/%s: %w",
				appName,
				entry.Name(),
				err,
			))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, logFile{
			source: filepath.ToSlash(filepath.Join(appName, entry.Name())),
			path:   logPath,
			info:   info,
		})
	}
	return files, warnings, nil
}
