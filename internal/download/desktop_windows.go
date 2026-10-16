package download

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	desktopShell32           = windows.NewLazySystemDLL("shell32.dll")
	shellExecuteEx           = desktopShell32.NewProc("ShellExecuteExW")
	parseDisplayName         = desktopShell32.NewProc("SHParseDisplayName")
	openFolderAndSelectItems = desktopShell32.NewProc("SHOpenFolderAndSelectItems")
)

// SHELLEXECUTEINFOW from shellapi.h; pointers/handles use native alignment.
type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Window     windows.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   windows.Handle
	IDList     unsafe.Pointer
	Class      *uint16
	ClassKey   windows.Handle
	HotKey     uint32
	Icon       windows.Handle
	Process    windows.Handle
}

const (
	seeMaskNoAsync = 0x100 // Finish the handoff before this helper exits.
	seeMaskNoUI    = 0x400
)

func windowsOpenInfo(path string) (shellExecuteInfo, error) {
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return shellExecuteInfo{}, err
	}
	return shellExecuteInfo{
		Size: uint32(unsafe.Sizeof(shellExecuteInfo{})),
		Mask: seeMaskNoAsync | seeMaskNoUI,
		Verb: windows.StringToUTF16Ptr("open"), File: file, Show: windows.SW_SHOWNORMAL,
		// No SEE_MASK_DOENVSUBST, process handle, command arguments, or wait for
		// the launched application. Percent signs remain filename data.
	}, nil
}

func nativeDesktop(path string, reveal bool) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && !errors.Is(err, syscall.Errno(1)) { // S_FALSE also initialized COM.
		return fmt.Errorf("initialize desktop COM: %w", err)
	}
	defer windows.CoUninitialize()
	if reveal {
		item, err := parseWindowsItem(path)
		if err != nil {
			return err
		}
		defer windows.CoTaskMemFree(item)
		// cidl=0 selects the single fully qualified item and opens its parent.
		result, _, _ := openFolderAndSelectItems.Call(uintptr(item), 0, 0, 0)
		return desktopHRESULT("select desktop item", result)
	}
	info, err := windowsOpenInfo(path)
	if err != nil {
		return err
	}
	result, _, callErr := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		return fmt.Errorf("open desktop file: %w", callErr)
	}
	return nil
}

func parseWindowsItem(path string) (unsafe.Pointer, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var item unsafe.Pointer
	result, _, _ := parseDisplayName.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&item)), 0, 0)
	if err := desktopHRESULT("parse desktop item", result); err != nil {
		return nil, err
	}
	return item, nil
}

func desktopHRESULT(operation string, result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("%s: HRESULT 0x%08x", operation, uint32(result))
	}
	return nil
}
