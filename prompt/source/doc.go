// Package source assembles the built-in content sources that turn a
// prompt.Req into []prompt.Section. Persona, context files, environment,
// and budget reminder are content concerns and need only prompt + stdlib;
// the fifth source, SkillSource, is the one Source whose two halves live
// in packages that must not see each other (skill does not know prompt
// exists), so it takes a SkillProvider interface here rather than
// importing skill.
//
// This package imports only prompt and the standard library; it does
// not import skill or agent.
package source
