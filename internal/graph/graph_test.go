// Package graph unit tests.
// SPDX-License-Identifier: Apache-2.0
package graph

import (
	"strings"
	"testing"

	"github.com/oaslananka/draforge/pkg/model"
)

func TestGraphFormats(t *testing.T) {
	mockGraph := &model.ResourceGraph{
		Nodes: []model.GraphNode{
			{ID: "pod/default/my-pod", Type: "Pod", Label: "my-pod"},
			{ID: "claim/default/my-claim", Type: "ResourceClaim", Label: "my-claim"},
			{ID: "device/gpu-0", Type: "Device", Label: "gpu-0"},
		},
		Edges: []model.GraphEdge{
			{From: "pod/default/my-pod", To: "claim/default/my-claim", Type: "claims"},
			{From: "claim/default/my-claim", To: "device/gpu-0", Type: "allocates"},
		},
	}

	// 1. Test DOT Output
	dot := ToDOT(mockGraph)
	if dot == "" {
		t.Error("ToDOT() returned empty string")
	}
	if !strings.Contains(dot, "my-pod") || !strings.Contains(dot, "my-claim") || !strings.Contains(dot, "gpu-0") {
		t.Errorf("ToDOT() missing nodes: %s", dot)
	}
	if !strings.Contains(dot, "allocates") || !strings.Contains(dot, "claims") {
		t.Errorf("ToDOT() missing edges: %s", dot)
	}

	// 2. Test Mermaid Output
	mermaid := ToMermaid(mockGraph)
	if mermaid == "" {
		t.Error("ToMermaid() returned empty string")
	}
	if !strings.Contains(mermaid, "my_pod") || !strings.Contains(mermaid, "my_claim") {
		t.Errorf("ToMermaid() missing formatted IDs: %s", mermaid)
	}

	// 3. Test JSON Output
	jsonData, err := ToJSON(mockGraph)
	if err != nil {
		t.Fatalf("ToJSON() failed: %v", err)
	}
	if len(jsonData) == 0 {
		t.Error("ToJSON() returned empty byte slice")
	}
}
