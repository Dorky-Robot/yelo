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

type activeTab int

const (
	tabBrowse activeTab = iota
	tabProfiles
)

type mode int

const (
	modeNormal mode = iota
	modeHelp
	modeBucketPicker
	modeConfirm
	modeFilter
	modeDetail
	modeLinkBucket // form: link a bucket to a profile
)

type confirmAction int

const (
	confirmDownload confirmAction = iota
	confirmRestore
	confirmUnlinkBucket
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type Model struct {
	cfg    *config.Config
	st     *state.State
	client aws.S3Client

	bucket string
	prefix string
	items  []aws.ObjectInfo

	tab     activeTab
	mode    mode
	submenu bool

	browseTable  table.Model
	profileTable table.Model
	spinner      spinner.Model
	help         help.Model
	filterInput  textinput.Model

	// Link bucket form (3 fields: bucket name, region, profile)
	formInputs [3]textinput.Model
	formFocus  int

	bucketPicker table.Model
	bucketList   []string

	confirmWhat   confirmAction
	confirmTarget string

	detail *aws.ObjectInfo

	profiles      []string
	profileStatus map[string]string // profile name → "ok"/"fail"/"testing"

	loading   string
	statusMsg string

	width  int
	height int
}

func NewModel(cfg *config.Config, st *state.State, client aws.S3Client, bucket string) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cyan)

	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("8")).Padding(0, 1)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(dim).PaddingRight(1)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(dim)
	h.ShortSeparator = ""

	bt := table.New(
		table.WithColumns(browseColumns(80)),
		table.WithFocused(true),
		table.WithHeight(10),
		table.WithStyles(tableStyles()),
	)

	pt := table.New(
		table.WithColumns(profileColumns(80)),
		table.WithFocused(false),
		table.WithHeight(10),
		table.WithStyles(tableStyles()),
	)

	fi := textinput.New()
	fi.Prompt = "filter: "
	fi.PromptStyle = lipgloss.NewStyle().Foreground(cyan)
	fi.TextStyle = lipgloss.NewStyle().Foreground(white)
	fi.Placeholder = "type to filter..."
	fi.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)

	bucketInput := textinput.New()
	bucketInput.Prompt = "  Bucket:   "
	bucketInput.PromptStyle = lipgloss.NewStyle().Foreground(cyan)
	bucketInput.Placeholder = "my-bucket"
	bucketInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)

	regionInput := textinput.New()
	regionInput.Prompt = "  Region:   "
	regionInput.PromptStyle = lipgloss.NewStyle().Foreground(dim)
	regionInput.Placeholder = "us-east-1 (optional)"
	regionInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)

	profileInput := textinput.New()
	profileInput.Prompt = "  Profile:  "
	profileInput.PromptStyle = lipgloss.NewStyle().Foreground(dim)
	profileInput.Placeholder = "(from selected row)"
	profileInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(dim)

	bp := table.New(
		table.WithColumns([]table.Column{{Title: "Bucket", Width: 40}}),
		table.WithFocused(true),
		table.WithHeight(10),
		table.WithStyles(tableStyles()),
	)

	return Model{
		cfg:          cfg,
		st:           st,
		client:       client,
		bucket:       bucket,
		prefix:       st.Prefix,
		tab:          tabBrowse,
		browseTable:  bt,
		profileTable: pt,
		spinner:      sp,
		help:         h,
		filterInput:  fi,
		formInputs:   [3]textinput.Model{bucketInput, regionInput, profileInput},
		bucketPicker: bp,
		profileStatus: map[string]string{},
	}
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
		m.rebuildProfileTable()
		return m, nil

	case profileTestMsg:
		if msg.ok {
			m.profileStatus[msg.bucket] = "ok"
			m.statusMsg = fmt.Sprintf("'%s' connected", msg.bucket)
		} else {
			m.profileStatus[msg.bucket] = "fail"
			m.statusMsg = fmt.Sprintf("'%s' failed: %v", msg.bucket, msg.err)
		}
		m.rebuildProfileTable()
		return m, clearFlashAfter(5 * time.Second)

	case awsConfigDoneMsg:
		// Returned from `aws configure` — reload profiles
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("aws configure: %v", msg.err)
		} else {
			m.statusMsg = "Profile configured"
		}
		// Reload profiles to pick up changes
		return m, tea.Batch(loadProfiles(), clearFlashAfter(3*time.Second))

	case clearFlashMsg:
		m.statusMsg = ""
		return m, nil

	case tea.KeyMsg:
		if m.loading != "" {
			return m, nil
		}
		return m.handleKey(msg)
	}

	if m.mode == modeFilter {
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.mode == modeLinkBucket {
		var cmd tea.Cmd
		m.formInputs[m.formFocus], cmd = m.formInputs[m.formFocus].Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) resizeTables() {
	contentH := m.height - 5
	if contentH < 3 {
		contentH = 3
	}
	m.browseTable.SetColumns(browseColumns(m.width))
	m.browseTable.SetWidth(m.width)
	m.browseTable.SetHeight(contentH)

	m.profileTable.SetColumns(profileColumns(m.width))
	m.profileTable.SetWidth(m.width)
	m.profileTable.SetHeight(contentH)

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
	case modeLinkBucket:
		return m.handleLinkForm(msg)
	case modeNormal:
		if key.Matches(msg, keys.Tab1) {
			m.tab = tabBrowse
			m.submenu = false
			m.browseTable.Focus()
			m.profileTable.Blur()
			return m, nil
		}
		if key.Matches(msg, keys.Tab2) {
			m.tab = tabProfiles
			m.submenu = false
			m.profileTable.Focus()
			m.browseTable.Blur()
			if len(m.profiles) == 0 {
				m.loading = "Loading profiles..."
				return m, loadProfiles()
			}
			return m, nil
		}
		switch m.tab {
		case tabBrowse:
			if m.submenu {
				return m.handleBrowseSubmenu(msg)
			}
			return m.handleBrowse(msg)
		case tabProfiles:
			if m.submenu {
				return m.handleProfilesSubmenu(msg)
			}
			return m.handleProfiles(msg)
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
		filtered := m.filteredItems()
		idx := m.browseTable.Cursor()
		if idx < 0 || idx >= len(filtered) {
			return m, nil
		}
		item := filtered[idx]
		if item.IsPrefix {
			m.prefix = item.Key
			m.loading = "Loading..."
			return m, tea.Batch(m.spinner.Tick, fetchList(m.client, m.bucket, m.prefix))
		}
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
		return m, m.filterInput.Focus()
	case key.Matches(msg, keys.More):
		m.submenu = true
	default:
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
		if idx >= 0 && idx < len(filtered) && !filtered[idx].IsPrefix {
			m.loading = "Loading metadata..."
			return m, tea.Batch(m.spinner.Tick, fetchDetail(m.client, m.bucket, filtered[idx].Key))
		}
	case key.Matches(msg, keys.Help):
		m.mode = modeHelp
		m.submenu = false
	}
	return m, nil
}

func (m Model) handleProfiles(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		m.saveState()
		return m, tea.Quit

	case key.Matches(msg, keys.Add):
		// Shell out to `aws configure` for the selected profile
		profile := m.selectedProfile()
		if profile == "" {
			profile = "default"
		}
		return m, runAWSConfigure(profile)

	case key.Matches(msg, keys.Test):
		profile := m.selectedProfile()
		if profile != "" {
			m.profileStatus[profile] = "testing"
			m.rebuildProfileTable()
			m.statusMsg = fmt.Sprintf("Testing '%s'...", profile)
			return m, testProfile(profile, profile)
		}

	case key.Matches(msg, keys.LinkBucket):
		m.beginLinkBucket()
		return m, m.formInputs[0].Focus()

	case key.Matches(msg, keys.More):
		m.submenu = true

	default:
		var cmd tea.Cmd
		m.profileTable, cmd = m.profileTable.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleProfilesSubmenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.More):
		m.submenu = false

	case key.Matches(msg, keys.AddSSO):
		profile := m.selectedProfile()
		if profile == "" {
			profile = "default"
		}
		m.submenu = false
		return m, runAWSConfigureSSO(profile)

	case key.Matches(msg, keys.Delete):
		// Unlink bucket from yelo config (doesn't touch AWS creds)
		idx := m.profileTable.Cursor()
		if idx >= 0 && idx < len(m.profiles) {
			profile := m.profiles[idx]
			// Find buckets linked to this profile and offer to unlink
			for _, b := range m.cfg.Buckets {
				if b.Profile == profile || (b.Profile == "" && profile == "default") {
					m.confirmWhat = confirmUnlinkBucket
					m.confirmTarget = b.Name
					m.mode = modeConfirm
					m.submenu = false
					return m, nil
				}
			}
			m.statusMsg = fmt.Sprintf("No buckets linked to '%s'", profile)
			m.submenu = false
			return m, clearFlashAfter(3 * time.Second)
		}

	case key.Matches(msg, keys.Default):
		// Set the default bucket for this profile
		for _, b := range m.cfg.Buckets {
			profile := m.selectedProfile()
			if b.Profile == profile || (b.Profile == "" && profile == "default") {
				_ = m.cfg.SetDefault(b.Name)
				_ = m.cfg.Save()
				m.statusMsg = fmt.Sprintf("Default set to '%s'", b.Name)
				m.rebuildProfileTable()
				m.submenu = false
				return m, clearFlashAfter(3 * time.Second)
			}
		}
		m.submenu = false

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
		if idx >= 0 && idx < len(m.bucketList) {
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
		case confirmUnlinkBucket:
			m.cfg.RemoveBucket(m.confirmTarget)
			_ = m.cfg.Save()
			m.statusMsg = fmt.Sprintf("Unlinked '%s'", m.confirmTarget)
			m.rebuildProfileTable()
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
			k := m.detail.Key
			class := m.detail.StorageClass
			restore := m.detail.RestoreStatus
			m.detail = nil
			if isGlacier(class) && restore != "available" {
				m.statusMsg = "Object is in Glacier — restore it first (r)"
				return m, clearFlashAfter(3 * time.Second)
			}
			m.confirmWhat = confirmDownload
			m.confirmTarget = k
			m.mode = modeConfirm
		}
	case key.Matches(msg, keys.Restore):
		if m.detail != nil && isGlacier(m.detail.StorageClass) {
			m.confirmWhat = confirmRestore
			m.confirmTarget = m.detail.Key
			m.mode = modeConfirm
			m.detail = nil
		}
	}
	return m, nil
}

func (m Model) handleLinkForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			m.statusMsg = fmt.Sprintf("Linked bucket '%s' → profile '%s'", name, profile)
			m.mode = modeNormal
			m.rebuildProfileTable()
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
	if idx < 0 || idx >= len(filtered) || filtered[idx].IsPrefix {
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
	if idx < 0 || idx >= len(filtered) || filtered[idx].IsPrefix {
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

func (m *Model) beginLinkBucket() {
	m.mode = modeLinkBucket
	m.formFocus = 0
	for i := range m.formInputs {
		m.formInputs[i].SetValue("")
	}
	// Pre-fill profile from selected row
	profile := m.selectedProfile()
	if profile != "" {
		m.formInputs[2].SetValue(profile)
	}
	m.updateFormStyles()
}

func (m Model) selectedProfile() string {
	idx := m.profileTable.Cursor()
	if idx >= 0 && idx < len(m.profiles) {
		return m.profiles[idx]
	}
	return ""
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

func profileColumns(w int) []table.Column {
	nameW := 20
	bucketW := 24
	regionW := 16
	statusW := w - nameW - bucketW - regionW - 4
	if statusW < 10 {
		statusW = 10
	}
	return []table.Column{
		{Title: "Profile", Width: nameW},
		{Title: "Linked Bucket", Width: bucketW},
		{Title: "Region", Width: regionW},
		{Title: "Status", Width: statusW},
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

func (m *Model) rebuildProfileTable() {
	// Build a lookup of profile → bucket config
	bucketByProfile := map[string]*config.BucketConfig{}
	for i := range m.cfg.Buckets {
		b := &m.cfg.Buckets[i]
		p := b.Profile
		if p == "" {
			p = "default"
		}
		bucketByProfile[p] = b
	}

	rows := make([]table.Row, len(m.profiles))
	for i, profile := range m.profiles {
		bucket := ""
		region := ""
		if b, ok := bucketByProfile[profile]; ok {
			bucket = b.Name
			if b.Name == m.cfg.DefaultBucket {
				bucket += " *"
			}
			region = b.Region
		}
		if bucket == "" {
			bucket = lipgloss.NewStyle().Foreground(dim).Render("(none)")
		}
		if region == "" {
			region = lipgloss.NewStyle().Foreground(dim).Render("(default)")
		}

		status := ""
		switch m.profileStatus[profile] {
		case "ok":
			status = lipgloss.NewStyle().Foreground(green).Render("● connected")
		case "fail":
			status = lipgloss.NewStyle().Foreground(red).Render("● failed")
		case "testing":
			status = lipgloss.NewStyle().Foreground(yellow).Render("● testing...")
		}

		rows[i] = table.Row{profile, bucket, region, status}
	}
	m.profileTable.SetRows(rows)
}

func (m *Model) rebuildBucketPicker() {
	rows := make([]table.Row, len(m.bucketList))
	for i, b := range m.bucketList {
		label := "  " + b
		if b == m.bucket {
			label = "* " + b
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

	usedH := lipgloss.Height(header) + lipgloss.Height(statusBar) + lipgloss.Height(helpBar)
	contentH := m.height - usedH
	if contentH < 1 {
		contentH = 1
	}
	content = lipgloss.NewStyle().Height(contentH).MaxHeight(contentH).Width(m.width).Render(content)

	view := lipgloss.JoinVertical(lipgloss.Left, header, content, statusBar, helpBar)

	switch m.mode {
	case modeBucketPicker:
		view = m.placeOverlay(view, m.viewBucketPicker(), "Select Bucket")
	case modeConfirm:
		view = m.placeOverlay(view, m.viewConfirm(), "Confirm")
	case modeDetail:
		view = m.placeOverlay(view, m.viewDetail(), "Object Detail")
	case modeLinkBucket:
		view = m.placeOverlay(view, m.viewLinkForm(), "Link Bucket to Profile")
	case modeHelp:
		view = m.placeOverlay(view, m.viewHelp(), "Help")
	}

	return view
}

func (m Model) viewHeader() string {
	renderTab := func(label, num string, active bool) string {
		numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("8")).Padding(0, 1)
		labelStyle := lipgloss.NewStyle().Foreground(dim)
		if active {
			numStyle = numStyle.Background(cyan)
			labelStyle = labelStyle.Foreground(cyan).Bold(true)
		}
		return numStyle.Render(num) + labelStyle.Render(" "+label+" ")
	}

	title := lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(" yelo ")
	tabs := title + "  " + renderTab("Browse", "1", m.tab == tabBrowse) + " " + renderTab("Profiles", "2", m.tab == tabProfiles)
	border := lipgloss.NewStyle().Foreground(dim).Render(strings.Repeat("─", m.width))
	return tabs + "\n" + border
}

func (m Model) viewContent() string {
	if m.loading != "" {
		return lipgloss.NewStyle().Padding(1, 2).Render(m.spinner.View() + " " + m.loading)
	}

	switch m.tab {
	case tabBrowse:
		if m.bucket == "" {
			return lipgloss.NewStyle().Foreground(dim).Padding(1, 2).Render("No bucket selected. Press b to pick a bucket.")
		}
		if m.mode == modeFilter {
			return m.filterInput.View() + "\n" + m.browseTable.View()
		}
		return m.browseTable.View()

	case tabProfiles:
		if len(m.profiles) == 0 {
			return lipgloss.NewStyle().Foreground(dim).Padding(1, 2).Render(
				"No AWS profiles found.\n\n" +
					"Run `aws configure` to set up credentials, or press a to launch it now.\n" +
					"Profiles are read from ~/.aws/credentials and ~/.aws/config.",
			)
		}
		return m.profileTable.View()
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
	} else if m.tab == tabProfiles {
		line = lipgloss.NewStyle().Foreground(dim).Padding(0, 1).Render("Profiles from ~/.aws/ — credentials managed by AWS CLI")
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
	case modeLinkBucket:
		return m.help.View(formKeyMap{})
	case modeNormal:
		switch {
		case m.tab == tabBrowse && m.submenu:
			return m.help.View(browseSubmenuKeyMap{})
		case m.tab == tabBrowse:
			return m.help.View(browseKeyMap{})
		case m.tab == tabProfiles && m.submenu:
			return m.help.View(profilesSubmenuKeyMap{})
		case m.tab == tabProfiles:
			return m.help.View(profilesKeyMap{})
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Overlay content
// ---------------------------------------------------------------------------

func (m Model) viewBucketPicker() string { return m.bucketPicker.View() }

func (m Model) viewConfirm() string {
	action := "download"
	switch m.confirmWhat {
	case confirmRestore:
		action = "restore"
	case confirmUnlinkBucket:
		action = "unlink"
	}
	return fmt.Sprintf("\n  %s '%s' ?\n\n  %s\n",
		action, path.Base(m.confirmTarget),
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

func (m Model) viewLinkForm() string {
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
	lines := []struct{ key, desc string }{
		{"1 / 2", "Switch tabs (Browse / Profiles)"},
		{"↑/k  ↓/j", "Navigate"},
		{"enter / l", "Open prefix or view detail"},
		{"h / bksp", "Go to parent directory"},
		{"g", "Download selected object"},
		{"r", "Initiate Glacier restore"},
		{"b", "Switch bucket"},
		{"/", "Filter listing"},
		{".", "Toggle secondary actions"},
		{"", ""},
		{"a", "Run `aws configure` for selected profile"},
		{"S", "Run `aws configure sso`"},
		{"t", "Test profile connectivity"},
		{"l", "Link a bucket to a profile"},
		{"d", "Unlink bucket from profile"},
		{"D", "Set default bucket"},
		{"", ""},
		{"R", "Refresh"},
		{"?", "Show this help"},
		{"q", "Quit (saves state)"},
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, h := range lines {
		if h.key == "" {
			b.WriteString("\n")
			continue
		}
		k := lipgloss.NewStyle().Foreground(cyan).Width(14).Render(h.key)
		b.WriteString(fmt.Sprintf("  %s  %s\n", k, h.desc))
	}
	return b.String()
}

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

	boxLines := strings.Split(box, "\n")
	if len(boxLines) > 0 {
		titleRendered := lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(" " + title + " ")
		top := []rune(boxLines[0])
		plainLen := len([]rune(" " + title + " "))
		if len(top) > 2+plainLen {
			boxLines[0] = string(top[:2]) + titleRendered + string(top[2+plainLen:])
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

func displayName(k, prefix string) string {
	name := strings.TrimPrefix(k, prefix)
	if name == "" {
		return k
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
