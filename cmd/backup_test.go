package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const backupConfigRoute = "GET /api/v1.0/onpremise/backup/config"

// backupNote is what describe backup tells a person the API cannot.
const backupNote = "The Cluster API reports the backup configuration only. It has no last-backup time and no backup health; check those in the Cluster Management Console."

// TestDescribeBackupHuman pins describe backup's summary, and the note that
// the API says nothing about whether backups run.
func TestDescribeBackupHuman(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{backupConfigRoute: {File: "cluster_backup_config.json"}})
	assertGolden(t, "describe_backup_human", runCLI(t, cfg, "describe", "backup").String())
}

// TestDescribeBackupJSON pins json output: the API object as-is, without the
// note.
func TestDescribeBackupJSON(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{backupConfigRoute: {File: "cluster_backup_config.json"}})
	r := runCLI(t, cfg, "describe", "backup", "-o", "json")
	var got, want interface{}
	raw, _ := os.ReadFile(filepath.Join("testdata", "api", "cluster_backup_config.json"))
	_ = json.Unmarshal(raw, &want)
	if err := json.Unmarshal([]byte(r.Stdout), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("json output differs from the API object (%v):\n%s", err, r.Stdout)
	}
	if strings.Contains(r.Stderr, "backup health") {
		t.Errorf("note in json output's stderr: %q", r.Stderr)
	}
}

// TestDescribeBackupDisabled pins how disabled backups read.
func TestDescribeBackupDisabled(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{backupConfigRoute: {Body: `{"enabled":false,"pauseBackups":false}`}})
	r := runCLI(t, cfg, "describe", "backup")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Enabled:                no") {
		t.Errorf("code %d, stdout:\n%s", r.Code, r.Stdout)
	}
}

// TestBackupHelpSaysConfigOnly pins that the help says what the command
// cannot show.
func TestBackupHelpSaysConfigOnly(t *testing.T) {
	c, _, err := testRoot().Find([]string{"describe", "backup"})
	if err != nil || !strings.Contains(c.Long, backupNote) {
		t.Errorf("describe backup help (%v):\n%s", err, c.Long)
	}
}
