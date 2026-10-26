package screens

import (
	"strings"
	"unicode/utf8"
)

// foldRune maps a Latin letter carrying one of the game font's verified
// diacritics (AESTHETIC.md — the set names may use) to its base ASCII
// letter, preserving case. The set is closed, so the fold is a plain
// switch: no unicode tables, no surprises outside the canon alphabet.
func foldRune(r rune) rune {
	switch r {
	case 'ą':
		return 'a'
	case 'ć':
		return 'c'
	case 'ę':
		return 'e'
	case 'ł':
		return 'l'
	case 'ń':
		return 'n'
	case 'ó':
		return 'o'
	case 'ś':
		return 's'
	case 'ź':
		return 'z'
	case 'ż':
		return 'z'
	case 'č':
		return 'c'
	case 'ď':
		return 'd'
	case 'ě':
		return 'e'
	case 'í':
		return 'i'
	case 'ň':
		return 'n'
	case 'ř':
		return 'r'
	case 'š':
		return 's'
	case 'ť':
		return 't'
	case 'ů':
		return 'u'
	case 'ž':
		return 'z'
	case 'Ą':
		return 'A'
	case 'Ć':
		return 'C'
	case 'Ę':
		return 'E'
	case 'Ł':
		return 'L'
	case 'Ń':
		return 'N'
	case 'Ó':
		return 'O'
	case 'Ś':
		return 'S'
	case 'Ź':
		return 'Z'
	case 'Ż':
		return 'Z'
	case 'Č':
		return 'C'
	case 'Ď':
		return 'D'
	case 'Ě':
		return 'E'
	case 'Í':
		return 'I'
	case 'Ň':
		return 'N'
	case 'Ř':
		return 'R'
	case 'Š':
		return 'S'
	case 'Ť':
		return 'T'
	case 'Ů':
		return 'U'
	case 'Ž':
		return 'Z'
	}
	return r
}

// foldString folds s's diacritics to base ASCII letters, so a query typed
// on any keyboard matches names the other way round too ("zaklady" finds
// Zakłady, and a typed "zakłady" still matches). The second result maps
// every folded byte to the byte offset of the source rune it came from —
// the fuzzy library reports matched BYTE offsets into the folded string,
// and the remap returns them as rune starts of the original. It is nil
// when nothing folded: the all-ASCII common case stays allocation-free
// and byte-exact.
func foldString(s string) (string, []int) {
	folds := false
	for _, r := range s {
		if foldRune(r) != r {
			folds = true
			break
		}
	}
	if !folds {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	orig := make([]int, 0, len(s))
	for off := 0; off < len(s); {
		r, size := utf8.DecodeRuneInString(s[off:])
		if f := foldRune(r); f != r {
			b.WriteByte(byte(f))
			orig = append(orig, off)
		} else {
			for i := off; i < off+size; i++ {
				b.WriteByte(s[i])
				orig = append(orig, off)
			}
		}
		off += size
	}
	return b.String(), orig
}
