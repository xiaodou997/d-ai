package riskcontrol

import (
	"strings"

	"github.com/mozillazg/go-pinyin"
)

// textToPinyin converts Chinese characters in s to their pinyin
// representation (without tone marks), concatenated into a single
// lowercase string. Non-Chinese characters (Latin letters, digits,
// punctuation) are preserved as-is.
//
// This is used both at keyword compile time (to pre-convert pinyin
// library entries like "微信" → "weixin") and at match time (to convert
// user input like "买葳信" → "maiwexin" so it hits the "weixin" pattern).
func textToPinyin(s string) string {
	if s == "" {
		return ""
	}
	a := pinyin.NewArgs()
	a.Style = pinyin.Normal

	var sb strings.Builder
	for _, r := range s {
		result := pinyin.Pinyin(string(r), a)
		if len(result) > 0 && len(result[0]) > 0 {
			sb.WriteString(result[0][0])
		} else {
			// Non-Chinese character (Latin, digit, punctuation, etc.)
			// — preserve as-is so mixed text like "买abc" → "maiabc".
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
