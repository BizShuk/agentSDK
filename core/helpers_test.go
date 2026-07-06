package core_test

import "encoding/json"

// jsonMarshal is a tiny helper to keep tests short.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }