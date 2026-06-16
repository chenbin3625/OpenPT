package clientemu

import (
	"fmt"
	"regexp"
	"strings"
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
		b.WriteString(e.encodeRune(rune(ch)))
	}
	return b.String()
}

func (e URLEncoder) EncodeString(s string) string {
	var b strings.Builder
	for _, ch := range s {
		b.WriteString(e.encodeRune(ch))
	}
	return b.String()
}

func (e URLEncoder) encodeRune(ch rune) string {
	if e.pattern != nil && e.pattern.MatchString(string(ch)) {
		return string(ch)
	}
	hex := fmt.Sprintf("%%%02x", ch)
	if strings.EqualFold(e.EncodedHexCase, "upper") {
		return strings.ToUpper(hex)
	}
	return strings.ToLower(hex)
}
