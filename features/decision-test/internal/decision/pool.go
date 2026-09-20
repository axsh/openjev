package decision

import (
	"context"
	"sync"

	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/logger"
)

// requestQueueCapacity bounds the singleton request queue. A full queue blocks
// the submitting handler, which that handler's own stall timer covers.
const requestQueueCapacity = 1024

// task is one question of one request, routed through the shared queue.
type task struct {
	ctx      context.Context
	state    string
	question domain.Question
	method   domain.Method
	results  chan<- result
}

// result is the outcome of one task, delivered to the submitting handler.
type result struct {
	id     string
	answer domain.Answer
	used   usage
	err    error
}

// executor answers one question. Service.one satisfies it.
type executor func(ctx context.Context, state string, question domain.Question, method domain.Method) (domain.Answer, usage, error)

// Pool is the process-wide worker pool. Handlers submit one task per question
// and collect results from their own response channel; workers are the only
// bound on concurrent engine calls.
type Pool struct {
	queue   chan task
	exec    executor
	log     *logger.Logger
	workers int
	wg      sync.WaitGroup
}

func newPool(workers int, exec executor, log *logger.Logger) *Pool {
	if workers < 1 {
		workers = 1
	}
	return &Pool{
		queue:   make(chan task, requestQueueCapacity),
		exec:    exec,
		log:     log,
		workers: workers,
	}
}

// Start launches the worker goroutines. They exit when ctx is done.
func (p *Pool) Start(ctx context.Context) {
	p.log.Debug("worker pool started", "workers", p.workers, "queue_capacity", requestQueueCapacity)
	for i := range p.workers {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
}

// Wait blocks until every worker has exited after Start's context ended.
func (p *Pool) Wait() {
	p.wg.Wait()
}

// Depth reports how many tasks are waiting in the request queue.
func (p *Pool) Depth() int {
	return len(p.queue)
}

// Workers reports the configured worker count.
func (p *Pool) Workers() int {
	return p.workers
}

func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-p.queue:
			if t.ctx.Err() != nil {
				// The handler already returned; nobody is waiting for this result.
				p.log.Debug("task skipped", "question_id", t.question.ID, "worker_id", id)
				continue
			}
			answer, used, err := p.exec(t.ctx, t.state, t.question, t.method)
			if err != nil && t.ctx.Err() != nil {
				p.log.Debug("task cancelled", "question_id", t.question.ID, "worker_id", id)
			}
			// The response channel is sized to the request's question count, so
			// this send never blocks even after the handler has returned.
			t.results <- result{id: t.question.ID, answer: answer, used: used, err: err}
		}
	}
}
