package provider_test

// Link every built-in adapter into the registry test binary. Adapters
// self-register from their package init(); without these blank imports
// the test binary would only see the registrations triggered by the
// adapters the test's own imports happen to pull in transitively, which
// is non-deterministic across `go test` invocations.
//
// New adapters added to provider/all get covered automatically.
import (
	_ "github.com/bizshuk/agentsdk/provider/all"
)
