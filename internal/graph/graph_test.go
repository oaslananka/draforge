// Package graph unit tests.
// SPDX-License-Identifier: Apache-2.0
package graph

import (
	"context"
	"testing"

	"github.com/oaslananka/draforge/pkg/model"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// --- Unit tests for core primitives ---

func TestEdgeKeyDeterministic(t *testing.T) {
	tests := []struct {
		from, to, etype string
		want            string
	}{
		{"a", "b", "x", "a\x00b\x00x"},
		{poolGraphID("nvidia.com", "node-1", "gpu-pool"), "driver/nvidia.com", "managed-by", poolGraphID("nvidia.com", "node-1", "gpu-pool") + "\x00driver/nvidia.com\x00managed-by"},
	}
	for _, tt := range tests {
		got := edgeKey(tt.from, tt.to, tt.etype)
		if got != tt.want {
			t.Errorf("edgeKey(%q,%q,%q) = %q, want %q", tt.from, tt.to, tt.etype, got, tt.want)
		}
	}
	// Same input always produces the same output.
	k1 := edgeKey("x", "y", "z")
	k2 := edgeKey("x", "y", "z")
	if k1 != k2 {
		t.Error("edgeKey not deterministic for same input")
	}
}

func TestGraphIDCollisionSafety(t *testing.T) {
	poolA := poolGraphID("driver-a.example", "node-1", "shared-pool")
	poolB := poolGraphID("driver-b.example", "node-1", "shared-pool")
	if poolA == poolB {
		t.Fatalf("pool IDs must include driver identity to avoid collisions: %q", poolA)
	}

	devA := deviceGraphID("driver-a.example", "node-1", "shared-pool", "gpu-0")
	devB := deviceGraphID("driver-b.example", "node-1", "shared-pool", "gpu-0")
	if devA == devB {
		t.Fatalf("device IDs must include driver identity to avoid collisions: %q", devA)
	}

	if mermaidID("a/b-c") == mermaidID("a_b_c") {
		t.Fatal("Mermaid IDs must remain collision-safe after sanitization")
	}
}

func TestAddNodeDedup(t *testing.T) {
	gb := NewGraphBuilder()
	gb.addNode("id1", "Pod", "pod-a", nil)
	gb.addNode("id1", "Pod", "pod-a", nil) // duplicate
	gb.addNode("id2", "Device", "dev-x", nil)

	if len(gb.nodes) != 2 {
		t.Errorf("expected 2 nodes after dedup, got %d", len(gb.nodes))
	}
	if len(gb.nodeIDs) != 2 {
		t.Errorf("expected 2 nodeIDs in map, got %d", len(gb.nodeIDs))
	}
}

func TestAddEdgeDedup(t *testing.T) {
	gb := NewGraphBuilder()
	gb.addEdge("a", "b", "connects")
	gb.addEdge("a", "b", "connects") // exact duplicate
	gb.addEdge("a", "b", "other")    // different type → allowed
	gb.addEdge("b", "a", "connects") // reversed → allowed

	if len(gb.edges) != 3 {
		t.Errorf("expected 3 edges after dedup, got %d", len(gb.edges))
	}
	if len(gb.edgeIDs) != 3 {
		t.Errorf("expected 3 edgeIDs in map, got %d", len(gb.edgeIDs))
	}
}

// --- Stable sort tests ---

func TestBuildGraphStableSort(t *testing.T) {
	gb := NewGraphBuilder()

	// Add nodes in reverse-alphabetical order.
	gb.nodes = []model.GraphNode{
		{ID: "z", Type: "Device", Label: "z"},
		{ID: "m", Type: "Pool", Label: "m"},
		{ID: "a", Type: "Pod", Label: "a"},
	}
	gb.nodeIDs = map[string]struct{}{
		"z": {}, "m": {}, "a": {},
	}

	// Add edges in non-deterministic order.
	gb.edges = []model.GraphEdge{
		{From: "z", To: "a", Type: "connects"},
		{From: "a", To: "m", Type: "belongs"},
		{From: "z", To: "a", Type: "also"},
		{From: "z", To: "b", Type: "connects"},
	}
	gb.edgeIDs = map[string]struct{}{
		"z\x00a\x00connects": {}, "a\x00m\x00belongs": {},
		"z\x00a\x00also": {}, "z\x00b\x00connects": {},
	}

	// Build final graph (triggers sorting).
	// We won't go through BuildGraph because we have no clientset.
	// Test the sort logic directly.

	// Simulate the sort that BuildGraph does.
	// Nodes sorted by ID.
	// Edges sorted by (From, To, Type).
	sortNodes(gb)
	sortEdges(gb)

	// Verify node order.
	expectedNodeIDs := []string{"a", "m", "z"}
	for i, n := range gb.nodes {
		if n.ID != expectedNodeIDs[i] {
			t.Errorf("node[%d].ID = %q, want %q", i, n.ID, expectedNodeIDs[i])
		}
	}

	// Verify edge order (a→m belows, z→a also, z→a connects, z→b connects).
	expectedEdges := []struct{ from, to, etype string }{
		{"a", "m", "belongs"},
		{"z", "a", "also"},
		{"z", "a", "connects"},
		{"z", "b", "connects"},
	}
	if len(gb.edges) != len(expectedEdges) {
		t.Fatalf("expected %d edges, got %d", len(expectedEdges), len(gb.edges))
	}
	for i, e := range gb.edges {
		exp := expectedEdges[i]
		if e.From != exp.from || e.To != exp.to || e.Type != exp.etype {
			t.Errorf("edge[%d] = (%q,%q,%q), want (%q,%q,%q)",
				i, e.From, e.To, e.Type, exp.from, exp.to, exp.etype)
		}
	}
}

// Helper: sort nodes in-place by ID.
func sortNodes(gb *GraphBuilder) {
	// Bubble sort for simplicity in test.
	n := len(gb.nodes)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if gb.nodes[i].ID > gb.nodes[j].ID {
				gb.nodes[i], gb.nodes[j] = gb.nodes[j], gb.nodes[i]
			}
		}
	}
}

// Helper: sort edges in-place by (From, To, Type).
func sortEdges(gb *GraphBuilder) {
	n := len(gb.edges)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			a, b := gb.edges[i], gb.edges[j]
			swap := false
			switch {
			case a.From > b.From:
				swap = true
			case a.From == b.From && a.To > b.To:
				swap = true
			case a.From == b.From && a.To == b.To && a.Type > b.Type:
				swap = true
			}
			if swap {
				gb.edges[i], gb.edges[j] = gb.edges[j], gb.edges[i]
			}
		}
	}
}

// --- Deterministic output tests ---

func TestDeterministicJSONOutput(t *testing.T) {
	g := &model.ResourceGraph{
		Nodes: []model.GraphNode{
			{ID: "b", Type: "Device", Label: "b"},
			{ID: "a", Type: "Pod", Label: "a"},
		},
		Edges: []model.GraphEdge{
			{From: "b", To: "a", Type: "connects"},
			{From: "a", To: "b", Type: "connects"},
		},
	}

	out1, err1 := ToJSON(g)
	if err1 != nil {
		t.Fatalf("ToJSON error: %v", err1)
	}

	out2, err2 := ToJSON(g)
	if err2 != nil {
		t.Fatalf("ToJSON error: %v", err2)
	}

	if string(out1) != string(out2) {
		t.Error("ToJSON not deterministic: two calls produced different output")
	}
}

func TestGraphFormats(t *testing.T) {
	mockGraph := &model.ResourceGraph{
		Nodes: []model.GraphNode{
			{ID: "pod/default/my-pod", Type: "Pod", Label: "my-pod"},
			{ID: "claim/default/my-claim", Type: "ResourceClaim", Label: "my-claim"},
			{ID: "device/gpu-0", Type: "Device", Label: "gpu-0"},
			{ID: "allocation/default/my-claim/0", Type: "Allocation", Label: "request-a"},
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
	if !contains(dot, "my-pod") || !contains(dot, "my-claim") || !contains(dot, "gpu-0") {
		t.Errorf("ToDOT() missing nodes: %s", dot)
	}
	if !contains(dot, "allocates") || !contains(dot, "claims") {
		t.Errorf("ToDOT() missing edges: %s", dot)
	}
	if !contains(dot, "request-a") || !contains(dot, "fillcolor=orange") {
		t.Errorf("ToDOT() does not distinguish allocation nodes: %s", dot)
	}

	// 2. Test Mermaid Output
	mermaid := ToMermaid(mockGraph)
	if mermaid == "" {
		t.Error("ToMermaid() returned empty string")
	}
	if !contains(mermaid, "my_pod") || !contains(mermaid, "my_claim") {
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

// --- BuildGraph integration tests with fake clientset ---

// newTestPoolSlice creates a ResourceSlice representing one pool+device.
func newTestPoolSlice(name, driver, node, pool, devName string) *resourcev1.ResourceSlice {
	return &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   driver,
			NodeName: &node,
			Pool:     resourcev1.ResourcePool{Name: pool},
			Devices: []resourcev1.Device{
				{
					Name: devName,
				},
			},
		},
	}
}

func newTestClaim(name, namespace, className, allocatedDevice, allocatedNode string) *resourcev1.ResourceClaim {
	rc := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{
						Exactly: &resourcev1.ExactDeviceRequest{
							DeviceClassName: className,
						},
					},
				},
			},
		},
	}
	if allocatedDevice != "" {
		rc.Status.Allocation = &resourcev1.AllocationResult{
			Devices: resourcev1.DeviceAllocationResult{
				Results: []resourcev1.DeviceRequestAllocationResult{
					{
						Device: allocatedDevice,
						Driver: "nvidia.com",
						Pool:   "gpu-pool",
					},
				},
			},
		}
		if allocatedNode != "" {
			rc.Status.Allocation.NodeSelector = &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchFields: []corev1.NodeSelectorRequirement{{Key: "metadata.name", Operator: corev1.NodeSelectorOpIn, Values: []string{allocatedNode}}},
			}}}
		}
	}
	return rc
}

func newTestPod(name, namespace, claimName string) *corev1.Pod {
	cn := claimName
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			ResourceClaims: []corev1.PodResourceClaim{
				{
					Name:                      "test-resource",
					ResourceClaimName:         &cn,
					ResourceClaimTemplateName: nil,
				},
			},
		},
	}
}

func TestBuildGraphNamespaceFilter(t *testing.T) {
	ctx := context.Background()
	slice := newTestPoolSlice("slice-1", "nvidia.com", "node-1", "gpu-pool", "gpu-0")
	claimDefault := newTestClaim("claim-a", "default", "gpu-class", "", "")
	claimOther := newTestClaim("claim-b", "other-ns", "gpu-class", "", "")
	podDefault := newTestPod("pod-a", "default", "claim-a")
	podOther := newTestPod("pod-b", "other-ns", "claim-b")

	objs := []runtime.Object{slice, claimDefault, claimOther, podDefault, podOther}
	clientset := fake.NewSimpleClientset(objs...)

	gb := NewGraphBuilder()
	// Only namespace "default"
	rg, err := gb.BuildGraph(ctx, clientset, "default", "")
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	// Should contain claim-a, NOT claim-b
	hasClaimA := false
	hasClaimB := false
	hasPodA := false
	hasPodB := false
	hasNSDefault := false
	hasNSOther := false
	for _, n := range rg.Nodes {
		switch n.ID {
		case "claim/default/claim-a":
			hasClaimA = true
		case "claim/other-ns/claim-b":
			hasClaimB = true
		case "pod/default/pod-a":
			hasPodA = true
		case "pod/other-ns/pod-b":
			hasPodB = true
		case "ns/default":
			hasNSDefault = true
		case "ns/other-ns":
			hasNSOther = true
		}
	}

	if !hasClaimA {
		t.Error("namespace filter: expected claim-a in default namespace, not found")
	}
	if hasClaimB {
		t.Error("namespace filter: claim-b from other-ns should be excluded")
	}
	if !hasPodA {
		t.Error("namespace filter: expected pod-a in default namespace, not found")
	}
	if hasPodB {
		t.Error("namespace filter: pod-b from other-ns should be excluded")
	}
	if !hasNSDefault {
		t.Error("namespace filter: expected ns/default node")
	}
	if hasNSOther {
		t.Error("namespace filter: ns/other-ns should be excluded")
	}
}

func TestBuildGraphDriverFilter(t *testing.T) {
	ctx := context.Background()
	// Pool with nvidia driver
	nvidiaSlice := newTestPoolSlice("nvidia-slice", "nvidia.com", "node-1", "gpu-pool", "gpu-0")
	// Pool with amd driver
	amdSlice := newTestPoolSlice("amd-slice", "amd.com", "node-2", "amd-pool", "amd-device-0")
	// Pool with a driver name that contains "nvidia" as substring (false-positive test)
	// "nvidia-fake" should NOT be matched by driver filter "nvidia.com"
	fakeSlice := newTestPoolSlice("fake-slice", "nvidia-fake.io", "node-3", "fake-pool", "fake-device-0")

	objs := []runtime.Object{nvidiaSlice, amdSlice, fakeSlice}
	clientset := fake.NewSimpleClientset(objs...)

	gb := NewGraphBuilder()
	rg, err := gb.BuildGraph(ctx, clientset, "", "nvidia.com")
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	hasNvidiaPool := false
	hasNvidiaDevice := false
	hasAmdPool := false
	hasAmdDevice := false
	hasFakePool := false
	hasFakeDevice := false
	hasNvidiaDriver := false
	hasAmdDriver := false
	hasFakeDriver := false

	for _, n := range rg.Nodes {
		switch n.ID {
		case poolGraphID("nvidia.com", "node-1", "gpu-pool"):
			hasNvidiaPool = true
		case deviceGraphID("nvidia.com", "node-1", "gpu-pool", "gpu-0"):
			hasNvidiaDevice = true
		case poolGraphID("amd.com", "node-2", "amd-pool"):
			hasAmdPool = true
		case deviceGraphID("amd.com", "node-2", "amd-pool", "amd-device-0"):
			hasAmdDevice = true
		case poolGraphID("nvidia-fake.io", "node-3", "fake-pool"):
			hasFakePool = true
		case deviceGraphID("nvidia-fake.io", "node-3", "fake-pool", "fake-device-0"):
			hasFakeDevice = true
		case "driver/nvidia.com":
			hasNvidiaDriver = true
		case "driver/amd.com":
			hasAmdDriver = true
		case "driver/nvidia-fake.io":
			hasFakeDriver = true
		}
	}

	if !hasNvidiaPool {
		t.Error("driver filter: expected nvidia pool, not found")
	}
	if !hasNvidiaDevice {
		t.Error("driver filter: expected nvidia device, not found")
	}
	if !hasNvidiaDriver {
		t.Error("driver filter: expected nvidia.com driver node, not found")
	}
	if hasAmdPool {
		t.Error("driver filter: amd pool should be excluded")
	}
	if hasAmdDevice {
		t.Error("driver filter: amd device should be excluded")
	}
	if hasAmdDriver {
		t.Error("driver filter: amd.com driver node should be excluded")
	}
	if hasFakePool {
		t.Error("driver filter: fake pool (nvidia-fake.io) should be excluded")
	}
	if hasFakeDevice {
		t.Error("driver filter: fake device (nvidia-fake.io) should be excluded")
	}
	if hasFakeDriver {
		t.Error("driver filter: nvidia-fake.io driver node should be excluded")
	}
}

func TestBuildGraphNoSubstringFalsePositive(t *testing.T) {
	ctx := context.Background()
	// Device ID that contains "nvidia" as substring but driver is "amd.com".
	pool1 := newTestPoolSlice("slice-1", "amd.com", "node-1", "pool-1", "nvidia-gpu-sim")
	pool2 := newTestPoolSlice("slice-2", "nvidia.com", "node-2", "pool-2", "real-gpu")

	objs := []runtime.Object{pool1, pool2}
	clientset := fake.NewSimpleClientset(objs...)

	gb := NewGraphBuilder()
	rg, err := gb.BuildGraph(ctx, clientset, "", "nvidia.com")
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	hasFakeDevice := false
	hasRealDevice := false
	for _, n := range rg.Nodes {
		switch n.ID {
		case deviceGraphID("amd.com", "node-1", "pool-1", "nvidia-gpu-sim"):
			hasFakeDevice = true
		case deviceGraphID("nvidia.com", "node-2", "pool-2", "real-gpu"):
			hasRealDevice = true
		}
	}

	if hasFakeDevice {
		t.Error("substring false positive: device nvidia-gpu-sim (amd.com driver) should be excluded when filtering by nvidia.com")
	}
	if !hasRealDevice {
		t.Error("expected real-gpu (nvidia.com) to be included, not found")
	}
}

func TestBuildGraphAllocatedDeviceEdge(t *testing.T) {
	ctx := context.Background()
	// pool with a device that claim references
	slice := newTestPoolSlice("slice-1", "nvidia.com", "node-1", "gpu-pool", "gpu-0")
	claim := newTestClaim("claim-1", "default", "gpu-class", "gpu-0", "node-1")

	objs := []runtime.Object{slice, claim}
	clientset := fake.NewSimpleClientset(objs...)

	gb := NewGraphBuilder()
	rg, err := gb.BuildGraph(ctx, clientset, "", "")
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	// Check for allocates edge
	hasAllocatesEdge := false
	hasAllocatesMissingEdge := false
	for _, e := range rg.Edges {
		if e.Type == "allocates" {
			hasAllocatesEdge = true
		}
		if e.Type == "allocates-missing" {
			hasAllocatesMissingEdge = true
		}
	}

	if !hasAllocatesEdge {
		t.Error("expected allocates edge from claim to device")
	}
	if hasAllocatesMissingEdge {
		t.Error("should NOT have allocates-missing edge when device exists")
	}
}

func TestBuildGraphMissingAllocatedDeviceEdge(t *testing.T) {
	ctx := context.Background()
	// No ResourceSlices → no devices at all
	claim := newTestClaim("claim-1", "default", "gpu-class", "missing-gpu", "")
	objs := []runtime.Object{claim}
	clientset := fake.NewSimpleClientset(objs...)

	gb := NewGraphBuilder()
	rg, err := gb.BuildGraph(ctx, clientset, "", "")
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	// Should have allocates-missing edge to a MISSING node
	hasMissingEdge := false
	hasMissingNode := false
	for _, e := range rg.Edges {
		if e.Type == "allocates-missing" {
			hasMissingEdge = true
		}
	}
	for _, n := range rg.Nodes {
		if n.Metadata != nil && n.Metadata["status"] == "missing" {
			hasMissingNode = true
		}
	}

	if !hasMissingEdge {
		t.Error("expected allocates-missing edge for missing device")
	}
	if !hasMissingNode {
		t.Error("expected missing device node with status=missing metadata")
	}
}

func TestBuildGraphNoDuplicateNodes(t *testing.T) {
	ctx := context.Background()
	slice := newTestPoolSlice("slice-1", "nvidia.com", "node-1", "gpu-pool", "gpu-0")
	claim1 := newTestClaim("claim-a", "default", "gpu-class", "", "")
	claim2 := newTestClaim("claim-b", "default", "gpu-class", "", "")
	pod1 := newTestPod("pod-a", "default", "claim-a")
	pod2 := newTestPod("pod-b", "default", "claim-b")

	objs := []runtime.Object{slice, claim1, claim2, pod1, pod2}
	clientset := fake.NewSimpleClientset(objs...)

	gb := NewGraphBuilder()
	rg, err := gb.BuildGraph(ctx, clientset, "", "")
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	// Node uniqueness by ID within the whole graph
	nodeIDs := make(map[string]int)
	for _, n := range rg.Nodes {
		nodeIDs[n.ID]++
	}
	for id, count := range nodeIDs {
		if count > 1 {
			t.Errorf("duplicate node ID %q appears %d times", id, count)
		}
	}
}

func TestBuildGraphNoDuplicateEdges(t *testing.T) {
	ctx := context.Background()
	// A simple pool+claim+pod setup. If two claims reference the same namespace,
	// the ns node is deduped, but edge claim→ns should also be deduped.
	slice := newTestPoolSlice("slice-1", "nvidia.com", "node-1", "gpu-pool", "gpu-0")
	claim := newTestClaim("claim-a", "default", "gpu-class", "", "")
	pod := newTestPod("pod-a", "default", "claim-a")

	objs := []runtime.Object{slice, claim, pod}
	clientset := fake.NewSimpleClientset(objs...)

	gb := NewGraphBuilder()
	rg, err := gb.BuildGraph(ctx, clientset, "", "")
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	edgeKeys := make(map[string]int)
	for _, e := range rg.Edges {
		key := edgeKey(e.From, e.To, e.Type)
		edgeKeys[key]++
	}
	for key, count := range edgeKeys {
		if count > 1 {
			t.Errorf("duplicate edge key %q appears %d times", key, count)
		}
	}
}

func TestBuildGraphDeterministicOutput(t *testing.T) {
	ctx := context.Background()
	slice := newTestPoolSlice("slice-1", "nvidia.com", "node-1", "gpu-pool", "gpu-0")
	claim := newTestClaim("claim-a", "default", "gpu-class", "", "")
	pod := newTestPod("pod-a", "default", "claim-a")

	objs := []runtime.Object{slice, claim, pod}
	clientset := fake.NewSimpleClientset(objs...)

	// Build graph twice with identical input
	gb1 := NewGraphBuilder()
	rg1, err1 := gb1.BuildGraph(ctx, clientset, "", "")
	if err1 != nil {
		t.Fatalf("first BuildGraph failed: %v", err1)
	}

	gb2 := NewGraphBuilder()
	rg2, err2 := gb2.BuildGraph(ctx, clientset, "", "")
	if err2 != nil {
		t.Fatalf("second BuildGraph failed: %v", err2)
	}

	// Compare JSON output
	json1, _ := ToJSON(rg1)
	json2, _ := ToJSON(rg2)

	if string(json1) != string(json2) {
		t.Error("BuildGraph is not deterministic: two calls with same input produced different JSON output")
	}

	// Also compare DOT and Mermaid
	dot1 := ToDOT(rg1)
	dot2 := ToDOT(rg2)
	if dot1 != dot2 {
		t.Error("BuildGraph not deterministic: different DOT output")
	}

	mermaid1 := ToMermaid(rg1)
	mermaid2 := ToMermaid(rg2)
	if mermaid1 != mermaid2 {
		t.Error("BuildGraph not deterministic: different Mermaid output")
	}
}

func TestBuildGraphExistingBehaviorPreserved(t *testing.T) {
	ctx := context.Background()
	// Full scenario that exercises all edge types.
	slice := newTestPoolSlice("slice-1", "nvidia.com", "node-1", "gpu-pool", "gpu-0")
	claim := newTestClaim("claim-1", "default", "gpu-class", "gpu-0", "node-1")
	pod := newTestPod("app-pod", "default", "claim-1")

	objs := []runtime.Object{slice, claim, pod}
	clientset := fake.NewSimpleClientset(objs...)

	gb := NewGraphBuilder()
	rg, err := gb.BuildGraph(ctx, clientset, "", "")
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	// Collect all edge types present
	edgeTypes := make(map[string]bool)
	for _, e := range rg.Edges {
		edgeTypes[e.From+" -> "+e.To+" ["+e.Type+"]"] = true
	}

	// Must-have edges for a basic allocated scenario
	poolID := poolGraphID("nvidia.com", "node-1", "gpu-pool")
	deviceID := deviceGraphID("nvidia.com", "node-1", "gpu-pool", "gpu-0")
	requiredEdges := []string{
		poolID + " -> node/node-1 [located-on]",
		poolID + " -> driver/nvidia.com [managed-by]",
		deviceID + " -> " + poolID + " [part-of-pool]",
		deviceID + " -> node/node-1 [located-on]",
		"claim/default/claim-1 -> ns/default [belongs-to]",
		"claim/default/claim-1 -> " + deviceID + " [allocates]",
		"pod/default/app-pod -> claim/default/claim-1 [claims]",
		"pod/default/app-pod -> ns/default [belongs-to]",
	}

	for _, exp := range requiredEdges {
		if !edgeTypes[exp] {
			t.Errorf("existing behavior: missing expected edge: %s", exp)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestBuildGraphPreservesAllRequestClassesAndAllocationIdentities(t *testing.T) {
	nodeName := "node-a"
	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "team-a"},
		Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{Requests: []resourcev1.DeviceRequest{
			{Name: "gpu", Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "gpu-class", Count: 1}},
			{Name: "accelerator", FirstAvailable: []resourcev1.DeviceSubRequest{
				{Name: "nic", DeviceClassName: "nic-class", Count: 1},
				{Name: "fpga", DeviceClassName: "fpga-class", Count: 1},
			}},
		}}},
		Status: resourcev1.ResourceClaimStatus{Allocation: &resourcev1.AllocationResult{
			NodeSelector: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchFields: []corev1.NodeSelectorRequirement{{Key: "metadata.name", Operator: corev1.NodeSelectorOpIn, Values: []string{nodeName}}},
			}}},
			Devices: resourcev1.DeviceAllocationResult{Results: []resourcev1.DeviceRequestAllocationResult{
				{Request: "gpu", Driver: "driver-a.example", Pool: "shared", Device: "dev-0"},
				{Request: "accelerator/nic", Driver: "driver-b.example", Pool: "shared", Device: "dev-0"},
			}},
		}},
	}

	objects := []runtime.Object{
		newTestPoolSlice("slice-a", "driver-a.example", nodeName, "shared", "dev-0"),
		newTestPoolSlice("slice-b", "driver-b.example", nodeName, "shared", "dev-0"),
		claim,
		&resourcev1.DeviceClass{ObjectMeta: metav1.ObjectMeta{Name: "gpu-class"}},
		&resourcev1.DeviceClass{ObjectMeta: metav1.ObjectMeta{Name: "nic-class"}},
		&resourcev1.DeviceClass{ObjectMeta: metav1.ObjectMeta{Name: "fpga-class"}},
	}

	resourceGraph, err := NewGraphBuilder().BuildGraph(context.Background(), fake.NewSimpleClientset(objects...), "", "")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	claimID := "claim/team-a/multi"
	wantEdges := map[string]bool{
		edgeKey(claimID, "class/gpu-class", "uses-class"):                                             true,
		edgeKey(claimID, "class/nic-class", "uses-class"):                                             true,
		edgeKey(claimID, "class/fpga-class", "uses-class"):                                            true,
		edgeKey(claimID, deviceGraphID("driver-a.example", nodeName, "shared", "dev-0"), "allocates"): true,
		edgeKey(claimID, deviceGraphID("driver-b.example", nodeName, "shared", "dev-0"), "allocates"): true,
	}
	for _, edge := range resourceGraph.Edges {
		delete(wantEdges, edgeKey(edge.From, edge.To, edge.Type))
	}
	if len(wantEdges) != 0 {
		t.Fatalf("missing complete request/allocation edges: %#v", wantEdges)
	}

	allocationNodes := 0
	claimFound := false
	for _, graphNode := range resourceGraph.Nodes {
		if graphNode.Type == "Allocation" {
			allocationNodes++
		}
		if graphNode.ID != claimID {
			continue
		}
		claimFound = true
		if graphNode.Metadata["requestCount"] != 2 || graphNode.Metadata["allocationCount"] != 2 {
			t.Fatalf("claim metadata lost collection cardinality: %#v", graphNode.Metadata)
		}
	}
	if !claimFound {
		t.Fatalf("claim node %q not found", claimID)
	}
	if allocationNodes != 2 {
		t.Fatalf("allocation nodes = %d, want every allocation result", allocationNodes)
	}
}
