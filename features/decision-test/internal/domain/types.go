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

// Text holds a JSON string, object, or array and renders it for the prompt.
// Strings are used as-is; objects and arrays are embedded as their JSON text.
type Text struct {
	raw json.RawMessage
}

// State is the shared context of a request; it follows the Text rules.
type State = Text

// TextOf wraps a plain string as Text for callers that build requests in Go.
func TextOf(s string) Text {
	raw, err := json.Marshal(s)
	if err != nil {
		return Text{}
	}
	return Text{raw: raw}
}

type Question struct {
	ID           string
	Type         string
	Instructions Text
	Criteria     []Criterion
	criteriaRaw  json.RawMessage
	sawCriteria  bool
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
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty" doc:"1 - H(p) / ln(n). H is the natural-log entropy of the option distribution. 1 when peaked, 0 when uniform. Not Jev's unpublished definition."`
	Method        Method             `json:"method"`
	Generation    *Generation        `json:"generation,omitempty"`
	Timings       *Timings           `json:"timings,omitempty"`
}

type Generation struct {
	Choice          string             `json:"-"`
	EmitChoice      bool               `json:"-"`
	Score           *float64           `json:"score,omitempty"`
	Noul            *float64           `json:"noul,omitempty"`
	Probabilities   map[string]float64 `json:"probabilities,omitempty"`
	Valid           bool               `json:"valid"`
	ValidationError string             `json:"validation_error"`
	GeneratedText   string             `json:"generated_text"`
	TTFTMs          float64            `json:"ttft_ms"`
	TotalMs         float64            `json:"total_ms"`
	OutputTokens    int                `json:"output_tokens"`
}

func (g Generation) MarshalJSON() ([]byte, error) {
	type wire struct {
		Choice          *string            `json:"choice,omitempty"`
		Score           *float64           `json:"score,omitempty"`
		Noul            *float64           `json:"noul,omitempty"`
		Probabilities   map[string]float64 `json:"probabilities,omitempty"`
		Valid           bool               `json:"valid"`
		ValidationError string             `json:"validation_error"`
		GeneratedText   string             `json:"generated_text"`
		TTFTMs          float64            `json:"ttft_ms"`
		TotalMs         float64            `json:"total_ms"`
		OutputTokens    int                `json:"output_tokens"`
	}
	out := wire{
		Score:           g.Score,
		Noul:            g.Noul,
		Probabilities:   g.Probabilities,
		Valid:           g.Valid,
		ValidationError: g.ValidationError,
		GeneratedText:   g.GeneratedText,
		TTFTMs:          g.TTFTMs,
		TotalMs:         g.TotalMs,
		OutputTokens:    g.OutputTokens,
	}
	if g.EmitChoice {
		choice := g.Choice
		out.Choice = &choice
	}
	return json.Marshal(out)
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
	Workers        int            `json:"workers" doc:"Configured worker count of the decision pool."`
	QueueDepth     int            `json:"queue_depth" doc:"Tasks currently waiting in the request queue."`
	Error          string         `json:"error,omitempty"`
}
