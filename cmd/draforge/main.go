// cmd/draforge/main.go
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oaslananka/draforge/internal/cluster"
	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/internal/doctor"
	"github.com/oaslananka/draforge/internal/explain"
	"github.com/oaslananka/draforge/internal/graph"
	"github.com/oaslananka/draforge/internal/server"
	"github.com/oaslananka/draforge/internal/tui"
	"github.com/oaslananka/draforge/pkg/model"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// 9. Scenario Command
	var scenarioFile string
	var scenarioCmd = &cobra.Command{
		Use:   "scenario",
		Short: "Manage virtual device scenarios (pools)",
	}
	var scenarioApplyCmd = &cobra.Command{
		Use:   "apply",
		Short: "Apply a SimulatedDevicePool scenario from file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if scenarioFile == "" {
				return fmt.Errorf("scenario file path must be specified via -f")
			}
			_, dynamicClient, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			content, err := os.ReadFile(scenarioFile)
			if err != nil {
				return fmt.Errorf("failed to read scenario file: %w", err)
			}
			var obj unstructured.Unstructured
			if err := yaml.Unmarshal(content, &obj.Object); err != nil {
				return fmt.Errorf("failed to parse scenario YAML: %w", err)
			}
			ns := obj.GetNamespace()
			if ns == "" {
				ns = namespace
				obj.SetNamespace(ns)
			}
			gvr := schema.GroupVersionResource{
				Group:    "draforge.oaslananka",
				Version:  "v1alpha1",
				Resource: "simulateddevicepools",
			}
			existing, err := dynamicClient.Resource(gvr).Namespace(ns).Get(context.Background(), obj.GetName(), metav1.GetOptions{})
			if err == nil {
				obj.SetResourceVersion(existing.GetResourceVersion())
				_, err = dynamicClient.Resource(gvr).Namespace(ns).Update(context.Background(), &obj, metav1.UpdateOptions{})
				if err == nil {
					fmt.Printf("Successfully updated scenario %s/%s\n", ns, obj.GetName())
				}
				return err
			}
			_, err = dynamicClient.Resource(gvr).Namespace(ns).Create(context.Background(), &obj, metav1.CreateOptions{})
			if err == nil {
				fmt.Printf("Successfully applied scenario %s/%s\n", ns, obj.GetName())
			}
			return err
		},
	}
	scenarioApplyCmd.Flags().StringVarP(&scenarioFile, "file", "f", "", "Path to the scenario YAML file")
	var scenarioResetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Delete all SimulatedDevicePool scenarios",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, dynamicClient, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			gvr := schema.GroupVersionResource{
				Group:    "draforge.oaslananka",
				Version:  "v1alpha1",
				Resource: "simulateddevicepools",
			}
			list, err := dynamicClient.Resource(gvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
			if err != nil {
				return err
			}
			for _, item := range list.Items {
				fmt.Printf("Deleting SimulatedDevicePool scenario %s/%s...\n", namespace, item.GetName())
				_ = dynamicClient.Resource(gvr).Namespace(namespace).Delete(context.Background(), item.GetName(), metav1.DeleteOptions{})
			}
			fmt.Println("All SimulatedDevicePool scenarios reset.")
			return nil
		},
	}
	scenarioCmd.AddCommand(scenarioApplyCmd)
	scenarioCmd.AddCommand(scenarioResetCmd)

	// 10. Inject / Clear Fault Commands
	var poolNameFlag string
	var faultTypeFlag string

	var injectFaultCmd = &cobra.Command{
		Use:   "inject-fault",
		Short: "Inject a fault into a SimulatedDevicePool",
		RunE: func(cmd *cobra.Command, args []string) error {
			if poolNameFlag == "" {
				return fmt.Errorf("--pool must be specified")
			}
			if faultTypeFlag == "" {
				return fmt.Errorf("--type must be specified (unhealthy, capacity-exhausted, disappear)")
			}
			if faultTypeFlag != "unhealthy" && faultTypeFlag != "capacity-exhausted" && faultTypeFlag != "disappear" {
				return fmt.Errorf("invalid fault type: %s", faultTypeFlag)
			}
			_, dynamicClient, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			gvr := schema.GroupVersionResource{
				Group:    "draforge.oaslananka",
				Version:  "v1alpha1",
				Resource: "simulateddevicepools",
			}
			sdp, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), poolNameFlag, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get pool %s/%s: %w", namespace, poolNameFlag, err)
			}
			err = unstructured.SetNestedField(sdp.Object, faultTypeFlag, "spec", "health")
			if err != nil {
				return err
			}
			_, err = dynamicClient.Resource(gvr).Namespace(namespace).Update(context.Background(), sdp, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			fmt.Printf("Successfully injected fault %q into pool %s/%s\n", faultTypeFlag, namespace, poolNameFlag)
			return nil
		},
	}
	injectFaultCmd.Flags().StringVar(&poolNameFlag, "pool", "", "Name of the SimulatedDevicePool")
	injectFaultCmd.Flags().StringVar(&faultTypeFlag, "type", "", "Type of fault (unhealthy, capacity-exhausted, disappear)")

	var clearFaultCmd = &cobra.Command{
		Use:   "clear-faults",
		Short: "Clear all faults from a SimulatedDevicePool",
		RunE: func(cmd *cobra.Command, args []string) error {
			if poolNameFlag == "" {
				return fmt.Errorf("--pool must be specified")
			}
			_, dynamicClient, _, err := cluster.NewClientset(kubeconfig)
			if err != nil {
				return err
			}
			gvr := schema.GroupVersionResource{
				Group:    "draforge.oaslananka",
				Version:  "v1alpha1",
				Resource: "simulateddevicepools",
			}
			sdp, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), poolNameFlag, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get pool %s/%s: %w", namespace, poolNameFlag, err)
			}
			err = unstructured.SetNestedField(sdp.Object, "healthy", "spec", "health")
			if err != nil {
				return err
			}
			_, err = dynamicClient.Resource(gvr).Namespace(namespace).Update(context.Background(), sdp, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			fmt.Printf("Successfully cleared faults from pool %s/%s\n", namespace, poolNameFlag)
			return nil
		},
	}
	clearFaultCmd.Flags().StringVar(&poolNameFlag, "pool", "", "Name of the SimulatedDevicePool")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(claimsCmd)
	rootCmd.AddCommand(graphCmd)
	rootCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(scenarioCmd)
	rootCmd.AddCommand(injectFaultCmd)
	rootCmd.AddCommand(clearFaultCmd)

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
