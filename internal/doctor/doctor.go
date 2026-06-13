// Package doctor performs diagnostics on the cluster's DRA configuration, drivers, claims, and pods.
// SPDX-License-Identifier: Apache-2.0
package doctor

import (
	"context"
	"fmt"
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
	Run(ctx context.Context, clientset *kubernetes.Clientset) model.DoctorCheckResult
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
func (r *Registry) RunDiagnostics(ctx context.Context, clientset *kubernetes.Clientset) model.DoctorReport {
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

// APIAvailabilityCheck verifies resource.k8s.io/v1beta1 exists.
type APIAvailabilityCheck struct{}

func (c *APIAvailabilityCheck) ID() string       { return "DRA-001" }
func (c *APIAvailabilityCheck) Name() string     { return "DRA API Availability" }
func (c *APIAvailabilityCheck) Category() string { return "cluster" }
func (c *APIAvailabilityCheck) Run(ctx context.Context, clientset *kubernetes.Clientset) model.DoctorCheckResult {
	res := model.DoctorCheckResult{
		ID:           c.ID(),
		Name:         c.Name(),
		Category:     c.Category(),
		Status:       model.StatusPass,
		Severity:     "critical",
		Evidence:     "resource.k8s.io/v1beta1 API group is active.",
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
				if v.Version == "v1beta1" {
					found = true
					break
				}
			}
		}
	}

	if !found {
		res.Status = model.StatusFail
		res.Evidence = "resource.k8s.io/v1beta1 API group is missing from the API server."
		res.Remediation = "Upgrade cluster to Kubernetes v1.32+ or enable the DynamicResourceAllocation feature gate."
	}

	return res
}

// KubernetesVersionCheck verifies the cluster version is compatible with redesigned DRA.
type KubernetesVersionCheck struct{}

func (c *KubernetesVersionCheck) ID() string       { return "DRA-002" }
func (c *KubernetesVersionCheck) Name() string     { return "Kubernetes Version Compatibility" }
func (c *KubernetesVersionCheck) Category() string { return "cluster" }
func (c *KubernetesVersionCheck) Run(ctx context.Context, clientset *kubernetes.Clientset) model.DoctorCheckResult {
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

	// Clean version parsing
	major := info.Major
	minor := strings.TrimSuffix(info.Minor, "+")
	if major < "1" || (major == "1" && minor < "32") {
		res.Status = model.StatusWarn
		res.Evidence = fmt.Sprintf("Kubernetes version %s.%s is less than v1.32. DRA v1beta1 features may not be supported.", major, minor)
		res.Remediation = "Consider upgrading to Kubernetes 1.32 or newer."
	}

	return res
}

// OrphanedClaimsCheck checks if there are claims without a pod using them.
type OrphanedClaimsCheck struct{}

func (c *OrphanedClaimsCheck) ID() string       { return "DRA-003" }
func (c *OrphanedClaimsCheck) Name() string     { return "Orphaned ResourceClaims" }
func (c *OrphanedClaimsCheck) Category() string { return "claim" }
func (c *OrphanedClaimsCheck) Run(ctx context.Context, clientset *kubernetes.Clientset) model.DoctorCheckResult {
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
func (c *PodReferenceCheck) Run(ctx context.Context, clientset *kubernetes.Clientset) model.DoctorCheckResult {
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

	_, _, claims, err := discovery.DiscoverDRA(ctx, clientset)
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to discover claims: %v", err)
		return res
	}

	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to list pods: %v", err)
		return res
	}

	claimMap := make(map[string]bool)
	for _, cl := range claims {
		claimMap[cl.Namespace+"/"+cl.Name] = true
	}

	invalidRefs := []string{}
	for _, pod := range podList.Items {
		for _, pc := range pod.Spec.ResourceClaims {
			if pc.ResourceClaimName != nil {
				target := pod.Namespace + "/" + *pc.ResourceClaimName
				if !claimMap[target] {
					invalidRefs = append(invalidRefs, pod.Namespace+"/"+pod.Name+" -> "+*pc.ResourceClaimName)
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

// StaleResourceSliceCheck checks if any ResourceSlice objects are stale or mismatched.
type StaleResourceSliceCheck struct{}

func (c *StaleResourceSliceCheck) ID() string       { return "DRA-005" }
func (c *StaleResourceSliceCheck) Name() string     { return "ResourceSlice Consistency" }
func (c *StaleResourceSliceCheck) Category() string { return "driver" }
func (c *StaleResourceSliceCheck) Run(ctx context.Context, clientset *kubernetes.Clientset) model.DoctorCheckResult {
	res := model.DoctorCheckResult{
		ID:           c.ID(),
		Name:         c.Name(),
		Category:     c.Category(),
		Status:       model.StatusPass,
		Severity:     "medium",
		Evidence:     "All ResourceSlices are consistent.",
		Remediation:  "No action required.",
		DocReference: "https://kubernetes.io/docs/concepts/extend-kubernetes/compute-resource-sharing/#resourceslice",
	}

	slices, err := clientset.ResourceV1beta1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		res.Status = model.StatusUnknown
		res.Evidence = fmt.Sprintf("Failed to list ResourceSlices: %v", err)
		return res
	}

	inconsistentSlices := []string{}
	for _, sl := range slices.Items {
		// A slice must either have devices or specify a non-empty driver name
		if sl.Spec.Driver == "" {
			inconsistentSlices = append(inconsistentSlices, sl.Name+" (empty driver)")
		}
	}

	if len(inconsistentSlices) > 0 {
		res.Status = model.StatusFail
		res.Evidence = fmt.Sprintf("Found inconsistent ResourceSlices: %s", strings.Join(inconsistentSlices, ", "))
		res.Remediation = "Check driver status and ensure the controller publishes valid ResourceSlices."
	}

	return res
}
