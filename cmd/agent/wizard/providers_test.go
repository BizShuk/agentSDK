package wizard_test

// Link every built-in provider adapter into the wizard test binary. The
// wizard enumerates model.provider choices via provider.Entries(); without
// these blank imports the test binary would see an empty registry and
// fail with "unknown field model.provider".
//
// Slim binaries that only ship a subset of adapters should import those
// specific packages instead of provider/all.
import (
	_ "github.com/bizshuk/agentsdk/provider/all"
)
