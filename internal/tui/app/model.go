package app

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dorkyrobot/yelo/internal/aws"
	"github.com/dorkyrobot/yelo/internal/config"
	"github.com/dorkyrobot/yelo/internal/output"
	"github.com/dorkyrobot/yelo/internal/state"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type activeTab int

const (
	tabBrowse activeTab = iota
	tabCredentials
)

type mode int

const (
	modeNormal mode = iota
	modeHelp
	modeBucketPicker
	modeConfirm
	modeFilter
	modeDetail
	modeAddBucket
	modeEditBucket
)

type confirmAction int

const (
	confirmDownload confirmAction = iota
	confirmRestore
	confirmRemoveBucket
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type Model struct {
	// Dependencies
	cfg    *config.Config
	st     *state.State
	client aws.S3Client

	// Navigation
	bucket string
	prefix string
	items  []aws.ObjectInfo

	// Tab + mode
	tab     activeTab
	mode    mode
	submenu bool

	// Components
	browseTable table.Model
	credsTable  table.Model
	spinner     spinner.Model
	help        help.Model
	filterInput textinput.Model

	// Form inputs (add/edit bucket)
	formInputs [3]textinput.Model // name, region, profile
	formFocus  int

	// Bucket picker
	bucketPicker table.Model
	bucketList   []string

	// Confirm dialog
	confirmWhat   confirmAction
	confirmTarget string

	// Detail overlay
	detail *aws.ObjectInfo

	// Credentials
	profiles      []string
	profileStatus map[string]string

	// Async
	loading   string
	statusMsg string

	// Terminal
	width  int
	height int
}

func NewModel(cfg *config.Config, st *state.State, client aws.S3Client, bucket string) Model {
	// Spinner
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cyan)

	// Help
	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("8")).Padding(0, 1)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(dim).PaddingRight(1)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(dim)
	h.ShortSeparator = ""

	// Browse table
	bt := table.New(
		table.WithColumns(browseColumns(80)),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithHeight(10),
		table.WithStyles(tableStyles()),
	)

	// Credentials table
	ct := table.New(
		table.WithColumns(credsColumns(80)),
		table.WithRows(nil),
		table.WithFocused(false),
		table.WithHeight(10),
		table.WithStyles(tableStyles()),
	)

	// Filter input
	fi := textinput.New()
	fi.Prompt = "filter: "
	fi.PromptStyle = lipgloss.NewStyle().Foreground(cyan)
	fi.TextStyle = lipgloss.NewStyle().Foreground(white)
	fi.Placeholder = "type to filter..."
	fi.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)

	// Form inputs
	nameInput := textinput.New()
	nameInput.Prompt = "  Name:     "
	nameInput.PromptStyle = lipgloss.NewStyle().Foreground(cyan)
	nameInput.Placeholder = "my-bucket"
	nameInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)
	nameInput.Focus()

	regionInput := textinput.New()
	regionInput.Prompt = "  Region:   "
	regionInput.PromptStyle = lipgloss.NewStyle().Foreground(dim)
	regionInput.Placeholder = "us-east-1"
	regionInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)

	profileInput := textinput.New()
	profileInput.Prompt = "  Profile:  "
	profileInput.PromptStyle = lipgloss.NewStyle().Foreground(dim)
	profileInput.Placeholder = "default"
	profileInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)

	// Bucket picker table
	bp := table.New(
		table.WithColumns([]table.Column{{Title: "Bucket", Width: 40}}),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithHeight(10),
		table.WithStyles(tableStyles()),
	)

	m := Model{
		cfg:           cfg,
		st:            st,
		client:        client,
		bucket:        bucket,
		prefix:        st.Prefix,
		tab:           tabBrowse,
		mode:          modeNormal,
		browseTable:   bt,
		credsTable:    ct,
		spinner:       sp,
		help:          h,
		filterInput:   fi,
		formInputs:    [3]textinput.Model{nameInput, regionInput, profileInput},
		bucketPicker:  bp,
		profileStatus: map[string]string{},
	}

	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick}
	if m.bucket == "" {
		m.loading = "Loading buckets..."
		cmds = append(cmds, fetchBuckets(m.client))
	} else {
		m.loading = "Loading..."
		cmds = append(cmds, fetchList(m.client, m.bucket, m.prefix))
	}
	return tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeTables()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case listResultMsg:
		m.loading = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, clearFlashAfter(5 * time.Second)
		}
		m.items = msg.items
		m.rebuildBrowseTable()
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
		m.rebuildBucketPicker()
		m.mode = modeBucketPicker
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
		m.rebuildCredsTable()
		return m, nil

	case profileTestMsg:
		if msg.ok {
			m.profileStatus[msg.bucket] = "ok"
			m.statusMsg = fmt.Sprintf("'%s' connected", msg.bucket)
		} else {
			m.profileStatus[msg.bucket] = "fail"
			m.statusMsg = fmt.Sprintf("'%s' failed: %v", msg.bucket, msg.err)
		}
		m.rebuildCredsTable()
		return m, clearFlashAfter(5 * time.Second)

	case clearFlashMsg:
		m.statusMsg = ""
		return m, nil

	case tea.KeyMsg:
		if m.loading != "" {
			return m, nil
		}
		return m.handleKey(msg)
	}

	// Forward to active sub-components if in input modes
	if m.mode == modeFilter {
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.mode == modeAddBucket || m.mode == modeEditBucket {
		var cmd tea.Cmd
		m.formInputs[m.formFocus], cmd = m.formInputs[m.formFocus].Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) resizeTables() {
	contentH := m.height - 5 // header(2) + status(2) + help(1)
	if contentH < 3 {
		contentH = 3
	}

	m.browseTable.SetColumns(browseColumns(m.width))
	m.browseTable.SetWidth(m.width)
	m.browseTable.SetHeight(contentH)

	m.credsTable.SetColumns(credsColumns(m.width))
	m.credsTable.SetWidth(m.width)
	m.credsTable.SetHeight(contentH)

	pickerW := min(50, m.width-4)
	m.bucketPicker.SetColumns([]table.Column{{Title: "Bucket", Width: pickerW - 4}})
	m.bucketPicker.SetHeight(min(15, m.height-6))

	m.help.Width = m.width
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeHelp:
		m.mode = modeNormal
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
		return m.handleForm(msg)

	case modeNormal:
		// Tab switching available in all normal contexts
		if key.Matches(msg, keys.Tab1) {
			m.tab = tabBrowse
			m.submenu = false
			m.browseTable.Focus()
			m.credsTable.Blur()
			return m, nil
		}
		if key.Matches(msg, keys.Tab2) {
			m.tab = tabCredentials
			m.submenu = false
			m.credsTable.Focus()
			m.browseTable.Blur()
			if len(m.cfg.Buckets) > 0 && len(m.profiles) == 0 {
				m.loading = "Loading profiles..."
				return m, loadProfiles()
			}
			m.rebuildCredsTable()
			return m, nil
		}

		switch m.tab {
		case tabBrowse:
			if m.submenu {
				return m.handleBrowseSubmenu(msg)
			}
			return m.handleBrowse(msg)
		case tabCredentials:
			if m.submenu {
				return m.handleCredsSubmenu(msg)
			}
			return m.handleCreds(msg)
		}
	}
	return m, nil
}

func (m Model) handleBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		m.saveState()
		return m, tea.Quit

	case key.Matches(msg, keys.Enter):
		row := m.browseTable.SelectedRow()
		if row == nil {
			return m, nil
		}
		idx := m.browseTable.Cursor()
		filtered := m.filteredItems()
		if idx >= len(filtered) {
			return m, nil
		}
		item := filtered[idx]
		if item.IsPrefix {
			m.prefix = item.Key
			m.loading = "Loading..."
			return m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))
		}
		// Object → show detail
		m.loading = "Loading metadata..."
		return m, tea.Batch(m.spinner.Tick, fetchDetail(m.client, m.bucket, item.Key))

	case key.Matches(msg, keys.Back):
		if m.prefix != "" {
			m.prefix = parentPrefix(m.prefix)
			m.loading = "Loading..."
			return m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))
		}

	case key.Matches(msg, keys.Get):
		return m.beginDownload()

	case key.Matches(msg, keys.Restore):
		return m.beginRestore()

	case key.Matches(msg, keys.Buckets):
		m.loading = "Loading buckets..."
		return m, tea.Batch(m.spinner.Tick, fetchBuckets(m.client))

	case key.Matches(msg, keys.Filter):
		m.mode = modeFilter
		m.filterInput.SetValue("")
		m.filterInput.Focus()
		return m, m.filterInput.Focus()

	case key.Matches(msg, keys.More):
		m.submenu = true
		return m, nil

	default:
		// Forward to table for navigation (j/k/up/down/pgup/pgdn)
		var cmd tea.Cmd
		m.browseTable, cmd = m.browseTable.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleBrowseSubmenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.More):
		m.submenu = false
	case key.Matches(msg, keys.Refresh):
		m.submenu = false
		m.loading = "Refreshing..."
		return m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))
	case key.Matches(msg, keys.Stat):
		m.submenu = false
		filtered := m.filteredItems()
		idx := m.browseTable.Cursor()
		if idx < len(filtered) && !filtered[idx].IsPrefix {
			m.loading = "Loading metadata..."
			return m, tea.Batch(m.spinner.Tick, fetchDetail(m.client, m.bucket, filtered[idx].Key))
		}
	case key.Matches(msg, keys.Help):
		m.mode = modeHelp
		m.submenu = false
	}
	return m, nil
}

func (m Model) handleCreds(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		m.saveState()
		return m, tea.Quit

	case key.Matches(msg, keys.Add):
		m.beginAddBucket()
		return m, m.formInputs[0].Focus()

	case key.Matches(msg, keys.Test):
		idx := m.credsTable.Cursor()
		if idx < len(m.cfg.Buckets) {
			b := m.cfg.Buckets[idx]
			profile := b.Profile
			if profile == "" {
				profile = m.cfg.Profile
			}
			if profile == "" {
				profile = "default"
			}
			m.profileStatus[b.Name] = "testing"
			m.rebuildCredsTable()
			m.statusMsg = fmt.Sprintf("Testing '%s'...", b.Name)
			return m, testProfile(b.Name, profile)
		}

	case key.Matches(msg, keys.Delete):
		idx := m.credsTable.Cursor()
		if idx < len(m.cfg.Buckets) {
			m.confirmWhat = confirmRemoveBucket
			m.confirmTarget = m.cfg.Buckets[idx].Name
			m.mode = modeConfirm
		}

	case key.Matches(msg, keys.More):
		m.submenu = true

	default:
		var cmd tea.Cmd
		m.credsTable, cmd = m.credsTable.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleCredsSubmenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.More):
		m.submenu = false

	case key.Matches(msg, keys.Edit):
		idx := m.credsTable.Cursor()
		if idx < len(m.cfg.Buckets) {
			b := m.cfg.Buckets[idx]
			m.beginEditBucket(b.Name, b.Region, b.Profile)
			m.submenu = false
			return m, m.formInputs[0].Focus()
		}

	case key.Matches(msg, keys.Default):
		idx := m.credsTable.Cursor()
		if idx < len(m.cfg.Buckets) {
			name := m.cfg.Buckets[idx].Name
			_ = m.cfg.SetDefault(name)
			_ = m.cfg.Save()
			m.statusMsg = fmt.Sprintf("Default set to '%s'", name)
			m.rebuildCredsTable()
			m.submenu = false
			return m, clearFlashAfter(3 * time.Second)
		}

	case key.Matches(msg, keys.Refresh):
		m.submenu = false
		m.loading = "Loading profiles..."
		return m, loadProfiles()

	case key.Matches(msg, keys.Help):
		m.mode = modeHelp
		m.submenu = false
	}
	return m, nil
}

func (m Model) handleBucketPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Cancel):
		if m.bucket != "" {
			m.mode = modeNormal
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		idx := m.bucketPicker.Cursor()
		if idx < len(m.bucketList) {
			m.bucket = m.bucketList[idx]
			m.prefix = ""
			m.mode = modeNormal
			m.loading = "Loading..."
			return m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))
		}

	default:
		var cmd tea.Cmd
		m.bucketPicker, cmd = m.bucketPicker.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Confirm):
		m.mode = modeNormal
		switch m.confirmWhat {
		case confirmDownload:
			m.loading = fmt.Sprintf("Downloading %s...", path.Base(m.confirmTarget))
			return m, tea.Batch(m.spinner.Tick, downloadObject(m.client, m.bucket, m.confirmTarget))
		case confirmRestore:
			m.loading = fmt.Sprintf("Restoring %s...", path.Base(m.confirmTarget))
			return m, tea.Batch(m.spinner.Tick, restoreObject(m.client, m.bucket, m.confirmTarget, 7, "Standard"))
		case confirmRemoveBucket:
			m.cfg.RemoveBucket(m.confirmTarget)
			_ = m.cfg.Save()
			m.statusMsg = fmt.Sprintf("Removed '%s'", m.confirmTarget)
			m.rebuildCredsTable()
			return m, clearFlashAfter(3 * time.Second)
		}
	case key.Matches(msg, keys.Cancel):
		m.mode = modeNormal
	}
	return m, nil
}

func (m Model) handleFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyEscape:
		m.mode = modeNormal
		m.filterInput.Blur()
		m.rebuildBrowseTable()
		return m, nil
	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		// Live-filter as user types
		m.rebuildBrowseTable()
		return m, cmd
	}
}

func (m Model) handleDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Cancel):
		m.mode = modeNormal
		m.detail = nil
	case key.Matches(msg, keys.Get):
		if m.detail != nil {
			m.mode = modeNormal
			key := m.detail.Key
			class := m.detail.StorageClass
			restore := m.detail.RestoreStatus
			m.detail = nil
			if isGlacier(class) && restore != "available" {
				m.statusMsg = "Object is in Glacier — restore it first (r)"
				return m, clearFlashAfter(3 * time.Second)
			}
			m.confirmWhat = confirmDownload
			m.confirmTarget = key
			m.mode = modeConfirm
		}
	case key.Matches(msg, keys.Restore):
		if m.detail != nil && isGlacier(m.detail.StorageClass) {
			k := m.detail.Key
			m.mode = modeConfirm
			m.confirmWhat = confirmRestore
			m.confirmTarget = k
			m.detail = nil
		}
	}
	return m, nil
}

func (m Model) handleForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.mode = modeNormal
		return m, nil

	case tea.KeyTab:
		m.formInputs[m.formFocus].Blur()
		m.formFocus = (m.formFocus + 1) % 3
		m.updateFormStyles()
		return m, m.formInputs[m.formFocus].Focus()

	case tea.KeyShiftTab:
		m.formInputs[m.formFocus].Blur()
		m.formFocus = (m.formFocus + 2) % 3
		m.updateFormStyles()
		return m, m.formInputs[m.formFocus].Focus()

	case tea.KeyEnter:
		name := m.formInputs[0].Value()
		if name != "" {
			region := m.formInputs[1].Value()
			profile := m.formInputs[2].Value()
			m.cfg.AddBucket(name, region, profile)
			_ = m.cfg.Save()
			m.statusMsg = fmt.Sprintf("Saved bucket '%s'", name)
			m.mode = modeNormal
			m.rebuildCredsTable()
			return m, clearFlashAfter(3 * time.Second)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.formInputs[m.formFocus], cmd = m.formInputs[m.formFocus].Update(msg)
		return m, cmd
	}
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func (m Model) beginDownload() (Model, tea.Cmd) {
	filtered := m.filteredItems()
	idx := m.browseTable.Cursor()
	if idx >= len(filtered) || filtered[idx].IsPrefix {
		return m, nil
	}
	item := filtered[idx]
	if isGlacier(item.StorageClass) {
		m.statusMsg = "Object is in Glacier — restore it first (r)"
		return m, clearFlashAfter(3 * time.Second)
	}
	m.confirmWhat = confirmDownload
	m.confirmTarget = item.Key
	m.mode = modeConfirm
	return m, nil
}

func (m Model) beginRestore() (Model, tea.Cmd) {
	filtered := m.filteredItems()
	idx := m.browseTable.Cursor()
	if idx >= len(filtered) || filtered[idx].IsPrefix {
		return m, nil
	}
	item := filtered[idx]
	if !isGlacier(item.StorageClass) {
		m.statusMsg = "Not a Glacier object"
		return m, clearFlashAfter(3 * time.Second)
	}
	m.confirmWhat = confirmRestore
	m.confirmTarget = item.Key
	m.mode = modeConfirm
	return m, nil
}

func (m *Model) beginAddBucket() {
	m.mode = modeAddBucket
	m.formFocus = 0
	for i := range m.formInputs {
		m.formInputs[i].SetValue("")
	}
	m.updateFormStyles()
}

func (m *Model) beginEditBucket(name, region, profile string) {
	m.mode = modeEditBucket
	m.formFocus = 0
	m.formInputs[0].SetValue(name)
	m.formInputs[1].SetValue(region)
	m.formInputs[2].SetValue(profile)
	m.updateFormStyles()
}

func (m *Model) updateFormStyles() {
	for i := range m.formInputs {
		if i == m.formFocus {
			m.formInputs[i].PromptStyle = lipgloss.NewStyle().Foreground(cyan).Bold(true)
			m.formInputs[i].TextStyle = lipgloss.NewStyle().Foreground(white)
		} else {
			m.formInputs[i].PromptStyle = lipgloss.NewStyle().Foreground(dim)
			m.formInputs[i].TextStyle = lipgloss.NewStyle().Foreground(dim)
		}
	}
}

func (m *Model) saveState() {
	m.st.Bucket = m.bucket
	m.st.Prefix = m.prefix
	_ = m.st.Save()
}

// ---------------------------------------------------------------------------
// Table builders
// ---------------------------------------------------------------------------

func browseColumns(w int) []table.Column {
	nameW := w - 20
	if nameW < 20 {
		nameW = 20
	}
	return []table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Class", Width: 8},
		{Title: "Size", Width: 10},
	}
}

func credsColumns(w int) []table.Column {
	nameW := 24
	return []table.Column{
		{Title: "Bucket", Width: nameW},
		{Title: "Region", Width: 16},
		{Title: "Profile", Width: 16},
		{Title: "Status", Width: w - nameW - 16 - 16 - 4},
	}
}

func (m *Model) rebuildBrowseTable() {
	filtered := m.filteredItems()
	rows := make([]table.Row, len(filtered))
	for i, item := range filtered {
		name := displayName(item.Key, m.prefix)
		if item.IsPrefix {
			rows[i] = table.Row{
				lipgloss.NewStyle().Foreground(cyan).Render(name),
				lipgloss.NewStyle().Foreground(dim).Render("PRE"),
				"",
			}
		} else {
			rows[i] = table.Row{
				name,
				lipgloss.NewStyle().Foreground(storageClassColor(item.StorageClass)).Render(storageClassLabel(item.StorageClass)),
				output.FormatSize(item.Size),
			}
		}
	}
	m.browseTable.SetRows(rows)
}

func (m *Model) rebuildCredsTable() {
	buckets := m.cfg.Buckets
	rows := make([]table.Row, len(buckets))
	for i, b := range buckets {
		name := b.Name
		if name == m.cfg.DefaultBucket {
			name += " *"
		}
		region := b.Region
		if region == "" {
			region = lipgloss.NewStyle().Foreground(dim).Render("(default)")
		}
		profile := b.Profile
		if profile == "" {
			profile = lipgloss.NewStyle().Foreground(dim).Render("(default)")
		}
		status := ""
		switch m.profileStatus[b.Name] {
		case "ok":
			status = lipgloss.NewStyle().Foreground(green).Render("● connected")
		case "fail":
			status = lipgloss.NewStyle().Foreground(red).Render("● failed")
		case "testing":
			status = lipgloss.NewStyle().Foreground(yellow).Render("● testing...")
		}
		rows[i] = table.Row{name, region, profile, status}
	}
	m.credsTable.SetRows(rows)
}

func (m *Model) rebuildBucketPicker() {
	rows := make([]table.Row, len(m.bucketList))
	for i, b := range m.bucketList {
		label := b
		if b == m.bucket {
			label = "* " + b
		} else {
			label = "  " + b
		}
		rows[i] = table.Row{label}
	}
	m.bucketPicker.SetRows(rows)
	m.bucketPicker.SetCursor(0)
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	header := m.viewHeader()
	content := m.viewContent()
	statusBar := m.viewStatusBar()
	helpBar := m.viewHelpBar()

	// Pad content
	usedH := lipgloss.Height(header) + lipgloss.Height(statusBar) + lipgloss.Height(helpBar)
	contentH := m.height - usedH
	if contentH < 1 {
		contentH = 1
	}
	content = lipgloss.NewStyle().Height(contentH).MaxHeight(contentH).Width(m.width).Render(content)

	view := lipgloss.JoinVertical(lipgloss.Left, header, content, statusBar, helpBar)

	// Overlays
	switch m.mode {
	case modeBucketPicker:
		view = m.placeOverlay(view, m.viewBucketPicker(), "Select Bucket")
	case modeConfirm:
		view = m.placeOverlay(view, m.viewConfirm(), "Confirm")
	case modeDetail:
		view = m.placeOverlay(view, m.viewDetail(), "Object Detail")
	case modeAddBucket:
		view = m.placeOverlay(view, m.viewForm(), "Add Bucket")
	case modeEditBucket:
		view = m.placeOverlay(view, m.viewForm(), "Edit Bucket")
	case modeHelp:
		view = m.placeOverlay(view, m.viewHelp(), "Help")
	}

	return view
}

func (m Model) viewHeader() string {
	tab := func(label string, num string, active bool) string {
		numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("8")).Padding(0, 1)
		labelStyle := lipgloss.NewStyle().Foreground(dim)
		if active {
			numStyle = numStyle.Background(cyan)
			labelStyle = labelStyle.Foreground(cyan).Bold(true)
		}
		return numStyle.Render(num) + labelStyle.Render(" "+label+" ")
	}

	title := lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(" yelo ")
	tabs := title + "  " + tab("Browse", "1", m.tab == tabBrowse) + " " + tab("Credentials", "2", m.tab == tabCredentials)

	border := lipgloss.NewStyle().Foreground(dim).Width(m.width).Render(strings.Repeat("─", m.width))
	return tabs + "\n" + border
}

func (m Model) viewContent() string {
	if m.loading != "" {
		return lipgloss.NewStyle().Padding(1, 2).Render(
			m.spinner.View() + " " + m.loading,
		)
	}

	switch m.tab {
	case tabBrowse:
		if m.bucket == "" {
			return lipgloss.NewStyle().Foreground(dim).Padding(1, 2).Render(
				"No bucket selected. Press b to pick a bucket.",
			)
		}
		if m.mode == modeFilter {
			return m.filterInput.View() + "\n" + m.browseTable.View()
		}
		return m.browseTable.View()

	case tabCredentials:
		if len(m.cfg.Buckets) == 0 {
			msg := lipgloss.NewStyle().Foreground(dim).Padding(1, 2).Render(
				"No buckets configured. Press a to add one.",
			)
			if len(m.profiles) > 0 {
				msg += "\n\n" + lipgloss.NewStyle().Foreground(dim).Padding(0, 2).Render(
					fmt.Sprintf("AWS profiles found: %s", strings.Join(m.profiles, ", ")),
				)
			}
			return msg
		}
		view := m.credsTable.View()
		if len(m.profiles) > 0 {
			view += "\n" + lipgloss.NewStyle().Foreground(dim).Padding(0, 2).Render(
				fmt.Sprintf("AWS profiles: %s", strings.Join(m.profiles, ", ")),
			)
		}
		return view
	}
	return ""
}

func (m Model) viewStatusBar() string {
	border := lipgloss.NewStyle().Foreground(dim).Render(strings.Repeat("─", m.width))

	var line string
	if m.loading != "" {
		line = lipgloss.NewStyle().Foreground(cyan).Padding(0, 1).Render(m.spinner.View() + " " + m.loading)
	} else if m.statusMsg != "" {
		color := yellow
		if strings.HasPrefix(m.statusMsg, "Error") || strings.Contains(m.statusMsg, "failed") {
			color = red
		}
		line = lipgloss.NewStyle().Foreground(color).Padding(0, 1).Render(m.statusMsg)
	} else if m.tab == tabBrowse && m.bucket != "" {
		line = lipgloss.NewStyle().Foreground(dim).Padding(0, 1).Render(m.bucket + ":/" + m.prefix)
	}

	return border + "\n" + line
}

func (m Model) viewHelpBar() string {
	switch m.mode {
	case modeHelp:
		return m.help.View(helpKeyMap{})
	case modeBucketPicker:
		return m.help.View(bucketPickerKeyMap{})
	case modeConfirm:
		return m.help.View(confirmKeyMap{})
	case modeFilter:
		return m.help.View(filterKeyMap{})
	case modeDetail:
		return m.help.View(detailKeyMap{})
	case modeAddBucket, modeEditBucket:
		return m.help.View(formKeyMap{})
	case modeNormal:
		switch {
		case m.tab == tabBrowse && m.submenu:
			return m.help.View(browseSubmenuKeyMap{})
		case m.tab == tabBrowse:
			return m.help.View(browseKeyMap{})
		case m.tab == tabCredentials && m.submenu:
			return m.help.View(credentialsSubmenuKeyMap{})
		case m.tab == tabCredentials:
			return m.help.View(credentialsKeyMap{})
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Overlay views (content only — wrapping + placement done by placeOverlay)
// ---------------------------------------------------------------------------

func (m Model) viewBucketPicker() string {
	return m.bucketPicker.View()
}

func (m Model) viewConfirm() string {
	action := "download"
	if m.confirmWhat == confirmRestore {
		action = "restore"
	} else if m.confirmWhat == confirmRemoveBucket {
		action = "remove"
	}
	target := path.Base(m.confirmTarget)

	return fmt.Sprintf("\n  %s '%s' ?\n\n  %s\n",
		action, target,
		lipgloss.NewStyle().Foreground(dim).Render("Press y to confirm, n or Esc to cancel"),
	)
}

func (m Model) viewDetail() string {
	if m.detail == nil {
		return ""
	}
	obj := m.detail
	var b strings.Builder

	field := func(label, value string) {
		l := lipgloss.NewStyle().Foreground(dim).Width(12).Align(lipgloss.Right).Render(label)
		b.WriteString(fmt.Sprintf("  %s  %s\n", l, value))
	}

	b.WriteString("\n")
	field("Key", path.Base(obj.Key))
	field("Path", obj.Key)
	field("Size", fmt.Sprintf("%s (%d B)", output.FormatSize(obj.Size), obj.Size))
	field("Class", lipgloss.NewStyle().Foreground(storageClassColor(obj.StorageClass)).Render(storageClassLabel(obj.StorageClass)))
	field("Modified", obj.LastModified)
	if obj.ContentType != "" {
		field("Type", obj.ContentType)
	}
	if obj.ETag != "" {
		field("ETag", obj.ETag)
	}
	if isGlacier(obj.StorageClass) {
		label, color := restoreLabel(obj.RestoreStatus)
		if label == "" {
			label = "not restored"
			color = dim
		}
		field("Restore", lipgloss.NewStyle().Foreground(color).Render(label))
	}
	b.WriteString("\n")

	return b.String()
}

func (m Model) viewForm() string {
	var b strings.Builder
	b.WriteString("\n")
	for i := range m.formInputs {
		b.WriteString(m.formInputs[i].View())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewHelp() string {
	helpLines := []struct{ key, desc string }{
		{"1 / 2", "Switch tabs"},
		{"↑/k  ↓/j", "Navigate up / down"},
		{"enter / l", "Open prefix or view detail"},
		{"h / bksp", "Go to parent directory"},
		{"g", "Download selected object"},
		{"r", "Initiate Glacier restore"},
		{"b", "Switch bucket"},
		{"/", "Filter current listing"},
		{".", "Toggle secondary actions"},
		{"R", "Refresh listing"},
		{"s", "View object detail"},
		{"a", "Add bucket (Credentials)"},
		{"t", "Test connection (Credentials)"},
		{"d", "Remove bucket (Credentials)"},
		{"e", "Edit bucket (Credentials)"},
		{"D", "Set default bucket"},
		{"?", "Show this help"},
		{"q", "Quit (saves state)"},
	}

	var b strings.Builder
	b.WriteString("\n")
	for _, h := range helpLines {
		k := lipgloss.NewStyle().Foreground(cyan).Width(14).Render(h.key)
		b.WriteString(fmt.Sprintf("  %s  %s\n", k, h.desc))
	}
	return b.String()
}

// placeOverlay renders content in a centered bordered box over base.
func (m Model) placeOverlay(base, content, title string) string {
	contentLines := strings.Count(content, "\n") + 1
	w := min(60, m.width-4)
	h := min(contentLines+2, m.height-4)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyan).
		Width(w).
		Height(h).
		Render(content)

	// Inject title into top border line
	boxLines := strings.Split(box, "\n")
	if len(boxLines) > 0 {
		titleRendered := lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(" " + title + " ")
		top := []rune(boxLines[0])
		if len(top) > 4 {
			// Replace chars 2..2+titleLen with the rendered title
			plain := " " + title + " "
			plainLen := len([]rune(plain))
			if 2+plainLen < len(top) {
				boxLines[0] = string(top[:2]) + titleRendered + string(top[2+plainLen:])
			}
		}
	}
	box = strings.Join(boxLines, "\n")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.NoColor{}),
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m Model) filteredItems() []aws.ObjectInfo {
	filter := m.filterInput.Value()
	if filter == "" {
		return m.items
	}
	lower := strings.ToLower(filter)
	var result []aws.ObjectInfo
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(displayName(item.Key, m.prefix)), lower) {
			result = append(result, item)
		}
	}
	return result
}

func displayName(key, prefix string) string {
	name := strings.TrimPrefix(key, prefix)
	if name == "" {
		return key
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
