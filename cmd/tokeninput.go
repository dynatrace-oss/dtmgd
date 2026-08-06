package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

const (
	flagToken      = "token"
	flagTokenStdin = "token-stdin"
)

// tokenInput is the decision the CLI makes about where a token value comes
// from, separated from the process globals so it can be exercised without a
// controlling terminal.
type tokenInput struct {
	// flagValue is the --token value, and flagChanged records whether the
	// flag was present at all. Both are needed: --token "" is a user error
	// worth reporting, while an absent flag means "ask me".
	flagValue   string
	flagChanged bool
	// fromStdin is --token-stdin.
	fromStdin bool
	// stdin is where --token-stdin reads from.
	stdin io.Reader
	// prompt reads a secret interactively with echo disabled. It is nil when
	// no terminal is attached, which is what makes the non-interactive error
	// reachable in tests.
	prompt func() (string, error)
}

// resolve returns the token value, or an error explaining which input to use.
func (in tokenInput) resolve() (string, error) {
	switch {
	case in.fromStdin && in.flagChanged:
		return "", fmt.Errorf("--%s and --%s cannot be combined; choose one", flagToken, flagTokenStdin)

	case in.fromStdin:
		return readTokenFrom(in.stdin)

	case in.flagChanged:
		if in.flagValue == "" {
			return "", fmt.Errorf("--%s was given an empty value", flagToken)
		}
		// The token is already in the argument vector by the time any Go code
		// runs — the kernel copies it there at execve(2), and nothing in this
		// process can remove it from /proc/<pid>/cmdline. So this warns rather
		// than protects: the value in hand is compromised, and the point is to
		// stop the next one from being.
		output.PrintWarning(
			"--%s puts the token in the process list and your shell history. Use --%s, or omit the flag to be prompted.",
			flagToken, flagTokenStdin)
		return in.flagValue, nil

	case in.prompt != nil:
		return in.prompt()
	}

	return "", fmt.Errorf(
		"no token supplied. Pipe one in with --%s, or run this command from a terminal to be prompted",
		flagTokenStdin)
}

// readTokenFrom consumes a token from r, which is stdin in production.
//
// Whitespace is trimmed because the common invocations all add some — a
// here-string, `echo` without -n, or a secrets file ending in a newline — and
// no Dynatrace API token contains any.
func readTokenFrom(r io.Reader) (string, error) {
	if r == nil {
		return "", fmt.Errorf("--%s was requested but no input is available", flagTokenStdin)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read token from stdin: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("no token found on stdin")
	}
	return token, nil
}

// promptForToken reads a token from the terminal with echo disabled, so it
// reaches neither the screen nor the shell's history.
func promptForToken() (string, error) {
	fmt.Fprint(os.Stderr, "API token: ")
	data, err := term.ReadPassword(int(os.Stdin.Fd()))
	// ReadPassword swallows the newline the user typed, so without this the
	// next line of output starts on the prompt line.
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("failed to read token: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("no token entered")
	}
	return token, nil
}

// tokenInputFromCmd wires the flags and process state into a tokenInput.
func tokenInputFromCmd(cmd *cobra.Command) tokenInput {
	value, _ := cmd.Flags().GetString(flagToken)
	fromStdin, _ := cmd.Flags().GetBool(flagTokenStdin)

	in := tokenInput{
		flagValue:   value,
		flagChanged: cmd.Flags().Changed(flagToken),
		fromStdin:   fromStdin,
		stdin:       os.Stdin,
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		in.prompt = promptForToken
	}
	return in
}
