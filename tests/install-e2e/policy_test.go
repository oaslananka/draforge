package installe2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

type versionPolicy struct {
	KindVersion           string                   `json:"kindVersion"`
	Profiles              map[string][]matrixEntry `json:"profiles"`
	NetworkPolicyProvider networkPolicyProvider    `json:"networkPolicyProvider"`
}

type networkPolicyProvider struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	ManifestURL string `json:"manifestUrl"`
	SHA256      string `json:"sha256"`
}

type matrixEntry struct {
	Kubernetes string `json:"kubernetes"`
	NodeImage  string `json:"nodeImage"`
}

func TestKubernetesVersionPolicy(t *testing.T) {
	data := readFixture(t, "kubernetes-versions.json")
	var policy versionPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatalf("decode Kubernetes version policy: %v", err)
	}
	if !strings.HasPrefix(policy.KindVersion, "v") {
		t.Fatalf("kindVersion must be an explicit v-prefixed release, got %q", policy.KindVersion)
	}

	if policy.NetworkPolicyProvider.Name != "calico" {
		t.Fatalf("install E2E must use a NetworkPolicy-capable CNI, got %q", policy.NetworkPolicyProvider.Name)
	}
	if !strings.HasPrefix(policy.NetworkPolicyProvider.Version, "v3.") {
		t.Fatalf("Calico version must be explicitly pinned, got %q", policy.NetworkPolicyProvider.Version)
	}
	if !strings.Contains(policy.NetworkPolicyProvider.ManifestURL, "/"+policy.NetworkPolicyProvider.Version+"/manifests/calico.yaml") {
		t.Fatalf("Calico manifest URL does not match version %q: %q", policy.NetworkPolicyProvider.Version, policy.NetworkPolicyProvider.ManifestURL)
	}
	if len(policy.NetworkPolicyProvider.SHA256) != 64 {
		t.Fatalf("Calico manifest must have a SHA-256 digest, got %q", policy.NetworkPolicyProvider.SHA256)
	}

	pullRequest := policy.Profiles["pull-request"]
	full := policy.Profiles["full"]
	if len(pullRequest) != 1 {
		t.Fatalf("pull-request profile must contain exactly one target, got %d", len(pullRequest))
	}
	if len(full) < 2 {
		t.Fatalf("full profile must contain at least two targets, got %d", len(full))
	}

	fullImages := make(map[string]struct{}, len(full))
	for _, entry := range full {
		validateMatrixEntry(t, entry)
		fullImages[entry.NodeImage] = struct{}{}
	}
	for _, entry := range pullRequest {
		validateMatrixEntry(t, entry)
		if _, ok := fullImages[entry.NodeImage]; !ok {
			t.Fatalf("pull-request target %q must also be present in the full profile", entry.NodeImage)
		}
	}

	provider := policy.NetworkPolicyProvider
	if provider.Name != "calico" || !strings.HasPrefix(provider.Version, "v") {
		t.Fatalf("unexpected NetworkPolicy provider %#v", provider)
	}
	expectedURL := "https://raw.githubusercontent.com/projectcalico/calico/" + provider.Version + "/manifests/calico.yaml"
	if provider.ManifestURL != expectedURL {
		t.Fatalf("NetworkPolicy manifest URL %q must equal %q", provider.ManifestURL, expectedURL)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(provider.SHA256) {
		t.Fatalf("NetworkPolicy manifest SHA-256 is not pinned: %q", provider.SHA256)
	}
}

func TestInstallResourceFixturesUseTypedDRAIdentity(t *testing.T) {
	objects := decodeDocuments(t, readFixture(t, "resources.yaml"))
	byKind := indexByKind(t, objects)

	deviceClass := typedObject[resourcev1.DeviceClass](t, byKind, "DeviceClass")
	if deviceClass.Name != "draforge-e2e-gpu" {
		t.Fatalf("unexpected DeviceClass name %q", deviceClass.Name)
	}

	claim := typedObject[resourcev1.ResourceClaim](t, byKind, "ResourceClaim")
	if claim.Namespace != "draforge-e2e" || claim.Name != "e2e-gpu-claim" {
		t.Fatalf("unexpected claim identity %s/%s", claim.Namespace, claim.Name)
	}
	if len(claim.Spec.Devices.Requests) != 1 || claim.Spec.Devices.Requests[0].Exactly == nil {
		t.Fatalf("claim must contain one exact device request: %#v", claim.Spec.Devices.Requests)
	}
	exact := claim.Spec.Devices.Requests[0].Exactly
	if exact.DeviceClassName != deviceClass.Name || exact.Count != 1 {
		t.Fatalf("claim request must select one %q device: %#v", deviceClass.Name, exact)
	}

	pool := byKind["SimulatedDevicePool"]
	if pool.GetNamespace() != claim.Namespace || pool.GetName() != "e2e-gpu-pool" {
		t.Fatalf("unexpected simulator pool identity %s/%s", pool.GetNamespace(), pool.GetName())
	}
	driver, found, err := unstructured.NestedString(pool.Object, "spec", "driverName")
	if err != nil || !found || driver != "sim.draforge.oaslananka" {
		t.Fatalf("unexpected simulator driver %q, found=%t, err=%v", driver, found, err)
	}
}

func TestClaimConsumerFixtureReferencesExistingClaim(t *testing.T) {
	objects := decodeDocuments(t, readFixture(t, "workload.yaml"))
	byKind := indexByKind(t, objects)
	pod := typedObject[corev1.Pod](t, byKind, "Pod")

	if pod.Namespace != "draforge-e2e" || pod.Name != "e2e-claim-consumer" {
		t.Fatalf("unexpected consumer Pod identity %s/%s", pod.Namespace, pod.Name)
	}
	if len(pod.Spec.ResourceClaims) != 1 || pod.Spec.ResourceClaims[0].ResourceClaimName == nil {
		t.Fatalf("consumer Pod must reference one existing ResourceClaim: %#v", pod.Spec.ResourceClaims)
	}
	if got := *pod.Spec.ResourceClaims[0].ResourceClaimName; got != "e2e-gpu-claim" {
		t.Fatalf("consumer Pod references claim %q", got)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("consumer Pod must disable service-account token automount")
	}
	if pod.Spec.SecurityContext == nil || pod.Spec.SecurityContext.RunAsNonRoot == nil || !*pod.Spec.SecurityContext.RunAsNonRoot {
		t.Fatal("consumer Pod must enforce runAsNonRoot")
	}
	if pod.Spec.SecurityContext.RunAsUser == nil || *pod.Spec.SecurityContext.RunAsUser != 1000 {
		t.Fatal("consumer Pod must use the chart-compatible numeric UID 1000")
	}
	if len(pod.Spec.Containers) != 1 || len(pod.Spec.Containers[0].Resources.Claims) != 1 {
		t.Fatalf("consumer container must request the Pod claim: %#v", pod.Spec.Containers)
	}
	if got := pod.Spec.Containers[0].Resources.Claims[0].Name; got != pod.Spec.ResourceClaims[0].Name {
		t.Fatalf("container claim %q does not match Pod claim %q", got, pod.Spec.ResourceClaims[0].Name)
	}
	container := pod.Spec.Containers[0]
	if container.SecurityContext == nil || container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
		t.Fatal("consumer container must use a read-only root filesystem")
	}
	if container.Resources.Requests.StorageEphemeral().IsZero() || container.Resources.Limits.StorageEphemeral().IsZero() {
		t.Fatal("consumer container must bound ephemeral storage")
	}
}

func TestNetworkPolicyProbeFixturesAreIsolatedAndHardened(t *testing.T) {
	objects := decodeDocuments(t, readFixture(t, "network-policy-probes.yaml"))
	if len(objects) != 2 {
		t.Fatalf("expected two NetworkPolicy probe Pods, got %d", len(objects))
	}

	pods := make(map[string]corev1.Pod, len(objects))
	for _, object := range objects {
		if object.GetKind() != "Pod" {
			t.Fatalf("NetworkPolicy probe fixture contains %s", object.GetKind())
		}
		var pod corev1.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &pod); err != nil {
			t.Fatalf("convert NetworkPolicy probe Pod: %v", err)
		}
		pods[pod.Name] = pod
	}

	allowed, allowedFound := pods["network-policy-allowed"]
	denied, deniedFound := pods["network-policy-denied"]
	if !allowedFound || !deniedFound {
		t.Fatalf("missing allowed or denied NetworkPolicy probe: %#v", pods)
	}
	if allowed.Namespace != "draforge-system" || allowed.Labels["draforge.oaslananka/metrics-client"] != "true" {
		t.Fatalf("allowed probe must match the controller metrics policy: %#v", allowed.ObjectMeta)
	}
	if denied.Namespace != "draforge-e2e" {
		t.Fatalf("denied probe must originate outside the system namespace, got %q", denied.Namespace)
	}
	if _, exists := denied.Labels["draforge.oaslananka/metrics-client"]; exists {
		t.Fatal("denied probe must not carry the controller metrics client label")
	}

	for _, pod := range []corev1.Pod{allowed, denied} {
		if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
			t.Fatalf("probe %s must disable service-account token automount", pod.Name)
		}
		if pod.Spec.SecurityContext == nil || pod.Spec.SecurityContext.RunAsNonRoot == nil || !*pod.Spec.SecurityContext.RunAsNonRoot {
			t.Fatalf("probe %s must enforce runAsNonRoot", pod.Name)
		}
		if pod.Spec.SecurityContext.RunAsUser == nil || *pod.Spec.SecurityContext.RunAsUser != 1000 {
			t.Fatalf("probe %s must use the chart-compatible numeric UID 1000", pod.Name)
		}
		if len(pod.Spec.Containers) != 1 {
			t.Fatalf("probe %s must contain one container", pod.Name)
		}
		container := pod.Spec.Containers[0]
		if container.ImagePullPolicy != corev1.PullNever {
			t.Fatalf("probe %s must use a preloaded local image", pod.Name)
		}
		if container.SecurityContext == nil || container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
			t.Fatalf("probe %s must use a read-only root filesystem", pod.Name)
		}
		if container.Resources.Requests.StorageEphemeral().IsZero() || container.Resources.Limits.StorageEphemeral().IsZero() {
			t.Fatalf("probe %s must bound ephemeral storage", pod.Name)
		}
	}
}

func validateMatrixEntry(t *testing.T, entry matrixEntry) {
	t.Helper()
	if !strings.HasPrefix(entry.Kubernetes, "v1.") {
		t.Fatalf("invalid Kubernetes version %q", entry.Kubernetes)
	}
	if !strings.Contains(entry.NodeImage, entry.Kubernetes+"@sha256:") {
		t.Fatalf("node image %q must be digest-pinned to Kubernetes %s", entry.NodeImage, entry.Kubernetes)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func decodeDocuments(t *testing.T, data []byte) []*unstructured.Unstructured {
	t.Helper()
	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var objects []*unstructured.Unstructured
	for {
		var raw map[string]interface{}
		err := decoder.Decode(&raw)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode fixture YAML: %v", err)
		}
		if len(raw) == 0 {
			continue
		}
		objects = append(objects, &unstructured.Unstructured{Object: raw})
	}
	return objects
}

func indexByKind(t *testing.T, objects []*unstructured.Unstructured) map[string]*unstructured.Unstructured {
	t.Helper()
	result := make(map[string]*unstructured.Unstructured, len(objects))
	for _, object := range objects {
		kind := object.GetKind()
		if kind == "" {
			t.Fatal("fixture object is missing kind")
		}
		if _, exists := result[kind]; exists {
			t.Fatalf("fixture contains duplicate kind %q", kind)
		}
		result[kind] = object
	}
	return result
}

func typedObject[T any](t *testing.T, objects map[string]*unstructured.Unstructured, kind string) *T {
	t.Helper()
	object, ok := objects[kind]
	if !ok {
		t.Fatalf("fixture is missing %s", kind)
	}
	var typed T
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &typed); err != nil {
		t.Fatalf("convert %s fixture to typed object: %v", kind, err)
	}
	return &typed
}
