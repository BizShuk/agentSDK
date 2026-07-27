package core

import "context"

type generateOnlyProvider struct{}

func (generateOnlyProvider) Generate(context.Context, ModelRequest) (ModelResult, error) {
	return ModelResult{}, nil
}

type streamOnlyProvider struct{}

func (streamOnlyProvider) Stream(context.Context, ModelRequest) (<-chan ModelChunk, error) {
	ch := make(chan ModelChunk)
	close(ch)
	return ch, nil
}

var (
	_ Provider       = generateOnlyProvider{}
	_ StreamProvider = streamOnlyProvider{}
)
