package spec

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Validate checks an EXPANDED config. Run Expand first, or call Prepare
// which sequences both — validating a raw config would reject empty
// fields that expansion was going to fill.
//
// What it does NOT check is as deliberate as what it does. A tier and an
// explicit block are never in conflict: `tier: oneshot` alongside a
// reasoning block, or alongside tools, is legal. Tier only decides which
// blocks default to on; every block stays independently overridable, and
// explicit always wins. The one-shot case is bounded structurally anyway
// — with no tools bound the model cannot emit a tool call, so the engine
// short-circuits to COMPLETED after a single model call whatever the
// reasoning style says.
//
// Errors are joined, so one call reports every problem rather than
// forcing a fix-and-rerun cycle.
func (c Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	rank, ok := Rank(c.Tier)
	if !ok {
		add("spec: unknown tier %q (want one of %v)", c.Tier, Tiers())
		return errors.Join(errs...)
	}

	// --- name: required exactly when something persists ---
	if c.Name == "" && persists(c) {
		add("spec: name is required when memory.store=%q (state and WAL live under ~/.config/<name>)", MEMORY_STORE_FILE)
	}
	if strings.ContainsAny(c.Name, `/\`) {
		add("spec: name %q must not contain a path separator", c.Name)
	}

	// --- model ---
	// checkVariant rejects the empty string as "call Expand before
	// Validate"; credential_kind allows "" on purpose (it means "auto"),
	// so the check is inlined here instead.
	credKinds := Values(VariantChoices("model.credential_kind"))
	if c.Model.CredentialKind != "" && !slices.Contains(credKinds, c.Model.CredentialKind) {
		add("spec: unknown model.credential_kind %q (want one of %v)", c.Model.CredentialKind, credKinds)
	}

	// --- reasoning ---
	styles := Values(StyleChoices())
	if !slices.Contains(styles, c.Reasoning.Style) {
		add("spec: unknown reasoning.style %q (want one of %v)", c.Reasoning.Style, styles)
	}
	seen := map[string]bool{}
	for _, s := range c.Reasoning.Enable {
		if !slices.Contains(styles, s) {
			add("spec: unknown reasoning.enable entry %q (want one of %v)", s, styles)
		}
		if seen[s] {
			add("spec: duplicate reasoning.enable entry %q", s)
		}
		seen[s] = true
	}
	// Style must be registered, else core.NewDecide dispatches to a
	// missing rule and emits NOTIFY error instead of reasoning. That is a
	// typo, not a design choice, so it fails here rather than at runtime.
	if slices.Contains(styles, c.Reasoning.Style) && !seen[c.Reasoning.Style] {
		add("spec: reasoning.style %q is not in reasoning.enable %v — the engine would dispatch to an unregistered rule",
			c.Reasoning.Style, c.Reasoning.Enable)
	}

	// --- limits ---
	if c.Limits.MaxTurns < 0 {
		add("spec: limits.max_turns must not be negative, got %d", c.Limits.MaxTurns)
	}
	if c.Limits.MaxRounds < 0 {
		add("spec: limits.max_rounds must not be negative, got %d", c.Limits.MaxRounds)
	}
	if c.Limits.MaxToolCalls < 0 {
		add("spec: limits.max_tool_calls must not be negative, got %d", c.Limits.MaxToolCalls)
	}
	// A round costs at least one turn, so a turn ceiling below the round
	// ceiling makes the round budget unreachable — always a swapped pair
	// rather than an intent.
	if c.Limits.MaxTurns > 0 && c.Limits.MaxRounds > 0 && c.Limits.MaxTurns < c.Limits.MaxRounds {
		add("spec: limits.max_turns (%d) is below limits.max_rounds (%d) — a round costs at least one turn, so the round budget could never be reached",
			c.Limits.MaxTurns, c.Limits.MaxRounds)
	}
	if c.Limits.MaxWallTime != "" {
		if _, err := time.ParseDuration(c.Limits.MaxWallTime); err != nil {
			add("spec: limits.max_wall_time %q is not a Go duration: %v", c.Limits.MaxWallTime, err)
		}
	}
	checkVariant(add, "limits.autonomy", c.Limits.Autonomy)

	// --- blocks ---
	if c.Middleware != nil {
		checkVariant(add, "middleware.preset", c.Middleware.Preset)
	}
	if c.Memory != nil {
		checkVariant(add, "memory.store", c.Memory.Store)
		checkVariant(add, "memory.compaction", c.Memory.Compaction)
	}
	if c.Tools != nil {
		builtin := Values(VariantChoices("tools.builtin"))
		for _, t := range c.Tools.Builtin {
			if !slices.Contains(builtin, t) {
				add("spec: unknown tools.builtin entry %q (want one of %v)", t, builtin)
			}
		}
	}
	if c.Safety != nil {
		checkVariant(add, "safety.mode", c.Safety.Mode)
		checkVariant(add, "safety.fallback", c.Safety.Fallback)
		for _, spec := range concat(c.Safety.Deny, c.Safety.Ask, c.Safety.Allow) {
			if !strings.Contains(spec, "(") || !strings.HasSuffix(spec, ")") {
				add("spec: safety rule %q must look like tool(target), e.g. bash(sudo:*)", spec)
			}
		}
	}
	if c.Prompt != nil {
		sources := Values(VariantChoices("prompt.sources"))
		for _, s := range c.Prompt.Sources {
			if !slices.Contains(sources, s) {
				add("spec: unknown prompt.sources entry %q (want one of %v)", s, sources)
			}
		}
		if c.Prompt.MaxBytes < 0 {
			add("spec: prompt.max_bytes must not be negative, got %d", c.Prompt.MaxBytes)
		}
	}
	if c.Subagents != nil {
		if c.Subagents.MaxDepth < 1 {
			add("spec: subagents.max_depth must be at least 1, got %d", c.Subagents.MaxDepth)
		}
		if c.Subagents.MaxTurns < 1 {
			add("spec: subagents.max_turns must be at least 1, got %d", c.Subagents.MaxTurns)
		}
	}
	if c.Output != nil {
		checkVariant(add, "output.format", c.Output.Format)
	}

	// --- cross-block coherence ---
	// The skill index and the task tool are prompt/tool contributions; a
	// block that can never take effect is a silent no-op, and silent
	// no-ops in config are how people lose an afternoon.
	if c.Skills != nil && c.Prompt != nil && !slices.Contains(c.Prompt.Sources, SOURCE_SKILLS) {
		add("spec: skills is enabled but prompt.sources lacks %q — the skill index would never reach the model",
			SOURCE_SKILLS)
	}
	if c.Safety != nil && c.Tools == nil && c.Subagents == nil {
		add("spec: safety is enabled but no tools are — nothing to gate")
	}
	if c.Sessions != nil && !persists(c) {
		add("spec: sessions needs memory.store=%q — lineage is metadata over the state store", MEMORY_STORE_FILE)
	}
	if rank == tierRank[TIER_ONESHOT] && persists(c) && c.Name == "" {
		add("spec: tier %q with persistence still needs a name", TIER_ONESHOT)
	}

	return errors.Join(errs...)
}

// Prepare is the normal entry point: expand the tier, then validate the
// result. Order matters — validation reads fields expansion fills in.
func (c Config) Prepare() (Config, error) {
	out, err := c.Expand()
	if err != nil {
		return Config{}, err
	}
	if err := out.Validate(); err != nil {
		return Config{}, err
	}
	return out, nil
}

// persists reports whether the config writes state to disk.
func persists(c Config) bool {
	return c.Memory != nil && c.Memory.Store == MEMORY_STORE_FILE
}

// checkVariant rejects a value that is not among a key's choices. An
// empty value is reported too: by validation time Expand should have
// filled it, so empty means the caller skipped Expand.
func checkVariant(add func(string, ...any), key, got string) {
	allowed := Values(VariantChoices(key))
	if len(allowed) == 0 {
		return
	}
	if got == "" {
		add("spec: %s is empty — call Expand before Validate", key)
		return
	}
	if !slices.Contains(allowed, got) {
		add("spec: unknown %s %q (want one of %v)", key, got, allowed)
	}
}

func concat(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}
