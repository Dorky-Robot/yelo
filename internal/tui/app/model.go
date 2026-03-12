package app

import (
	"fmt"
	"path"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dorkyrobot/yelo/internal/aws"
	"github.com/dorkyrobot/yelo/internal/config"
	"github.com/dorkyrobot/yelo/internal/output"
	"github.com/dorkyrobot/yelo/internal/state"
)

// Braille spinner frames.
var spinner = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// tab identifies the active tab.
type tab int

const (
	tabBrowse tab = iota
	tabCredentials
)

// mode is the state machine for the TUI.
type mode int

const (
	modeNormal mode = iota
	modeHelp
	modeBucketPicker
	modeConfirm
	modeFilter
	modeDetail
	modeAddBucket   // multi-field form
	modeEditBucket  // multi-field form
)

// addField identifies which field is focused in add/edit forms.
type addField int

const (
	fieldName addField = iota
	fieldRegion
	fieldProfile
)

// confirmAction identifies what the confirm dialog will do.
type confirmAction int

const (
	confirmDownload confirmAction = iota
	confirmRestore
	confirmRemoveBucket
)

// Model is the top-level bubbletea model.
type Model struct {
	// Dependencies
	cfg    *config.Config
	st     *state.State
	client aws.S3Client

	// Navigation
	bucket string
	prefix string
	items  []aws.ObjectInfo

	// Tab + mode state machine
	tab     tab
	mode    mode
	submenu bool

	// Browse state
	selected int
	filter   string

	// Detail pane (for modeDetail)
	detail *aws.ObjectInfo

	// Bucket picker
	bucketList   []string
	bucketCursor int

	// Confirm dialog
	confirmWhat   confirmAction
	confirmTarget string

	// Add/edit bucket form
	formField   addField
	formName    string
	formRegion  string
	formProfile string

	// Credentials tab
	profiles       []string
	profileCursor  int
	profileStatus  map[string]string // "ok", "fail", "testing"

	// Async
	loading    string // non-empty = show spinner, block input
	statusMsg  string
	spinnerTick int

	// Terminal
	width  int
	height int
}

// NewModel creates the initial TUI model.
func NewModel(cfg *config.Config, st *state.State, client aws.S3Client, bucket string) Model {
	return Model{
		cfg:           cfg,
		st:            st,
		client:        client,
		bucket:        bucket,
		prefix:        st.Prefix,
		tab:           tabBrowse,
		mode:          modeNormal,
		profileStatus: map[string]string{},
	}
}

func (m Model) Init() tea.Cmd {
	if m.bucket == "" {
		return fetchBuckets(m.client)
	}
	return fetchList(m.client, m.bucket, m.prefix)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case listResultMsg:
		m.loading = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, clearFlashAfter(5 * time.Second)
		}
		m.items = msg.items
		m.selected = 0
		m.detail = nil
		m.statusMsg = fmt.Sprintf("%d items", len(msg.items))
		return m, clearFlashAfter(3 * time.Second)

	case detailResultMsg:
		m.loading = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, clearFlashAfter(5 * time.Second)
		}
		m.detail = msg.info
		m.mode = modeDetail
		return m, nil

	case bucketsResultMsg:
		m.loading = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, clearFlashAfter(5 * time.Second)
		}
		m.bucketList = msg.buckets
		m.bucketCursor = 0
		// If no bucket set, force picker; otherwise show as overlay
		if m.bucket == "" {
			m.mode = modeBucketPicker
		} else {
			m.mode = modeBucketPicker
		}
		return m, nil

	case downloadCompleteMsg:
		m.loading = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Download failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Downloaded %s → %s", path.Base(msg.key), msg.localPath)
		}
		return m, clearFlashAfter(5 * time.Second)

	case restoreCompleteMsg:
		m.loading = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Restore failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Restore initiated: %s", path.Base(msg.key))
		}
		return m, clearFlashAfter(5 * time.Second)

	case profilesResultMsg:
		m.loading = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, clearFlashAfter(5 * time.Second)
		}
		m.profiles = msg.profiles
		m.profileCursor = 0
		return m, nil

	case profileTestMsg:
		if msg.ok {
			m.profileStatus[msg.profile] = "ok"
			m.statusMsg = fmt.Sprintf("Profile '%s' connected", msg.profile)
		} else {
			m.profileStatus[msg.profile] = "fail"
			m.statusMsg = fmt.Sprintf("Profile '%s' failed: %v", msg.profile, msg.err)
		}
		return m, clearFlashAfter(5 * time.Second)

	case clearFlashMsg:
		m.statusMsg = ""
		return m, nil

	case tea.KeyMsg:
		// Block input during loading
		if m.loading != "" {
			return m, nil
		}
		return m.handleKey(msg)
	}

	m.spinnerTick++
	return m, nil
}

// ---------------------------------------------------------------------------
// Key handling — dispatched by mode
// ---------------------------------------------------------------------------

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeHelp:
		if isKey(msg, "esc", "q") {
			m.mode = modeNormal
		}
		return m, nil

	case modeBucketPicker:
		return m.handleBucketPicker(msg)

	case modeConfirm:
		return m.handleConfirm(msg)

	case modeFilter:
		return m.handleFilter(msg)

	case modeDetail:
		return m.handleDetail(msg)

	case modeAddBucket, modeEditBucket:
		return m.handleBucketForm(msg)

	case modeNormal:
		switch m.tab {
		case tabBrowse:
			if m.submenu {
				return m.handleBrowseSubmenu(msg)
			}
			return m.handleBrowse(msg)
		case tabCredentials:
			if m.submenu {
				return m.handleCredentialsSubmenu(msg)
			}
			return m.handleCredentials(msg)
		}
	}

	return m, nil
}

func (m Model) handleBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredItems()
	switch {
	case isKey(msg, "q", "ctrl+c"):
		m.saveState()
		return m, tea.Quit
	case isKey(msg, "1"):
		m.tab = tabBrowse
		m.submenu = false
	case isKey(msg, "2"):
		m.tab = tabCredentials
		m.submenu = false
		m.loading = "Loading profiles..."
		return m, loadProfiles()
	case isKey(msg, "j", "down"):
		if m.selected < len(filtered)-1 {
			m.selected++
		}
	case isKey(msg, "k", "up"):
		if m.selected > 0 {
			m.selected--
		}
	case isKey(msg, "enter", "l"):
		if m.selected < len(filtered) {
			item := filtered[m.selected]
			if item.IsPrefix {
				m.prefix = item.Key
				m.selected = 0
				m.filter = ""
				m.loading = "Loading..."
				return m, fetchList(m.client, m.bucket, m.prefix)
			}
			// Object: show detail
			m.loading = "Loading metadata..."
			return m, fetchDetail(m.client, m.bucket, item.Key)
		}
	case isKey(msg, "h", "backspace"):
		if m.prefix != "" {
			m.prefix = parentPrefix(m.prefix)
			m.selected = 0
			m.filter = ""
			m.loading = "Loading..."
			return m, fetchList(m.client, m.bucket, m.prefix)
		}
	case isKey(msg, "g"):
		return m.beginDownload(filtered)
	case isKey(msg, "r"):
		return m.beginRestore(filtered)
	case isKey(msg, "b"):
		m.loading = "Loading buckets..."
		return m, fetchBuckets(m.client)
	case isKey(msg, "/"):
		m.mode = modeFilter
		m.filter = ""
	case isKey(msg, "."):
		m.submenu = true
	}
	return m, nil
}

func (m Model) handleBrowseSubmenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "."):
		m.submenu = false
	case isKey(msg, "R"):
		m.submenu = false
		m.loading = "Refreshing..."
		m.filter = ""
		return m, fetchList(m.client, m.bucket, m.prefix)
	case isKey(msg, "s"):
		filtered := m.filteredItems()
		if m.selected < len(filtered) && !filtered[m.selected].IsPrefix {
			m.loading = "Loading metadata..."
			m.submenu = false
			return m, fetchDetail(m.client, m.bucket, filtered[m.selected].Key)
		}
	case isKey(msg, "?"):
		m.mode = modeHelp
		m.submenu = false
	}
	return m, nil
}

func (m Model) handleCredentials(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	buckets := m.cfg.Buckets
	switch {
	case isKey(msg, "q", "ctrl+c"):
		m.saveState()
		return m, tea.Quit
	case isKey(msg, "1"):
		m.tab = tabBrowse
		m.submenu = false
	case isKey(msg, "2"):
		m.tab = tabCredentials
		m.submenu = false
	case isKey(msg, "j", "down"):
		if m.profileCursor < len(buckets)-1 {
			m.profileCursor++
		}
	case isKey(msg, "k", "up"):
		if m.profileCursor > 0 {
			m.profileCursor--
		}
	case isKey(msg, "a"):
		m.mode = modeAddBucket
		m.formField = fieldName
		m.formName = ""
		m.formRegion = ""
		m.formProfile = ""
	case isKey(msg, "t"):
		// Test the selected bucket's profile
		if m.profileCursor < len(buckets) {
			b := buckets[m.profileCursor]
			profile := b.Profile
			if profile == "" {
				profile = m.cfg.Profile
			}
			if profile == "" {
				profile = "default"
			}
			m.profileStatus[b.Name] = "testing"
			m.statusMsg = fmt.Sprintf("Testing '%s'...", b.Name)
			return m, testProfile(profile)
		}
	case isKey(msg, "d"):
		if m.profileCursor < len(buckets) {
			m.confirmWhat = confirmRemoveBucket
			m.confirmTarget = buckets[m.profileCursor].Name
			m.mode = modeConfirm
		}
	case isKey(msg, "."):
		m.submenu = true
	}
	return m, nil
}

func (m Model) handleCredentialsSubmenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	buckets := m.cfg.Buckets
	switch {
	case isKey(msg, "."):
		m.submenu = false
	case isKey(msg, "e"):
		if m.profileCursor < len(buckets) {
			b := buckets[m.profileCursor]
			m.mode = modeEditBucket
			m.formField = fieldName
			m.formName = b.Name
			m.formRegion = b.Region
			m.formProfile = b.Profile
			m.submenu = false
		}
	case isKey(msg, "D"):
		if m.profileCursor < len(buckets) {
			name := buckets[m.profileCursor].Name
			if err := m.cfg.SetDefault(name); err == nil {
				_ = m.cfg.Save()
				m.statusMsg = fmt.Sprintf("Default bucket set to '%s'", name)
			}
			m.submenu = false
			return m, clearFlashAfter(3 * time.Second)
		}
	case isKey(msg, "R"):
		m.submenu = false
		m.loading = "Loading profiles..."
		return m, loadProfiles()
	case isKey(msg, "?"):
		m.mode = modeHelp
		m.submenu = false
	}
	return m, nil
}

func (m Model) handleBucketPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "esc"):
		if m.bucket != "" {
			m.mode = modeNormal
		}
	case isKey(msg, "j", "down"):
		if m.bucketCursor < len(m.bucketList)-1 {
			m.bucketCursor++
		}
	case isKey(msg, "k", "up"):
		if m.bucketCursor > 0 {
			m.bucketCursor--
		}
	case isKey(msg, "enter"):
		if m.bucketCursor < len(m.bucketList) {
			m.bucket = m.bucketList[m.bucketCursor]
			m.prefix = ""
			m.selected = 0
			m.mode = modeNormal
			m.loading = "Loading..."
			return m, fetchList(m.client, m.bucket, m.prefix)
		}
	}
	return m, nil
}

func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "y", "Y"):
		m.mode = modeNormal
		switch m.confirmWhat {
		case confirmDownload:
			m.loading = fmt.Sprintf("Downloading %s...", path.Base(m.confirmTarget))
			return m, downloadObject(m.client, m.bucket, m.confirmTarget)
		case confirmRestore:
			m.loading = fmt.Sprintf("Restoring %s...", path.Base(m.confirmTarget))
			return m, restoreObject(m.client, m.bucket, m.confirmTarget, 7, "Standard")
		case confirmRemoveBucket:
			m.cfg.RemoveBucket(m.confirmTarget)
			_ = m.cfg.Save()
			m.statusMsg = fmt.Sprintf("Removed '%s'", m.confirmTarget)
			if m.profileCursor > 0 {
				m.profileCursor--
			}
			return m, clearFlashAfter(3 * time.Second)
		}
	case isKey(msg, "n", "N", "esc"):
		m.mode = modeNormal
	}
	return m, nil
}

func (m Model) handleFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "esc", "enter"):
		m.mode = modeNormal
	case msg.Type == tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.selected = 0
		}
	case msg.Type == tea.KeyRunes:
		m.filter += string(msg.Runes)
		m.selected = 0
	}
	return m, nil
}

func (m Model) handleDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "esc", "q"):
		m.mode = modeNormal
		m.detail = nil
	case isKey(msg, "g"):
		if m.detail != nil {
			m.mode = modeNormal
			return m.beginDownloadKey(m.detail.Key, m.detail.StorageClass, m.detail.RestoreStatus)
		}
	case isKey(msg, "r"):
		if m.detail != nil && isGlacier(m.detail.StorageClass) {
			m.mode = modeNormal
			m.confirmWhat = confirmRestore
			m.confirmTarget = m.detail.Key
			m.mode = modeConfirm
		}
	}
	return m, nil
}

func (m Model) handleBucketForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.mode = modeNormal
	case tea.KeyTab, tea.KeyShiftTab:
		if msg.Type == tea.KeyShiftTab {
			m.formField = (m.formField + 2) % 3 // backwards
		} else {
			m.formField = (m.formField + 1) % 3
		}
	case tea.KeyEnter:
		if m.formName != "" {
			m.cfg.AddBucket(m.formName, m.formRegion, m.formProfile)
			_ = m.cfg.Save()
			m.statusMsg = fmt.Sprintf("Saved bucket '%s'", m.formName)
			m.mode = modeNormal
			return m, clearFlashAfter(3 * time.Second)
		}
	case tea.KeyBackspace:
		m.formBackspace()
	case tea.KeyRunes:
		m.formInsert(string(msg.Runes))
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (m Model) beginDownload(filtered []aws.ObjectInfo) (Model, tea.Cmd) {
	if m.selected >= len(filtered) || filtered[m.selected].IsPrefix {
		return m, nil
	}
	item := filtered[m.selected]
	return m.beginDownloadKey(item.Key, item.StorageClass, "")
}

func (m Model) beginDownloadKey(key, storageClass, restoreStatus string) (Model, tea.Cmd) {
	if isGlacier(storageClass) && restoreStatus != "available" {
		m.statusMsg = "Object is in Glacier — restore it first (r)"
		return m, clearFlashAfter(3 * time.Second)
	}
	m.confirmWhat = confirmDownload
	m.confirmTarget = key
	m.mode = modeConfirm
	return m, nil
}

func (m Model) beginRestore(filtered []aws.ObjectInfo) (Model, tea.Cmd) {
	if m.selected >= len(filtered) || filtered[m.selected].IsPrefix {
		return m, nil
	}
	item := filtered[m.selected]
	if !isGlacier(item.StorageClass) {
		m.statusMsg = "Not a Glacier object"
		return m, clearFlashAfter(3 * time.Second)
	}
	m.confirmWhat = confirmRestore
	m.confirmTarget = item.Key
	m.mode = modeConfirm
	return m, nil
}

func (m *Model) formBackspace() {
	switch m.formField {
	case fieldName:
		m.formName = dropLast(m.formName)
	case fieldRegion:
		m.formRegion = dropLast(m.formRegion)
	case fieldProfile:
		m.formProfile = dropLast(m.formProfile)
	}
}

func (m *Model) formInsert(s string) {
	switch m.formField {
	case fieldName:
		m.formName += s
	case fieldRegion:
		m.formRegion += s
	case fieldProfile:
		m.formProfile += s
	}
}

func (m *Model) saveState() {
	m.st.Bucket = m.bucket
	m.st.Prefix = m.prefix
	_ = m.st.Save()
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	// 4-part vertical layout: header | table | status bar | keybinding bar
	header := m.viewHeader()
	content := m.viewContent()
	statusBar := m.viewStatusBar()
	keyBar := m.viewKeyBar()

	// Assemble — pad content to fill available height
	usedLines := lipgloss.Height(header) + lipgloss.Height(statusBar) + lipgloss.Height(keyBar)
	contentHeight := m.height - usedLines
	if contentHeight < 1 {
		contentHeight = 1
	}
	content = lipgloss.NewStyle().Height(contentHeight).Width(m.width).Render(content)

	base := lipgloss.JoinVertical(lipgloss.Left, header, content, statusBar, keyBar)

	// Draw overlays on top
	return m.drawOverlays(base)
}

func (m Model) viewHeader() string {
	tab1Label := " 1 "
	tab1Desc := " Browse "
	tab2Label := " 2 "
	tab2Desc := " Credentials "

	tab1LabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("8"))
	tab1DescStyle := lipgloss.NewStyle().Foreground(dim)
	tab2LabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("8"))
	tab2DescStyle := lipgloss.NewStyle().Foreground(dim)

	if m.tab == tabBrowse {
		tab1LabelStyle = tab1LabelStyle.Background(cyan)
		tab1DescStyle = tab1DescStyle.Foreground(cyan).Bold(true)
	}
	if m.tab == tabCredentials {
		tab2LabelStyle = tab2LabelStyle.Background(cyan)
		tab2DescStyle = tab2DescStyle.Foreground(cyan).Bold(true)
	}

	title := lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(" yelo ")
	tabs := fmt.Sprintf("%s  %s%s  %s%s",
		title,
		tab1LabelStyle.Render(tab1Label), tab1DescStyle.Render(tab1Desc),
		tab2LabelStyle.Render(tab2Label), tab2DescStyle.Render(tab2Desc),
	)

	border := lipgloss.NewStyle().Foreground(dim).Render(strings.Repeat("─", m.width))
	return tabs + "\n" + border
}

func (m Model) viewContent() string {
	switch m.tab {
	case tabBrowse:
		return m.viewBrowseTable()
	case tabCredentials:
		return m.viewCredentialsTable()
	default:
		return ""
	}
}

func (m Model) viewBrowseTable() string {
	if m.loading != "" {
		frame := spinner[m.spinnerTick/2%len(spinner)]
		return lipgloss.NewStyle().Foreground(cyan).Padding(1, 2).Render(
			fmt.Sprintf("%s %s", frame, m.loading),
		)
	}

	filtered := m.filteredItems()

	if m.bucket == "" {
		return lipgloss.NewStyle().Foreground(dim).Padding(1, 2).Render(
			"No bucket selected. Press b to pick a bucket.",
		)
	}

	if len(filtered) == 0 {
		msg := "(empty)"
		if m.filter != "" {
			msg = fmt.Sprintf("no matches for %q", m.filter)
		}
		return lipgloss.NewStyle().Foreground(dim).Padding(1, 2).Render(msg)
	}

	// Column header
	nameCol := max(20, m.width-30)
	headerLine := fmt.Sprintf("  %-*s %-7s %8s", nameCol, "NAME", "CLASS", "SIZE")
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(headerLine))
	b.WriteString("\n")

	// Calculate visible range
	tableHeight := m.height - 6 // header(2) + col header(1) + status(1) + keybar(1) + slack
	if tableHeight < 1 {
		tableHeight = 1
	}
	offset := 0
	if m.selected >= tableHeight {
		offset = m.selected - tableHeight + 1
	}

	for i := offset; i < len(filtered) && i < offset+tableHeight; i++ {
		item := filtered[i]
		name := displayName(item.Key, m.prefix)
		selected := i == m.selected

		var line string
		if item.IsPrefix {
			display := name
			classCell := lipgloss.NewStyle().Foreground(dim).Render("PRE    ")
			sizeCell := lipgloss.NewStyle().Foreground(dim).Render(strings.Repeat(" ", 8))
			if len(display) > nameCol {
				display = display[:nameCol-1] + "~"
			}
			nameCell := lipgloss.NewStyle().Foreground(cyan).Render(fmt.Sprintf("%-*s", nameCol, display))
			line = fmt.Sprintf("  %s %s %s", nameCell, classCell, sizeCell)
		} else {
			display := name
			if len(display) > nameCol {
				display = display[:nameCol-1] + "~"
			}
			nameCell := fmt.Sprintf("%-*s", nameCol, display)
			classLabel := storageClassLabel(item.StorageClass)
			classCell := lipgloss.NewStyle().Foreground(storageClassColor(item.StorageClass)).Render(fmt.Sprintf("%-7s", classLabel))
			sizeCell := fmt.Sprintf("%8s", output.FormatSize(item.Size))
			line = fmt.Sprintf("  %s %s %s", nameCell, classCell, sizeCell)
		}

		if selected {
			line = lipgloss.NewStyle().Background(selectBg).Bold(true).Width(m.width).Render(line)
		}
		b.WriteString(line)
		if i < offset+tableHeight-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) viewCredentialsTable() string {
	if m.loading != "" {
		frame := spinner[m.spinnerTick/2%len(spinner)]
		return lipgloss.NewStyle().Foreground(cyan).Padding(1, 2).Render(
			fmt.Sprintf("%s %s", frame, m.loading),
		)
	}

	buckets := m.cfg.Buckets
	if len(buckets) == 0 {
		return lipgloss.NewStyle().Foreground(dim).Padding(1, 2).Render(
			"No buckets configured. Press a to add one.",
		)
	}

	nameCol := 24
	regionCol := 16
	profileCol := 16
	headerLine := fmt.Sprintf("  %-*s %-*s %-*s %s", nameCol, "BUCKET", regionCol, "REGION", profileCol, "PROFILE", "STATUS")
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(headerLine))
	b.WriteString("\n")

	for i, bucket := range buckets {
		selected := i == m.profileCursor

		name := bucket.Name
		if name == m.cfg.DefaultBucket {
			name += " *"
		}
		region := bucket.Region
		if region == "" {
			region = lipgloss.NewStyle().Foreground(dim).Render("(default)")
		}
		profile := bucket.Profile
		if profile == "" {
			profile = lipgloss.NewStyle().Foreground(dim).Render("(default)")
		}

		status := ""
		switch m.profileStatus[bucket.Name] {
		case "ok":
			status = lipgloss.NewStyle().Foreground(green).Render("● connected")
		case "fail":
			status = lipgloss.NewStyle().Foreground(red).Render("● failed")
		case "testing":
			frame := spinner[m.spinnerTick/2%len(spinner)]
			status = lipgloss.NewStyle().Foreground(yellow).Render(frame + " testing")
		}

		line := fmt.Sprintf("  %-*s %-*s %-*s %s",
			nameCol, name,
			regionCol, region,
			profileCol, profile,
			status,
		)

		if selected {
			line = lipgloss.NewStyle().Background(selectBg).Bold(true).Width(m.width).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	// AWS profiles section
	if len(m.profiles) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(dim).Padding(0, 2).Render(
			fmt.Sprintf("AWS profiles found: %s", strings.Join(m.profiles, ", ")),
		))
	}

	return b.String()
}

func (m Model) viewStatusBar() string {
	border := lipgloss.NewStyle().Foreground(dim).Render(strings.Repeat("─", m.width))

	var msg string
	var color lipgloss.Color
	if m.loading != "" {
		frame := spinner[m.spinnerTick/2%len(spinner)]
		msg = fmt.Sprintf("%s %s", frame, m.loading)
		color = cyan
	} else if m.statusMsg != "" {
		msg = m.statusMsg
		if strings.HasPrefix(msg, "Error") || strings.HasPrefix(msg, "Download failed") || strings.HasPrefix(msg, "Restore failed") {
			color = red
		} else {
			color = yellow
		}
	} else if m.tab == tabBrowse && m.bucket != "" {
		msg = fmt.Sprintf("%s:/%s", m.bucket, m.prefix)
		color = dim
	}

	statusLine := lipgloss.NewStyle().Foreground(color).Padding(0, 1).Render(msg)
	return border + "\n" + statusLine
}

func (m Model) viewKeyBar() string {
	var bindings []binding

	switch m.mode {
	case modeHelp:
		bindings = helpKeys
	case modeBucketPicker:
		bindings = bucketPickerKeys
	case modeConfirm:
		bindings = confirmKeys
	case modeFilter:
		bindings = filterKeys
	case modeDetail:
		bindings = detailKeys
	case modeAddBucket, modeEditBucket:
		bindings = inputKeys
	case modeNormal:
		switch {
		case m.tab == tabBrowse && m.submenu:
			bindings = browseSubmenuKeys
		case m.tab == tabBrowse:
			bindings = browseKeys
		case m.tab == tabCredentials && m.submenu:
			bindings = credentialsSubmenuKeys
		case m.tab == tabCredentials:
			bindings = credentialsKeys
		}
	}

	var parts []string
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("8"))
	descStyle := lipgloss.NewStyle().Foreground(dim)
	for _, b := range bindings {
		parts = append(parts, keyStyle.Render(" "+b.key+" ")+descStyle.Render(" "+b.desc+" "))
	}
	return " " + strings.Join(parts, "")
}

// ---------------------------------------------------------------------------
// Overlays — drawn on top of the base view
// ---------------------------------------------------------------------------

func (m Model) drawOverlays(base string) string {
	switch m.mode {
	case modeBucketPicker:
		return m.overlayBucketPicker(base)
	case modeConfirm:
		return m.overlayConfirm(base)
	case modeDetail:
		return m.overlayDetail(base)
	case modeAddBucket:
		return m.overlayBucketForm(base, "Add Bucket")
	case modeEditBucket:
		return m.overlayBucketForm(base, "Edit Bucket")
	case modeHelp:
		return m.overlayHelp(base)
	}
	return base
}

func (m Model) overlayBucketPicker(base string) string {
	w := min(50, m.width-4)
	h := min(len(m.bucketList)+4, m.height-4)

	var content strings.Builder
	content.WriteString("\n")
	for i, b := range m.bucketList {
		if i >= h-3 {
			break
		}
		marker := "  "
		if b == m.bucket {
			marker = "* "
		}
		line := fmt.Sprintf(" %s%s", marker, b)
		if i == m.bucketCursor {
			content.WriteString(lipgloss.NewStyle().Background(selectBg).Bold(true).Width(w - 2).Render(line))
		} else {
			content.WriteString(line)
		}
		content.WriteString("\n")
	}

	return placeOverlay(base, m.width, m.height, w, h, "Select Bucket", content.String())
}

func (m Model) overlayConfirm(base string) string {
	action := "download"
	if m.confirmWhat == confirmRestore {
		action = "restore"
	} else if m.confirmWhat == confirmRemoveBucket {
		action = "remove"
	}
	target := path.Base(m.confirmTarget)

	msg := fmt.Sprintf("\n  %s '%s' ?\n\n  %s",
		action, target,
		lipgloss.NewStyle().Foreground(dim).Render("Press y to confirm, n or Esc to cancel"),
	)

	w := min(55, m.width-4)
	return placeOverlay(base, m.width, m.height, w, 6, "Confirm", msg)
}

func (m Model) overlayDetail(base string) string {
	if m.detail == nil {
		return base
	}
	obj := m.detail

	w := min(60, m.width-4)
	var content strings.Builder
	content.WriteString("\n")

	writeField := func(label, value string) {
		l := lipgloss.NewStyle().Foreground(dim).Width(12).Align(lipgloss.Right).Render(label)
		content.WriteString(fmt.Sprintf("  %s  %s\n", l, value))
	}

	writeField("Key", path.Base(obj.Key))
	writeField("Full path", obj.Key)
	writeField("Size", fmt.Sprintf("%s (%d bytes)", output.FormatSize(obj.Size), obj.Size))

	classLabel := storageClassLabel(obj.StorageClass)
	classStyled := lipgloss.NewStyle().Foreground(storageClassColor(obj.StorageClass)).Render(classLabel)
	writeField("Class", classStyled)
	writeField("Modified", obj.LastModified)

	if obj.ContentType != "" {
		writeField("Type", obj.ContentType)
	}
	if obj.ETag != "" {
		etag := obj.ETag
		maxEtag := w - 18
		if maxEtag > 0 && len(etag) > maxEtag {
			etag = etag[:maxEtag-3] + "..."
		}
		writeField("ETag", etag)
	}

	if isGlacier(obj.StorageClass) {
		label, color := restoreLabel(obj.RestoreStatus)
		if label == "" {
			label = "not restored"
			color = dim
		}
		writeField("Restore", lipgloss.NewStyle().Foreground(color).Render(label))
	}

	content.WriteString("\n")

	h := strings.Count(content.String(), "\n") + 2
	return placeOverlay(base, m.width, m.height, w, h, "Object Detail", content.String())
}

func (m Model) overlayBucketForm(base string, title string) string {
	w := min(55, m.width-4)

	renderField := func(label string, value string, field addField) string {
		active := m.formField == field
		cursor := ""
		marker := "  "
		var style lipgloss.Style
		if active {
			cursor = "_"
			marker = "> "
			style = lipgloss.NewStyle().Foreground(white).Bold(true)
		} else {
			style = lipgloss.NewStyle().Foreground(dim)
		}
		return style.Render(fmt.Sprintf("%s%-10s %s%s", marker, label+":", value, cursor))
	}

	content := fmt.Sprintf("\n%s\n%s\n%s\n",
		renderField("Name", m.formName, fieldName),
		renderField("Region", m.formRegion, fieldRegion),
		renderField("Profile", m.formProfile, fieldProfile),
	)

	return placeOverlay(base, m.width, m.height, w, 10, title, content)
}

func (m Model) overlayHelp(base string) string {
	w := min(60, m.width-4)

	helpLines := []struct{ key, desc string }{
		{"1 / 2", "Switch tabs"},
		{"j / k", "Navigate up / down"},
		{"enter / l", "Open prefix or view object detail"},
		{"h / backspace", "Go to parent directory"},
		{"g", "Download selected object"},
		{"r", "Initiate Glacier restore"},
		{"b", "Switch bucket"},
		{"/", "Filter current listing"},
		{".", "Toggle secondary actions"},
		{"R", "Refresh"},
		{"s", "View object stat/detail"},
		{"a", "Add bucket (Credentials tab)"},
		{"t", "Test connection (Credentials tab)"},
		{"d", "Remove bucket (Credentials tab)"},
		{"D", "Set default bucket"},
		{"q", "Quit (saves state)"},
	}

	var content strings.Builder
	content.WriteString("\n")
	for _, h := range helpLines {
		k := lipgloss.NewStyle().Foreground(cyan).Width(16).Render(h.key)
		content.WriteString(fmt.Sprintf("  %s %s\n", k, h.desc))
	}

	h := len(helpLines) + 3
	return placeOverlay(base, m.width, m.height, w, h, "Help", content.String())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func placeOverlay(base string, termW, termH, w, h int, title, content string) string {
	// Build the dialog box
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyan).
		Width(w).
		Height(h)

	titleStyled := lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(fmt.Sprintf(" %s ", title))
	box := border.Render(content)

	// Inject title into top border
	boxLines := strings.Split(box, "\n")
	if len(boxLines) > 0 {
		topBorder := boxLines[0]
		if len(topBorder) > 4 {
			// Replace part of top border with title
			titleStr := fmt.Sprintf(" %s ", title)
			titleRendered := lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(titleStr)
			_ = titleStyled // use the rendered version
			runes := []rune(topBorder)
			insertPoint := 2
			if insertPoint+len([]rune(titleStr)) < len(runes) {
				boxLines[0] = string(runes[:insertPoint]) + titleRendered + string(runes[insertPoint+len([]rune(titleStr)):])
			}
		}
	}
	box = strings.Join(boxLines, "\n")

	// Center the box on the base
	baseLines := strings.Split(base, "\n")
	boxLines = strings.Split(box, "\n")

	startY := (termH - len(boxLines)) / 2
	startX := (termW - w - 2) / 2 // -2 for border
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i, boxLine := range boxLines {
		row := startY + i
		if row >= len(baseLines) {
			break
		}
		baseLine := baseLines[row]
		baseRunes := []rune(baseLine)
		boxRunes := []rune(boxLine)

		// Pad base line if needed
		for len(baseRunes) < startX+len(boxRunes) {
			baseRunes = append(baseRunes, ' ')
		}

		// Overwrite base with box
		result := make([]rune, len(baseRunes))
		copy(result, baseRunes)
		for j, r := range boxRunes {
			pos := startX + j
			if pos < len(result) {
				result[pos] = r
			}
		}
		baseLines[row] = string(result)
	}

	return strings.Join(baseLines, "\n")
}

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

func isKey(msg tea.KeyMsg, keys ...string) bool {
	s := msg.String()
	for _, k := range keys {
		if s == k {
			return true
		}
	}
	return false
}

func dropLast(s string) string {
	if len(s) == 0 {
		return ""
	}
	return s[:len(s)-1]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
