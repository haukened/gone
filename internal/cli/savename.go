package cli

import (
	"strings"
)

// windowsInvalid lists characters Windows forbids in file names that
// envelope.SanitizeFileName does not already remove.
const windowsInvalid = `<>:"|?*`

// reservedStems are Windows device names, which are unusable as file
// names with or without an extension (docs/protocol.md §6.1).
var reservedStems = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"CONIN$": true, "CONOUT$": true,
}

// saveName applies the portable save-name policy to an already sanitized
// attachment name. The same policy is used on every OS so a link saves
// identically everywhere. A leading '.' or '-' becomes '_', so a sender
// cannot plant hidden startup files (.zshenv, .gitconfig) in the output
// directory or produce a name that reads as a command-line flag.
//
// Parameters:
//   - name: name from envelope.Unpack (already sanitized).
//
// Returns a name that is safe to create on Windows, macOS and Linux.
func saveName(name string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(windowsInvalid, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return fallbackSaveName
	}
	if name[0] == '.' || name[0] == '-' {
		name = "_" + name[1:]
	}
	if isReservedStem(name) {
		return "_" + name
	}
	return name
}

// fallbackSaveName replaces names that become empty.
const fallbackSaveName = "file"

// isReservedStem reports whether the part of name before the first '.' is a
// Windows device name, ignoring case and trailing spaces.
//
// Parameters:
//   - name: candidate file name.
//
// Returns true when Windows would treat name as a device.
func isReservedStem(name string) bool {
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	if reservedStems[stem] {
		return true
	}
	return isNumberedReservedStem(stem)
}

// isNumberedReservedStem reports whether stem is a COM/LPT device with a
// Windows-recognized number suffix.
//
// Parameters:
//   - stem: upper-cased candidate stem.
//
// Returns true for COM1-COM9, LPT1-LPT9, and superscript ¹-³ variants.
func isNumberedReservedStem(stem string) bool {
	if len(stem) < 4 {
		return false
	}
	prefix, digit := stem[:3], stem[3:]
	return isReservedPortPrefix(prefix) && isReservedPortDigit(digit)
}

// isReservedPortPrefix reports whether prefix names a numbered device class.
//
// Parameters:
//   - prefix: candidate three-character prefix.
//
// Returns true for COM and LPT.
func isReservedPortPrefix(prefix string) bool {
	return prefix == "COM" || prefix == "LPT"
}

// isReservedPortDigit reports whether digit is a Windows device number suffix.
//
// Parameters:
//   - digit: candidate suffix.
//
// Returns true for ASCII 1-9 or superscript ¹-³.
func isReservedPortDigit(digit string) bool {
	return (len(digit) == 1 && digit[0] >= '1' && digit[0] <= '9') || isSuperscriptDigit(digit)
}

// isSuperscriptDigit reports whether s is ¹, ² or ³, which Windows also
// accepts as COM/LPT port numbers.
//
// Parameters:
//   - s: candidate suffix.
//
// Returns true for a single superscript one, two or three.
func isSuperscriptDigit(s string) bool {
	return s == "¹" || s == "²" || s == "³"
}
