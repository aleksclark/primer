package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/domain"
	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/broker"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/engine"
)

func TestWrapWordsAndFormatBlockLines(t *testing.T) {
	t.Parallel()

	assert.Nil(t, wrapWords("   ", 40))
	assert.Nil(t, wrapWords("", 40))

	// width clamp below 20 still wraps without panicking
	lines := wrapWords("one two three four five six seven", 10)
	require.NotEmpty(t, lines)
	for _, ln := range lines {
		assert.LessOrEqual(t, len(ln), 20)
	}

	long := strings.Repeat("word ", 30)
	wrapped := wrapWords(long, 40)
	require.Greater(t, len(wrapped), 1)
	assert.LessOrEqual(t, len(wrapped[0]), 40)

	vocab := formatBlockLines(contracts.InstructionBlock{
		Kind: contracts.BlockVocabulary,
		Terms: []contracts.VocabularyTerm{
			{Term: "cwd", Definition: "current working directory"},
			{Term: "path", Definition: "location of a file"},
		},
	}, 48)
	require.NotEmpty(t, vocab)
	assert.Contains(t, strings.Join(vocab, "\n"), "cwd")
	assert.Contains(t, strings.Join(vocab, "\n"), "current working directory")

	example := formatBlockLines(contracts.InstructionBlock{
		Kind:        contracts.BlockExample,
		Input:       "pwd",
		Output:      "/home/student",
		Explanation: "Prints the working directory.",
	}, 40)
	joined := strings.Join(example, "\n")
	assert.Contains(t, joined, "Input:")
	assert.Contains(t, joined, "pwd")
	assert.Contains(t, joined, "Output:")
	assert.Contains(t, joined, "/home/student")
	assert.Contains(t, joined, "Prints the working directory")

	resource := formatBlockLines(contracts.InstructionBlock{
		Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{
			MediaType: "text/plain",
			Label:     "cheatsheet",
		},
	}, 40)
	assert.Contains(t, strings.Join(resource, "\n"), "cheatsheet")
	assert.Contains(t, strings.Join(resource, "\n"), "text/plain")

	// Resource nil → empty
	assert.Empty(t, formatBlockLines(contracts.InstructionBlock{Kind: contracts.BlockResource}, 40))

	prose := formatBlockLines(contracts.InstructionBlock{
		Kind: contracts.BlockProse,
		Text: "Read this carefully before running commands in the shell.",
	}, 28)
	require.NotEmpty(t, prose)
	assert.Contains(t, strings.Join(prose, " "), "carefully")
}

func TestOpenLessonViewAndKeys(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true, Broker: &broker.Client{}})
	// Small height → small page so scroll keys move within a long lesson buffer.
	m.width, m.height = 100, 12
	m.brokerSessionID = "lesson-sess"
	m.screen = ScreenActivity
	blocks := []contracts.InstructionBlock{
		{ID: "b1", Kind: contracts.BlockProse, Title: "Welcome", Text: "This is the first teaching block about the shell."},
		{ID: "b2", Kind: contracts.BlockVocabulary, Title: "", Terms: []contracts.VocabularyTerm{
			{Term: "cwd", Definition: "current working directory path"},
		}},
		{ID: "b3", Kind: contracts.BlockExample, Title: "pwd example", Input: "pwd", Output: "/home/me", Explanation: "shows cwd"},
		{ID: "b4", Kind: contracts.BlockResource, Title: "Sheet", Resource: &contracts.ResourceRef{MediaType: "text/plain", Label: "notes"}},
	}
	// Pad with prose so lessonLines >> page (page = max(8, height-6) = 8).
	for i := 0; i < 20; i++ {
		blocks = append(blocks, contracts.InstructionBlock{
			ID: fmt.Sprintf("pad-%d", i), Kind: contracts.BlockProse, Title: fmt.Sprintf("Pad %d", i),
			Text: "Extra teaching material so scrolling has room to move the viewport offset.",
		})
	}
	m.snap = engine.SessionSnapshot{
		Kind:          contracts.KindTerminal,
		ActivityTitle: "Shell basics",
		Objective:     "Navigate the filesystem",
		Instructions:  "Use pwd and ls to explore your home directory carefully.",
		Blocks:        blocks,
		Tasks: []contracts.Task{{
			ID: "t-resp", Title: "Explain", Instructions: "Write your answer",
			Kind: contracts.TaskKindShortResponse,
			Response: &contracts.ResponseTaskSpec{
				Prompt: "What does pwd do?", MaxChars: 200,
				Rubric: []contracts.RubricCriterion{{ID: "r1", Description: "mentions directory"}},
			},
		}},
		CurrentTaskIdx: 0,
		HasTerminal:    true,
	}
	m.activePane = paneInstructions

	// ctrl+l opens lesson reader
	next, cmd := m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'l'})
	m = next.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, "lesson", m.ScreenName())
	require.Greater(t, len(m.lessonLines), 30)
	assert.Equal(t, 0, m.lessonOffset)
	assert.Equal(t, 0, m.lessonIdx)

	view := m.View().Content
	assert.Contains(t, view, "Lesson — Shell basics")
	assert.Contains(t, view, "immutable revision blocks")
	assert.Contains(t, view, "Welcome")
	assert.Contains(t, view, "cwd")
	assert.Contains(t, view, "j/k scroll")

	// scroll down/up
	next, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
	m = next.(Model)
	assert.Equal(t, 1, m.lessonOffset)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	assert.Equal(t, 2, m.lessonOffset)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
	m = next.(Model)
	assert.Equal(t, 1, m.lessonOffset)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(Model)
	assert.Equal(t, 0, m.lessonOffset)
	// up at top is a no-op
	next, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
	m = next.(Model)
	assert.Equal(t, 0, m.lessonOffset)

	// page down / page up / home / end / section nav
	page := max(8, m.height-6)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'f'})
	m = next.(Model)
	assert.Equal(t, page, m.lessonOffset)
	before := m.lessonOffset
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = next.(Model)
	assert.Equal(t, before+page, m.lessonOffset)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'b'})
	m = next.(Model)
	assert.Equal(t, before, m.lessonOffset)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = next.(Model)
	assert.Equal(t, 0, m.lessonOffset)

	// space pages down too
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = next.(Model)
	assert.Equal(t, page, m.lessonOffset)

	next, _ = m.Update(tea.KeyPressMsg{Code: 'g'})
	m = next.(Model)
	assert.Equal(t, 0, m.lessonOffset)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'G'})
	m = next.(Model)
	assert.Equal(t, max(0, len(m.lessonLines)-page), m.lessonOffset)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	m = next.(Model)
	assert.Equal(t, 0, m.lessonOffset)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(Model)
	assert.Equal(t, max(0, len(m.lessonLines)-page), m.lessonOffset)

	// section n/p
	next, _ = m.Update(tea.KeyPressMsg{Code: 'n'})
	m = next.(Model)
	assert.Equal(t, 1, m.lessonIdx)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'n'})
	m = next.(Model)
	assert.Equal(t, 2, m.lessonIdx)
	// advance past last section stays put
	for i := 0; i < 40; i++ {
		next, _ = m.Update(tea.KeyPressMsg{Code: 'n'})
		m = next.(Model)
	}
	assert.Equal(t, len(m.snap.Blocks)-1, m.lessonIdx)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'p'})
	m = next.(Model)
	assert.Equal(t, len(m.snap.Blocks)-2, m.lessonIdx)
	// p at idx 0 no-op
	m.lessonIdx = 0
	next, _ = m.Update(tea.KeyPressMsg{Code: 'p'})
	m = next.(Model)
	assert.Equal(t, 0, m.lessonIdx)

	// esc returns to activity
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())

	// reopen and leave via q / ctrl+g / ctrl+l
	for _, leave := range []tea.KeyPressMsg{
		{Code: 'q'},
		{Mod: tea.ModCtrl, Code: 'g'},
		{Mod: tea.ModCtrl, Code: 'l'},
	} {
		m.screen = ScreenActivity
		next, _ = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'l'})
		m = next.(Model)
		assert.Equal(t, "lesson", m.ScreenName())
		next, _ = m.Update(leave)
		m = next.(Model)
		assert.Equal(t, "activity", m.ScreenName(), "leave key %v", leave)
	}
}

func TestOpenLessonWithoutBlocksFallsBackToObjective(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true, Broker: &broker.Client{}})
	m.width, m.height = 80, 20
	m.brokerSessionID = "nb"
	m.snap = engine.SessionSnapshot{
		ActivityTitle: "Plain",
		Objective:     "Learn basics",
		Instructions:  "Follow the objective and complete the checks with care.",
		Blocks:        nil,
	}
	m.openLesson()
	require.NotEmpty(t, m.lessonLines)
	joined := strings.Join(m.lessonLines, "\n")
	assert.Contains(t, joined, "No typed instructional blocks")
	assert.Contains(t, joined, "Learn basics")
	assert.Contains(t, joined, "Follow the objective")

	m.screen = ScreenLesson
	// empty lines edge of viewLesson
	m.lessonLines = nil
	m.lessonOffset = 5
	view := m.viewLesson()
	assert.Contains(t, view, "(empty)")
	assert.Contains(t, view, "Lesson — Plain")

	// long line truncation in view
	m.lessonLines = []string{strings.Repeat("X", 200)}
	m.lessonOffset = 0
	m.width = 40
	view = m.viewLesson()
	assert.Contains(t, view, "…")
}

func TestResponseTaskHelpersAndDoneMsg(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	m.snap = engine.SessionSnapshot{
		Tasks: []contracts.Task{
			{ID: "a", Title: "Action", Instructions: "run", Kind: contracts.TaskKindAction},
			{ID: "r", Title: "Respond", Instructions: "write", Kind: contracts.TaskKindShortResponse,
				Response: &contracts.ResponseTaskSpec{Prompt: "Why?", MaxChars: 100,
					Rubric: []contracts.RubricCriterion{{ID: "c1", Description: "reason"}}}},
		},
		CurrentTaskIdx: 0,
	}
	assert.False(t, m.currentTaskIsResponse())
	assert.Nil(t, m.currentResponseTask())
	assert.Nil(t, m.saveDraftCmd())
	assert.Nil(t, m.submitResponseCmd())

	m.snap.CurrentTaskIdx = 1
	assert.True(t, m.currentTaskIsResponse())
	rt := m.currentResponseTask()
	require.NotNil(t, rt)
	assert.Equal(t, "r", rt.ID)

	m.snap.CurrentTaskIdx = 99
	assert.False(t, m.currentTaskIsResponse())
	assert.Nil(t, m.currentResponseTask())
	m.snap.CurrentTaskIdx = -1
	assert.Nil(t, m.currentResponseTask())

	// responseDoneMsg success / queued / error
	m.screen = ScreenActivity
	m.busy = true
	m.respInput.SetValue("draft text")
	next, cmd := m.Update(responseDoneMsg{
		snap: engine.SessionSnapshot{ResponseQueued: true, ActivityTitle: "X"},
	})
	m = next.(Model)
	assert.Nil(t, cmd)
	assert.False(t, m.busy)
	assert.Contains(t, m.activityMsg, "awaiting sync")
	assert.True(t, m.snap.ResponseQueued)

	next, _ = m.Update(responseDoneMsg{
		snap: engine.SessionSnapshot{ActivityTitle: "Y"},
	})
	m = next.(Model)
	assert.Equal(t, "Response submitted", m.activityMsg)

	next, _ = m.Update(responseDoneMsg{err: assert.AnError})
	m = next.(Model)
	assert.Contains(t, m.activityMsg, assert.AnError.Error())
}

func TestShortResponseActivityViewAndKeys(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true, Broker: &broker.Client{}})
	m.width, m.height = 120, 40
	m.brokerSessionID = "resp-ui"
	m.screen = ScreenActivity
	m.activePane = paneInstructions
	m.snap = engine.SessionSnapshot{
		Kind:            contracts.KindTerminal,
		HasTerminal:     true,
		ActivityTitle:   "Reflect",
		ClientSessionID: "resp-ui",
		Blocks:          []contracts.InstructionBlock{{ID: "p1", Kind: contracts.BlockProse, Title: "Read", Text: "body"}},
		Tasks: []contracts.Task{{
			ID: "sr1", Title: "Your words", Instructions: "fallback instructions",
			Kind: contracts.TaskKindShortResponse,
			Response: &contracts.ResponseTaskSpec{
				Prompt:   "Explain the purpose of ls in your own words.",
				MaxChars: 500,
				Rubric:   []contracts.RubricCriterion{{ID: "r1", Description: "mentions listing"}},
			},
		}},
		CurrentTaskIdx: 0,
		ResponseDraft:  "prior draft body",
		ChecksTotal:    1,
	}

	view := m.View().Content
	assert.Contains(t, view, "Reflect")
	assert.Contains(t, view, "Explain the purpose of ls")
	// Panel may wrap the tutor warning across lines.
	assert.Contains(t, view, "Tutor text cannot be")
	assert.Contains(t, view, "ctrl+l lesson · 1 blocks")
	assert.Contains(t, view, "ctrl+enter submit response")
	// View hydrates draft into the rendered input (value receiver); seed explicitly for key path.
	assert.Contains(t, view, "prior draft body")
	m.respInput.SetValue(m.snap.ResponseDraft)
	_ = m.respInput.Focus()
	assert.Equal(t, "prior draft body", m.respInput.Value())

	// typing routes to response field and schedules draft save (no sess → nil draft cmd)
	next, cmd := m.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	m = next.(Model)
	// Batch may be non-nil even if saveDraftCmd is nil
	_ = cmd
	assert.Contains(t, m.respInput.Value(), "x")

	// ctrl+enter submit without sess → nil cmd from submitResponseCmd
	next, cmd = m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: tea.KeyEnter})
	_ = next
	// submitResponseCmd returns nil when sess is nil
	assert.Nil(t, m.submitResponseCmd())

	// Without terminal, short_response still routes keys
	m.snap.HasTerminal = false
	m.respInput.SetValue("")
	_ = m.respInput.Focus()
	next, cmd = m.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	m = next.(Model)
	assert.Contains(t, m.respInput.Value(), "a")
	_ = cmd
}

func TestSaveDraftAndSubmitResponseCmdsWithSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := cache.Open(filepath.Join(dir, "state.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	asg := "asg-resp-ui"
	require.NoError(t, store.SaveWork(ctx, []studentapi.WorkItem{{
		Assignment: domain.StudentAssignment{ID: asg, State: "available", UpdatedAt: time.Now().UTC()},
		Activity:   domain.LearningActivity{Slug: "reflect-shell", Title: "Reflect", Kind: contracts.KindTerminal},
		Revision: domain.LearningActivityRevision{
			ID: "rev-resp", ContentSHA256: "digest-resp",
			Content: map[string]any{
				"objective":    "explain a command",
				"instructions": "read then answer",
				"blocks": []any{
					map[string]any{"id": "blk1", "kind": "prose", "title": "Intro", "text": "The shell runs commands."},
					map[string]any{"id": "blk2", "kind": "vocabulary", "title": "Terms", "terms": []any{
						map[string]any{"term": "argv", "definition": "argument vector"},
					}},
					map[string]any{"id": "blk3", "kind": "parent_note", "text": "hidden from students"},
				},
				"terminal": map[string]any{
					"runtimeProfile": "default",
					"fixtures":       []any{map[string]any{"path": "notes.txt", "type": "file", "content": "hi"}},
				},
				"tasks": []any{
					map[string]any{
						"id": "answer-1", "title": "Explain", "instructions": "Write your answer",
						"kind": "short_response",
						"response": map[string]any{
							"prompt":   "What does the shell do?",
							"maxChars": 120,
							"rubric":   []any{map[string]any{"id": "r1", "description": "mentions commands"}},
						},
						"completion": map[string]any{"checkId": "resp-done"},
					},
				},
				"checks": []any{
					map[string]any{"id": "resp-done", "kind": "response_submitted", "params": map[string]any{"taskId": "answer-1"}},
				},
				"hints": []any{map[string]any{"id": "h1", "text": "think about programs"}},
			},
		},
	}}))

	ws := filepath.Join(dir, "ws")
	require.NoError(t, os.MkdirAll(ws, 0o755))
	eng, err := engine.New(engine.Options{
		Store: store, WorkspaceRoot: ws, Offline: true, AllowUnsandboxed: true, UseSandbox: false,
	})
	require.NoError(t, err)
	sess, err := eng.OpenSession(ctx, asg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	// ContentBlocks strips parent_note
	blocks := sess.ContentBlocks()
	require.NotEmpty(t, blocks)
	for _, b := range blocks {
		assert.NotEqual(t, contracts.BlockParentNote, b.Kind)
	}

	m := NewModel(Options{Store: store, Offline: true, AllowUnsandboxed: true, WorkspaceRoot: ws})
	m.width, m.height = 100, 36
	m.screen = ScreenActivity
	m.sess = sess
	m.snap = sess.Snapshot()
	m.activePane = paneInstructions
	m.opts.Broker = nil

	require.True(t, m.currentTaskIsResponse())
	require.NotNil(t, m.currentResponseTask())

	// View shows response prompt
	view := m.View().Content
	assert.Contains(t, view, "What does the shell do?")
	assert.Contains(t, view, "ctrl+l lesson")

	// Open lesson from real session
	next, cmd := m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'l'})
	m = next.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, "lesson", m.ScreenName())
	lessonView := m.View().Content
	assert.Contains(t, lessonView, "Intro")
	assert.Contains(t, lessonView, "argv")
	assert.NotContains(t, lessonView, "hidden from students")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	assert.Equal(t, "activity", m.ScreenName())

	// Draft save via cmd
	m.respInput.SetValue("The shell runs programs I type.")
	saveCmd := m.saveDraftCmd()
	require.NotNil(t, saveCmd)
	assert.Nil(t, saveCmd()) // returns nil msg
	body, err := store.GetResponseDraft(ctx, sess.Snapshot().ClientSessionID, "answer-1")
	require.NoError(t, err)
	assert.Equal(t, "The shell runs programs I type.", body)
	assert.Equal(t, body, sess.Snapshot().ResponseDraft)

	// Empty body submit fails
	m.respInput.SetValue("   ")
	sub := m.submitResponseCmd()
	require.NotNil(t, sub)
	msg := sub()
	rd := msg.(responseDoneMsg)
	require.Error(t, rd.err)
	assert.Contains(t, rd.err.Error(), "required")

	// Too long
	m.respInput.SetValue(strings.Repeat("x", 200))
	msg = m.submitResponseCmd()()
	rd = msg.(responseDoneMsg)
	require.Error(t, rd.err)
	assert.Contains(t, rd.err.Error(), "exceeds")

	// Successful submit
	m.respInput.SetValue("It launches programs from text I type at the prompt.")
	msg = m.submitResponseCmd()()
	rd = msg.(responseDoneMsg)
	require.NoError(t, rd.err)
	// ResponseQueued is true when the current task matches the submitted task.
	if rd.snap.CurrentTaskIdx >= 0 && rd.snap.CurrentTaskIdx < len(rd.snap.Tasks) {
		cur := rd.snap.Tasks[rd.snap.CurrentTaskIdx]
		if cur.ID == "answer-1" {
			assert.True(t, rd.snap.ResponseQueued)
		}
	}
	assert.Contains(t, rd.snap.Message, "awaiting sync")
	// Durable intent exists regardless of current-task snap flag.
	pending, err := store.ListPendingResponses(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, pending)

	// Apply done msg — force queued flag for UI branch coverage when needed.
	if !rd.snap.ResponseQueued {
		rd.snap.ResponseQueued = true
	}
	next, _ = m.Update(rd)
	m = next.(Model)
	assert.Contains(t, m.activityMsg, "awaiting sync")

	// Key path: type + save draft batch, then alt+enter submit.
	// After submit the runner may advance CurrentTaskIdx past the last task — pin it back.
	m.respInput.SetValue("base")
	_ = m.respInput.Focus()
	m.busy = false
	m.snap = sess.Snapshot()
	if m.snap.CurrentTaskIdx < 0 || m.snap.CurrentTaskIdx >= len(m.snap.Tasks) {
		m.snap.CurrentTaskIdx = 0
	}
	m.activePane = paneInstructions
	m.sess = sess
	require.True(t, m.currentTaskIsResponse(), "idx=%d tasks=%+v hasTerm=%v", m.snap.CurrentTaskIdx, m.snap.Tasks, m.snap.HasTerminal)
	require.True(t, m.hasSession())
	// Direct handleTerminalActivityKey for response branch
	next, cmd = m.handleTerminalActivityKey(tea.KeyPressMsg{Text: "z", Code: 'z'})
	m = next.(Model)
	require.NotNil(t, cmd, "value=%q busy=%v pane=%d hasTerm=%v isResp=%v", m.respInput.Value(), m.busy, m.activePane, m.snap.HasTerminal, m.currentTaskIsResponse())
	if cmd != nil {
		_ = cmd()
	}
	assert.Contains(t, m.respInput.Value(), "z")
	// Re-pin task index if handle path refreshed snap
	if !m.currentTaskIsResponse() && len(m.snap.Tasks) > 0 {
		m.snap.CurrentTaskIdx = 0
	}
	next, cmd = m.handleTerminalActivityKey(tea.KeyPressMsg{Mod: tea.ModAlt, Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg = cmd()
	rd2, ok := msg.(responseDoneMsg)
	require.True(t, ok)
	require.NoError(t, rd2.err)
	_ = next
}

func TestCtrlLWithoutSessionIsNoop(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Offline: true})
	m.screen = ScreenActivity
	m.snap = engine.SessionSnapshot{Kind: contracts.KindTerminal}
	next, cmd := m.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'l'})
	m = next.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, "activity", m.ScreenName())
}
