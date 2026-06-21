// Package explain unit tests.
// SPDX-License-Identifier: Apache-2.0
package explain

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEvaluateCEL(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		attributes map[string]string
		capacities map[string]int64
		expected   bool
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
			name:       "attribute missing and inequality",
			expression: `device.attributes["family"] != "h100"`,
			attributes: map[string]string{},
			capacities: map[string]int64{},
			expected:   true,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluateCEL(tt.expression, tt.attributes, tt.capacities)
			if result != tt.expected {
				t.Errorf("evaluateCEL(%q) = %v, expected %v", tt.expression, result, tt.expected)
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
			if len(child.Children) != 3 {
				t.Fatalf("expected 3 children for evaluation node, got %d", len(child.Children))
			}
			if !strings.Contains(child.Children[0].Message, "1 rejected because selector evaluated to false") {
				t.Errorf("unexpected selector mismatch child: %s", child.Children[0].Message)
			}
			if !strings.Contains(child.Children[1].Message, "0 rejected because device health status was unhealthy") {
				t.Errorf("unexpected unhealthy check child: %s", child.Children[1].Message)
			}
			if !strings.Contains(child.Children[2].Message, "1 rejected because requested capacity") {
				t.Errorf("unexpected capacity mismatch child: %s", child.Children[2].Message)
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

func ptr[T any](v T) *T {
	return &v
}
