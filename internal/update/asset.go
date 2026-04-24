package update

import (
	"fmt"
	"runtime"
)

type Platform struct {
	OS   string
	Arch string
}

func CurrentPlatform() Platform {
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

func (p Platform) normalize() Platform {
	if p.OS == "" {
		p.OS = runtime.GOOS
	}
	if p.Arch == "" {
		p.Arch = runtime.GOARCH
	}
	return p
}

func (p Platform) AssetName() (string, error) {
	p = p.normalize()
	switch p {
	case Platform{OS: "darwin", Arch: "amd64"}:
		return "rdtui-darwin-amd64", nil
	case Platform{OS: "darwin", Arch: "arm64"}:
		return "rdtui-darwin-arm64", nil
	case Platform{OS: "linux", Arch: "amd64"}:
		return "rdtui-linux-amd64", nil
	case Platform{OS: "linux", Arch: "arm64"}:
		return "rdtui-linux-arm64", nil
	case Platform{OS: "windows", Arch: "amd64"}:
		return "rdtui-windows-amd64.exe", nil
	case Platform{OS: "windows", Arch: "arm64"}:
		return "rdtui-windows-arm64.exe", nil
	default:
		return "", fmt.Errorf("self-update is unsupported for %s/%s", p.OS, p.Arch)
	}
}

func SelectAsset(release Release, platform Platform) (Asset, string, error) {
	assetName, err := platform.AssetName()
	if err != nil {
		return Asset{}, "", err
	}
	asset, ok := AssetByName(release, assetName)
	if !ok {
		return Asset{}, assetName, fmt.Errorf("release %s does not contain expected asset %q", release.TagName, assetName)
	}
	if asset.BrowserDownloadURL == "" {
		return Asset{}, assetName, fmt.Errorf("release asset %q has no download URL", assetName)
	}
	return asset, assetName, nil
}

func AssetByName(release Release, name string) (Asset, bool) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return Asset{}, false
}
