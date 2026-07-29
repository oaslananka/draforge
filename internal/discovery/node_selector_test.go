// Package discovery tests exact allocation node resolution.
// SPDX-License-Identifier: Apache-2.0
package discovery

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
)

func exactFieldNode(name string) corev1.NodeSelectorRequirement {
	return corev1.NodeSelectorRequirement{
		Key:      "metadata.name",
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{name},
	}
}

func exactHostnameNode(name string) corev1.NodeSelectorRequirement {
	return corev1.NodeSelectorRequirement{
		Key:      "kubernetes.io/hostname",
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{name},
	}
}

func TestAllocationNodeNameResolvesExactSelectors(t *testing.T) {
	for name, allocation := range map[string]*resourcev1.AllocationResult{
		"nil allocation": nil,
		"no selector":    {},
		"field selector": {
			NodeSelector: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchFields: []corev1.NodeSelectorRequirement{exactFieldNode("node-a")},
			}}},
		},
		"hostname expression": {
			NodeSelector: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{exactHostnameNode("node-b")},
			}}},
		},
		"matching field and expression": {
			NodeSelector: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchFields:      []corev1.NodeSelectorRequirement{exactFieldNode("node-c")},
				MatchExpressions: []corev1.NodeSelectorRequirement{exactHostnameNode("node-c")},
			}}},
		},
		"matching terms": {
			NodeSelector: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{MatchFields: []corev1.NodeSelectorRequirement{exactFieldNode("node-d")}},
				{MatchExpressions: []corev1.NodeSelectorRequirement{exactHostnameNode("node-d")}},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := allocationNodeName(allocation)
			want := ""
			switch name {
			case "field selector":
				want = "node-a"
			case "hostname expression":
				want = "node-b"
			case "matching field and expression":
				want = "node-c"
			case "matching terms":
				want = "node-d"
			}
			if got != want {
				t.Fatalf("allocationNodeName() = %q, want %q", got, want)
			}
		})
	}
}

func TestAllocationNodeNameRejectsAmbiguousSelectors(t *testing.T) {
	for name, selector := range map[string]*corev1.NodeSelector{
		"non exact requirement": {NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchFields: []corev1.NodeSelectorRequirement{{
				Key: "metadata.name", Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a", "node-b"},
			}},
		}}},
		"conflicting requirements": {NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchFields:      []corev1.NodeSelectorRequirement{exactFieldNode("node-a")},
			MatchExpressions: []corev1.NodeSelectorRequirement{exactHostnameNode("node-b")},
		}}},
		"conflicting terms": {NodeSelectorTerms: []corev1.NodeSelectorTerm{
			{MatchFields: []corev1.NodeSelectorRequirement{exactFieldNode("node-a")}},
			{MatchFields: []corev1.NodeSelectorRequirement{exactFieldNode("node-b")}},
		}},
		"unrelated selector": {NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key: "topology.kubernetes.io/zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"zone-a"},
			}},
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			allocation := &resourcev1.AllocationResult{NodeSelector: selector}
			if got := allocationNodeName(allocation); got != "" {
				t.Fatalf("allocationNodeName() = %q, want empty", got)
			}
		})
	}
}

func TestMergeExactNode(t *testing.T) {
	for _, test := range []struct {
		current string
		next    string
		want    string
	}{
		{next: "node-a", want: "node-a"},
		{current: "node-a", next: "node-a", want: "node-a"},
		{current: "node-a", next: "node-b", want: ""},
	} {
		if got := mergeExactNode(test.current, test.next); got != test.want {
			t.Fatalf("mergeExactNode(%q, %q) = %q, want %q", test.current, test.next, got, test.want)
		}
	}
}
