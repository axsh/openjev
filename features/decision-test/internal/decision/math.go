package decision

import (
	"fmt"
	"math"
)

type TokenLogprob struct {
	ID      int
	Logprob float64
}

type Label struct {
	Letter  string
	TokenID int
}

func Softmax(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}
	max := values[0]
	for _, value := range values[1:] {
		if value > max {
			max = value
		}
	}
	out := make([]float64, len(values))
	sum := 0.0
	for i, value := range values {
		out[i] = math.Exp(value - max)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

func Confidence(probs []float64) float64 {
	n := len(probs)
	if n <= 1 {
		return 1
	}
	entropy := 0.0
	for _, p := range probs {
		if p > 0 {
			entropy -= p * math.Log(p)
		}
	}
	return 1 - entropy/math.Log(float64(n))
}

func WeightedScore(probs []float64) float64 {
	score := 0.0
	for i, p := range probs {
		score += float64(i) * p
	}
	return score
}

func ArgMax(probs []float64) int {
	if len(probs) == 0 {
		return -1
	}
	index := 0
	for i := 1; i < len(probs); i++ {
		if probs[i] > probs[index] {
			index = i
		}
	}
	return index
}

func PickLogprobs(top []TokenLogprob, labels []Label) ([]float64, error) {
	out := make([]float64, len(labels))
	for i, label := range labels {
		found := false
		for _, item := range top {
			if item.ID == label.TokenID {
				out[i] = item.Logprob
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("missing option logit for %s", label.Letter)
		}
	}
	return out, nil
}
