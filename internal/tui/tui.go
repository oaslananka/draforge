// Package tui provides a Bubble Tea terminal UI for browsing and inspecting DRA resources.
// SPDX-License-Identifier: Apache-2.0
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/oaslananka/draforge/internal/discovery"
	"github.com/oaslananka/draforge/internal/doctor"
	"github.com/oaslananka/draforge/pkg/model"
	"k8s.io/client-go/kubernetes"
)

type activeView int

const (
	viewSummary activeView = iota
	viewPools
	viewDevices
	viewClaims
	viewDoctor
)

// modelState holds the TUI state.
type modelState struct {
	clientset     *kubernetes.Clientset
	activeTab     activeView
	pools         []model.DevicePool
	devices       []model.Device
	claims        []model.ResourceClaimInfo
	docReport     model.DoctorReport
	err           error
	loading       bool
	lastRefreshed time.Time
}

// NewModel initializes the TUI model.
func NewModel(clientset *kubernetes.Clientset) modelState {
	return modelState{
		clientset: clientset,
		activeTab: viewSummary,
		loading:   true,
	}
}

// Init initializes the Bubble Tea program.
func (m modelState) Init() tea.Cmd {
	return m.refreshCmd()
}

// Update handles incoming messages and updates TUI state.
func (m modelState) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.loading = true
			return m, m.refreshCmd()
		case "1":
			m.activeTab = viewSummary
		case "2":
			m.activeTab = viewPools
		case "3":
			m.activeTab = viewDevices
		case "4":
			m.activeTab = viewClaims
		case "5":
			m.activeTab = viewDoctor
		}

	case refreshMsg:
		m.loading = false
		m.lastRefreshed = time.Now()
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.pools = msg.pools
			m.devices = msg.devices
			m.claims = msg.claims
			m.docReport = msg.docReport
			m.err = nil
		}
	}

	return m, nil
}

// View renders the terminal UI layout.
func (m modelState) View() string {
	var s strings.Builder

	// Header
	s.WriteString("┌────────────────────────────────────────────────────────┐\n")
	s.WriteString("│                   DRAForge Terminal UI                 │\n")
	s.WriteString("└────────────────────────────────────────────────────────┘\n\n")

	// Tabs Header
	tabs := []string{"[1] Summary", "[2] Pools", "[3] Devices", "[4] Claims", "[5] Doctor"}
	for i, t := range tabs {
		if activeView(i) == m.activeTab {
			fmt.Fprintf(&s, " > \x1b[1;36m%s\x1b[0m < ", t)
		} else {
			fmt.Fprintf(&s, "   %s   ", t)
		}
	}
	s.WriteString("\n\n")

	if m.loading {
		s.WriteString(" Loading cluster data from DigitalOcean...\n")
		return s.String()
	}

	if m.err != nil {
		fmt.Fprintf(&s, " \x1b[1;31mError connecting to cluster:\x1b[0m %v\n\n", m.err)
		s.WriteString(" Press 'r' to retry or 'q' to quit.\n")
		return s.String()
	}

	// Active View Content
	switch m.activeTab {
	case viewSummary:
		s.WriteString("== Cluster Overview ==\n")
		fmt.Fprintf(&s, "  Active Device Pools: %d\n", len(m.pools))
		fmt.Fprintf(&s, "  Discovered Devices:  %d\n", len(m.devices))
		fmt.Fprintf(&s, "  Resource Claims:     %d\n", len(m.claims))
		fmt.Fprintf(&s, "  Diagnostics Run:     %s\n", m.lastRefreshed.Format("15:04:05"))

	case viewPools:
		s.WriteString("== Simulated Resource Pools ==\n")
		if len(m.pools) == 0 {
			s.WriteString("  No active device pools found.\n")
		} else {
			fmt.Fprintf(&s, "  %-25s %-15s %-15s %-10s\n", "NAME", "DRIVER", "NODE", "DEVICES")
			s.WriteString(strings.Repeat("-", 70) + "\n")
			for _, p := range m.pools {
				fmt.Fprintf(&s, "  %-25s %-15s %-15s %-10d\n", p.Name, p.DriverName, p.NodeName, p.DeviceCount)
			}
		}

	case viewDevices:
		s.WriteString("== Discovered Devices ==\n")
		if len(m.devices) == 0 {
			s.WriteString("  No devices published. Activate SimulatedDevicePools.\n")
		} else {
			fmt.Fprintf(&s, "  %-25s %-12s %-15s %-10s\n", "DEVICE ID", "TYPE", "NODE", "STATUS")
			s.WriteString(strings.Repeat("-", 70) + "\n")
			for _, d := range m.devices {
				idTrunc := d.Name
				if len(idTrunc) > 25 {
					idTrunc = idTrunc[:22] + "..."
				}
				fmt.Fprintf(&s, "  %-25s %-12s %-15s %-10s\n", idTrunc, d.Type, d.NodeName, d.Status)
			}
		}

	case viewClaims:
		s.WriteString("== Resource Claims ==\n")
		if len(m.claims) == 0 {
			s.WriteString("  No active ResourceClaims found.\n")
		} else {
			fmt.Fprintf(&s, "  %-25s %-15s %-35s %-12s %-15s\n", "CLAIM NAME", "NAMESPACE", "CLASSES", "ALLOCATIONS", "STATUS")
			s.WriteString(strings.Repeat("-", 110) + "\n")
			for _, claim := range m.claims {
				classes := strings.Join(claim.RequestedClassNames(), ",")
				if classes == "" {
					classes = "-"
				}
				fmt.Fprintf(&s, "  %-25s %-15s %-35s %-12d %-15s\n", claim.Name, claim.Namespace, classes, len(claim.EffectiveAllocations()), claim.Status)
			}
		}

	case viewDoctor:
		s.WriteString("== Diagnostics (Doctor) ==\n")
		if len(m.docReport.Results) == 0 {
			s.WriteString("  No diagnostic runs available.\n")
		} else {
			for _, res := range m.docReport.Results {
				statusColor := "\x1b[32m" // Green
				switch res.Status {
				case model.StatusFail:
					statusColor = "\x1b[31m" // Red
				case model.StatusWarn:
					statusColor = "\x1b[33m" // Yellow
				}
				fmt.Fprintf(&s, "  [%s%s\x1b[0m] %s: %s\n", statusColor, res.Status, res.ID, res.Name)
				fmt.Fprintf(&s, "        Evidence: %s\n\n", res.Evidence)
			}
		}
	}

	s.WriteString("\n [r] Refresh Data  |  [q] Quit TUI\n")
	return s.String()
}

// refreshMsg triggers UI updates.
type refreshMsg struct {
	pools     []model.DevicePool
	devices   []model.Device
	claims    []model.ResourceClaimInfo
	docReport model.DoctorReport
	err       error
}

func (m modelState) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		pools, devices, claims, err := discovery.DiscoverDRA(ctx, m.clientset)
		if err != nil {
			return refreshMsg{err: err}
		}

		docRegistry := doctor.NewRegistry()
		report := docRegistry.RunDiagnostics(ctx, m.clientset)

		return refreshMsg{
			pools:     pools,
			devices:   devices,
			claims:    claims,
			docReport: report,
		}
	}
}

// RunTUI launches the Bubble Tea TUI.
func RunTUI(clientset *kubernetes.Clientset) error {
	p := tea.NewProgram(NewModel(clientset))
	_, err := p.Run()
	return err
}
