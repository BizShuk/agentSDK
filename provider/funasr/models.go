package funasr

import "github.com/bizshuk/agentsdk/provider"

// DefaultCatalog returns the bundled FunASR model catalog.
//
// Model id 是 server 端 models.json 的 registry key，不是 modelscope/hf 路徑；
// 部署端（~/projects/platform/inf 的 funasr service）負責把 id 對應到實際
// checkpoint。ContextWindow / MaxTokens 保持零：ASR 以音訊長度計量，不以 token。
//
// 推薦（🏆）與理由：
//   - qwen3-asr-0.6b — 最佳綜合性價比：中/英/粵全五星、52 語言/方言，0.6B
//     體積在本地部署仍有高速度。
//   - qwen3-asr-1.7b — 精度優先：同語言覆蓋，換 3 倍參數量買準確度，
//     速度可接受時的首選。
//   - fun-asr-nano — 中文複雜場景：0.8B，中/英/日+方言，FunASR 原生支援最完整。
//   - sensevoice — CPU/低成本首選：234M 最小最快、50+ 語言，
//     也是本 adapter 的預設 model。
//   - whisper-large-v3-turbo — Whisper 實用首選：~809M 保留 99+ 語言覆蓋，
//     速度遠勝 large-v3，作為跨語言 baseline。
func DefaultCatalog() []provider.ModelSpec {
	transcribeOnly := func(id, family string) provider.ModelSpec {
		return provider.ModelSpec{
			ID:               id,
			Family:           family,
			Capabilities:     []provider.Capability{provider.CAPABILITY_TRANSCRIBE},
			InputModalities:  []provider.Modality{provider.MODALITY_AUDIO},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
		}
	}
	return []provider.ModelSpec{
		// 🏆 最佳綜合性價比：0.6B，中/英/粵 ⭐5，52 語言/方言，速度 ⭐4。
		transcribeOnly("qwen3-asr-0.6b", "qwen3-asr"),
		// 🏆 精度優先：1.7B，語言覆蓋同 0.6B，準確度更高、速度 ⭐3。
		transcribeOnly("qwen3-asr-1.7b", "qwen3-asr"),
		// 🏆 中文複雜場景：0.8B，中/英/日+方言，FunASR 原生模型。
		transcribeOnly("fun-asr-nano", "fun-asr"),
		// FunASR 多語言：0.8B，31 語言，粵語 ⭐4。
		transcribeOnly("fun-asr-mlt-nano", "fun-asr"),
		// 🏆 CPU/低成本首選：234M，50+ 語言，速度 ⭐5——本 adapter 預設。
		transcribeOnly("sensevoice", "sensevoice"),
		// 中文 + timestamp/hotword：220M，主要中/粵，速度 ⭐5。
		transcribeOnly("paraformer-zh", "paraformer"),
		// 老牌高精度 baseline：1.55B，99+ 語言，速度 ⭐2。
		transcribeOnly("whisper-large-v3", "whisper"),
		// 🏆 Whisper 實用首選：~809M，99+ 語言，速度 ⭐4。
		transcribeOnly("whisper-large-v3-turbo", "whisper"),
		// 英文高速 ASR：主要英文、無粵語；需部署端掛 NeMo runtime 才可用。
		transcribeOnly("parakeet-tdt", "parakeet"),
	}
}
