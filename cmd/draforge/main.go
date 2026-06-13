// cmd/draforge/main.go
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/oaslananka/draforge/internal/cluster"
	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/internal/doctor"
	"github.com/oaslananka/draforge/internal/explain"
	"github.com/oaslananka/draforge/internal/graph"
	"github.com/oaslananka/draforge/internal/server"
	"github.com/oaslananka/draforge/internal/tui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	kubeconfig string
	namespace  string
	outputFmt  string
	versionVal = "v0.1.0"
	commitSHA  = "dev"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "draforge",
		Short: "DRAForge: Observe, simulate, and diagnose Kubernetes DRA.",
	}

	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to the kubeconfig file")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, json, yaml, dot, mermaid)")

	// 1. Version Command
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version and commit SHA",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("DRAForge %s (Commit: %s)\n", versionVal, commitSHA)
		},
	}

	// 2. Discover Command
	var discoverCmd = &cobra.Command{
		Use:   "discover",
		Short: "Discover and model active DRA resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientset, _, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}

			pools, devices, claims, err := discovery.DiscoverDRA(context.Background(), clientset)
			if err != nil {
				return err
			}

			switch outputFmt {
			case "json":
				data, _ := json.MarshalIndent(map[string]interface{}{
					"pools":   pools,
					"devices": devices,
					"claims":  claims,
				}, "", "  ")
				fmt.Println(string(data))
			case "yaml":
				data, _ := yaml.Marshal(map[string]interface{}{
					"pools":   pools,
					"devices": devices,
					"claims":  claims,
				})
				fmt.Println(string(data))
			default:
				fmt.Printf("Discovered %d Device Pools, %d Devices, %d ResourceClaims\n", len(pools), len(devices), len(claims))
			}
			return nil
		},
	}

	// 3. Claims Command
	var claimsCmd = &cobra.Command{
		Use:   "claims",
		Short: "List ResourceClaims",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientset, _, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			_, _, claims, err := discovery.DiscoverDRA(context.Background(), clientset)
			if err != nil {
				return err
			}

			if outputFmt == "json" {
				data, _ := json.MarshalIndent(claims, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("%-25s %-15s %-15s %-15s\n", "CLAIM NAME", "NAMESPACE", "CLASS", "STATUS")
				fmt.Println(string(make([]byte, 75)))
				for _, c := range claims {
					fmt.Printf("%-25s %-15s %-15s %-15s\n", c.Name, c.Namespace, c.DeviceClassName, c.Status)
				}
			}
			return nil
		},
	}

	// 4. Graph Command
	var graphCmd = &cobra.Command{
		Use:   "graph",
		Short: "Construct the resource relationship graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientset, _, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			gb := graph.NewGraphBuilder()
			g, err := gb.BuildGraph(context.Background(), clientset, namespace, "")
			if err != nil {
				return err
			}

			switch outputFmt {
			case "dot":
				fmt.Println(graph.ToDOT(g))
			case "mermaid":
				fmt.Println(graph.ToMermaid(g))
			default:
				data, _ := json.MarshalIndent(g, "", "  ")
				fmt.Println(string(data))
			}
			return nil
		},
	}

	// 5. Explain Command
	var explainCmd = &cobra.Command{
		Use:   "explain [claim-name]",
		Short: "Explain why a ResourceClaim is pending",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientset, _, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			res, err := explain.ExplainClaim(context.Background(), clientset, namespace, args[0])
			if err != nil {
				return err
			}

			if outputFmt == "json" {
				data, _ := json.MarshalIndent(res, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("Claim: %s (Allocated: %t)\n", res.TargetName, res.Allocated)
				fmt.Println("Reason Tree:")
				printReasonNode(res.ReasonTree, 0)
				if len(res.Remedy) > 0 {
					fmt.Println("\nSuggested Remedies:")
					for i, rem := range res.Remedy {
						fmt.Printf("  %d. %s\n", i+1, rem)
					}
				}
			}
			return nil
		},
	}

	// 6. Doctor Command
	var doctorCmd = &cobra.Command{
		Use:   "doctor",
		Short: "Run cluster and driver diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientset, _, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			registry := doctor.NewRegistry()
			report := registry.RunDiagnostics(context.Background(), clientset)

			if outputFmt == "json" {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("Diagnostics Report (%s)\n", report.Timestamp.Format(time.RFC1123))
				fmt.Printf("Summary: PASS=%d WARN=%d FAIL=%d\n\n", report.Summary["PASS"], report.Summary["WARN"], report.Summary["FAIL"])
				for _, res := range report.Results {
					fmt.Printf("[%s] %s: %s\n", res.Status, res.ID, res.Name)
					fmt.Printf("      Evidence: %s\n", res.Evidence)
					if res.Status != "PASS" {
						fmt.Printf("      Remediation: %s\n", res.Remediation)
					}
				}
			}
			return nil
		},
	}

	// 7. TUI Command
	var tuiCmd = &cobra.Command{
		Use:   "tui",
		Short: "Launch Bubble Tea Terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientset, _, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			return tui.RunTUI(clientset)
		},
	}

	// 8. Serve Command
	var serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Start HTTP API and React SPA Dashboard server",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientset, _, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			srv := server.NewServer(clientset, 8080)
			return srv.Start(context.Background())
		},
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(claimsCmd)
	rootCmd.AddCommand(graphCmd)
	rootCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(serveCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func printReasonNode(node model.ReasonNode, depth int) {
	indent := strings.Repeat("  ", depth)
	statusSym := "✔"
	if node.Confidence == "confirmed" {
		statusSym = "✖"
	}
	fmt.Printf("%s%s %s (%s)\n", indent, statusSym, node.Message, node.Confidence)
	for _, child := range node.Children {
		printReasonNode(child, depth+1)
	}
}
