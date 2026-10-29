package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
)

// addWatchFlags registers --watch and --watch-interval on cmd, bound to a.Opts.
func (a *App) addWatchFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVarP(&a.Opts.Watch, "watch", "w", false, "re-run the command periodically")
	cmd.PersistentFlags().DurationVar(&a.Opts.WatchInterval, "watch-interval", 5*time.Second, "interval between watch refreshes (e.g. 5s, 30s, 1m)")
}

// watchOrRun executes fn once, or in a watch loop if --watch is set.
func (a *App) watchOrRun(fn func() error) error {
	if a.Opts.Watch {
		return a.runWatch(fn)
	}
	return fn()
}

// Returns nil only when interrupted by signal.
func (a *App) runWatch(fn func() error) error {
	// Run once immediately
	a.clearScreen()
	a.printWatchHeader()
	if err := fn(); err != nil {
		fmt.Fprintf(os.Stderr, "✗ %s\n", err)
	}

	ticker := time.NewTicker(a.Opts.WatchInterval)
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
			a.printWatchHeader()
			if err := fn(); err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s\n", err)
			}
		}
	}
}

func (a *App) clearScreen() {
	// Best effort: a failed terminal escape sequence is cosmetic only.
	_, _ = fmt.Fprint(a.Out, "\033[H\033[2J")
}

func (a *App) printWatchHeader() {
	fmt.Fprintf(os.Stderr, "Every %s — press Ctrl+C to stop\n\n", a.Opts.WatchInterval)
}
