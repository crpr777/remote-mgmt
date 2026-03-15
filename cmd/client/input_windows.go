//go:build windows

package main

import (
	"fmt"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procSendInput        = user32.NewProc("SendInput")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	INPUT_MOUSE    = 0
	INPUT_KEYBOARD = 1

	// Mouse event flags
	MOUSEEVENTF_MOVE        = 0x0001
	MOUSEEVENTF_LEFTDOWN    = 0x0002
	MOUSEEVENTF_LEFTUP      = 0x0004
	MOUSEEVENTF_RIGHTDOWN   = 0x0008
	MOUSEEVENTF_RIGHTUP     = 0x0010
	MOUSEEVENTF_MIDDLEDOWN  = 0x0020
	MOUSEEVENTF_MIDDLEUP    = 0x0040
	MOUSEEVENTF_WHEEL       = 0x0800
	MOUSEEVENTF_HWHEEL      = 0x1000
	MOUSEEVENTF_ABSOLUTE    = 0x8000
	MOUSEEVENTF_VIRTUALDESK = 0x4000

	// Keyboard event flags
	KEYEVENTF_KEYUP   = 0x0002
	KEYEVENTF_UNICODE = 0x0004

	// System metrics
	SM_CXSCREEN = 0
	SM_CYSCREEN = 1
)

// INPUT structure for SendInput
type mouseInput struct {
	typ uint32
	mi  mouseInputData
}

type mouseInputData struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type keyboardInput struct {
	typ uint32
	ki  keyboardInputData
}

type keyboardInputData struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
	_           [8]byte // padding to match INPUT union size
}

// mouseAction is the unified interface called by main.go
func mouseAction(input MouseInput) error {
	switch input.Action {
	case "move":
		return setCursorPos(input.X, input.Y)
	case "click":
		if err := setCursorPos(input.X, input.Y); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
		return sendMouseClick(MOUSEEVENTF_LEFTDOWN | MOUSEEVENTF_LEFTUP)
	case "doubleclick":
		if err := setCursorPos(input.X, input.Y); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
		if err := sendMouseClick(MOUSEEVENTF_LEFTDOWN | MOUSEEVENTF_LEFTUP); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
		return sendMouseClick(MOUSEEVENTF_LEFTDOWN | MOUSEEVENTF_LEFTUP)
	case "rightclick":
		if err := setCursorPos(input.X, input.Y); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
		return sendMouseClick(MOUSEEVENTF_RIGHTDOWN | MOUSEEVENTF_RIGHTUP)
	case "scroll":
		return sendMouseScroll(input.DeltaY)
	default:
		return fmt.Errorf("unknown mouse action: %s", input.Action)
	}
}

func setCursorPos(x, y int) error {
	ret, _, err := procSetCursorPos.Call(uintptr(x), uintptr(y))
	if ret == 0 {
		return fmt.Errorf("SetCursorPos failed: %v", err)
	}
	return nil
}

func sendMouseClick(flags uint32) error {
	var inputs [2]mouseInput

	// Mouse down
	inputs[0] = mouseInput{
		typ: INPUT_MOUSE,
		mi: mouseInputData{
			dwFlags: flags & (MOUSEEVENTF_LEFTDOWN | MOUSEEVENTF_RIGHTDOWN | MOUSEEVENTF_MIDDLEDOWN),
		},
	}

	// Mouse up
	inputs[1] = mouseInput{
		typ: INPUT_MOUSE,
		mi: mouseInputData{
			dwFlags: flags & (MOUSEEVENTF_LEFTUP | MOUSEEVENTF_RIGHTUP | MOUSEEVENTF_MIDDLEUP),
		},
	}

	// If both down and up are combined, send both
	if flags&(MOUSEEVENTF_LEFTDOWN|MOUSEEVENTF_LEFTUP) == (MOUSEEVENTF_LEFTDOWN | MOUSEEVENTF_LEFTUP) {
		inputs[0].mi.dwFlags = MOUSEEVENTF_LEFTDOWN
		inputs[1].mi.dwFlags = MOUSEEVENTF_LEFTUP
	} else if flags&(MOUSEEVENTF_RIGHTDOWN|MOUSEEVENTF_RIGHTUP) == (MOUSEEVENTF_RIGHTDOWN | MOUSEEVENTF_RIGHTUP) {
		inputs[0].mi.dwFlags = MOUSEEVENTF_RIGHTDOWN
		inputs[1].mi.dwFlags = MOUSEEVENTF_RIGHTUP
	}

	ret, _, err := procSendInput.Call(
		2,
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if ret != 2 {
		return fmt.Errorf("SendInput failed: %v", err)
	}
	return nil
}

func sendMouseScroll(delta int) error {
	input := mouseInput{
		typ: INPUT_MOUSE,
		mi: mouseInputData{
			dwFlags:   MOUSEEVENTF_WHEEL,
			mouseData: uint32(delta * 120), // 120 = one notch
		},
	}

	ret, _, err := procSendInput.Call(
		1,
		uintptr(unsafe.Pointer(&input)),
		unsafe.Sizeof(input),
	)
	if ret != 1 {
		return fmt.Errorf("SendInput scroll failed: %v", err)
	}
	return nil
}

// keyboardAction is the unified interface called by main.go
func keyboardAction(input KeyboardInput) error {
	switch input.Action {
	case "type":
		return sendUnicodeString(input.Text)
	case "keydown":
		return sendKey(input.Key, false)
	case "keyup":
		return sendKey(input.Key, true)
	case "hotkey":
		return sendHotkey(input.Text, input.Modifiers)
	default:
		return fmt.Errorf("unknown keyboard action: %s", input.Action)
	}
}

func sendUnicodeString(text string) error {
	// Convert to UTF-16
	runes := utf16.Encode([]rune(text))

	for _, r := range runes {
		// Key down
		inputDown := keyboardInput{
			typ: INPUT_KEYBOARD,
			ki: keyboardInputData{
				wScan:   r,
				dwFlags: KEYEVENTF_UNICODE,
			},
		}

		// Key up
		inputUp := keyboardInput{
			typ: INPUT_KEYBOARD,
			ki: keyboardInputData{
				wScan:   r,
				dwFlags: KEYEVENTF_UNICODE | KEYEVENTF_KEYUP,
			},
		}

		ret, _, err := procSendInput.Call(
			1,
			uintptr(unsafe.Pointer(&inputDown)),
			unsafe.Sizeof(inputDown),
		)
		if ret != 1 {
			return fmt.Errorf("SendInput key down failed: %v", err)
		}

		ret, _, err = procSendInput.Call(
			1,
			uintptr(unsafe.Pointer(&inputUp)),
			unsafe.Sizeof(inputUp),
		)
		if ret != 1 {
			return fmt.Errorf("SendInput key up failed: %v", err)
		}
	}
	return nil
}

// Virtual key codes for common keys
var vkCodes = map[string]uint16{
	"backspace": 0x08,
	"tab":       0x09,
	"enter":     0x0D,
	"return":    0x0D,
	"shift":     0x10,
	"ctrl":      0x11,
	"control":   0x11,
	"alt":       0x12,
	"escape":    0x1B,
	"esc":       0x1B,
	"space":     0x20,
	" ":         0x20,
	"left":      0x25,
	"up":        0x26,
	"right":     0x27,
	"down":      0x28,
	"delete":    0x2E,
	"del":       0x2E,
	"f1":        0x70,
	"f2":        0x71,
	"f3":        0x72,
	"f4":        0x73,
	"f5":        0x74,
	"f6":        0x75,
	"f7":        0x76,
	"f8":        0x77,
	"f9":        0x78,
	"f10":       0x79,
	"f11":       0x7A,
	"f12":       0x7B,
}

func sendKey(key string, keyUp bool) error {
	vk, ok := vkCodes[key]
	if !ok {
		// Try as single character
		if len(key) == 1 {
			return sendUnicodeString(key)
		}
		return fmt.Errorf("unknown key: %s", key)
	}

	flags := uint32(0)
	if keyUp {
		flags = KEYEVENTF_KEYUP
	}

	input := keyboardInput{
		typ: INPUT_KEYBOARD,
		ki: keyboardInputData{
			wVk:     vk,
			dwFlags: flags,
		},
	}

	ret, _, err := procSendInput.Call(
		1,
		uintptr(unsafe.Pointer(&input)),
		unsafe.Sizeof(input),
	)
	if ret != 1 {
		return fmt.Errorf("SendInput key failed: %v", err)
	}
	return nil
}

func sendHotkey(key string, modifiers []string) error {
	// Press modifiers
	for _, mod := range modifiers {
		if err := sendKey(mod, false); err != nil {
			return err
		}
	}

	// Press main key
	if err := sendUnicodeString(key); err != nil {
		return err
	}

	// Release modifiers (in reverse)
	for i := len(modifiers) - 1; i >= 0; i-- {
		if err := sendKey(modifiers[i], true); err != nil {
			return err
		}
	}

	return nil
}
