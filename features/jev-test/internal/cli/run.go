package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"openjev/features/jev-test/internal/check"
	"openjev/features/jev-test/internal/logger"
	"openjev/features/jev-test/internal/probe"
)

const (
	ExitOK    = 0
	ExitAPI   = 1
	ExitUsage = 2
)

func Run(args []string, stdout, stderr io.Writer, log *logger.Logger) int {
	fs := flag.NewFlagSet("jev-test", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "features/decision-test/testdata/bank.json", "Request JSON file")
	keyFile := fs.String("key-file", "tmp/typesafe-api-key.txt", "API key file")
	url := fs.String("url", "https://api.typesafe.ai/v1/systemone", "Jev endpoint")
	model := fs.String("model", "jev-latest", "Model used when the input omits model")
	timeout := fs.Duration("timeout", 10*time.Minute, "HTTP timeout")
	asJSON := fs.Bool("json", false, "Write one JSON object to stdout")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	token, err := probe.ReadKey(*keyFile)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitUsage
	}
	loaded, err := probe.Load(*input, *model)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitUsage
	}
	if log != nil {
		log.Debug("probe starting", "url", *url, "model", *model, "input", *input, "questions", len(loaded.Questions), "key_file", *keyFile)
	}
	res, err := probe.Post(context.Background(), *url, token, loaded.Body, *timeout, log)
	if err != nil {
		if log != nil {
			log.Error("post failed", "error", err.Error(), "duration_ms", res.WallMs, "url", *url)
		}
		fmt.Fprintln(stderr, err.Error())
		return ExitAPI
	}
	if res.Status < 200 || res.Status >= 300 {
		snippet := res.Body
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		if log != nil {
			log.Error("post rejected", "status", res.Status, "duration_ms", res.WallMs, "body", string(snippet))
		}
		fmt.Fprintf(stderr, "status: %d\n", res.Status)
		fmt.Fprintf(stderr, "wall_ms: %s\n", formatFloat(res.WallMs))
		stderr.Write(res.Body)
		if len(res.Body) == 0 || res.Body[len(res.Body)-1] != '\n' {
			fmt.Fprintln(stderr)
		}
		if *asJSON {
			writeJSON(stdout, jsonOut(res.Status, res.WallMs, len(loaded.Questions), check.Report{ContentOK: false}))
		}
		return ExitAPI
	}
	report := check.Check(loaded.Questions, res.Body)
	if log != nil {
		log.Info("probe finished", "status", res.Status, "duration_ms", res.WallMs, "content_ok", report.ContentOK, "questions", len(loaded.Questions))
	}
	if *asJSON {
		out := jsonOut(res.Status, res.WallMs, len(loaded.Questions), report)
		writeJSON(stdout, out)
	} else {
		writeHuman(stdout, res.Status, res.WallMs, len(loaded.Questions), report)
	}
	if !report.ContentOK {
		for _, reason := range report.Reasons {
			fmt.Fprintln(stderr, reason)
		}
		return ExitAPI
	}
	return ExitOK
}

func writeHuman(w io.Writer, status int, wall float64, questions int, report check.Report) {
	fmt.Fprintf(w, "status: %d\n", status)
	fmt.Fprintf(w, "wall_ms: %s\n", formatFloat(wall))
	if report.ServerElapsedMs != nil {
		fmt.Fprintf(w, "server_elapsed_ms: %s\n", formatFloat(*report.ServerElapsedMs))
	}
	fmt.Fprintf(w, "model: %s\n", report.Model)
	fmt.Fprintf(w, "questions: %d\n", questions)
	fmt.Fprintf(w, "content_ok: %t\n", report.ContentOK)
	if report.Usage != nil {
		fmt.Fprintf(w, "usage_input_tokens: %d\n", report.Usage.InputTokens)
		fmt.Fprintf(w, "usage_output_tokens: %d\n", report.Usage.OutputTokens)
	}
	for _, answer := range report.Answers {
		switch answer.Type {
		case "choice":
			fmt.Fprintf(w, "%s type=choice choice=%s confidence=%s\n", answer.ID, answer.Choice, formatFloat(answer.Confidence))
		case "score":
			fmt.Fprintf(w, "%s type=score score=%s confidence=%s\n", answer.ID, formatFloat(answer.Score), formatFloat(answer.Confidence))
		case "noul":
			fmt.Fprintf(w, "%s type=noul noul=%s\n", answer.ID, formatFloat(answer.Noul))
		}
	}
}

type jsonReport struct {
	Status          int                   `json:"status"`
	WallMs          float64               `json:"wall_ms"`
	ServerElapsedMs *float64              `json:"server_elapsed_ms,omitempty"`
	Model           string                `json:"model,omitempty"`
	Questions       int                   `json:"questions"`
	ContentOK       bool                  `json:"content_ok"`
	Usage           *check.Usage          `json:"usage,omitempty"`
	Answers         map[string]answerJSON `json:"answers,omitempty"`
}

type answerJSON struct {
	Type       string   `json:"type"`
	Choice     string   `json:"choice,omitempty"`
	Score      *float64 `json:"score,omitempty"`
	Noul       *float64 `json:"noul,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
}

func jsonOut(status int, wall float64, questions int, report check.Report) jsonReport {
	out := jsonReport{
		Status:          status,
		WallMs:          wall,
		ServerElapsedMs: report.ServerElapsedMs,
		Model:           report.Model,
		Questions:       questions,
		ContentOK:       report.ContentOK,
		Usage:           report.Usage,
	}
	if len(report.Answers) == 0 || !report.ContentOK && report.Model == "" {
		return out
	}
	out.Answers = map[string]answerJSON{}
	for _, answer := range report.Answers {
		item := answerJSON{Type: answer.Type}
		switch answer.Type {
		case "choice":
			item.Choice = answer.Choice
			if answer.HasConf {
				v := answer.Confidence
				item.Confidence = &v
			}
		case "score":
			if answer.HasScore {
				v := answer.Score
				item.Score = &v
			}
			if answer.HasConf {
				v := answer.Confidence
				item.Confidence = &v
			}
		case "noul":
			if answer.HasNoul {
				v := answer.Noul
				item.Noul = &v
			}
		}
		out.Answers[answer.ID] = item
	}
	return out
}

func writeJSON(w io.Writer, out jsonReport) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(out)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
