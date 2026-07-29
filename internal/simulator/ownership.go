// Package simulator defines explicit ownership for cluster-scoped ResourceSlices.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	resourceSliceCleanupFinalizer  = "draforge.oaslananka/resourceslice-cleanup"
	resourceSliceManagedByLabel    = "draforge.oaslananka/managed-by"
	resourceSliceSDPNameLabel      = "draforge.oaslananka/sdp-name"
	resourceSliceSDPNamespaceLabel = "draforge.oaslananka/sdp-namespace"
	resourceSliceSDPUIDLabel       = "draforge.oaslananka/sdp-uid"
)

func resourceSliceOwnershipLabels(sdp *unstructured.Unstructured) map[string]string {
	return map[string]string{
		resourceSliceManagedByLabel:    "simulator",
		resourceSliceSDPNameLabel:      sdp.GetName(),
		resourceSliceSDPNamespaceLabel: sdp.GetNamespace(),
		resourceSliceSDPUIDLabel:       string(sdp.GetUID()),
	}
}

func mergeStringMaps(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overlay))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func removeString(values []string, removed string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != removed {
			result = append(result, value)
		}
	}
	return result
}

func (r *Reconciler) ensureResourceSliceCleanupFinalizer(
	ctx context.Context,
	sdp *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	if containsString(sdp.GetFinalizers(), resourceSliceCleanupFinalizer) {
		return sdp, nil
	}
	updated := sdp.DeepCopy()
	updated.SetFinalizers(append(updated.GetFinalizers(), resourceSliceCleanupFinalizer))
	stored, err := r.dynamicClient.Resource(sdpGVR).Namespace(sdp.GetNamespace()).Update(
		ctx,
		updated,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("add ResourceSlice cleanup finalizer to SimulatedDevicePool %s/%s: %w", sdp.GetNamespace(), sdp.GetName(), err)
	}
	return stored, nil
}

func (r *Reconciler) finalizeDeletingSimulatedDevicePool(
	ctx context.Context,
	sdp *unstructured.Unstructured,
) error {
	if !containsString(sdp.GetFinalizers(), resourceSliceCleanupFinalizer) {
		return nil
	}

	expectedNames, err := r.expectedResourceSliceNames(ctx, sdp)
	if err != nil {
		return err
	}
	slices, err := r.clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list ResourceSlices for SimulatedDevicePool finalization: %w", err)
	}
	for index := range slices.Items {
		slice := &slices.Items[index]
		if !resourceSliceBelongsToSDP(slice, sdp, expectedNames) {
			continue
		}
		if deleteErr := r.deleteResourceSlice(ctx, slice.Name); deleteErr != nil {
			return fmt.Errorf("delete ResourceSlice %s while finalizing SimulatedDevicePool %s/%s: %w", slice.Name, sdp.GetNamespace(), sdp.GetName(), deleteErr)
		}
	}

	updated := sdp.DeepCopy()
	updated.SetFinalizers(removeString(updated.GetFinalizers(), resourceSliceCleanupFinalizer))
	if _, err := r.dynamicClient.Resource(sdpGVR).Namespace(sdp.GetNamespace()).Update(
		ctx,
		updated,
		metav1.UpdateOptions{},
	); err != nil {
		return fmt.Errorf("remove ResourceSlice cleanup finalizer from SimulatedDevicePool %s/%s: %w", sdp.GetNamespace(), sdp.GetName(), err)
	}
	return nil
}

func (r *Reconciler) expectedResourceSliceNames(
	ctx context.Context,
	sdp *unstructured.Unstructured,
) (map[string]struct{}, error) {
	spec, found := decodeSimulatedDevicePoolSpec(sdp)
	if !found {
		return map[string]struct{}{}, nil
	}
	targetNodes, err := r.resolveTargetNodes(ctx, spec.targetNodes)
	if err != nil {
		return nil, fmt.Errorf("resolve target nodes while finalizing SimulatedDevicePool %s/%s: %w", sdp.GetNamespace(), sdp.GetName(), err)
	}
	names := make(map[string]struct{}, len(targetNodes))
	for _, nodeName := range targetNodes {
		names[simulatedResourceSliceName(sdp, nodeName)] = struct{}{}
	}
	return names, nil
}

func resourceSliceBelongsToSDP(
	slice *resourcev1.ResourceSlice,
	sdp *unstructured.Unstructured,
	expectedNames map[string]struct{},
) bool {
	if slice.Labels[resourceSliceManagedByLabel] != "simulator" {
		return false
	}
	if slice.Labels[resourceSliceSDPUIDLabel] != "" {
		return slice.Labels[resourceSliceSDPUIDLabel] == string(sdp.GetUID()) &&
			slice.Labels[resourceSliceSDPNamespaceLabel] == sdp.GetNamespace() &&
			slice.Labels[resourceSliceSDPNameLabel] == sdp.GetName()
	}
	_, expectedLegacyName := expectedNames[slice.Name]
	return expectedLegacyName && slice.Labels[resourceSliceSDPNameLabel] == sdp.GetName()
}

func (r *Reconciler) deleteResourceSlice(ctx context.Context, name string) error {
	err := r.clientset.ResourceV1().ResourceSlices().Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
