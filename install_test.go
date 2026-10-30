package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The install scripts are tested against a fake release: an httptest server
// laid out like GitHub's release downloads (<base>/v<version>/<asset>),
// serving an archive with a stand-in binary that prints "dtmgd <version>",
// and a checksums.txt for it.

const testVersion = "0.0.0-test"

// releaseOpts changes the fake release's archive. beforeChecksum runs before
// checksums.txt is written, so the checksum matches its result;
// afterChecksum runs after, so the checksum no longer matches.
type releaseOpts struct {
	beforeChecksum func([]byte) []byte
	afterChecksum  func([]byte) []byte
}

// archiveName is the release asset for this platform, as .goreleaser.yaml
// names it.
func archiveName(version string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("dtmgd_%s_windows_%s.zip", version, runtime.GOARCH)
	}
	return fmt.Sprintf("dtmgd_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
}

// fakeBinary builds a program for this platform that prints
// "dtmgd <version> (commit: fake, built: unknown)" when run with "version".
func fakeBinary(t *testing.T, version string) []byte {
	t.Helper()
	dir := t.TempDir()
	src := fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("dtmgd %s (commit: fake, built: unknown)")
		return
	}
	fmt.Println("fake dtmgd")
}
`, version)
	for name, data := range map[string]string{"go.mod": "module fakedtmgd\n\ngo 1.21\n", "main.go": src} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "fake.bin")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the fake binary: %v\n%s", err, b)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// packArchive packs the binary (dtmgd, or dtmgd.exe on Windows) and a
// LICENSE the way the release archives hold them.
func packArchive(t *testing.T, bin []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	license := []byte("license text\n")
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		for name, data := range map[string][]byte{"dtmgd.exe": bin, "LICENSE": license} {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		mode int64
		data []byte
	}{{"LICENSE", 0o644, license}, {"dtmgd", 0o755, bin}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeRelease serves release version of the fake binary and returns the
// download base URL. Any other path is a 404.
func fakeRelease(t *testing.T, version string, opts releaseOpts) string {
	t.Helper()
	archive := packArchive(t, fakeBinary(t, version))
	if opts.beforeChecksum != nil {
		archive = opts.beforeChecksum(archive)
	}
	sum := sha256.Sum256(archive)
	name := archiveName(version)
	// Another asset's line first, so a script that takes the first line fails.
	checksums := fmt.Sprintf("%s  dtmgd_%s_plan9_mips.tar.gz\n%s  %s\n",
		strings.Repeat("0", 64), version, hex.EncodeToString(sum[:]), name)
	if opts.afterChecksum != nil {
		archive = opts.afterChecksum(archive)
	}
	files := map[string][]byte{
		"/v" + version + "/" + name:       archive,
		"/v" + version + "/checksums.txt": []byte(checksums),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// installEnv is the environment the scripts run with: the fake release, an
// install directory, and a temp HOME, so nothing outside the test changes.
func installEnv(t *testing.T, base, version, dir string) []string {
	t.Helper()
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "DTMGD_") || k == "GITHUB_TOKEN" || k == "HOME" {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"HOME="+t.TempDir(),
		"DTMGD_DOWNLOAD_BASE="+base,
		"DTMGD_VERSION="+version,
		"DTMGD_INSTALL_DIR="+dir,
		"DTMGD_NO_MODIFY_PATH=1",
	)
}

func runScript(t *testing.T, env []string, name string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code = 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %s: %v", name, err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

func runInstallSh(t *testing.T, base, version, dir string) (stdout, stderr string, code int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for Linux and macOS")
	}
	return runScript(t, installEnv(t, base, version, dir), "sh", "install.sh")
}

func runInstallPs1(t *testing.T, base, version, dir string) (stdout, stderr string, code int) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("install.ps1 is for Windows")
	}
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not found")
	}
	return runScript(t, installEnv(t, base, version, dir), "pwsh", "-NoProfile", "-NonInteractive", "-File", "install.ps1")
}

// installedVersion runs the installed binary's version command.
func installedVersion(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("running %s version: %v\n%s", path, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeOldBinary(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertNotInstalled(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s was installed", path)
	}
}

func flipByte(b []byte) []byte {
	c := bytes.Clone(b)
	c[len(c)/2] ^= 0xff
	return c
}

func TestInstallShInstallsAVerifiedBinary(t *testing.T) {
	dir := t.TempDir()
	base := fakeRelease(t, testVersion, releaseOpts{})
	stdout, stderr, code := runInstallSh(t, base, testVersion, dir)
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	bin := filepath.Join(dir, "dtmgd")
	if got := installedVersion(t, bin); !strings.HasPrefix(got, "dtmgd "+testVersion+" ") {
		t.Errorf("installed binary reports %q", got)
	}
	if !strings.Contains(stdout, bin) {
		t.Errorf("stdout does not name the install path %s:\n%s", bin, stdout)
	}
}

func TestInstallShRefusesAChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	base := fakeRelease(t, testVersion, releaseOpts{afterChecksum: flipByte})
	_, stderr, code := runInstallSh(t, base, testVersion, dir)
	if code == 0 {
		t.Fatal("exit 0 for a tampered archive")
	}
	if !strings.Contains(stderr, "checksum") {
		t.Errorf("stderr does not mention the checksum:\n%s", stderr)
	}
	assertNotInstalled(t, filepath.Join(dir, "dtmgd"))
}

func TestInstallShFailsWhenTheAssetIsMissing(t *testing.T) {
	dir := t.TempDir()
	base := fakeRelease(t, testVersion, releaseOpts{})
	_, stderr, code := runInstallSh(t, base, "0.0.0-missing", dir)
	if code == 0 {
		t.Fatal("exit 0 for a release that does not exist")
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("stderr has no Error: line:\n%s", stderr)
	}
	assertNotInstalled(t, filepath.Join(dir, "dtmgd"))
}

func TestInstallShKeepsTheOldBinaryWhenTheArchiveIsTruncated(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "dtmgd")
	writeOldBinary(t, bin)
	// The checksum matches the truncated bytes, so only extraction can fail.
	base := fakeRelease(t, testVersion, releaseOpts{beforeChecksum: func(b []byte) []byte { return b[:len(b)/2] }})
	_, _, code := runInstallSh(t, base, testVersion, dir)
	if code == 0 {
		t.Fatal("exit 0 for a truncated archive")
	}
	if got, err := os.ReadFile(bin); err != nil || string(got) != "old" {
		t.Errorf("the old binary was replaced or removed: %q, %v", got, err)
	}
}

func TestInstallShReplacesAnExistingBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "dtmgd")
	writeOldBinary(t, bin)
	base := fakeRelease(t, testVersion, releaseOpts{})
	stdout, stderr, code := runInstallSh(t, base, "v"+testVersion, dir) // a leading v is accepted
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if got := installedVersion(t, bin); !strings.HasPrefix(got, "dtmgd "+testVersion+" ") {
		t.Errorf("installed binary reports %q", got)
	}
}

func TestInstallPs1InstallsAVerifiedBinary(t *testing.T) {
	dir := t.TempDir()
	base := fakeRelease(t, testVersion, releaseOpts{})
	stdout, stderr, code := runInstallPs1(t, base, testVersion, dir)
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if got := installedVersion(t, filepath.Join(dir, "dtmgd.exe")); !strings.HasPrefix(got, "dtmgd "+testVersion+" ") {
		t.Errorf("installed binary reports %q", got)
	}
}

func TestInstallPs1RefusesAChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	base := fakeRelease(t, testVersion, releaseOpts{afterChecksum: flipByte})
	stdout, stderr, code := runInstallPs1(t, base, testVersion, dir)
	if code == 0 {
		t.Fatalf("exit 0 for a tampered archive\nstdout:\n%s", stdout)
	}
	if !strings.Contains(stdout+stderr, "checksum") {
		t.Errorf("output does not mention the checksum:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	assertNotInstalled(t, filepath.Join(dir, "dtmgd.exe"))
}
