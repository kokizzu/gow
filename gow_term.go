package main

import (
	"github.com/mitranim/gg"
	"golang.org/x/sys/unix"
)

// https://en.wikipedia.org/wiki/ANSI_escape_code
const (
	// Standard terminal escape sequence. Same as "\x1b" or "\033".
	TermEsc = string(rune(27))

	// Control Sequence Introducer. Used for other codes.
	TermEscCsi = TermEsc + `[`

	// Update cursor position to first row, first column.
	TermEscCup = TermEscCsi + `1;1H`

	// Supposed to clear the screen without clearing the scrollback, aka soft
	// clear. Seems insufficient on its own, at least in some terminals.
	TermEscErase2 = TermEscCsi + `2J`

	// Supposed to clear the screen and the scrollback, aka hard clear. Seems
	// insufficient on its own, at least in some terminals.
	TermEscErase3 = TermEscCsi + `3J`

	// Supposed to reset the terminal to initial state, aka super hard clear.
	// Seems insufficient on its own, at least in some terminals.
	TermEscReset = TermEsc + `c`

	// Clear screen without clearing scrollback.
	TermEscClearSoft = TermEscCup + TermEscErase2

	// Clear screen AND scrollback.
	TermEscClearHard = TermEscCup + TermEscReset + TermEscErase3
)

/*
By default, any regular terminal uses what's known as "cooked mode", where the
terminal buffers lines before sending them to the foreground process, and
interprets ASCII control codes on stdin by sending the corresponding OS signals
to the process. We switch it into "raw mode", where it mostly forwards inputs
to our process's stdin as-is, and interprets fewer special ASCII codes.

This allows to support special key combinations such as ^R for restarting a
subprocess. Unfortunately, this also makes us responsible for interpreting the
rest of the ASCII control codes. Our current support for that is incomplete.

The terminal state is shared between all super- and sub-processes. Changes
persist even after our process terminates. We endeavor to restore the previous
state before exiting.

References:

	https://en.wikibooks.org/wiki/Serial_Programming/termios

	man termios
*/
type TermState struct{ gg.Opt[unix.Termios] }

func (self *TermState) Deinit() {
	if !self.IsNull() {
		defer self.Clear()
		gg.Nop1(unix.IoctlSetTermios(FD_TERM, ioctlWriteTermios, &self.Val))
	}
}

func (self *TermState) Init(main *Main) {
	self.Deinit()

	if !main.Opt.Raw {
		return
	}

	state, err := unix.IoctlGetTermios(FD_TERM, ioctlReadTermios)
	if err != nil {
		log.Println(`unable to read terminal state:`, err)
		return
	}
	prev := *state

	/**
	In raw mode, we support multiple modes of echoing stdin to stdout. Each
	approach has different issues.

	Most terminals, in addition to echoing non-special characters, also have
	special support for various ASCII control codes, printing them in the
	so-called "caret notation". Codes that send signals are cosmetically printed
	as hotkeys such as `^C`, `^R`, and so on. The delete code (127) should cause
	the terminal to delete one character before the caret, moving the caret. At
	the time of writing, the built-in MacOS terminal doesn't properly handle the
	delete character when operating in raw mode, printing it in the caret
	notation `^?`, which is a jarring and useless change from non-raw mode.

	The workaround we use by default (mode `EchoModeGow`) is to suppress default
	echoing in raw mode, and echo by ourselves in the `Stdio` type. We don't
	print the caret notation at all. This works fine for most characters, but at
	least in some terminals, deletion via the delete character (see above)
	doesn't seem to work when we echo the character as-is.

	Other modes allow to suppress echoing completely or fall back on the buggy
	terminal default.
	*/
	switch main.Opt.Echo {
	case EchoModeNone:
		state.Lflag &^= unix.ECHO

	case EchoModeGow:
		// We suppress the default echoing here and replicate it ourselves in
		// `Stdio.OnByteAny`.
		state.Lflag &^= unix.ECHO

	case EchoModePreserve:
		// The point of this mode is to preserve the previous echo mode of the
		// terminal, whatever it was.

	default:
		panic(main.Opt.Echo.errInvalid())
	}

	// Don't buffer lines.
	state.Lflag &^= unix.ICANON

	// No signals.
	state.Lflag &^= unix.ISIG

	// Seems unnecessary on my system. Might be needed elsewhere.
	// state.Cflag |= unix.CS8
	// state.Cc[unix.VMIN] = 1
	// state.Cc[unix.VTIME] = 0

	err = unix.IoctlSetTermios(FD_TERM, ioctlWriteTermios, state)
	if err != nil {
		log.Println(`unable to switch terminal to raw mode:`, err)
		return
	}

	self.Set(prev)
}
