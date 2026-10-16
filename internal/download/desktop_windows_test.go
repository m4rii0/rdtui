package download

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func init() {
	nativeDesktopTestMain = func() {
		executable, err := os.Executable()
		if err != nil {
			panic(err)
		}
		if fixture := os.Getenv("RDTUI_TEST_NATIVE_OPEN_EXE"); fixture != "" && strings.EqualFold(executable, fixture) {
			if os.Getenv(desktopActionEnv) != "" {
				panic("opened application inherited internal helper mode")
			}
			// This is a copied Go test binary, not a user/downloaded executable.
			// Its only behavior is capturing its literal name and a bounded
			// event handshake, so the parent can wait for its clean exit.
			data := fmt.Sprintf("%d\n%s", os.Getpid(), executable)
			if err := os.WriteFile(os.Getenv("RDTUI_TEST_NATIVE_CAPTURE"), []byte(data), 0o600); err != nil {
				panic(err)
			}
			entered, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(os.Getenv("RDTUI_TEST_NATIVE_ENTERED")))
			if err != nil {
				panic(err)
			}
			release, err := windows.OpenEvent(windows.SYNCHRONIZE, false, windows.StringToUTF16Ptr(os.Getenv("RDTUI_TEST_NATIVE_RELEASE")))
			if err != nil {
				panic(err)
			}
			if err := windows.SetEvent(entered); err != nil {
				panic(err)
			}
			result, err := windows.WaitForSingleObject(release, 10000)
			_ = windows.CloseHandle(entered)
			_ = windows.CloseHandle(release)
			if err != nil || result != windows.WAIT_OBJECT_0 {
				os.Exit(1)
			}
			os.Exit(0)
		}
		if os.Getenv("RDTUI_TEST_USE_NATIVE_DESKTOP") == "1" {
			if handled, err := RunDesktopHelper(os.Stdin); handled {
				if err != nil {
					_, _ = fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				os.Exit(0)
			}
		}
	}
}

func TestWindowsNativeOpenExecutesOnlyLiteralFixture(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	capture, sentinel := filepath.Join(dir, "capture"), filepath.Join(dir, "sentinel-evaluated")
	t.Setenv("RDTUI_TEST_NATIVE_CAPTURE", capture)
	t.Setenv("RDTUI_TEST_USE_NATIVE_DESKTOP", "1")
	t.Setenv("RDTUI_LAUNCH_SENTINEL", "must-not-expand")
	t.Setenv("RDTUI_SENTINEL_FILE", sentinel)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(dir, "rdtui-launch-sentinel.cmd"), []byte("@echo evaluated>\"%RDTUI_SENTINEL_FILE%\"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fixture&rdtui-launch-sentinel&.exe", "%RDTUI_LAUNCH_SENTINEL%.exe", "with spaces 日本語 película.exe"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			copyFixture := func() error {
				source, err := os.Open(executable)
				if err != nil {
					return err
				}
				defer func() { _ = source.Close() }()
				target, err := os.Create(path)
				if err != nil {
					return err
				}
				_, copyErr := io.Copy(target, source)
				return errors.Join(copyErr, target.Close())
			}
			if err := copyFixture(); err != nil {
				t.Fatal(err)
			}
			event := func(suffix string) (string, windows.Handle) {
				t.Helper()
				name := `Local\rdtui-desktop-test-` + rand.Text() + suffix
				handle, err := windows.CreateEvent(nil, 1, 0, windows.StringToUTF16Ptr(name))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = windows.CloseHandle(handle) })
				return name, handle
			}
			enteredName, entered := event("entered")
			releaseName, release := event("release")
			t.Cleanup(func() { _ = windows.SetEvent(release) })
			t.Setenv("RDTUI_TEST_NATIVE_OPEN_EXE", path)
			t.Setenv("RDTUI_TEST_NATIVE_ENTERED", enteredName)
			t.Setenv("RDTUI_TEST_NATIVE_RELEASE", releaseName)
			ctx, cancel := context.WithTimeout(t.Context(), helperTimeout)
			defer cancel()
			cmd, err := windowsOpenCommand(ctx, path, false)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != executable || len(cmd.Args) != 1 {
				t.Fatal("refusing to run a command interpreter or filename arguments")
			}
			if err := runHelper(ctx, cmd); err != nil {
				t.Fatal(err)
			}
			if result, err := windows.WaitForSingleObject(entered, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
				t.Fatalf("literal fixture did not start: %v %v", result, err)
			}
			data, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			var pid uint32
			pidText, actualPath, complete := strings.Cut(string(data), "\n")
			if _, err := fmt.Sscanf(pidText, "%d", &pid); err != nil || !complete {
				t.Fatalf("invalid fixture acknowledgement: %q %v", data, err)
			}
			process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = windows.SetEvent(release)
				if result, err := windows.WaitForSingleObject(process, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
					t.Errorf("fixture cleanup did not exit: %v %v", result, err)
				}
				_ = windows.CloseHandle(process)
			})
			if err := windows.SetEvent(release); err != nil {
				t.Fatal(err)
			}
			if result, err := windows.WaitForSingleObject(process, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
				t.Fatalf("fixture did not exit: %v %v", result, err)
			}
			if !strings.EqualFold(actualPath, path) {
				t.Fatalf("opened different file: %q, want %q", actualPath, path)
			}
			if _, err := os.Stat(sentinel); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("sentinel command evaluated: %v", err)
			}
		})
	}
}

// These native tests avoid launching Explorer or a file association. The open
// boundary is inspected directly; reveal uses the real Shell PIDL parser and
// filesystem-path round trip, not the UI-changing selection call.
func TestWindowsNativeDesktopLiteralPaths(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && !errors.Is(err, syscall.Errno(1)) {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	t.Setenv("RDTUI_LAUNCH_SENTINEL", "must-not-expand")
	for _, name := range []string{"movie&rdtui-launch-sentinel&.txt", "%RDTUI_LAUNCH_SENTINEL%.txt", "with spaces 日本語 película.txt"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		info, err := windowsOpenInfo(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := windows.UTF16PtrToString(info.File); got != path || windows.UTF16PtrToString(info.Verb) != "open" || info.Parameters != nil || info.Mask&0x200 != 0 || info.Mask&0x40 != 0 || info.Mask&0x02000000 != 0 {
			t.Fatalf("open changed file/interpreted parameters or owns user app: %+v, file=%q", info, got)
		}
		item, err := parseWindowsItem(path)
		if err != nil {
			t.Fatal(err)
		}
		var buffer [32768]uint16
		getPath := desktopShell32.NewProc("SHGetPathFromIDListEx")
		result, _, callErr := getPath.Call(uintptr(item), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0)
		windows.CoTaskMemFree(item)
		if result == 0 {
			t.Fatal(callErr)
		}
		if got := windows.UTF16ToString(buffer[:]); !strings.EqualFold(got, path) {
			t.Fatalf("reveal resolved a different item: %q, want %q", got, path)
		}
	}
}

func TestWindowsDesktopProtocolClearsModeOnInvalidRequest(t *testing.T) {
	t.Setenv(desktopActionEnv, "open")
	handled, err := RunDesktopHelper(strings.NewReader(`invalid"&rdtui-launch-sentinel&.txt`))
	if !handled || err == nil || os.Getenv(desktopActionEnv) != "" {
		t.Fatalf("invalid helper request/mode inheritance: %v %v", handled, err)
	}
}
