package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func mailFirstCode(codes []mailCode, kind string) *mailCode {
	for i := range codes {
		if codes[i].Type == kind {
			return &codes[i]
		}
	}
	return nil
}

func TestMailCodesNumeric(t *testing.T) {
	cases := []struct {
		name          string
		subject, text string
		value         string
		confidence    float64
		maxConfidence float64
		none          bool
	}{
		{name: "六位验证码加关键词", subject: "验证码: 482910", value: "482910", confidence: 1},
		{name: "四位验证码", subject: "Your code is 7391", value: "7391"},
		{name: "八位验证码", subject: "OTP: 12345678", value: "12345678"},
		{name: "过滤年份", subject: "Copyright 2024", none: true},
		{name: "过滤电话上下文", text: "Phone: 123456", none: true},
		{name: "过滤金额上下文", text: "Total amount: $123456", none: true},
		{name: "过滤邮编上下文", text: "zip code 94025", none: true},
		{name: "无关键词低置信度", subject: "Hello 482910 world", value: "482910", maxConfidence: 0.5},
		{name: "冒号加分", subject: "verification code: 482910", value: "482910", confidence: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			codes := extractMailCodes(tc.subject, tc.text, "")
			if tc.none {
				if len(codes) != 0 {
					t.Fatalf("不应提取到验证码: %+v", codes)
				}
				return
			}
			code := mailFirstCode(codes, "numeric")
			if code == nil {
				t.Fatalf("没有数字验证码: %+v", codes)
			}
			if code.Value != tc.value {
				t.Fatalf("值错误: %q want %q", code.Value, tc.value)
			}
			if tc.confidence > 0 && code.Confidence != tc.confidence {
				t.Fatalf("置信度错误: %v want %v", code.Confidence, tc.confidence)
			}
			if tc.maxConfidence > 0 && code.Confidence >= tc.maxConfidence {
				t.Fatalf("置信度应低于 %v: %v", tc.maxConfidence, code.Confidence)
			}
		})
	}
}

func TestMailCodesAlphanumeric(t *testing.T) {
	codes := extractMailCodes("Your verification code is Ab3X9z", "", "")
	code := mailFirstCode(codes, "alphanumeric")
	if code == nil || code.Value != "Ab3X9z" {
		t.Fatalf("混合字母数字验证码未提取: %+v", codes)
	}

	if code := mailFirstCode(extractMailCodes("verification code is ABCDEF", "", ""), "alphanumeric"); code != nil {
		t.Fatalf("纯字母不应提取: %+v", code)
	}
	if code := mailFirstCode(extractMailCodes("Hello Ab3X9z world", "", ""), "alphanumeric"); code != nil {
		t.Fatalf("无关键词不应提取: %+v", code)
	}
}

func TestMailCodesLinks(t *testing.T) {
	cases := []struct {
		name   string
		html   string
		value  string
		min    float64
		absent bool
	}{
		{name: "验证链接", html: `<a href="https://example.com/verify?token=abc123">Verify</a>`, value: "verify", min: 0.85},
		{name: "确认链接", html: `<a href="https://example.com/confirm/abc">Confirm</a>`, value: "confirm"},
		{name: "跳过退订链接", html: `<a href="https://example.com/unsubscribe?id=123">Unsubscribe</a>`, absent: true},
		{name: "跳过非 HTTP 链接", html: `<a href="mailto:test@example.com">Email</a>`, absent: true},
		{name: "token 参数加分", html: `<a href="https://example.com/verify?token=abc">Click</a>`, min: 0.9},
		{name: "解码 HTML 实体", html: `<a href="https://replit.com/action-code?mode=verifyEmail&amp;oobCode=abc123&amp;apiKey=key&amp;lang=en">Verify</a>`, value: "https://replit.com/action-code?mode=verifyEmail&oobCode=abc123&apiKey=key&lang=en"},
		{name: "拒绝伪 HTTP 协议", html: `<a href="httpjavascript:verify?token=abc">Verify</a>`, absent: true},
		{name: "拒绝没有主机的 URL", html: `<a href="https:///verify?token=abc">Verify</a>`, absent: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := mailFirstCode(extractMailCodes("", "", tc.html), "link")
			if tc.absent {
				if code != nil {
					t.Fatalf("不应提取链接: %+v", code)
				}
				return
			}
			if code == nil {
				t.Fatal("没有提取到链接")
			}
			if tc.value != "" && !strings.Contains(code.Value, tc.value) {
				t.Fatalf("链接值错误: %q", code.Value)
			}
			if tc.min > 0 && code.Confidence < tc.min {
				t.Fatalf("置信度过低: %v", code.Confidence)
			}
		})
	}
}

func TestMailCodesHTMLHandling(t *testing.T) {
	encoded := extractMailCodes("", "", `<p>Your code: &#52;&#56;&#50;&#57;&#49;&#48;</p>`)
	if len(encoded) != 1 || encoded[0].Value != "482910" {
		t.Fatalf("HTML 编码的验证码丢失: %+v", encoded)
	}
	codes := extractMailCodes("", "", `<div>Your verification code is <b>394821</b></div>`)
	if len(codes) == 0 || codes[0].Value != "394821" || codes[0].Type != "numeric" {
		t.Fatalf("HTML 去标签后提取失败: %+v", codes)
	}

	codes = extractMailCodes("", "", `<style>.code{color:red}</style><script>var x=123456</script><p>verification code: 789012</p>`)
	if len(codes) != 1 || codes[0].Value != "789012" {
		t.Fatalf("脚本与样式未剥离: %+v", codes)
	}
}

func TestMailCodesSortAndDedupe(t *testing.T) {
	codes := extractMailCodes("verification code: 123456", "random number 987654", "")
	if len(codes) < 2 {
		t.Fatalf("应提取多个候选: %+v", codes)
	}
	for i := 1; i < len(codes); i++ {
		if codes[i-1].Confidence < codes[i].Confidence {
			t.Fatalf("未按置信度降序: %+v", codes)
		}
	}

	codes = extractMailCodes("code: 123456", "Your verification code is 123456", "")
	count := 0
	for _, code := range codes {
		if code.Value == "123456" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("同值未去重: %+v", codes)
	}
}

func TestMailCodesCombinedAndEmpty(t *testing.T) {
	codes := extractMailCodes("Your OTP is 482910", "", `<p>Code: 482910</p><a href="https://example.com/verify?token=abc">Verify</a>`)
	types := map[string]bool{}
	for _, code := range codes {
		types[code.Type] = true
	}
	if !types["numeric"] || !types["link"] {
		t.Fatalf("数字验证码与链接应同时提取: %+v", codes)
	}

	if codes := extractMailCodes("Welcome to our service", "Thank you for signing up.", ""); len(codes) != 0 {
		t.Fatalf("无验证码时应返回空: %+v", codes)
	}
}

func TestMailCodesContextAndClip(t *testing.T) {
	codes := extractMailCodes("您的验证码是 482910，请在 10 分钟内输入，请勿泄露给他人。", "", "")
	if len(codes) == 0 {
		t.Fatal("中文上下文提取失败")
	}
	if !strings.Contains(codes[0].Context, "验证码") {
		t.Fatalf("上下文未包含关键词: %q", codes[0].Context)
	}
	if count := utf8.RuneCountInString(codes[0].Context); count > 80 {
		t.Fatalf("上下文过长: %d", count)
	}
}
