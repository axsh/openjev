package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"openjev/features/decision-test/internal/domain"
)

type LoadOptions struct {
	Input       string
	Server      string
	Method      string
	Concurrency int
	Slots       int
	// Workers is a report label only; it does not change the server.
	Workers int
	// Questions keeps only the first N questions of the input; 0 sends the input unchanged.
	Questions int
	JSON      bool
}

type LatencyMs struct {
	Min float64 `json:"min"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

type LoadReport struct {
	Slots           int            `json:"slots"`
	Workers         int            `json:"workers"`
	Concurrency     int            `json:"concurrency"`
	Requests        int            `json:"requests"`
	Questions       int            `json:"questions"`
	Success         int            `json:"success"`
	Errors          int            `json:"errors"`
	Answers         int            `json:"answers"`
	AnswersMissing  int            `json:"answers_missing"`
	Statuses        map[string]int `json:"statuses"`
	WallMs          float64        `json:"wall_ms"`
	LatencyMs       LatencyMs      `json:"latency_ms"`
	ThroughputRps   float64        `json:"throughput_rps"`
	QuestionsPerSec float64        `json:"questions_per_sec"`
	DirectMsSum     *float64       `json:"direct_ms_sum,omitempty"`
}

type loadResult struct {
	status    int
	latency   float64
	answers   int
	direct    float64
	hasDirect bool
	err       error
}

func Load(ctx context.Context, opt LoadOptions, stdout, stderr io.Writer) error {
	raw, err := os.ReadFile(opt.Input)
	if err != nil {
		return err
	}
	body, err := domain.TakeQuestions(raw, opt.Questions)
	if err != nil {
		return err
	}
	body, err = domain.ApplyMethod(body, opt.Method)
	if err != nil {
		return err
	}
	questions, err := domain.QuestionCount(body)
	if err != nil {
		return err
	}
	transport := &http.Transport{MaxConnsPerHost: 0, MaxIdleConnsPerHost: opt.Concurrency}
	client := &http.Client{Timeout: 10 * time.Minute, Transport: transport}
	results := make([]loadResult, opt.Concurrency)
	startGate := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < opt.Concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-startGate
			results[i] = oneLoad(ctx, client, opt.Server, body)
		}(i)
	}
	wallStart := time.Now()
	close(startGate)
	wg.Wait()
	report := summarize(opt, questions, results, float64(time.Since(wallStart).Microseconds())/1000)
	if err := writeLoad(stdout, opt.JSON, report); err != nil {
		return err
	}
	if report.Errors > 0 {
		fmt.Fprintf(stderr, "errors %d\n", report.Errors)
	}
	if report.AnswersMissing > 0 {
		fmt.Fprintf(stderr, "answers_missing %d\n", report.AnswersMissing)
	}
	if report.Errors > 0 || report.AnswersMissing > 0 {
		return fmt.Errorf("errors %d answers_missing %d", report.Errors, report.AnswersMissing)
	}
	return nil
}

func oneLoad(ctx context.Context, client *http.Client, server string, body []byte) loadResult {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, trimRightSlash(server)+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return loadResult{latency: float64(time.Since(start).Microseconds()) / 1000, err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return loadResult{latency: float64(time.Since(start).Microseconds()) / 1000, err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	result := loadResult{status: resp.StatusCode, latency: float64(time.Since(start).Microseconds()) / 1000}
	if err != nil {
		result.err = err
		return result
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.err = fmt.Errorf("status %d", resp.StatusCode)
		return result
	}
	result.answers, result.direct, result.hasDirect = inspectAnswers(raw)
	return result
}

// inspectAnswers counts the answers in a response body and sums their direct_ms.
func inspectAnswers(raw []byte) (count int, direct float64, hasDirect bool) {
	var body struct {
		Answers map[string]struct {
			Timings *struct {
				DirectMs float64 `json:"direct_ms"`
			} `json:"timings"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return 0, 0, false
	}
	for _, ans := range body.Answers {
		count++
		if ans.Timings == nil {
			continue
		}
		direct += ans.Timings.DirectMs
		hasDirect = true
	}
	return count, direct, hasDirect
}

func summarize(opt LoadOptions, questions int, results []loadResult, wall float64) LoadReport {
	report := LoadReport{
		Slots:       opt.Slots,
		Workers:     opt.Workers,
		Concurrency: opt.Concurrency,
		Requests:    opt.Concurrency,
		Questions:   questions,
		Statuses:    map[string]int{},
		WallMs:      wall,
	}
	latencies := make([]float64, 0, len(results))
	direct := 0.0
	sawDirect := false
	for _, result := range results {
		latencies = append(latencies, result.latency)
		key := "0"
		if result.status != 0 {
			key = fmt.Sprintf("%d", result.status)
		}
		report.Statuses[key]++
		if result.err != nil || result.status < 200 || result.status >= 300 {
			report.Errors++
			continue
		}
		report.Success++
		report.Answers += result.answers
		if result.hasDirect {
			direct += result.direct
			sawDirect = true
		}
	}
	report.AnswersMissing = report.Success*report.Questions - report.Answers
	sort.Float64s(latencies)
	if len(latencies) > 0 {
		report.LatencyMs = LatencyMs{
			Min: latencies[0],
			P50: percentile(latencies, 50),
			P95: percentile(latencies, 95),
			Max: latencies[len(latencies)-1],
		}
	}
	if wall == 0 {
		report.ThroughputRps = 0
		report.QuestionsPerSec = 0
	} else {
		report.ThroughputRps = float64(report.Success) / (wall / 1000)
		report.QuestionsPerSec = float64(report.Answers) / (wall / 1000)
	}
	if sawDirect {
		report.DirectMsSum = &direct
	}
	return report
}

func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	index := int(math.Ceil(p/100*float64(n))) - 1
	if index < 0 {
		index = 0
	}
	if index >= n {
		index = n - 1
	}
	return sorted[index]
}

func writeLoad(stdout io.Writer, asJSON bool, report LoadReport) error {
	if asJSON {
		raw, err := json.Marshal(report)
		if err != nil {
			return err
		}
		_, err = stdout.Write(append(raw, '\n'))
		return err
	}
	fmt.Fprintf(stdout, "slots: %d\n", report.Slots)
	fmt.Fprintf(stdout, "workers: %d\n", report.Workers)
	fmt.Fprintf(stdout, "concurrency: %d\n", report.Concurrency)
	fmt.Fprintf(stdout, "requests: %d\n", report.Requests)
	fmt.Fprintf(stdout, "questions: %d\n", report.Questions)
	fmt.Fprintf(stdout, "success: %d\n", report.Success)
	fmt.Fprintf(stdout, "errors: %d\n", report.Errors)
	fmt.Fprintf(stdout, "answers: %d\n", report.Answers)
	fmt.Fprintf(stdout, "answers_missing: %d\n", report.AnswersMissing)
	keys := make([]string, 0, len(report.Statuses))
	for key := range report.Statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(stdout, "status_%s: %d\n", key, report.Statuses[key])
	}
	fmt.Fprintf(stdout, "wall_ms: %.3f\n", report.WallMs)
	fmt.Fprintf(stdout, "latency_min_ms: %.3f\n", report.LatencyMs.Min)
	fmt.Fprintf(stdout, "latency_p50_ms: %.3f\n", report.LatencyMs.P50)
	fmt.Fprintf(stdout, "latency_p95_ms: %.3f\n", report.LatencyMs.P95)
	fmt.Fprintf(stdout, "latency_max_ms: %.3f\n", report.LatencyMs.Max)
	fmt.Fprintf(stdout, "throughput_rps: %.3f\n", report.ThroughputRps)
	fmt.Fprintf(stdout, "questions_per_sec: %.3f\n", report.QuestionsPerSec)
	if report.DirectMsSum != nil {
		fmt.Fprintf(stdout, "direct_ms_sum: %.3f\n", *report.DirectMsSum)
	}
	return nil
}

func trimRightSlash(server string) string {
	for len(server) > 0 && server[len(server)-1] == '/' {
		server = server[:len(server)-1]
	}
	return server
}
