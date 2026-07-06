package google

import "encoding/json"

// unmarshalJSON is split out from provider.go so the latter stays
// narrow and avoids the encoding/json import at the top.
func unmarshalJSON(data []byte, v any) error { return json.Unmarshal(data, v) }