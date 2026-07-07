package clientemu

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type URLEncoder struct {
	EncodingExclusionPattern string `json:"encodingExclusionPattern"`
	EncodedHexCase           string `json:"encodedHexCase"`
	pattern                  *regexp.Regexp
}

func (e *URLEncoder) compile() error {
	re, err := regexp.Compile("^(?:" + e.EncodingExclusionPattern + ")$")
	if err != nil {
		return err
	}
	e.pattern = re
	return nil
}

func (e URLEncoder) EncodeBytes(data []byte) string {
	var b strings.Builder
	for _, ch := range data {
		b.WriteString(e.encodeByte(ch))
	}
	return b.String()
}

func (e URLEncoder) EncodeString(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		ch, size := utf8.DecodeRuneInString(s)
		if ch == utf8.RuneError && size == 1 {
			b.WriteString(e.encodeByte(s[0]))
			s = s[1:]
			continue
		}
		if ch <= 0xff {
			b.WriteString(e.encodeByte(byte(ch)))
		} else {
			buf := make([]byte, utf8.RuneLen(ch))
			utf8.EncodeRune(buf, ch)
			b.WriteString(e.EncodeBytes(buf))
		}
		s = s[size:]
	}
	return b.String()
}

func (e URLEncoder) encodeByte(ch byte) string {
	if e.pattern != nil && e.pattern.MatchString(string(rune(ch))) {
		return string(rune(ch))
	}
	hex := fmt.Sprintf("%%%02x", ch)
	if strings.EqualFold(e.EncodedHexCase, "upper") {
		return strings.ToUpper(hex)
	}
	return strings.ToLower(hex)
}
