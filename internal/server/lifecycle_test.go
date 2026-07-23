// Package server tests HTTP lifecycle and shutdown behavior.
// SPDX-License-Identifier: Apache-2.0
package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

type fakeHTTPServer struct {
	listenStarted chan struct{}
	serveDone     chan struct{}
	shutdown      func(context.Context) error

	mu            sync.Mutex
	shutdownCalls int
	closeCalls    int
}

func newFakeHTTPServer() *fakeHTTPServer {
	return &fakeHTTPServer{
		listenStarted: make(chan struct{}),
		serveDone:     make(chan struct{}),
	}
}

func (s *fakeHTTPServer) ListenAndServe() error {
	close(s.listenStarted)
	<-s.serveDone
	return http.ErrServerClosed
}

func (s *fakeHTTPServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shutdownCalls++
	s.mu.Unlock()
	if s.shutdown != nil {
		return s.shutdown(ctx)
	}
	select {
	case <-s.serveDone:
	default:
		close(s.serveDone)
	}
	return nil
}

func (s *fakeHTTPServer) Close() error {
	s.mu.Lock()
	s.closeCalls++
	s.mu.Unlock()
	select {
	case <-s.serveDone:
	default:
		close(s.serveDone)
	}
	return nil
}

func TestRunHTTPServerCancelsRequestsBeforeGracefulShutdown(t *testing.T) {
	server := newFakeHTTPServer()
	ctx, cancel := context.WithCancel(context.Background())
	requestsCanceled := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(ctx, server, time.Second, func() {
			close(requestsCanceled)
		})
	}()

	<-server.listenStarted
	cancel()
	select {
	case <-requestsCanceled:
	case <-time.After(time.Second):
		t.Fatal("request contexts were not canceled before shutdown")
	}
	if err := <-done; err != nil {
		t.Fatalf("graceful shutdown failed: %v", err)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if server.shutdownCalls != 1 {
		t.Fatalf("expected one Shutdown call, got %d", server.shutdownCalls)
	}
	if server.closeCalls != 0 {
		t.Fatalf("unexpected force Close calls: %d", server.closeCalls)
	}
}

func TestRunHTTPServerForcesCloseAfterShutdownTimeout(t *testing.T) {
	server := newFakeHTTPServer()
	server.shutdown = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(ctx, server, 20*time.Millisecond, func() {})
	}()

	<-server.listenStarted
	cancel()
	err := <-done
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown deadline error, got %v", err)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if server.shutdownCalls != 1 {
		t.Fatalf("expected one Shutdown call, got %d", server.shutdownCalls)
	}
	if server.closeCalls != 1 {
		t.Fatalf("expected one force Close call, got %d", server.closeCalls)
	}
}
