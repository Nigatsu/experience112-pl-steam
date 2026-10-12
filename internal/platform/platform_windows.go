//go:build windows

package platform

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetLogicalDrives = kernel32.NewProc("GetLogicalDrives")
	procSetConsoleCP     = kernel32.NewProc("SetConsoleCP")
	procSetConsoleOutCP  = kernel32.NewProc("SetConsoleOutputCP")
	user32               = syscall.NewLazyDLL("user32.dll")
	procEnumDisplayDevW  = user32.NewProc("EnumDisplayDevicesW")
	ntdll                = syscall.NewLazyDLL("ntdll.dll")
)

const (
	keyWow64_64 = 0x0100
	cpUTF8      = 65001
)

// InitConsole switches the console to UTF-8 so Polish messages display.
func InitConsole() {
	procSetConsoleOutCP.Call(cpUTF8)
	procSetConsoleCP.Call(cpUTF8)
}

// IsWine reports whether the program runs under Wine/CrossOver.
func IsWine() bool {
	return ntdll.NewProc("wine_get_version").Find() == nil
}

// DriveRoots returns the roots of all logical drives ("C:\", ...).
func DriveRoots() []string {
	mask, _, _ := procGetLogicalDrives.Call()
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<i) != 0 {
			out = append(out, string(rune('A'+i))+`:\`)
		}
	}
	return out
}

// SteamRoots returns candidate Steam installation folders.
func SteamRoots() []string {
	var out []string
	for _, s := range []string{
		strings.ReplaceAll(regString(syscall.HKEY_CURRENT_USER, `Software\Valve\Steam`, "SteamPath"), "/", `\`),
		regString(syscall.HKEY_LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"),
		regString(syscall.HKEY_LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath"),
		`C:\Program Files (x86)\Steam`,
		`C:\Program Files\Steam`,
	} {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func openKey(root syscall.Handle, path string, access uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var k syscall.Handle
	err = syscall.RegOpenKeyEx(root, p, 0, access, &k)
	return k, err
}

func regString(root syscall.Handle, path, name string) string {
	k, err := openKey(root, path, syscall.KEY_READ|keyWow64_64)
	if err != nil {
		if k, err = openKey(root, path, syscall.KEY_READ); err != nil {
			return ""
		}
	}
	defer syscall.RegCloseKey(k)
	n, _ := syscall.UTF16PtrFromString(name)
	var typ, size uint32
	if syscall.RegQueryValueEx(k, n, nil, &typ, nil, &size) != nil || size == 0 {
		return ""
	}
	buf := make([]uint16, size/2+1)
	if syscall.RegQueryValueEx(k, n, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// UserDword reads a REG_DWORD from HKCU. ok is false if it does not exist.
func UserDword(path, name string) (v uint32, ok bool) {
	k, err := openKey(syscall.HKEY_CURRENT_USER, path, syscall.KEY_READ)
	if err != nil {
		return 0, false
	}
	defer syscall.RegCloseKey(k)
	n, _ := syscall.UTF16PtrFromString(name)
	var typ uint32
	size := uint32(4)
	if syscall.RegQueryValueEx(k, n, nil, &typ, (*byte)(unsafe.Pointer(&v)), &size) != nil || typ != syscall.REG_DWORD {
		return 0, false
	}
	return v, true
}

// SetUserDword writes a REG_DWORD to HKCU, creating the key if needed.
func SetUserDword(path, name string, v uint32) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var k syscall.Handle
	if r, _, _ := procRegCreateKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(p)), 0, 0, 0,
		uintptr(syscall.KEY_WRITE), 0, uintptr(unsafe.Pointer(&k)), 0); r != 0 {
		return syscall.Errno(r)
	}
	defer syscall.RegCloseKey(k)
	n, _ := syscall.UTF16PtrFromString(name)
	if r, _, _ := procRegSetValueExW.Call(uintptr(k), uintptr(unsafe.Pointer(n)), 0, syscall.REG_DWORD,
		uintptr(unsafe.Pointer(&v)), 4); r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

// DeleteUserValue removes a value from an HKCU key.
func DeleteUserValue(path, name string) error {
	k, err := openKey(syscall.HKEY_CURRENT_USER, path, syscall.KEY_WRITE)
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(k)
	n, _ := syscall.UTF16PtrFromString(name)
	if r, _, _ := procRegDeleteValueW.Call(uintptr(k), uintptr(unsafe.Pointer(n))); r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

type displayDevice struct {
	cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

const displayDevicePrimary = 4

// PrimaryGPU returns the primary display adapter, if it can be determined.
func PrimaryGPU() (GPU, bool) {
	for i := 0; i < 16; i++ {
		var d displayDevice
		d.cb = uint32(unsafe.Sizeof(d))
		if r, _, _ := procEnumDisplayDevW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&d)), 0); r == 0 {
			break
		}
		if d.StateFlags&displayDevicePrimary != 0 {
			id := strings.ToUpper(syscall.UTF16ToString(d.DeviceID[:]))
			return GPU{Name: syscall.UTF16ToString(d.DeviceString[:]), NVIDIA: strings.Contains(id, "VEN_10DE")}, true
		}
	}
	return GPU{}, false
}
