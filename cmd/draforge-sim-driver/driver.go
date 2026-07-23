// Package main contains the synthetic node driver's CDI runtime.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	cdiFileName         = "draforge-sim-devices.json"
	cdiKind             = "draforge.oaslananka/sim"
	simulatedDriverName = "sim.draforge.oaslananka"

	readinessReasonInitializing  = "initializing"
	readinessReasonReady         = "ready"
	readinessReasonKubernetesAPI = "kubernetes_api_unavailable"
	readinessReasonCDIWrite      = "cdi_write_failed"
)

type outputMode string

const (
	outputModeDemo outputMode = "demo"
	outputModeNode outputMode = "node"
)

func parseOutputMode(value string) (outputMode, error) {
	mode := outputMode(value)
	switch mode {
	case outputModeDemo, outputModeNode:
		return mode, nil
	default:
		return "", fmt.Errorf("output mode must be %q or %q", outputModeDemo, outputModeNode)
	}
}

// CDIDevice represents a CDI device entry.
type CDIDevice struct {
	Name           string         `json:"name"`
	ContainerEdits ContainerEdits `json:"containerEdits"`
}

// ContainerEdits describes environment injected for a synthetic device.
type ContainerEdits struct {
	Env []string `json:"env"`
}

// CDISpec is the generated Container Device Interface document.
type CDISpec struct {
	CDIVersion string      `json:"cdiVersion"`
	Kind       string      `json:"kind"`
	Devices    []CDIDevice `json:"devices"`
}

type atomicCDIWriter struct {
	createTemp func(dir, pattern string) (*os.File, error)
	rename     func(oldPath, newPath string) error
	openDir    func(path string) (*os.File, error)
}

func newAtomicCDIWriter() *atomicCDIWriter {
	return &atomicCDIWriter{
		createTemp: os.CreateTemp,
		rename:     os.Rename,
		openDir:    os.Open,
	}
}

func (w *atomicCDIWriter) Write(path string, spec CDISpec) (returnErr error) {
	data, marshalErr := json.MarshalIndent(spec, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshal CDI document: %w", marshalErr)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	temp, createErr := w.createTemp(dir, ".draforge-cdi-*")
	if createErr != nil {
		return fmt.Errorf("create temporary CDI document: %w", createErr)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write temporary CDI document: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("fsync temporary CDI document: %w", err)
	}
	if err := temp.Chmod(0o644); err != nil {
		return fmt.Errorf("set temporary CDI permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("fsync temporary CDI permissions: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary CDI document: %w", err)
	}
	if err := validateGeneratedFile(tempPath); err != nil {
		return err
	}
	if err := w.rename(tempPath, path); err != nil {
		return fmt.Errorf("atomically replace CDI document: %w", err)
	}

	dirHandle, openErr := w.openDir(dir)
	if openErr != nil {
		return fmt.Errorf("open CDI directory for fsync: %w", openErr)
	}
	defer func() {
		if closeErr := dirHandle.Close(); closeErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("close CDI directory: %w", closeErr)
		}
	}()
	if err := dirHandle.Sync(); err != nil {
		return fmt.Errorf("fsync CDI directory: %w", err)
	}
	return nil
}

func validateGeneratedFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat generated CDI document: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("generated CDI document is not a regular file")
	}
	if info.Mode().Perm() != 0o644 {
		return fmt.Errorf("generated CDI document mode is %o, want 644", info.Mode().Perm())
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("generated CDI document ownership is unavailable")
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("generated CDI document owner uid is %d, want %d", stat.Uid, os.Geteuid())
	}
	return nil
}

func prepareCDIDirectory(path string, mode outputMode) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("CDI directory must be an absolute path")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create %s CDI directory %q: %w", mode, path, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat CDI directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("CDI directory %q must not be a symbolic link", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("CDI path %q is not a directory", path)
	}

	probe, err := os.CreateTemp(path, ".draforge-write-probe-*")
	if err != nil {
		return fmt.Errorf("CDI directory %q is not writable: %w", path, err)
	}
	probePath := probe.Name()
	defer func() { _ = os.Remove(probePath) }()
	if err := probe.Chmod(0o644); err != nil {
		_ = probe.Close()
		return fmt.Errorf("set CDI write probe permissions: %w", err)
	}
	if err := probe.Sync(); err != nil {
		_ = probe.Close()
		return fmt.Errorf("fsync CDI write probe: %w", err)
	}
	if err := probe.Close(); err != nil {
		return fmt.Errorf("close CDI write probe: %w", err)
	}
	if err := validateGeneratedFile(probePath); err != nil {
		return fmt.Errorf("validate CDI directory ownership and permissions: %w", err)
	}
	return nil
}

type driverStatus struct {
	mu          sync.RWMutex
	mode        outputMode
	ready       bool
	reason      string
	message     string
	devices     int
	lastSuccess *time.Time
}

func newDriverStatus(mode outputMode) *driverStatus {
	return &driverStatus{
		mode:    mode,
		reason:  readinessReasonInitializing,
		message: "CDI document has not been generated yet",
	}
}

func (s *driverStatus) MarkReady(devices int) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
	s.reason = readinessReasonReady
	s.message = "CDI document is current"
	s.devices = devices
	s.lastSuccess = &now
}

func (s *driverStatus) MarkFailure(reason, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = false
	s.reason = reason
	s.message = message
}

func (s *driverStatus) Readiness() (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready, s.reason
}

type readinessPayload struct {
	Status      string     `json:"status"`
	Ready       bool       `json:"ready"`
	Mode        outputMode `json:"mode"`
	Reason      string     `json:"reason"`
	Message     string     `json:"message"`
	Devices     int        `json:"devices"`
	LastSuccess *time.Time `json:"lastSuccess,omitempty"`
}

func (s *driverStatus) snapshot() readinessPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := "not_ready"
	if s.ready {
		status = "ready"
	}
	return readinessPayload{
		Status:      status,
		Ready:       s.ready,
		Mode:        s.mode,
		Reason:      s.reason,
		Message:     s.message,
		Devices:     s.devices,
		LastSuccess: s.lastSuccess,
	}
}

func newHealthHandler(status *driverStatus) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"live"}`))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		payload := status.snapshot()
		w.Header().Set("Content-Type", "application/json")
		if !payload.Ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(payload)
	})
	return mux
}

type cdiDriver struct {
	mode      outputMode
	clientset kubernetes.Interface
	nodeName  string
	cdiPath   string
	writer    *atomicCDIWriter
	status    *driverStatus
}

func newCDIDriver(mode outputMode, clientset kubernetes.Interface, nodeName, cdiPath string, writer *atomicCDIWriter, status *driverStatus) *cdiDriver {
	return &cdiDriver{
		mode:      mode,
		clientset: clientset,
		nodeName:  nodeName,
		cdiPath:   cdiPath,
		writer:    writer,
		status:    status,
	}
}

func (d *cdiDriver) Refresh(ctx context.Context) error {
	if d.mode == outputModeDemo {
		spec := demoCDISpec()
		if err := d.writer.Write(d.cdiPath, spec); err != nil {
			d.status.MarkFailure(readinessReasonCDIWrite, "could not atomically update the CDI document")
			return err
		}
		d.status.MarkReady(len(spec.Devices))
		return nil
	}
	if d.clientset == nil {
		d.status.MarkFailure(readinessReasonKubernetesAPI, "Kubernetes client is unavailable")
		return errors.New("kubernetes client is unavailable")
	}

	claims, err := d.clientset.ResourceV1().ResourceClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		d.status.MarkFailure(readinessReasonKubernetesAPI, "Kubernetes ResourceClaims could not be listed")
		return fmt.Errorf("list ResourceClaims: %w", err)
	}
	spec := buildCDISpec(claims.Items, d.nodeName)
	if err := d.writer.Write(d.cdiPath, spec); err != nil {
		d.status.MarkFailure(readinessReasonCDIWrite, "could not atomically update the CDI document")
		return err
	}
	d.status.MarkReady(len(spec.Devices))
	return nil
}

func buildCDISpec(claims []resourcev1.ResourceClaim, nodeName string) CDISpec {
	orderedClaims := append([]resourcev1.ResourceClaim(nil), claims...)
	sort.Slice(orderedClaims, func(i, j int) bool {
		left := orderedClaims[i].Namespace + "/" + orderedClaims[i].Name
		right := orderedClaims[j].Namespace + "/" + orderedClaims[j].Name
		return left < right
	})

	seen := make(map[string]struct{})
	devices := make([]CDIDevice, 0)
	for _, claim := range orderedClaims {
		if claim.Status.Allocation == nil {
			continue
		}
		nodeMatched := allocationMatchesNode(claim.Status.Allocation, nodeName)
		for _, result := range claim.Status.Allocation.Devices.Results {
			if !nodeMatched && result.Pool != nodeName {
				continue
			}
			if !isSimulatedDriver(result.Driver) {
				continue
			}
			identity := qualifiedDeviceIdentity(result)
			if _, exists := seen[identity]; exists {
				continue
			}
			seen[identity] = struct{}{}
			devices = append(devices, CDIDevice{
				Name: cdiDeviceName(identity, result.Device),
				ContainerEdits: ContainerEdits{Env: []string{
					"DRAFORGE_VIRTUAL_IDENTITY=" + identity,
					"DRAFORGE_VIRTUAL_DRIVER=" + result.Driver,
					"DRAFORGE_VIRTUAL_POOL=" + result.Pool,
					"DRAFORGE_VIRTUAL_DEVICE=" + result.Device,
					"DRAFORGE_VIRTUAL_TYPE=sim-device",
					"DRAFORGE_CLAIM_NAME=" + claim.Name,
					"DRAFORGE_CLAIM_NAMESPACE=" + claim.Namespace,
				}},
			})
		}
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	return CDISpec{CDIVersion: "0.5.0", Kind: cdiKind, Devices: devices}
}

func allocationMatchesNode(allocation *resourcev1.AllocationResult, nodeName string) bool {
	if allocation.NodeSelector == nil {
		return false
	}
	for _, term := range allocation.NodeSelector.NodeSelectorTerms {
		for _, requirement := range term.MatchFields {
			if requirement.Key == "metadata.name" && containsString(requirement.Values, nodeName) {
				return true
			}
		}
		for _, requirement := range term.MatchExpressions {
			if requirement.Key == "kubernetes.io/hostname" && containsString(requirement.Values, nodeName) {
				return true
			}
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func isSimulatedDriver(driver string) bool {
	return driver == simulatedDriverName || strings.HasSuffix(driver, ".draforge.oaslananka")
}

func qualifiedDeviceIdentity(result resourcev1.DeviceRequestAllocationResult) string {
	return result.Driver + "/" + result.Pool + "/" + result.Device
}

func cdiDeviceName(identity, device string) string {
	hash := sha256.Sum256([]byte(identity))
	suffix := hex.EncodeToString(hash[:12])
	prefix := sanitizeCDIName(device)
	if prefix == "" {
		prefix = "device"
	}
	if len(prefix) > 38 {
		prefix = strings.Trim(prefix[:38], "-._")
	}
	return prefix + "-" + suffix
}

func sanitizeCDIName(value string) string {
	var builder strings.Builder
	previousSeparator := false
	for _, r := range strings.ToLower(value) {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_'
		if valid {
			builder.WriteRune(r)
			previousSeparator = false
			continue
		}
		if !previousSeparator {
			builder.WriteByte('-')
			previousSeparator = true
		}
	}
	return strings.Trim(builder.String(), "-._")
}

func demoCDISpec() CDISpec {
	return CDISpec{
		CDIVersion: "0.5.0",
		Kind:       cdiKind,
		Devices: []CDIDevice{
			{
				Name: "demo-dev-0",
				ContainerEdits: ContainerEdits{Env: []string{
					"DRAFORGE_VIRTUAL_IDENTITY=demo/dev-0",
					"DRAFORGE_VIRTUAL_DEVICE=dev-0",
					"DRAFORGE_VIRTUAL_TYPE=gpu",
				}},
			},
			{
				Name: "demo-dev-1",
				ContainerEdits: ContainerEdits{Env: []string{
					"DRAFORGE_VIRTUAL_IDENTITY=demo/dev-1",
					"DRAFORGE_VIRTUAL_DEVICE=dev-1",
					"DRAFORGE_VIRTUAL_TYPE=camera",
				}},
			},
		},
	}
}
