package agent_test

// Link every built-in provider adapter into the agent test binary.
// Agent.ProviderChoices and ModelChoices enumerate registry entries;
// without these blank imports the test binary would see an empty
// registry and the tracking / default tests would fail.
import (
	_ "github.com/bizshuk/agentsdk/provider/all"
)