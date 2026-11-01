package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

// runInteractive runs dtmgd like runCLI, but as if a person sat at a
// terminal typing stdin.
func runInteractive(t *testing.T, cfgPath, stdin string, args ...string) cliResult {
	t.Helper()
	for _, v := range agentmode.EnvVars() {
		t.Setenv(v, "")
	}
	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb, args: append(args, "--config", cfgPath)}
	a.interactive = func() bool { return true }
	a.stdin = strings.NewReader(stdin)
	code := a.run()
	return cliResult{Stdout: out.String(), Stderr: errb.String(), Code: code}
}

// editorScript sets EDITOR to a script that replaces the edited file with
// content.
func editorScript(t *testing.T, content string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake editor is a shell script")
	}
	path := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '"+content+"' > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", path)
	t.Setenv("VISUAL", "")
}

func editFake(t *testing.T, version string) (*fakeAPI, string) {
	t.Helper()
	return applyFake(t, "readwrite-all", map[string]route{
		objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default"),
		putPrefix + "obj-alerting-1": {Body: `{"code":200,"objectId":"obj-alerting-1"}`},
		clusterVersionPath:           {Body: nodeList(version)},
	})
}

func TestEditRefusesWithoutATerminal(t *testing.T) {
	api, cfg := editFake(t, "1.300.0.20250101-000000")
	if r := runCLI(t, cfg, "edit", "setting", "obj-alerting-1"); r.Code != 2 || !strings.Contains(r.Stderr, "apply") {
		t.Errorf("no terminal: %s", r)
	}
	if r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "edit", "setting", "obj-alerting-1"); r.Code != 2 {
		t.Errorf("agent mode: %s", r)
	}
	if api.totalHits() != 0 {
		t.Errorf("%d requests, want none", api.totalHits())
	}
}

func TestEditUpdatesTheObject(t *testing.T) {
	editorScript(t, `name: changed\n`)
	api, cfg := editFake(t, "1.300.0.20250101-000000")
	r := runInteractive(t, cfg, "", "edit", "setting", "obj-alerting-1", "-o", "json")
	if rep := decodeReport(t, r.Stdout); r.Code != 0 || actions(rep) != "updated" {
		t.Fatalf("%s", r)
	}
	b := putBody(t, api, "obj-alerting-1")
	if b["value"].(map[string]interface{})["name"] != "changed" || b["updateToken"] != "server-obj-alerting-1" {
		t.Errorf("PUT body %v", b)
	}
}

func TestEditUnchangedSendsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses true(1) as the editor")
	}
	t.Setenv("EDITOR", "true")
	api, cfg := editFake(t, "1.300.0.20250101-000000")
	r := runInteractive(t, cfg, "", "edit", "setting", "obj-alerting-1")
	if r.Code != 0 || !strings.Contains(r.Stderr, "Edit cancelled") || api.Hits(putPrefix+"obj-alerting-1") != 0 {
		t.Errorf("%s", r)
	}
}

func TestEditValidateOnly(t *testing.T) {
	editorScript(t, `name: changed\n`)
	api, cfg := editFake(t, "1.300.0.20250101-000000")
	if r := runInteractive(t, cfg, "", "edit", "setting", "obj-alerting-1", "--validate-only"); r.Code != 0 {
		t.Fatalf("%s", r)
	}
	if qs := api.Queries(putPrefix + "obj-alerting-1"); len(qs) != 1 || qs[0].Get("validateOnly") != "true" {
		t.Errorf("PUT queries %v", qs)
	}
	old, cfg := editFake(t, "1.287.0.20240101-000000")
	if r := runInteractive(t, cfg, "", "edit", "setting", "obj-alerting-1", "--validate-only"); r.Code != 2 || old.Hits(putPrefix+"obj-alerting-1") != 0 {
		t.Errorf("1.287: %s", r)
	}
}
