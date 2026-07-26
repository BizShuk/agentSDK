package source

import "github.com/bizshuk/agentsdk/prompt"

// PersonaSource contributes fixed identity text.
//
// It is a thin alias for Static at ORDER_PERSONA, named because "the
// persona" is a concept the config layer spells out and a reader should
// be able to grep for.
func PersonaSource(persona string) prompt.Source {
	return prompt.Static(prompt.SLOT_SYSTEM, "persona", persona, prompt.ORDER_PERSONA)
}
