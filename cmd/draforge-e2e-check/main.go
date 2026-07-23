// Command draforge-e2e-check validates remote go test JSON output.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/oaslananka/draforge/internal/e2echeck"
)

func main() {
	filePath := flag.String("file", "", "path to go test -json output")
	requiredTest := flag.String("required-test", "TestSmoke", "test that must execute and pass")
	flag.Parse()

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --file is required")
		os.Exit(2)
	}

	file, err := os.Open(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: open test results: %v\n", err)
		os.Exit(2)
	}
	summary, checkErr := e2echeck.Check(file, *requiredTest)
	if closeErr := file.Close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "ERROR: close test results: %v\n", closeErr)
		os.Exit(2)
	}
	if checkErr != nil {
		fmt.Fprintf(os.Stderr, "ERROR: remote E2E result verification failed: %v (executed=%d passed=%d failed=%d skipped=%d)\n",
			checkErr, summary.Executed, summary.Passed, summary.Failed, summary.Skipped)
		os.Exit(1)
	}
	fmt.Printf("Remote E2E results verified: executed=%d passed=%d failed=%d skipped=%d required=%s\n",
		summary.Executed, summary.Passed, summary.Failed, summary.Skipped, *requiredTest)
}
