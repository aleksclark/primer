package validatecmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type activityItem struct {
	result Result
}

func (a activityItem) Title() string {
	status := okStyle.Render("PASS")
	if !a.result.OK {
		status = errStyle.Render("FAIL")
	}
	title := a.result.Title
	if title == "" {
		title = a.result.Slug
	}
	return fmt.Sprintf("%s  %s", status, title)
}

func (a activityItem) Description() string {
	if !a.result.OK {
		return a.result.Error
	}
	return fmt.Sprintf("%s · %d tasks · %d checks · %d fixtures",
		a.result.Slug, a.result.Tasks, a.result.Checks, a.result.Fixtures)
}

func (a activityItem) FilterValue() string {
	return a.result.Slug + " " + a.result.Title
}

type tuiModel struct {
	list     list.Model
	results  []Result
	width    int
	height   int
	quitting bool
}

// RunTUI validates activities and shows a Bubble Tea list of pass/fail rows.
func RunTUI(opts Options) error {
	opts.Stdout = nil
	opts.Stderr = nil
	if !opts.Materialize {
		opts.Materialize = true
	}
	results, err := Run(opts)
	if err != nil {
		return err
	}
	items := make([]list.Item, 0, len(results))
	for _, r := range results {
		items = append(items, activityItem{result: r})
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.SetSpacing(1)
	l := list.New(items, delegate, 80, 20)
	l.Title = "Activity validation"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
		}
	}
	m := tuiModel{list: l, results: results}
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(tuiModel); ok {
		_ = fm
	}
	if !AllOK(results) {
		return fmt.Errorf("validation failed for %d activit(ies)", failCount(results))
	}
	return nil
}

func failCount(results []Result) int {
	n := 0
	for _, r := range results {
		if !r.OK {
			n++
		}
	}
	return n
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h := msg.Height - 4
		if h < 6 {
			h = 6
		}
		m.list.SetSize(msg.Width-2, h)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}
	passed := 0
	for _, r := range m.results {
		if r.OK {
			passed++
		}
	}
	header := titleStyle.Render("Primer activity-validate")
	summary := fmt.Sprintf("%d/%d passed", passed, len(m.results))
	if passed == len(m.results) && len(m.results) > 0 {
		summary = okStyle.Render(summary)
	} else {
		summary = errStyle.Render(summary)
	}
	body := m.list.View()
	footer := helpStyle.Render("q quit · / filter")
	return strings.Join([]string{header + "  " + summary, "", body, footer}, "\n")
}
