// Package doctor performs diagnostics on the cluster's DRA configuration, drivers, claims, and pods.
// SPDX-License-Identifier: Apache-2.0
package doctor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/pkg/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Check is the interface that all diagnostic checks must implement.
type Check interface {
	ID() string
	Name() string
	Category() string
	Run(ctx context.Context, clientset kubernetes.Interface) model.DoctorCheckResult
}

// Registry manages the set of diagnostic checks.
type Registry struct {
	checks []Check
}

// NewRegistry instantiates a Registry with all default checks.
func NewRegistry() *Registry {
	r := &Registry{}
	r.Register(&APIAvailabilityCheck{})
	r.Register(&KubernetesVersionCheck{})
	r.Register(&OrphanedClaimsCheck{})
	r.Register(&PodReferenceCheck{})
	r.Register(&StaleResourceSliceCheck{})
	return r
}

// Register adds a check to the registry.
func (r *Registry) Register(c Check) {
	r.checks = append(r.checks, c)
}

// RunDiagnostics executes all registered checks and returns a summary report.
func (r *Registry) RunDiagnostics(ctx context.Context, clientset kubernetes.Interface) model.DoctorReport {
	report := model.DoctorReport{
		Timestamp: time.Now(),
		Summary:   make(map[string]int),
		Results:   []model.DoctorCheckResult{},
	}

	for _, check := range r.checks {
		res := check.Run(ctx, clientset)
		report.Results = append(report.Results, res)
		report.Summary[string(res.Status)]++
	}

	return report
}

// --- Check Implementations ---

// APIAvailabilityCheck verifies resource.k8s.io/v1 exists.
type APIAvailabilityCheck struct{}

func (c *APIAvailabilityCheck) ID() string       { return "DRA-001" }
func (c *APIAvailabilityCheck) Name() string     { return "DRA API Availability" }
func (c *APIAvailabilityCheck) Category() string { return "cluster" }
func (c *APIAvailabilityCheck) Run(ctx context.Context, clientset kubernetes.Interface) model.DoctorCheckResult {
	res := model.DoctorCheckResult{
		ID:           c.ID(),
		Name:         c.Name(),
		Category:     c.Category(),
		Status:       model.StatusPass,
		Severity:     "critical",
		Evidence:     "resource.k8s.io/v1 or v1beta1 API group is active.",
		Remediation:  "No action required.",
		DocReference: "https://kubernetes.io/docs/concepts/extend-kubernetes/compute-resource-sharing/",
	}

	groups, err := clientset.Discovery().ServerGroups()
	if err != nil {
		res.Status = model.StatusFail
		res.Evidence = fmt.Sprintf("Failed to query API server groups: %v", err)
		res.Remediation = "Verify cluster connectivity and API server health."
		return res
	}

	found := false
	for _, g := range groups.Groups {
		if g.Name == "resource.k8s.io" {
			for _, v := range g.Versions {
				if v.Version == "v1beta1" || v.Version == "v1" {
					found = true
					break
				}
			}
		}
	}

	if !found {
		res.Status = model.StatusFail
		res.Evidence = "resource.k8s.io/v1 API group is missing from the API server."
		res.Remediation = "Enable or upgrade to a Kubernetes version that serves resource.k8s.io/v1 DRA APIs (v1.34+)."
	}

	return res
}

// KubernetesVersionCheck verifies the cluster version is compatible with redesigned DRA.
type KubernetesVersionCheck struct{}

func (c *KubernetesVersionCheck) ID() string       { return "DRA-002" }
func (c *KubernetesVersionCheck) Name() string     { return "Kubernetes Version Compatibility" }
func (c *KubernetesVersionCheck) Category() string { return "cluster" }
func (c *KubernetesVersionCheck) Run(ctx context.Context, clientset kubernetes.Interface) model.DoctorCheckResult {
	res := model.DoctorCheckResult{
		ID:           c.ID(),
		Name:         c.Name(),
		Category:     c.Category(),
		Status:       model.StatusPass,
		Severity:     "high",
		Evidence:     "Kubernetes version is compatible.",
		Remediation:  "No action required.",
		DocReference: "https://kubernetes.io/docs/concepts/extend-kubernetes/compute-resource-sharing/",
	}

	info, err := clientset.Discovery().ServerVersion()
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to fetch cluster version: %v", err)
		res.Remediation = "Verify cluster connectivity."
		return res
	}

	res.Evidence = fmt.Sprintf("Detected Kubernetes version %s.", info.GitVersion)

	majorRaw := info.Major
	minorRaw := strings.TrimSuffix(info.Minor, "+")

	major, err := strconv.Atoi(majorRaw)
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to parse Kubernetes major version %q: %v", majorRaw, err)
		return res
	}
	minor, err := strconv.Atoi(minorRaw)
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to parse Kubernetes minor version %q: %v", minorRaw, err)
		return res
	}

	if major < 1 || (major == 1 && minor < 32) {
		res.Status = model.StatusFail
		res.Evidence = fmt.Sprintf("Kubernetes version %s.%s is older than v1.32. DRAForge requires DRA beta or GA.", majorRaw, minorRaw)
		res.Remediation = "Upgrade to Kubernetes v1.34+ which ships resource.k8s.io/v1 DRA API by default."
	} else if major == 1 && minor < 34 {
		res.Status = model.StatusWarn
		res.Evidence = fmt.Sprintf("Kubernetes version %s.%s runs DRA in beta (v1.32–v1.33). The v1 API is available but consider upgrading to v1.34+ for GA stability.", majorRaw, minorRaw)
		res.Remediation = "Upgrade to Kubernetes v1.34+ for GA DRA support."
	}

	return res
}

// OrphanedClaimsCheck checks if there are claims without a pod using them.
type OrphanedClaimsCheck struct{}

func (c *OrphanedClaimsCheck) ID() string       { return "DRA-003" }
func (c *OrphanedClaimsCheck) Name() string     { return "Orphaned ResourceClaims" }
func (c *OrphanedClaimsCheck) Category() string { return "claim" }
func (c *OrphanedClaimsCheck) Run(ctx context.Context, clientset kubernetes.Interface) model.DoctorCheckResult {
	res := model.DoctorCheckResult{
		ID:           c.ID(),
		Name:         c.Name(),
		Category:     c.Category(),
		Status:       model.StatusPass,
		Severity:     "medium",
		Evidence:     "No orphaned ResourceClaims found.",
		Remediation:  "No action required.",
		DocReference: "https://kubernetes.io/docs/concepts/extend-kubernetes/compute-resource-sharing/#resourceclaim",
	}

	_, _, claims, err := discovery.DiscoverDRA(ctx, clientset)
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to list claims: %v", err)
		return res
	}

	orphans := []string{}
	for _, cl := range claims {
		if cl.OwnerPodName == "" {
			orphans = append(orphans, cl.Namespace+"/"+cl.Name)
		}
	}

	if len(orphans) > 0 {
		res.Status = model.StatusWarn
		res.Evidence = fmt.Sprintf("Found %d ResourceClaims without consuming pods: %s", len(orphans), strings.Join(orphans, ", "))
		res.Remediation = "Delete unused ResourceClaims to free up cluster/node capacity."
	}

	return res
}

// PodReferenceCheck checks if pods reference missing or invalid ResourceClaims.
type PodReferenceCheck struct{}

func (c *PodReferenceCheck) ID() string       { return "DRA-004" }
func (c *PodReferenceCheck) Name() string     { return "Pod Claim References" }
func (c *PodReferenceCheck) Category() string { return "claim" }
func (c *PodReferenceCheck) Run(ctx context.Context, clientset kubernetes.Interface) model.DoctorCheckResult {
	res := model.DoctorCheckResult{
		ID:           c.ID(),
		Name:         c.Name(),
		Category:     c.Category(),
		Status:       model.StatusPass,
		Severity:     "high",
		Evidence:     "All pod claim references are valid.",
		Remediation:  "No action required.",
		DocReference: "https://kubernetes.io/docs/concepts/extend-kubernetes/compute-resource-sharing/",
	}

	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to list pods: %v", err)
		return res
	}

	apiClaims, err := clientset.ResourceV1().ResourceClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to list API claims for reference check: %v", err)
		return res
	}

	claimReservedForMap := make(map[string][]string)
	for _, cl := range apiClaims.Items {
		var reservedFor []string
		for _, ref := range cl.Status.ReservedFor {
			reservedFor = append(reservedFor, ref.Name)
		}
		claimReservedForMap[cl.Namespace+"/"+cl.Name] = reservedFor
	}

	invalidRefs := []string{}
	for _, pod := range podList.Items {
		for _, pc := range pod.Spec.ResourceClaims {
			if pc.ResourceClaimName != nil {
				target := pod.Namespace + "/" + *pc.ResourceClaimName
				reservedFor, exists := claimReservedForMap[target]
				if !exists {
					invalidRefs = append(invalidRefs, pod.Namespace+"/"+pod.Name+" -> "+*pc.ResourceClaimName+" (claim not found)")
				} else {
					podFound := false
					for _, name := range reservedFor {
						if name == pod.Name {
							podFound = true
							break
						}
					}
					if !podFound {
						invalidRefs = append(invalidRefs, pod.Namespace+"/"+pod.Name+" -> "+*pc.ResourceClaimName+" (pod not in claim ReservedFor list)")
					}
				}
			}
		}
	}

	if len(invalidRefs) > 0 {
		res.Status = model.StatusFail
		res.Evidence = fmt.Sprintf("Found pods referencing non-existent ResourceClaims: %s", strings.Join(invalidRefs, ", "))
		res.Remediation = "Ensure referenced ResourceClaims are created before deploying consuming pods."
	}

	return res
}

// StaleResourceSliceCheck checks if any ResourceSlice objects are stale, unavailable, or mismatched.
type StaleResourceSliceCheck struct{}

func (c *StaleResourceSliceCheck) ID() string       { return "DRA-005" }
func (c *StaleResourceSliceCheck) Name() string     { return "ResourceSlice Consistency" }
func (c *StaleResourceSliceCheck) Category() string { return "driver" }
func (c *StaleResourceSliceCheck) Run(ctx context.Context, clientset kubernetes.Interface) model.DoctorCheckResult {
	res := model.DoctorCheckResult{
		ID:           c.ID(),
		Name:         c.Name(),
		Category:     c.Category(),
		Status:       model.StatusPass,
		Severity:     "medium",
		Evidence:     "All ResourceSlices are consistent and active.",
		Remediation:  "No action required.",
		DocReference: "https://kubernetes.io/docs/concepts/extend-kubernetes/compute-resource-sharing/#resourceslice",
	}

	slices, err := clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to list ResourceSlices: %v", err)
		return res
	}

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	nodeMap := make(map[string]bool)
	nodeReadyMap := make(map[string]bool)
	if err == nil {
		for _, n := range nodes.Items {
			nodeMap[n.Name] = true
			isReady := false
			for _, cond := range n.Status.Conditions {
				if cond.Type == "Ready" && cond.Status == "True" {
					isReady = true
					break
				}
			}
			nodeReadyMap[n.Name] = isReady
		}
	}

	inconsistentSlices := []string{}
	staleSlices := []string{}
	unavailableSlices := []string{}

	for _, sl := range slices.Items {
		// Inconsistent: empty driver name
		if sl.Spec.Driver == "" {
			inconsistentSlices = append(inconsistentSlices, sl.Name+" (empty driver)")
		}

		// Stale / Mismatched node reference
		if sl.Spec.NodeName != nil {
			nodeName := *sl.Spec.NodeName
			if len(nodeMap) > 0 { // listed nodes successfully
				if !nodeMap[nodeName] {
					staleSlices = append(staleSlices, fmt.Sprintf("%s (references non-existent node %q)", sl.Name, nodeName))
				} else if !nodeReadyMap[nodeName] {
					unavailableSlices = append(unavailableSlices, fmt.Sprintf("%s (node %q is not Ready)", sl.Name, nodeName))
				}
			}
		}

		// Custom health label check (e.g. draforge.oaslananka/health != healthy)
		if healthVal, exists := sl.Labels["draforge.oaslananka/health"]; exists && healthVal != "healthy" && healthVal != "" {
			unavailableSlices = append(unavailableSlices, fmt.Sprintf("%s (health is %q)", sl.Name, healthVal))
		}
	}

	var evidence []string
	var remediation []string
	status := model.StatusPass

	if len(inconsistentSlices) > 0 {
		status = model.StatusFail
		evidence = append(evidence, fmt.Sprintf("Found inconsistent ResourceSlices: %s", strings.Join(inconsistentSlices, ", ")))
		remediation = append(remediation, "Check driver status and ensure the controller publishes valid ResourceSlices with non-empty driver names.")
	}

	if len(staleSlices) > 0 {
		status = model.StatusFail
		evidence = append(evidence, fmt.Sprintf("Found stale ResourceSlices referencing non-existent nodes: %s", strings.Join(staleSlices, ", ")))
		remediation = append(remediation, "Delete the orphaned/stale ResourceSlices or ensure their respective nodes are properly re-joined to the cluster.")
	}

	if len(unavailableSlices) > 0 {
		if status != model.StatusFail {
			status = model.StatusWarn
		}
		evidence = append(evidence, fmt.Sprintf("Found unavailable ResourceSlices: %s", strings.Join(unavailableSlices, ", ")))
		remediation = append(remediation, "Check physical/simulated device status, verify worker node health, and restart the driver daemonset if necessary.")
	}

	if len(evidence) > 0 {
		res.Status = status
		res.Evidence = strings.Join(evidence, "; ")
		res.Remediation = strings.Join(remediation, " ")
	}

	return res
}
