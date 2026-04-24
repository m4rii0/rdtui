package update

import (
	"context"
	"fmt"
)

type CheckOptions struct {
	CurrentVersion string
	Platform       Platform
	Client         *Client
}

type CheckResult struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	Release         Release
	Asset           Asset
	AssetName       string
}

type UpdateOptions struct {
	CurrentVersion string
	Platform       Platform
	Client         *Client
	DryRun         bool
	ExePath        string
	TempDir        string
	CommandRunner  CommandRunner
}

type UpdateResult struct {
	CheckResult
	DryRun          bool
	Installed       bool
	WindowsManual   bool
	TargetPath      string
	ReplacementPath string
	BackupPath      string
}

func Check(ctx context.Context, opts CheckOptions) (CheckResult, error) {
	current, err := NormalizeReleasedVersion(opts.CurrentVersion)
	if err != nil {
		return CheckResult{}, err
	}

	client := opts.Client
	if client == nil {
		client = NewClient()
	}
	release, err := client.LatestRelease(ctx)
	if err != nil {
		return CheckResult{CurrentVersion: current}, err
	}
	latest, err := NormalizeReleasedVersion(release.TagName)
	if err != nil {
		return CheckResult{CurrentVersion: current}, fmt.Errorf("latest release version cannot be checked: %w", err)
	}

	res := CheckResult{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: IsNewer(current, latest),
		Release:         release,
	}
	if !res.UpdateAvailable {
		return res, nil
	}

	asset, assetName, err := SelectAsset(release, opts.Platform)
	if err != nil {
		return res, err
	}
	res.Asset = asset
	res.AssetName = assetName
	return res, nil
}

func Update(ctx context.Context, opts UpdateOptions) (UpdateResult, error) {
	check, err := Check(ctx, CheckOptions{
		CurrentVersion: opts.CurrentVersion,
		Platform:       opts.Platform,
		Client:         opts.Client,
	})
	res := UpdateResult{CheckResult: check, DryRun: opts.DryRun}
	if err != nil || !check.UpdateAvailable || opts.DryRun {
		return res, err
	}

	client := opts.Client
	if client == nil {
		client = NewClient()
	}
	checksumAsset, ok := AssetByName(check.Release, checksumsAssetName)
	if !ok {
		return res, fmt.Errorf("release %s does not contain %s", check.LatestVersion, checksumsAssetName)
	}
	if checksumAsset.BrowserDownloadURL == "" {
		return res, fmt.Errorf("release asset %q has no download URL", checksumsAssetName)
	}

	assetData, err := client.Download(ctx, check.Asset.BrowserDownloadURL)
	if err != nil {
		return res, err
	}
	checksumData, err := client.Download(ctx, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return res, err
	}
	expected, err := ExpectedChecksum(checksumData, check.AssetName)
	if err != nil {
		return res, err
	}
	if err := VerifySHA256(assetData, expected); err != nil {
		return res, err
	}

	install, err := Install(ctx, assetData, InstallOptions{
		GOOS:          opts.Platform.normalize().OS,
		ExePath:       opts.ExePath,
		TempDir:       opts.TempDir,
		Version:       check.LatestVersion,
		CommandRunner: opts.CommandRunner,
	})
	res.Installed = install.Installed
	res.WindowsManual = install.WindowsManual
	res.TargetPath = install.TargetPath
	res.ReplacementPath = install.ReplacementPath
	res.BackupPath = install.BackupPath
	return res, err
}
