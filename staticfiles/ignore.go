package staticfiles

import "slices"

// DefaultIgnorePatterns are Django's: "CVS", ".*", "*~".
var DefaultIgnorePatterns = []string{"CVS", ".*", "*~"}

// matchesPatterns reports whether name matches one of the patterns (Django's
// utils.matches_patterns).
func matchesPatterns(name string, patterns []string) bool {
	return slices.ContainsFunc(patterns, func(pattern string) bool {
		return fnmatch(pattern, name)
	})
}

// fnmatch reports whether the whole name matches the shell pattern, by the
// rules of Python's fnmatch.fnmatchcase, which Django's ignore patterns
// follow. They differ from path.Match: "*" and "?" also match a slash, a set
// is negated with "!" and not "^", a backslash is an ordinary character, and
// a "[" without its "]" matches itself, so no pattern is malformed.
func fnmatch(pattern, name string) bool {
	pat, str := []rune(pattern), []rune(name)
	p, s := 0, 0
	// star is the position of the last "*" seen and starEnd where its match
	// ends in str: on a mismatch the star takes one more character.
	star, starEnd := -1, 0
	for s < len(str) {
		if p < len(pat) {
			if pat[p] == '*' {
				star, starEnd = p, s
				p++
				continue
			}
			if width, ok := matchOne(pat[p:], str[s]); ok {
				p += width
				s++
				continue
			}
		}
		if star < 0 {
			return false
		}
		starEnd++
		p, s = star+1, starEnd
	}
	for p < len(pat) && pat[p] == '*' {
		p++
	}
	return p == len(pat)
}

// matchOne matches the character c against the first element of pat, which is
// not a "*": a "?", a set or a literal. It returns the length of the element
// and whether c matches it.
func matchOne(pat []rune, c rune) (width int, ok bool) {
	switch pat[0] {
	case '?':
		return 1, true
	case '[':
		end := setEnd(pat)
		if end < 0 {
			return 1, c == '['
		}
		return end + 1, setMatches(pat[1:end], c)
	default:
		return 1, pat[0] == c
	}
}

// setEnd returns the index of the "]" that closes the set starting at pat[0],
// or -1 when the set is not closed. A "]" right after the "[" or the "[!" is a
// member of the set.
func setEnd(pat []rune) int {
	j := 1
	if j < len(pat) && pat[j] == '!' {
		j++
	}
	if j < len(pat) && pat[j] == ']' {
		j++
	}
	for j < len(pat) && pat[j] != ']' {
		j++
	}
	if j >= len(pat) {
		return -1
	}
	return j
}

// setMatches reports whether c is in the set whose body, the part between the
// brackets, is given. A hyphen between two characters makes a range, except
// right after another range; a range that runs backwards is empty.
func setMatches(body []rune, c rune) bool {
	negate := body[0] == '!'
	if negate {
		body = body[1:]
	}
	matched := false
	for i := 0; i < len(body); i++ {
		lo, hi := body[i], body[i]
		if i+2 < len(body) && body[i+1] == '-' {
			hi = body[i+2]
			i += 2
		}
		if lo <= c && c <= hi {
			matched = true
		}
	}
	return matched != negate
}
