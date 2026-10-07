//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

// enableANSI turns on VT100/ANSI escape processing for the current console
// output handle. Modern Windows Terminal has this on already; classic
// conhost (plain cmd.exe window) needs it set explicitly or color escape
// codes print as literal garbage instead of colors.
func enableANSI() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	const stdOutputHandle = ^uintptr(11) + 1 // -11 as uintptr (STD_OUTPUT_HANDLE)
	h, _, _ := getStdHandle.Call(stdOutputHandle)
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return
	}
	var mode uint32
	r, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return // not a real console (e.g. redirected to a file) — fine, just skip
	}
	setConsoleMode.Call(h, uintptr(mode|enableVirtualTerminalProcessing))
}
