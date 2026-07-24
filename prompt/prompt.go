// Package prompt owns one decision: what content goes into the model's
// context window, and in what order.
//
// That decision was previously spread across four places — the contextfile
// package read the AGENTS.md hierarchy, skill.Registry rendered its index,
// the composition root concatenated them by hand, and memory trimmed
// whatever came out. Nobody owned "what do we send this turn". The
// context-file loader now lives here (see LoadContextFiles) because it is
// fixed behaviour with no customisation seam; skill still produces its
// own content because it has a registry, types, and a spawner.
//
// The division of labour with memory is deliberate and worth keeping:
//
//	prompt decides WHAT GOES IN     — policy, pure data assembly
//	memory decides WHAT GETS CUT    — mechanism, token counting and compaction
//
// Merging them would make injection and trimming mutually recursive.
//
// prompt imports core and the standard library only. Producers like
// skill are adapted into Sources by the composition root, so no harness
// package imports another.
package prompt

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// Slot is where a piece of content belongs in the conversation.
type Slot string

const (
	// SLOT_SYSTEM is seeded once: identity, project instructions, the
	// skill index, environment facts.
	SLOT_SYSTEM Slot = "system"
	// SLOT_USER is the expanded user input for a turn — command
	// expansion, @-mentions, template rendering already applied.
	SLOT_USER Slot = "user"
	// SLOT_REMINDER is re-injected every turn: outstanding work, budget
	// left, rules worth restating. It rides along with the user message
	// rather than rewriting the system prompt, which would break prompt
	// caching on every turn.
	SLOT_REMINDER Slot = "reminder"
)

// Order values for the built-in system contributors, spaced so callers
// can slot custom sources between them.
//
// The sequence runs from least to most volatile, and that is the point:
// prompt caching keys on a stable prefix, so anything that changes every
// turn has to come last or it invalidates everything before it.
const (
	ORDER_PERSONA  = 10 // almost never changes
	ORDER_FILES    = 20 // changes when AGENTS.md changes
	ORDER_SKILLS   = 30 // changes when a skill is installed
	ORDER_ENV      = 40 // changes every run
	ORDER_REMINDER = 50 // changes every turn
)

// DEFAULT_MAX_BYTES caps assembled content. It matches the per-section
// budget applied to the context-file loader so a project's instructions
// cannot blow past the total prompt budget on their own.
const DEFAULT_MAX_BYTES = 256 * 1024

// Section is one contributor's output for one slot.
type Section struct {
	Slot  Slot
	Name  string // source identifier, for debugging and budget reporting
	Text  string
	Order int // lower sorts first; ties keep source registration order

	// seq is the contributing source's index, filled in by Builder so
	// that equal Order values sort by registration rather than by
	// whatever order the sources happened to return.
	seq int
}

// Req is what a Source gets to look at. It is read-only input: a Source
// must not mutate State, and Builder passes it by value to make that the
// path of least resistance.
type Req struct {
	Cwd   string
	Turn  int
	Input string     // this turn's raw user input
	State core.State // for sources that report on progress or budget
}

// Source contributes sections. Implementations live wherever their data
// lives — the composition root adapts skill and anything
// application-specific into this interface; context-file loading lives
// inside this package as LoadContextFiles.
type Source interface {
	Sections(ctx context.Context, req Req) ([]Section, error)
}

// SourceFunc adapts a plain function to Source.
type SourceFunc func(ctx context.Context, req Req) ([]Section, error)

// Sections implements Source.
func (f SourceFunc) Sections(ctx context.Context, req Req) ([]Section, error) { return f(ctx, req) }

// Static returns a Source that always contributes one fixed section —
// the shape a persona or any other constant text takes.
func Static(slot Slot, name, text string, order int) Source {
	return SourceFunc(func(context.Context, Req) ([]Section, error) {
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return []Section{{Slot: slot, Name: name, Text: text, Order: order}}, nil
	})
}

// Builder assembles sections into messages.
//
// The zero value is usable and produces nothing, which matters: a tier
// with no prompt sources configured should seed an empty conversation
// rather than require a nil check at every call site.
type Builder struct {
	Sources  []Source
	MaxBytes int // <= 0 uses DEFAULT_MAX_BYTES

	// OnBudget, when set, is called for each section dropped by the byte
	// budget. Silent truncation of a project's instructions is the kind
	// of thing that should be visible in a log.
	OnBudget func(dropped Section, limit int)
}

// Seed builds the opening messages: one system message merging every
// SLOT_SYSTEM section, then the user message if Req.Input is set.
//
// One system message rather than several, because providers disagree on
// the shape — Anthropic takes a separate system parameter, OpenAI Chat
// takes a system-role message. Keeping State to a single system message
// leaves that difference in the adapter where it belongs.
func (b Builder) Seed(ctx context.Context, req Req) ([]core.Message, error) {
	secs, err := b.collect(ctx, req)
	if err != nil {
		return nil, err
	}

	var out []core.Message
	if sys := b.merge(secs, SLOT_SYSTEM); sys != "" {
		out = append(out, message(core.ROLE_SYSTEM, sys))
	}
	if user := joinNonEmpty(b.merge(secs, SLOT_USER), req.Input); user != "" {
		out = append(out, message(core.ROLE_USER, user))
	}
	return out, nil
}

// Turn builds the messages for a subsequent turn: reminders first, then
// the user input, merged into a single user message.
//
// Reminders ride with the user message on purpose. Rewriting the system
// prompt each turn would invalidate the provider's cached prefix, which
// is the entire reason the system slot is ordered stable-to-volatile.
func (b Builder) Turn(ctx context.Context, req Req) ([]core.Message, error) {
	secs, err := b.collect(ctx, req)
	if err != nil {
		return nil, err
	}
	text := joinNonEmpty(b.merge(secs, SLOT_REMINDER), b.merge(secs, SLOT_USER), req.Input)
	if text == "" {
		return nil, nil
	}
	return []core.Message{message(core.ROLE_USER, text)}, nil
}

// collect runs every source, sorts the result, and applies the byte
// budget. A source that fails aborts the build: half a system prompt is
// worse than a clear error, because the model would silently operate
// without instructions it was supposed to have.
func (b Builder) collect(ctx context.Context, req Req) ([]Section, error) {
	var secs []Section
	for i, src := range b.Sources {
		if src == nil {
			continue
		}
		got, err := src.Sections(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("prompt: source %d: %w", i, err)
		}
		for _, s := range got {
			if strings.TrimSpace(s.Text) == "" {
				continue
			}
			s.seq = i
			secs = append(secs, s)
		}
	}

	// Stable within an Order: registration order decides, so a caller can
	// control placement without inventing unique Order values.
	sort.SliceStable(secs, func(i, j int) bool {
		if secs[i].Order != secs[j].Order {
			return secs[i].Order < secs[j].Order
		}
		return secs[i].seq < secs[j].seq
	})

	return b.applyBudget(secs), nil
}

// applyBudget drops sections from the END once the limit is reached.
//
// Dropping the tail rather than truncating mid-text is what keeps the
// cache-stable prefix intact: persona and project instructions survive,
// the volatile environment block is sacrificed first. Truncating in the
// middle would corrupt whichever section straddled the boundary.
func (b Builder) applyBudget(secs []Section) []Section {
	limit := b.MaxBytes
	if limit <= 0 {
		limit = DEFAULT_MAX_BYTES
	}
	var used int
	for i, s := range secs {
		if used+len(s.Text) > limit {
			for _, dropped := range secs[i:] {
				if b.OnBudget != nil {
					b.OnBudget(dropped, limit)
				}
			}
			return secs[:i]
		}
		used += len(s.Text)
	}
	return secs
}

// merge concatenates one slot's sections, blank-line separated.
func (b Builder) merge(secs []Section, slot Slot) string {
	var parts []string
	for _, s := range secs {
		if s.Slot == slot {
			parts = append(parts, strings.TrimSpace(s.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

// joinNonEmpty concatenates the non-blank parts, blank-line separated.
func joinNonEmpty(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, "\n\n")
}

func message(role core.Role, text string) core.Message {
	return core.Message{
		Role:  role,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}},
		Ts:    time.Now().UTC(),
	}
}
