// Command draforge-e2e-check validates remote go test JSON output.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/oaslananka/draforge/internal/e2echeck"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("draforge-e2e-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	filePath := fs.String("file", "", "path to go test -json output")
	requiredTest := fs.String("required-test", "TestSmoke", "test that must execute and pass")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *filePath == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR: --file is required")
		return 2
	}

	file, err := os.Open(*filePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: open test results: %v\n", err)
		return 2
	}
	summary, checkErr := e2echeck.Check(file, *requiredTest)
	if closeErr := file.Close(); closeErr != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: close test results: %v\n", closeErr)
		return 2
	}
	if checkErr != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: remote E2E result verification failed: %v (executed=%d passed=%d failed=%d skipped=%d)\n",
			checkErr, summary.Executed, summary.Passed, summary.Failed, summary.Skipped)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Remote E2E results verified: executed=%d passed=%d failed=%d skipped=%d required=%s\n",
		summary.Executed, summary.Passed, summary.Failed, summary.Skipped, *requiredTest)
	return 0
}
