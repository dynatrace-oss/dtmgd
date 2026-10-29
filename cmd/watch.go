package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// watchOptions are a watchable command's --watch flags.
type watchOptions struct {
	on       bool
	interval time.Duration
}

// addWatchFlags registers --watch and --watch-interval on cmd. Only commands
// that re-run meaningfully (get problems, get events) have them; elsewhere
// the flags used to be accepted and silently ignored.
func addWatchFlags(cmd *cobra.Command, w *watchOptions) {
	cmd.Flags().BoolVarP(&w.on, "watch", "w", false, "re-run the command periodically")
	cmd.Flags().DurationVar(&w.interval, "watch-interval", 5*time.Second, "interval between watch refreshes (e.g. 5s, 30s, 1m)")
}

// watchOrRun executes fn once, or in a watch loop if --watch is set.
func (a *App) watchOrRun(w watchOptions, fn func() error) error {
	if w.on {
		return a.runWatch(w, fn)
	}
	return fn()
}

// Returns nil only when interrupted by signal.
func (a *App) runWatch(w watchOptions, fn func() error) error {
	// Run once immediately
	a.clearScreen()
	a.printWatchHeader(w)
	if err := fn(); err != nil {
		output.PrintHumanError("%s", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr)
			return nil
		case <-ticker.C:
			a.clearScreen()
			a.printWatchHeader(w)
			if err := fn(); err != nil {
				output.PrintHumanError("%s", err)
			}
		}
	}
}

// clearScreen clears the terminal between watch ticks. It only does so for a
// table on a terminal: in JSON, YAML or agent output the escape sequence would
// corrupt the stream, and in plain mode there is no terminal to clear.
func (a *App) clearScreen() {
	if f := a.format(); (f != output.FormatTable && f != output.FormatWide) || a.agentMode() || a.plain() {
		return
	}
	// Best effort: a failed terminal escape sequence is cosmetic only.
	_, _ = fmt.Fprint(a.Out, "\033[H\033[2J")
}

func (a *App) printWatchHeader(w watchOptions) {
	output.PrintInfo("Every %s — press Ctrl+C to stop\n", w.interval)
}
