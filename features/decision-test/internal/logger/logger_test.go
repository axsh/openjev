package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestInfoIncludesComponentAndModel(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf).WithComponent("decision")
	log.Info("ready", "model_id", "minicpm5-2b-q4_k_m")
	got := buf.String()
	if !strings.Contains(got, "component=decision") {
		t.Fatalf("component missing: %s", got)
	}
	if !strings.Contains(got, "model_id=minicpm5-2b-q4_k_m") {
		t.Fatalf("model_id missing: %s", got)
	}
}

func TestDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	New(&buf).Debug("direct completed", "question_id", "queue")
	if !strings.Contains(buf.String(), "level=DEBUG") {
		t.Fatalf("debug level missing: %s", buf.String())
	}
}

func TestErrorField(t *testing.T) {
	var buf bytes.Buffer
	New(&buf).Error("llama request failed", "error", "status 500")
	if !strings.Contains(buf.String(), "error=") {
		t.Fatalf("error field missing: %s", buf.String())
	}
}
