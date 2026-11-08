package identity

import (
	"regexp"
	"strings"
)

const smsRedaction = "[短信验证码已略]"

// Inbox examples are not a supported-target catalog and must not send codes to the analysis API.
var smsCode = regexp.MustCompile(`(?i)(?:verification code(?: is)?|security code(?: is)?|login code(?: is)?|your code(?: is)?|code is|use code|please enter|\bcode\s*[:：]|код подтверждения|ваш код|验证码(?:为|是)?|校验码(?:为|是)?|动态码(?:为|是)?)[\s:：=【】\[\]()'"-]{0,8}[0-9]{4,8}\b|\b[0-9]{4,8}[\s-]{0,4}(?:is your|是您的验证码)`)

func messageEvidence(text string) bool {
	return strings.Contains(text, smsRedaction) || smsCode.MatchString(text)
}
