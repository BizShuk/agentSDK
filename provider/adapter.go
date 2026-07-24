package registry

import "github.com/bizshuk/agentsdk/core"

// Adapter is the full contract every provider adapter implements.
//
// It bundles the runtime-facing core.Provider contract (which runtime
// and proxy call directly) with the registration metadata the registry
// and CLI listings rely on. Every provider/<name>/provider.go asserts
// `var _ registry.Adapter = (*Provider)(nil)` so the compile-time proof
// lives next to the implementation.
//
// Name and Metadata are provider-side data; the registry reads them
// after construction. For pre-construction access (e.g. credential
// resolution and `--list-models` before any credential is loaded), the
// registry uses Entry.Metadata directly — Adapter is the
// post-construction view.
type Adapter interface {
	core.Provider

	// Name returns "<family>:<model>" — convenience accessor used in
	// logs and wire-format metadata. Family alone is ID().
	Name() string

	// Metadata is the registration-time description of the adapter.
	// Label renders in CLI menus; Note explains the credential model;
	// APIKeyEnv is the ordered credential env list (highest precedence
	// first); BaseURLEnv is the endpoint override env var (empty =
	// adapter built-in default).
	Metadata() Metadata
}

// Metadata describes an adapter for registration and CLI listing.
//
// The struct lives on Adapter rather than on Entry because Label and
// Note describe the same artifact as the Provider methods — splitting
// them across Entry and the Provider would let the two drift apart.
// Entry.Metadata and Adapter.Metadata() must return identical values;
// register.go is responsible for wiring both from one literal (see
// the adapterMetadata() helper in each provider/<name>/register.go).
type Metadata struct {
	Label      string
	Note       string
	APIKeyEnv  []string
	BaseURLEnv string
}
