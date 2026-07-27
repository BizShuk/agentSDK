package svc

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Audio returns a typed UnsupportedCapabilityError for unsupported audio requests.
func Audio(providerName string) error {
	return &provider.UnsupportedCapabilityError{
		Provider:   providerName,
		Capability: provider.Capability("audio"),
	}
}
