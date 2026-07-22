// Package server SSE integration tests.
// SPDX-License-Identifier: Apache-2.0
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (r *flushRecorder) Flush() {
	r.flushed = true
}

func TestRequestLoggingPreservesResponseControllerFlush(t *testing.T) {
	srv := NewServer(nil, 0)
	writer := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	var flushErr error

	handler := srv.requestLogging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flushErr = http.NewResponseController(w).Flush()
	}))
	handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/api/stream", nil))

	if flushErr != nil {
		t.Fatalf("expected wrapped ResponseWriter to support Flush, got %v", flushErr)
	}
	if !writer.flushed {
		t.Fatal("expected Flush to reach the underlying ResponseWriter")
	}
}

func TestSSEStreamThroughMiddlewareSurvivesWriteTimeout(t *testing.T) {
	srv := NewServer(fake.NewSimpleClientset(), 0)
	testServer := httptest.NewUnstartedServer(srv.handler())
	writeTimeout := 50 * time.Millisecond
	testServer.Config.WriteTimeout = writeTimeout
	testServer.Start()
	defer testServer.Close()

	requestLog := make(chan string, 1)
	srv.logf = func(format string, args ...any) {
		requestLog <- fmt.Sprintf(format, args...)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testServer.URL+"/api/stream", nil)
	if err != nil {
		t.Fatalf("create SSE request: %v", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	startedAt := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("connect to SSE endpoint: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	initialData := readSSEData(t, reader)
	assertGraphEvent(t, "initial", initialData)

	broadcastCtx, stopBroadcaster := context.WithCancel(context.Background())
	defer stopBroadcaster()
	go srv.runSSEBroadcaster(broadcastCtx, 3*writeTimeout)

	laterData := readSSEData(t, reader)
	assertGraphEvent(t, "later", laterData)
	if elapsed := time.Since(startedAt); elapsed <= writeTimeout {
		t.Fatalf("expected connection to outlive write timeout %s, lasted %s", writeTimeout, elapsed)
	}

	cancel()
	_ = resp.Body.Close()
	waitForSSEClients(t, srv, 0)

	select {
	case logText := <-requestLog:
		if !strings.Contains(logText, "[GET] /api/stream") || !strings.Contains(logText, " - 200 (") {
			t.Fatalf("expected status and duration in request log, got %q", logText)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for request log after SSE disconnect")
	}
}

func TestBroadcastSkipsSlowClient(t *testing.T) {
	srv := NewServer(nil, 0)
	slow := make(chan string, 1)
	fast := make(chan string, 1)
	slow <- "already-buffered"

	srv.clients[slow] = true
	srv.clients[fast] = true
	srv.broadcast("fresh")

	if got := <-slow; got != "already-buffered" {
		t.Fatalf("expected slow client buffer to remain unchanged, got %q", got)
	}
	select {
	case got := <-fast:
		if got != "fresh" {
			t.Fatalf("expected fresh update for healthy client, got %q", got)
		}
	default:
		t.Fatal("healthy client did not receive broadcast")
	}
}

func assertGraphEvent(t *testing.T, name, data string) {
	t.Helper()
	var graph map[string]any
	if err := json.Unmarshal([]byte(data), &graph); err != nil {
		t.Fatalf("%s SSE event is not valid JSON: %v", name, err)
	}
	if _, ok := graph["nodes"]; !ok {
		t.Fatalf("%s SSE graph is missing nodes: %s", name, data)
	}
	if _, ok := graph["edges"]; !ok {
		t.Fatalf("%s SSE graph is missing edges: %s", name, data)
	}
}

func readSSEData(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var data string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE event: %v", err)
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		switch {
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && data != "":
			return data
		}
	}
}

func waitForSSEClients(t *testing.T, srv *Server, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		got := len(srv.clients)
		srv.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	srv.mu.Lock()
	got := len(srv.clients)
	srv.mu.Unlock()
	t.Fatalf("expected %d SSE clients, got %d", want, got)
}
