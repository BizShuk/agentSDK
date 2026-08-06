package benchmark

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// readMedia loads one input file and returns its bytes plus lowercase
// extension without the dot. Relative paths resolve against the benchmark
// root so every provider-model package shares testdata/.
func readMedia(root, file string) ([]byte, string, error) {
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, file)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read input %s: %w", file, err)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return data, ext, nil
}

// mimeByExt maps a media file extension to its MIME type.
func mimeByExt(ext string) string {
	switch ext {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	default:
		return ""
	}
}

// mediaPart wraps input bytes as the matching chat message part.
func mediaPart(data []byte, ext string) (core.Part, error) {
	mime := mimeByExt(ext)
	switch {
	case strings.HasPrefix(mime, "image/"):
		return core.Part{Kind: core.PART_KIND_IMAGE, Image: data, ImageMIME: mime}, nil
	case strings.HasPrefix(mime, "audio/"):
		return core.Part{Kind: core.PART_KIND_AUDIO, Audio: data, AudioMIME: mime}, nil
	default:
		return core.Part{}, fmt.Errorf("unsupported input extension %q", ext)
	}
}

// extByMIME picks an output file extension from a MIME type.
func extByMIME(mime, fallback string) string {
	switch strings.ToLower(mime) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "audio/wav":
		return ".wav"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	default:
		return fallback
	}
}

// extByAudioFormat picks an output extension from a provider format label
// such as "mp3", "mp3_44100_128", or "pcm_16000".
func extByAudioFormat(format string) string {
	f := strings.ToLower(format)
	switch {
	case strings.Contains(f, "pcm"), strings.Contains(f, "wav"):
		return ".wav"
	case strings.Contains(f, "flac"):
		return ".flac"
	default:
		return ".mp3"
	}
}
