package cmd

import (
	"fmt"
	"time"

	"github.com/bizshuk/agentsdk/utils/video/audio"
	"github.com/bizshuk/agentsdk/utils/video/ffmpegutil"
	"github.com/bizshuk/agentsdk/utils/video/frames"
	"github.com/bizshuk/agentsdk/utils/video/subtitles"
	"github.com/spf13/cobra"
)

const defaultQwenScript = "utils/video/subtitles/pyasr/qwen_transcribe.py"

type commandFlags struct {
	audioOut          string
	audioSampleRateHz int
	audioChannels     int

	framesOutDir    string
	framesInterval  time.Duration
	framesSceneThr  float64
	framesMaxFrames int

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
}

func NewCommand() *cobra.Command {
	var flags commandFlags

	videoCmd := &cobra.Command{
		Use:   "video",
		Short: "Video preprocessing utilities (audio, frames, subtitles extraction)",
	}

	audioCmd := &cobra.Command{
		Use:   "audio <video>",
		Short: "Extract audio track from a video into a WAV file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAudio(cmd, args, &flags)
		},
	}
	audioCmd.Flags().StringVar(&flags.audioOut, "out", "./audio.wav", "output .wav path")
	audioCmd.Flags().IntVar(&flags.audioSampleRateHz, "sample-rate", audio.DefaultSampleRateHz, "sample rate in Hz")
	audioCmd.Flags().IntVar(&flags.audioChannels, "channels", audio.DefaultChannels, "number of audio channels (1=mono, 2=stereo)")

	framesCmd := &cobra.Command{
		Use:   "frames <video>",
		Short: "Extract still frames from a video into a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFrames(cmd, args, &flags)
		},
	}
	framesCmd.Flags().StringVar(&flags.framesOutDir, "out", "./frames-out", "output directory for extracted frames")
	framesCmd.Flags().DurationVar(&flags.framesInterval, "interval", frames.DefaultInterval, "sampling interval (e.g. 2s)")
	framesCmd.Flags().Float64Var(&flags.framesSceneThr, "scene-threshold", 0, "additionally sample on scene changes above this score (0..1)")
	framesCmd.Flags().IntVar(&flags.framesMaxFrames, "max-frames", 0, "cap on emitted frames (0 = unlimited)")

	subtitlesCmd := &cobra.Command{
		Use:   "subtitles <video>",
		Short: "Extract audio and transcribe it into a .srt subtitle file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSubtitles(cmd, args, &flags)
		},
	}
	subtitlesCmd.Flags().StringVar(&flags.subtitlesOut, "out", "./out.srt", "output .srt path")
	subtitlesCmd.Flags().StringVar(&flags.subtitlesWorkDir, "work-dir", "./subtitles-work", "scratch dir for the intermediate audio track")
	subtitlesCmd.Flags().BoolVar(&flags.subtitlesKeepAudio, "keep-audio", false, "keep the intermediate .wav after transcription")
	subtitlesCmd.Flags().StringVar(&flags.subtitlesEngine, "engine", "noop", "transcription engine: whisper | qwen3 | noop")
	subtitlesCmd.Flags().StringVar(&flags.whisperBin, "whisper-bin", "", "path to whisper.cpp whisper-cli/main binary (--engine whisper)")
	subtitlesCmd.Flags().StringVar(&flags.whisperModel, "whisper-model", "", "path to a ggml whisper model file (--engine whisper)")
	subtitlesCmd.Flags().StringVar(&flags.whisperLang, "whisper-lang", "auto", "whisper language code, or auto (--engine whisper)")
	subtitlesCmd.Flags().StringVar(&flags.qwenScript, "qwen-script", defaultQwenScript, "path to qwen_transcribe.py (--engine qwen3)")
	subtitlesCmd.Flags().StringVar(&flags.qwenModel, "qwen-model", "", "mlx-community model id, empty = wrapper script default (--engine qwen3)")
	subtitlesCmd.Flags().StringVar(&flags.qwenLang, "qwen-lang", "", "language code, empty = auto-detect mixed EN/ZH (--engine qwen3)")
	subtitlesCmd.Flags().DurationVar(&flags.qwenChunkDuration, "qwen-chunk-duration", 10*time.Second, "subtitle-cue chunk length (--engine qwen3)")

	videoCmd.AddCommand(audioCmd, framesCmd, subtitlesCmd)
	return videoCmd
}

func runAudio(cmd *cobra.Command, args []string, flags *commandFlags) error {
	if _, err := audio.Extract(cmd.Context(), args[0], flags.audioOut, audio.Options{
		SampleRateHz: flags.audioSampleRateHz,
		Channels:     flags.audioChannels,
	}); err != nil {
		return err
	}
	fmt.Printf("extracted audio to %s\n", flags.audioOut)
	return nil
}

func runFrames(cmd *cobra.Command, args []string, flags *commandFlags) error {
	if dur, err := ffmpegutil.Probe(cmd.Context(), args[0]); err == nil {
		fmt.Printf("source duration: %s\n", dur)
	}

	got, err := frames.Extract(cmd.Context(), args[0], flags.framesOutDir, frames.Options{
		Interval:       flags.framesInterval,
		SceneThreshold: flags.framesSceneThr,
		MaxFrames:      flags.framesMaxFrames,
	})
	if err != nil {
		return err
	}

	fmt.Printf("extracted %d frame(s) into %s\n", len(got), flags.framesOutDir)
	for _, frame := range got {
		fmt.Printf("  %s\t@%s\n", frame.Path, frame.Timestamp)
	}
	return nil
}

func runSubtitles(cmd *cobra.Command, args []string, flags *commandFlags) error {
	var transcriber subtitles.Transcriber
	switch flags.subtitlesEngine {
	case "whisper":
		if flags.whisperBin == "" || flags.whisperModel == "" {
			return fmt.Errorf("--engine whisper requires --whisper-bin and --whisper-model")
		}
		transcriber = subtitles.WhisperCPPTranscriber{
			BinPath:   flags.whisperBin,
			ModelPath: flags.whisperModel,
			Language:  flags.whisperLang,
		}
	case "qwen3":
		transcriber = subtitles.QwenMLXTranscriber{
			ScriptPath:    flags.qwenScript,
			Model:         flags.qwenModel,
			Language:      flags.qwenLang,
			ChunkDuration: flags.qwenChunkDuration,
		}
	case "noop":
		fmt.Println("warning: --engine noop produces 0 segments; pass --engine whisper or --engine qwen3 for real transcription")
		transcriber = subtitles.NoopTranscriber{}
	default:
		return fmt.Errorf("unknown --engine %q: want whisper | qwen3 | noop", flags.subtitlesEngine)
	}

	segments, err := subtitles.Generate(cmd.Context(), args[0], flags.subtitlesWorkDir, transcriber, flags.subtitlesKeepAudio)
	if err != nil {
		return err
	}
	if err := subtitles.WriteSRT(segments, flags.subtitlesOut); err != nil {
		return err
	}

	fmt.Printf("wrote %d segment(s) to %s\n", len(segments), flags.subtitlesOut)
	return nil
}
