package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sympozium-ai/sympozium/internal/ipc"
)

// A result written after the first poll is decoded, and both files are unlinked.
func TestRoundTripSuccess(t *testing.T) {
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "request-1.json")
	resPath := filepath.Join(dir, "result-1.json")

	go func() {
		time.Sleep(500 * time.Millisecond)
		// A partial write first, then the complete result.
		_ = os.WriteFile(resPath, []byte(`{"status":"suc`), 0o644)
		time.Sleep(2 * time.Second)
		_ = os.WriteFile(resPath, []byte(`{"requestId":"1","status":"success","response":"hi"}`), 0o644)
	}()

	var out ipc.DelegateResult
	err := ipcRoundTrip(context.Background(), reqPath, resPath, 200*time.Millisecond, 10*time.Second,
		ipc.SpawnRequest{RequestID: "1", Task: "t", TargetPersona: "writer", PackName: "p"}, &out)
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	if out.Status != "success" || out.Response != "hi" {
		t.Fatalf("bad decode: %+v", out)
	}
	if _, err := os.Stat(reqPath); !os.IsNotExist(err) {
		t.Error("request file not cleaned up")
	}
	if _, err := os.Stat(resPath); !os.IsNotExist(err) {
		t.Error("result file not cleaned up")
	}
}

// The request payload lands on disk in the wire format the bridge routes on.
func TestRoundTripWritesRequest(t *testing.T) {
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "request-9.json")
	resPath := filepath.Join(dir, "result-9.json")

	var out ipc.DelegateResult
	_ = ipcRoundTrip(context.Background(), reqPath, resPath, 50*time.Millisecond, 300*time.Millisecond,
		ipc.SpawnRequest{RequestID: "9", Task: "write a poem", AgentID: "default", TargetPersona: "writer", PackName: "pack"}, &out)

	data, err := os.ReadFile(reqPath)
	if err != nil {
		t.Fatalf("request should survive a failed round-trip: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	for _, k := range []string{"requestId", "task", "agentId", "targetPersona", "packName"} {
		if _, ok := got[k]; !ok {
			t.Errorf("wire format missing %q; got %v", k, got)
		}
	}
}

// The timeout error names the budget that elapsed.
func TestRoundTripTimeout(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	var out ipc.DelegateResult
	err := ipcRoundTrip(context.Background(), filepath.Join(dir, "req.json"), filepath.Join(dir, "res.json"),
		100*time.Millisecond, 600*time.Millisecond, ipc.SpawnRequest{}, &out)
	if err == nil || err.Error() != "timed out after 600ms" {
		t.Fatalf("want timeout error, got %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("overshot the deadline: %s", d)
	}
}

// A cancelled run context aborts the poll without waiting for the next tick.
func TestRoundTripCtxCancel(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()

	start := time.Now()
	var out ipc.DelegateResult
	err := ipcRoundTrip(ctx, filepath.Join(dir, "req.json"), filepath.Join(dir, "res.json"),
		2*time.Second, 10*time.Minute, ipc.SpawnRequest{}, &out)
	if err != context.Canceled {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("did not abort promptly: %s", d)
	}
}

// The wait inherits the run deadline.
func TestEffectiveDelegateTimeout(t *testing.T) {
	delegateTimeout = 0

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	if d := effectiveDelegateTimeout(ctx); d < 59*time.Minute {
		t.Errorf("should inherit the 60m run budget, got %s", d)
	}

	if d := effectiveDelegateTimeout(context.Background()); d != 10*time.Minute {
		t.Errorf("no-deadline ctx should fall back to 10m, got %s", d)
	}

	delegateTimeout = 15 * time.Minute
	defer func() { delegateTimeout = 0 }()
	if d := effectiveDelegateTimeout(ctx); d != 15*time.Minute {
		t.Errorf("DELEGATE_TIMEOUT should cap below the run budget, got %s", d)
	}
}
