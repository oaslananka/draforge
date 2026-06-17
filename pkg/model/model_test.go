// Package model unit tests.
// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDeviceDefaults(t *testing.T) {
	d := Device{}
	if d.ID != "" {
		t.Errorf("expected empty ID, got %q", d.ID)
	}
	if d.Attributes != nil {
		t.Error("expected nil Attributes map")
	}
	if d.Capacities != nil {
		t.Error("expected nil Capacities map")
	}
	if !d.LastUpdated.IsZero() {
		t.Error("expected zero LastUpdated")
	}
}

func TestDeviceJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := Device{
		ID:          "dev-1",
		Name:        "gpu-0",
		Type:        "gpu",
		Status:      "healthy",
		NodeName:    "node-0",
		PoolName:    "pool-gpu",
		IsSynthetic: true,
		LastUpdated: now,
		Attributes:  map[string]string{"vendor": "nvidia", "model": "h100"},
		Capacities:  map[string]int64{"memory": 81920, "cores": 80},
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got Device
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.ID != d.ID {
		t.Errorf("ID: got %q, want %q", got.ID, d.ID)
	}
	if got.Name != d.Name {
		t.Errorf("Name: got %q, want %q", got.Name, d.Name)
	}
	if got.Type != d.Type {
		t.Errorf("Type: got %q, want %q", got.Type, d.Type)
	}
	if got.Status != d.Status {
		t.Errorf("Status: got %q, want %q", got.Status, d.Status)
	}
	if got.NodeName != d.NodeName {
		t.Errorf("NodeName: got %q, want %q", got.NodeName, d.NodeName)
	}
	if got.PoolName != d.PoolName {
		t.Errorf("PoolName: got %q, want %q", got.PoolName, d.PoolName)
	}
	if got.IsSynthetic != d.IsSynthetic {
		t.Errorf("IsSynthetic: got %v, want %v", got.IsSynthetic, d.IsSynthetic)
	}
	if !got.LastUpdated.Equal(d.LastUpdated) {
		t.Errorf("LastUpdated: got %v, want %v", got.LastUpdated, d.LastUpdated)
	}
	if got.Attributes["vendor"] != "nvidia" {
		t.Errorf("Attributes[vendor]: got %q, want nvidia", got.Attributes["vendor"])
	}
	if got.Capacities["memory"] != 81920 {
		t.Errorf("Capacities[memory]: got %d, want 81920", got.Capacities["memory"])
	}
}

func TestDevicePoolDefaults(t *testing.T) {
	p := DevicePool{}
	if p.Name != "" {
		t.Errorf("expected empty Name, got %q", p.Name)
	}
	if p.Health != "" {
		t.Errorf("expected empty Health, got %q", p.Health)
	}
	if p.Labels != nil {
		t.Error("expected nil Labels map")
	}
}

func TestDevicePoolJSONRoundTrip(t *testing.T) {
	p := DevicePool{
		Name:        "gpu-pool",
		DriverName:  "sim.draforge.oaslananka",
		NodeName:    "node-0",
		DeviceCount: 4,
		DeviceType:  "gpu",
		Health:      "healthy",
		IsSynthetic: true,
		Labels:      map[string]string{"region": "us-east"},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got DevicePool
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.Name != p.Name {
		t.Errorf("Name: got %q, want %q", got.Name, p.Name)
	}
	if got.DriverName != p.DriverName {
		t.Errorf("DriverName: got %q, want %q", got.DriverName, p.DriverName)
	}
	if got.DeviceCount != p.DeviceCount {
		t.Errorf("DeviceCount: got %d, want %d", got.DeviceCount, p.DeviceCount)
	}
	if got.Health != p.Health {
		t.Errorf("Health: got %q, want %q", got.Health, p.Health)
	}
	if got.Labels["region"] != "us-east" {
		t.Errorf("Labels[region]: got %q, want us-east", got.Labels["region"])
	}
}

func TestResourceClaimInfoDefaults(t *testing.T) {
	r := ResourceClaimInfo{}
	if r.Status != "" {
		t.Errorf("expected empty Status, got %q", r.Status)
	}
	if r.AllocatedDevice != "" {
		t.Errorf("expected empty AllocatedDevice, got %q", r.AllocatedDevice)
	}
}

func TestResourceClaimInfoJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	r := ResourceClaimInfo{
		Name:            "claim-1",
		Namespace:       "default",
		DeviceClassName: "gpu-class",
		Status:          "allocated",
		OwnerPodName:    "pod-1",
		AllocatedDevice: "dev-0",
		AllocatedNode:   "node-0",
		AllocatedDriver: "sim.draforge.oaslananka",
		CreatedAt:       now,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got ResourceClaimInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.Name != r.Name {
		t.Errorf("Name: got %q, want %q", got.Name, r.Name)
	}
	if got.Status != r.Status {
		t.Errorf("Status: got %q, want %q", got.Status, r.Status)
	}
	if got.AllocatedDevice != r.AllocatedDevice {
		t.Errorf("AllocatedDevice: got %q, want %q", got.AllocatedDevice, r.AllocatedDevice)
	}
}

func TestGraphNodeJSONRoundTrip(t *testing.T) {
	n := GraphNode{
		ID:    "node-1",
		Type:  "Pod",
		Label: "my-pod",
		Metadata: map[string]interface{}{
			"namespace": "default",
			"phase":     "Running",
		},
	}

	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got GraphNode
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.ID != n.ID {
		t.Errorf("ID: got %q, want %q", got.ID, n.ID)
	}
	if got.Type != n.Type {
		t.Errorf("Type: got %q, want %q", got.Type, n.Type)
	}
	if got.Label != n.Label {
		t.Errorf("Label: got %q, want %q", got.Label, n.Label)
	}
}

func TestGraphEdgeDirection(t *testing.T) {
	e := GraphEdge{
		From: "pod-1",
		To:   "claim-1",
		Type: "binds-to",
	}
	if e.From != "pod-1" {
		t.Errorf("From: got %q, want pod-1", e.From)
	}
	if e.To != "claim-1" {
		t.Errorf("To: got %q, want claim-1", e.To)
	}
	if e.Type != "binds-to" {
		t.Errorf("Type: got %q, want binds-to", e.Type)
	}
}

func TestResourceGraphStructure(t *testing.T) {
	g := ResourceGraph{
		Nodes: []GraphNode{
			{ID: "pod-1", Type: "Pod", Label: "test-pod"},
			{ID: "claim-1", Type: "Claim", Label: "test-claim"},
		},
		Edges: []GraphEdge{
			{From: "pod-1", To: "claim-1", Type: "binds-to"},
		},
	}

	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(g.Edges))
	}

	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got ResourceGraph
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(got.Nodes) != len(g.Nodes) {
		t.Errorf("node count mismatch: got %d, want %d", len(got.Nodes), len(g.Nodes))
	}
}

func TestReasonNodeTree(t *testing.T) {
	root := ReasonNode{
		Message:    "Claim pending",
		Confidence: "confirmed",
		Evidence:   "No matching node found",
		SourceType: "Claim",
		Children: []ReasonNode{
			{
				Message:    "Insufficient capacity",
				Confidence: "probable",
				Evidence:   "All nodes have 0 available GPU slots",
				SourceType: "Node",
				FieldPath:  "status.capacity",
			},
		},
	}

	if root.Message != "Claim pending" {
		t.Errorf("expected root message, got %q", root.Message)
	}
	if root.Confidence != "confirmed" {
		t.Errorf("expected confirmed, got %q", root.Confidence)
	}
	if root.Evidence != "No matching node found" {
		t.Errorf("expected root evidence, got %q", root.Evidence)
	}
	if root.SourceType != "Claim" {
		t.Errorf("expected source type Claim, got %q", root.SourceType)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	if root.Children[0].Confidence != "probable" {
		t.Errorf("expected probable, got %q", root.Children[0].Confidence)
	}
}

func TestExplainResultAllocated(t *testing.T) {
	r := ExplainResult{
		TargetName: "my-claim",
		TargetType: "claim",
		Allocated:  true,
		ReasonTree: ReasonNode{Message: "Allocated successfully", Confidence: "confirmed"},
		Remedy:     nil,
	}

	if !r.Allocated {
		t.Error("expected Allocated to be true")
	}
	if r.TargetName != "my-claim" {
		t.Errorf("TargetName: got %q, want my-claim", r.TargetName)
	}
	if r.TargetType != "claim" {
		t.Errorf("TargetType: got %q, want claim", r.TargetType)
	}
	if r.Remedy != nil {
		t.Errorf("expected nil Remedy, got %#v", r.Remedy)
	}
}

func TestDoctorCheckStatusConstants(t *testing.T) {
	tests := []struct {
		status DoctorCheckStatus
		want   string
	}{
		{StatusPass, "PASS"},
		{StatusWarn, "WARN"},
		{StatusFail, "FAIL"},
		{StatusSkip, "SKIP"},
		{StatusUnknown, "UNKNOWN"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("DoctorCheckStatus(%s): got %q, want %q", tt.want, string(tt.status), tt.want)
		}
	}
}

func TestDoctorCheckResultSeverity(t *testing.T) {
	r := DoctorCheckResult{
		ID:          "D001",
		Name:        "API Server Reachable",
		Category:    "cluster",
		Status:      StatusPass,
		Severity:    "critical",
		Evidence:    "API responded in 50ms",
		Remediation: "",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got DoctorCheckResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.ID != "D001" {
		t.Errorf("ID: got %q, want D001", got.ID)
	}
	if got.Status != StatusPass {
		t.Errorf("Status: got %q, want PASS", got.Status)
	}
	if got.Severity != "critical" {
		t.Errorf("Severity: got %q, want critical", got.Severity)
	}
}

func TestDoctorReportSummary(t *testing.T) {
	r := DoctorReport{
		Timestamp: time.Now(),
		Summary:   map[string]int{"PASS": 5, "WARN": 1, "FAIL": 0},
		Results: []DoctorCheckResult{
			{ID: "D001", Status: StatusPass},
			{ID: "D002", Status: StatusPass},
			{ID: "D003", Status: StatusWarn},
		},
	}

	if r.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
	if r.Summary["PASS"] != 5 {
		t.Errorf("Summary[PASS]: got %d, want 5", r.Summary["PASS"])
	}
	if len(r.Results) != 3 {
		t.Errorf("expected 3 results, got %d", len(r.Results))
	}
}

func TestDoctorReportJSONRoundTrip(t *testing.T) {
	r := DoctorReport{
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Summary:   map[string]int{"PASS": 2, "FAIL": 1},
		Results: []DoctorCheckResult{
			{ID: "D001", Status: StatusPass, Severity: "high"},
			{ID: "D002", Status: StatusFail, Severity: "critical"},
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got DoctorReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !got.Timestamp.Equal(r.Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", got.Timestamp, r.Timestamp)
	}
	if got.Summary["PASS"] != r.Summary["PASS"] {
		t.Errorf("Summary[PASS]: got %d, want %d", got.Summary["PASS"], r.Summary["PASS"])
	}
	if len(got.Results) != len(r.Results) {
		t.Errorf("Results count: got %d, want %d", len(got.Results), len(r.Results))
	}
}

func TestDeviceStatusValues(t *testing.T) {
	d := Device{Status: "healthy"}
	if d.Status != "healthy" {
		t.Errorf("expected healthy, got %q", d.Status)
	}
	d.Status = "unhealthy"
	if d.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", d.Status)
	}
	d.Status = "allocated"
	if d.Status != "allocated" {
		t.Errorf("expected allocated, got %q", d.Status)
	}
	d.Status = "pending"
	if d.Status != "pending" {
		t.Errorf("expected pending, got %q", d.Status)
	}
}
