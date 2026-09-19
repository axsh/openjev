package prompt

import (
	"strings"

	"openjev/features/decision-test/internal/domain"
)

const systemPrompt = "Make the requested decision from the supplied state. Follow the output format exactly."

const generationInstruction = `Estimate the probability that each allowed option is the correct decision.
Return only one JSON object mapping each option to its probability. Form every key as "<label>: <full option text>" using the allowed options above.
For example, if the unrelated options were "A. Route north" and "B. Route south", valid output would be:
{"A: Route north": 0.65, "B: Route south": 0.35}
For the actual decision, include every supplied option exactly once and in order. Each value must be a JSON number from 0 to 1, and the probabilities must sum to 1. Output JSON only, with no markdown or explanation.`

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func Messages(state string, instructions string, criteria []domain.Criterion, mode domain.Method) []Message {
	labels := make([]string, len(criteria))
	lines := make([]string, len(criteria))
	for i, item := range criteria {
		labels[i] = string(rune('A' + i))
		lines[i] = labels[i] + ". " + item.Text
	}
	instruction := generationInstruction
	if mode == domain.MethodDirect {
		instruction = "Reply with exactly one option letter from: " + strings.Join(labels, ", ") + "."
	}
	user := "State:\n" + state + "\n\nQuestion:\n" + instructions + "\n\nAllowed options:\n" + strings.Join(lines, "\n") + "\n\n" + instruction
	return []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: user},
	}
}

func ExpectedGenerationKeys(criteria []domain.Criterion) []string {
	keys := make([]string, len(criteria))
	for i, item := range criteria {
		keys[i] = string(rune('A'+i)) + ": " + item.Text
	}
	return keys
}
