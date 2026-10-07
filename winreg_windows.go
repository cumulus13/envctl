//go:build windows

package main

import (
	"fmt"
	"sort"
	"syscall"
	"unsafe"
)

// All constants below are stable, public Win32 ABI values, unchanged since
// Windows NT. This is the documented registry API — the same mechanism
// regedit.exe, System Properties, and setx.exe use. It is NOT the PEB/
// process-memory technique: it only ever touches the registry values that
// represent *permanent* env vars, never another process's live memory.
const (
	hkeyCurrentUser  = 0x80000001
	hkeyLocalMachine = 0x80000002

	keyQueryValue       = 0x0001
	keySetValue         = 0x0002
	keyEnumerateSubKeys = 0x0008
	keyNotify           = 0x0010
	standardRightsRead  = 0x00020000
	standardRightsWrite = 0x00020000
	syncronizeMask      = 0x00100000
	keyRead             = (standardRightsRead | keyQueryValue | keyEnumerateSubKeys | keyNotify) &^ syncronizeMask
	keyWrite            = (standardRightsWrite | keySetValue) &^ syncronizeMask

	regSZ       = 1
	regExpandSZ = 2

	errorSuccess   = 0
	errorMoreData  = 234
	errorNoMoreItm = 259

	hwndBroadcast   = 0xffff
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var (
	advapi32               = syscall.NewLazyDLL("advapi32.dll")
	user32                 = syscall.NewLazyDLL("user32.dll")
	procRegOpenKeyExW      = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW   = advapi32.NewProc("RegQueryValueExW")
	procRegSetValueExW     = advapi32.NewProc("RegSetValueExW")
	procRegEnumValueW      = advapi32.NewProc("RegEnumValueW")
	procRegCloseKey        = advapi32.NewProc("RegCloseKey")
	procRegFlushKey        = advapi32.NewProc("RegFlushKey")
	procSendMessageTimeout = user32.NewProc("SendMessageTimeoutW")
)

// scope identifies which registry hive/subkey holds the permanent vars.
type scope int

const (
	scopeUser scope = iota
	scopeSystem
)

func (s scope) String() string {
	if s == scopeSystem {
		return "SYSTEM"
	}
	return "USER"
}

func (s scope) hive() uintptr {
	if s == scopeSystem {
		return hkeyLocalMachine
	}
	return hkeyCurrentUser
}

func (s scope) subKey() string {
	if s == scopeSystem {
		return `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	}
	return `Environment`
}

func regOpenKey(hive uintptr, subKey string, access uint32) (syscall.Handle, error) {
	var h syscall.Handle
	p, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return 0, err
	}
	r, _, _ := procRegOpenKeyExW.Call(
		hive,
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(access),
		uintptr(unsafe.Pointer(&h)),
	)
	if r != errorSuccess {
		return 0, fmt.Errorf("RegOpenKeyExW(%s) failed: error 0x%x", subKey, r)
	}
	return h, nil
}

func regClose(h syscall.Handle) {
	procRegCloseKey.Call(uintptr(h))
}

// envEntry is one permanent environment variable read from the registry.
type envEntry struct {
	Name  string
	Value string
	Type  uint32 // regSZ or regExpandSZ
	Scope scope
}

// listPermanent enumerates every value under the given scope's Environment key.
func listPermanent(s scope) ([]envEntry, error) {
	h, err := regOpenKey(s.hive(), s.subKey(), keyRead)
	if err != nil {
		return nil, err
	}
	defer regClose(h)

	var out []envEntry
	for i := uint32(0); ; i++ {
		nameBuf := make([]uint16, 16384)
		nameLen := uint32(len(nameBuf))
		var valType uint32
		dataBuf := make([]byte, 65536)
		dataLen := uint32(len(dataBuf))

		r, _, _ := procRegEnumValueW.Call(
			uintptr(h),
			uintptr(i),
			uintptr(unsafe.Pointer(&nameBuf[0])),
			uintptr(unsafe.Pointer(&nameLen)),
			0,
			uintptr(unsafe.Pointer(&valType)),
			uintptr(unsafe.Pointer(&dataBuf[0])),
			uintptr(unsafe.Pointer(&dataLen)),
		)
		if r == errorNoMoreItm {
			break
		}
		if r != errorSuccess {
			return nil, fmt.Errorf("RegEnumValueW index %d failed: error 0x%x", i, r)
		}
		name := syscall.UTF16ToString(nameBuf[:nameLen])
		if valType != regSZ && valType != regExpandSZ {
			continue // skip REG_DWORD etc. (not a normal env var)
		}
		val := utf16BufToString(dataBuf[:dataLen])
		out = append(out, envEntry{Name: name, Value: val, Type: valType, Scope: s})
	}
	sort.Slice(out, func(i, j int) bool {
		return lowerASCII(out[i].Name) < lowerASCII(out[j].Name)
	})
	return out, nil
}

func utf16BufToString(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	// trim trailing NUL(s)
	for len(u) > 0 && u[len(u)-1] == 0 {
		u = u[:len(u)-1]
	}
	return syscall.UTF16ToString(u)
}

// getPermanentValue reads a single value, returning ok=false if it doesn't exist.
func getPermanentValue(s scope, name string) (value string, regType uint32, ok bool, err error) {
	h, err := regOpenKey(s.hive(), s.subKey(), keyRead)
	if err != nil {
		return "", 0, false, err
	}
	defer regClose(h)

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", 0, false, err
	}
	var valType uint32
	var dataLen uint32
	// first call: get size
	r, _, _ := procRegQueryValueExW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		uintptr(unsafe.Pointer(&valType)),
		0,
		uintptr(unsafe.Pointer(&dataLen)),
	)
	if r == 2 /* ERROR_FILE_NOT_FOUND */ {
		return "", 0, false, nil
	}
	if r != errorSuccess && r != errorMoreData {
		return "", 0, false, fmt.Errorf("RegQueryValueExW(size) failed: error 0x%x", r)
	}
	if dataLen == 0 {
		return "", valType, true, nil
	}
	dataBuf := make([]byte, dataLen)
	r, _, _ = procRegQueryValueExW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		uintptr(unsafe.Pointer(&valType)),
		uintptr(unsafe.Pointer(&dataBuf[0])),
		uintptr(unsafe.Pointer(&dataLen)),
	)
	if r != errorSuccess {
		return "", 0, false, fmt.Errorf("RegQueryValueExW(data) failed: error 0x%x", r)
	}
	return utf16BufToString(dataBuf[:dataLen]), valType, true, nil
}

// setPermanentValue writes exactly ONE named value under the scope's
// Environment key. It never touches any other value — this is a targeted
// RegSetValueEx on a single name, not a block replace.
func setPermanentValue(s scope, name, value string, regType uint32) error {
	h, err := regOpenKey(s.hive(), s.subKey(), keyWrite)
	if err != nil {
		return err
	}
	defer regClose(h)

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	valUTF16, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	// valUTF16 includes its own trailing NUL already.
	dataBytes := make([]byte, len(valUTF16)*2)
	for i, u := range valUTF16 {
		dataBytes[2*i] = byte(u)
		dataBytes[2*i+1] = byte(u >> 8)
	}

	r, _, _ := procRegSetValueExW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		uintptr(regType),
		uintptr(unsafe.Pointer(&dataBytes[0])),
		uintptr(len(dataBytes)),
	)
	if r != errorSuccess {
		return fmt.Errorf("RegSetValueExW failed: error 0x%x (try running as Administrator for SYSTEM scope)", r)
	}
	procRegFlushKey.Call(uintptr(h))
	return nil
}

// broadcastEnvChange tells other top-level windows (Explorer, new shells,
// etc.) that the environment changed, the same WM_SETTINGCHANGE broadcast
// setx.exe and the System Properties dialog send. It does NOT and cannot
// alter the environment of already-running shells — see README/notes.
func broadcastEnvChange() {
	envPtr, _ := syscall.UTF16PtrFromString("Environment")
	procSendMessageTimeout.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(smtoAbortIfHung),
		5000,
		0,
	)
}
