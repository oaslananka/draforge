// Package simulator implements the virtual device pool manager and reconciliation logic.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/oaslananka/draforge/internal/draeval"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

var (
	sdpGVR = schema.GroupVersionResource{
		Group:    "draforge.oaslananka",
		Version:  "v1alpha1",
		Resource: "simulateddevicepools",
	}
)

// Reconciler watches SimulatedDevicePools and creates/updates Kubernetes ResourceSlices.
type Reconciler struct {
	selectorEvaluator     *draeval.Evaluator
	eventRecorder         record.EventRecorder
	clientset             kubernetes.Interface
	dynamicClient         dynamic.Interface
	ReconcileErrorsCount  int64
	ReconcileRetriesCount int64
	TerminalErrorsCount   int64
	AllocationsSimulated  int64
	ActiveFaultsCount     int64
	leaderState           atomic.Bool
}

// NewReconciler creates a Reconciler instance.
func NewReconciler(clientset kubernetes.Interface, dyn dynamic.Interface) *Reconciler {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	var eventRecorder record.EventRecorder
	if clientset != nil {
		eventBroadcaster.StartRecordingToSink(&clientcorev1.EventSinkImpl{Interface: clientset.CoreV1().Events("")})
		eventRecorder = eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "draforge-controller"})
	} else {
		eventRecorder = record.NewFakeRecorder(100)
	}
	return &Reconciler{
		selectorEvaluator: draeval.NewEvaluator(),
		clientset:         clientset,
		dynamicClient:     dyn,
		eventRecorder:     eventRecorder,
	}
}

// SetLeader records whether this process currently owns the active controller lifecycle.
func (r *Reconciler) SetLeader(leader bool) {
	r.leaderState.Store(leader)
}

// IsLeader reports whether this process currently owns the active controller lifecycle.
func (r *Reconciler) IsLeader() bool {
	return r.leaderState.Load()
}

// Reconcile performs one sync iteration.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	// 1. List SimulatedDevicePools
	sdps, err := r.dynamicClient.Resource(sdpGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// If CRD is not registered yet, we skip reconciliation silently
		if strings.Contains(err.Error(), "could not find the scale subresource") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to list SimulatedDevicePools: %w", err)
	}

	// Track active slices we manage
	activeSliceNames := make(map[string]bool)
	localFaultCount := int64(0)

	for _, sdp := range sdps.Items {
		spec, found, _ := unstructured.NestedMap(sdp.Object, "spec")
		if !found {
			continue
		}

		driverName, _, _ := unstructured.NestedString(spec, "driverName")
		poolName, _, _ := unstructured.NestedString(spec, "poolName")
		if poolName == "" {
			poolName = sdp.GetName()
		}
		deviceType, _, _ := unstructured.NestedString(spec, "deviceType")
		deviceCount, _, _ := unstructured.NestedInt64(spec, "deviceCount")
		targetNodes, _, _ := unstructured.NestedStringSlice(spec, "targetNodes")
		health, _, _ := unstructured.NestedString(spec, "health")
		if health == "" {
			health = "healthy"
		}
		if health != "healthy" {
			localFaultCount++
		}

		// If no target nodes specified, apply to all nodes in the cluster
		if len(targetNodes) == 0 {
			nodes, listErr := r.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if listErr != nil {
				return fmt.Errorf("failed to list nodes: %w", listErr)
			}
			for _, node := range nodes.Items {
				targetNodes = append(targetNodes, node.Name)
			}
		}

		// Parse custom attributes and capacities
		rawAttrs, _, _ := unstructured.NestedMap(spec, "attributes")
		rawCaps, _, _ := unstructured.NestedMap(spec, "capacities")

		attrs := make(map[string]string)
		for k, v := range rawAttrs {
			if s, ok := v.(string); ok {
				attrs[k] = s
			}
		}
		attrs["driver"] = driverName
		attrs["type"] = deviceType

		caps := make(map[string]resource.Quantity)
		for k, v := range rawCaps {
			if s, ok := v.(string); ok {
				if q, parseErr := resource.ParseQuantity(s); parseErr == nil {
					caps[k] = q
				}
			}
		}

		var publishedSlices []string
		var activeFaults []string
		if health != "healthy" {
			activeFaults = append(activeFaults, health)
		}

		// For each target node, ensure ResourceSlice exists
		for _, nodeName := range targetNodes {
			sliceName := fmt.Sprintf("sim-slice-%s-%s-%s", sdp.GetName(), nodeName, sdp.GetNamespace())
			sliceName = strings.ReplaceAll(sliceName, ".", "-")

			if health == "disappear" {
				// disappear fault -> delete the slice if it exists
				_ = r.clientset.ResourceV1().ResourceSlices().Delete(ctx, sliceName, metav1.DeleteOptions{})
				r.eventRecorder.Eventf(&sdp, corev1.EventTypeNormal, "ResourceSliceDeleted", "Deleted ResourceSlice %s due to disappear fault", sliceName)
				continue
			}

			activeSliceNames[sliceName] = true
			publishedSlices = append(publishedSlices, sliceName)

			runDevCount := int(deviceCount)
			if health == "capacity-exhausted" {
				runDevCount = 0
				r.eventRecorder.Eventf(&sdp, corev1.EventTypeWarning, "CapacityExhausted", "Capacity exhausted for pool %s", poolName)
			}

			if ensureErr := r.ensureResourceSlice(ctx, sliceName, sdp.GetNamespace(), sdp.GetName(), driverName, poolName, nodeName, health, runDevCount, attrs, caps, len(targetNodes)); ensureErr != nil {
				klog.Errorf("Failed to ensure ResourceSlice %s: %v", sliceName, ensureErr)
				r.eventRecorder.Eventf(&sdp, corev1.EventTypeWarning, "ResourceSliceEnsureFailed", "Failed to ensure ResourceSlice %s: %v", sliceName, ensureErr)
			}
		}

		// Update the status subresource
		if statusErr := r.updateSDPStatus(ctx, &sdp, publishedSlices, activeFaults); statusErr != nil {
			// Ignore update errors if status subresource is not yet active/supported
			_ = statusErr
		}
	}

	// 2. Clean up orphaned ResourceSlices created by our controller
	slices, err := r.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range slices.Items {
			// Slices managed by simulator have the label: draforge.oaslananka/managed-by: simulator
			if s.Labels["draforge.oaslananka/managed-by"] == "simulator" {
				if !activeSliceNames[s.Name] {
					klog.Infof("Deleting orphaned ResourceSlice: %s", s.Name)
					_ = r.clientset.ResourceV1().ResourceSlices().Delete(ctx, s.Name, metav1.DeleteOptions{})
				}
			}
		}
	}

	atomic.StoreInt64(&r.ActiveFaultsCount, localFaultCount)
	return nil
}

func (r *Reconciler) ensureResourceSlice(ctx context.Context, name, namespace, sdpName, driverName, poolName, nodeName, health string, devCount int, attrs map[string]string, caps map[string]resource.Quantity, sliceCount int) error {
	slicesClient := r.clientset.ResourceV1().ResourceSlices()

	// Build device list
	var devices []resourcev1.Device
	for i := 0; i < devCount; i++ {
		devName := fmt.Sprintf("dev-%d", i)

		// Map attributes to v1 DeviceAttribute
		bAttrs := make(map[resourcev1.QualifiedName]resourcev1.DeviceAttribute)
		for k, v := range attrs {
			strVal := v
			bAttrs[resourcev1.QualifiedName(k)] = resourcev1.DeviceAttribute{
				StringValue: &strVal,
			}
		}

		// Map capacities to v1 DeviceCapacity
		bCaps := make(map[resourcev1.QualifiedName]resourcev1.DeviceCapacity)
		for k, v := range caps {
			bCaps[resourcev1.QualifiedName(k)] = resourcev1.DeviceCapacity{
				Value: v,
			}
		}

		devices = append(devices, resourcev1.Device{
			Name:       devName,
			Attributes: bAttrs,
			Capacity:   bCaps,
		})
	}

	// Build the ResourceSlice object
	slice := &resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"draforge.oaslananka/managed-by": "simulator",
				"draforge.oaslananka/sdp-name":   sdpName,
				"draforge.oaslananka/health":     health,
				"project":                        "draforge",
			},
		},
		Spec: resourcev1.ResourceSliceSpec{
			Driver:   driverName,
			NodeName: &nodeName,
			Pool: resourcev1.ResourcePool{
				Name:               poolName,
				Generation:         1,
				ResourceSliceCount: int64(sliceCount),
			},
			Devices: devices,
		},
	}

	// Check if already exists
	existing, err := slicesClient.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		// Update existing
		slice.ResourceVersion = existing.ResourceVersion

		if equality.Semantic.DeepEqual(existing.Spec, slice.Spec) && equality.Semantic.DeepEqual(existing.Labels, slice.Labels) {
			return nil
		}

		_, err = slicesClient.Update(ctx, slice, metav1.UpdateOptions{})
		return err
	}

	// Create new
	_, err = slicesClient.Create(ctx, slice, metav1.CreateOptions{})
	return err
}

func (r *Reconciler) updateSDPStatus(ctx context.Context, sdp *unstructured.Unstructured, publishedSlices []string, activeFaults []string) error {
	sdpCopy := sdp.DeepCopy()

	slicesSlice := make([]interface{}, len(publishedSlices))
	for i, s := range publishedSlices {
		slicesSlice[i] = s
	}

	faultsSlice := make([]interface{}, len(activeFaults))
	for i, f := range activeFaults {
		faultsSlice[i] = f
	}

	status := map[string]interface{}{
		"publishedSlices": slicesSlice,
		"activeFaults":    faultsSlice,
	}

	existingStatus, _, _ := unstructured.NestedMap(sdp.Object, "status")
	if equality.Semantic.DeepEqual(existingStatus, status) {
		return nil
	}

	err := unstructured.SetNestedMap(sdpCopy.Object, status, "status")
	if err != nil {
		return fmt.Errorf("failed to set status: %w", err)
	}

	_, err = r.dynamicClient.Resource(sdpGVR).Namespace(sdp.GetNamespace()).UpdateStatus(ctx, sdpCopy, metav1.UpdateOptions{})
	return err
}
