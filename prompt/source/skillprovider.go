package source

// SkillProvider is the seam between prompt/source and the skill registry.
// *skill.Registry satisfies this interface implicitly; consumers may pass
// any type whose SystemPrompt() renders the progressive-disclosure skill
// listing for SLOT_SYSTEM / ORDER_SKILLS.
//
// Declaring the interface here (rather than in skill) keeps the layering
// rule "skill does not know prompt exists" intact: skill never has to
// import prompt/source, and prompt/source never has to import skill.
type SkillProvider interface {
	SystemPrompt() string
}
