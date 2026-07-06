package security

import "encoding/json"

// marshalAny centralizes JSON encoding for the spotlight middleware.
// Split out from spotlight.go so the security package only imports
// encoding/json here, keeping the main middleware file narrow.
func marshalAny(v any) ([]byte, error) {
	return json.Marshal(v)
}