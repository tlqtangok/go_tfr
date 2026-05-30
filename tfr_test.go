package main_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tfrBin is the path to the test binary built in TestMain.
var tfrBin string

// TestMain builds the binary once, runs all tests, then cleans up.
func TestMain(m *testing.M) {
	bin, err := filepath.Abs("tfr_test_bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, "abs path:", err)
		os.Exit(1)
	}
	tfrBin = bin

	cmd := exec.Command("go", "build", "-o", tfrBin, ".")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s\n", err, out)
		os.Exit(1)
	}

	code := m.Run()
	os.Remove(tfrBin)
	os.Exit(code)
}

// run executes tfrBin in dir, feeding stdinData to stdin.
// Returns stdout, stderr, and the exit error (nil on success).
func run(t *testing.T, dir, stdinData string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(tfrBin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdinData)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	return out.String(), errOut.String(), err
}

// tor sends content and returns the slot string like "jd_42".
func tor(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, err := run(t, ".", "", args...)
	if err != nil {
		t.Fatalf("tor failed: %v\nstderr: %s", err, stderr)
	}
	slot := strings.TrimSpace(stdout)
	if !strings.HasPrefix(slot, "jd_") {
		t.Fatalf("tor: expected jd_N, got %q", slot)
	}
	return slot
}

// ── Basic ──────────────────────────────────────────────────────────────────

func TestBasicTorFr(t *testing.T) {
	slot := tor(t, "t", "hello_tfr_autotest")
	dir := t.TempDir()
	stdout, stderr, err := run(t, dir, "", "f", slot)
	if err != nil {
		t.Fatalf("fr: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "hello_tfr_autotest") {
		t.Errorf("expected 'hello_tfr_autotest' in stdout, got: %q", stdout)
	}
}

// ── Password ───────────────────────────────────────────────────────────────

// TestPasswordCorrect: tor with -pw, fr with correct -pw → succeeds.
func TestPasswordCorrect(t *testing.T) {
	slot := tor(t, "t", "pw_protected_content", "-pw", "correctpass")
	dir := t.TempDir()
	stdout, stderr, err := run(t, dir, "", "f", slot, "-pw", "correctpass")
	if err != nil {
		t.Fatalf("fr with correct pw: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "pw_protected_content") {
		t.Errorf("expected content in stdout, got: %q", stdout)
	}
}

// TestPasswordWrong: fr with wrong -pw → non-zero exit + error message.
func TestPasswordWrong(t *testing.T) {
	slot := tor(t, "t", "pw_protected_content2", "-pw", "correctpass")
	dir := t.TempDir()
	_, stderr, err := run(t, dir, "", "f", slot, "-pw", "wrongpass")
	if err == nil {
		t.Error("expected non-zero exit with wrong password")
	}
	if !strings.Contains(stderr, "wrong password") {
		t.Errorf("expected 'wrong password' in stderr, got: %q", stderr)
	}
}

// TestPasswordInteractive: fr without -pw flag, password read from stdin.
// readPassword falls back to stdin scanner when not a terminal.
func TestPasswordInteractive(t *testing.T) {
	slot := tor(t, "t", "interactive_pw_content", "-pw", "secretpw")
	dir := t.TempDir()
	// Pipe password via stdin (not a TTY → falls back to bufio.Scanner)
	stdout, stderr, err := run(t, dir, "secretpw\n", "f", slot)
	if err != nil {
		t.Fatalf("fr interactive pw: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "interactive_pw_content") {
		t.Errorf("expected content in stdout, got: %q", stdout)
	}
}

// TestPasswordNoSet: tor without -pw, fr without -pw → no prompt, succeeds.
func TestPasswordNoSet(t *testing.T) {
	slot := tor(t, "t", "no_pw_content")
	dir := t.TempDir()
	stdout, _, err := run(t, dir, "", "f", slot)
	if err != nil {
		t.Fatalf("fr no pw: %v", err)
	}
	if !strings.Contains(stdout, "no_pw_content") {
		t.Errorf("expected content in stdout, got: %q", stdout)
	}
}

// ── File overwrite ──────────────────────────────────────────────────────────

// TestOverwriteYes: file already exists in dest, user answers "yes" → overwritten.
func TestOverwriteYes(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "owtest.txt")
	os.WriteFile(srcFile, []byte("new content"), 0644)

	slot := tor(t, "t", srcFile)

	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "owtest.txt")
	os.WriteFile(destFile, []byte("old content"), 0644)

	_, stderr, err := run(t, destDir, "yes\n", "f", slot)
	if err != nil {
		t.Fatalf("fr overwrite yes: %v\nstderr: %s", err, stderr)
	}
	data, _ := os.ReadFile(destFile)
	if string(data) != "new content" {
		t.Errorf("file should be overwritten with 'new content', got: %q", string(data))
	}
}

// TestOverwriteNo: file exists, user answers "no" → not overwritten, non-zero exit.
func TestOverwriteNo(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "nowtest.txt")
	os.WriteFile(srcFile, []byte("new content"), 0644)

	slot := tor(t, "t", srcFile)

	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "nowtest.txt")
	os.WriteFile(destFile, []byte("original content"), 0644)

	_, _, err := run(t, destDir, "no\n", "f", slot)
	if err == nil {
		t.Error("expected non-zero exit when user declines overwrite")
	}
	data, _ := os.ReadFile(destFile)
	if string(data) != "original content" {
		t.Errorf("file should be untouched, got: %q", string(data))
	}
}

// TestTxtTxtNoPrompt: receiving a txt.txt (inline/stdin) always overwrites without prompt.
func TestTxtTxtNoPrompt(t *testing.T) {
	slot := tor(t, "t", "fresh_stdin_content") // non-existent arg → txt.txt

	destDir := t.TempDir()
	os.WriteFile(filepath.Join(destDir, "txt.txt"), []byte("stale content"), 0644)

	// stdin is empty — no prompt should appear
	stdout, stderr, err := run(t, destDir, "", "f", slot)
	if err != nil {
		t.Fatalf("fr txt.txt overwrite: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "fresh_stdin_content") {
		t.Errorf("expected fresh content in stdout, got: %q", stdout)
	}
	// stderr must NOT contain the overwrite prompt
	if strings.Contains(stderr, "overwrite") {
		t.Errorf("txt.txt should not trigger overwrite prompt, stderr: %q", stderr)
	}
}

// ── Folder ──────────────────────────────────────────────────────────────────

// TestFolderTransfer: send a directory → receive and verify structure restored.
func TestFolderTransfer(t *testing.T) {
	srcDir := t.TempDir()
	folderName := "tfrtestfolder"
	folderPath := filepath.Join(srcDir, folderName)
	os.MkdirAll(filepath.Join(folderPath, "sub"), 0755)
	os.WriteFile(filepath.Join(folderPath, "a.txt"), []byte("file a"), 0644)
	os.WriteFile(filepath.Join(folderPath, "b.txt"), []byte("file b"), 0644)
	os.WriteFile(filepath.Join(folderPath, "sub", "c.txt"), []byte("file c"), 0644)

	slot := tor(t, "t", folderPath)

	destDir := t.TempDir()
	_, stderr, err := run(t, destDir, "", "f", slot)
	if err != nil {
		t.Fatalf("fr folder: %v\nstderr: %s", err, stderr)
	}

	check := func(rel, want string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(destDir, rel))
		if err != nil {
			t.Errorf("missing %s: %v", rel, err)
			return
		}
		if string(data) != want {
			t.Errorf("%s: want %q, got %q", rel, want, string(data))
		}
	}
	check(filepath.Join(folderName, "a.txt"), "file a")
	check(filepath.Join(folderName, "b.txt"), "file b")
	check(filepath.Join(folderName, "sub", "c.txt"), "file c")
}

// ── Folder overwrite ─────────────────────────────────────────────────────────

// TestFolderOverwriteYes: dest folder exists, user answers "yes" → replaced.
func TestFolderOverwriteYes(t *testing.T) {
	srcDir := t.TempDir()
	folderPath := filepath.Join(srcDir, "myfolder")
	os.MkdirAll(folderPath, 0755)
	os.WriteFile(filepath.Join(folderPath, "new.txt"), []byte("new data"), 0644)

	slot := tor(t, "t", folderPath)

	destDir := t.TempDir()
	existingFolder := filepath.Join(destDir, "myfolder")
	os.MkdirAll(existingFolder, 0755)
	os.WriteFile(filepath.Join(existingFolder, "old.txt"), []byte("old data"), 0644)

	_, stderr, err := run(t, destDir, "yes\n", "f", slot)
	if err != nil {
		t.Fatalf("fr folder overwrite yes: %v\nstderr: %s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(destDir, "myfolder", "old.txt")); err == nil {
		t.Error("old.txt should be removed after folder overwrite")
	}
	data, err := os.ReadFile(filepath.Join(destDir, "myfolder", "new.txt"))
	if err != nil || string(data) != "new data" {
		t.Errorf("new.txt should have 'new data': err=%v, data=%q", err, string(data))
	}
}

// TestFolderOverwriteNo: dest folder exists, user answers "no" → untouched, non-zero exit.
func TestFolderOverwriteNo(t *testing.T) {
	srcDir := t.TempDir()
	folderPath := filepath.Join(srcDir, "keepfolder")
	os.MkdirAll(folderPath, 0755)
	os.WriteFile(filepath.Join(folderPath, "new.txt"), []byte("new data"), 0644)

	slot := tor(t, "t", folderPath)

	destDir := t.TempDir()
	existingFolder := filepath.Join(destDir, "keepfolder")
	os.MkdirAll(existingFolder, 0755)
	os.WriteFile(filepath.Join(existingFolder, "keep.txt"), []byte("keep data"), 0644)

	_, _, err := run(t, destDir, "no\n", "f", slot)
	if err == nil {
		t.Error("expected non-zero exit when user declines folder overwrite")
	}
	data, readErr := os.ReadFile(filepath.Join(existingFolder, "keep.txt"))
	if readErr != nil || string(data) != "keep data" {
		t.Errorf("keep.txt should be untouched: err=%v, data=%q", readErr, string(data))
	}
}
