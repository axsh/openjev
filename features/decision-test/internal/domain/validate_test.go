package domain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	account, err := os.ReadFile(filepath.Join("..", "..", "testdata", "account.json"))
	if err != nil {
		t.Fatal(err)
	}
	two := []byte(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`)
	tests := []struct {
		name    string
		raw     []byte
		wantErr string
		check   func(t *testing.T, req Request)
	}{
		{
			name: "ok_account",
			raw:  account,
			check: func(t *testing.T, req Request) {
				if len(req.Questions) != 1 || req.Questions[0].ID != "queue" {
					t.Fatalf("question: %+v", req.Questions)
				}
				got := make([]string, len(req.Questions[0].Criteria))
				for i, c := range req.Questions[0].Criteria {
					got[i] = c.Key
				}
				want := []string{"account_access", "billing", "close"}
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Fatalf("order %v", got)
				}
			},
		},
		{name: "state_empty", raw: []byte(`{"state":"","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`), wantErr: "state"},
		{name: "state_blank", raw: []byte(`{"state":"  ","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`), wantErr: "state"},
		{name: "state_null", raw: []byte(`{"state":null,"questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`), wantErr: "state"},
		{
			name: "state_object",
			raw:  []byte(`{"state":{"ticket":1},"questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`),
			check: func(t *testing.T, req Request) {
				text, err := req.State.PromptText()
				if err != nil {
					t.Fatal(err)
				}
				if text != `{"ticket":1}` {
					t.Fatalf("prompt text %q", text)
				}
			},
		},
		{name: "criteria_one", raw: []byte(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A"}}}}`), wantErr: "2"},
		{name: "criteria_21", raw: criteriaN(21), wantErr: "20"},
		{name: "empty_key", raw: []byte(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"":"x","b":"y"}}}}`), wantErr: "key"},
		{name: "duplicate_key", raw: []byte(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","a":"B","c":"C"}}}}`), wantErr: "duplicate"},
		{
			name: "null_description",
			raw:  []byte(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"billing":null,"other":"Else"}}}}`),
			check: func(t *testing.T, req Request) {
				if req.Questions[0].Criteria[0].Text != "billing" {
					t.Fatalf("text %q", req.Questions[0].Criteria[0].Text)
				}
			},
		},
		{
			name: "empty_description",
			raw:  []byte(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"billing":"","other":"Else"}}}}`),
			check: func(t *testing.T, req Request) {
				if req.Questions[0].Criteria[0].Text != "billing: " {
					t.Fatalf("text %q", req.Questions[0].Criteria[0].Text)
				}
			},
		},
		{name: "type_rank", raw: []byte(`{"state":"hello","questions":{"q":{"type":"rank","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`), wantErr: "not supported"},
		{name: "unknown_model", raw: []byte(`{"model":"other","state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`), wantErr: "model"},
		{
			name: "omit_model",
			raw:  two,
			check: func(t *testing.T, req Request) {
				if req.Model != "minicpm5-2b-q4_k_m" {
					t.Fatalf("model %q", req.Model)
				}
			},
		},
		{
			name: "method_default",
			raw:  two,
			check: func(t *testing.T, req Request) {
				if req.Method != MethodDirect {
					t.Fatalf("method %q", req.Method)
				}
			},
		},
		{name: "method_bad", raw: []byte(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}},"options":{"method":"stream"}}`), wantErr: "method"},
		{name: "no_questions", raw: []byte(`{"state":"hello","questions":{}}`), wantErr: "questions"},
		{name: "empty_instructions", raw: []byte(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"","criteria":{"a":"A","b":"B"}}}}`), wantErr: "instructions"},
		{
			name: "question_order",
			raw:  []byte(`{"state":"hello","questions":{"z":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}},"a":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`),
			check: func(t *testing.T, req Request) {
				if req.Questions[0].ID != "z" || req.Questions[1].ID != "a" {
					t.Fatalf("order %s %s", req.Questions[0].ID, req.Questions[1].ID)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := Validate(tt.raw, "minicpm5-2b-q4_k_m")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.check != nil {
				tt.check(t, req)
			}
		})
	}
}

func TestScoreAndNoul(t *testing.T) {
	score, err := os.ReadFile(filepath.Join("..", "..", "testdata", "score.json"))
	if err != nil {
		t.Fatal(err)
	}
	req, err := Validate(score, "minicpm5-2b-q4_k_m")
	if err != nil {
		t.Fatal(err)
	}
	q := req.Questions[0]
	if q.Type != "score" || q.Criteria[0].Key != "0" || q.Criteria[0].Text != "Calm" || q.Criteria[2].Text != "Very angry" {
		t.Fatalf("%+v", q.Criteria)
	}
	beforeType := []byte(`{"state":"hello","questions":{"frustration":{"criteria":["Calm","Angry"],"type":"score","instructions":"How?"}}}`)
	req, err = Validate(beforeType, "minicpm5-2b-q4_k_m")
	if err != nil || req.Questions[0].Criteria[1].Text != "Angry" {
		t.Fatal(err)
	}
	noul, err := os.ReadFile(filepath.Join("..", "..", "testdata", "noul.json"))
	if err != nil {
		t.Fatal(err)
	}
	req, err = Validate(noul, "minicpm5-2b-q4_k_m")
	if err != nil {
		t.Fatal(err)
	}
	if req.Questions[0].Criteria[0].Key != "true" || req.Questions[0].Criteria[1].Key != "false" {
		t.Fatalf("order %+v", req.Questions[0].Criteria)
	}
	if req.Questions[0].Criteria[0].Text != "Explicitly time-sensitive" {
		t.Fatalf("text %q", req.Questions[0].Criteria[0].Text)
	}
	omit, err := os.ReadFile(filepath.Join("..", "..", "testdata", "noul-omit.json"))
	if err != nil {
		t.Fatal(err)
	}
	req, err = Validate(omit, "minicpm5-2b-q4_k_m")
	if err != nil {
		t.Fatal(err)
	}
	if req.Questions[0].Criteria[0].Text != "true" || req.Questions[0].Criteria[1].Text != "false" {
		t.Fatalf("%+v", req.Questions[0].Criteria)
	}
	nullCrit := []byte(`{"state":"hello","questions":{"q":{"type":"noul","instructions":"Yes?","criteria":null}}}`)
	req, err = Validate(nullCrit, "minicpm5-2b-q4_k_m")
	if err != nil || req.Questions[0].Criteria[0].Text != "true" {
		t.Fatal(err)
	}
	rejects := []string{"score-one.json", "score-eleven.json", "score-object.json", "score-empty.json", "noul-true-only.json", "noul-extra-key.json", "type-rank.json"}
	for _, name := range rejects {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Validate(raw, "minicpm5-2b-q4_k_m"); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	rewritten, err := ApplyMethod(score, "generation")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rewritten), `"criteria":["Calm","Frustrated","Very angry"]`) {
		t.Fatalf("score rewrite %s", rewritten)
	}
	rewritten, err = ApplyMethod(noul, "both")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rewritten), `"true":"Explicitly time-sensitive"`) {
		t.Fatalf("noul rewrite %s", rewritten)
	}
}

func criteriaN(n int) []byte {
	var b strings.Builder
	b.WriteString(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"k`)
		b.WriteString(strings.Repeat("x", 0))
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(itoa(i))
		b.WriteString(`":"d"`)
	}
	b.WriteString(`}}}}`)
	return []byte(b.String())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [8]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
