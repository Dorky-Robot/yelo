package app

// keybinding definitions for rendering the context-sensitive bar.
type binding struct {
	key  string
	desc string
}

// Keybindings per context.
var (
	browseKeys = []binding{
		{"j/k", "nav"}, {"enter", "open"}, {"h", "back"},
		{"g", "get"}, {"r", "restore"}, {"b", "buckets"},
		{"/", "filter"}, {".", "more"}, {"q", "quit"},
	}

	browseSubmenuKeys = []binding{
		{"R", "refresh"}, {"s", "stat"}, {".", "back"}, {"?", "help"},
	}

	bucketPickerKeys = []binding{
		{"j/k", "nav"}, {"enter", "select"}, {"esc", "cancel"},
	}

	credentialsKeys = []binding{
		{"j/k", "nav"}, {"a", "add bucket"}, {"t", "test"},
		{"d", "remove"}, {".", "more"}, {"q", "quit"},
	}

	credentialsSubmenuKeys = []binding{
		{"e", "edit"}, {"D", "set default"}, {"R", "refresh"}, {".", "back"}, {"?", "help"},
	}

	confirmKeys = []binding{
		{"y", "confirm"}, {"n/esc", "cancel"},
	}

	inputKeys = []binding{
		{"Tab", "next field"}, {"Enter", "confirm"}, {"Esc", "cancel"},
	}

	helpKeys = []binding{
		{"esc/q", "close"},
	}

	filterKeys = []binding{
		{"enter/esc", "done"}, {"type", "filter"},
	}

	detailKeys = []binding{
		{"esc", "close"}, {"g", "get"}, {"r", "restore"},
	}
)
