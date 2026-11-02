package main

// Windows: pasta da Steam pelo registro e seleção de pasta pela API do shell (SHBrowseForFolderW),
// chamada direto pelo syscall, sem cgo.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

func steamRoots() []string {
	var roots []string
	for _, k := range []struct {
		root        syscall.Handle
		path, value string
	}{
		{syscall.HKEY_LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"},
		{syscall.HKEY_LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath"},
		{syscall.HKEY_CURRENT_USER, `Software\Valve\Steam`, "SteamPath"}, // em minúsculas
	} {
		if v := regString(k.root, k.path, k.value); v != "" {
			roots = append(roots, filepath.FromSlash(v))
		}
	}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if p := os.Getenv(env); p != "" {
			roots = append(roots, filepath.Join(p, "Steam"))
		}
	}
	return roots
}

func regString(root syscall.Handle, path, name string) string {
	var key syscall.Handle
	if syscall.RegOpenKeyEx(root, utf16Ptr(path), 0, syscall.KEY_READ, &key) != nil {
		return ""
	}
	defer syscall.RegCloseKey(key)
	buf := make([]uint16, 1024)
	size := uint32(len(buf) * 2)
	var typ uint32
	err := syscall.RegQueryValueEx(key, utf16Ptr(name), nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size)
	if err != nil || typ != syscall.REG_SZ {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

var (
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	ole32                    = syscall.NewLazyDLL("ole32.dll")
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	procCoInitializeEx       = ole32.NewProc("CoInitializeEx")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	procGetConsoleWindow     = kernel32.NewProc("GetConsoleWindow")
)

// browseInfo é a struct BROWSEINFOW.
type browseInfo struct {
	owner       uintptr
	root        uintptr
	displayName *uint16
	title       *uint16
	flags       uint32
	callback    uintptr
	lParam      uintptr
	image       int32
}

const (
	bifReturnOnlyFSDirs = 0x0001
	bifNewDialogStyle   = 0x0040
	coinitApartment     = 0x2
)

// pickFolder abre a janela de seleção de pasta. Devolve "" se o usuário cancelar.
func pickFolder(title string) (string, error) {
	// a janela usa COM, que exige a mesma thread do CoInitializeEx
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procCoInitializeEx.Call(0, coinitApartment)

	owner, _, _ := procGetConsoleWindow.Call() // abre na frente do terminal
	name := make([]uint16, syscall.MAX_PATH)
	bi := browseInfo{
		owner:       owner,
		displayName: &name[0],
		title:       utf16Ptr(title),
		flags:       bifReturnOnlyFSDirs | bifNewDialogStyle,
	}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil
	}
	defer procCoTaskMemFree.Call(pidl)
	path := make([]uint16, syscall.MAX_PATH)
	if ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0]))); ok == 0 {
		return "", errors.New("a pasta escolhida não está no disco")
	}
	return syscall.UTF16ToString(path), nil
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}
