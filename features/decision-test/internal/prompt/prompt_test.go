package prompt

import (
	"strings"
	"testing"

	"openjev/features/decision-test/internal/domain"
)

const accountState = "A customer says a password reset succeeded, but every login attempt still returns ‘account locked’. Two unlock emails were requested and neither arrived."

func accountCriteria() []domain.Criterion {
	return []domain.Criterion{
		{Key: "account_access", Text: "account_access: Account access support"},
		{Key: "billing", Text: "billing: Billing support"},
		{Key: "close", Text: "close: Close as resolved"},
	}
}

func TestDirectAccount(t *testing.T) {
	msgs := Messages(accountState, "Which queue should handle this request?", accountCriteria(), domain.MethodDirect)
	if msgs[0].Content != systemPrompt {
		t.Fatalf("system %q", msgs[0].Content)
	}
	want := "State:\n" + accountState + "\n\nQuestion:\nWhich queue should handle this request?\n\nAllowed options:\nA. account_access: Account access support\nB. billing: Billing support\nC. close: Close as resolved\n\nReply with exactly one option letter from: A, B, C."
	if msgs[1].Content != want {
		t.Fatalf("user\n%s\nwant\n%s", msgs[1].Content, want)
	}
	if strings.Contains(msgs[1].Content, `"queue"`) {
		t.Fatal("question id leaked into the prompt")
	}
}

func TestGenerationInstruction(t *testing.T) {
	msgs := Messages(accountState, "Which queue should handle this request?", accountCriteria(), domain.MethodGeneration)
	if !strings.Contains(msgs[1].Content, generationInstruction) {
		t.Fatal("generation instruction missing")
	}
	if strings.Contains(msgs[1].Content, "Reply with exactly one option letter") {
		t.Fatal("direct instruction leaked into generation")
	}
	if !strings.Contains(msgs[1].Content, `{"A: Route north": 0.65, "B: Route south": 0.35}`) {
		t.Fatal("example missing")
	}
}

func TestNullDescription(t *testing.T) {
	criteria := []domain.Criterion{{Key: "billing", Text: "billing"}, {Key: "other", Text: "other: Else"}}
	msgs := Messages("s", "q", criteria, domain.MethodDirect)
	if !strings.Contains(msgs[1].Content, "A. billing\nB. other: Else\n") {
		t.Fatalf("options:\n%s", msgs[1].Content)
	}
}

func TestObjectState(t *testing.T) {
	msgs := Messages(`{"ticket":1}`, "q", accountCriteria()[:2], domain.MethodDirect)
	if !strings.HasPrefix(msgs[1].Content, "State:\n{\"ticket\":1}\n\nQuestion:\n") {
		t.Fatalf("state prefix:\n%s", msgs[1].Content)
	}
}

func TestExpectedKeys(t *testing.T) {
	keys := ExpectedGenerationKeys(accountCriteria())
	if keys[0] != "A: account_access: Account access support" || keys[2] != "C: close: Close as resolved" {
		t.Fatalf("keys %v", keys)
	}
}
