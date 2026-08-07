package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"time"
)

const MAX_MANIFEST_BYTES int64 = 32 << 20

// Diff summarizes model-level changes between two pricing snapshots.
type Diff struct {
	Added   []string
	Changed []string
	Removed []string
}

// Fetch downloads and validates an OpenRouter-compatible pricing manifest.
func Fetch(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	fetchedAt time.Time,
) (Snapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse pricing manifest URL: %w", err)
	}
	query := parsed.Query()
	query.Set("output_modalities", "all")
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build pricing manifest request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetch pricing manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return Snapshot{}, fmt.Errorf("fetch pricing manifest: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MAX_MANIFEST_BYTES+1))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read pricing manifest: %w", err)
	}
	if int64(len(raw)) > MAX_MANIFEST_BYTES {
		return Snapshot{}, fmt.Errorf("read pricing manifest: response exceeds %d bytes", MAX_MANIFEST_BYTES)
	}
	snapshot, err := DecodeOpenRouterManifest(bytes.NewReader(raw), fetchedAt)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Source = parsed.String()
	return snapshot, nil
}

// Compare returns stable, sorted model IDs for a snapshot preview.
func Compare(before, after Snapshot) Diff {
	var diff Diff
	for id, next := range after.Models {
		current, exists := before.Models[id]
		switch {
		case !exists:
			diff.Added = append(diff.Added, id)
		case !reflect.DeepEqual(current, next):
			diff.Changed = append(diff.Changed, id)
		}
	}
	for id := range before.Models {
		if _, exists := after.Models[id]; !exists {
			diff.Removed = append(diff.Removed, id)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Changed)
	sort.Strings(diff.Removed)
	return diff
}

// ReadSnapshot loads a versioned snapshot from disk.
func ReadSnapshot(path string) (Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read pricing snapshot: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode pricing snapshot: %w", err)
	}
	if len(snapshot.Models) == 0 {
		if snapshot.Source == "" {
			return Snapshot{}, fmt.Errorf("pricing snapshot source is empty")
		}
		return snapshot, nil
	}
	if err := validateSnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// WriteSnapshot atomically replaces path with a validated snapshot.
func WriteSnapshot(path string, snapshot Snapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pricing snapshot: %w", err)
	}
	raw = append(raw, '\n')

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("stat pricing snapshot: %w", statErr)
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".pricing-*.json")
	if err != nil {
		return fmt.Errorf("create pricing snapshot temp file: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("chmod pricing snapshot temp file: %w", err)
	}
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write pricing snapshot temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync pricing snapshot temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close pricing snapshot temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace pricing snapshot: %w", err)
	}
	return nil
}

// DefaultSnapshotPath returns the source-tree snapshot used by the maintainer
// refresh command.
func DefaultSnapshotPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("provider", "pricing", "snapshot.json")
	}
	return filepath.Join(filepath.Dir(file), "snapshot.json")
}

func validateSnapshot(snapshot Snapshot) error {
	if len(snapshot.Models) == 0 {
		return fmt.Errorf("pricing snapshot models are empty")
	}
	if snapshot.Source == "" {
		return fmt.Errorf("pricing snapshot source is empty")
	}
	if snapshot.PricingAsOf == "" {
		return fmt.Errorf("pricing snapshot pricing_as_of is empty")
	}
	if _, err := time.Parse(time.RFC3339, snapshot.PricingAsOf); err != nil {
		return fmt.Errorf("pricing snapshot pricing_as_of: %w", err)
	}
	for id, rate := range snapshot.Models {
		if id == "" {
			return fmt.Errorf("pricing snapshot model id is empty")
		}
		if err := validateRate(rate); err != nil {
			return fmt.Errorf("pricing snapshot model %s: %w", id, err)
		}
	}
	return nil
}
