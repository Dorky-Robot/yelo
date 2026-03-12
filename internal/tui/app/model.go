package app

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dorkyrobot/yelo/internal/aws"
	"github.com/dorkyrobot/yelo/internal/config"
	"github.com/dorkyrobot/yelo/internal/output"
	"github.com/dorkyrobot/yelo/internal/state"
)

type viewMode int

const (
	modeBrowser viewMode = iota
	modeBuckets
	modeHelp
)

// Model is the top-level bubbletea model.
type Model struct {
	cfg    *config.Config
	state  *state.State
	client aws.S3Client

	// Navigation
	bucket string
	prefix string
	items  []aws.ObjectInfo

	// UI state
	cursor  int
	offset  int // scroll offset
	detail  *aws.ObjectInfo
	mode    viewMode
	filter  string
	filtering bool

	// Bucket picker
	bucketList   []string
	bucketCursor int

	// Async state
	loading       bool
	loadingDetail bool
	spinner       spinner.Model

	// Flash message
	flash string

	// Terminal size
	width  int
	height int
}

func NewModel(cfg *config.Config, st *state.State, client aws.S3Client, bucket string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorTitle)

	return Model{
		cfg:    cfg,
		state:  st,
		client: client,
		bucket: bucket,
		prefix: st.Prefix,
		spinner: s,
		loading: true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchList(m.client, m.bucket, m.prefix),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case listResultMsg:
		m.loading = false
		if msg.err != nil {
			m.flash = fmt.Sprintf("error: %v", msg.err)
			return m, clearFlashAfter(5 * time.Second)
		}
		m.items = msg.items
		m.cursor = 0
		m.offset = 0
		m.detail = nil
		m.loadingDetail = false
		// Auto-fetch detail for first item
		if cmd := m.fetchCurrentDetail(); cmd != nil {
			m.loadingDetail = true
			return m, cmd
		}
		return m, nil

	case detailResultMsg:
		m.loadingDetail = false
		if msg.err != nil {
			m.detail = nil
			return m, nil
		}
		m.detail = msg.info
		return m, nil

	case bucketsResultMsg:
		if msg.err != nil {
			m.flash = fmt.Sprintf("error: %v", msg.err)
			return m, clearFlashAfter(5 * time.Second)
		}
		m.bucketList = msg.buckets
		m.bucketCursor = 0
		m.mode = modeBuckets
		return m, nil

	case downloadCompleteMsg:
		if msg.err != nil {
			m.flash = fmt.Sprintf("download failed: %v", msg.err)
		} else {
			m.flash = fmt.Sprintf("downloaded %s -> %s", msg.key, msg.localPath)
		}
		return m, clearFlashAfter(5 * time.Second)

	case restoreCompleteMsg:
		if msg.err != nil {
			m.flash = fmt.Sprintf("restore failed: %v", msg.err)
		} else {
			m.flash = fmt.Sprintf("restore initiated: %s", msg.key)
		}
		return m, clearFlashAfter(5 * time.Second)

	case flashMsg:
		m.flash = string(msg)
		return m, clearFlashAfter(5 * time.Second)

	case clearFlashMsg:
		m.flash = ""
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Filtering mode
	if m.filtering {
		switch {
		case key.Matches(msg, keys.Escape), key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.filtering = false
			return m, nil
		case msg.Type == tea.KeyBackspace:
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.cursor = 0
				m.offset = 0
			}
			return m, nil
		case msg.Type == tea.KeyRunes:
			m.filter += string(msg.Runes)
			m.cursor = 0
			m.offset = 0
			return m, nil
		}
		return m, nil
	}

	// Bucket picker mode
	if m.mode == modeBuckets {
		return m.handleBucketKey(msg)
	}

	// Help overlay
	if m.mode == modeHelp {
		m.mode = modeBrowser
		return m, nil
	}

	// Browser mode
	switch {
	case key.Matches(msg, keys.Quit):
		m.saveState()
		return m, tea.Quit

	case key.Matches(msg, keys.Up):
		filtered := m.filteredItems()
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
			return m, m.onCursorChange(filtered)
		}

	case key.Matches(msg, keys.Down):
		filtered := m.filteredItems()
		if m.cursor < len(filtered)-1 {
			m.cursor++
			m.ensureVisible()
			return m, m.onCursorChange(filtered)
		}

	case key.Matches(msg, keys.Enter):
		return m.handleEnter()

	case key.Matches(msg, keys.Back):
		return m.navigateUp()

	case key.Matches(msg, keys.Get):
		return m.handleGet()

	case key.Matches(msg, keys.Restore):
		return m.handleRestore()

	case key.Matches(msg, keys.Buckets):
		return m, fetchBuckets(m.client)

	case key.Matches(msg, keys.Refresh):
		m.loading = true
		m.filter = ""
		return m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))

	case key.Matches(msg, keys.Search):
		m.filtering = true
		m.filter = ""
		return m, nil

	case key.Matches(msg, keys.Help):
		m.mode = modeHelp
		return m, nil
	}

	return m, nil
}

func (m Model) handleBucketKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Quit):
		m.mode = modeBrowser
		return m, nil

	case key.Matches(msg, keys.Up):
		if m.bucketCursor > 0 {
			m.bucketCursor--
		}

	case key.Matches(msg, keys.Down):
		if m.bucketCursor < len(m.bucketList)-1 {
			m.bucketCursor++
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if m.bucketCursor < len(m.bucketList) {
			selected := m.bucketList[m.bucketCursor]
			if selected != m.bucket {
				m.bucket = selected
				m.prefix = ""
				m.mode = modeBrowser
				m.loading = true
				m.filter = ""
				// Recreate client if region/profile differ
				return m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))
			}
			m.mode = modeBrowser
		}
	}

	return m, nil
}

func (m *Model) handleEnter() (Model, tea.Cmd) {
	filtered := m.filteredItems()
	if m.cursor >= len(filtered) {
		return *m, nil
	}

	item := filtered[m.cursor]
	if item.IsPrefix {
		// Navigate into prefix
		m.prefix = item.Key
		m.loading = true
		m.filter = ""
		m.detail = nil
		return *m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))
	}

	return *m, nil
}

func (m *Model) navigateUp() (Model, tea.Cmd) {
	if m.prefix == "" {
		return *m, nil
	}
	// Go up one level
	m.prefix = parentPrefix(m.prefix)
	m.loading = true
	m.filter = ""
	m.detail = nil
	return *m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))
}

func (m *Model) handleGet() (Model, tea.Cmd) {
	filtered := m.filteredItems()
	if m.cursor >= len(filtered) {
		return *m, nil
	}

	item := filtered[m.cursor]
	if item.IsPrefix {
		return *m, nil
	}

	// Check Glacier
	if isGlacier(item.StorageClass) {
		if m.detail != nil && m.detail.RestoreStatus != "available" {
			m.flash = "object is in Glacier; restore it first (r)"
			return *m, clearFlashAfter(3 * time.Second)
		}
	}

	m.flash = fmt.Sprintf("downloading %s...", path.Base(item.Key))
	return *m, downloadObject(m.client, m.bucket, item.Key)
}

func (m *Model) handleRestore() (Model, tea.Cmd) {
	filtered := m.filteredItems()
	if m.cursor >= len(filtered) {
		return *m, nil
	}

	item := filtered[m.cursor]
	if item.IsPrefix {
		return *m, nil
	}

	if !isGlacier(item.StorageClass) {
		m.flash = "not a Glacier object"
		return *m, clearFlashAfter(3 * time.Second)
	}

	m.flash = fmt.Sprintf("restoring %s...", path.Base(item.Key))
	return *m, restoreObject(m.client, m.bucket, item.Key, 7, "Standard")
}

func (m Model) fetchCurrentDetail() tea.Cmd {
	filtered := m.filteredItems()
	if m.cursor >= len(filtered) {
		return nil
	}
	item := filtered[m.cursor]
	if item.IsPrefix {
		return nil
	}
	return fetchDetail(m.client, m.bucket, item.Key)
}

func (m Model) onCursorChange(filtered []aws.ObjectInfo) tea.Cmd {
	if m.cursor >= len(filtered) {
		return nil
	}
	item := filtered[m.cursor]
	if item.IsPrefix {
		m.detail = nil
		return nil
	}
	m.loadingDetail = true
	return fetchDetail(m.client, m.bucket, item.Key)
}

func (m Model) filteredItems() []aws.ObjectInfo {
	if m.filter == "" {
		return m.items
	}
	var result []aws.ObjectInfo
	lower := strings.ToLower(m.filter)
	for _, item := range m.items {
		name := displayName(item.Key, m.prefix)
		if strings.Contains(strings.ToLower(name), lower) {
			result = append(result, item)
		}
	}
	return result
}

func (m *Model) ensureVisible() {
	listHeight := m.listHeight()
	if listHeight <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+listHeight {
		m.offset = m.cursor - listHeight + 1
	}
}

func (m Model) listHeight() int {
	// header(1) + top border(1) + bottom border(1) + statusbar(1) = 4 lines of chrome
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) saveState() {
	m.state.Bucket = m.bucket
	m.state.Prefix = m.prefix
	_ = m.state.Save()
}

// View renders the full UI.
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	switch m.mode {
	case modeBuckets:
		return m.viewBuckets()
	case modeHelp:
		return m.viewHelp()
	default:
		return m.viewBrowser()
	}
}

func (m Model) viewBrowser() string {
	// Header
	location := m.bucket + ":/" + m.prefix
	header := headerStyle.Width(m.width).Render(
		fmt.Sprintf(" yelo  %s", location),
	)

	// Calculate panel widths
	detailWidth := 40
	if m.width < 80 {
		detailWidth = 0 // collapse detail on narrow terminals
	}
	listWidth := m.width - detailWidth
	if detailWidth > 0 {
		listWidth -= 1 // gap between panels
	}

	contentHeight := m.height - 3 // header + status bar

	// Left panel: file browser
	listPanel := m.renderList(listWidth, contentHeight)

	// Right panel: detail
	var content string
	if detailWidth > 0 {
		detailPanel := m.renderDetail(detailWidth, contentHeight)
		content = lipgloss.JoinHorizontal(lipgloss.Top, listPanel, " ", detailPanel)
	} else {
		content = listPanel
	}

	// Status bar
	statusBar := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, statusBar)
}

func (m Model) renderList(width, height int) string {
	if height < 1 {
		height = 1
	}

	var b strings.Builder
	filtered := m.filteredItems()

	if m.loading {
		line := fmt.Sprintf(" %s loading...", m.spinner.View())
		b.WriteString(line)
		b.WriteString("\n")
		for i := 1; i < height; i++ {
			b.WriteString("\n")
		}
		return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
	}

	if len(filtered) == 0 {
		msg := " (empty)"
		if m.filter != "" {
			msg = fmt.Sprintf(" no matches for %q", m.filter)
		}
		b.WriteString(dimStyle.Render(msg))
		b.WriteString("\n")
		for i := 1; i < height; i++ {
			b.WriteString("\n")
		}
		return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
	}

	// Add ".." entry if we have a prefix
	showParent := m.prefix != ""

	visibleStart := m.offset
	visibleEnd := m.offset + height
	totalItems := len(filtered)
	if showParent {
		totalItems++
	}
	if visibleEnd > totalItems {
		visibleEnd = totalItems
	}

	for i := visibleStart; i < visibleEnd; i++ {
		var line string
		isCursor := i == m.cursor

		if showParent && i == 0 {
			// The ".." entry — but we handle this differently
			// Actually, let's prepend ".." into the display
			line = m.renderParentEntry(width, isCursor)
		} else {
			idx := i
			if showParent {
				idx = i - 1
			}
			if idx < len(filtered) {
				line = m.renderItem(filtered[idx], width, isCursor)
			}
		}
		b.WriteString(line)
		if i < visibleEnd-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines
	rendered := strings.Count(b.String(), "\n") + 1
	for rendered < height {
		b.WriteString("\n")
		rendered++
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

func (m Model) renderParentEntry(width int, selected bool) string {
	name := " ../"
	if selected {
		return selectedStyle.Width(width).Render(">" + name)
	}
	return prefixStyle.Width(width).Render(" " + name)
}

func (m Model) renderItem(item aws.ObjectInfo, width int, selected bool) string {
	name := displayName(item.Key, m.prefix)

	if item.IsPrefix {
		display := " " + name
		if selected {
			return selectedStyle.Width(width).Render(">" + display)
		}
		return prefixStyle.Width(width).Render(" " + display)
	}

	// Object: name + class badge + size
	classBadge := storageClassStyle(item.StorageClass)
	size := output.FormatSize(item.Size)

	// Calculate available space for name
	// Format: " name    CLASS  size"
	nameWidth := width - 18 // rough space for class + size + padding
	if nameWidth < 10 {
		nameWidth = 10
	}
	if len(name) > nameWidth {
		name = name[:nameWidth-1] + "~"
	}

	rightPart := fmt.Sprintf(" %s  %7s", classBadge, size)
	padding := width - lipgloss.Width(name) - lipgloss.Width(rightPart) - 2
	if padding < 1 {
		padding = 1
	}

	line := fmt.Sprintf(" %s%s%s", name, strings.Repeat(" ", padding), rightPart)

	if selected {
		return selectedStyle.Render(">") + lipgloss.NewStyle().Width(width - 1).Render(line)
	}
	return " " + lipgloss.NewStyle().Width(width - 1).Render(line)
}

func (m Model) renderDetail(width, height int) string {
	var b strings.Builder

	if m.loadingDetail {
		b.WriteString(fmt.Sprintf(" %s", m.spinner.View()))
		for i := 1; i < height; i++ {
			b.WriteString("\n")
		}
		return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
	}

	if m.detail == nil {
		filtered := m.filteredItems()
		if m.cursor < len(filtered) && filtered[m.cursor].IsPrefix {
			b.WriteString(dimStyle.Render(" (directory)"))
		}
		for i := 1; i < height; i++ {
			b.WriteString("\n")
		}
		return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
	}

	obj := m.detail
	lines := []string{
		fmt.Sprintf("%s  %s", labelStyle.Render("Key"), path.Base(obj.Key)),
		fmt.Sprintf("%s  %s (%d B)", labelStyle.Render("Size"), output.FormatSize(obj.Size), obj.Size),
		fmt.Sprintf("%s  %s", labelStyle.Render("Class"), storageClassStyle(obj.StorageClass)),
		fmt.Sprintf("%s  %s", labelStyle.Render("Modified"), obj.LastModified),
	}
	if obj.ContentType != "" {
		lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render("Type"), obj.ContentType))
	}
	if obj.ETag != "" {
		etag := obj.ETag
		if len(etag) > width-14 {
			etag = etag[:width-17] + "..."
		}
		lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render("ETag"), etag))
	}
	lines = append(lines, fmt.Sprintf("%s  %s", labelStyle.Render("Restore"), restoreStatusStyle(obj.RestoreStatus)))

	for _, line := range lines {
		b.WriteString(" " + line + "\n")
	}

	// Pad
	rendered := len(lines)
	for rendered < height {
		b.WriteString("\n")
		rendered++
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

func (m Model) renderStatusBar() string {
	if m.flash != "" {
		return statusBarStyle.Width(m.width).Render(m.flash)
	}

	if m.filtering {
		return statusBarStyle.Width(m.width).Render(
			fmt.Sprintf("filter: %s_", m.filter),
		)
	}

	hints := "j/k:nav  enter:open  h:back  g:get  r:restore  b:buckets  /:filter  R:refresh  ?:help  q:quit"
	return statusBarStyle.Width(m.width).Render(hints)
}

func (m Model) viewBuckets() string {
	var b strings.Builder
	title := headerStyle.Width(m.width).Render(" yelo  Select Bucket")
	b.WriteString(title)
	b.WriteString("\n")

	contentHeight := m.height - 3

	if len(m.bucketList) == 0 {
		b.WriteString(dimStyle.Render(" (no buckets found)"))
	} else {
		for i, bucket := range m.bucketList {
			if i >= contentHeight {
				break
			}
			marker := "  "
			if bucket == m.bucket {
				marker = "* "
			}
			line := fmt.Sprintf(" %s%s", marker, bucket)
			if i == m.bucketCursor {
				b.WriteString(selectedStyle.Render(">"+line) + "\n")
			} else {
				b.WriteString(" " + line + "\n")
			}
		}
	}

	// Pad
	lines := strings.Count(b.String(), "\n")
	for lines < m.height-1 {
		b.WriteString("\n")
		lines++
	}

	b.WriteString(statusBarStyle.Width(m.width).Render("j/k:nav  enter:select  esc:cancel"))

	return b.String()
}

func (m Model) viewHelp() string {
	var b strings.Builder
	title := headerStyle.Width(m.width).Render(" yelo  Help")
	b.WriteString(title)
	b.WriteString("\n\n")

	helpLines := []struct{ key, desc string }{
		{"j / k / arrows", "Navigate up/down"},
		{"enter / l", "Open prefix / select object"},
		{"backspace / h", "Go to parent directory"},
		{"g", "Download selected object"},
		{"r", "Initiate Glacier restore (Standard tier, 7 days)"},
		{"b", "Switch bucket"},
		{"R", "Refresh listing"},
		{"/", "Filter current listing"},
		{"esc", "Cancel filter / close overlay"},
		{"q / Ctrl+C", "Quit (saves state)"},
	}

	for _, h := range helpLines {
		k := lipgloss.NewStyle().Foreground(colorSelected).Width(18).Render(h.key)
		b.WriteString(fmt.Sprintf("  %s %s\n", k, h.desc))
	}

	// Pad
	lines := strings.Count(b.String(), "\n")
	for lines < m.height-1 {
		b.WriteString("\n")
		lines++
	}

	b.WriteString(statusBarStyle.Width(m.width).Render("Press any key to close"))

	return b.String()
}

// Helpers

func displayName(key, prefix string) string {
	name := strings.TrimPrefix(key, prefix)
	if name == "" {
		name = key
	}
	return name
}

func parentPrefix(prefix string) string {
	p := strings.TrimSuffix(prefix, "/")
	if p == "" {
		return ""
	}
	dir := path.Dir(p)
	if dir == "." || dir == "/" {
		return ""
	}
	return dir + "/"
}

func isGlacier(class string) bool {
	switch class {
	case "GLACIER", "DEEP_ARCHIVE", "GLACIER_IR":
		return true
	}
	return false
}
