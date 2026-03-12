package app

import "github.com/charmbracelet/bubbles/key"

// keyMap defines all keybindings and implements help.KeyMap.
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

	// Submenu actions
	Refresh key.Binding
	Stat    key.Binding
	Help    key.Binding

	// Credentials tab
	Add     key.Binding
	Test    key.Binding
	Delete  key.Binding
	Edit    key.Binding
	Default key.Binding

	// Tab switching
	Tab1 key.Binding
	Tab2 key.Binding

	// Dialog
	Confirm key.Binding
	Cancel  key.Binding
	Tab     key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter", "l"),
		key.WithHelp("enter", "open"),
	),
	Back: key.NewBinding(
		key.WithKeys("h", "backspace"),
		key.WithHelp("h", "back"),
	),
	Get: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "get"),
	),
	Restore: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "restore"),
	),
	Buckets: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "buckets"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	More: key.NewBinding(
		key.WithKeys("."),
		key.WithHelp(".", "more"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),

	// Submenu
	Refresh: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "refresh"),
	),
	Stat: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "stat"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),

	// Credentials
	Add: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "add bucket"),
	),
	Test: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "test"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "remove"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit"),
	),
	Default: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "set default"),
	),

	// Tabs
	Tab1: key.NewBinding(
		key.WithKeys("1"),
		key.WithHelp("1", "browse"),
	),
	Tab2: key.NewBinding(
		key.WithKeys("2"),
		key.WithHelp("2", "credentials"),
	),

	// Dialog
	Confirm: key.NewBinding(
		key.WithKeys("y", "Y"),
		key.WithHelp("y", "confirm"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("n", "N", "esc"),
		key.WithHelp("esc", "cancel"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("Tab", "next field"),
	),
}

// Help groups for different contexts.

type browseKeyMap struct{}

func (browseKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Enter, keys.Back, keys.Get, keys.Restore, keys.Buckets, keys.Filter, keys.More, keys.Quit}
}
func (browseKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{browseKeyMap{}.ShortHelp(), {keys.Refresh, keys.Stat, keys.Help}}
}

type browseSubmenuKeyMap struct{}

func (browseSubmenuKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Refresh, keys.Stat, keys.More, keys.Help}
}
func (browseSubmenuKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{browseSubmenuKeyMap{}.ShortHelp()}
}

type credentialsKeyMap struct{}

func (credentialsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Add, keys.Test, keys.Delete, keys.More, keys.Quit}
}
func (credentialsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{credentialsKeyMap{}.ShortHelp(), {keys.Edit, keys.Default, keys.Refresh, keys.Help}}
}

type credentialsSubmenuKeyMap struct{}

func (credentialsSubmenuKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Edit, keys.Default, keys.Refresh, keys.More, keys.Help}
}
func (credentialsSubmenuKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{credentialsSubmenuKeyMap{}.ShortHelp()}
}

type confirmKeyMap struct{}

func (confirmKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Confirm, keys.Cancel}
}
func (confirmKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{confirmKeyMap{}.ShortHelp()}
}

type formKeyMap struct{}

func (formKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Tab, keys.Enter, keys.Cancel}
}
func (formKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{formKeyMap{}.ShortHelp()}
}

type detailKeyMap struct{}

func (detailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Cancel, keys.Get, keys.Restore}
}
func (detailKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{detailKeyMap{}.ShortHelp()}
}

type bucketPickerKeyMap struct{}

func (bucketPickerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Enter, keys.Cancel}
}
func (bucketPickerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{bucketPickerKeyMap{}.ShortHelp()}
}

type filterKeyMap struct{}

func (filterKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter/esc", "done")),
	}
}
func (filterKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{filterKeyMap{}.ShortHelp()} }

type helpKeyMap struct{}

func (helpKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Cancel}
}
func (helpKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{helpKeyMap{}.ShortHelp()} }
