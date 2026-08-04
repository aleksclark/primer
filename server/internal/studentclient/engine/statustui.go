package engine

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusModel is a small Bubble Tea view of harness Status.
type StatusModel struct {
	status   Status
	quitting bool
	title    string
}

// NewStatusModel builds a status TUI model.
func NewStatusModel(title string, st Status) StatusModel {
	if title == "" {
		title = "Primer student harness"
	}
	return StatusModel{title: title, status: st}
}

// SetStatus updates the displayed status (for non-interactive refresh).
func (m *StatusModel) SetStatus(st Status) { m.status = st }

func (m StatusModel) Init() tea.Cmd { return nil }

func (m StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "enter":
			m.quitting = true
			return m, tea.Quit
		}
	case Status:
		m.status = msg
	}
	return m, nil
}

func (m StatusModel) View() string {
	if m.quitting {
		return ""
	}
	st := m.status
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Phase:        %s\n", st.Phase)
	fmt.Fprintf(&b, "Sync:         %s\n", st.Sync)
	fmt.Fprintf(&b, "Work items:   %d\n", st.WorkDownloaded)
	if st.ActivitySlug != "" {
		fmt.Fprintf(&b, "Activity:     %s\n", st.ActivitySlug)
	}
	if st.AssignmentID != "" {
		fmt.Fprintf(&b, "Assignment:   %s\n", shortID(st.AssignmentID))
	}
	fmt.Fprintf(&b, "Commands run: %d\n", st.CommandsRun)
	fmt.Fprintf(&b, "Checks:       %d/%d passed\n", st.ChecksPassed, st.ChecksTotal)
	if st.RequiredPassed {
		b.WriteString(okStyle.Render("Required checks: PASS"))
	} else if st.ChecksTotal > 0 {
		b.WriteString(warnStyle.Render("Required checks: incomplete"))
	} else {
		b.WriteString("Required checks: —")
	}
	b.WriteString("\n")
	if st.CompletionAcked {
		b.WriteString(okStyle.Render("Completion: synced"))
	} else if st.CompletionQueued {
		b.WriteString(warnStyle.Render("Completion: awaiting sync"))
	} else {
		b.WriteString("Completion: —")
	}
	b.WriteString("\n")
	if st.Offline {
		b.WriteString(warnStyle.Render("Mode: offline"))
		b.WriteString("\n")
	}
	if st.Message != "" {
		fmt.Fprintf(&b, "Message: %s\n", st.Message)
	}
	if st.LastError != "" {
		fmt.Fprintf(&b, "%s\n", errStyle.Render("Error: "+st.LastError))
	}
	b.WriteString("\n(q to quit)\n")
	return b.String()
}

// RunStatusTUI blocks rendering status until quit.
func RunStatusTUI(title string, st Status) error {
	p := tea.NewProgram(NewStatusModel(title, st))
	_, err := p.Run()
	return err
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8] + "…"
	}
	return id
}
