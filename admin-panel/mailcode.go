package main

// 从邮件标题/正文/HTML 中提取验证码与验证链接，参考 mail-hub 的 code-extractor。

import (
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type mailCode struct {
	Type       string  `json:"type"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Context    string  `json:"context"`
}

var (
	mailCodeKeywordRE = regexp.MustCompile(`(?i)验证码|verification|verify|code|otp|pin|passcode|one.?time|确认码|安全码|动态码|captcha`)
	mailLinkKeywordRE = regexp.MustCompile(`(?i)verify|confirm|activate|token|click|validate|magic|login|auth|reset|unsubscribe`)

	mailNumericRE = regexp.MustCompile(`\b(\d{4,8})\b`)
	mailAlnumRE   = regexp.MustCompile(`\b([A-Za-z0-9]{4,10})\b`)
	mailHrefRE    = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)

	mailYearRE        = regexp.MustCompile(`^(202[0-9]|203[0-9])$`)
	mailPureDigitsRE  = regexp.MustCompile(`^\d+$`)
	mailPureLettersRE = regexp.MustCompile(`^[A-Za-z]+$`)
	mailColonRE       = regexp.MustCompile(`[:：]\s*\d`)
	mailZipContextRE  = regexp.MustCompile(`(?i)zip|postal|邮编`)
	mailPhoneRE       = regexp.MustCompile(`(?i)phone|电话|手机|tel|fax`)
	mailMoneyRE       = regexp.MustCompile(`(?i)\$|¥|€|£|价格|金额|amount|price|total|fee`)
	mailUnsubscribeRE = regexp.MustCompile(`(?i)unsubscribe`)
	mailVerifyRE      = regexp.MustCompile(`(?i)verify|confirm|activate`)
	mailMagicRE       = regexp.MustCompile(`(?i)magic|login.*token|auth.*token`)
	mailParamRE       = regexp.MustCompile(`(?i)token=|code=|key=`)

	mailStyleRE  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	mailScriptRE = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	mailTagRE    = regexp.MustCompile(`<[^>]+>`)
	mailSpaceRE  = regexp.MustCompile(`\s+`)
)

func mailStripHTML(html string) string {
	text := mailStyleRE.ReplaceAllString(html, "")
	text = mailScriptRE.ReplaceAllString(text, "")
	text = mailTagRE.ReplaceAllString(text, " ")
	text = mailDecodeEntities(text)
	return strings.TrimSpace(mailSpaceRE.ReplaceAllString(text, " "))
}

func mailDecodeEntities(value string) string {
	return html.UnescapeString(value)
}

// mailCodeContext 按字符而不是字节取上下文，中文邮件里的关键词才不会因为 UTF-8 变宽被挤出窗口。
func mailCodeContext(text string, bytePos, radius int) string {
	pos := utf8.RuneCountInString(text[:bytePos])
	runes := []rune(text)
	start := pos - radius
	if start < 0 {
		start = 0
	}
	end := pos + radius
	if end > len(runes) {
		end = len(runes)
	}
	return strings.TrimSpace(string(runes[start:end]))
}

func mailNumericCodes(text string) []mailCode {
	var codes []mailCode
	for _, match := range mailNumericRE.FindAllStringSubmatchIndex(text, -1) {
		value := text[match[2]:match[3]]
		context := mailCodeContext(text, match[0], 40)
		if mailYearRE.MatchString(value) {
			continue
		}
		if mailPhoneRE.MatchString(context) {
			continue
		}
		if mailMoneyRE.MatchString(context) {
			continue
		}
		if len(value) == 5 && mailZipContextRE.MatchString(context) {
			continue
		}
		confidence := 0.3
		if mailCodeKeywordRE.MatchString(context) {
			confidence = 0.9
		}
		if len(value) == 6 {
			confidence += 0.05
		}
		if mailColonRE.MatchString(context) {
			confidence += 0.05
		}
		if confidence > 1 {
			confidence = 1
		}
		codes = append(codes, mailCode{Type: "numeric", Value: value, Confidence: confidence, Context: context})
	}
	return codes
}

func mailAlphanumericCodes(text string) []mailCode {
	var codes []mailCode
	for _, match := range mailAlnumRE.FindAllStringSubmatchIndex(text, -1) {
		value := text[match[2]:match[3]]
		if mailPureDigitsRE.MatchString(value) || mailPureLettersRE.MatchString(value) {
			continue
		}
		context := mailCodeContext(text, match[0], 40)
		if !mailCodeKeywordRE.MatchString(context) {
			continue
		}
		codes = append(codes, mailCode{Type: "alphanumeric", Value: value, Confidence: 0.7, Context: context})
	}
	return codes
}

func mailLinkCodes(html string) []mailCode {
	var codes []mailCode
	for _, match := range mailHrefRE.FindAllStringSubmatchIndex(html, -1) {
		link := mailDecodeEntities(html[match[2]:match[3]])
		parsed, err := url.Parse(link)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			continue
		}
		if !mailLinkKeywordRE.MatchString(link) {
			continue
		}
		if mailUnsubscribeRE.MatchString(link) {
			continue
		}
		confidence := 0.6
		if mailVerifyRE.MatchString(link) {
			confidence = 0.85
		}
		if mailMagicRE.MatchString(link) {
			confidence = 0.8
		}
		if mailParamRE.MatchString(link) {
			confidence += 0.1
		}
		if confidence > 1 {
			confidence = 1
		}
		codes = append(codes, mailCode{Type: "link", Value: link, Confidence: confidence, Context: mailClip(link, 80)})
	}
	return codes
}

func extractMailCodes(subject, text, html string) []mailCode {
	plain := text
	if plain == "" && html != "" {
		plain = mailStripHTML(html)
	}
	full := subject + " " + plain

	codes := append(mailNumericCodes(full), mailAlphanumericCodes(full)...)
	if html != "" {
		codes = append(codes, mailLinkCodes(html)...)
	}

	sort.SliceStable(codes, func(i, j int) bool { return codes[i].Confidence > codes[j].Confidence })

	seen := make(map[string]bool, len(codes))
	unique := codes[:0]
	for _, code := range codes {
		if seen[code.Value] {
			continue
		}
		seen[code.Value] = true
		unique = append(unique, code)
	}
	return unique
}
