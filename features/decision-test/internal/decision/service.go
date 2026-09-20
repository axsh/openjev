package decision

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/engine"
	"openjev/features/decision-test/internal/logger"
	"openjev/features/decision-test/internal/prompt"
)

var ErrEngine = errors.New("engine")

// defaultStallTimeout guards Services built without an explicit StallTimeout.
const defaultStallTimeout = 15 * time.Second

type Service struct {
	Engine  engine.Engine
	Labels  []engine.Label
	Log     *logger.Logger
	ModelID string
	// Pool answers the questions of every request; see StartPool.
	Pool *Pool
	// StallTimeout is how long a handler waits without any submit or receive
	// progress before it returns the answers collected so far.
	StallTimeout time.Duration
}

type usage struct {
	in  int
	out int
}

// StartPool creates the singleton pool backed by this service and starts its workers.
func (s *Service) StartPool(ctx context.Context, workers int) {
	s.Pool = newPool(workers, s.one, s.Log)
	s.Pool.Start(ctx)
}

// Run submits one task per question to the pool and collects the answers from
// a response channel private to this call. If neither the submitted nor the
// received count changes for StallTimeout, it stops waiting and returns the
// answers collected so far; missing question IDs are simply absent.
func (s *Service) Run(ctx context.Context, req domain.Request) (domain.Response, error) {
	state, err := req.State.PromptText()
	if err != nil {
		return domain.Response{}, err
	}
	if s.Pool == nil {
		return domain.Response{}, errors.New("worker pool is not started")
	}
	stall := s.StallTimeout
	if stall <= 0 {
		stall = defaultStallTimeout
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	total := len(req.Questions)
	results := make(chan result, total)
	resp := domain.Response{Model: req.Model, Answers: map[string]domain.Answer{}}
	submitted, received := 0, 0
	timer := time.NewTimer(stall)
	defer timer.Stop()
	progress := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(stall)
	}
	start := time.Now()
	for received < total {
		var submit chan<- task // nil once every question is queued; a nil channel is never selected
		var next task
		if submitted < total {
			submit = s.Pool.queue
			next = task{ctx: ctx, state: state, question: req.Questions[submitted], method: req.Method, results: results}
		}
		select {
		case submit <- next:
			submitted++
			progress()
		case r := <-results:
			received++
			if r.err != nil {
				return domain.Response{}, r.err
			}
			resp.Answers[r.id] = r.answer
			resp.Usage.InputTokens += r.used.in
			resp.Usage.OutputTokens += r.used.out
			progress()
		case <-timer.C:
			missing := make([]string, 0, total-len(resp.Answers))
			for _, question := range req.Questions {
				if _, ok := resp.Answers[question.ID]; !ok {
					missing = append(missing, question.ID)
				}
			}
			s.Log.Warn("systemone stalled", "questions", total, "submitted", submitted, "received", received, "missing_ids", strings.Join(missing, ","), "stall_timeout_ms", stall.Milliseconds())
			return resp, nil
		case <-ctx.Done():
			return domain.Response{}, ctx.Err()
		}
	}
	s.Log.Debug("systemone completed", "questions", total, "answered", len(resp.Answers), "missing", total-len(resp.Answers), "duration_ms", float64(time.Since(start).Microseconds())/1000)
	return resp, nil
}

func (s *Service) one(ctx context.Context, state string, question domain.Question, method domain.Method) (domain.Answer, usage, error) {
	if len(s.Labels) < len(question.Criteria) {
		return domain.Answer{}, usage{}, fmt.Errorf("%w: not enough labels", ErrEngine)
	}
	instructions, err := question.Instructions.PromptText()
	if err != nil {
		// Validation rejects these before dispatch; keep the engine error shape if it ever slips through.
		return domain.Answer{}, usage{}, fmt.Errorf("%w: %v", ErrEngine, err)
	}
	engLabels := s.Labels[:len(question.Criteria)]
	decLabels := make([]Label, len(engLabels))
	for i, label := range engLabels {
		decLabels[i] = Label{Letter: label.Letter, TokenID: label.TokenID}
	}
	answer := domain.Answer{Type: question.Type, Method: method}
	var used usage
	var directMs float64
	if method == domain.MethodDirect || method == domain.MethodBoth {
		read, err := s.Engine.ReadLabelLogprobs(ctx, prompt.Messages(state, instructions, question.Criteria, domain.MethodDirect, question.Type), engLabels)
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
		applyDirect(&answer, question, Softmax(logits))
		directMs = read.ElapsedMs
		used.in += read.PromptTokens
		used.out += read.CompletionTokens
		s.Log.Debug("direct completed", "question_id", question.ID, "question_type", question.Type)
		if method == domain.MethodDirect {
			answer.Timings = &domain.Timings{DirectMs: directMs}
		}
	}
	if method == domain.MethodGeneration || method == domain.MethodBoth {
		s.Log.Debug("generation started", "question_id", question.ID, "question_type", question.Type)
		raw, err := s.Engine.Generate(ctx, prompt.Messages(state, instructions, question.Criteria, domain.MethodGeneration, question.Type))
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
			EmitChoice:      question.Type == "choice",
		}
		if checked.Valid {
			applyGeneration(generated, &answer, question, checked, method)
		}
		answer.Generation = generated
		used.in += raw.PromptTokens
		used.out += raw.CompletionTokens
		if method == domain.MethodGeneration {
			answer.Timings = &domain.Timings{GenerationMs: raw.TotalMs}
		} else {
			ratio := raw.TotalMs / directMs
			answer.Timings = &domain.Timings{DirectMs: directMs, GenerationMs: raw.TotalMs, Ratio: &ratio}
		}
	}
	return answer, used, nil
}

func applyDirect(answer *domain.Answer, question domain.Question, probs []float64) {
	switch question.Type {
	case "score":
		score := WeightedScore(probs)
		confidence := Confidence(probs)
		answer.Score = &score
		answer.Confidence = &confidence
		answer.Legend = map[string]string{}
		answer.Probabilities = map[string]float64{}
		for i, item := range question.Criteria {
			answer.Legend[item.Key] = item.Text
			answer.Probabilities[item.Key] = probs[i]
		}
	case "noul":
		noul := probs[0]
		answer.Noul = &noul
	default:
		index := ArgMax(probs)
		confidence := Confidence(probs)
		answer.Choice = question.Criteria[index].Key
		answer.Probabilities = map[string]float64{}
		for i, item := range question.Criteria {
			answer.Probabilities[item.Key] = probs[i]
		}
		answer.Confidence = &confidence
	}
}

func applyGeneration(generated *domain.Generation, answer *domain.Answer, question domain.Question, checked GenResult, method domain.Method) {
	probs := ordered(checked.Probabilities, question.Criteria)
	switch question.Type {
	case "score":
		score := WeightedScore(probs)
		generated.Score = &score
		generated.Probabilities = checked.Probabilities
		if method == domain.MethodGeneration {
			confidence := Confidence(probs)
			answer.Score = &score
			answer.Probabilities = checked.Probabilities
			answer.Confidence = &confidence
		}
	case "noul":
		noul := probs[0]
		generated.Noul = &noul
		generated.Probabilities = checked.Probabilities
		if method == domain.MethodGeneration {
			answer.Noul = &noul
		}
	default:
		generated.Choice = checked.ChoiceKey
		generated.Probabilities = checked.Probabilities
		if method == domain.MethodGeneration {
			confidence := Confidence(probs)
			answer.Choice = checked.ChoiceKey
			answer.Probabilities = checked.Probabilities
			answer.Confidence = &confidence
		}
	}
}

func ordered(probs map[string]float64, criteria []domain.Criterion) []float64 {
	out := make([]float64, len(criteria))
	for i, item := range criteria {
		out[i] = probs[item.Key]
	}
	return out
}
