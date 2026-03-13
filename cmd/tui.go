package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func init() {
	register("tui", runTUI, "tui — interactive terminal UI")
}

func runTUI(env *Env, args []string) error {
	bin, err := exec.LookPath("yelo")
	if err != nil {
		return fmt.Errorf("yelo (Rust binary) not found in PATH — install with: cargo install --path . (from the yelo repo)")
	}

	// Replace this process with the Rust yelo binary in TUI mode.
	return syscall.Exec(bin, append([]string{bin, "tui"}, args...), os.Environ())
}
