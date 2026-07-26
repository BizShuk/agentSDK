package source

import (
	"context"

	"github.com/bizshuk/agentsdk/prompt"
)

// SkillSource adapts a skill registry's progressive-disclosure listing —
// names and descriptions only, with bodies loaded on demand by the skill
// tool rather than pushed into every request.
//
// SkillSource takes a SkillProvider interface rather than *skill.Registry
// so prompt/source does not need to import skill. *skill.Registry
// satisfies the interface implicitly (it has a SystemPrompt() method),
// so callers that already hold a registry just pass it positionally.
//
// A nil SkillProvider is a valid input: the source contributes nothing
// when no skill registry is wired into the call site, which is the
// correct behaviour for tiers and tests that exercise a single Source.
func SkillSource(p SkillProvider) prompt.Source {
	return prompt.SourceFunc(func(_ context.Context, _ prompt.Req) ([]prompt.Section, error) {
		if p == nil {
			return nil, nil
		}
		return []prompt.Section{{
			Slot:  prompt.SLOT_SYSTEM,
			Name:  "skills",
			Text:  p.SystemPrompt(),
			Order: prompt.ORDER_SKILLS,
		}}, nil
	})
}
