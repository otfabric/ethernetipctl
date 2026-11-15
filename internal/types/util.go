// SPDX-License-Identifier: MIT

package types

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func sortStrings(s []string) { sort.Strings(s) }

// printable returns a string a device sent for text and table output: every rune that
// strconv.Quote escapes (control characters, DEL, C1, U+2028 and U+2029, other runes that
// do not print) and every octet that is not UTF-8 is written as its Go escape ("\x1b",
// "\r", "\u2028", "\xff"); everything else is kept as it is. JSON escapes these itself.
func printable(s string) string {
	plain := true
	for _, r := range s {
		if r == utf8.RuneError || !strconv.IsPrint(r) {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			_, _ = fmt.Fprintf(&b, `\x%02x`, s[i])
		case strconv.IsPrint(r):
			b.WriteString(s[i : i+n])
		default:
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
		}
		i += n
	}
	return b.String()
}
