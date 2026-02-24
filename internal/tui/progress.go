package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/dorkyrobot/yelo/internal/aws"
	"github.com/dorkyrobot/yelo/internal/output"
)

// NewProgressFunc returns a ProgressFunc that renders a progress bar to stderr.
// If stderr is not a TTY, returns nil (no progress output).
func NewProgressFunc(label string) aws.ProgressFunc {
	if !output.IsTTY(os.Stderr) {
		return nil
	}

	return func(transferred, total int64) {
		if total <= 0 {
			fmt.Fprintf(os.Stderr, "\r%s: %s", label, output.FormatSize(transferred))
			return
		}

		pct := float64(transferred) / float64(total)
		barWidth := 30
		filled := int(pct * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}

		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		fmt.Fprintf(os.Stderr, "\r%s: [%s] %3.0f%% %s/%s",
			label, bar, pct*100,
			output.FormatSize(transferred),
			output.FormatSize(total),
		)

		if transferred >= total {
			fmt.Fprintln(os.Stderr)
		}
	}
}
