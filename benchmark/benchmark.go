// Package benchmark drives predefined multimodal cases against one
// provider-model pair and stores every result on disk.
//
// The flow is deliberately flat: run → iterate cases → query the provider →
// store the result. A failing case is reported and skipped, never aborting
// the session. Each provider-model pair lives in its own runnable package
// under benchmark/, and results land in that package's
// tmp/<session-id>/case-NN-<name>/ directory.
package benchmark

//go:generate go run ./gen

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"

	// Every benchmark binary needs the full adapter set linked in.
	_ "github.com/bizshuk/agentsdk/provider/all"
)

// Target pins the provider-model pair one benchmark package runs against.
// Model is the chat model; media kinds default to the adapter's own model
// unless the case overrides it.
type Target struct {
	Provider string
	Model    string
}

// timeouts caps one case per kind; asynchronous video generation upstream
// routinely takes minutes.
var timeouts = map[Kind]time.Duration{
	KIND_CHAT:       2 * time.Minute,
	KIND_IMAGE:      5 * time.Minute,
	KIND_SPEECH:     2 * time.Minute,
	KIND_TRANSCRIBE: 2 * time.Minute,
	KIND_MUSIC:      5 * time.Minute,
	KIND_VIDEO:      15 * time.Minute,
}

// Root returns the benchmark package directory inside the repository,
// located from this source file — benchmark binaries are run with `go run`
// from the repo. testdata/ and the pkg/ pair packages anchor here.
func Root() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Dir(file)
}

// RunPair runs cases against target and stores results at the pair's
// canonical location, benchmark/pkg/<PairSlug>/tmp/<session-id>/ — the same
// directory whether the pair was reached from its pinned package, from the
// cmd flags, or from a catalog sweep.
func RunPair(ctx context.Context, target Target, cases []Case) error {
	pairDir := filepath.Join(Root(), "pkg", PairSlug(target.Provider, target.Model))
	return Run(ctx, target, cases, pairDir)
}

// Main is the shared entrypoint every provider-model package calls from its
// main(). Results are stored under the calling package's own directory,
// located via the caller's source path.
func Main(target Target, cases []Case) {
	pkgDir := "."
	if _, file, _, ok := runtime.Caller(1); ok {
		pkgDir = filepath.Dir(file)
	}
	if err := Run(context.Background(), target, cases, pkgDir); err != nil {
		fmt.Fprintf(os.Stderr, "benchmark: %v\n", err)
		os.Exit(1)
	}
}

// Run executes every case against the target and persists one Record per
// case plus a session-level summary.json. Case failures are reported and
// skipped; only storage-infrastructure failures return an error.
func Run(ctx context.Context, target Target, cases []Case, pkgDir string) error {
	sessionID := time.Now().Format(SESSION_TIME_FORMAT)
	sessionDir, err := makeSessionDir(pkgDir, sessionID)
	if err != nil {
		return err
	}
	root := Root() // shared testdata/ anchor, independent of nesting depth

	fmt.Printf("benchmark %s model=%s session=%s (%d cases)\n",
		target.Provider, orDefault(target.Model), sessionID, len(cases))
	for _, warn := range offCatalog(target, cases) {
		fmt.Println(warn)
	}

	records := make([]Record, 0, len(cases))
	okCount := 0
	for i, c := range cases {
		caseDir, err := makeCaseDir(sessionDir, i, c.Name)
		if err != nil {
			return err
		}
		rec := runCase(ctx, target, c, root, caseDir)
		if err := writeJSON(filepath.Join(caseDir, "meta.json"), rec); err != nil {
			return err
		}
		records = append(records, rec)

		if rec.Status == STATUS_OK {
			okCount++
			fmt.Printf("[%d/%d] %s/%s ok (%.1fs) → %v\n",
				i+1, len(cases), c.Kind, c.Name,
				float64(rec.DurationMs)/1000, rec.Outputs)
		} else {
			fmt.Printf("[%d/%d] %s/%s FAIL: %s\n",
				i+1, len(cases), c.Kind, c.Name, rec.Error)
		}
	}

	if err := writeJSON(filepath.Join(sessionDir, "summary.json"), records); err != nil {
		return err
	}
	fmt.Printf("done: %d/%d ok → %s\n", okCount, len(cases), sessionDir)
	return nil
}

func orDefault(model string) string {
	if model == "" {
		return "(adapter default)"
	}
	return model
}

// offCatalog reports every pinned model — the chat Target.Model and each
// media Case.Model — missing from the provider's bundled DefaultCatalog.
// These are warnings, not errors: live catalogs, local Ollama pulls, and
// media models the snapshot does not list (grok image) are legitimate.
func offCatalog(target Target, cases []Case) []string {
	specs, ok := provider.Catalog(target.Provider)
	if !ok || len(specs) == 0 {
		return nil
	}
	inCatalog := make(map[string]bool, len(specs))
	ids := make([]string, 0, len(specs))
	for _, spec := range specs {
		inCatalog[spec.ID] = true
		ids = append(ids, spec.ID)
	}

	var warns []string
	seen := map[string]bool{}
	check := func(model string) {
		if model == "" || seen[model] || inCatalog[model] {
			return
		}
		seen[model] = true
		warns = append(warns, fmt.Sprintf(
			"warning: model %s is not in the %s DefaultCatalog (%s)",
			model, target.Provider, strings.Join(ids, ", ")))
	}
	check(target.Model)
	for _, c := range cases {
		check(c.Model)
	}
	return warns
}

// runCase queries the provider for one case and stores its outputs. Every
// failure is folded into the returned Record.
func runCase(ctx context.Context, target Target, c Case, root, caseDir string) Record {
	rec := Record{
		Case:      c.Name,
		Kind:      c.Kind,
		Provider:  target.Provider,
		Model:     target.Model,
		Prompt:    c.Prompt,
		InputFile: c.InputFile,
		StartedAt: time.Now().UTC(),
	}
	if c.Kind != KIND_CHAT {
		rec.Model = c.Model
	}

	timeout, ok := timeouts[c.Kind]
	if !ok {
		rec.Status = STATUS_FAIL
		rec.Error = fmt.Sprintf("unknown case kind %q", c.Kind)
		return rec
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	outputs, extra, err := dispatch(ctx, target, c, root, caseDir)
	rec.DurationMs = time.Since(rec.StartedAt).Milliseconds()
	rec.Outputs = outputs
	rec.Extra = extra
	if err != nil {
		rec.Status = STATUS_FAIL
		rec.Error = err.Error()
		return rec
	}
	rec.Status = STATUS_OK
	return rec
}

func dispatch(ctx context.Context, target Target, c Case, root, caseDir string) ([]string, map[string]string, error) {
	switch c.Kind {
	case KIND_CHAT:
		return runChat(ctx, target, c, root, caseDir)
	case KIND_IMAGE:
		return runImage(ctx, target, c, root, caseDir)
	case KIND_SPEECH:
		return runSpeech(ctx, target, c, caseDir)
	case KIND_TRANSCRIBE:
		return runTranscribe(ctx, target, c, root, caseDir)
	case KIND_VIDEO:
		return runVideo(ctx, target, c, caseDir)
	case KIND_MUSIC:
		return runMusic(ctx, target, c, caseDir)
	default:
		return nil, nil, fmt.Errorf("unknown case kind %q", c.Kind)
	}
}

func runChat(ctx context.Context, target Target, c Case, root, caseDir string) ([]string, map[string]string, error) {
	adapter, err := provider.New(target.Provider, provider.Options{Model: target.Model})
	if err != nil {
		return nil, nil, err
	}

	parts := []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: c.Prompt}}
	if c.InputFile != "" {
		data, ext, err := readMedia(root, c.InputFile)
		if err != nil {
			return nil, nil, err
		}
		part, err := mediaPart(data, ext)
		if err != nil {
			return nil, nil, err
		}
		parts = append(parts, part)
	}

	res, err := adapter.Generate(ctx, core.ModelRequest{
		Messages: []core.Message{{Role: core.ROLE_USER, Parts: parts, Ts: time.Now().UTC()}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("generate: %w", err)
	}
	res = res.NormalizeContent()

	var outputs []string
	if res.Text != "" {
		name, err := saveText(caseDir, "output.txt", res.Text)
		if err != nil {
			return nil, nil, err
		}
		outputs = append(outputs, name)
	}
	mediaCount := 0
	for _, part := range res.Parts {
		var data []byte
		var ext string
		switch {
		case part.Kind == core.PART_KIND_IMAGE && len(part.Image) > 0:
			data, ext = part.Image, extByMIME(part.ImageMIME, ".png")
		case part.Kind == core.PART_KIND_AUDIO && len(part.Audio) > 0:
			data, ext = part.Audio, extByMIME(part.AudioMIME, ".mp3")
		default:
			continue
		}
		mediaCount++
		name, err := saveBytes(caseDir, fmt.Sprintf("output-%d%s", mediaCount, ext), data)
		if err != nil {
			return nil, nil, err
		}
		outputs = append(outputs, name)
	}
	if len(outputs) == 0 {
		return nil, nil, fmt.Errorf("model returned no text and no media (stop=%s)", res.StopReason)
	}

	extra := map[string]string{
		"stop_reason": res.StopReason,
		"tokens": fmt.Sprintf("%d/%d",
			res.Usage.PromptTokens, res.Usage.CompletionTokens),
	}
	return outputs, extra, nil
}

func runImage(ctx context.Context, target Target, c Case, root, caseDir string) ([]string, map[string]string, error) {
	generator, err := provider.NewImage(target.Provider, provider.Options{})
	if err != nil {
		return nil, nil, err
	}

	req := provider.ImageRequest{Model: c.Model, Prompt: c.Prompt}
	if c.InputFile != "" {
		data, ext, err := readMedia(root, c.InputFile)
		if err != nil {
			return nil, nil, err
		}
		req.SubjectReferences = []provider.ImageReference{{
			Base64:   base64.StdEncoding.EncodeToString(data),
			MIMEType: mimeByExt(ext),
		}}
	}

	res, err := generator.GenerateImage(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("generate image: %w", err)
	}
	if len(res.Images) == 0 {
		return nil, nil, fmt.Errorf("provider returned no images")
	}

	var outputs []string
	extra := map[string]string{"images": fmt.Sprintf("%d", len(res.Images))}
	for i, img := range res.Images {
		var name string
		switch {
		case img.Base64 != "":
			raw, err := base64.StdEncoding.DecodeString(img.Base64)
			if err != nil {
				return nil, nil, fmt.Errorf("decode image %d: %w", i+1, err)
			}
			name, err = saveBytes(caseDir,
				fmt.Sprintf("output-%d%s", i+1, extByMIME(img.MIMEType, ".png")), raw)
			if err != nil {
				return nil, nil, err
			}
		case img.URL != "":
			// URL lifetimes are provider-defined; the link is stored as-is.
			name, err = saveText(caseDir, fmt.Sprintf("output-%d.url", i+1), img.URL)
			if err != nil {
				return nil, nil, err
			}
		default:
			continue
		}
		outputs = append(outputs, name)
		if img.RevisedPrompt != "" {
			extra["revised_prompt"] = img.RevisedPrompt
		}
	}
	return outputs, extra, nil
}

func runSpeech(ctx context.Context, target Target, c Case, caseDir string) ([]string, map[string]string, error) {
	generator, err := provider.NewSpeech(target.Provider, provider.Options{})
	if err != nil {
		return nil, nil, err
	}

	res, err := generator.GenerateSpeech(ctx, provider.SpeechRequest{Model: c.Model, Text: c.Prompt})
	if err != nil {
		return nil, nil, fmt.Errorf("generate speech: %w", err)
	}
	if len(res.Audio.Bytes) == 0 {
		return nil, nil, fmt.Errorf("provider returned empty audio")
	}

	name, err := saveBytes(caseDir, "output"+extByAudioFormat(res.Audio.Format), res.Audio.Bytes)
	if err != nil {
		return nil, nil, err
	}
	extra := map[string]string{
		"format":      res.Audio.Format,
		"duration_ms": fmt.Sprintf("%d", res.Info.DurationMs),
	}
	return []string{name}, extra, nil
}

func runTranscribe(ctx context.Context, target Target, c Case, root, caseDir string) ([]string, map[string]string, error) {
	data, ext, err := readMedia(root, c.InputFile)
	if err != nil {
		return nil, nil, err
	}
	transcriber, err := provider.NewTranscriber(target.Provider, provider.Options{})
	if err != nil {
		return nil, nil, err
	}

	res, err := transcriber.Transcribe(ctx, provider.TranscribeRequest{
		Model: c.Model,
		Audio: provider.AudioSource{Bytes: data, Format: ext},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("transcribe: %w", err)
	}

	name, err := saveText(caseDir, "output.txt", res.Text)
	if err != nil {
		return nil, nil, err
	}
	extra := map[string]string{
		"language": res.Language,
		"words":    fmt.Sprintf("%d", len(res.Words)),
	}
	return []string{name}, extra, nil
}

func runVideo(ctx context.Context, target Target, c Case, caseDir string) ([]string, map[string]string, error) {
	generator, err := provider.NewVideo(target.Provider, provider.Options{})
	if err != nil {
		return nil, nil, err
	}

	res, err := generator.GenerateVideo(ctx, provider.VideoRequest{
		Mode:       provider.VIDEO_MODE_TEXT,
		Model:      c.Model,
		Prompt:     c.Prompt,
		OutputPath: filepath.Join(caseDir, "output.mp4"),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("generate video: %w", err)
	}
	return []string{filepath.Base(res.Path)}, nil, nil
}

func runMusic(ctx context.Context, target Target, c Case, caseDir string) ([]string, map[string]string, error) {
	generator, err := provider.NewMusic(target.Provider, provider.Options{})
	if err != nil {
		return nil, nil, err
	}

	res, err := generator.GenerateMusic(ctx, provider.MusicRequest{
		Model:  c.Model,
		Prompt: c.Prompt,
		Lyrics: c.Lyrics,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("generate music: %w", err)
	}

	extra := map[string]string{
		"format":      res.Audio.Format,
		"duration_ms": fmt.Sprintf("%d", res.Info.DurationMilliseconds),
	}
	switch {
	case res.Audio.Hex != "":
		raw, err := hex.DecodeString(res.Audio.Hex)
		if err != nil {
			return nil, nil, fmt.Errorf("decode music audio: %w", err)
		}
		name, err := saveBytes(caseDir, "output"+extByAudioFormat(res.Audio.Format), raw)
		if err != nil {
			return nil, nil, err
		}
		return []string{name}, extra, nil
	case res.Audio.URL != "":
		name, err := saveText(caseDir, "output.url", res.Audio.URL)
		if err != nil {
			return nil, nil, err
		}
		return []string{name}, extra, nil
	default:
		return nil, nil, fmt.Errorf("provider returned empty audio")
	}
}
