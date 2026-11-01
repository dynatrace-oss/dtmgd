package config

import (
	"strings"
	"testing"
)

func TestParseSafetyLevel(t *testing.T) {
	if l, err := ParseSafetyLevel(""); err != nil || l != SafetyReadOnly {
		t.Errorf(`"" = %q, %v; want readonly`, l, err)
	}
	for _, s := range []string{"readonly", "readwrite-mine", "readwrite-all", "dangerously-unrestricted"} {
		if l, err := ParseSafetyLevel(s); err != nil || string(l) != s {
			t.Errorf("%q = %q, %v", s, l, err)
		}
	}
	_, err := ParseSafetyLevel("read-write")
	if err == nil {
		t.Fatal("read-write parsed")
	}
	for _, s := range []string{"readonly", "readwrite-mine", "readwrite-all", "dangerously-unrestricted"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not name %s", err, s)
		}
	}
}

func TestSafetyLevelAllows(t *testing.T) {
	want := map[SafetyLevel][3]bool{ // create, update, delete
		SafetyReadOnly:      {false, false, false},
		SafetyReadWriteMine: {true, false, false},
		SafetyReadWriteAll:  {true, true, true},
		SafetyUnrestricted:  {true, true, true},
	}
	for level, w := range want {
		for i, op := range []Operation{OpCreate, OpUpdate, OpDelete} {
			if got := level.Allows(op); got != w[i] {
				t.Errorf("%s.Allows(%s) = %v", level, op, got)
			}
		}
	}
}

func TestSetSafetyLevel(t *testing.T) {
	cfg := &Config{Contexts: []NamedContext{{Name: "a", Context: Context{Host: "https://cluster.example.com", EnvID: "e"}}}}
	if err := cfg.SetSafetyLevel("a", SafetyReadWriteAll); err != nil || cfg.Contexts[0].Context.SafetyLevel != "readwrite-all" {
		t.Errorf("err %v, level %q", err, cfg.Contexts[0].Context.SafetyLevel)
	}
	if l, err := cfg.Contexts[0].Context.Level(); err != nil || l != SafetyReadWriteAll {
		t.Errorf("Level() = %q, %v", l, err)
	}
	if err := cfg.SetSafetyLevel("nope", SafetyReadOnly); err == nil {
		t.Error("an unknown context was accepted")
	}
}
