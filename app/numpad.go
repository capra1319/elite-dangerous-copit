package main

// These are Windows virtual-key identifiers. No key input is injected or hooked:
// the platform reader only consumes events belonging to this app's console.
type consoleKey struct {
	Down       bool
	VirtualKey uint16
	Controls   uint32
}

type keyLatch struct{ pressed map[uint16]bool }

func (l *keyLatch) command(k consoleKey) string {
	if l.pressed == nil {
		l.pressed = make(map[uint16]bool)
	}
	if !k.Down {
		delete(l.pressed, k.VirtualKey)
		return ""
	}
	if l.pressed[k.VirtualKey] {
		return ""
	}
	l.pressed[k.VirtualKey] = true
	// Ignore Alt, Ctrl and Shift combinations. Num Lock/Caps Lock are harmless.
	if k.Controls&0x1f != 0 {
		return ""
	}
	switch k.VirtualKey {
	case 0x61:
		return "t1 toggle"
	case 0x62:
		return "t2 toggle"
	case 0x63:
		return "t3 toggle"
	case 0x64:
		return "t4 toggle"
	case 0x65:
		return "b1"
	case 0x66:
		return "b2"
	case 0x67:
		return "b3"
	case 0x68:
		return "b4"
	case 0x69:
		return "b5"
	case 0x60:
		return "b6"
	case 0x0d:
		return "close"
	case 0x70:
		return "help"
	case 0x1b:
		return "quit"
	default:
		return ""
	}
}

const numpadHelp = `NUMPAD ALTERNATE CONTROLS (mock only)
  Num 1    Toggle landing gear       Num 6    Galaxy map
  Num 2    Toggle cargo scoop        Num 7    System map
  Num 3    Toggle ship lights        Num 8    Next fire group
  Num 4    Toggle hardpoints         Num 9    Heat sink (simulated)
  Num 5    Toggle night vision       Num 0    Chaff (simulated)
  Enter    Close simulated map       Esc      Quit
Use Num Lock ON and keep this terminal focused. No Enter after numpad presses.
Hold a key = one action; release it before the next press.
Top-row numbers are not shortcuts. No keys are sent to the real game.
For scenarios or explicit on/off commands, relaunch Start-Mock.cmd.`
