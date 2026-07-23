// Package graph constructs and formats the DRA resource relationship graph.
// SPDX-License-Identifier: Apache-2.0
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"unicode"

	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/pkg/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const classNodePrefix = "class/"

// GraphBuilder manages construction and formatting of the resource graph.
// It uses internal dedup maps to guarantee deterministic, O(1) deduplicated output.
type GraphBuilder struct {
	nodes []model.GraphNode
	edges []model.GraphEdge

	nodeIDs map[string]struct{}
	edgeIDs map[string]struct{}
}

// NewGraphBuilder instantiates a GraphBuilder.
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{}
}

// edgeKey returns a deterministic key for deduplicating edges.
// The \x00 separator ensures from/to/type cannot produce collisions.
func edgeKey(from, to, edgeType string) string {
	return from + "\x00" + to + "\x00" + edgeType
}

func graphID(kind string, parts ...string) string {
	encoded := make([]string, 0, len(parts)+1)
	encoded = append(encoded, kind)
	for _, part := range parts {
		encoded = append(encoded, fmt.Sprintf("%d:%s", len(part), part))
	}
	return strings.Join(encoded, "/")
}

func poolGraphID(driverName, nodeName, poolName string) string {
	return graphID("pool", driverName, nodeName, poolName)
}

func deviceGraphID(driverName, nodeName, poolName, deviceName string) string {
	return graphID("device", driverName, nodeName, poolName, deviceName)
}

func missingDeviceGraphID(driverName, nodeName, poolName, deviceName string) string {
	return graphID("device", "missing", driverName, nodeName, poolName, deviceName)
}

func allocationGraphID(namespace, claimName string, index int, allocation model.ClaimAllocation) string {
	return graphID("allocation", namespace, claimName, fmt.Sprintf("%d", index), allocation.Request, allocation.DriverName, allocation.NodeName, allocation.PoolName, allocation.DeviceName)
}

func mermaidID(raw string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(raw))

	var sb strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
		}
	}
	return fmt.Sprintf("n%x_%s", h.Sum32(), sb.String())
}

// BuildGraph queries the cluster and builds nodes and edges representing the live relationships.
func (gb *GraphBuilder) BuildGraph(ctx context.Context, clientset kubernetes.Interface, namespaceFilter, driverFilter string) (*model.ResourceGraph, error) {
	gb.reset()

	pools, devices, claims, err := discovery.DiscoverDRA(ctx, clientset)
	if err != nil {
		return nil, err
	}
	classSet := discoverClassSet(ctx, clientset)

	gb.addNamespaces(claims, namespaceFilter)
	gb.addPools(pools, driverFilter)
	gb.addDevices(devices, driverFilter)
	gb.addClasses(classSet)
	gb.addClaims(claims, devices, classSet, namespaceFilter, driverFilter)
	gb.sort()

	return &model.ResourceGraph{Nodes: gb.nodes, Edges: gb.edges}, nil
}

func (gb *GraphBuilder) reset() {
	gb.nodes = []model.GraphNode{}
	gb.edges = []model.GraphEdge{}
	gb.nodeIDs = make(map[string]struct{})
	gb.edgeIDs = make(map[string]struct{})
}

func discoverClassSet(ctx context.Context, clientset kubernetes.Interface) map[string]struct{} {
	classSet := make(map[string]struct{})
	classes, err := clientset.ResourceV1().DeviceClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return classSet
	}
	for _, deviceClass := range classes.Items {
		classSet[deviceClass.Name] = struct{}{}
	}
	return classSet
}

func (gb *GraphBuilder) addNamespaces(claims []model.ResourceClaimInfo, namespaceFilter string) {
	namespaces := make(map[string]struct{})
	for _, claim := range claims {
		if !matchesNamespace(claim, namespaceFilter) {
			continue
		}
		namespaces[claim.Namespace] = struct{}{}
	}
	for namespace := range namespaces {
		gb.addNode("ns/"+namespace, "Namespace", namespace, nil)
	}
}

func (gb *GraphBuilder) addPools(pools []model.DevicePool, driverFilter string) {
	drivers := make(map[string]struct{})
	for _, pool := range pools {
		if !matchesDriver(pool.DriverName, driverFilter) {
			continue
		}
		drivers[pool.DriverName] = struct{}{}
		poolID := poolGraphID(pool.DriverName, pool.NodeName, pool.Name)
		gb.addNode(poolID, "ResourcePool", pool.Name, map[string]interface{}{
			"driver":    pool.DriverName,
			"node":      pool.NodeName,
			"synthetic": pool.IsSynthetic,
			"health":    pool.Health,
		})
		if pool.NodeName != "" {
			gb.addNode("node/"+pool.NodeName, "Node", pool.NodeName, nil)
			gb.addEdge(poolID, "node/"+pool.NodeName, "located-on")
		}
		gb.addEdge(poolID, "driver/"+pool.DriverName, "managed-by")
	}
	for driver := range drivers {
		gb.addNode("driver/"+driver, "Driver", driver, nil)
	}
}

func (gb *GraphBuilder) addDevices(devices []model.Device, driverFilter string) {
	for _, device := range devices {
		if !matchesDriver(device.DriverName, driverFilter) {
			continue
		}
		deviceID := deviceGraphID(device.DriverName, device.NodeName, device.PoolName, device.Name)
		gb.addNode(deviceID, "Device", device.Name, map[string]interface{}{
			"driver":    device.DriverName,
			"pool":      device.PoolName,
			"type":      device.Type,
			"node":      device.NodeName,
			"synthetic": device.IsSynthetic,
			"status":    device.Status,
		})
		if device.PoolName != "" {
			gb.addEdge(deviceID, poolGraphID(device.DriverName, device.NodeName, device.PoolName), "part-of-pool")
		}
		if device.NodeName != "" {
			gb.addNode("node/"+device.NodeName, "Node", device.NodeName, nil)
			gb.addEdge(deviceID, "node/"+device.NodeName, "located-on")
		}
	}
}

func (gb *GraphBuilder) addClasses(classSet map[string]struct{}) {
	for className := range classSet {
		gb.addNode(classNodePrefix+className, "DeviceClass", className, map[string]interface{}{"status": "available"})
	}
}

func (gb *GraphBuilder) addClaims(claims []model.ResourceClaimInfo, devices []model.Device, classSet map[string]struct{}, namespaceFilter, driverFilter string) {
	for _, claim := range claims {
		if !matchesNamespace(claim, namespaceFilter) {
			continue
		}
		gb.addClaim(claim, devices, classSet, driverFilter)
	}
}

func (gb *GraphBuilder) addClaim(claim model.ResourceClaimInfo, devices []model.Device, classSet map[string]struct{}, driverFilter string) {
	claimID := "claim/" + claim.Namespace + "/" + claim.Name
	classNames := claim.RequestedClassNames()
	gb.addNode(claimID, "ResourceClaim", claim.Name, map[string]interface{}{
		"namespace":       claim.Namespace,
		"status":          claim.Status,
		"classes":         classNames,
		"requestCount":    len(claim.Requests),
		"allocationCount": len(claim.Allocations),
		"requests":        claim.Requests,
		"allocations":     claim.Allocations,
	})
	gb.addEdge(claimID, "ns/"+claim.Namespace, "belongs-to")
	gb.addClaimClasses(claimID, classNames, classSet)
	gb.addClaimAllocations(claimID, claim, devices, driverFilter)
	gb.addClaimOwner(claimID, claim)
}

func (gb *GraphBuilder) addClaimClasses(claimID string, classNames []string, classSet map[string]struct{}) {
	for _, className := range classNames {
		if _, exists := classSet[className]; !exists {
			gb.addNode(classNodePrefix+className, "DeviceClass", className+" (MISSING)", map[string]interface{}{"status": "missing"})
		}
		gb.addEdge(claimID, classNodePrefix+className, "uses-class")
	}
}

func (gb *GraphBuilder) addClaimAllocations(claimID string, claim model.ResourceClaimInfo, devices []model.Device, driverFilter string) {
	for index, allocation := range claim.EffectiveAllocations() {
		if !matchesDriver(allocation.DriverName, driverFilter) {
			continue
		}
		gb.addClaimAllocation(claimID, claim, index, allocation, devices)
	}
}

func (gb *GraphBuilder) addClaimAllocation(claimID string, claim model.ResourceClaimInfo, index int, allocation model.ClaimAllocation, devices []model.Device) {
	allocationID := allocationGraphID(claim.Namespace, claim.Name, index, allocation)
	label := allocation.Request
	if label == "" {
		label = allocation.DeviceName
	}
	gb.addNode(allocationID, "Allocation", label, map[string]interface{}{
		"request": allocation.Request,
		"driver":  allocation.DriverName,
		"pool":    allocation.PoolName,
		"device":  allocation.DeviceName,
		"node":    allocation.NodeName,
	})
	gb.addEdge(claimID, allocationID, "has-allocation")

	deviceID, found := allocatedDeviceID(devices, allocation)
	if found {
		gb.addEdge(allocationID, deviceID, "resolves-to")
		gb.addEdge(claimID, deviceID, "allocates")
		return
	}
	gb.addMissingAllocation(claimID, allocationID, allocation)
}

func (gb *GraphBuilder) addMissingAllocation(claimID, allocationID string, allocation model.ClaimAllocation) {
	missingID := missingDeviceGraphID(allocation.DriverName, allocation.NodeName, allocation.PoolName, allocation.DeviceName)
	gb.addNode(missingID, "Device", allocation.DeviceName+" (MISSING)", map[string]interface{}{
		"status": "missing",
		"driver": allocation.DriverName,
		"pool":   allocation.PoolName,
		"node":   allocation.NodeName,
	})
	gb.addEdge(allocationID, missingID, "resolves-to-missing")
	gb.addEdge(claimID, missingID, "allocates-missing")
}

func (gb *GraphBuilder) addClaimOwner(claimID string, claim model.ResourceClaimInfo) {
	if claim.OwnerPodName == "" {
		return
	}
	podID := "pod/" + claim.Namespace + "/" + claim.OwnerPodName
	gb.addNode(podID, "Pod", claim.OwnerPodName, map[string]interface{}{"namespace": claim.Namespace})
	gb.addEdge(podID, claimID, "claims")
	gb.addEdge(podID, "ns/"+claim.Namespace, "belongs-to")
	for _, nodeName := range claimAllocationNodes(claim) {
		gb.addNode("node/"+nodeName, "Node", nodeName, nil)
		gb.addEdge(podID, "node/"+nodeName, "runs-on")
	}
}

func matchesNamespace(claim model.ResourceClaimInfo, namespaceFilter string) bool {
	return namespaceFilter == "" || claim.Namespace == namespaceFilter
}

func matchesDriver(driverName, driverFilter string) bool {
	return driverFilter == "" || driverName == driverFilter
}

func (gb *GraphBuilder) sort() {
	sort.Slice(gb.nodes, func(i, j int) bool { return gb.nodes[i].ID < gb.nodes[j].ID })
	sort.Slice(gb.edges, func(i, j int) bool {
		if gb.edges[i].From != gb.edges[j].From {
			return gb.edges[i].From < gb.edges[j].From
		}
		if gb.edges[i].To != gb.edges[j].To {
			return gb.edges[i].To < gb.edges[j].To
		}
		return gb.edges[i].Type < gb.edges[j].Type
	})
}

func allocatedDeviceID(devices []model.Device, allocation model.ClaimAllocation) (string, bool) {
	matches := make([]model.Device, 0, 1)
	for _, device := range devices {
		if device.Name != allocation.DeviceName || device.DriverName != allocation.DriverName {
			continue
		}
		if allocation.PoolName != "" && device.PoolName != allocation.PoolName {
			continue
		}
		if allocation.NodeName != "" && device.NodeName != allocation.NodeName {
			continue
		}
		matches = append(matches, device)
	}
	if len(matches) != 1 {
		return "", false
	}
	device := matches[0]
	return deviceGraphID(device.DriverName, device.NodeName, device.PoolName, device.Name), true
}

func claimAllocationNodes(claim model.ResourceClaimInfo) []string {
	seen := make(map[string]struct{})
	nodes := make([]string, 0)
	for _, allocation := range claim.EffectiveAllocations() {
		if allocation.NodeName == "" {
			continue
		}
		if _, exists := seen[allocation.NodeName]; exists {
			continue
		}
		seen[allocation.NodeName] = struct{}{}
		nodes = append(nodes, allocation.NodeName)
	}
	sort.Strings(nodes)
	return nodes
}

// addNode adds a node if it does not already exist (O(1) dedup via map).
func (gb *GraphBuilder) addNode(id, nodeType, label string, meta map[string]interface{}) {
	if gb.nodeIDs == nil {
		gb.nodeIDs = make(map[string]struct{})
	}
	if _, exists := gb.nodeIDs[id]; exists {
		return
	}
	gb.nodeIDs[id] = struct{}{}
	gb.nodes = append(gb.nodes, model.GraphNode{
		ID:       id,
		Type:     nodeType,
		Label:    label,
		Metadata: meta,
	})
}

// addEdge adds an edge if it does not already exist (O(1) dedup via map).
func (gb *GraphBuilder) addEdge(from, to, edgeType string) {
	if gb.edgeIDs == nil {
		gb.edgeIDs = make(map[string]struct{})
	}
	key := edgeKey(from, to, edgeType)
	if _, exists := gb.edgeIDs[key]; exists {
		return
	}
	gb.edgeIDs[key] = struct{}{}
	gb.edges = append(gb.edges, model.GraphEdge{
		From: from,
		To:   to,
		Type: edgeType,
	})
}

// ToJSON serializes the graph to JSON format.
func ToJSON(g *model.ResourceGraph) ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}

// ToDOT formats the graph in Graphviz DOT representation.
func ToDOT(g *model.ResourceGraph) string {
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=filled, color=lightblue];\n\n")

	// Print nodes
	for _, n := range g.Nodes {
		escapedLabel := strings.ReplaceAll(n.Label, "\"", "\\\"")
		color := "lightblue"
		switch n.Type {
		case "Pod":
			color = "lightgreen"
		case "ResourceClaim":
			color = "lightyellow"
		case "Device":
			color = "lightpink"
			if n.Metadata != nil && n.Metadata["status"] == "missing" {
				color = "red"
			}
		case "Allocation":
			color = "orange"
		case "Driver":
			color = "purple"
		}
		fmt.Fprintf(&sb, "  \"%s\" [label=\"%s\\n(%s)\", fillcolor=%s];\n", n.ID, escapedLabel, n.Type, color)
	}
	sb.WriteString("\n")

	// Print edges
	for _, e := range g.Edges {
		fmt.Fprintf(&sb, "  \"%s\" -> \"%s\" [label=\"%s\"];\n", e.From, e.To, e.Type)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// ToMermaid formats the graph in Mermaid flowchart representation.
func ToMermaid(g *model.ResourceGraph) string {
	var sb strings.Builder
	sb.WriteString("graph LR\n")

	// Mermaid nodes
	for _, n := range g.Nodes {
		cleanID := mermaidID(n.ID)
		escapedLabel := strings.ReplaceAll(n.Label, "\"", "")
		fmt.Fprintf(&sb, "  %s[\"%s<br/>(%s)\"]\n", cleanID, escapedLabel, n.Type)
	}
	sb.WriteString("\n")

	// Mermaid edges
	for _, e := range g.Edges {
		cleanFrom := mermaidID(e.From)
		cleanTo := mermaidID(e.To)
		fmt.Fprintf(&sb, "  %s -->|%s| %s\n", cleanFrom, e.Type, cleanTo)
	}

	return sb.String()
}
