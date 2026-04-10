package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// AgentEnvVars lists environment variables that indicate an AI agent context.
var AgentEnvVars = []string{
	"CLAUDECODE",
	"CURSOR_AGENT",
	"GITHUB_COPILOT",
	"AMAZON_Q",
	"KIRO",
	"AI_AGENT",
	"JUNIE",
	"OPENCODE",
}

// DetectAgent returns true if the process is running inside an AI agent.
func DetectAgent() bool {
	for _, v := range AgentEnvVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

// AgentResponse is the structured envelope for AI agent output.
type AgentResponse struct {
	OK      bool                   `json:"ok"`
	Result  interface{}            `json:"result,omitempty"`
	Error   *AgentError            `json:"error,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// AgentError represents an error in agent envelope mode.
type AgentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AgentPrinter wraps output in a structured JSON envelope for AI agents.
type AgentPrinter struct {
	w        io.Writer
	resource string
}

// NewAgentPrinter creates a printer that outputs agent envelopes.
func NewAgentPrinter(resource string) *AgentPrinter {
	return &AgentPrinter{w: os.Stdout, resource: resource}
}

func (p *AgentPrinter) Print(v interface{}) error {
	return p.encode(AgentResponse{
		OK:     true,
		Result: v,
		Context: map[string]interface{}{
			"resource": p.resource,
		},
	})
}

func (p *AgentPrinter) PrintList(v interface{}) error {
	return p.Print(v)
}

func (p *AgentPrinter) encode(resp AgentResponse) error {
	enc := json.NewEncoder(p.w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

// PrintAgentError prints an error in agent envelope format.
func PrintAgentError(err error) {
	resp := AgentResponse{
		OK:    false,
		Error: &AgentError{Code: "error", Message: err.Error()},
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Fprintln(os.Stdout, string(data))
}
