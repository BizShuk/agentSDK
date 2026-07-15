#!/usr/bin/env python3
"""Standalone Qwen3-ASR transcription wrapper around mlx_audio.stt.

Runs independently of the Go binary — `python3 qwen_transcribe.py --audio
foo.wav --output-path /tmp/out` produces /tmp/out.json with the same
{"text", "segments": [{"text", "start", "end"}]} shape the Go
QwenMLXTranscriber (internal/preprocess/subtitles/qwen_mlx.go) parses.

Exists as a thin wrapper (not a CLI flag on mlx_audio's own `generate.py`)
because that CLI's --language always sends a real language code (default
"en") with no way to request language=None — Qwen3-ASR's auto-detect mode
for mixed EN/ZH content, the reason this project (see
~/projects/playground/pkg/voiceon) picked Qwen3-ASR over whisper.cpp in the
first place. Calling generate_transcription() directly here lets --language
be omitted to mean auto-detect.

Requires: Apple Silicon (MLX has no other backend) + `pip install mlx-audio`.
The model (~700MB for the 0.6B variant) downloads to ~/.cache/huggingface/
on first use.
"""

import argparse
import sys


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--audio", required=True, help="path to a WAV file")
    parser.add_argument(
        "--output-path",
        required=True,
        help="output path prefix; mlx_audio writes <output-path>.json",
    )
    parser.add_argument(
        "--model",
        default="mlx-community/Qwen3-ASR-0.6B-8bit",
        help="mlx-community model id (default matches voiceon's default)",
    )
    parser.add_argument(
        "--language",
        default=None,
        help="language code (e.g. en, zh); omit for auto-detect (mixed EN/ZH)",
    )
    parser.add_argument(
        "--chunk-duration",
        type=float,
        default=10.0,
        help="seconds per subtitle-cue chunk (default: 10.0 — mlx_audio's own "
        "CLI defaults to 30.0, too coarse for subtitle timing)",
    )
    parser.add_argument("--max-tokens", type=int, default=8192)
    args = parser.parse_args()

    try:
        from mlx_audio.stt.generate import generate_transcription
    except ImportError as exc:
        print(
            f"mlx-audio not installed (pip install mlx-audio): {exc}",
            file=sys.stderr,
        )
        return 1

    generate_transcription(
        model=args.model,
        audio=args.audio,
        output_path=args.output_path,
        format="json",
        language=args.language,
        chunk_duration=args.chunk_duration,
        max_tokens=args.max_tokens,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
