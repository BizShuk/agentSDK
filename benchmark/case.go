package benchmark

import "slices"

// Kind selects which provider capability a case exercises.
type Kind string

const (
	KIND_CHAT       Kind = "chat"       // text (+ optional image/audio input) → text
	KIND_IMAGE      Kind = "image"      // prompt (+ optional reference image) → image
	KIND_SPEECH     Kind = "speech"     // text → audio
	KIND_TRANSCRIBE Kind = "transcribe" // audio file → text
	KIND_VIDEO      Kind = "video"      // prompt → mp4
	KIND_MUSIC      Kind = "music"      // prompt + lyrics → audio
)

// Case is one predefined input to run against a provider-model pair.
type Case struct {
	Name string
	Kind Kind

	// Prompt is the text input; for KIND_SPEECH it is the text to synthesize.
	Prompt string

	// Model overrides the media model for non-chat kinds. Empty keeps the
	// adapter's own default. Chat always uses Target.Model.
	Model string

	// InputFile is a media input path, resolved against the benchmark root
	// when relative: an image or audio file for chat, a reference image for
	// image-to-image, the audio to transcribe for KIND_TRANSCRIBE.
	InputFile string

	// Lyrics feeds KIND_MUSIC providers that require them.
	Lyrics string
}

// WithModel returns a copy of cases with every empty Model pinned to model —
// how a pair package names the media model it benchmarks instead of riding
// the adapter default. A Model already set on a case is kept.
func WithModel(model string, cases []Case) []Case {
	out := slices.Clone(cases)
	for i := range out {
		if out[i].Model == "" {
			out[i].Model = model
		}
	}
	return out
}

// ChatCases exercises the chat surface: plain text, light reasoning, and
// vision input.
func ChatCases() []Case {
	return []Case{
		{
			Name:   "text-basic",
			Kind:   KIND_CHAT,
			Prompt: "Reply with exactly one word: pong",
		},
		{
			Name:   "text-reasoning",
			Kind:   KIND_CHAT,
			Prompt: "Which number is larger, 9.11 or 9.9? Answer in one short sentence.",
		},
		{
			Name:      "vision-describe",
			Kind:      KIND_CHAT,
			Prompt:    "Describe this image in one short sentence.",
			InputFile: "testdata/shape.png",
		},
	}
}

// ImageCases exercises text-to-image generation.
func ImageCases() []Case {
	return []Case{
		{
			Name:   "text-to-image",
			Kind:   KIND_IMAGE,
			Prompt: "A minimal flat illustration of a red circle on a white background",
		},
	}
}

// SpeechCases exercises text-to-speech synthesis.
func SpeechCases() []Case {
	return []Case{
		{
			Name:   "text-to-speech",
			Kind:   KIND_SPEECH,
			Prompt: "Hello from the agent SDK benchmark.",
		},
	}
}

// TranscribeCases exercises speech-to-text. The bundled tone.wav is a plain
// sine tone, so the transcript is expected to be empty — the case verifies
// the upload/decode pipeline, not recognition quality. Point InputFile at a
// real recording for a meaningful transcript.
func TranscribeCases() []Case {
	return []Case{
		{
			Name:      "speech-to-text",
			Kind:      KIND_TRANSCRIBE,
			InputFile: "testdata/tone.wav",
		},
	}
}

// VideoCases exercises text-to-video generation. Generation is asynchronous
// upstream and routinely takes minutes.
func VideoCases() []Case {
	return []Case{
		{
			Name:   "text-to-video",
			Kind:   KIND_VIDEO,
			Prompt: "A paper airplane gliding across a clear blue sky",
		},
	}
}

// MusicCases exercises text-to-music generation.
func MusicCases() []Case {
	return []Case{
		{
			Name:   "text-to-music",
			Kind:   KIND_MUSIC,
			Prompt: "An upbeat cheerful acoustic guitar tune",
			Lyrics: "##\nSunny day, on my way\nSing along, all day long\n##",
		},
	}
}
