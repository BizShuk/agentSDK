package antigravity

import "fmt"

// Validate runs the DTO sanity check before we hand the body to the HTTP
// client. The gateway answers malformed bodies with a bare 400 and no
// field name, so catching the shape errors here is the difference between
// an actionable message and a debugging session.
//
// Returns nil on success or a descriptive error otherwise.
func (r CloudCodeRequest) Validate() error {
	if r.Model == "" {
		return fmt.Errorf("antigravity: model is required")
	}
	if r.Project == "" {
		return fmt.Errorf("antigravity: project is required")
	}
	if len(r.Request.Contents) == 0 {
		return fmt.Errorf("antigravity: at least one content entry is required")
	}
	for i, c := range r.Request.Contents {
		if c.Role != "user" && c.Role != "model" {
			return fmt.Errorf("antigravity: contents[%d] role %q must be user|model (system goes to systemInstruction)", i, c.Role)
		}
		if len(c.Parts) == 0 {
			return fmt.Errorf("antigravity: contents[%d] has no parts", i)
		}
	}
	return nil
}
