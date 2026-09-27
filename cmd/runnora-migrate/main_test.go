package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupLegacy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"config.yaml":    "app:\n  name: x\n",
		"runbooks/a.yml": "desc: A-1 test\nrunners:\n  req: http://localhost:1\nsteps:\n  s:\n    test: true\n",
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRun(t *testing.T) {
	t.Run("dry-run does not write", func(t *testing.T) {
		dir := setupLegacy(t)
		var out, errOut bytes.Buffer
		if code := run([]string{dir}, &out, &errOut); code != 0 {
			t.Fatalf("exit %d: %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "dry-run") || !strings.Contains(out.String(), "create  runnora.yaml") {
			t.Errorf("report:\n%s", out.String())
		}
		if _, err := os.Stat(filepath.Join(dir, "runnora.yaml")); !os.IsNotExist(err) {
			t.Error("dry-run wrote runnora.yaml")
		}
	})

	t.Run("write refuses outside a git work tree", func(t *testing.T) {
		dir := setupLegacy(t)
		var out, errOut bytes.Buffer
		if code := run([]string{"--write", dir}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "書き込みを中止") {
			t.Fatalf("exit %d: %s", code, errOut.String())
		}
		if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
			t.Error("config.yaml should remain")
		}
	})

	t.Run("write with allow-dirty", func(t *testing.T) {
		dir := setupLegacy(t)
		var out, errOut bytes.Buffer
		if code := run([]string{"--write", "--allow-dirty", "--env", "unit=config.yaml", dir}, &out, &errOut); code != 0 {
			t.Fatalf("exit %d: %s", code, errOut.String())
		}
		data, err := os.ReadFile(filepath.Join(dir, "runnora.yaml"))
		if err != nil || !strings.Contains(string(data), "  unit:\n") {
			t.Fatalf("runnora.yaml: %v\n%s", err, data)
		}
	})

	t.Run("write in a clean git work tree", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git not available")
		}
		dir := setupLegacy(t)
		for _, args := range [][]string{
			{"init", "-q"}, {"add", "-A"},
			{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "v1"},
		} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
		var out, errOut bytes.Buffer
		if code := run([]string{"--write", dir}, &out, &errOut); code != 0 {
			t.Fatalf("exit %d: %s", code, errOut.String())
		}
		// 書き込んだ後は未コミットの変更があるので、もう一度 --write すると止まる (変更がなければ止まらない)
		out.Reset()
		errOut.Reset()
		if code := run([]string{"--write", dir}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "変更はありません") {
			t.Fatalf("second run: exit %d\n%s\n%s", code, out.String(), errOut.String())
		}
	})

	t.Run("bad --env", func(t *testing.T) {
		var out, errOut bytes.Buffer
		if code := run([]string{"--env", "unit", "."}, &out, &errOut); code != 2 {
			t.Fatalf("exit %d", code)
		}
	})
}
