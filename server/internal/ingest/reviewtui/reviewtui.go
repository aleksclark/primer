// Package reviewtui is a small Bubble Tea UI for answering content-review.yaml.
// For each unresolved entry the operator picks a candidate (or skips); chosen
// TMDB/TVDB ids are written back so the next resolve pass can apply them.
package reviewtui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aleksclark/primer/server/internal/ingest/manifest"
)

// Result summarizes one interactive session.
type Result struct {
	// Chosen is how many entries received a candidate pick this session.
	Chosen int
	// Skipped is how many entries were left unanswered.
	Skipped int
	// QuitEarly is true when the operator quit before finishing the queue.
	QuitEarly bool
	// Review is the (possibly updated) review document. Save it with
	// manifest.SaveReview when Chosen > 0.
	Review *manifest.Review
}

// pending are entries that still need a human pick.
func pending(r *manifest.Review) []int {
	var idxs []int
	if r == nil {
		return idxs
	}
	for i, e := range r.Entries {
		if e.ChosenTMDB == 0 && e.ChosenTVDB == 0 {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

// Run launches the interactive picker. path is only used in the header.
func Run(review *manifest.Review, path string) (*Result, error) {
	if review == nil {
		review = &manifest.Review{}
	}
	queue := pending(review)
	if len(queue) == 0 {
		return &Result{Review: review}, nil
	}

	m := newModel(review, queue, path)
	prog := tea.NewProgram(m, tea.WithAltScreen())
	final, err := prog.Run()
	if err != nil {
		return nil, err
	}
	out, ok := final.(model)
	if !ok {
		return nil, fmt.Errorf("unexpected tui model type %T", final)
	}
	return &Result{
		Chosen:    out.chosen,
		Skipped:   out.skipped,
		QuitEarly: out.quitEarly,
		Review:    out.review,
	}, nil
}

// candidateItem adapts a manifest.Candidate for bubbles/list.
type candidateItem struct {
	cand   manifest.Candidate
	idx    int
	kind   string // movie|series — decides which id is primary
	chosen bool
}

func (c candidateItem) Title() string {
	title := c.cand.Title
	if c.cand.Year > 0 {
		title = fmt.Sprintf("%s (%d)", title, c.cand.Year)
	}
	if c.chosen {
		return "★ " + title
	}
	return "  " + title
}

func (c candidateItem) Description() string {
	var ids []string
	if c.cand.TVDB != 0 {
		ids = append(ids, fmt.Sprintf("tvdb=%d", c.cand.TVDB))
	}
	if c.cand.TMDB != 0 {
		ids = append(ids, fmt.Sprintf("tmdb=%d", c.cand.TMDB))
	}
	line := strings.Join(ids, "  ")
	if c.cand.Overview != "" {
		ov := c.cand.Overview
		if len(ov) > 120 {
			ov = ov[:117] + "…"
		}
		if line != "" {
			line += "  ·  "
		}
		line += ov
	}
	if line == "" {
		return "(no provider ids)"
	}
	return line
}

func (c candidateItem) FilterValue() string {
	return c.cand.Title
}

type keyMap struct {
	Select key.Binding
	Skip   key.Binding
	Quit   key.Binding
	Next   key.Binding
	Prev   key.Binding
}

var keys = keyMap{
	Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose")),
	Skip:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "skip")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "save & quit")),
	Next:   key.NewBinding(key.WithKeys("n", "tab"), key.WithHelp("n", "next entry")),
	Prev:   key.NewBinding(key.WithKeys("p", "shift+tab"), key.WithHelp("p", "prev entry")),
}

type model struct {
	review    *manifest.Review
	queue     []int // indices into review.Entries
	pos       int   // position in queue
	list      list.Model
	path      string
	width     int
	height    int
	chosen    int
	skipped   int
	quitEarly bool
	done      bool
}

func newModel(review *manifest.Review, queue []int, path string) model {
	m := model{
		review: review,
		queue:  queue,
		path:   path,
	}
	m.list = m.buildList()
	return m
}

func (m model) entry() *manifest.ReviewEntry {
	if m.pos < 0 || m.pos >= len(m.queue) {
		return nil
	}
	return &m.review.Entries[m.queue[m.pos]]
}

func (m model) buildList() list.Model {
	e := m.entry()
	items := []list.Item{}
	if e != nil {
		for i, c := range e.Candidates {
			items = append(items, candidateItem{
				cand:   c,
				idx:    i,
				kind:   e.Kind,
				chosen: (c.TMDB != 0 && c.TMDB == e.ChosenTMDB) || (c.TVDB != 0 && c.TVDB == e.ChosenTVDB),
			})
		}
	}
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("229")).
		BorderForeground(lipgloss.Color("57"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("252")).
		BorderForeground(lipgloss.Color("57"))
	delegate.SetHeight(2)
	delegate.SetSpacing(1)

	l := list.New(items, delegate, 80, 16)
	l.Title = ""
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Select, keys.Skip, keys.Next, keys.Prev, keys.Quit}
	}
	l.AdditionalFullHelpKeys = l.AdditionalShortHelpKeys
	if e != nil && len(e.Candidates) == 0 {
		l.SetShowPagination(false)
	}
	return l
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		header := 8
		footer := 2
		h := msg.Height - header - footer
		if h < 6 {
			h = 6
		}
		m.list.SetSize(msg.Width-2, h)
		return m, nil

	case tea.KeyMsg:
		// When filtering, let the list own most keys.
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitEarly = m.pos < len(m.queue)
			m.done = true
			return m, tea.Quit

		case key.Matches(msg, keys.Select):
			if item, ok := m.list.SelectedItem().(candidateItem); ok {
				m.applyChoice(item.cand)
				m.chosen++
				return m.advance()
			}
			// No candidates: treat enter as skip.
			m.skipped++
			return m.advance()

		case key.Matches(msg, keys.Skip):
			m.skipped++
			return m.advance()

		case key.Matches(msg, keys.Next):
			if m.pos+1 < len(m.queue) {
				m.pos++
				m.list = m.buildList()
				m.list.SetSize(max(m.width-2, 20), max(m.height-10, 6))
			}
			return m, nil

		case key.Matches(msg, keys.Prev):
			if m.pos > 0 {
				m.pos--
				m.list = m.buildList()
				m.list.SetSize(max(m.width-2, 20), max(m.height-10, 6))
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) advance() (tea.Model, tea.Cmd) {
	if m.pos+1 >= len(m.queue) {
		m.done = true
		return m, tea.Quit
	}
	m.pos++
	m.list = m.buildList()
	m.list.SetSize(max(m.width-2, 20), max(m.height-10, 6))
	return m, nil
}

func (m *model) applyChoice(c manifest.Candidate) {
	e := m.entry()
	if e == nil {
		return
	}
	// Prefer the id that matches the kind; fall back to whichever is set.
	switch e.Kind {
	case manifest.KindSeries:
		e.ChosenTVDB = c.TVDB
		e.ChosenTMDB = c.TMDB
		if e.ChosenTVDB == 0 && c.TMDB != 0 {
			e.ChosenTMDB = c.TMDB
		}
	case manifest.KindMovie:
		e.ChosenTMDB = c.TMDB
		e.ChosenTVDB = c.TVDB
	default:
		e.ChosenTMDB = c.TMDB
		e.ChosenTVDB = c.TVDB
	}
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Padding(0, 1)
	metaStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	headerStyle = lipgloss.NewStyle().MarginBottom(1)
)

func (m model) View() string {
	if m.done {
		return ""
	}
	e := m.entry()
	if e == nil {
		return "No pending review entries.\n"
	}

	progress := fmt.Sprintf("%d / %d", m.pos+1, len(m.queue))
	head := titleStyle.Render(" content-ingest review ")
	head += "  " + metaStyle.Render(progress)
	if m.path != "" {
		head += "  " + metaStyle.Render(m.path)
	}

	wantYear := ""
	if e.Year > 0 {
		wantYear = fmt.Sprintf(" (%d)", e.Year)
	}
	info := fmt.Sprintf("%s%s  ·  %s  ·  id=%s", e.Title, wantYear, e.Kind, e.ID)
	if e.Reason != "" {
		info += "  ·  " + e.Reason
	}

	var body string
	if len(e.Candidates) == 0 {
		body = warnStyle.Render("No candidates from lookup. Press enter/s to skip, or q to quit.") +
			"\n" + metaStyle.Render("Look the title up yourself and edit review.yaml by hand if needed.")
	} else {
		body = m.list.View()
	}

	help := metaStyle.Render("enter choose · s skip · n/p next/prev · / filter · q save & quit")
	status := ""
	if m.chosen > 0 || m.skipped > 0 {
		status = okStyle.Render(fmt.Sprintf("chosen %d · skipped %d this session", m.chosen, m.skipped))
	}

	return headerStyle.Render(head) + "\n" +
		lipgloss.NewStyle().Bold(true).Render(info) + "\n\n" +
		body + "\n" +
		help + "\n" + status
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
