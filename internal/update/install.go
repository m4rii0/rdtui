package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

type InstallOptions struct {
	GOOS          string
	ExePath       string
	TempDir       string
	Version       string
	CommandRunner CommandRunner
}

type InstallResult struct {
	Installed       bool
	WindowsManual   bool
	TargetPath      string
	ReplacementPath string
	BackupPath      string
}

func Install(ctx context.Context, asset []byte, opts InstallOptions) (InstallResult, error) {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "windows" {
		return writeWindowsReplacement(asset, opts)
	}
	return installUnix(ctx, asset, opts)
}

func writeWindowsReplacement(asset []byte, opts InstallOptions) (InstallResult, error) {
	dir := opts.TempDir
	if dir == "" {
		created, err := os.MkdirTemp("", "rdtui-update-*")
		if err != nil {
			return InstallResult{}, fmt.Errorf("create update temp dir: %w", err)
		}
		dir = created
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create update temp dir: %w", err)
	}

	target, err := executablePath(opts.ExePath)
	if err != nil {
		return InstallResult{}, err
	}
	name := "rdtui-update.exe"
	if opts.Version != "" {
		name = "rdtui-" + strings.TrimPrefix(opts.Version, "v") + ".exe"
	}
	replacement := filepath.Join(dir, name)
	if err := os.WriteFile(replacement, asset, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("write verified replacement: %w", err)
	}
	return InstallResult{WindowsManual: true, TargetPath: target, ReplacementPath: replacement}, nil
}

func installUnix(ctx context.Context, asset []byte, opts InstallOptions) (InstallResult, error) {
	target, err := executablePath(opts.ExePath)
	if err != nil {
		return InstallResult{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return InstallResult{TargetPath: target}, fmt.Errorf("stat current executable: %w", err)
	}
	if info.IsDir() {
		return InstallResult{TargetPath: target}, fmt.Errorf("current executable path is a directory: %s", target)
	}

	dir := filepath.Dir(target)
	base := filepath.Base(target)
	tmp, err := os.CreateTemp(dir, "."+base+".update-*")
	if err != nil {
		return InstallResult{TargetPath: target}, fmt.Errorf("create update temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(asset); err != nil {
		_ = tmp.Close()
		return InstallResult{TargetPath: target}, fmt.Errorf("write update temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return InstallResult{TargetPath: target}, fmt.Errorf("close update temp file: %w", err)
	}
	mode := info.Mode().Perm() | 0o111
	if err := os.Chmod(tmpPath, mode); err != nil {
		return InstallResult{TargetPath: target}, fmt.Errorf("mark update executable: %w", err)
	}

	backupPath, err := reserveBackupPath(dir, base)
	if err != nil {
		return InstallResult{TargetPath: target}, err
	}
	res := InstallResult{TargetPath: target, BackupPath: backupPath}

	if err := os.Rename(target, backupPath); err != nil {
		return res, fmt.Errorf("backup current executable: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		if restoreErr := os.Rename(backupPath, target); restoreErr != nil {
			return res, fmt.Errorf("install update: %w; rollback failed and backup remains at %s: %v", err, backupPath, restoreErr)
		}
		return res, fmt.Errorf("install update: %w", err)
	}
	cleanupTmp = false

	if err := validateInstalled(ctx, target, opts.Version, opts.CommandRunner); err != nil {
		if restoreErr := rollbackInstalled(target, backupPath); restoreErr != nil {
			return res, fmt.Errorf("%w; rollback failed and backup remains at %s: %v", err, backupPath, restoreErr)
		}
		return res, err
	}

	res.Installed = true
	if err := os.Remove(backupPath); err == nil {
		res.BackupPath = ""
	}
	return res, nil
}

func executablePath(path string) (string, error) {
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("locate current executable: %w", err)
		}
		path = exe
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("resolve executable symlink: %w", err)
		}
		return resolved, nil
	}
	return abs, nil
}

func reserveBackupPath(dir, base string) (string, error) {
	file, err := os.CreateTemp(dir, "."+base+".backup-*")
	if err != nil {
		return "", fmt.Errorf("reserve backup path: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("reserve backup path: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("reserve backup path: %w", err)
	}
	return path, nil
}

func validateInstalled(ctx context.Context, path, version string, runner CommandRunner) error {
	if version == "" {
		return nil
	}
	if runner == nil {
		runner = defaultCommandRunner
	}
	output, err := runner(ctx, path, "--version")
	if err != nil {
		return fmt.Errorf("validate updated binary: %w", err)
	}
	expected := "rdtui " + version
	if strings.TrimSpace(string(output)) != expected {
		return fmt.Errorf("validate updated binary: expected %q, got %q", expected, strings.TrimSpace(string(output)))
	}
	return nil
}

func rollbackInstalled(target, backupPath string) error {
	failedPath := target + ".failed-" + time.Now().UTC().Format("20060102150405")
	if err := os.Rename(target, failedPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(backupPath, target); err != nil {
		return err
	}
	_ = os.Remove(failedPath)
	return nil
}

func defaultCommandRunner(ctx context.Context, path string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	return cmd.CombinedOutput()
}
