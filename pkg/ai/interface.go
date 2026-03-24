package ai

import "context"

// Client is the interface for interacting with an AI analysis service.
// Implementations cover Claude, OpenAI, Gemini, etc.
// All implementations must be safe for concurrent use.
type Client interface {
	// Analyze submits a Flux failure context and returns an analysis with a proposed fix.
	Analyze(ctx context.Context, req AnalysisRequest) (*AnalysisResult, error)

	// ValidateIntent submits the live runtime state and plan documents and returns
	// whether the running environment satisfies the declared intent.
	ValidateIntent(ctx context.Context, req IntentValidationRequest) (*IntentValidationResult, error)
}
