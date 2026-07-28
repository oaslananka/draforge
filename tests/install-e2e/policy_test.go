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
	var policy versionPolicy
	if err := json.Unmarshal(readFixture(t, "kubernetes-versions.json"), &policy); err != nil {
		t.Fatalf("decode Kubernetes version policy: %v", err)
	}

	validateKindVersion(t, policy.KindVersion)
	validateNetworkPolicyProvider(t, policy.NetworkPolicyProvider)
	validateProfiles(t, policy.Profiles)
}

func TestInstallResourceFixturesUseTypedDRAIdentity(t *testing.T) {
	objects := decodeDocuments(t, readFixture(t, "resources.yaml"))
	byKind := indexByKind(t, objects)

	deviceClass := typedObject[resourcev1.DeviceClass](t, byKind, "DeviceClass")
	validateDeviceClass(t, deviceClass)

	claim := typedObject[resourcev1.ResourceClaim](t, byKind, "ResourceClaim")
	validateResourceClaim(t, claim, deviceClass.Name)
	validateSimulatorPool(t, byKind["SimulatedDevicePool"], claim.Namespace)
}

func TestClaimConsumerFixtureReferencesExistingClaim(t *testing.T) {
	objects := decodeDocuments(t, readFixture(t, "workload.yaml"))
	pod := typedObject[corev1.Pod](t, indexByKind(t, objects), "Pod")

	validateConsumerIdentity(t, pod)
	validateConsumerPodSecurity(t, pod)
	validateConsumerContainer(t, pod)
}

func TestNetworkPolicyProbeFixturesAreIsolatedAndHardened(t *testing.T) {
	objects := decodeDocuments(t, readFixture(t, "network-policy-probes.yaml"))
	if len(objects) != 2 {
		t.Fatalf("expected two NetworkPolicy probe Pods, got %d", len(objects))
	}

	pods := decodeProbePods(t, objects)
	allowed := requiredProbePod(t, pods, "network-policy-allowed")
	denied := requiredProbePod(t, pods, "network-policy-denied")

	validateAllowedProbe(t, allowed)
	validateDeniedProbe(t, denied)
	validateProbeHardening(t, allowed)
	validateProbeHardening(t, denied)
}

func validateKindVersion(t *testing.T, version string) {
	t.Helper()
	if !strings.HasPrefix(version, "v") {
		t.Fatalf("kindVersion must be an explicit v-prefixed release, got %q", version)
	}
}

func validateNetworkPolicyProvider(t *testing.T, provider networkPolicyProvider) {
	t.Helper()
	if provider.Name != "calico" {
		t.Fatalf("install E2E must use a NetworkPolicy-capable CNI, got %q", provider.Name)
	}
	if !strings.HasPrefix(provider.Version, "v3.") {
		t.Fatalf("Calico version must be explicitly pinned, got %q", provider.Version)
	}
	expectedURL := "https://raw.githubusercontent.com/projectcalico/calico/" + provider.Version + "/manifests/calico.yaml"
	if provider.ManifestURL != expectedURL {
		t.Fatalf("NetworkPolicy manifest URL %q must equal %q", provider.ManifestURL, expectedURL)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(provider.SHA256) {
		t.Fatalf("NetworkPolicy manifest SHA-256 is not pinned: %q", provider.SHA256)
	}
}

func validateProfiles(t *testing.T, profiles map[string][]matrixEntry) {
	t.Helper()
	pullRequest := profiles["pull-request"]
	full := profiles["full"]
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

func validateDeviceClass(t *testing.T, deviceClass *resourcev1.DeviceClass) {
	t.Helper()
	if deviceClass.Name != "draforge-e2e-gpu" {
		t.Fatalf("unexpected DeviceClass name %q", deviceClass.Name)
	}
	if len(deviceClass.Spec.Selectors) != 1 || deviceClass.Spec.Selectors[0].CEL == nil {
		t.Fatalf("DeviceClass must contain one CEL selector: %#v", deviceClass.Spec.Selectors)
	}
	expression := deviceClass.Spec.Selectors[0].CEL.Expression
	for _, fragment := range []string{
		`device.driver == "sim.draforge.oaslananka"`,
		`device.attributes["sim.draforge.oaslananka"].type == "gpu"`,
	} {
		if !strings.Contains(expression, fragment) {
			t.Fatalf("DeviceClass CEL selector %q is missing %q", expression, fragment)
		}
	}
}

func validateResourceClaim(t *testing.T, claim *resourcev1.ResourceClaim, deviceClassName string) {
	t.Helper()
	if claim.Namespace != "draforge-e2e" || claim.Name != "e2e-gpu-claim" {
		t.Fatalf("unexpected claim identity %s/%s", claim.Namespace, claim.Name)
	}
	if len(claim.Spec.Devices.Requests) != 1 || claim.Spec.Devices.Requests[0].Exactly == nil {
		t.Fatalf("claim must contain one exact device request: %#v", claim.Spec.Devices.Requests)
	}
	exact := claim.Spec.Devices.Requests[0].Exactly
	if exact.DeviceClassName != deviceClassName || exact.Count != 1 {
		t.Fatalf("claim request must select one %q device: %#v", deviceClassName, exact)
	}
	if len(exact.Selectors) != 1 || exact.Selectors[0].CEL == nil {
		t.Fatalf("claim request must contain one CEL selector: %#v", exact.Selectors)
	}
	expression := exact.Selectors[0].CEL.Expression
	for _, fragment := range []string{
		`device.attributes["sim.draforge.oaslananka"].model == "DRAForge-E2E-GPU"`,
		`device.capacity["sim.draforge.oaslananka"].memory.compareTo(quantity("8Gi")) >= 0`,
	} {
		if !strings.Contains(expression, fragment) {
			t.Fatalf("claim CEL selector %q is missing %q", expression, fragment)
		}
	}
}

func validateSimulatorPool(t *testing.T, pool *unstructured.Unstructured, namespace string) {
	t.Helper()
	if pool == nil {
		t.Fatal("fixture is missing SimulatedDevicePool")
	}
	if pool.GetNamespace() != namespace || pool.GetName() != "e2e-gpu-pool" {
		t.Fatalf("unexpected simulator pool identity %s/%s", pool.GetNamespace(), pool.GetName())
	}
	driver, found, err := unstructured.NestedString(pool.Object, "spec", "driverName")
	if err != nil || !found || driver != "sim.draforge.oaslananka" {
		t.Fatalf("unexpected simulator driver %q, found=%t, err=%v", driver, found, err)
	}
}

func validateConsumerIdentity(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	if pod.Namespace != "draforge-e2e" || pod.Name != "e2e-claim-consumer" {
		t.Fatalf("unexpected consumer Pod identity %s/%s", pod.Namespace, pod.Name)
	}
	if len(pod.Spec.ResourceClaims) != 1 || pod.Spec.ResourceClaims[0].ResourceClaimName == nil {
		t.Fatalf("consumer Pod must reference one existing ResourceClaim: %#v", pod.Spec.ResourceClaims)
	}
	if got := *pod.Spec.ResourceClaims[0].ResourceClaimName; got != "e2e-gpu-claim" {
		t.Fatalf("consumer Pod references claim %q", got)
	}
}

func validateConsumerPodSecurity(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("consumer Pod must disable service-account token automount")
	}
	securityContext := pod.Spec.SecurityContext
	if securityContext == nil || securityContext.RunAsNonRoot == nil || !*securityContext.RunAsNonRoot {
		t.Fatal("consumer Pod must enforce runAsNonRoot")
	}
	if securityContext.RunAsUser == nil || *securityContext.RunAsUser != 1000 {
		t.Fatal("consumer Pod must use the chart-compatible numeric UID 1000")
	}
}

func validateConsumerContainer(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	if len(pod.Spec.Containers) != 1 || len(pod.Spec.Containers[0].Resources.Claims) != 1 {
		t.Fatalf("consumer container must request the Pod claim: %#v", pod.Spec.Containers)
	}
	container := pod.Spec.Containers[0]
	if got := container.Resources.Claims[0].Name; got != pod.Spec.ResourceClaims[0].Name {
		t.Fatalf("container claim %q does not match Pod claim %q", got, pod.Spec.ResourceClaims[0].Name)
	}
	if container.SecurityContext == nil || container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
		t.Fatal("consumer container must use a read-only root filesystem")
	}
	if container.Resources.Requests.StorageEphemeral().IsZero() || container.Resources.Limits.StorageEphemeral().IsZero() {
		t.Fatal("consumer container must bound ephemeral storage")
	}
}

func decodeProbePods(t *testing.T, objects []*unstructured.Unstructured) map[string]corev1.Pod {
	t.Helper()
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
	return pods
}

func requiredProbePod(t *testing.T, pods map[string]corev1.Pod, name string) corev1.Pod {
	t.Helper()
	pod, found := pods[name]
	if !found {
		t.Fatalf("missing NetworkPolicy probe %q", name)
	}
	return pod
}

func validateAllowedProbe(t *testing.T, pod corev1.Pod) {
	t.Helper()
	if pod.Namespace != "draforge-system" || pod.Labels["draforge.oaslananka/metrics-client"] != "true" {
		t.Fatalf("allowed probe must match the controller metrics policy: %#v", pod.ObjectMeta)
	}
}

func validateDeniedProbe(t *testing.T, pod corev1.Pod) {
	t.Helper()
	if pod.Namespace != "draforge-e2e" {
		t.Fatalf("denied probe must originate outside the system namespace, got %q", pod.Namespace)
	}
	if _, exists := pod.Labels["draforge.oaslananka/metrics-client"]; exists {
		t.Fatal("denied probe must not carry the controller metrics client label")
	}
}

func validateProbeHardening(t *testing.T, pod corev1.Pod) {
	t.Helper()
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatalf("probe %s must disable service-account token automount", pod.Name)
	}
	securityContext := pod.Spec.SecurityContext
	if securityContext == nil || securityContext.RunAsNonRoot == nil || !*securityContext.RunAsNonRoot {
		t.Fatalf("probe %s must enforce runAsNonRoot", pod.Name)
	}
	if securityContext.RunAsUser == nil || *securityContext.RunAsUser != 1000 {
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
