package clientemu

import (
	"fmt"
	"strconv"
	"strings"
)

type regexAlgorithm struct {
	pattern string
	tokens  []regexToken
}

type regexToken struct {
	choices []rune
	literal []rune
	repeat  int
}

func newRegexAlgorithm(pattern string) (Algorithm, error) {
	if pattern == "" {
		return nil, fmt.Errorf("regex pattern is required")
	}
	tokens, err := parseRegexPattern(pattern)
	if err != nil {
		return nil, err
	}
	return regexAlgorithm{pattern: pattern, tokens: tokens}, nil
}

func (a regexAlgorithm) Generate() string {
	var b strings.Builder
	for _, t := range a.tokens {
		repeat := t.repeat
		if repeat == 0 {
			repeat = 1
		}
		for i := 0; i < repeat; i++ {
			if len(t.choices) > 0 {
				b.WriteRune(t.choices[randInt(len(t.choices))])
			} else {
				b.WriteString(string(t.literal))
			}
		}
	}
	return b.String()
}

func parseRegexPattern(p string) ([]regexToken, error) {
	var out []regexToken
	r := []rune(p)
	for i := 0; i < len(r); {
		switch r[i] {
		case '[':
			j := i + 1
			escaped := false
			for ; j < len(r); j++ {
				if !escaped && r[j] == ']' {
					break
				}
				escaped = !escaped && r[j] == '\\'
				if r[j] != '\\' {
					escaped = false
				}
			}
			if j >= len(r) {
				return nil, fmt.Errorf("unterminated character class in %q", p)
			}
			choices, err := parseClass(r[i+1 : j])
			if err != nil {
				return nil, err
			}
			i = j + 1
			rep, ni := parseRepeat(r, i)
			i = ni
			out = append(out, regexToken{choices: choices, repeat: rep})
		case '(':
			j := i + 1
			for ; j < len(r) && r[j] != ')'; j++ {
			}
			if j >= len(r) {
				return nil, fmt.Errorf("unterminated group in %q", p)
			}
			lit := unescapeRunes(r[i+1 : j])
			i = j + 1
			rep, ni := parseRepeat(r, i)
			i = ni
			out = append(out, regexToken{literal: lit, repeat: rep})
		case '\\':
			if i+1 >= len(r) {
				return nil, fmt.Errorf("dangling escape in %q", p)
			}
			out = append(out, regexToken{literal: []rune{r[i+1]}, repeat: 1})
			i += 2
		default:
			out = append(out, regexToken{literal: []rune{r[i]}, repeat: 1})
			i++
		}
	}
	return out, nil
}

func parseRepeat(r []rune, i int) (int, int) {
	if i >= len(r) || r[i] != '{' {
		return 1, i
	}
	j := i + 1
	for ; j < len(r) && r[j] != '}'; j++ {
	}
	if j >= len(r) {
		return 1, i
	}
	n, err := strconv.Atoi(string(r[i+1 : j]))
	if err != nil || n < 1 {
		return 1, j + 1
	}
	return n, j + 1
}

func parseClass(r []rune) ([]rune, error) {
	var choices []rune
	for i := 0; i < len(r); {
		start, ni := readClassRune(r, i)
		if ni < len(r)-1 && r[ni] == '-' {
			end, endI := readClassRune(r, ni+1)
			if end < start {
				return nil, fmt.Errorf("invalid range %q-%q", string(start), string(end))
			}
			for ch := start; ch <= end; ch++ {
				choices = append(choices, ch)
			}
			i = endI
			continue
		}
		choices = append(choices, start)
		i = ni
	}
	return choices, nil
}

func readClassRune(r []rune, i int) (rune, int) {
	if r[i] == '\\' && i+1 < len(r) {
		return r[i+1], i + 2
	}
	return r[i], i + 1
}

func unescapeRunes(r []rune) []rune {
	out := make([]rune, 0, len(r))
	for i := 0; i < len(r); i++ {
		if r[i] == '\\' && i+1 < len(r) {
			i++
		}
		out = append(out, r[i])
	}
	return out
}
