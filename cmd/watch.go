package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"time"
)

var (
	watchFlag     bool
	watchInterval time.Duration
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&watchFlag, "watch", "w", false, "re-run the command periodically")
	rootCmd.PersistentFlags().DurationVar(&watchInterval, "watch-interval", 5*time.Second, "interval between watch refreshes (e.g. 5s, 30s, 1m)")
}

// watchOrRun executes fn once, or in a watch loop if --watch is set.
func watchOrRun(fn func() error) error {
	if watchFlag {
		return runWatch(fn)
	}
	return fn()
}
// Returns nil only when interrupted by signal.
func runWatch(fn func() error) error {
	// Run once immediately
	clearScreen()
	printWatchHeader()
	if err := fn(); err != nil {
		fmt.Fprintf(os.Stderr, "✗ %s\n", err)
	}

	ticker := time.NewTicker(watchInterval)
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
			clearScreen()
			printWatchHeader()
			if err := fn(); err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s\n", err)
			}
		}
	}
}

func clearScreen() {
	fmt.Fprint(os.Stdout, "\033[H\033[2J")
}

func printWatchHeader() {
	fmt.Fprintf(os.Stderr, "Every %s — press Ctrl+C to stop\n\n", watchInterval)
}
