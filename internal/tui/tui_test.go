// Package tui unit tests.
// SPDX-License-Identifier: Apache-2.0
package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/oaslananka/draforge/pkg/model"
)

func TestNewModel(t *testing.T) {
	m := NewModel(nil)
	if m.activeTab != viewSummary {
		t.Errorf("expected activeTab viewSummary (0), got %d", m.activeTab)
	}
	if !m.loading {
		t.Error("expected loading to be true on init")
	}
	if m.clientset != nil {
		t.Error("expected nil clientset")
	}
}

func TestUpdateQuitKeys(t *testing.T) {
	m := NewModel(nil)
	for _, key := range []string{"q", "ctrl+c"} {
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd == nil {
			t.Errorf("expected quit cmd for key %q, got nil", key)
		}
		if _, ok := result.(modelState); !ok {
			t.Errorf("expected modelState for key %q", key)
		}
	}
}

func TestUpdateTabSwitch(t *testing.T) {
	m := NewModel(nil)

	tests := []struct {
		key     string
		view    activeView
		viewStr string
	}{
		{"1", viewSummary, "Cluster Overview"},
		{"2", viewPools, "Simulated Resource Pools"},
		{"3", viewDevices, "Discovered Devices"},
		{"4", viewClaims, "Resource Claims"},
		{"5", viewDoctor, "Diagnostics (Doctor)"},
	}

	for _, tt := range tests {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
		updated, ok := result.(modelState)
		if !ok {
			t.Fatalf("expected modelState for key %q", tt.key)
		}
		if updated.activeTab != tt.view {
			t.Errorf("key %q: expected tab %d, got %d", tt.key, tt.view, updated.activeTab)
		}
	}
}

func TestUpdateRefreshKey(t *testing.T) {
	m := NewModel(nil)
	m.loading = false
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	updated, ok := result.(modelState)
	if !ok {
		t.Fatal("expected modelState")
	}
	if !updated.loading {
		t.Error("expected loading to be true after refresh key")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd from refresh key")
	}
}

func TestViewLoading(t *testing.T) {
	m := NewModel(nil)
	m.loading = true
	v := m.View()
	if !strings.Contains(v, "Loading") {
		t.Errorf("loading view should contain 'Loading', got: %q", v)
	}
}

func TestViewError(t *testing.T) {
	m := NewModel(nil)
	m.loading = false
	m.err = errTestConnection
	v := m.View()
	if !strings.Contains(v, "Error connecting") {
		t.Errorf("error view should contain 'Error connecting', got: %q", v)
	}
	if !strings.Contains(v, "'r' to retry") {
		t.Errorf("error view should show retry tip, got: %q", v)
	}
}

func TestViewSummaryTab(t *testing.T) {
	m := NewModel(nil)
	m.loading = false

	// Inject some data to exercise the summary rendering.
	m.pools = []model.DevicePool{
		{Name: "pool-a", DriverName: "test", NodeName: "node-1", DeviceCount: 4},
	}
	m.devices = []model.Device{
		{Name: "gpu-0", Type: "gpu", NodeName: "node-1", Status: "healthy"},
	}
	m.claims = []model.ResourceClaimInfo{
		{Name: "claim-1", Namespace: "default", DeviceClassName: "gpu-class", Status: "Allocated"},
	}
	m.lastRefreshed = time.Now()

	v := m.View()
	if !strings.Contains(v, "Cluster Overview") {
		t.Errorf("summary view should contain 'Cluster Overview', got: %q", v)
	}
	if !strings.Contains(v, "Active Device Pools: 1") {
		t.Errorf("summary should show pool count, got: %q", v)
	}
}

func TestViewPoolsTab(t *testing.T) {
	m := NewModel(nil)
	m.loading = false
	m.activeTab = viewPools
	m.pools = []model.DevicePool{
		{Name: "pool-1", DriverName: "sim", NodeName: "node-1", DeviceCount: 2},
	}

	v := m.View()
	if !strings.Contains(v, "Simulated Resource Pools") {
		t.Errorf("pools view header missing, got: %q", v)
	}
	if !strings.Contains(v, "pool-1") {
		t.Errorf("pools view should list pool names, got: %q", v)
	}
}

func TestViewDevicesTab(t *testing.T) {
	m := NewModel(nil)
	m.loading = false
	m.activeTab = viewDevices
	m.devices = []model.Device{
		{Name: "fpga-0", Type: "fpga", NodeName: "node-2", Status: "Allocated"},
	}

	v := m.View()
	if !strings.Contains(v, "Discovered Devices") {
		t.Errorf("devices view header missing, got: %q", v)
	}
	if !strings.Contains(v, "fpga-0") {
		t.Errorf("devices view should list device IDs, got: %q", v)
	}
}

func TestViewClaimsTabEmpty(t *testing.T) {
	m := NewModel(nil)
	m.loading = false
	m.activeTab = viewClaims
	// No claims injected → should show empty message.
	v := m.View()
	if !strings.Contains(v, "No active ResourceClaims") {
		t.Errorf("claims empty view should show hint, got: %q", v)
	}
}

func TestViewDoctorTab(t *testing.T) {
	m := NewModel(nil)
	m.loading = false
	m.activeTab = viewDoctor
	m.docReport = model.DoctorReport{
		Results: []model.DoctorCheckResult{
			{ID: "DR01", Name: "API Check", Status: model.StatusPass, Evidence: "reachable"},
		},
	}
	v := m.View()
	if !strings.Contains(v, "Diagnostics (Doctor)") {
		t.Errorf("doctor view header missing, got: %q", v)
	}
	if !strings.Contains(v, "API Check") {
		t.Errorf("doctor view should list check names, got: %q", v)
	}
}

func TestRefreshMsg(t *testing.T) {
	m := NewModel(nil)
	m.loading = true

	msg := refreshMsg{
		pools: []model.DevicePool{
			{Name: "pool-x", DriverName: "x", NodeName: "n1", DeviceCount: 2},
		},
		devices: []model.Device{
			{Name: "dev-a", Type: "nic", NodeName: "n1", Status: "healthy"},
		},
		claims: []model.ResourceClaimInfo{
			{Name: "c1", Namespace: "default", DeviceClassName: "class-a", Status: "Pending"},
		},
		docReport: model.DoctorReport{
			Results: []model.DoctorCheckResult{
				{ID: "DR02", Name: "Version", Status: model.StatusWarn, Evidence: "mismatch"},
			},
		},
	}

	result, _ := m.Update(msg)
	updated, ok := result.(modelState)
	if !ok {
		t.Fatal("expected modelState")
	}
	if updated.loading {
		t.Error("expected loading false after refreshMsg")
	}
	if updated.err != nil {
		t.Errorf("expected no error, got: %v", updated.err)
	}
	if len(updated.pools) != 1 || updated.pools[0].Name != "pool-x" {
		t.Error("pools not updated from refreshMsg")
	}
	if len(updated.devices) != 1 || updated.devices[0].Name != "dev-a" {
		t.Error("devices not updated from refreshMsg")
	}
	if len(updated.claims) != 1 || updated.claims[0].Name != "c1" {
		t.Error("claims not updated from refreshMsg")
	}
	if len(updated.docReport.Results) != 1 {
		t.Error("docReport not updated from refreshMsg")
	}
}

func TestRefreshMsgError(t *testing.T) {
	m := NewModel(nil)
	m.loading = true

	msg := refreshMsg{err: errTestRefresh}
	result, _ := m.Update(msg)
	updated, ok := result.(modelState)
	if !ok {
		t.Fatal("expected modelState")
	}
	if updated.loading {
		t.Error("expected loading false after error refreshMsg")
	}
	if !errors.Is(updated.err, errTestRefresh) {
		t.Errorf("expected errTestRefresh, got: %v", updated.err)
	}
}

func TestActiveTabHighlight(t *testing.T) {
	m := NewModel(nil)
	m.loading = false

	// Active tab = viewSummary (index 0) → [1] Summary should be highlighted.
	m.activeTab = viewSummary
	v := m.View()
	// The highlighted tab uses ANSI escape codes (bold+cyan) "> [1] Summary <"
	if !strings.Contains(v, "Summary") {
		t.Errorf("summary tab label should appear, got: %q", v)
	}

	// Switch to viewDoctor
	m.activeTab = viewDoctor
	v = m.View()
	if !strings.Contains(v, "Doctor") {
		t.Errorf("doctor tab label should appear, got: %q", v)
	}
}

// Sentinel errors for testing error paths.
var errTestConnection = errTui("simulated connection error")
var errTestRefresh = errTui("simulated refresh failure")

type errTui string

func (e errTui) Error() string { return string(e) }
