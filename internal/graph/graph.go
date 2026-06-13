// Package graph constructs and formats the DRA resource relationship graph.
// SPDX-License-Identifier: Apache-2.0
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/pkg/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GraphBuilder manages construction and formatting of the resource graph.
type GraphBuilder struct {
	nodes []model.GraphNode
	edges []model.GraphEdge
}

// NewGraphBuilder instantiates a GraphBuilder.
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{
		nodes: []model.GraphNode{},
		edges: []model.GraphEdge{},
	}
}

// BuildGraph queries the cluster and builds nodes and edges representing the live relationships.
func (gb *GraphBuilder) BuildGraph(ctx context.Context, clientset kubernetes.Interface, namespaceFilter, driverFilter string) (*model.ResourceGraph, error) {
	gb.nodes = []model.GraphNode{}
	gb.edges = []model.GraphEdge{}

	// 1. Fetch live objects
	pools, devices, claims, err := discovery.DiscoverDRA(ctx, clientset)
	if err != nil {
		return nil, err
	}

	classes, err := clientset.ResourceV1beta1().DeviceClasses().List(ctx, metav1.ListOptions{})
	var classList []string
	if err == nil {
		for _, c := range classes.Items {
			classList = append(classList, c.Name)
		}
	}

	// 2. Add Namespace nodes (deduplicated)
	nsMap := make(map[string]bool)
	for _, c := range claims {
		if namespaceFilter != "" && c.Namespace != namespaceFilter {
			continue
		}
		nsMap[c.Namespace] = true
	}

	for ns := range nsMap {
		gb.addNode("ns/"+ns, "Namespace", ns, nil)
	}

	// 3. Add Driver and Pool nodes
	driverMap := make(map[string]bool)
	for _, p := range pools {
		if driverFilter != "" && p.DriverName != driverFilter {
			continue
		}
		driverMap[p.DriverName] = true

		poolID := "pool/" + p.Name
		gb.addNode(poolID, "ResourcePool", p.Name, map[string]interface{}{
			"driver":    p.DriverName,
			"node":      p.NodeName,
			"synthetic": p.IsSynthetic,
			"health":    p.Health,
		})

		// Edge: Pool -> Node (if node-local)
		if p.NodeName != "" {
			gb.addEdge(poolID, "node/"+p.NodeName, "located-on")
			gb.addNode("node/"+p.NodeName, "Node", p.NodeName, nil)
		}

		// Edge: Pool -> Driver
		gb.addEdge(poolID, "driver/"+p.DriverName, "managed-by")
	}

	for drv := range driverMap {
		gb.addNode("driver/"+drv, "Driver", drv, nil)
	}

	// 4. Add Device nodes
	for _, d := range devices {
		if driverFilter != "" && !strings.Contains(d.ID, driverFilter) {
			continue
		}

		devID := "device/" + d.ID
		gb.addNode(devID, "Device", d.Name, map[string]interface{}{
			"type":      d.Type,
			"node":      d.NodeName,
			"synthetic": d.IsSynthetic,
			"status":    d.Status,
		})

		// Edge: Device -> Pool
		if d.PoolName != "" {
			gb.addEdge(devID, "pool/"+d.PoolName, "part-of-pool")
		}

		// Edge: Device -> Node
		if d.NodeName != "" {
			gb.addEdge(devID, "node/"+d.NodeName, "located-on")
			gb.addNode("node/"+d.NodeName, "Node", d.NodeName, nil)
		}
	}

	// 5. Add DeviceClass nodes
	for _, cName := range classList {
		gb.addNode("class/"+cName, "DeviceClass", cName, nil)
	}

	// 6. Add Claim and Pod nodes
	for _, c := range claims {
		if namespaceFilter != "" && c.Namespace != namespaceFilter {
			continue
		}

		claimID := "claim/" + c.Namespace + "/" + c.Name
		gb.addNode(claimID, "ResourceClaim", c.Name, map[string]interface{}{
			"namespace": c.Namespace,
			"status":    c.Status,
			"class":     c.DeviceClassName,
		})

		// Edge: Claim -> Namespace
		gb.addEdge(claimID, "ns/"+c.Namespace, "belongs-to")

		// Edge: Claim -> DeviceClass
		if c.DeviceClassName != "" {
			gb.addEdge(claimID, "class/"+c.DeviceClassName, "uses-class")
		}

		// Edge: Claim -> Allocated Device
		if c.AllocatedDevice != "" {
			// Find exact matching device node ID
			foundDevice := false
			for _, dev := range devices {
				if dev.Name == c.AllocatedDevice && (c.AllocatedNode == "" || dev.NodeName == c.AllocatedNode) {
					gb.addEdge(claimID, "device/"+dev.ID, "allocates")
					foundDevice = true
					break
				}
			}
			// Missing edge check: if allocated device is missing, link to virtual ID
			if !foundDevice {
				missingDevID := "device/missing/" + c.AllocatedDevice
				gb.addNode(missingDevID, "Device", c.AllocatedDevice+" (MISSING)", map[string]interface{}{
					"status": "missing",
				})
				gb.addEdge(claimID, missingDevID, "allocates-missing")
			}
		}

		// Edge: Pod -> Claim
		if c.OwnerPodName != "" {
			podID := "pod/" + c.Namespace + "/" + c.OwnerPodName
			gb.addNode(podID, "Pod", c.OwnerPodName, map[string]interface{}{
				"namespace": c.Namespace,
			})
			gb.addEdge(podID, claimID, "claims")
			gb.addEdge(podID, "ns/"+c.Namespace, "belongs-to")

			// Edge: Pod -> Node (if allocated/running)
			if c.AllocatedNode != "" {
				gb.addEdge(podID, "node/"+c.AllocatedNode, "runs-on")
				gb.addNode("node/"+c.AllocatedNode, "Node", c.AllocatedNode, nil)
			}
		}
	}

	return &model.ResourceGraph{
		Nodes: gb.nodes,
		Edges: gb.edges,
	}, nil
}

func (gb *GraphBuilder) addNode(id, nodeType, label string, meta map[string]interface{}) {
	// Deduplicate nodes
	for _, n := range gb.nodes {
		if n.ID == id {
			return
		}
	}
	gb.nodes = append(gb.nodes, model.GraphNode{
		ID:       id,
		Type:     nodeType,
		Label:    label,
		Metadata: meta,
	})
}

func (gb *GraphBuilder) addEdge(from, to, edgeType string) {
	// Deduplicate edges
	for _, e := range gb.edges {
		if e.From == from && e.To == to && e.Type == edgeType {
			return
		}
	}
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
		case "Driver":
			color = "purple"
		}
		sb.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\\n(%s)\", fillcolor=%s];\n", n.ID, escapedLabel, n.Type, color))
	}
	sb.WriteString("\n")

	// Print edges
	for _, e := range g.Edges {
		sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [label=\"%s\"];\n", e.From, e.To, e.Type))
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
		cleanID := strings.ReplaceAll(n.ID, "/", "_")
		cleanID = strings.ReplaceAll(cleanID, "-", "_")
		cleanID = strings.ReplaceAll(cleanID, ".", "_")
		escapedLabel := strings.ReplaceAll(n.Label, "\"", "")
		sb.WriteString(fmt.Sprintf("  %s[\"%s<br/>(%s)\"]\n", cleanID, escapedLabel, n.Type))
	}
	sb.WriteString("\n")

	// Mermaid edges
	for _, e := range g.Edges {
		cleanFrom := strings.ReplaceAll(e.From, "/", "_")
		cleanFrom = strings.ReplaceAll(cleanFrom, "-", "_")
		cleanFrom = strings.ReplaceAll(cleanFrom, ".", "_")
		cleanTo := strings.ReplaceAll(e.To, "/", "_")
		cleanTo = strings.ReplaceAll(cleanTo, "-", "_")
		cleanTo = strings.ReplaceAll(cleanTo, ".", "_")
		sb.WriteString(fmt.Sprintf("  %s -->|%s| %s\n", cleanFrom, e.Type, cleanTo))
	}

	return sb.String()
}
