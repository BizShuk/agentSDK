package cmd

import (
	"fmt"
	"time"

	"github.com/bizshuk/agentsdk/video/audio"
	"github.com/bizshuk/agentsdk/video/ffmpegutil"
	"github.com/bizshuk/agentsdk/video/frames"
	"github.com/bizshuk/agentsdk/video/subtitles"
	"github.com/spf13/cobra"
)

var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "Video preprocessing utilities (audio, frames, subtitles extraction)",
}

var audioCmd = &cobra.Command{
	Use:   "audio <video>",
	Short: "Extract audio track from a video into a WAV file",
	Args:  cobra.ExactArgs(1),
	RunE:  runAudio,
}

var (
	audioOut          string
	audioSampleRateHz int
	audioChannels     int
)

var framesCmd = &cobra.Command{
	Use:   "frames <video>",
	Short: "Extract still frames from a video into a directory",
	Args:  cobra.ExactArgs(1),
	RunE:  runFrames,
}

var (
	framesOutDir    string
	framesInterval  time.Duration
	framesSceneThr  float64
	framesMaxFrames int
)

var subtitlesCmd = &cobra.Command{
	Use:   "subtitles <video>",
	Short: "Extract audio and transcribe it into a .srt subtitle file",
	Args:  cobra.ExactArgs(1),
	RunE:  runSubtitles,
}

var (
	subtitlesOut       string
	subtitlesWorkDir   string
	subtitlesKeepAudio bool
	subtitlesEngine    string
	whisperBin         string
	whisperModel       string
	whisperLang        string
	qwenScript         string
	qwenModel          string
	qwenLang           string
	qwenChunkDuration  time.Duration
)

func init() {
	// video audio
	audioCmd.Flags().StringVar(&audioOut, "out", "./audio.wav", "output .wav path")
	audioCmd.Flags().IntVar(&audioSampleRateHz, "sample-rate", audio.DefaultSampleRateHz, "sample rate in Hz")
	audioCmd.Flags().IntVar(&audioChannels, "channels", audio.DefaultChannels, "number of audio channels (1=mono, 2=stereo)")
	videoCmd.AddCommand(audioCmd)

	// video frames
	framesCmd.Flags().StringVar(&framesOutDir, "out", "./frames-out", "output directory for extracted frames")
	framesCmd.Flags().DurationVar(&framesInterval, "interval", frames.DefaultInterval, "sampling interval (e.g. 2s)")
	framesCmd.Flags().Float64Var(&framesSceneThr, "scene-threshold", 0, "additionally sample on scene changes above this score (0..1)")
	framesCmd.Flags().IntVar(&framesMaxFrames, "max-frames", 0, "cap on emitted frames (0 = unlimited)")
	videoCmd.AddCommand(framesCmd)

	// video subtitles
	subtitlesCmd.Flags().StringVar(&subtitlesOut, "out", "./out.srt", "output .srt path")
	subtitlesCmd.Flags().StringVar(&subtitlesWorkDir, "work-dir", "./subtitles-work", "scratch dir for the intermediate audio track")
	subtitlesCmd.Flags().BoolVar(&subtitlesKeepAudio, "keep-audio", false, "keep the intermediate .wav after transcription")
	subtitlesCmd.Flags().StringVar(&subtitlesEngine, "engine", "noop", "transcription engine: whisper | qwen3 | noop")

	subtitlesCmd.Flags().StringVar(&whisperBin, "whisper-bin", "", "path to whisper.cpp whisper-cli/main binary (--engine whisper)")
	subtitlesCmd.Flags().StringVar(&whisperModel, "whisper-model", "", "path to a ggml whisper model file (--engine whisper)")
	subtitlesCmd.Flags().StringVar(&whisperLang, "whisper-lang", "auto", "whisper language code, or auto (--engine whisper)")

	subtitlesCmd.Flags().StringVar(&qwenScript, "qwen-script", "video/subtitles/pyasr/qwen_transcribe.py", "path to qwen_transcribe.py (--engine qwen3)")
	subtitlesCmd.Flags().StringVar(&qwenModel, "qwen-model", "", "mlx-community model id, empty = wrapper script default (--engine qwen3)")
	subtitlesCmd.Flags().StringVar(&qwenLang, "qwen-lang", "", "language code, empty = auto-detect mixed EN/ZH (--engine qwen3)")
	subtitlesCmd.Flags().DurationVar(&qwenChunkDuration, "qwen-chunk-duration", 10*time.Second, "subtitle-cue chunk length (--engine qwen3)")
	videoCmd.AddCommand(subtitlesCmd)
}

func runAudio(cmd *cobra.Command, args []string) error {
	videoFile := args[0]
	ctx := cmd.Context()

	if _, err := audio.Extract(ctx, videoFile, audioOut, audio.Options{
		SampleRateHz: audioSampleRateHz,
		Channels:     audioChannels,
	}); err != nil {
		return err
	}

	fmt.Printf("extracted audio to %s\n", audioOut)
	return nil
}

func runFrames(cmd *cobra.Command, args []string) error {
	videoFile := args[0]
	ctx := cmd.Context()

	if dur, err := ffmpegutil.Probe(ctx, videoFile); err == nil {
		fmt.Printf("source duration: %s\n", dur)
	}

	got, err := frames.Extract(ctx, videoFile, framesOutDir, frames.Options{
		Interval:       framesInterval,
		SceneThreshold: framesSceneThr,
		MaxFrames:      framesMaxFrames,
	})
	if err != nil {
		return err
	}

	fmt.Printf("extracted %d frame(s) into %s\n", len(got), framesOutDir)
	for _, f := range got {
		fmt.Printf("  %s\t@%s\n", f.Path, f.Timestamp)
	}
	return nil
}

func runSubtitles(cmd *cobra.Command, args []string) error {
	videoFile := args[0]
	ctx := cmd.Context()

	var transcriber subtitles.Transcriber
	switch subtitlesEngine {
	case "whisper":
		if whisperBin == "" || whisperModel == "" {
			return fmt.Errorf("--engine whisper requires --whisper-bin and --whisper-model")
		}
		transcriber = subtitles.WhisperCPPTranscriber{
			BinPath:   whisperBin,
			ModelPath: whisperModel,
			Language:  whisperLang,
		}
	case "qwen3":
		transcriber = subtitles.QwenMLXTranscriber{
			ScriptPath:    qwenScript,
			Model:         qwenModel,
			Language:      qwenLang,
			ChunkDuration: qwenChunkDuration,
		}
	case "noop":
		fmt.Println("warning: --engine noop produces 0 segments; pass --engine whisper or --engine qwen3 for real transcription")
		transcriber = subtitles.NoopTranscriber{}
	default:
		return fmt.Errorf("unknown --engine %q: want whisper | qwen3 | noop", subtitlesEngine)
	}

	segments, err := subtitles.Generate(ctx, videoFile, subtitlesWorkDir, transcriber, subtitlesKeepAudio)
	if err != nil {
		return err
	}
	if err := subtitles.WriteSRT(segments, subtitlesOut); err != nil {
		return err
	}

	fmt.Printf("wrote %d segment(s) to %s\n", len(segments), subtitlesOut)
	return nil
}
