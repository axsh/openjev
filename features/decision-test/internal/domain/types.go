package domain

import "encoding/json"

type Method string

const (
	MethodDirect     Method = "direct"
	MethodGeneration Method = "generation"
	MethodBoth       Method = "both"
)

type Request struct {
	Model     string
	State     State
	Questions []Question
	Method    Method
}

type State struct {
	raw json.RawMessage
}

type Question struct {
	ID           string
	Type         string
	Instructions string
	Criteria     []Criterion
}

type Criterion struct {
	Key         string
	Description *string
	Text        string
}

type Response struct {
	Model     string            `json:"model"`
	Answers   map[string]Answer `json:"answers"`
	Usage     Usage             `json:"usage"`
	ElapsedMs int64             `json:"elapsedMs"`
}

type Answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty" doc:"1 - H(p) / ln(n). H is the natural-log entropy of the option distribution. 1 when peaked, 0 when uniform. Not Jev's unpublished definition."`
	Method        Method             `json:"method"`
	Generation    *Generation        `json:"generation,omitempty"`
	Timings       *Timings           `json:"timings,omitempty"`
}

type Generation struct {
	Choice          string             `json:"choice"`
	Probabilities   map[string]float64 `json:"probabilities,omitempty"`
	Valid           bool               `json:"valid"`
	ValidationError string             `json:"validation_error"`
	GeneratedText   string             `json:"generated_text"`
	TTFTMs          float64            `json:"ttft_ms"`
	TotalMs         float64            `json:"total_ms"`
	OutputTokens    int                `json:"output_tokens"`
}

type Timings struct {
	DirectMs     float64  `json:"direct_ms,omitempty"`
	GenerationMs float64  `json:"generation_ms,omitempty"`
	Ratio        *float64 `json:"ratio,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Health struct {
	Ready          bool           `json:"ready"`
	Model          string         `json:"model"`
	LlamaReachable bool           `json:"llama_reachable"`
	LabelTokenIDs  map[string]int `json:"label_token_ids"`
	Error          string         `json:"error,omitempty"`
}
