// Package model defines the unified data structures for DRAForge.
// SPDX-License-Identifier: Apache-2.0
package model

import (
	"time"
)

// Device represents a physical or synthetic device.
type Device struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`        // gpu, camera, fpga, nic, etc.
	Status      string            `json:"status"`      // healthy, unhealthy, allocated, pending
	NodeName    string            `json:"nodeName"`    // Node where the device resides
	PoolName    string            `json:"poolName"`    // Pool the device belongs to
	Attributes  map[string]string `json:"attributes"`  // Model, vendor, driver-version, etc.
	Capacities  map[string]int64  `json:"capacities"`  // Custom consumable capacities
	IsSynthetic bool              `json:"isSynthetic"` // True if created by DRAForge simulator
	LastUpdated time.Time         `json:"lastUpdated"`
}

// DevicePool represents a pool of resource devices.
type DevicePool struct {
	Name        string            `json:"name"`
	DriverName  string            `json:"driverName"`
	NodeName    string            `json:"nodeName"`
	DeviceCount int               `json:"deviceCount"`
	DeviceType  string            `json:"deviceType"`
	Health      string            `json:"health"` // healthy, degraded, offline
	IsSynthetic bool              `json:"isSynthetic"`
	Labels      map[string]string `json:"labels"`
}

// ResourceClaimInfo represents the state of a ResourceClaim.
type ResourceClaimInfo struct {
	Name            string    `json:"name"`
	Namespace       string    `json:"namespace"`
	DeviceClassName string    `json:"deviceClassName"`
	Status          string    `json:"status"`       // pending, allocated, deallocating
	OwnerPodName    string    `json:"ownerPodName"` // Pod using the claim
	AllocatedDevice string    `json:"allocatedDevice,omitempty"`
	AllocatedNode   string    `json:"allocatedNode,omitempty"`
	AllocatedDriver string    `json:"allocatedDriver,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

// GraphNode represents a vertex in the resource graph.
type GraphNode struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // Pod, Claim, Device, Pool, Node, etc.
	Label    string                 `json:"label"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// GraphEdge represents a directed relationship between vertices.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // allocates, runs-on, binds-to, etc.
}

// ResourceGraph is the complete live relationship graph.
type ResourceGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// ReasonNode represents a node in the explain reason tree.
type ReasonNode struct {
	Message    string       `json:"message"`
	Confidence string       `json:"confidence"` // confirmed, probable, informational
	Evidence   string       `json:"evidence"`   // Source of finding (e.g. event, condition)
	SourceType string       `json:"sourceType"` // Pod, Claim, Node, etc.
	FieldPath  string       `json:"fieldPath,omitempty"`
	Children   []ReasonNode `json:"children,omitempty"`
}

// ExplainResult is the final explanation tree.
type ExplainResult struct {
	TargetName string     `json:"targetName"`
	TargetType string     `json:"targetType"` // claim, pod
	Allocated  bool       `json:"allocated"`
	ReasonTree ReasonNode `json:"reasonTree"`
	Remedy     []string   `json:"remedy"`
}

// DoctorCheckStatus represents diagnostic check outcomes.
type DoctorCheckStatus string

const (
	StatusPass    DoctorCheckStatus = "PASS"
	StatusWarn    DoctorCheckStatus = "WARN"
	StatusFail    DoctorCheckStatus = "FAIL"
	StatusSkip    DoctorCheckStatus = "SKIP"
	StatusUnknown DoctorCheckStatus = "UNKNOWN"
)

// DoctorCheckResult represents a single diagnostic check result.
type DoctorCheckResult struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Category     string            `json:"category"` // cluster, driver, claim
	Status       DoctorCheckStatus `json:"status"`
	Severity     string            `json:"severity"` // critical, high, medium, low, info
	Evidence     string            `json:"evidence"`
	Remediation  string            `json:"remediation"`
	DocReference string            `json:"docReference"`
}

// DoctorReport contains all diagnostic check results.
type DoctorReport struct {
	Timestamp time.Time           `json:"timestamp"`
	Summary   map[string]int      `json:"summary"` // PASS: X, WARN: Y, FAIL: Z, etc.
	Results   []DoctorCheckResult `json:"results"`
}
