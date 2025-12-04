package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type tuiApp struct {
	app          *tview.Application
	config       AppConfig
	status       *tview.TextView
	logView      *tview.TextView
	modeDrop     *tview.DropDown
	dbField      *tview.InputField
	userField    *tview.InputField
	oauthField   *tview.InputField
	channelField *tview.InputField
	logSink      *tuiLogSink

	mu        sync.Mutex
	store     *QuoteStore
	lastCount int
	ctx       context.Context
}

// runTUI launches an interactive terminal UI for configuring and monitoring the quote bot.
// It renders a form for Twitch/DB settings, persists updates to go-quote.config.json,
// tails logs, and polls the database for health and activity changes.
func runTUI(ctx context.Context, cfg AppConfig) error {
	app := tview.NewApplication()
	status := buildStatusView()
	logView := buildLogView()

	tui := &tuiApp{
		app:       app,
		config:    cfg,
		status:    status,
		logView:   logView,
		lastCount: -1,
		ctx:       ctx,
	}

	tui.logSink = newTUILogSink(app, logView, 400)
	log.SetOutput(io.MultiWriter(os.Stderr, tui.logSink))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	form := tui.buildForm()

	right := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(status, 0, 1, false).
		AddItem(logView, 0, 2, false)

	root := tview.NewFlex().
		AddItem(form, 0, 1, true).
		AddItem(right, 0, 2, false)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyCtrlC, event.Rune() == 'q':
			app.Stop()
			return nil
		case event.Rune() == 'r':
			go tui.refreshHealth()
			return nil
		}
		return event
	})

	go tui.healthLoop()
	tui.logf("TUI started (mode=%s, db=%s, channel=%s)", cfg.Mode, cfg.DBPath, cfg.TwitchChannel)

	if err := app.SetRoot(root, true).EnableMouse(true).Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}
	tui.closeStore()
	return nil
}

func (t *tuiApp) healthLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-t.ctx.Done():
			t.app.Stop()
			return
		case <-ticker.C:
			t.refreshHealth()
		}
	}
}

func (t *tuiApp) buildForm() *tview.Form {
	modeIdx := 0
	modes := []string{"tui", "twitch", "cli"}
	for i, val := range modes {
		if strings.EqualFold(val, t.config.Mode) {
			modeIdx = i
			break
		}
	}

	t.modeDrop = tview.NewDropDown().
		SetLabel("Mode").
		SetOptions(modes, func(option string, _ int) {
			t.mu.Lock()
			t.config.Mode = strings.ToLower(option)
			t.mu.Unlock()
		})
	t.modeDrop.SetCurrentOption(modeIdx)

	form := tview.NewForm().
		AddFormItem(t.modeDrop)

	t.dbField = tview.NewInputField().SetLabel("DB Path").SetText(t.config.DBPath).SetFieldWidth(40)
	t.userField = tview.NewInputField().SetLabel("Twitch User").SetText(t.config.TwitchUser).SetFieldWidth(40)
	t.oauthField = tview.NewInputField().SetLabel("Twitch OAuth").SetText(t.config.TwitchOAuth).SetMaskCharacter('*').SetFieldWidth(40)
	t.channelField = tview.NewInputField().SetLabel("Twitch Channel").SetText(t.config.TwitchChannel).SetFieldWidth(40)

	form.
		AddFormItem(t.dbField).
		AddFormItem(t.userField).
		AddFormItem(t.oauthField).
		AddFormItem(t.channelField).
		AddButton("Save config", t.saveConfig).
		AddButton("Refresh health", func() { go t.refreshHealth() }).
		AddButton("Quit", func() { t.app.Stop() })

	form.SetBorder(true).SetTitle(" Setup ").SetTitleAlign(tview.AlignLeft)
	form.SetButtonsAlign(tview.AlignLeft)
	form.SetBorderPadding(1, 1, 2, 2)
	return form
}

func (t *tuiApp) saveConfig() {
	cfg := t.collectConfig()
	if err := saveConfigFile(configFileName, cfg); err != nil {
		t.logf("Failed saving config: %v", err)
		return
	}

	t.mu.Lock()
	t.config = cfg
	t.lastCount = -1
	t.closeStoreLocked()
	t.mu.Unlock()

	t.logf("Config saved to %s (mode=%s, channel=%s, db=%s)", configFileName, cfg.Mode, cfg.TwitchChannel, cfg.DBPath)
	go t.refreshHealth()
}

func (t *tuiApp) collectConfig() AppConfig {
	_, mode := t.modeDrop.GetCurrentOption()
	cfg := AppConfig{
		Mode:          strings.ToLower(mode),
		DBPath:        strings.TrimSpace(t.dbField.GetText()),
		TwitchUser:    strings.TrimSpace(t.userField.GetText()),
		TwitchOAuth:   strings.TrimSpace(t.oauthField.GetText()),
		TwitchChannel: strings.TrimSpace(t.channelField.GetText()),
	}
	return cfg
}

func (t *tuiApp) refreshHealth() {
	healthCtx, cancel := context.WithTimeout(t.ctx, 4*time.Second)
	defer cancel()

	cfg := t.collectConfig()
	store, err := t.ensureStore(healthCtx, cfg.DBPath)
	if err != nil {
		t.renderStatus(cfg, -1, nil, err)
		return
	}

	count, err := store.Count(healthCtx)
	if err != nil {
		t.renderStatus(cfg, -1, nil, err)
		return
	}

	var latest *Quote
	if q, err := store.Latest(healthCtx); err == nil {
		latest = q
	}

	t.renderStatus(cfg, count, latest, nil)
	t.detectChanges(count, latest)
}

func (t *tuiApp) renderStatus(cfg AppConfig, count int, latest *Quote, err error) {
	var sb strings.Builder
	sb.WriteString("[::b]Setup & Health[::-]\n")
	sb.WriteString(fmt.Sprintf("Mode: %s\n", strings.ToLower(cfg.Mode)))
	sb.WriteString(fmt.Sprintf("DB: %s\n", cfg.DBPath))
	if cfg.Mode == "twitch" {
		sb.WriteString(fmt.Sprintf("Twitch: %s @ #%s\n", emptyPlaceholder(cfg.TwitchUser), emptyPlaceholder(cfg.TwitchChannel)))
	}
	if err != nil {
		sb.WriteString(fmt.Sprintf("[red]Database error: %v[-]\n", err))
	} else {
		sb.WriteString(fmt.Sprintf("[green]Database healthy[-] | Quotes: %d\n", max(count, 0)))
		if latest != nil {
			sb.WriteString(fmt.Sprintf("Latest: #%d by %s at %s\n", latest.ID, latest.Author, latest.CreatedAt.Format(time.RFC822)))
		}
	}
	sb.WriteString("\nKeys: [q] quit  [r] refresh  [Save config] button\n")

	text := sb.String()
	t.app.QueueUpdateDraw(func() {
		t.status.SetText(text)
	})
}

func (t *tuiApp) detectChanges(count int, latest *Quote) {
	t.mu.Lock()
	prev := t.lastCount
	t.lastCount = count
	t.mu.Unlock()

	if prev >= 0 && count != prev {
		t.logf("Quote count changed: %d -> %d", prev, count)
		if latest != nil {
			t.logf("Latest quote #%d \"%s\" - %s", latest.ID, truncate(latest.Text, 60), latest.Author)
		}
	}
}

func (t *tuiApp) ensureStore(ctx context.Context, dbPath string) (*QuoteStore, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.store != nil && t.config.DBPath == dbPath {
		return t.store, nil
	}

	if t.store != nil {
		t.store.Close()
		t.store = nil
	}

	store, err := NewQuoteStore(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	t.store = store
	t.config.DBPath = dbPath
	return store, nil
}

func (t *tuiApp) closeStore() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeStoreLocked()
}

func (t *tuiApp) closeStoreLocked() {
	if t.store != nil {
		_ = t.store.Close()
		t.store = nil
	}
}

func (t *tuiApp) logf(format string, args ...any) {
	log.Printf(format, args...)
}

type tuiLogSink struct {
	app   *tview.Application
	view  *tview.TextView
	limit int

	mu    sync.Mutex
	lines []string
}

func newTUILogSink(app *tview.Application, view *tview.TextView, limit int) *tuiLogSink {
	return &tuiLogSink{
		app:   app,
		view:  view,
		limit: limit,
	}
}

func (l *tuiLogSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, line := range strings.Split(string(p), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		l.lines = append(l.lines, trimmed)
		if len(l.lines) > l.limit {
			diff := len(l.lines) - l.limit
			l.lines = l.lines[diff:]
		}
	}

	text := strings.Join(l.lines, "\n")
	l.app.QueueUpdateDraw(func() {
		l.view.SetText(text)
	})

	return len(p), nil
}

func buildStatusView() *tview.TextView {
	view := tview.NewTextView()
	view.SetDynamicColors(true)
	view.SetWrap(true)
	view.SetBorder(true)
	view.SetTitle(" Health ")
	view.SetTitleAlign(tview.AlignLeft)
	view.SetText("Checking database status...")
	return view
}

func buildLogView() *tview.TextView {
	view := tview.NewTextView()
	view.SetDynamicColors(true)
	view.SetScrollable(true)
	view.SetWrap(false)
	view.SetBorder(true)
	view.SetTitle(" Logs ")
	view.SetTitleAlign(tview.AlignLeft)
	view.SetChangedFunc(func() {
		view.ScrollToEnd()
	})
	return view
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func emptyPlaceholder(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<not set>"
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
