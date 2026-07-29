// Package simulator implements the virtual device pool manager and reconciliation logic.
// SPDX-License-Identifier: Apache-2.0
package simulator

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/oaslananka/draforge/internal/draeval"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
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

	// Check if already exists. Only a real NotFound result permits creation;
	// authorization, timeout, and transport errors must reach the workqueue policy.
	existing, err := slicesClient.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		slice.ResourceVersion = existing.ResourceVersion
		if equality.Semantic.DeepEqual(existing.Spec, slice.Spec) && equality.Semantic.DeepEqual(existing.Labels, slice.Labels) {
			return nil
		}
		if _, updateErr := slicesClient.Update(ctx, slice, metav1.UpdateOptions{}); updateErr != nil {
			return fmt.Errorf("update ResourceSlice %s: %w", name, updateErr)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get ResourceSlice %s: %w", name, err)
	}

	if _, createErr := slicesClient.Create(ctx, slice, metav1.CreateOptions{}); createErr != nil {
		return fmt.Errorf("create ResourceSlice %s: %w", name, createErr)
	}
	return nil
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
