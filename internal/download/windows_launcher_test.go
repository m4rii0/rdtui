package download

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var nativeDesktopTestMain func()

func TestMain(m *testing.M) {
	if nativeDesktopTestMain != nil {
		nativeDesktopTestMain()
	}
	if action := os.Getenv(desktopActionEnv); action != "" {
		_ = os.Unsetenv(desktopActionEnv)
		// Replace only the native desktop side effect. Exercise the real child
		// command and data decoder without launching a user's app or Explorer.
		err := consumeDesktopPath(os.Stdin, action, func(path string, reveal bool) error {
			if os.Getenv("RDTUI_TEST_DESKTOP_BLOCK") == "1" {
				if _, err := fmt.Fprintln(os.Stdout, "entered"); err != nil {
					return err
				}
				<-time.NewTimer(time.Hour).C // deliberately stuck native-call fixture
			}
			return os.WriteFile(os.Getenv("RDTUI_TEST_DESKTOP_CAPTURE"), []byte(fmt.Sprintf("%v\n%s", reveal, path)), 0o600)
		})
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestWindowsDesktopPathIsDataNotCommandText(t *testing.T) {
	for _, path := range []string{
		`C:\Downloads\movie&rdtui-launch-sentinel&.mkv`,
		`C:\Downloads\%RDTUI_LAUNCH_SENTINEL%.mkv`,
		`C:\Downloads\with spaces\日本語 película.mkv`,
		`C:\Downloads\quote"&rdtui-launch-sentinel&.mkv`,
	} {
		for _, reveal := range []bool{false, true} {
			cmd, err := windowsOpenCommand(t.Context(), path, reveal)
			if err != nil {
				t.Fatal(err)
			}
			// Never run the vulnerable command, even on Windows. The filename
			// must not be exposed to a command interpreter or argv parsing.
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != executable || !slices.Equal(cmd.Args, []string{executable}) {
				t.Errorf("path exposed to command parsing (reveal=%v): %q", reveal, cmd.Args)
				continue
			}
			if cmd.Stdin == nil {
				t.Fatal("no data channel")
			}
			data, err := io.ReadAll(cmd.Stdin)
			if err != nil || string(data) != path {
				t.Fatalf("literal path changed: %q, %v", data, err)
			}
		}
	}
}

func TestWindowsDesktopHelperRoundTripsLiteralFiles(t *testing.T) {
	dir := t.TempDir()
	capture, sentinel := filepath.Join(dir, "capture"), filepath.Join(dir, "sentinel-evaluated")
	t.Setenv("RDTUI_TEST_DESKTOP_CAPTURE", capture)
	t.Setenv("RDTUI_LAUNCH_SENTINEL", "expanded-not-literal")
	t.Setenv("RDTUI_SENTINEL_FILE", sentinel)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// A benign marker fixture, never intentionally invoked. No arbitrary
	// commands, real downloaded files, associations, or desktop apps are used.
	if err := os.WriteFile(filepath.Join(dir, "rdtui-launch-sentinel.cmd"), []byte("@echo evaluated>\"%RDTUI_SENTINEL_FILE%\"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"movie&rdtui-launch-sentinel&.txt", "%RDTUI_LAUNCH_SENTINEL%.txt", "with spaces 日本語 película.txt"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		for _, reveal := range []bool{false, true} {
			cmd, err := windowsOpenCommand(t.Context(), path, reveal)
			if err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != executable || !slices.Equal(cmd.Args, []string{executable}) {
				t.Fatal("refusing to execute an interpreter/filename command in this test")
			}
			if err := runHelper(t.Context(), cmd); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(capture)
			if want := fmt.Sprintf("%v\n%s", reveal, path); err != nil || string(data) != want {
				t.Fatalf("changed native-boundary path/action: %q, %v, want %q", data, err, want)
			}
			if _, err := os.Stat(sentinel); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("sentinel evaluated: %v", err)
			}
		}
	}
}

func TestWindowsDesktopHelperRejectsInvalidData(t *testing.T) {
	for _, path := range []string{"", "has\x00nul", `quote"&rdtui-launch-sentinel&.txt`, strings.Repeat("x", maxDesktopPathBytes+1)} {
		called := false
		err := consumeDesktopPath(strings.NewReader(path), "open", func(string, bool) error { called = true; return nil })
		if err == nil || called {
			t.Fatalf("invalid data reached native launch: err=%v called=%v", err, called)
		}
	}
	if err := consumeDesktopPath(strings.NewReader("valid"), "unknown", func(string, bool) error { t.Fatal("invalid action launched"); return nil }); err == nil {
		t.Fatal("invalid action accepted")
	}
}

func TestWindowsDesktopHelperCancellationReapsOwnedChild(t *testing.T) {
	t.Setenv("RDTUI_TEST_DESKTOP_BLOCK", "1")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd, err := windowsOpenCommand(ctx, `C:\fixture.txt`, false)
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- runHelper(ctx, cmd) }()
	t.Cleanup(func() {
		cancel()
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		select {
		case <-done:
		case <-cleanup.Done():
			t.Error("owned desktop child failed to stop")
		}
	})
	entry := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(output).ReadString('\n')
		if err == nil && line != "entered\n" {
			err = fmt.Errorf("unexpected entry: %q", line)
		}
		entry <- err
	}()
	guard, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	select {
	case err := <-entry:
		if err != nil {
			t.Fatal(err)
		}
	case <-guard.Done():
		t.Fatal("child did not acknowledge native-call entry")
	}
	cancel()
	select {
	case err := <-done:
		done <- err // cleanup also joins the early-failure path
		if !errors.Is(err, context.Canceled) || cmd.ProcessState == nil || !errors.Is(cmd.Process.Kill(), os.ErrProcessDone) {
			t.Fatalf("helper cancellation was not reaped: %v, %v", err, cmd.ProcessState)
		}
	case <-guard.Done():
		t.Fatal("child ignored cancellation after acknowledged entry")
	}
}
