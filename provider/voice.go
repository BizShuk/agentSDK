package provider

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
)

// VoiceLister is the optional speech capability for enumerating the voices a
// text-to-speech surface can synthesize with. It is paired with
// SpeechGenerator the way SpeechStreamer is: an adapter implements it only
// when the vendor exposes a voice catalog, and callers type-assert the value
// NewSpeech returns to find out.
//
// The request vocabulary follows ElevenLabs' voice-search surface — the
// richest catalog among registered vendors — reduced to the subset other
// vendors can honor: free-text search, one category filter, and forward-only
// pagination. Vendors without server-side search filter locally; vendors
// without pagination return the full list and reject a resume token.
type VoiceLister interface {
	ListVoices(ctx context.Context, request VoiceListRequest) (VoiceListResult, error)
}

// VoiceListRequest is the provider-neutral input to a voice catalog query.
// Zero values list everything the vendor exposes, one vendor-default page at
// a time.
type VoiceListRequest struct {
	// Search filters by free text across id, name, description, and labels.
	Search string `json:"search,omitempty"`

	// Category selects one provider-defined voice class — "premade" or
	// "cloned" on ElevenLabs, "system" or "voice_cloning" on MiniMax.
	// Empty lists every class.
	Category string `json:"category,omitempty"`

	// PageSize bounds one page. Zero keeps the provider default.
	PageSize int `json:"page_size,omitempty"`

	// PageToken resumes after a prior page's NextPageToken.
	PageToken string `json:"page_token,omitempty"`

	// Auth overrides construction-time or decorated credentials for this call.
	Auth core.Auth `json:"auth,omitempty"`
}

// Validate checks provider-independent voice list invariants.
func (r VoiceListRequest) Validate() error {
	if r.PageSize < 0 {
		return fmt.Errorf("voice page size must not be negative")
	}
	return nil
}

// Voice is one synthesizable voice. Fields stay zero when the vendor does not
// report them.
type Voice struct {
	ID          string            `json:"id"`
	Name        string            `json:"name,omitempty"`
	Category    string            `json:"category,omitempty"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	PreviewURL  string            `json:"preview_url,omitempty"`

	// CreatedAtUnix is seconds since epoch. Vendors reporting only a date
	// resolve it to UTC midnight.
	CreatedAtUnix int64 `json:"created_at_unix,omitempty"`
}

// VoiceListResult is one page of the catalog. NextPageToken stays empty on
// vendors without server-side pagination even when HasMore is true.
type VoiceListResult struct {
	Voices        []Voice `json:"voices"`
	HasMore       bool    `json:"has_more,omitempty"`
	TotalCount    int     `json:"total_count,omitempty"`
	NextPageToken string  `json:"next_page_token,omitempty"`
}
