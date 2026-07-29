// Package simulator implements SimulatedDevicePool synchronization.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

type simulatedDevicePoolSpec struct {
	driverName  string
	poolName    string
	deviceType  string
	deviceCount int64
	targetNodes []string
	health      string
	attributes  map[string]string
	capacities  map[string]resource.Quantity
}

// Reconcile performs one authoritative pool synchronization pass.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	sdps, err := r.dynamicClient.Resource(sdpGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// During installation, the CRD may not be discoverable yet.
		if strings.Contains(err.Error(), "could not find the scale subresource") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("list SimulatedDevicePools: %w", err)
	}

	activeSliceNames := make(map[string]bool)
	localFaultCount := int64(0)
	for index := range sdps.Items {
		faultCount, reconcileErr := r.reconcileSimulatedDevicePool(ctx, &sdps.Items[index], activeSliceNames)
		if reconcileErr != nil {
			return reconcileErr
		}
		localFaultCount += faultCount
	}
	if err := r.cleanupOrphanedResourceSlices(ctx, activeSliceNames); err != nil {
		return err
	}
	atomic.StoreInt64(&r.ActiveFaultsCount, localFaultCount)
	return nil
}

func (r *Reconciler) reconcileSimulatedDevicePool(
	ctx context.Context,
	sdp *unstructured.Unstructured,
	activeSliceNames map[string]bool,
) (int64, error) {
	if sdp.GetDeletionTimestamp() != nil {
		return 0, r.finalizeDeletingSimulatedDevicePool(ctx, sdp)
	}
	finalizedSDP, err := r.ensureResourceSliceCleanupFinalizer(ctx, sdp)
	if err != nil {
		return 0, err
	}
	sdp = finalizedSDP

	spec, found := decodeSimulatedDevicePoolSpec(sdp)
	if !found {
		return 0, nil
	}

	targetNodes, err := r.resolveTargetNodes(ctx, spec.targetNodes)
	if err != nil {
		return 0, fmt.Errorf("resolve target nodes for SimulatedDevicePool %s/%s: %w", sdp.GetNamespace(), sdp.GetName(), err)
	}

	publishedSlices := make([]string, 0, len(targetNodes))
	for _, nodeName := range targetNodes {
		sliceName, published, reconcileErr := r.reconcilePoolResourceSlice(
			ctx,
			sdp,
			spec,
			nodeName,
			len(targetNodes),
			activeSliceNames,
		)
		if reconcileErr != nil {
			return 0, reconcileErr
		}
		if published {
			publishedSlices = append(publishedSlices, sliceName)
		}
	}

	activeFaults := []string(nil)
	faultCount := int64(0)
	if spec.health != "healthy" {
		activeFaults = []string{spec.health}
		faultCount = 1
	}
	if err := r.updateSDPStatus(ctx, sdp, publishedSlices, activeFaults); err != nil {
		r.eventRecorder.Eventf(sdp, corev1.EventTypeWarning, "StatusUpdateFailed", "Failed to update SimulatedDevicePool status: %v", err)
		return 0, fmt.Errorf("update SimulatedDevicePool %s/%s status: %w", sdp.GetNamespace(), sdp.GetName(), err)
	}
	return faultCount, nil
}

func decodeSimulatedDevicePoolSpec(sdp *unstructured.Unstructured) (simulatedDevicePoolSpec, bool) {
	rawSpec, found, _ := unstructured.NestedMap(sdp.Object, "spec")
	if !found {
		return simulatedDevicePoolSpec{}, false
	}

	spec := simulatedDevicePoolSpec{}
	spec.driverName, _, _ = unstructured.NestedString(rawSpec, "driverName")
	spec.poolName, _, _ = unstructured.NestedString(rawSpec, "poolName")
	if spec.poolName == "" {
		spec.poolName = sdp.GetName()
	}
	spec.deviceType, _, _ = unstructured.NestedString(rawSpec, "deviceType")
	spec.deviceCount, _, _ = unstructured.NestedInt64(rawSpec, "deviceCount")
	spec.targetNodes, _, _ = unstructured.NestedStringSlice(rawSpec, "targetNodes")
	spec.health, _, _ = unstructured.NestedString(rawSpec, "health")
	if spec.health == "" {
		spec.health = "healthy"
	}
	spec.attributes = decodeStringAttributes(rawSpec, spec.driverName, spec.deviceType)
	spec.capacities = decodeCapacities(rawSpec)
	return spec, true
}

func decodeStringAttributes(rawSpec map[string]any, driverName, deviceType string) map[string]string {
	rawAttributes, _, _ := unstructured.NestedMap(rawSpec, "attributes")
	attributes := make(map[string]string, len(rawAttributes)+2)
	for name, rawValue := range rawAttributes {
		if value, ok := rawValue.(string); ok {
			attributes[name] = value
		}
	}
	attributes["driver"] = driverName
	attributes["type"] = deviceType
	return attributes
}

func decodeCapacities(rawSpec map[string]any) map[string]resource.Quantity {
	rawCapacities, _, _ := unstructured.NestedMap(rawSpec, "capacities")
	capacities := make(map[string]resource.Quantity, len(rawCapacities))
	for name, rawValue := range rawCapacities {
		value, ok := rawValue.(string)
		if !ok {
			continue
		}
		quantity, err := resource.ParseQuantity(value)
		if err == nil {
			capacities[name] = quantity
		}
	}
	return capacities
}

func (r *Reconciler) resolveTargetNodes(ctx context.Context, configured []string) ([]string, error) {
	if len(configured) > 0 {
		return configured, nil
	}
	nodes, err := r.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Nodes: %w", err)
	}
	targetNodes := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		targetNodes = append(targetNodes, node.Name)
	}
	return targetNodes, nil
}

func (r *Reconciler) reconcilePoolResourceSlice(
	ctx context.Context,
	sdp *unstructured.Unstructured,
	spec simulatedDevicePoolSpec,
	nodeName string,
	sliceCount int,
	activeSliceNames map[string]bool,
) (string, bool, error) {
	sliceName := simulatedResourceSliceName(sdp, nodeName)
	if spec.health == "disappear" {
		if err := r.deleteResourceSlice(ctx, sliceName); err != nil {
			r.eventRecorder.Eventf(sdp, corev1.EventTypeWarning, "ResourceSliceDeleteFailed", "Failed to delete ResourceSlice %s: %v", sliceName, err)
			return "", false, fmt.Errorf("delete ResourceSlice %s: %w", sliceName, err)
		}
		r.eventRecorder.Eventf(sdp, corev1.EventTypeNormal, "ResourceSliceDeleted", "Deleted ResourceSlice %s due to disappear fault", sliceName)
		return "", false, nil
	}

	activeSliceNames[sliceName] = true
	deviceCount := int(spec.deviceCount)
	if spec.health == "capacity-exhausted" {
		deviceCount = 0
		r.eventRecorder.Eventf(sdp, corev1.EventTypeWarning, "CapacityExhausted", "Capacity exhausted for pool %s", spec.poolName)
	}
	if err := r.ensureResourceSlice(ctx, desiredResourceSlice{
		name:        sliceName,
		sdp:         sdp,
		driverName:  spec.driverName,
		poolName:    spec.poolName,
		nodeName:    nodeName,
		health:      spec.health,
		deviceCount: deviceCount,
		attributes:  spec.attributes,
		capacities:  spec.capacities,
		sliceCount:  sliceCount,
	}); err != nil {
		klog.Errorf("Failed to ensure ResourceSlice %s: %v", sliceName, err)
		r.eventRecorder.Eventf(sdp, corev1.EventTypeWarning, "ResourceSliceEnsureFailed", "Failed to ensure ResourceSlice %s: %v", sliceName, err)
		return "", false, fmt.Errorf("ensure ResourceSlice %s: %w", sliceName, err)
	}
	return sliceName, true, nil
}

func simulatedResourceSliceName(sdp *unstructured.Unstructured, nodeName string) string {
	name := fmt.Sprintf("sim-slice-%s-%s-%s", sdp.GetName(), nodeName, sdp.GetNamespace())
	return strings.ReplaceAll(name, ".", "-")
}

func (r *Reconciler) cleanupOrphanedResourceSlices(ctx context.Context, activeSliceNames map[string]bool) error {
	slices, err := r.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list ResourceSlices for orphan cleanup: %w", err)
	}
	for _, slice := range slices.Items {
		if slice.Labels["draforge.oaslananka/managed-by"] != "simulator" || activeSliceNames[slice.Name] {
			continue
		}
		klog.Infof("Deleting orphaned ResourceSlice: %s", slice.Name)
		if err := r.deleteResourceSlice(ctx, slice.Name); err != nil {
			return fmt.Errorf("delete orphaned ResourceSlice %s: %w", slice.Name, err)
		}
	}
	return nil
}
