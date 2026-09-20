package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"openjev/features/decision-test/internal/domain"
)

func TestLoadParallel(t *testing.T) {
	var inFlight int32
	var maxSeen int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if n <= old || atomic.CompareAndSwapInt32(&maxSeen, old, n) {
				break
			}
		}
		time.Sleep(80 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"answers":{"q":{"timings":{"direct_ms":1}}}}`))
	}))
	defer srv.Close()
	path := writeInput(t, `{"state":"hello","questions":{"q":{"type":"choice","instructions":"Which?","criteria":{"a":"A","b":"B"}}}}`)
	var stdout, stderr bytes.Buffer
	err := Load(context.Background(), LoadOptions{Input: path, Server: srv.URL, Concurrency: 10, Slots: 2, JSON: true}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&maxSeen) != 10 {
		t.Fatalf("max %d", maxSeen)
	}
	var report LoadReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Slots != 2 || report.Success != 10 || report.Errors != 0 || report.Statuses["200"] != 10 {
		t.Fatalf("%+v", report)
	}
	if report.DirectMsSum == nil || *report.DirectMsSum != 10 {
		t.Fatalf("direct %#v", report.DirectMsSum)
	}
	if report.Questions != 1 || report.Answers != 10 || report.AnswersMissing != 0 || report.QuestionsPerSec <= 0 || report.Workers != 0 {
		t.Fatalf("question fields %+v", report)
	}
}

const threeQuestions = `{"state":"hello","questions":{` +
	`"a":{"type":"choice","instructions":"Which a?","criteria":{"x":"X","y":"Y"}},` +
	`"b":{"type":"score","instructions":"How b?","criteria":["Low","High"]},` +
	`"c":{"type":"noul","instructions":"Is c?"}}}`

func TestLoadTakesQuestions(t *testing.T) {
	var bodies [][]byte
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, raw)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"answers":{"a":{"timings":{"direct_ms":2}},"b":{"timings":{"direct_ms":3}}}}`))
	}))
	defer srv.Close()
	path := writeInput(t, threeQuestions)
	var stdout, stderr bytes.Buffer
	err := Load(context.Background(), LoadOptions{Input: path, Server: srv.URL, Concurrency: 3, Slots: 1, Workers: 30, Questions: 2, JSON: true}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("%v stderr %s", err, stderr.String())
	}
	if len(bodies) != 3 {
		t.Fatalf("requests %d", len(bodies))
	}
	for _, body := range bodies {
		count, err := domain.QuestionCount(body)
		if err != nil || count != 2 {
			t.Fatalf("count %d err %v body %s", count, err, body)
		}
		if !bytes.Contains(body, []byte(`"a":`)) || !bytes.Contains(body, []byte(`"b":`)) || bytes.Contains(body, []byte(`"c":`)) {
			t.Fatalf("body %s", body)
		}
	}
	var report LoadReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Workers != 30 || report.Questions != 2 || report.Answers != 6 || report.AnswersMissing != 0 || report.Success != 3 {
		t.Fatalf("%+v", report)
	}
	if report.DirectMsSum == nil || *report.DirectMsSum != 15 {
		t.Fatalf("direct %#v", report.DirectMsSum)
	}
}

func TestLoadCountsMissingAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"answers":{"a":{},"b":{}}}`))
	}))
	defer srv.Close()
	path := writeInput(t, threeQuestions)
	var stdout, stderr bytes.Buffer
	err := Load(context.Background(), LoadOptions{Input: path, Server: srv.URL, Concurrency: 2, Slots: 1, Questions: 3, JSON: true}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing answers")
	}
	var report LoadReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Success != 2 || report.Errors != 0 || report.Questions != 3 || report.Answers != 4 || report.AnswersMissing != 2 {
		t.Fatalf("%+v", report)
	}
	if !strings.Contains(stderr.String(), "answers_missing 2") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestLoadQuestionsExceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer srv.Close()
	path := writeInput(t, threeQuestions)
	var stdout, stderr bytes.Buffer
	err := Load(context.Background(), LoadOptions{Input: path, Server: srv.URL, Concurrency: 1, Slots: 1, Questions: 4, JSON: true}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "input has 3 questions; --questions 4 exceeds it") {
		t.Fatalf("error %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("server was called %d times", hits.Load())
	}
}

func TestLoadHumanReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"answers":{"a":{},"b":{},"c":{}}}`))
	}))
	defer srv.Close()
	path := writeInput(t, threeQuestions)
	var stdout, stderr bytes.Buffer
	if err := Load(context.Background(), LoadOptions{Input: path, Server: srv.URL, Concurrency: 2, Slots: 3, Workers: 30, JSON: false}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"slots: 3\n", "workers: 30\n", "questions: 3\n", "answers: 6\n", "answers_missing: 0\n", "questions_per_sec: "} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("missing %q in\n%s", want, stdout.String())
		}
	}
}

func TestLoadCountsFailure(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer srv.Close()
	path := writeInput(t, `{"state":"hello","questions":{"q":{"type":"choice","instructions":"Which?","criteria":{"a":"A","b":"B"}}}}`)
	var stdout, stderr bytes.Buffer
	err := Load(context.Background(), LoadOptions{Input: path, Server: srv.URL, Concurrency: 4, Slots: 1, JSON: true}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	var report LoadReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Errors != 1 || report.Success != 3 {
		t.Fatalf("%+v", report)
	}
}
