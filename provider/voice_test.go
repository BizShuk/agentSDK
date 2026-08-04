package provider_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

type fakeSpeechGenerator struct{}

func (fakeSpeechGenerator) GenerateSpeech(
	context.Context,
	provider.SpeechRequest,
) (provider.SpeechResult, error) {
	return provider.SpeechResult{}, nil
}

type fakeVoiceListingSpeech struct{ fakeSpeechGenerator }

func (fakeVoiceListingSpeech) ListVoices(
	context.Context,
	provider.VoiceListRequest,
) (provider.VoiceListResult, error) {
	return provider.VoiceListResult{}, nil
}

type fakeStreamingVoiceListingSpeech struct{ fakeVoiceListingSpeech }

func (fakeStreamingVoiceListingSpeech) StreamSpeech(
	context.Context,
	provider.SpeechRequest,
) (io.ReadCloser, error) {
	return nil, nil
}

func TestWithSpeechDecoratorPreservesOptionalCapabilities(t *testing.T) {
	decorator := func(context.Context) (core.Auth, error) { return core.Auth{}, nil }

	plain := provider.WithSpeechDecorator("fake", fakeSpeechGenerator{}, decorator)
	_, lists := plain.(provider.VoiceLister)
	_, streams := plain.(provider.SpeechStreamer)
	assert.False(t, lists, "plain generator must not advertise a voice catalog")
	assert.False(t, streams, "plain generator must not advertise streaming")

	listing := provider.WithSpeechDecorator("fake", fakeVoiceListingSpeech{}, decorator)
	_, lists = listing.(provider.VoiceLister)
	_, streams = listing.(provider.SpeechStreamer)
	assert.True(t, lists, "voice catalog must survive decoration")
	assert.False(t, streams)

	both := provider.WithSpeechDecorator("fake", fakeStreamingVoiceListingSpeech{}, decorator)
	_, lists = both.(provider.VoiceLister)
	_, streams = both.(provider.SpeechStreamer)
	assert.True(t, lists)
	assert.True(t, streams)
}
