package decision

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/logger"
)

func poolQuestion(id string) domain.Question {
	return domain.Question{ID: id, Type: "choice", Instructions: domain.TextOf("Which?")}
}

func TestPoolMaxConcurrency(t *testing.T) {
	var inFlight, maxSeen atomic.Int32
	release := make(chan struct{})
	exec := func(ctx context.Context, state string, q domain.Question, m domain.Method) (domain.Answer, usage, error) {
		n := inFlight.Add(1)
		for {
			old := maxSeen.Load()
			if n <= old || maxSeen.CompareAndSwap(old, n) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		return domain.Answer{Type: "choice", Choice: q.ID}, usage{in: 1, out: 1}, nil
	}
	p := newPool(4, exec, logger.New(io.Discard))
	p.Start(t.Context())
	if p.Workers() != 4 {
		t.Fatalf("workers %d", p.Workers())
	}
	results := make(chan result, 10)
	for i := range 10 {
		p.queue <- task{ctx: context.Background(), state: "s", question: poolQuestion(fmt.Sprintf("q%02d", i)), method: domain.MethodDirect, results: results}
	}
	deadline := time.Now().Add(2 * time.Second)
	for inFlight.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if maxSeen.Load() != 4 || p.Depth() != 6 {
		t.Fatalf("max %d depth %d", maxSeen.Load(), p.Depth())
	}
	close(release)
	got := map[string]bool{}
	for range 10 {
		select {
		case r := <-results:
			if r.err != nil {
				t.Fatal(r.err)
			}
			got[r.id] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout after %d results", len(got))
		}
	}
	if len(got) != 10 || maxSeen.Load() != 4 {
		t.Fatalf("results %d max %d", len(got), maxSeen.Load())
	}
}

func TestPoolSkipsCancelledTask(t *testing.T) {
	var calls atomic.Int32
	exec := func(ctx context.Context, state string, q domain.Question, m domain.Method) (domain.Answer, usage, error) {
		calls.Add(1)
		return domain.Answer{Type: "choice", Choice: q.ID}, usage{}, nil
	}
	logs := &syncBuffer{}
	p := newPool(1, exec, logger.New(logs))
	p.Start(t.Context())
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	results := make(chan result, 2)
	p.queue <- task{ctx: dead, state: "s", question: poolQuestion("dead"), method: domain.MethodDirect, results: results}
	p.queue <- task{ctx: context.Background(), state: "s", question: poolQuestion("live"), method: domain.MethodDirect, results: results}
	select {
	case r := <-results:
		if r.id != "live" {
			t.Fatalf("result %s", r.id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	if calls.Load() != 1 || len(results) != 0 {
		t.Fatalf("calls %d pending %d", calls.Load(), len(results))
	}
	if !strings.Contains(logs.String(), "task skipped") || !strings.Contains(logs.String(), "question_id=dead") {
		t.Fatalf("log:\n%s", logs.String())
	}
}

func TestPoolStopsOnContext(t *testing.T) {
	var calls atomic.Int32
	exec := func(ctx context.Context, state string, q domain.Question, m domain.Method) (domain.Answer, usage, error) {
		calls.Add(1)
		return domain.Answer{}, usage{}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := newPool(2, exec, logger.New(io.Discard))
	p.Start(ctx)
	cancel()
	p.Wait()
	results := make(chan result, 1)
	p.queue <- task{ctx: context.Background(), state: "s", question: poolQuestion("late"), method: domain.MethodDirect, results: results}
	time.Sleep(100 * time.Millisecond)
	if calls.Load() != 0 || p.Depth() != 1 {
		t.Fatalf("calls %d depth %d", calls.Load(), p.Depth())
	}
}
