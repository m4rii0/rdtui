package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type releaseFixture struct {
	t    *testing.T
	dir  string
	root string
	env  []string
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("release automation runs in Bash on Linux")
	}
	for _, tool := range []string{"git", "bash"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("release tests require %s: %v", tool, err)
		}
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	f := &releaseFixture{t: t, dir: t.TempDir(), root: root}
	f.git("init", "--quiet")
	f.git("-c", "user.name=Release Test", "-c", "user.email=test@example.com",
		"commit", "--quiet", "--allow-empty", "-m", "fixture")
	return f
}

func (f *releaseFixture) command(name string, args ...string) *exec.Cmd {
	f.t.Helper()
	cmd := exec.CommandContext(f.t.Context(), name, args...)
	cmd.Dir = f.dir
	cmd.Env = append(os.Environ(), f.env...)
	return cmd
}

func (f *releaseFixture) git(args ...string) string {
	f.t.Helper()
	// Fixture commits/tags must never invoke a developer's configured signer.
	args = append([]string{"-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false"}, args...)
	out, err := f.command("git", args...).CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *releaseFixture) write(path, content string, mode os.FileMode) {
	f.t.Helper()
	path = filepath.Join(f.dir, path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *releaseFixture) read(path string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, path))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func TestReleaseFixtureIgnoresGitSigning(t *testing.T) {
	// Developer signing preferences must not require credentials for fixtures.
	t.Setenv("GIT_CONFIG_COUNT", "3")
	t.Setenv("GIT_CONFIG_KEY_0", "commit.gpgSign")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_CONFIG_KEY_1", "tag.gpgSign")
	t.Setenv("GIT_CONFIG_VALUE_1", "true")
	t.Setenv("GIT_CONFIG_KEY_2", "gpg.program")
	t.Setenv("GIT_CONFIG_VALUE_2", filepath.Join(t.TempDir(), "no-signer"))
	f := newReleaseFixture(t)
	f.git("tag", "v1.2.3")
	if got := f.git("tag", "--list"); got != "v1.2.3" {
		t.Fatalf("fixture tag = %q", got)
	}
}

func TestReleaseVersion(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		bump    string
		channel string
		want    string
	}{
		{"initial stable", nil, "patch", "stable", "v0.0.1"},
		{"initial rc", nil, "patch", "rc", "v0.0.1-rc.1"},
		{"patch", []string{"v1.2.3"}, "patch", "stable", "v1.2.4"},
		{"minor", []string{"v1.2.3"}, "minor", "stable", "v1.3.0"},
		{"major", []string{"v1.2.3"}, "major", "stable", "v2.0.0"},
		{"numeric stable order", []string{"v1.9.99", "v1.10.0", "v1.2.10"}, "patch", "stable", "v1.10.1"},
		{"patch rc", []string{"v1.2.3"}, "patch", "rc", "v1.2.4-rc.1"},
		{"minor rc", []string{"v1.2.3"}, "minor", "rc", "v1.3.0-rc.1"},
		{"major rc", []string{"v1.2.3"}, "major", "rc", "v2.0.0-rc.1"},
		{"numeric rc order", []string{"v1.2.3", "v1.3.0-rc.2", "v1.3.0-rc.9"}, "minor", "rc", "v1.3.0-rc.10"},
		{"rc gaps", []string{"v1.2.3", "v1.3.0-rc.1", "v1.3.0-rc.12"}, "minor", "rc", "v1.3.0-rc.13"},
		{"other targets", []string{"v1.2.3", "v1.2.4-rc.8", "v2.0.0-rc.99"}, "minor", "rc", "v1.3.0-rc.1"},
		{"stable after rcs", []string{"v1.2.3", "v1.3.0-rc.1", "v1.3.0-rc.12"}, "minor", "stable", "v1.3.0"},
		{"unrelated tags", []string{"nightly", "v9.0.0-rc.99"}, "patch", "stable", "v0.0.1"},
		{"malformed tags", []string{"release-99", "v9.0", "v8.0.0-beta.1", "v7.0.0+build", "v06.0.0",
			"v1.3.0-rc.0", "v1.3.0-rc.01", "v1.3.0-rc.99.extra", "v1.2.3"}, "minor", "rc", "v1.3.0-rc.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			for _, tag := range tt.tags {
				f.git("tag", tag)
			}
			before := f.git("tag", "--list")
			cmd := f.command("bash", filepath.Join(f.root, "scripts/release-version.sh"), tt.bump, tt.channel)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil || string(out) != tt.want+"\n" || stderr.Len() != 0 {
				t.Fatalf("calculate: stdout=%q stderr=%q err=%v; want %q", out, stderr.String(), err, tt.want)
			}
			if after := f.git("tag", "--list"); after != before {
				t.Fatalf("calculator mutated tags: before=%q after=%q", before, after)
			}
		})
	}
}

func TestReleaseVersionRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"patch"}, {"patch", "rc", "extra"}, {"invalid", "rc"}, {"minor", "beta"}, {"", "stable"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			f := newReleaseFixture(t)
			cmd := f.command("bash", append([]string{filepath.Join(f.root, "scripts/release-version.sh")}, args...)...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err == nil || len(out) != 0 || !strings.Contains(stderr.String(), "Usage:") {
				t.Fatalf("invalid arguments: stdout=%q stderr=%q err=%v", out, stderr.String(), err)
			}
		})
	}
}

// workflowCommands extracts the actual shell blocks, in workflow order, without
// introducing a YAML dependency. actionlint validates the full workflow syntax.
func workflowCommands(t *testing.T, names ...string) string {
	t.Helper()
	data, err := os.ReadFile("../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var script strings.Builder
	var found []string
	var name string
	inRun := false
	for line := range strings.SplitSeq(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "      - name: "); ok {
			name, inRun = value, false
			continue
		}
		if !slices.Contains(names, name) {
			continue
		}
		if value, ok := strings.CutPrefix(line, "        run: "); ok {
			found = append(found, name)
			inRun = value == "|"
			if !inRun {
				script.WriteString(value + "\n")
			}
			continue
		}
		if inRun {
			if value, ok := strings.CutPrefix(line, "          "); ok {
				script.WriteString(value + "\n")
			} else if line != "" {
				inRun = false
			}
		}
	}
	for _, name := range names {
		if !slices.Contains(found, name) {
			t.Fatalf("workflow has no run block named %q", name)
		}
	}
	return script.String()
}

func TestReleasePrepareOutputsRC(t *testing.T) {
	f := newReleaseFixture(t)
	f.git("tag", "v1.2.3")
	f.git("tag", "v1.3.0-rc.9")
	calculator, err := os.ReadFile(filepath.Join(f.root, "scripts/release-version.sh"))
	if err != nil {
		t.Fatal(err)
	}
	f.write("scripts/release-version.sh", string(calculator), 0o644)
	f.env = []string{"BUMP=minor", "CHANNEL=rc", "GITHUB_OUTPUT=" + filepath.Join(f.dir, "output"),
		"GITHUB_STEP_SUMMARY=" + filepath.Join(f.dir, "summary")}
	out, err := f.command("bash", "-euo", "pipefail", "-c", workflowCommands(t, "Calculate next version")).CombinedOutput()
	if err != nil {
		t.Fatalf("prepare: %v\n%s", err, out)
	}
	if got := f.read("output"); got != "new_version=v1.3.0-rc.10\n" {
		t.Fatalf("version output = %q", got)
	}
	if got := f.read("summary"); !strings.Contains(got, "rc") {
		t.Fatalf("summary omits channel: %q", got)
	}
}

func TestReleasePublication(t *testing.T) {
	for _, tt := range []struct {
		name     string
		channel  string
		version  string
		existing string
		want     []string
	}{
		{"rc", "rc", "v1.3.0-rc.10", "0", []string{"--prerelease", "--latest=false"}},
		{"stable", "stable", "v1.3.0", "0", []string{"--latest"}},
		{"existing", "rc", "v1.3.0-rc.10", "1", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			// Mock only the external GitHub API boundary; run the real workflow shell.
			f.write("tools/gh", "#!/bin/sh\n"+
				"printf '%s\\0' \"$@\" > \"$GH_LOG\"\n"+
				"if [ \"$2\" = view ]; then [ \"$GH_EXISTS\" = 1 ]; fi\n", 0o755)
			f.env = []string{"PATH=" + filepath.Join(f.dir, "tools") + ":" + os.Getenv("PATH"),
				"NEW_VERSION=" + tt.version, "CHANNEL=" + tt.channel, "GH_EXISTS=" + tt.existing,
				"GH_LOG=" + filepath.Join(f.dir, "gh-args")}
			assets := []string{"bin/rdtui-linux-amd64", "bin/checksums.txt", "bin/checksums.txt.sigstore.json"}
			for _, asset := range assets {
				f.write(asset, "fixture", 0o644)
			}
			out, err := f.command("bash", "-euo", "pipefail", "-c", workflowCommands(t, "Publish GitHub release")).CombinedOutput()
			if err != nil {
				t.Fatalf("publish: %v\n%s", err, out)
			}
			want := append([]string{"release", "create", tt.version}, assets...)
			if tt.existing == "1" {
				want[1] = "upload"
				want = append(want, "--clobber")
			} else {
				want = append(want, "--verify-tag")
				want = append(want, tt.want...)
				want = append(want, "--title", tt.version, "--notes", "Release "+tt.version)
			}
			got := strings.Split(strings.TrimSuffix(f.read("gh-args"), "\x00"), "\x00")
			if !slices.Equal(got, want) {
				t.Fatalf("gh args = %q; want %q", got, want)
			}
		})
	}
}

func TestReleaseFailureDoesNotPushTag(t *testing.T) {
	for _, failure := range []string{"MAKE_EXIT=1", "COSIGN_EXIT=1"} {
		t.Run(failure, func(t *testing.T) {
			f := newReleaseFixture(t)
			remote := filepath.Join(f.dir, "remote.git")
			f.git("init", "--bare", "--quiet", remote)
			f.git("remote", "add", "origin", remote)
			f.write("tools/make", "#!/bin/sh\nexit \"${MAKE_EXIT:-0}\"\n", 0o755)
			f.write("tools/cosign", "#!/bin/sh\nexit \"${COSIGN_EXIT:-0}\"\n", 0o755)
			f.env = []string{"PATH=" + filepath.Join(f.dir, "tools") + ":" + os.Getenv("PATH"),
				"NEW_VERSION=v1.3.0-rc.10", failure}
			out, err := f.command("bash", "-euo", "pipefail", "-c",
				workflowCommands(t, "Verify", "Sign checksums", "Tag release")).CombinedOutput()
			if err == nil {
				t.Fatalf("expected failed workflow: %s", out)
			}
			if tags := f.git("--git-dir", remote, "tag", "--list"); tags != "" {
				t.Fatalf("failed workflow pushed tags: %s", tags)
			}
		})
	}
}
