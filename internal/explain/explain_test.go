// Package explain unit tests.
// SPDX-License-Identifier: Apache-2.0
package explain

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oaslananka/draforge/pkg/model"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEvaluateCEL(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		attributes  map[string]string
		capacities  map[string]int64
		expected    bool
		expectError bool
	}{
		{
			name:       "empty expression",
			expression: "",
			attributes: map[string]string{},
			capacities: map[string]int64{},
			expected:   true,
		},
		{
			name:       "attribute equality match",
			expression: `device.attributes["family"] == "h100"`,
			attributes: map[string]string{"family": "h100"},
			capacities: map[string]int64{},
			expected:   true,
		},
		{
			name:       "attribute equality mismatch",
			expression: `device.attributes["family"] == "h100"`,
			attributes: map[string]string{"family": "a100"},
			capacities: map[string]int64{},
			expected:   false,
		},
		{
			name:       "attribute inequality match",
			expression: `device.attributes["family"] != "h100"`,
			attributes: map[string]string{"family": "a100"},
			capacities: map[string]int64{},
			expected:   true,
		},
		{
			name:        "attribute missing and inequality",
			expression:  `device.attributes["family"] != "h100"`,
			attributes:  map[string]string{},
			capacities:  map[string]int64{},
			expectError: true, // CEL evaluating missing map key is an error
		},
		{
			name:       "capacity comparison match",
			expression: `device.capacity["memory"] >= 80000000000`,
			attributes: map[string]string{},
			capacities: map[string]int64{"memory": 80000000000},
			expected:   true,
		},
		{
			name:       "capacity comparison mismatch",
			expression: `device.capacity["memory"] >= 80000000000`,
			attributes: map[string]string{},
			capacities: map[string]int64{"memory": 40000000000},
			expected:   false,
		},
		{
			name:       "compound expression match",
			expression: `device.attributes["family"] == "h100" && device.capacity["memory"] >= 80000000000`,
			attributes: map[string]string{"family": "h100"},
			capacities: map[string]int64{"memory": 80000000000},
			expected:   true,
		},
		{
			name:       "list membership match",
			expression: `device.attributes["family"] in ["a100", "h100"]`,
			attributes: map[string]string{"family": "h100"},
			capacities: map[string]int64{},
			expected:   true,
		},
		{
			name:       "list membership mismatch",
			expression: `device.attributes["family"] in ["a100", "h100"]`,
			attributes: map[string]string{"family": "t4"},
			capacities: map[string]int64{},
			expected:   false,
		},
		{
			name:        "invalid expression syntax",
			expression:  `device.attributes["family"] =? "h100"`,
			attributes:  map[string]string{"family": "h100"},
			capacities:  map[string]int64{},
			expectError: true,
		},
		{
			name:        "non-boolean return type",
			expression:  `device.attributes["family"]`,
			attributes:  map[string]string{"family": "h100"},
			capacities:  map[string]int64{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateCEL(tt.expression, tt.attributes, tt.capacities)
			if tt.expectError {
				if err == nil {
					t.Errorf("evaluateCEL(%q) expected error but got none", tt.expression)
				}
			} else {
				if err != nil {
					t.Errorf("evaluateCEL(%q) unexpected error: %v", tt.expression, err)
				}
				if result != tt.expected {
					t.Errorf("evaluateCEL(%q) = %v, expected %v", tt.expression, result, tt.expected)
				}
			}
		})
	}
}

func TestExplainClaim_Success(t *testing.T) {
	ctx := context.Background()
	claimName := "success-claim"
	namespace := "default"

	// Create objects for a successfully allocated claim
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: []resourcev1.DeviceRequestAllocationResult{
						{
							Device: "gpu-0",
							Driver: "gpu-driver",
							Pool:   "gpu-pool",
						},
					},
				},
				NodeSelector: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-0"},
								},
							},
						},
					},
				},
			},
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{
					Name: "gpu-pod",
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(claim)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	if !result.Allocated {
		t.Errorf("expected Allocated to be true, got false")
	}
	if !strings.Contains(result.ReasonTree.Message, "successfully allocated") {
		t.Errorf("unexpected reason message: %s", result.ReasonTree.Message)
	}
	if !strings.Contains(result.ReasonTree.Evidence, "Reserved for consumers: gpu-pod") {
		t.Errorf("expected Evidence to contain 'Reserved for consumers: gpu-pod', got: %s", result.ReasonTree.Evidence)
	}
}

func TestExplainClaim_MissingDeviceClass(t *testing.T) {
	ctx := context.Background()
	claimName := "pending-claim"
	namespace := "default"

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{
					Name: "gpu-pod",
				},
			},
		},
	}

	// No DeviceClass registered, and no devices registered
	clientset := fake.NewSimpleClientset(claim)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	if result.Allocated {
		t.Errorf("expected Allocated to be false, got true")
	}

	foundDeviceClassRemedy := false
	for _, remedy := range result.Remedy {
		if strings.Contains(remedy, "Create the missing DeviceClass 'gpu-class'") {
			foundDeviceClassRemedy = true
			break
		}
	}
	if !foundDeviceClassRemedy {
		t.Errorf("expected remedy to create DeviceClass, got: %v", result.Remedy)
	}
}

func TestExplainClaim_SelectorMismatchAndCapacity(t *testing.T) {
	ctx := context.Background()
	claimName := "gpu-claim"
	namespace := "default"
	nodeName := "node-0"

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{
					Name: "gpu-pod",
				},
			},
		},
	}

	class := &resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu-class",
		},
		Spec: resourcev1.DeviceClassSpec{
			Selectors: []resourcev1.DeviceSelector{
				{
					CEL: &resourcev1.CELDeviceSelector{
						Expression: `device.attributes["family"] == "h100"`,
					},
				},
			},
		},
	}

	// Device slice with 2 devices: one mismatching attributes, one matching but already allocated
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu-slice",
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "gpu-driver",
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name: "gpu-pool",
			},
			Devices: []resourcev1.Device{
				{
					Name: "gpu-mismatch",
					Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"family": {
							StringValue: ptr("a100"),
						},
					},
				},
				{
					Name: "gpu-match-allocated",
					Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"family": {
							StringValue: ptr("h100"),
						},
					},
				},
			},
		},
	}

	// A different claim that allocates gpu-match-allocated on node-0
	otherClaim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-claim",
			Namespace: namespace,
		},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: []resourcev1.DeviceRequestAllocationResult{
						{
							Device: "gpu-match-allocated",
							Driver: "gpu-driver",
							Pool:   "gpu-pool",
						},
					},
				},
				NodeSelector: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{nodeName},
								},
							},
						},
					},
				},
			},
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{
					Name: "other-pod",
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(claim, class, slice, otherClaim)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	if result.Allocated {
		t.Errorf("expected Allocated to be false, got true")
	}

	// Verify the evaluation summary
	foundSummary := false
	for _, child := range result.ReasonTree.Children {
		if strings.Contains(child.Message, "devices evaluated") {
			foundSummary = true
			if len(child.Children) != 2 {
				t.Fatalf("expected 2 children for evaluation node, got %d", len(child.Children))
			}
			if !strings.Contains(child.Children[0].Message, "1 rejected because selector evaluated to false") {
				t.Errorf("unexpected selector mismatch child: %s", child.Children[0].Message)
			}
			if !strings.Contains(child.Children[1].Message, "1 rejected because requested capacity") {
				t.Errorf("unexpected capacity mismatch child: %s", child.Children[1].Message)
			}
		}
	}
	if !foundSummary {
		t.Errorf("expected evaluation summary node in children")
	}
}

func TestExplainClaim_EventsAndRedaction(t *testing.T) {
	ctx := context.Background()
	claimName := "event-claim"
	namespace := "default"

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{
					Name: "gpu-pod",
				},
			},
		},
	}

	dummyToken := "dop_v1_1111111122222222333333334444444455555555666666667777777788888888"
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claim-event",
			Namespace: namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "ResourceClaim",
			Name:      claimName,
			Namespace: namespace,
		},
		Reason:  "FailedAllocation",
		Message: "Failed to allocate: token is " + dummyToken,
		Type:    "Warning",
	}

	clientset := fake.NewSimpleClientset(claim, event)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	// Verify that Event exists in children and message is redacted
	foundEvent := false
	for _, child := range result.ReasonTree.Children {
		if strings.Contains(child.Message, "Cluster Event: FailedAllocation") {
			foundEvent = true
			if strings.Contains(child.Evidence, dummyToken) {
				t.Errorf("event message was not redacted: %s", child.Evidence)
			}
			if !strings.Contains(child.Evidence, "[REDACTED_DIGITALOCEAN_TOKEN]") {
				t.Errorf("expected [REDACTED_DIGITALOCEAN_TOKEN] in message: %s", child.Evidence)
			}
		}
	}
	if !foundEvent {
		t.Errorf("expected Cluster Event in children")
	}
}

func TestExplainClaim_DelayedAllocation(t *testing.T) {
	ctx := context.Background()
	claimName := "delayed-claim"
	namespace := "default"

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{}, // Empty, waiting for first consumer
		},
	}

	clientset := fake.NewSimpleClientset(claim)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	foundDelayed := false
	for _, child := range result.ReasonTree.Children {
		if strings.Contains(child.Message, "delayed allocation") {
			foundDelayed = true
			break
		}
	}
	if !foundDelayed {
		t.Errorf("expected delayed allocation message in children")
	}

	foundPodRemedy := false
	for _, remedy := range result.Remedy {
		if strings.Contains(remedy, "Deploy a Pod referencing this ResourceClaim") {
			foundPodRemedy = true
			break
		}
	}
	if !foundPodRemedy {
		t.Errorf("expected remedy to deploy a pod, got: %v", result.Remedy)
	}
}

func TestExplainClaim_UnhealthyDevices(t *testing.T) {
	ctx := context.Background()
	claimName := "unhealthy-device-claim"
	namespace := "default"
	nodeName := "node-1"

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{
					Name: "gpu-pod",
				},
			},
		},
	}

	class := &resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu-class",
		},
		Spec: resourcev1.DeviceClassSpec{
			Selectors: []resourcev1.DeviceSelector{
				{
					CEL: &resourcev1.CELDeviceSelector{
						Expression: `device.attributes["family"] == "h100"`,
					},
				},
			},
		},
	}

	// 1 healthy device (mismatched family), 1 unhealthy device (matched family)
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unhealthy-slice",
			Labels: map[string]string{
				"draforge.oaslananka/health": "degraded",
			},
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "gpu-driver",
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name: "gpu-pool",
			},
			Devices: []resourcev1.Device{
				{
					Name: "gpu-unhealthy",
					Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"family": {
							StringValue: ptr("h100"),
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(claim, class, slice)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	if result.Allocated {
		t.Errorf("expected Allocated to be false, got true")
	}

	// Verify that unhealthy rejection was recorded
	foundUnhealthyRejection := false
	for _, child := range result.ReasonTree.Children {
		if strings.Contains(child.Message, "devices evaluated") {
			for _, subChild := range child.Children {
				if strings.Contains(subChild.Message, "rejected because device health status was unhealthy") {
					foundUnhealthyRejection = true
				}
			}
		}
	}

	if !foundUnhealthyRejection {
		t.Errorf("expected explanation to include unhealthy device rejection message")
	}

	foundUnhealthyRemedy := false
	for _, remedy := range result.Remedy {
		if strings.Contains(remedy, "resolve health/degradation states") {
			foundUnhealthyRemedy = true
		}
	}

	if !foundUnhealthyRemedy {
		t.Errorf("expected remediation to mention resolving health states, got: %v", result.Remedy)
	}
}

func TestExplainClaim_InvalidCEL(t *testing.T) {
	ctx := context.Background()
	claimName := "invalid-cel-claim"
	namespace := "default"
	nodeName := "node-1"

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{
					Name: "gpu-pod",
				},
			},
		},
	}

	class := &resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu-class",
		},
		Spec: resourcev1.DeviceClassSpec{
			Selectors: []resourcev1.DeviceSelector{
				{
					CEL: &resourcev1.CELDeviceSelector{
						Expression: `device.attributes["family"] =? "h100"`, // Invalid syntax
					},
				},
			},
		},
	}

	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unhealthy-slice",
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "gpu-driver",
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name: "gpu-pool",
			},
			Devices: []resourcev1.Device{
				{
					Name: "gpu-1",
					Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"family": {
							StringValue: ptr("h100"),
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(claim, class, slice)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	if result.Allocated {
		t.Errorf("expected Allocated to be false, got true")
	}

	foundInvalidCELRejection := false
	for _, child := range result.ReasonTree.Children {
		if strings.Contains(child.Message, "devices evaluated") {
			for _, subChild := range child.Children {
				if strings.Contains(subChild.Message, "rejected because selector expression failed or was unsupported") {
					foundInvalidCELRejection = true
				}
			}
		}
	}

	if !foundInvalidCELRejection {
		t.Errorf("expected explanation to include invalid CEL rejection message")
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestExplainClaim_OmitZeroCountNodes(t *testing.T) {
	ctx := context.Background()
	claimName := "test-omit-zero"
	namespace := "default"
	nodeName := "node-1"

	// Create claim, device class, and a resource slice that creates a scenario
	// where ONLY capacity is missing, but no unhealthy devices or selector mismatches.
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "gpu-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{
					Name: "gpu-pod",
				},
			},
		},
	}

	class := &resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu-class",
		},
		Spec: resourcev1.DeviceClassSpec{
			Selectors: []resourcev1.DeviceSelector{
				{
					CEL: &resourcev1.CELDeviceSelector{
						Expression: `device.attributes["family"] == "h100"`,
					},
				},
			},
		},
	}

	// This slice contains a single h100 device that is healthy, but we will allocate it to a DIFFERENT claim
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-slice",
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   "gpu-driver",
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name: "gpu-pool",
			},
			Devices: []resourcev1.Device{
				{
					Name: "gpu-1",
					Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"family": {
							StringValue: ptr("h100"),
						},
					},
				},
			},
		},
	}

	otherClaim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-claim",
			Namespace: namespace,
		},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: []resourcev1.DeviceRequestAllocationResult{
						{
							Device: "gpu-1",
							Driver: "gpu-driver",
							Pool:   "gpu-pool",
						},
					},
				},
				NodeSelector: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{nodeName},
								},
							},
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(claim, class, slice, otherClaim)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	foundSummary := false
	for _, child := range result.ReasonTree.Children {
		if strings.Contains(child.Message, "devices evaluated") {
			foundSummary = true
			if len(child.Children) != 1 {
				t.Fatalf("expected 1 child for evaluation node (only capacity), got %d", len(child.Children))
			}
			if !strings.Contains(child.Children[0].Message, "1 rejected because requested capacity") {
				t.Errorf("unexpected child message: %s", child.Children[0].Message)
			}
		}
	}

	if !foundSummary {
		t.Errorf("expected explanation to include evaluation summary")
	}
}

func TestExplainClaim_PendingNodeSelector(t *testing.T) {
	ctx := context.Background()
	claimName := "pending-selector-claim"
	namespace := "default"

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              claimName,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: "test-class",
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{
				{Name: "test-consumer-pod"},
			},
			Allocation: &resourcev1.AllocationResult{
				NodeSelector: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-0"},
								},
							},
						},
					},
				},
			},
		},
	}

	class := &resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
	}

	clientset := fake.NewSimpleClientset(claim, class)
	result, err := ExplainClaim(ctx, clientset, namespace, claimName)
	if err != nil {
		t.Fatalf("ExplainClaim failed: %v", err)
	}

	if result.Allocated {
		t.Errorf("expected Allocated to be false, got true")
	}

	foundNodeSelectorReason := false
	for _, child := range result.ReasonTree.Children {
		if strings.Contains(child.Message, "Node selector computed") {
			foundNodeSelectorReason = true
			break
		}
	}
	if !foundNodeSelectorReason {
		t.Errorf("expected to find node selector reason node in children")
	}
}

func TestExplainClaim_WithAdvancedFeatures(t *testing.T) {
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim",
			Namespace: "default",
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Name: "req1",
						Exactly: &resourcev1.ExactDeviceRequest{
							Tolerations: []resourcev1.DeviceToleration{
								{Key: "test", Value: "val", Effect: resourcev1.DeviceTaintEffect("NoSchedule")},
							},
						},
					},
				},
			},
		},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				NodeSelector: &corev1.NodeSelector{},
			},
		},
	}

	clientset := fake.NewSimpleClientset(claim)

	result, err := ExplainClaim(context.TODO(), clientset, "default", "test-claim")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the informational node was added to the children of the pending node
	foundWarning := false
	for _, child := range result.ReasonTree.Children {
		if child.Confidence == "informational" && strings.Contains(child.Message, "advanced v1.36 features") {
			foundWarning = true
			break
		}
	}

	if !foundWarning {
		t.Errorf("expected warning node for advanced features, but it was not found")
	}
}

func TestExplainClaimAllocatedEvidenceIncludesEveryAllocation(t *testing.T) {
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "team-a"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: []resourcev1.DeviceRequest{
			{Name: "gpu", Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "gpu-class", Count: 2}},
		}}},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				NodeSelector: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchFields: []corev1.NodeSelectorRequirement{{Key: "metadata.name", Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}},
				}}},
				Devices: resourcev1.DeviceAllocationResult{Results: []resourcev1.DeviceRequestAllocationResult{
					{Request: "gpu", Driver: "driver-a.example", Pool: "shared", Device: "dev-0"},
					{Request: "gpu", Driver: "driver-b.example", Pool: "shared", Device: "dev-0"},
				}},
			},
			ReservedFor: []resourcev1.ResourceClaimConsumerReference{{Name: "consumer"}},
		},
	}

	result, err := ExplainClaim(context.Background(), fake.NewSimpleClientset(claim), "team-a", "multi")
	if err != nil {
		t.Fatalf("ExplainClaim: %v", err)
	}
	for _, identity := range []string{
		"gpu: driver-a.example/shared/dev-0 on node-a",
		"gpu: driver-b.example/shared/dev-0 on node-a",
	} {
		if !strings.Contains(result.ReasonTree.Evidence, identity) {
			t.Fatalf("allocated evidence missing %q: %s", identity, result.ReasonTree.Evidence)
		}
	}
}

func TestExplainClaimPendingPreservesEveryRequestedClass(t *testing.T) {
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "team-a"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: []resourcev1.DeviceRequest{
			{Name: "gpu", Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "missing-gpu"}},
			{Name: "accelerator", FirstAvailable: []resourcev1.DeviceSubRequest{
				{Name: "nic", DeviceClassName: "existing-nic"},
				{Name: "fpga", DeviceClassName: "missing-fpga"},
			}},
		}}},
	}
	class := &resourcev1.DeviceClass{ObjectMeta: metav1.ObjectMeta{Name: "existing-nic"}}

	result, err := ExplainClaim(context.Background(), fake.NewSimpleClientset(claim, class), "team-a", "multi")
	if err != nil {
		t.Fatalf("ExplainClaim: %v", err)
	}

	allMessages := result.ReasonTree.Message + "\n" + result.ReasonTree.Evidence
	var walk func([]model.ReasonNode)
	walk = func(nodes []model.ReasonNode) {
		for _, node := range nodes {
			allMessages += "\n" + node.Message + "\n" + node.Evidence
			walk(node.Children)
		}
	}
	walk(result.ReasonTree.Children)
	for _, className := range []string{"missing-gpu", "existing-nic", "missing-fpga"} {
		if !strings.Contains(allMessages, className) {
			t.Fatalf("explanation does not expose class %q:\n%s", className, allMessages)
		}
	}
	for _, className := range []string{"missing-gpu", "missing-fpga"} {
		found := false
		for _, remedy := range result.Remedy {
			found = found || strings.Contains(remedy, className)
		}
		if !found {
			t.Fatalf("missing remediation for class %q: %#v", className, result.Remedy)
		}
	}
}
