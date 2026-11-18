package manage

import (
	"fmt"
	"strings"
	"unicode"
)

// suggestionCutoff is the similarity a name needs to be suggested, the
// default cutoff of Python's difflib.get_close_matches.
const suggestionCutoff = 0.6

// closeMatch returns the name most similar to word, the first element of
// difflib.get_close_matches(word, names) that Django suggests for an unknown
// command. Of two names with the same score the greater one wins, as in
// Python, where the candidates are ordered as (score, name) pairs.
func closeMatch(word string, names []string) (string, bool) {
	best, bestScore, found := "", 0.0, false
	for _, name := range names {
		score := ratio(name, word)
		if score < suggestionCutoff {
			continue
		}
		if !found || score > bestScore || (score == bestScore && name > best) {
			best, bestScore, found = name, score, true
		}
	}
	return best, found
}

// ratio is difflib.SequenceMatcher(None, a, b).ratio(): twice the number of
// matching characters over the total number of characters, where the matches
// are found by taking the longest common block and then recursing into the
// pieces to its left and to its right. difflib's junk heuristic only starts
// at 200 characters and is left out.
func ratio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	total := len(ra) + len(rb)
	if total == 0 {
		return 1
	}
	return 2 * float64(matchingRunes(ra, rb)) / float64(total)
}

// matchingRunes counts the characters of the matching blocks of a and b.
func matchingRunes(a, b []rune) int {
	ai, bi, size := longestMatch(a, b)
	if size == 0 {
		return 0
	}
	return matchingRunes(a[:ai], b[:bi]) + size + matchingRunes(a[ai+size:], b[bi+size:])
}

// longestMatch finds the longest block common to a and b. Of several blocks
// of that length it returns the one that starts earliest in a, and of those
// the one that starts earliest in b, as SequenceMatcher.find_longest_match
// does.
func longestMatch(a, b []rune) (ai, bi, size int) {
	// prev[j+1] is the length of the common block that ends at a[i-1] and
	// b[j]; cur is the same row for a[i].
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := range a {
		for j := range b {
			if a[i] != b[j] {
				cur[j+1] = 0
				continue
			}
			k := prev[j] + 1
			cur[j+1] = k
			if k > size {
				ai, bi, size = i-k+1, j-k+1, k
			}
		}
		prev, cur = cur, prev
	}
	return ai, bi, size
}

// pyRepr quotes s the way Python's repr does for a str, which is how Django
// and argparse show a value in a message (%r): single quotes, or double
// quotes when s holds a single quote and no double quote.
func pyRepr(s string) string {
	quote := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteRune(quote)
	for _, r := range s {
		switch {
		case r == quote || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsPrint(r):
			b.WriteRune(r)
		case r < 0x100:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x10000:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteRune(quote)
	return b.String()
}
