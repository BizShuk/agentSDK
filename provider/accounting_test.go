package provider

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/pricing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountingAdapterStub struct {
	result core.ModelResult
	chunks []core.ModelChunk
}

func (s accountingAdapterStub) Generate(context.Context, core.ModelRequest) (core.ModelResult, error) {
	return s.result, nil
}

func (s accountingAdapterStub) Stream(context.Context, core.ModelRequest) (<-chan core.ModelChunk, error) {
	out := make(chan core.ModelChunk, len(s.chunks))
	for _, chunk := range s.chunks {
		out <- chunk
	}
	close(out)
	return out, nil
}

func accountingSnapshot() pricing.Snapshot {
	return pricing.Snapshot{
		Source:      pricing.OPENROUTER_MODELS_URL,
		PricingAsOf: time.Unix(0, 0).UTC().Format(time.RFC3339),
		Models: map[string]pricing.Rate{
			"meta/muse": {Prompt: "0.001", Completion: "0.002"},
		},
	}
}

func TestAccountingAdapterEstimatesBlockingResult(t *testing.T) {
	wrapped := withAccounting("meta", "muse", accountingAdapterStub{result: core.ModelResult{
		Usage: core.TokenUsage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
	}}, accountingSnapshot())

	got, err := wrapped.Generate(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	assert.Equal(t, "0.0080000000", got.Cost.AmountUSD)
	assert.Equal(t, core.COST_STATUS_ESTIMATED, got.Cost.Status)
}

func TestAccountingAdapterPreservesExactProviderCost(t *testing.T) {
	exact := core.ExactCostFromUSDTicks(25)
	wrapped := withAccounting("meta", "muse", accountingAdapterStub{result: core.ModelResult{
		Usage: core.TokenUsage{InputTokens: 2},
		Cost:  exact,
	}}, accountingSnapshot())

	got, err := wrapped.Generate(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	assert.Equal(t, exact, got.Cost)
}

func TestAccountingAdapterPricesOnlyTerminalStreamChunk(t *testing.T) {
	wrapped := withAccounting("meta", "muse", accountingAdapterStub{chunks: []core.ModelChunk{
		{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hello"},
		{Done: true, Usage: core.TokenUsage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5}},
	}}, accountingSnapshot())

	stream, err := wrapped.Stream(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	var chunks []core.ModelChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 2)
	assert.Empty(t, chunks[0].Cost.Status)
	assert.Equal(t, "0.0080000000", chunks[1].Cost.AmountUSD)
	assert.Equal(t, core.COST_STATUS_ESTIMATED, chunks[1].Cost.Status)
}

func TestAccountingAdapterMakesEveryOllamaResultFree(t *testing.T) {
	wrapped := withAccounting("ollama", "any-model", accountingAdapterStub{result: core.ModelResult{
		Usage: core.TokenUsage{InputTokens: 1000, OutputTokens: 1000},
	}}, pricing.Snapshot{})

	got, err := wrapped.Generate(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	assert.Equal(t, core.FreeCost(), got.Cost)
}

type imageAccountingStub struct {
	result ImageResult
}

func (s imageAccountingStub) GenerateImage(context.Context, ImageRequest) (ImageResult, error) {
	return s.result, nil
}

func TestImageAccountingCountsOutputsAndEstimatesTokenCost(t *testing.T) {
	wrapped := withImageAccounting("meta", "muse", imageAccountingStub{result: ImageResult{
		Images: []Image{{URL: "https://example.test/image.png"}},
		Usage:  ImageUsage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
	}}, accountingSnapshot())

	got, err := wrapped.GenerateImage(context.Background(), ImageRequest{})
	require.NoError(t, err)
	assert.Equal(t, 1, got.Usage.GeneratedImages)
	assert.Equal(t, "0.0080000000", got.Cost.AmountUSD)
	assert.Equal(t, core.COST_STATUS_ESTIMATED, got.Cost.Status)
}

type transcriberAccountingStub struct{}

func (transcriberAccountingStub) Transcribe(context.Context, TranscribeRequest) (TranscribeResult, error) {
	return TranscribeResult{Text: "hello"}, nil
}

func TestTranscriberAccountingCarriesAudioDurationAndUnknownPrice(t *testing.T) {
	wrapped := withTranscriberAccounting(
		"elevenlabs",
		"scribe_v2",
		transcriberAccountingStub{},
		pricing.Snapshot{},
	)

	got, err := wrapped.Transcribe(context.Background(), TranscribeRequest{
		Audio: AudioSource{Bytes: []byte("audio"), DurationMilliseconds: 1250},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1250), got.Usage.AudioDurationMilliseconds)
	assert.Equal(t, core.COST_STATUS_UNPRICED, got.Cost.Status)
}

type streamingSpeechAccountingStub struct{}

func (streamingSpeechAccountingStub) GenerateSpeech(
	context.Context,
	SpeechRequest,
) (SpeechResult, error) {
	return SpeechResult{Info: SpeechInfo{DurationMs: 500}}, nil
}

func (streamingSpeechAccountingStub) StreamSpeech(
	context.Context,
	SpeechRequest,
) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("audio")), nil
}

func TestSpeechAccountingPreservesStreamingAndRecordsUsage(t *testing.T) {
	wrapped := withSpeechAccounting(
		"elevenlabs",
		"eleven_multilingual_v2",
		streamingSpeechAccountingStub{},
		pricing.Snapshot{},
	)
	_, ok := wrapped.(SpeechStreamer)
	assert.True(t, ok)

	got, err := wrapped.GenerateSpeech(context.Background(), SpeechRequest{Text: "你好"})
	require.NoError(t, err)
	assert.Equal(t, 2, got.Usage.Characters)
	assert.Equal(t, int64(500), got.Usage.AudioDurationMilliseconds)
	assert.Equal(t, core.COST_STATUS_UNPRICED, got.Cost.Status)
}
