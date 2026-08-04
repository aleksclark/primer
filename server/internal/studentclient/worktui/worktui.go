// Package worktui is a minimal Bubble Tea work-queue viewer used by Phase 1
// trifle e2e tests against a fake or in-process HTTP server.
package worktui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Options configure the stub TUI.
type Options struct {
	BaseURL     string
	DeviceToken string
	HTTPClient  *http.Client
}

type workItem struct {
	TitleText string
	State     string
	Slug      string
}

func (w workItem) Title() string       { return w.TitleText }
func (w workItem) Description() string { return fmt.Sprintf("%s · %s", w.Slug, w.State) }
func (w workItem) FilterValue() string { return w.TitleText + " " + w.Slug }

type model struct {
	list     list.Model
	opts     Options
	err      string
	loaded   bool
	quitting bool
	width    int
	height   int
}

type loadedMsg struct {
	items []list.Item
	err   error
}

// NewModel builds the root model.
func NewModel(opts Options) model {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 80, 20)
	l.Title = "Student work queue"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false)
	return model{list: l, opts: opts}
}

func (m model) Init() tea.Cmd {
	return m.fetch
}

func (m model) fetch() tea.Msg {
	client := m.opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(m.opts.BaseURL, "/")+"/student/work", nil)
	if err != nil {
		return loadedMsg{err: err}
	}
	req.Header.Set("Authorization", "Bearer "+m.opts.DeviceToken)
	resp, err := client.Do(req)
	if err != nil {
		return loadedMsg{err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return loadedMsg{err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return loadedMsg{err: fmt.Errorf("work status %d: %s", resp.StatusCode, string(body))}
	}
	var payload struct {
		Items []struct {
			Assignment struct {
				State string `json:"state"`
			} `json:"assignment"`
			Activity struct {
				Title string `json:"title"`
				Slug  string `json:"slug"`
			} `json:"activity"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return loadedMsg{err: err}
	}
	items := make([]list.Item, 0, len(payload.Items))
	for _, it := range payload.Items {
		title := it.Activity.Title
		if title == "" {
			title = it.Activity.Slug
		}
		items = append(items, workItem{
			TitleText: title,
			State:     it.Assignment.State,
			Slug:      it.Activity.Slug,
		})
	}
	return loadedMsg{items: items}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)
	case loadedMsg:
		m.loaded = true
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.list.SetItems(msg.items)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	if m.err != "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("error: "+m.err) + "\n(q to quit)\n"
	}
	if !m.loaded {
		return "Loading work queue…\n"
	}
	if len(m.list.Items()) == 0 {
		return m.list.View() + "\nNo assignments.\n"
	}
	return m.list.View()
}

// Run starts the TUI (blocking).
func Run(opts Options) error {
	p := tea.NewProgram(NewModel(opts))
	_, err := p.Run()
	return err
}
