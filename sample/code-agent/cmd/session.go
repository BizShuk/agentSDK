package cmd

import (
	"context"

	"github.com/bizshuk/agentsdk/core"
)

// sessionRequest selects which conversation openState starts from.
type sessionRequest struct {
	continueLatest bool
	resumeID       string
	forkID         string
}

// openState returns the State to run: a fresh session, the latest one, a
// specific resume, or a fork copy.
func (p *agentParts) openState(ctx context.Context, req sessionRequest) (core.State, error) {
	store := p.AppConfig.StateStore
	switch {
	case req.forkID != "":
		meta, err := p.Sessions.Fork(ctx, req.forkID, "fork of "+req.forkID)
		if err != nil {
			return core.State{}, err
		}
		return store.Load(ctx, meta.ID)
	case req.resumeID != "":
		return store.Load(ctx, req.resumeID)
	case req.continueLatest:
		meta, err := p.Sessions.Latest(p.Cwd)
		if err != nil {
			return core.State{}, err
		}
		return store.Load(ctx, meta.ID)
	default:
		if _, err := p.Sessions.Begin(p.AppConfig.RunID, "", p.Cwd); err != nil {
			return core.State{}, err
		}
		return p.seed, nil
	}
}
