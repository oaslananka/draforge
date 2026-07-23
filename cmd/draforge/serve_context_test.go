// Package main tests serve command lifecycle behavior.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"testing"
	"time"
)

func TestServeUsesCommandContext(t *testing.T) {
	original := runServeCommand
	t.Cleanup(func() {
		runServeCommand = original
	})

	started := make(chan context.Context, 1)
	runServeCommand = func(ctx context.Context, _ string, _ serveOptions) error {
		started <- ctx
		<-ctx.Done()
		return nil
	}

	root := NewRootCommand()
	root.SetArgs([]string{"serve"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- root.ExecuteContext(ctx)
	}()

	var serveCtx context.Context
	select {
	case serveCtx = <-started:
	case <-time.After(time.Second):
		t.Fatal("serve command did not start")
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve command returned an error during cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve command did not stop after context cancellation")
	}
	if serveCtx.Err() != context.Canceled {
		t.Fatalf("serve runtime did not receive the command context cancellation: %v", serveCtx.Err())
	}
}

func TestServeLifecycleFlagsRegistered(t *testing.T) {
	root := NewRootCommand()
	serveCmd, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("could not find serve subcommand: %v", err)
	}
	for name, expected := range map[string]string{
		"readiness-timeout":      "2s",
		"readiness-grace-period": "15s",
		"shutdown-timeout":       "5s",
	} {
		flag := serveCmd.Flag(name)
		if flag == nil {
			t.Fatalf("--%s flag not registered", name)
		}
		if flag.DefValue != expected {
			t.Fatalf("--%s default = %q, want %q", name, flag.DefValue, expected)
		}
	}
}
