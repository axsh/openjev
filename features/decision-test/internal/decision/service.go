package decision

import (
	"context"
	"errors"
	"fmt"

	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/engine"
	"openjev/features/decision-test/internal/logger"
	"openjev/features/decision-test/internal/prompt"
)

var ErrEngine = errors.New("engine")

type Service struct {
	Engine  engine.Engine
	Labels  []engine.Label
	Log     *logger.Logger
	ModelID string
}

type usage struct {
	in  int
	out int
}

func (s *Service) Run(ctx context.Context, req domain.Request) (domain.Response, error) {
	state, err := req.State.PromptText()
	if err != nil {
		return domain.Response{}, err
	}
	resp := domain.Response{Model: req.Model, Answers: map[string]domain.Answer{}}
	for _, question := range req.Questions {
		answer, used, err := s.one(ctx, state, question, req.Method)
		if err != nil {
			return domain.Response{}, err
		}
		resp.Answers[question.ID] = answer
		resp.Usage.InputTokens += used.in
		resp.Usage.OutputTokens += used.out
	}
	return resp, nil
}

func (s *Service) one(ctx context.Context, state string, question domain.Question, method domain.Method) (domain.Answer, usage, error) {
	if len(s.Labels) < len(question.Criteria) {
		return domain.Answer{}, usage{}, fmt.Errorf("%w: not enough labels", ErrEngine)
	}
	engLabels := s.Labels[:len(question.Criteria)]
	decLabels := make([]Label, len(engLabels))
	for i, label := range engLabels {
		decLabels[i] = Label{Letter: label.Letter, TokenID: label.TokenID}
	}
	answer := domain.Answer{Type: "choice", Method: method}
	var used usage
	var directMs float64
	if method == domain.MethodDirect || method == domain.MethodBoth {
		read, err := s.Engine.ReadLabelLogprobs(ctx, prompt.Messages(state, question.Instructions, question.Criteria, domain.MethodDirect), engLabels)
		if err != nil {
			return domain.Answer{}, usage{}, fmt.Errorf("%w: %v", ErrEngine, err)
		}
		top := make([]TokenLogprob, len(read.Top))
		for i, item := range read.Top {
			top[i] = TokenLogprob{ID: item.ID, Logprob: item.Logprob}
		}
		logits, err := PickLogprobs(top, decLabels)
		if err != nil {
			return domain.Answer{}, usage{}, fmt.Errorf("%w: %v", ErrEngine, err)
		}
		probs := Softmax(logits)
		index := ArgMax(probs)
		confidence := Confidence(probs)
		answer.Choice = question.Criteria[index].Key
		answer.Probabilities = map[string]float64{}
		for i, item := range question.Criteria {
			answer.Probabilities[item.Key] = probs[i]
		}
		answer.Confidence = &confidence
		directMs = read.ElapsedMs
		used.in += read.PromptTokens
		used.out += read.CompletionTokens
		s.Log.Debug("direct completed", "question_id", question.ID)
		if method == domain.MethodDirect {
			answer.Timings = &domain.Timings{DirectMs: directMs}
		}
	}
	if method == domain.MethodGeneration || method == domain.MethodBoth {
		s.Log.Debug("generation started", "question_id", question.ID)
		raw, err := s.Engine.Generate(ctx, prompt.Messages(state, question.Instructions, question.Criteria, domain.MethodGeneration))
		if err != nil {
			return domain.Answer{}, usage{}, fmt.Errorf("%w: %v", ErrEngine, err)
		}
		checked := ValidateGeneration(raw.Text, question.Criteria)
		generated := &domain.Generation{
			Valid:           checked.Valid,
			ValidationError: checked.ValidationError,
			GeneratedText:   raw.Text,
			TTFTMs:          raw.TTFTMs,
			TotalMs:         raw.TotalMs,
			OutputTokens:    raw.CompletionTokens,
		}
		if checked.Valid {
			generated.Choice = checked.ChoiceKey
			generated.Probabilities = checked.Probabilities
		}
		answer.Generation = generated
		used.in += raw.PromptTokens
		used.out += raw.CompletionTokens
		if method == domain.MethodGeneration {
			if checked.Valid {
				answer.Choice = checked.ChoiceKey
				answer.Probabilities = checked.Probabilities
				confidence := Confidence(ordered(checked.Probabilities, question.Criteria))
				answer.Confidence = &confidence
			}
			answer.Timings = &domain.Timings{GenerationMs: raw.TotalMs}
		} else {
			ratio := raw.TotalMs / directMs
			answer.Timings = &domain.Timings{DirectMs: directMs, GenerationMs: raw.TotalMs, Ratio: &ratio}
		}
	}
	return answer, used, nil
}

func ordered(probs map[string]float64, criteria []domain.Criterion) []float64 {
	out := make([]float64, len(criteria))
	for i, item := range criteria {
		out[i] = probs[item.Key]
	}
	return out
}
