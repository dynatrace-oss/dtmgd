package config

import (
	"fmt"
	"strings"
)

// SafetyLevel is how much dtmgd may change through a context. It is
// dtmgd's own guard, not a security boundary: a token without write scopes
// cannot write whatever the level says.
type SafetyLevel string

// The write levels, named as dtctl names them. A context without one is
// readonly, so a config written before writes existed stays read-only.
const (
	SafetyReadOnly      SafetyLevel = "readonly"
	SafetyReadWriteMine SafetyLevel = "readwrite-mine"
	SafetyReadWriteAll  SafetyLevel = "readwrite-all"
	SafetyUnrestricted  SafetyLevel = "dangerously-unrestricted"
)

// safetyLevels are the valid levels, least permissive first.
var safetyLevels = []SafetyLevel{SafetyReadOnly, SafetyReadWriteMine, SafetyReadWriteAll, SafetyUnrestricted}

// Operation is a kind of write a level allows or not.
type Operation string

// The writes dtmgd makes.
const (
	OpCreate Operation = "create"
	OpUpdate Operation = "update"
	OpDelete Operation = "delete"
)

// ParseSafetyLevel reads a context's safety-level: "" is readonly, and an
// unknown value is an error naming the valid ones, never a silent readonly.
func ParseSafetyLevel(s string) (SafetyLevel, error) {
	if s == "" {
		return SafetyReadOnly, nil
	}
	for _, l := range safetyLevels {
		if SafetyLevel(s) == l {
			return l, nil
		}
	}
	names := make([]string, len(safetyLevels))
	for i, l := range safetyLevels {
		names[i] = string(l)
	}
	return "", fmt.Errorf("unknown safety-level %q (want one of: %s)", s, strings.Join(names, ", "))
}

// Allows reports whether l permits op. readwrite-mine allows creating only:
// dtmgd cannot tell who owns a settings object, so it cannot allow changing
// "your own" ones. dangerously-unrestricted allows nothing more than
// readwrite-all today; it keeps dtctl's set of levels.
func (l SafetyLevel) Allows(op Operation) bool {
	switch l {
	case SafetyReadWriteMine:
		return op == OpCreate
	case SafetyReadWriteAll, SafetyUnrestricted:
		return true
	}
	return false
}

// Level is the context's write level (see ParseSafetyLevel).
func (c Context) Level() (SafetyLevel, error) {
	return ParseSafetyLevel(c.SafetyLevel)
}

// SetSafetyLevel sets the write level of the context named name.
func (c *Config) SetSafetyLevel(name string, level SafetyLevel) error {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			c.Contexts[i].Context.SafetyLevel = string(level)
			return nil
		}
	}
	return fmt.Errorf("context %q not found", name)
}
