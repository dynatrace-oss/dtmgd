package output

import (
	"encoding/json"
	"io"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

// The agent envelope is dtctl's shared contract (sdk/agentmode), so an agent
// that drives both CLIs parses one shape:
//
//	{"ok": true,  "result": …, "context": {"verb", "resource", "total", "has_more", "warnings", "suggestions"}}
//	{"ok": false, "result": null, "error": {"code", "message", "status_code", "suggestions"}}

// DetectAgent reports whether dtmgd is running inside an AI agent, using the
// same signals as dtctl.
func DetectAgent() bool {
	return agentmode.Detect().Detected
}

// AgentPrinter writes results as agent envelopes.
type AgentPrinter struct {
	w   io.Writer
	ctx *agentmode.ResponseContext
}

// NewAgentPrinter returns a printer that writes success envelopes to w with
// ctx as their context.
func NewAgentPrinter(w io.Writer, ctx *agentmode.ResponseContext) *AgentPrinter {
	return &AgentPrinter{w: w, ctx: ctx}
}

func (p *AgentPrinter) Print(v interface{}) error {
	return agentmode.WriteSuccess(p.w, v, p.ctx)
}

func (p *AgentPrinter) PrintList(v interface{}) error {
	return p.Print(v)
}

// WriteAgentError writes an error envelope.
func WriteAgentError(w io.Writer, d *agentmode.ErrorDetail) error {
	return agentmode.WriteError(w, d)
}

// WriteAgentPartial writes a failure envelope that still carries a result:
// some environments of a multi-environment call answered and some did not.
func WriteAgentPartial(w io.Writer, result interface{}, d *agentmode.ErrorDetail, ctx *agentmode.ResponseContext) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(agentmode.Response{OK: false, Result: result, Error: d, Context: ctx})
}
