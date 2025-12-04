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
	header       *tview.TextView
	footer       *tview.TextView
	modeDrop     *tview.DropDown
	dbField      *tview.InputField
	userField    *tview.InputField
	oauthField   *tview.InputField
	channelField *tview.InputField
	logSink      *tuiLogSink
	shortcutLine string

	mu          sync.Mutex
	store       *QuoteStore
	lastCount   int
	lastRefresh time.Time
	ctx         context.Context
}

// runTUI launches an interactive terminal UI for configuring and monitoring the quote bot.
// It renders a form for Twitch/DB settings, persists updates to go-quote.config.json,
// tails logs, and polls the database for health and activity changes.
func runTUI(ctx context.Context, cfg AppConfig) error {
	app := tview.NewApplication()
	header := buildHeaderBar()
	status := buildStatusView()
	logView := buildLogView()
	shortcutLine := formatShortcutLine()
	footer := buildFooterBar(shortcutLine)

	tui := &tuiApp{
		app:          app,
		config:       cfg,
		header:       header,
		status:       status,
		logView:      logView,
		footer:       footer,
		shortcutLine: shortcutLine,
		lastCount:    -1,
		ctx:          ctx,
	}

	tui.logSink = newTUILogSink(app, logView, 400)
	log.SetOutput(io.MultiWriter(os.Stderr, tui.logSink))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	form := tui.buildForm()

	right := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(status, 0, 1, false).
		AddItem(logView, 0, 2, false)

	body := tview.NewGrid().
		SetRows(0).
		SetColumns(48, 1, 0).
		AddItem(form, 0, 0, 1, 1, 0, 0, true).
		AddItem(tview.NewBox(), 0, 1, 1, 1, 0, 0, false).
		AddItem(right, 0, 2, 1, 1, 0, 0, false)

	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(header, 3, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(footer, 2, 0, false)

	app.SetInputCapture(tui.captureKeys)

	go tui.healthLoop()
	tui.logf("TUI started (mode=%s, db=%s, channel=%s)", cfg.Mode, cfg.DBPath, cfg.TwitchChannel)
	tui.renderHeader(cfg, -1, nil)
	tui.flashFooter("Ready. Tab through fields, Ctrl+S to save, Ctrl+R to refresh.")

	if err := app.SetRoot(root, true).EnableMouse(true).Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}
	tui.closeStore()
	return nil
}

func (t *tuiApp) captureKeys(event *tcell.EventKey) *tcell.EventKey {
	switch {
	case event.Key() == tcell.KeyCtrlC || event.Key() == tcell.KeyCtrlQ || event.Rune() == 'q':
		t.flashFooter("Exiting...")
		t.app.Stop()
		return nil
	case event.Key() == tcell.KeyCtrlS:
		t.saveConfig()
		return nil
	case event.Key() == tcell.KeyCtrlR:
		go t.refreshHealth()
		t.flashFooter("Manual health refresh.")
		return nil
	case event.Key() == tcell.KeyCtrlL:
		t.app.SetFocus(t.logView)
		t.flashFooter("Focused logs. Use PgUp/PgDn to scroll.")
		return nil
	case event.Key() == tcell.KeyCtrlF:
		t.app.SetFocus(t.modeDrop)
		t.flashFooter("Focused form. Use Tab/Shift+Tab to move.")
		return nil
	}
	return event
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
		t.flashFooter("Failed to save config. Check file permissions.")
		return
	}

	t.mu.Lock()
	t.config = cfg
	t.lastCount = -1
	t.closeStoreLocked()
	t.mu.Unlock()
	t.renderHeader(cfg, t.lastCount, nil)

	t.logf("Config saved to %s (mode=%s, channel=%s, db=%s)", configFileName, cfg.Mode, cfg.TwitchChannel, cfg.DBPath)
	t.flashFooter("Config saved. Refreshing health...")
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
	refreshedAt := time.Now()
	t.mu.Lock()
	t.lastRefresh = refreshedAt
	t.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("[::b]Connection[::-]\n")
	sb.WriteString(fmt.Sprintf("• Mode: [white]%s[-]\n", strings.ToLower(cfg.Mode)))
	sb.WriteString(fmt.Sprintf("• DB: [white]%s[-]\n", cfg.DBPath))
	if cfg.Mode == "twitch" {
		sb.WriteString(fmt.Sprintf("• Twitch: [white]%s[-] @ #%s\n", emptyPlaceholder(cfg.TwitchUser), emptyPlaceholder(cfg.TwitchChannel)))
	}

	sb.WriteString("\n[::b]Health[::-]\n")
	if err != nil {
		sb.WriteString(fmt.Sprintf("[red::b]Database error[-]: %v\n", err))
		sb.WriteString("Check the DB path, permissions, or try a manual refresh (Ctrl+R).\n")
	} else {
		sb.WriteString(fmt.Sprintf("[green]OK[-] • Quotes: %d\n", max(count, 0)))
		if latest != nil {
			sb.WriteString(fmt.Sprintf("Latest: #%d by %s at %s\n", latest.ID, latest.Author, latest.CreatedAt.Format(time.RFC822)))
		} else {
			sb.WriteString("No quotes stored yet.\n")
		}
	}
	sb.WriteString(fmt.Sprintf("\nChecked at %s • auto refresh every 3s\n", refreshedAt.Format("15:04:05")))

	text := sb.String()
	t.app.QueueUpdateDraw(func() {
		t.status.SetText(text)
	})
	t.renderHeader(cfg, count, latest)
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

func buildHeaderBar() *tview.TextView {
	view := tview.NewTextView()
	view.SetDynamicColors(true)
	view.SetWrap(true)
	view.SetTextAlign(tview.AlignLeft)
	view.SetBorder(false)
	view.SetBackgroundColor(tcell.ColorDarkCyan)
	view.SetTextColor(tcell.ColorWhite)
	return view
}

func buildFooterBar(shortcuts string) *tview.TextView {
	view := tview.NewTextView()
	view.SetDynamicColors(true)
	view.SetWrap(false)
	view.SetTextAlign(tview.AlignLeft)
	view.SetBorder(false)
	view.SetBackgroundColor(tcell.ColorDarkGray)
	view.SetTextColor(tcell.ColorWhite)
	view.SetText(fmt.Sprintf("%s\n Ready.", shortcuts))
	return view
}

func formatShortcutLine() string {
	shortcuts := []struct {
		key   string
		label string
	}{
		{"^S", "Save"},
		{"^R", "Refresh"},
		{"^L", "Focus Logs"},
		{"^F", "Focus Form"},
		{"^Q", "Quit"},
	}
	var parts []string
	for _, sc := range shortcuts {
		parts = append(parts, fmt.Sprintf("[black:lightgray] %s [-:-] %s", sc.key, sc.label))
	}
	return strings.Join(parts, "  ")
}

func (t *tuiApp) flashFooter(msg string) {
	t.app.QueueUpdateDraw(func() {
		t.footer.SetText(fmt.Sprintf("%s\n %s", t.shortcutLine, msg))
	})
}

func (t *tuiApp) renderHeader(cfg AppConfig, count int, latest *Quote) {
	var latestLine string
	switch {
	case latest != nil:
		latestLine = fmt.Sprintf("#%d %s - %s", latest.ID, truncate(latest.Text, 32), latest.Author)
	case count >= 0:
		latestLine = fmt.Sprintf("%d stored quotes", count)
	default:
		latestLine = "warming up..."
	}

	t.mu.Lock()
	refreshed := t.lastRefresh
	t.mu.Unlock()
	lastTick := "not yet"
	if !refreshed.IsZero() {
		lastTick = refreshed.Format("15:04:05")
	}

	header := fmt.Sprintf(
		"[black:lightcyan] GO-QUOTE TUI [-:-]  [white::b]Mode[-]: %s   [white::b]DB[-]: %s\n[white::b]Channel[-]: #%s   [white::b]Last refresh[-]: %s   [white::b]Latest[-]: %s",
		strings.ToUpper(cfg.Mode),
		cfg.DBPath,
		emptyPlaceholder(cfg.TwitchChannel),
		lastTick,
		latestLine,
	)
	t.app.QueueUpdateDraw(func() {
		t.header.SetText(header)
	})
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
