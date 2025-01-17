package ssh

import (
	"regexp"
	"strconv"
	"strings"
)

// KeyText keeps mapping a string representation to keys.
// https://github.com/jesseduffield/lazygit/blob/master/pkg/gui/keybindings.go
// https://github.com/Nerdmaster/terminal/blob/master/key_constants.go
// https://github.com/gdamore/tcell/blob/master/key.go
var KeyText = map[string]rune{
	"c-a": KeyCtrlA, "c-b": KeyCtrlB, "c-c": KeyCtrlC, "c-d": KeyCtrlD, "c-e": KeyCtrlE, "c-f": KeyCtrlF,
	"c-g": KeyCtrlG, "c-h": KeyCtrlH, "c-i": KeyCtrlI, "c-j": KeyCtrlJ, "c-k": KeyCtrlK, "c-l": KeyCtrlL,
	"c-m": KeyCtrlM, "c-n": KeyCtrlN, "c-o": KeyCtrlO, "c-p": KeyCtrlP, "c-q": KeyCtrlQ, "c-r": KeyCtrlR,
	"c-s": KeyCtrlS, "c-t": KeyCtrlT, "c-u": KeyCtrlU, "c-v": KeyCtrlV, "c-w": KeyCtrlW, "c-x": KeyCtrlX,
	"c-y": KeyCtrlY, "c-z": KeyCtrlZ,

	"CtrlA": KeyCtrlA, "CtrlB": KeyCtrlB, "CtrlC": KeyCtrlC, "CtrlD": KeyCtrlD, "CtrlE": KeyCtrlE, "CtrlF": KeyCtrlF,
	"CtrlG": KeyCtrlG, "CtrlH": KeyCtrlH, "CtrlI": KeyCtrlI, "CtrlJ": KeyCtrlJ, "CtrlK": KeyCtrlK, "CtrlL": KeyCtrlL,
	"CtrlM": KeyCtrlM, "CtrlN": KeyCtrlN, "CtrlO": KeyCtrlO, "CtrlP": KeyCtrlP, "CtrlQ": KeyCtrlQ, "CtrlR": KeyCtrlR,
	"CtrlS": KeyCtrlS, "CtrlT": KeyCtrlT, "CtrlU": KeyCtrlU, "CtrlV": KeyCtrlV, "CtrlW": KeyCtrlW, "CtrlX": KeyCtrlX,
	"CtrlY": KeyCtrlY, "CtrlZ": KeyCtrlZ, "Escape": KeyEscape, "LeftBracket": KeyLeftBracket,
	"RightBracket": KeyRightBracket, "Enter": KeyEnter, "N": KeyEnter, "Backspace": KeyBackspace,
	"Up": KeyUp, "Down": KeyDown, "Left": KeyLeft, "Right": KeyRight,
	"Home": KeyHome, "End": KeyEnd, "PasteStart": KeyPasteStart, "PasteEnd": KeyPasteEnd, "Insert": KeyInsert,
	"Del": KeyDelete, "PgUp": KeyPgUp, "PgDn": KeyPgDn, "Pause": KeyPause,
	"F1": KeyF1, "F2": KeyF2, "F3": KeyF3, "F4": KeyF4, "F5": KeyF5, "F6": KeyF6, "F7": KeyF7,
	"F8": KeyF8, "F9": KeyF9, "F10": KeyF10, "F11": KeyF11, "F12": KeyF12,
}

var numReg = regexp.MustCompile(`^\d+`)

func ConvertKeys(s string) [][]byte {
	groups := make([][]byte, 0)

	for s != "" {
		start := strings.Index(s, "{")
		end := strings.Index(s, "}")
		if start < 0 || end < 0 || start > end {
			groups = append(groups, []byte(s))
			break
		}

		if start > 0 {
			groups = append(groups, []byte(s[:start]))
		}

		rawKey := strings.TrimSpace(s[start+1 : end])
		if rawKey == "-" {
			groups = append(groups, []byte(""))
		} else {
			key := rawKey
			num := 1
			if numStr := numReg.FindString(key); numStr != "" {
				num, _ = strconv.Atoi(numStr)
				key = key[len(numStr):]
			}

			found := false
			for k, v := range KeyText {
				if strings.EqualFold(k, key) {
					vbytes := []byte(string([]rune{v}))
					for i := 0; i < num; i++ {
						groups = append(groups, vbytes)
					}
					found = true
					break

				}
			}

			if !found {
				groups = append(groups, []byte(rawKey))
			}
		}
		s = s[end+1:]
	}

	return groups
}

// Giant list of key constants.  Everything above KeyUnknown matches an actual
// ASCII key value.  After that, we have various pseudo-keys in order to
// represent complex byte sequences that correspond to keys like Page up, Right
// arrow, etc.
const (
	KeyCtrlA = 1 + iota
	KeyCtrlB
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlF
	KeyCtrlG
	KeyCtrlH
	KeyCtrlI
	KeyCtrlJ
	KeyCtrlK
	KeyCtrlL
	KeyCtrlM
	KeyCtrlN
	KeyCtrlO
	KeyCtrlP
	KeyCtrlQ
	KeyCtrlR
	KeyCtrlS
	KeyCtrlT
	KeyCtrlU
	KeyCtrlV
	KeyCtrlW
	KeyCtrlX
	KeyCtrlY
	KeyCtrlZ
	KeyEscape
	KeyLeftBracket  = '['
	KeyRightBracket = ']'
	KeyEnter        = '\n'
	KeyBackspace    = 127
	KeyUnknown      = 0xd800 /* UTF-16 surrogate area */ + iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPasteStart
	KeyPasteEnd
	KeyInsert
	KeyDelete
	KeyPgUp
	KeyPgDn
	KeyPause
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)
