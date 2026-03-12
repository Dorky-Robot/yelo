package app

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Back    key.Binding
	Get     key.Binding
	Restore key.Binding
	Buckets key.Binding
	Filter  key.Binding
	More    key.Binding
	Quit    key.Binding

	// Submenu
	Refresh key.Binding
	Stat    key.Binding
	Help    key.Binding

	// Profiles tab
	Add        key.Binding
	AddSSO     key.Binding
	Test       key.Binding
	Delete     key.Binding
	Edit       key.Binding
	Default    key.Binding
	LinkBucket key.Binding

	// Tabs
	Tab1 key.Binding
	Tab2 key.Binding

	// Dialog
	Confirm key.Binding
	Cancel  key.Binding
	Tab     key.Binding
}

var keys = keyMap{
	Up:    key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "up")),
	Down:  key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "down")),
	Enter: key.NewBinding(key.WithKeys("enter", "l"), key.WithHelp("enter", "open")),
	Back:  key.NewBinding(key.WithKeys("h", "backspace"), key.WithHelp("h", "back")),
	Get:   key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "get")),

	Restore: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restore")),
	Buckets: key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "buckets")),
	Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	More:    key.NewBinding(key.WithKeys("."), key.WithHelp(".", "more")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

	Refresh: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
	Stat:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stat")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),

	Add:        key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "aws configure")),
	AddSSO:     key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "aws sso")),
	Test:       key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "test")),
	Delete:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "unlink")),
	Edit:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
	Default:    key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "set default")),
	LinkBucket: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "link bucket")),

	Tab1: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "browse")),
	Tab2: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "profiles")),

	Confirm: key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y", "confirm")),
	Cancel:  key.NewBinding(key.WithKeys("n", "N", "esc"), key.WithHelp("esc", "cancel")),
	Tab:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("Tab", "next field")),
}

// Help groups per context.

type browseKeyMap struct{}

func (browseKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Enter, keys.Back, keys.Get, keys.Restore, keys.Buckets, keys.Filter, keys.More, keys.Quit}
}
func (browseKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{browseKeyMap{}.ShortHelp()}
}

type browseSubmenuKeyMap struct{}

func (browseSubmenuKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Refresh, keys.Stat, keys.More, keys.Help}
}
func (browseSubmenuKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{browseSubmenuKeyMap{}.ShortHelp()}
}

type profilesKeyMap struct{}

func (profilesKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Add, keys.Test, keys.LinkBucket, keys.More, keys.Quit}
}
func (profilesKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{profilesKeyMap{}.ShortHelp()}
}

type profilesSubmenuKeyMap struct{}

func (profilesSubmenuKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.AddSSO, keys.Delete, keys.Default, keys.Refresh, keys.More, keys.Help}
}
func (profilesSubmenuKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{profilesSubmenuKeyMap{}.ShortHelp()}
}

type confirmKeyMap struct{}

func (confirmKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Confirm, keys.Cancel}
}
func (confirmKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{confirmKeyMap{}.ShortHelp()} }

type formKeyMap struct{}

func (formKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Tab, keys.Enter, keys.Cancel}
}
func (formKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{formKeyMap{}.ShortHelp()} }

type detailKeyMap struct{}

func (detailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Cancel, keys.Get, keys.Restore}
}
func (detailKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{detailKeyMap{}.ShortHelp()} }

type bucketPickerKeyMap struct{}

func (bucketPickerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Enter, keys.Cancel}
}
func (bucketPickerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{bucketPickerKeyMap{}.ShortHelp()}
}

type filterKeyMap struct{}

func (filterKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter/esc", "done"))}
}
func (filterKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{filterKeyMap{}.ShortHelp()} }

type helpKeyMap struct{}

func (helpKeyMap) ShortHelp() []key.Binding { return []key.Binding{keys.Cancel} }
func (helpKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{helpKeyMap{}.ShortHelp()} }
