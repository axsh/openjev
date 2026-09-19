package engine

import (
	"context"

	"openjev/features/decision-test/internal/prompt"
)

type Label struct {
	Letter  string
	TokenID int
}

type TokenLogprob struct {
	ID      int
	Logprob float64
}

type ReadResult struct {
	Top              []TokenLogprob
	PromptTokens     int
	CompletionTokens int
	ElapsedMs        float64
}

type GenRaw struct {
	Text             string
	TTFTMs           float64
	TotalMs          float64
	PromptTokens     int
	CompletionTokens int
}

type Engine interface {
	ReadLabelLogprobs(ctx context.Context, messages []prompt.Message, labels []Label) (ReadResult, error)
	Generate(ctx context.Context, messages []prompt.Message) (GenRaw, error)
	Health(ctx context.Context) (reachable bool, err error)
}
