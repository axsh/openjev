package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"openjev/features/decision-test/internal/domain"
)

type Options struct {
	Input  string
	Server string
	Method string
	JSON   bool
}

func Decide(ctx context.Context, opt Options, stdout, stderr io.Writer) error {
	raw, err := os.ReadFile(opt.Input)
	if err != nil {
		return err
	}
	body, err := domain.ApplyMethod(raw, opt.Method)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(opt.Server, "/")+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(stderr, "status %d\n", resp.StatusCode)
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if opt.JSON {
		_, err = stdout.Write(respBody)
		return err
	}
	return writeHuman(stdout, respBody)
}

func writeHuman(stdout io.Writer, raw []byte) error {
	var body struct {
		Answers map[string]struct {
			Type          string             `json:"type"`
			Choice        string             `json:"choice"`
			Score         *float64           `json:"score"`
			Noul          *float64           `json:"noul"`
			Confidence    *float64           `json:"confidence"`
			Probabilities map[string]float64 `json:"probabilities"`
			Timings       *struct {
				DirectMs     float64  `json:"direct_ms"`
				GenerationMs float64  `json:"generation_ms"`
				Ratio        *float64 `json:"ratio"`
			} `json:"timings"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	ids := make([]string, 0, len(body.Answers))
	for id := range body.Answers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ans := body.Answers[id]
		fmt.Fprintf(stdout, "%s\n", id)
		switch ans.Type {
		case "score":
			if ans.Score != nil {
				fmt.Fprintf(stdout, "  score: %.3f\n", *ans.Score)
			}
			if ans.Confidence != nil {
				fmt.Fprintf(stdout, "  confidence: %.3f\n", *ans.Confidence)
			}
		case "noul":
			if ans.Noul != nil {
				fmt.Fprintf(stdout, "  noul: %.3f\n", *ans.Noul)
			}
		default:
			fmt.Fprintf(stdout, "  choice: %s\n", ans.Choice)
			if ans.Confidence != nil {
				fmt.Fprintf(stdout, "  confidence: %.3f\n", *ans.Confidence)
			}
		}
		if ans.Type != "noul" {
			keys := make([]string, 0, len(ans.Probabilities))
			for key := range ans.Probabilities {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(stdout, "  %s  %.3f\n", key, ans.Probabilities[key])
			}
		}
		if ans.Timings == nil {
			continue
		}
		if ans.Timings.DirectMs != 0 {
			fmt.Fprintf(stdout, "  direct_ms: %.3f\n", ans.Timings.DirectMs)
		}
		if ans.Timings.GenerationMs != 0 {
			fmt.Fprintf(stdout, "  generation_ms: %.3f\n", ans.Timings.GenerationMs)
		}
		if ans.Timings.Ratio != nil {
			fmt.Fprintf(stdout, "  ratio: %.3f\n", *ans.Timings.Ratio)
		}
	}
	return nil
}
