package provider

import (
	"context"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/pricing"
)

type accountingAdapter struct {
	adapter      Adapter
	providerName string
	modelName    string
	snapshot     pricing.Snapshot
}

func withAccounting(
	providerName string,
	modelName string,
	adapter Adapter,
	snapshot pricing.Snapshot,
) Adapter {
	if adapter == nil {
		return nil
	}
	base := &accountingAdapter{
		adapter:      adapter,
		providerName: providerName,
		modelName:    modelName,
		snapshot:     snapshot,
	}
	if lister, ok := adapter.(ModelLister); ok {
		return &accountingAdapterModelLister{accountingAdapter: base, lister: lister}
	}
	return base
}

type accountingAdapterModelLister struct {
	*accountingAdapter
	lister ModelLister
}

func (a *accountingAdapterModelLister) ListModels(ctx context.Context) ([]ModelSpec, error) {
	return a.lister.ListModels(ctx)
}

func (a *accountingAdapter) Generate(
	ctx context.Context,
	req core.ModelRequest,
) (core.ModelResult, error) {
	result, err := a.adapter.Generate(ctx, req)
	if err != nil {
		return result, err
	}
	result.Cost = a.cost(result.Cost, result.Usage)
	return result, nil
}

func (a *accountingAdapter) Stream(
	ctx context.Context,
	req core.ModelRequest,
) (<-chan core.ModelChunk, error) {
	input, err := a.adapter.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	output := make(chan core.ModelChunk, 1)
	go func() {
		defer close(output)
		for chunk := range input {
			if chunk.Done {
				chunk.Cost = a.cost(chunk.Cost, chunk.Usage)
			}
			select {
			case <-ctx.Done():
				return
			case output <- chunk:
			}
		}
	}()
	return output, nil
}

func (a *accountingAdapter) cost(existing core.Cost, usage core.TokenUsage) core.Cost {
	return accountingCost(existing, a.providerName, a.modelName, usage, a.snapshot)
}

func accountingCost(
	existing core.Cost,
	providerName string,
	modelName string,
	usage core.TokenUsage,
	snapshot pricing.Snapshot,
) core.Cost {
	if existing.Status != "" {
		return existing
	}
	return snapshot.Estimate(providerName, modelName, usage)
}

func requestModel(configured, requested string) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	return configured
}

type imageAccounting struct {
	generator    ImageGenerator
	providerName string
	modelName    string
	snapshot     pricing.Snapshot
}

func withImageAccounting(
	providerName string,
	modelName string,
	generator ImageGenerator,
	snapshot pricing.Snapshot,
) ImageGenerator {
	if generator == nil {
		return nil
	}
	return &imageAccounting{
		generator: generator, providerName: providerName, modelName: modelName, snapshot: snapshot,
	}
}

func (a *imageAccounting) GenerateImage(
	ctx context.Context,
	req ImageRequest,
) (ImageResult, error) {
	result, err := a.generator.GenerateImage(ctx, req)
	if err != nil {
		return result, err
	}
	if result.Usage.GeneratedImages == 0 {
		result.Usage.GeneratedImages = len(result.Images)
	}
	if result.Usage.TotalTokens == 0 {
		result.Usage.TotalTokens = result.Usage.InputTokens + result.Usage.OutputTokens
	}
	result.Cost = accountingCost(result.Cost, a.providerName, requestModel(a.modelName, req.Model), core.TokenUsage{
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		TotalTokens:  result.Usage.TotalTokens,
	}, a.snapshot)
	return result, nil
}

type videoAccounting struct {
	generator    VideoGenerator
	providerName string
	modelName    string
	snapshot     pricing.Snapshot
}

func withVideoAccounting(
	providerName string,
	modelName string,
	generator VideoGenerator,
	snapshot pricing.Snapshot,
) VideoGenerator {
	if generator == nil {
		return nil
	}
	return &videoAccounting{
		generator: generator, providerName: providerName, modelName: modelName, snapshot: snapshot,
	}
}

func (a *videoAccounting) MaxPromptLength() int {
	return a.generator.MaxPromptLength()
}

func (a *videoAccounting) GenerateVideo(
	ctx context.Context,
	req VideoRequest,
) (VideoResult, error) {
	result, err := a.generator.GenerateVideo(ctx, req)
	if err != nil {
		return result, err
	}
	if result.Usage.GeneratedVideos == 0 && result.Path != "" {
		result.Usage.GeneratedVideos = 1
	}
	if result.Usage.DurationMilliseconds == 0 && req.Duration > 0 {
		result.Usage.DurationMilliseconds = int64(req.Duration) * 1000
	}
	result.Cost = accountingCost(
		result.Cost, a.providerName, requestModel(a.modelName, req.Model), core.TokenUsage{}, a.snapshot,
	)
	return result, nil
}

type musicAccounting struct {
	generator    MusicGenerator
	providerName string
	modelName    string
	snapshot     pricing.Snapshot
}

func withMusicAccounting(
	providerName string,
	modelName string,
	generator MusicGenerator,
	snapshot pricing.Snapshot,
) MusicGenerator {
	if generator == nil {
		return nil
	}
	return &musicAccounting{
		generator: generator, providerName: providerName, modelName: modelName, snapshot: snapshot,
	}
}

func (a *musicAccounting) GenerateMusic(
	ctx context.Context,
	req MusicRequest,
) (MusicResult, error) {
	result, err := a.generator.GenerateMusic(ctx, req)
	if err != nil {
		return result, err
	}
	if result.Usage.GeneratedTracks == 0 && (result.Audio.URL != "" || result.Audio.Hex != "") {
		result.Usage.GeneratedTracks = 1
	}
	if result.Usage.DurationMilliseconds == 0 {
		result.Usage.DurationMilliseconds = result.Info.DurationMilliseconds
	}
	result.Cost = accountingCost(
		result.Cost, a.providerName, requestModel(a.modelName, req.Model), core.TokenUsage{}, a.snapshot,
	)
	return result, nil
}

type transcriberAccounting struct {
	transcriber  Transcriber
	providerName string
	modelName    string
	snapshot     pricing.Snapshot
}

func withTranscriberAccounting(
	providerName string,
	modelName string,
	transcriber Transcriber,
	snapshot pricing.Snapshot,
) Transcriber {
	if transcriber == nil {
		return nil
	}
	return &transcriberAccounting{
		transcriber: transcriber, providerName: providerName, modelName: modelName, snapshot: snapshot,
	}
}

func (a *transcriberAccounting) Transcribe(
	ctx context.Context,
	req TranscribeRequest,
) (TranscribeResult, error) {
	result, err := a.transcriber.Transcribe(ctx, req)
	if err != nil {
		return result, err
	}
	if result.Usage.AudioDurationMilliseconds == 0 {
		result.Usage.AudioDurationMilliseconds = req.Audio.DurationMilliseconds
	}
	result.Cost = accountingCost(
		result.Cost, a.providerName, requestModel(a.modelName, req.Model), core.TokenUsage{}, a.snapshot,
	)
	return result, nil
}

type speechAccounting struct {
	generator    SpeechGenerator
	providerName string
	modelName    string
	snapshot     pricing.Snapshot
}

func (a *speechAccounting) GenerateSpeech(
	ctx context.Context,
	req SpeechRequest,
) (SpeechResult, error) {
	result, err := a.generator.GenerateSpeech(ctx, req)
	if err != nil {
		return result, err
	}
	if result.Usage.Characters == 0 {
		result.Usage.Characters = utf8.RuneCountInString(req.Text)
	}
	if result.Usage.AudioDurationMilliseconds == 0 {
		result.Usage.AudioDurationMilliseconds = result.Info.DurationMs
	}
	result.Cost = accountingCost(
		result.Cost, a.providerName, requestModel(a.modelName, req.Model), core.TokenUsage{}, a.snapshot,
	)
	return result, nil
}

type speechAccountingStreamer struct {
	*speechAccounting
	streamer SpeechStreamer
}

func (a *speechAccountingStreamer) StreamSpeech(
	ctx context.Context,
	req SpeechRequest,
) (io.ReadCloser, error) {
	return a.streamer.StreamSpeech(ctx, req)
}

type speechAccountingVoiceLister struct {
	*speechAccounting
	lister VoiceLister
}

func (a *speechAccountingVoiceLister) ListVoices(
	ctx context.Context,
	req VoiceListRequest,
) (VoiceListResult, error) {
	return a.lister.ListVoices(ctx, req)
}

type speechAccountingStreamerVoiceLister struct {
	*speechAccounting
	streamer SpeechStreamer
	lister   VoiceLister
}

type speechAccountingModelLister struct {
	*speechAccounting
	lister ModelLister
}

func (a *speechAccountingModelLister) ListModels(ctx context.Context) ([]ModelSpec, error) {
	return a.lister.ListModels(ctx)
}

type speechAccountingStreamerModelLister struct {
	*speechAccounting
	streamer SpeechStreamer
	lister   ModelLister
}

func (a *speechAccountingStreamerModelLister) StreamSpeech(
	ctx context.Context,
	req SpeechRequest,
) (io.ReadCloser, error) {
	return a.streamer.StreamSpeech(ctx, req)
}

func (a *speechAccountingStreamerModelLister) ListModels(ctx context.Context) ([]ModelSpec, error) {
	return a.lister.ListModels(ctx)
}

type speechAccountingVoiceModelLister struct {
	*speechAccounting
	voices VoiceLister
	models ModelLister
}

func (a *speechAccountingVoiceModelLister) ListVoices(
	ctx context.Context,
	req VoiceListRequest,
) (VoiceListResult, error) {
	return a.voices.ListVoices(ctx, req)
}

func (a *speechAccountingVoiceModelLister) ListModels(ctx context.Context) ([]ModelSpec, error) {
	return a.models.ListModels(ctx)
}

type speechAccountingAllOptional struct {
	*speechAccounting
	streamer SpeechStreamer
	voices   VoiceLister
	models   ModelLister
}

func (a *speechAccountingAllOptional) StreamSpeech(
	ctx context.Context,
	req SpeechRequest,
) (io.ReadCloser, error) {
	return a.streamer.StreamSpeech(ctx, req)
}

func (a *speechAccountingAllOptional) ListVoices(
	ctx context.Context,
	req VoiceListRequest,
) (VoiceListResult, error) {
	return a.voices.ListVoices(ctx, req)
}

func (a *speechAccountingAllOptional) ListModels(ctx context.Context) ([]ModelSpec, error) {
	return a.models.ListModels(ctx)
}

func (a *speechAccountingStreamerVoiceLister) StreamSpeech(
	ctx context.Context,
	req SpeechRequest,
) (io.ReadCloser, error) {
	return a.streamer.StreamSpeech(ctx, req)
}

func (a *speechAccountingStreamerVoiceLister) ListVoices(
	ctx context.Context,
	req VoiceListRequest,
) (VoiceListResult, error) {
	return a.lister.ListVoices(ctx, req)
}

func withSpeechAccounting(
	providerName string,
	modelName string,
	generator SpeechGenerator,
	snapshot pricing.Snapshot,
) SpeechGenerator {
	if generator == nil {
		return nil
	}
	base := &speechAccounting{
		generator: generator, providerName: providerName, modelName: modelName, snapshot: snapshot,
	}
	streamer, streams := generator.(SpeechStreamer)
	lister, lists := generator.(VoiceLister)
	modelLister, listsModels := generator.(ModelLister)
	switch {
	case streams && lists && listsModels:
		return &speechAccountingAllOptional{
			speechAccounting: base, streamer: streamer, voices: lister, models: modelLister,
		}
	case streams && listsModels:
		return &speechAccountingStreamerModelLister{
			speechAccounting: base, streamer: streamer, lister: modelLister,
		}
	case lists && listsModels:
		return &speechAccountingVoiceModelLister{
			speechAccounting: base, voices: lister, models: modelLister,
		}
	case listsModels:
		return &speechAccountingModelLister{speechAccounting: base, lister: modelLister}
	case streams && lists:
		return &speechAccountingStreamerVoiceLister{speechAccounting: base, streamer: streamer, lister: lister}
	case streams:
		return &speechAccountingStreamer{speechAccounting: base, streamer: streamer}
	case lists:
		return &speechAccountingVoiceLister{speechAccounting: base, lister: lister}
	default:
		return base
	}
}

type translatorAccounting struct {
	translator   Translator
	providerName string
	modelName    string
	snapshot     pricing.Snapshot
}

func (a *translatorAccounting) Translate(
	ctx context.Context,
	req TranslateRequest,
) (TranslateResult, error) {
	result, err := a.translator.Translate(ctx, req)
	if err != nil {
		return result, err
	}
	result.Cost = accountingCost(
		result.Cost, a.providerName, requestModel(a.modelName, req.Model), result.Usage, a.snapshot,
	)
	return result, nil
}

type translatorAccountingStreamer struct {
	*translatorAccounting
	streamer TranslateStreamer
}

func (a *translatorAccountingStreamer) StreamTranslation(
	ctx context.Context,
	req TranslateRequest,
) (<-chan TranslateChunk, error) {
	return a.streamer.StreamTranslation(ctx, req)
}

func withTranslateAccounting(
	providerName string,
	modelName string,
	translator Translator,
	snapshot pricing.Snapshot,
) Translator {
	if translator == nil {
		return nil
	}
	base := &translatorAccounting{
		translator: translator, providerName: providerName, modelName: modelName, snapshot: snapshot,
	}
	if streamer, ok := translator.(TranslateStreamer); ok {
		return &translatorAccountingStreamer{translatorAccounting: base, streamer: streamer}
	}
	return base
}
