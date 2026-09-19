package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
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
