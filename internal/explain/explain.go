// Package explain implements the reasoning engine that explains ResourceClaim allocation statuses.
// SPDX-License-Identifier: Apache-2.0
package explain

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/internal/redaction"
	"github.com/oaslananka/draforge/pkg/model"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	attrRegex = regexp.MustCompile(`device\.attributes\[['"]([^'"]+)['"]\]\s*(==|!=)\s*['"]([^'"]*)['"]`)
	capRegex  = regexp.MustCompile(`device\.capacity\[['"]([^'"]+)['"]\]\s*(==|!=|>=|<=|>|<)\s*([0-9]+)`)
)

// evaluateCEL parses and evaluates simple CEL expressions against device attributes and capacities.
func evaluateCEL(expression string, attributes map[string]string, capacities map[string]int64) bool {
	if strings.TrimSpace(expression) == "" {
		return true
	}

	parts := strings.Split(expression, "&&")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if matches := attrRegex.FindStringSubmatch(part); len(matches) == 4 {
			key := matches[1]
			op := matches[2]
			val := matches[3]

			deviceVal, exists := attributes[key]
			if !exists {
				if op == "!=" {
					continue
				}
				return false
			}

			switch op {
			case "==":
				if deviceVal != val {
					return false
				}
			case "!=":
				if deviceVal == val {
					return false
				}
			}
			continue
		}

		if matches := capRegex.FindStringSubmatch(part); len(matches) == 4 {
			key := matches[1]
			op := matches[2]
			valStr := matches[3]
			val, _ := strconv.ParseInt(valStr, 10, 64)

			deviceVal, exists := capacities[key]
			if !exists {
				return false
			}

			switch op {
			case "==":
				if deviceVal != val {
					return false
				}
			case "!=":
				if deviceVal == val {
					return false
				}
			case ">=":
				if deviceVal < val {
					return false
				}
			case "<=":
				if deviceVal > val {
					return false
				}
			case ">":
				if deviceVal <= val {
					return false
				}
			case "<":
				if deviceVal >= val {
					return false
				}
			}
			continue
		}
	}

	return true
}

// ExplainClaim analyzes a ResourceClaim and returns an explanation tree.
func ExplainClaim(ctx context.Context, clientset kubernetes.Interface, namespace, claimName string) (*model.ExplainResult, error) {
	// 1. Fetch live DRA resources
	_, devices, claims, err := discovery.DiscoverDRA(ctx, clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to discover cluster state: %w", err)
	}

	// 2. Fetch actual ResourceClaim from API to check ReservedFor status
	liveClaim, err := clientset.ResourceV1().ResourceClaims(namespace).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ResourceClaim %s/%s: %w", namespace, claimName, err)
	}

	var target *model.ResourceClaimInfo
	for _, c := range claims {
		if c.Name == claimName && c.Namespace == namespace {
			target = &c
			break
		}
	}

	if target == nil {
		return nil, fmt.Errorf("ResourceClaim %s/%s not found", namespace, claimName)
	}

	result := &model.ExplainResult{
		TargetName: claimName,
		TargetType: "claim",
		Allocated:  target.Status == "Allocated",
		Remedy:     []string{},
	}

	if result.Allocated {
		result.ReasonTree = model.ReasonNode{
			Message:    "ResourceClaim successfully allocated.",
			Confidence: "confirmed",
			Evidence:   fmt.Sprintf("Claim status is Allocated. Bound to device %s on node %s.", target.AllocatedDevice, target.AllocatedNode),
			SourceType: "ResourceClaim",
			FieldPath:  ".status.allocation",
		}
		return result, nil
	}

	// Explain Pending Claim
	rootNode := model.ReasonNode{
		Message:    "Claim could not be allocated.",
		Confidence: "confirmed",
		Evidence:   "Claim status is Pending.",
		SourceType: "ResourceClaim",
		FieldPath:  ".status.allocation",
	}

	// Check DeviceClass existence
	classExists := false
	classNames, err := clientset.ResourceV1().DeviceClasses().List(ctx, metav1.ListOptions{})
	var deviceClass *resourcev1.DeviceClass
	if err == nil {
		for _, cls := range classNames.Items {
			if cls.Name == target.DeviceClassName {
				classExists = true
				deviceClass = &cls
				break
			}
		}
	}

	if !classExists {
		child := model.ReasonNode{
			Message:    fmt.Sprintf("Requested DeviceClass '%s' does not exist in the cluster.", target.DeviceClassName),
			Confidence: "confirmed",
			Evidence:   "DeviceClass list query returned no matches.",
			SourceType: "DeviceClass",
			FieldPath:  ".spec.deviceClassName",
		}
		rootNode.Children = append(rootNode.Children, child)
		result.Remedy = append(result.Remedy, fmt.Sprintf("Create the missing DeviceClass '%s' in the cluster.", target.DeviceClassName))
	}

	// Evaluate candidate devices & drivers matching selectors
	selectorPassed := 0
	capacityMatched := 0

	for _, d := range devices {
		// Evaluated against selectors in DeviceClass
		passedSelector := true
		if deviceClass != nil {
			for _, sel := range deviceClass.Spec.Selectors {
				if sel.CEL != nil {
					if !evaluateCEL(sel.CEL.Expression, d.Attributes, d.Capacities) {
						passedSelector = false
						break
					}
				}
			}
		}

		if !passedSelector {
			continue
		}
		selectorPassed++

		// Capacity / Availability check (check if already allocated)
		isAllocated := false
		for _, c := range claims {
			if c.Status == "Allocated" && c.AllocatedDevice == d.Name && c.AllocatedNode == d.NodeName {
				isAllocated = true
				break
			}
		}

		if isAllocated {
			continue
		}
		capacityMatched++
	}

	totalDevices := len(devices)
	if totalDevices == 0 {
		rootNode.Children = append(rootNode.Children, model.ReasonNode{
			Message:    "No devices discovered in the cluster.",
			Confidence: "confirmed",
			Evidence:   "ResourceSlice query returned 0 devices.",
			SourceType: "ResourceSlice",
		})
		result.Remedy = append(result.Remedy, "Register a DRA driver or deploy a SimulatedDevicePool scenario.")
	} else {
		rootNode.Children = append(rootNode.Children, model.ReasonNode{
			Message:    fmt.Sprintf("%d candidate devices evaluated", totalDevices),
			Confidence: "inferred",
			Evidence:   fmt.Sprintf("Evaluated %d devices: %d rejected due to selector mismatch.", totalDevices, totalDevices-selectorPassed),
			SourceType: "ResourceSlice",
			Children: []model.ReasonNode{
				{
					Message:    fmt.Sprintf("%d rejected because selector evaluated to false", totalDevices-selectorPassed),
					Confidence: "confirmed",
					SourceType: "DeviceClass",
				},
				{
					Message:    fmt.Sprintf("%d rejected because requested capacity (already allocated) was unavailable", selectorPassed-capacityMatched),
					Confidence: "inferred",
					SourceType: "ResourceSlice",
				},
			},
		})

		if capacityMatched == 0 && totalDevices > 0 {
			result.Remedy = append(result.Remedy, "Release existing allocations or increase device counts in the pool.")
		}
	}

	// 3. Incorporate cluster Events referencing the claim/pod
	eventsList, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ev := range eventsList.Items {
			if ev.InvolvedObject.Name == claimName && ev.InvolvedObject.Kind == "ResourceClaim" {
				redactedMsg := redaction.RedactString(ev.Message)
				rootNode.Children = append(rootNode.Children, model.ReasonNode{
					Message:    fmt.Sprintf("Cluster Event: %s", ev.Reason),
					Confidence: "probable",
					Evidence:   fmt.Sprintf("[%s] %s", ev.Type, redactedMsg),
					SourceType: "Event",
				})
			}
		}
	}

	// 4. Check for delayed allocation (WaitForFirstConsumer)
	if len(liveClaim.Status.ReservedFor) == 0 {
		rootNode.Children = append(rootNode.Children, model.ReasonNode{
			Message:    "Claim uses delayed allocation, waiting for first consumer pod.",
			Confidence: "confirmed",
			Evidence:   "status.reservedFor is empty, no active pod has claimed this resource.",
			SourceType: "ResourceClaim",
			FieldPath:  ".status.reservedFor",
		})
		result.Remedy = append(result.Remedy, "Deploy a Pod referencing this ResourceClaim to trigger allocation.")
	}

	result.ReasonTree = rootNode
	return result, nil
}
