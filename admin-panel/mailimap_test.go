package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// imapTestMessage 是假 IMAP 服务器上的一封邮件；header 为完整邮件头，body 为 MIME 正文。
type imapTestMessage struct {
	uid    uint32
	header string
	body   string
}

type imapTestServer struct {
	t           *testing.T
	ln          net.Listener
	mu          sync.Mutex
	commands    []string
	dialed      []string
	messages    []imapTestMessage
	authFail    bool
	textFetches int
}

func newImapTestServer(t *testing.T, messages ...imapTestMessage) *imapTestServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听假 IMAP 服务器失败：%v", err)
	}
	s := &imapTestServer{t: t, ln: ln}
	s.setMessages(messages...)
	go s.serve()
	t.Cleanup(func() { ln.Close() })
	return s
}

// imapTestCRLF 让测试用例能用普通换行书写协议内容。
func imapTestCRLF(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\n", "\r\n")
}

func (s *imapTestServer) channel() *mailImapChannel {
	c := newMailImapChannel()
	c.dial = func(addr string) (net.Conn, error) {
		s.mu.Lock()
		s.dialed = append(s.dialed, addr)
		s.mu.Unlock()
		return net.Dial("tcp", s.ln.Addr().String())
	}
	return c
}

func (s *imapTestServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *imapTestServer) setMessages(messages ...imapTestMessage) {
	for i := range messages {
		messages[i].header = imapTestCRLF(messages[i].header)
		messages[i].body = imapTestCRLF(messages[i].body)
	}
	s.mu.Lock()
	s.messages = messages
	s.mu.Unlock()
}

func (s *imapTestServer) snapshot() []imapTestMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]imapTestMessage(nil), s.messages...)
}

func (s *imapTestServer) find(uid uint32) (imapTestMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, msg := range s.messages {
		if msg.uid == uid {
			return msg, true
		}
	}
	return imapTestMessage{}, false
}

func (s *imapTestServer) commandCount(sub string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, command := range s.commands {
		if strings.Contains(command, sub) {
			count++
		}
	}
	return count
}

func (s *imapTestServer) dialedAddresses() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.dialed...)
}

func (s *imapTestServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	fmt.Fprint(w, "* OK fake IMAP ready\r\n")
	w.Flush()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		s.mu.Lock()
		s.commands = append(s.commands, line)
		authFail := s.authFail
		s.mu.Unlock()
		space := strings.IndexByte(line, ' ')
		if space < 0 {
			return
		}
		tag, command := line[:space], strings.TrimSpace(line[space+1:])
		upper := strings.ToUpper(command)
		switch {
		case strings.HasPrefix(upper, "ID "):
			fmt.Fprintf(w, "%s OK ID completed\r\n", tag)
		case strings.HasPrefix(upper, "LOGIN "):
			if authFail {
				// 故意带服务器原文，验证实现不会透给用户。
				fmt.Fprintf(w, "%s NO [AUTHENTICATIONFAILED] invalid password for user\r\n", tag)
			} else {
				fmt.Fprintf(w, "%s OK LOGIN completed\r\n", tag)
			}
		case upper == "SELECT INBOX":
			fmt.Fprintf(w, "* %d EXISTS\r\n%s OK [READ-WRITE] SELECT completed\r\n", len(s.snapshot()), tag)
		case upper == "UID FETCH * (UID)":
			messages := s.snapshot()
			if len(messages) > 0 {
				fmt.Fprintf(w, "* 1 FETCH (UID %d)\r\n", messages[len(messages)-1].uid)
			}
			fmt.Fprintf(w, "%s OK FETCH completed\r\n", tag)
		case strings.HasPrefix(upper, "UID FETCH "):
			s.handleFetch(w, tag, command)
		case upper == "LOGOUT":
			fmt.Fprint(w, "* BYE\r\n")
			fmt.Fprintf(w, "%s OK LOGOUT completed\r\n", tag)
			w.Flush()
			return
		default:
			fmt.Fprintf(w, "%s BAD unsupported\r\n", tag)
		}
		w.Flush()
	}
}

func (s *imapTestServer) handleFetch(w *bufio.Writer, tag, command string) {
	fields := strings.Fields(command)
	if len(fields) < 3 {
		fmt.Fprintf(w, "%s BAD fetch\r\n", tag)
		return
	}
	target, upper := fields[2], strings.ToUpper(command)
	switch {
	case strings.Contains(upper, "HEADER.FIELDS") && strings.HasSuffix(target, ":*"):
		from, _ := strconv.ParseUint(strings.TrimSuffix(target, ":*"), 10, 32)
		seq := 0
		for _, msg := range s.snapshot() {
			if uint64(msg.uid) < from {
				continue
			}
			seq++
			header := imapTestFilterHeader(msg.header)
			fmt.Fprintf(w, "* %d FETCH (UID %d BODY[HEADER.FIELDS (FROM SUBJECT DATE MESSAGE-ID)] {%d}\r\n%s)\r\n", seq, msg.uid, len(header), header)
		}
		fmt.Fprintf(w, "%s OK FETCH completed\r\n", tag)
	case strings.Contains(upper, "HEADER.FIELDS"):
		uid, _ := strconv.ParseUint(target, 10, 32)
		msg, ok := s.find(uint32(uid))
		if !ok {
			fmt.Fprintf(w, "%s NO no such message\r\n", tag)
			return
		}
		header := imapTestFilterHeader(msg.header)
		fmt.Fprintf(w, "* 1 FETCH (BODY[HEADER.FIELDS (FROM SUBJECT DATE MESSAGE-ID)] {%d}\r\n%s)\r\n", len(header), header)
		fmt.Fprintf(w, "%s OK FETCH completed\r\n", tag)
	case strings.Contains(upper, "BODY.PEEK[HEADER]"):
		uid, _ := strconv.ParseUint(target, 10, 32)
		msg, ok := s.find(uint32(uid))
		if !ok {
			fmt.Fprintf(w, "%s NO no such message\r\n", tag)
			return
		}
		s.mu.Lock()
		s.textFetches++
		s.mu.Unlock()
		fmt.Fprintf(w, "* 1 FETCH (BODY[HEADER] {%d}\r\n", len(msg.header))
		w.WriteString(msg.header)
		fmt.Fprintf(w, " BODY[TEXT] {%d}\r\n", len(msg.body))
		w.WriteString(msg.body)
		w.WriteString(")\r\n")
		fmt.Fprintf(w, "%s OK FETCH completed\r\n", tag)
	default:
		fmt.Fprintf(w, "%s BAD fetch\r\n", tag)
	}
}

// imapTestFilterHeader 模拟 HEADER.FIELDS：只回列出的头。
func imapTestFilterHeader(header string) string {
	fields := map[string]bool{"from": true, "subject": true, "date": true, "message-id": true}
	var out strings.Builder
	for _, line := range strings.Split(header, "\r\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		if fields[strings.ToLower(strings.TrimSpace(line[:colon]))] {
			out.WriteString(line)
			out.WriteString("\r\n")
		}
	}
	return out.String()
}

func imapTestInbox(address string) mailInboxData {
	return mailInboxData{Provider: "imap", Address: address, AuthData: map[string]string{
		"host": "imap.gmail.com", "port": "993", "username": address, "password": "app-pass",
	}}
}

func TestMailImapLifecycle(t *testing.T) {
	html := `<p>验证码：<b>482910</b></p><a href="https://example.com/verify?token=abc123">verify</a>`
	body := "--b1\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\n" +
		"Content-Transfer-Encoding: quoted-printable\n\n" +
		"=E9=AA=8C=E8=AF=81=E7=A0=81=EF=BC=9A 482910\n" +
		"--b1\n" +
		"Content-Type: text/html; charset=\"utf-8\"\n" +
		"Content-Transfer-Encoding: base64\n\n" +
		base64.StdEncoding.EncodeToString([]byte(html)) + "\n" +
		"--b1--\n"
	header41 := "From: =?UTF-8?B?5L2g5aW9?= <no-reply@example.com>\n" +
		"Subject: =?UTF-8?Q?=E9=AA=8C=E8=AF=81=E7=A0=81?=\n" +
		"Date: Mon, 02 Jan 2026 03:04:05 +0800\n" +
		"Message-ID: <m41@example.com>\n" +
		"Content-Type: multipart/alternative; boundary=\"b1\"\n"
	header42 := "From: notice@example.com\nSubject: Second notice\nMessage-ID: <m42@example.com>\n"
	fake := newImapTestServer(t,
		imapTestMessage{uid: 41, header: header41, body: body},
		imapTestMessage{uid: 42, header: header42},
	)
	c := fake.channel()

	data, err := c.Connect("User@gmail.com", "app-pass", "")
	if err != nil {
		t.Fatalf("Connect 失败：%v", err)
	}
	if data.Provider != "imap" || data.Address != "User@gmail.com" {
		t.Fatalf("接入记录异常：%+v", data)
	}
	if data.AuthData["host"] != "imap.gmail.com" || data.AuthData["port"] != "993" ||
		data.AuthData["username"] != "User@gmail.com" || data.AuthData["password"] != "app-pass" {
		t.Fatalf("AuthData 异常：%v", data.AuthData)
	}
	if got := fake.dialedAddresses(); len(got) == 0 || got[0] != "imap.gmail.com:993" {
		t.Fatalf("host 映射未生效：%v", got)
	}

	in := imapTestInbox(data.Address)
	in.AuthData = data.AuthData
	msgs, err := c.Messages(in)
	if err != nil {
		t.Fatalf("Messages 失败：%v", err)
	}
	if len(msgs) != 2 || msgs[0].ID != "42" || msgs[1].ID != "41" {
		t.Fatalf("最近邮件顺序异常：%+v", msgs)
	}
	if msgs[1].Subject != "验证码" || msgs[1].From != "你好 <no-reply@example.com>" {
		t.Fatalf("邮件头解码异常：%+v", msgs[1])
	}
	if msgs[1].ReceivedAt != "2026-01-01T19:04:05Z" {
		t.Fatalf("时间未按 UTC 输出：%q", msgs[1].ReceivedAt)
	}
	if msgs[0].ReceivedAt != "" {
		t.Fatalf("缺少 Date 时应留空：%q", msgs[0].ReceivedAt)
	}

	detail, err := c.Message(in, "41")
	if err != nil {
		t.Fatalf("Message 失败：%v", err)
	}
	if detail.ID != "41" || detail.Subject != "验证码" || detail.From != "你好 <no-reply@example.com>" {
		t.Fatalf("详情邮件头异常：%+v", detail)
	}
	if detail.Text != "验证码： 482910" {
		t.Fatalf("text/plain 解析异常：%q", detail.Text)
	}
	if detail.HTML != html {
		t.Fatalf("text/html 原文异常：%q", detail.HTML)
	}
	if detail.Excerpt != detail.Text {
		t.Fatalf("Excerpt 未取正文前 200 字：%q", detail.Excerpt)
	}
}

func TestMailImapEmptyInbox(t *testing.T) {
	c := newImapTestServer(t).channel()
	msgs, err := c.Messages(imapTestInbox("user@gmail.com"))
	if err != nil {
		t.Fatalf("空邮箱报错：%v", err)
	}
	if msgs == nil || len(msgs) != 0 {
		t.Fatalf("空邮箱应返回空切片：%#v", msgs)
	}
}

func TestMailImapAuthFailureHidesServerText(t *testing.T) {
	fake := newImapTestServer(t)
	fake.mu.Lock()
	fake.authFail = true
	fake.mu.Unlock()
	_, err := fake.channel().Connect("user@gmail.com", "wrong-pass", "")
	if err == nil {
		t.Fatal("认证失败仍返回成功")
	}
	text := err.Error()
	if !strings.Contains(text, "登录失败") {
		t.Fatalf("错误文案不是中文提示：%q", text)
	}
	for _, leaked := range []string{"AUTHENTICATIONFAILED", "invalid password", "wrong-pass", "user@gmail.com"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("错误泄露了服务器原文或凭据：%q", text)
		}
	}
}

func TestMailImapHostValidation(t *testing.T) {
	for _, host := range []string{
		"127.0.0.1", "10.0.0.1", "localhost", "imap.gmail.com:993",
		"imap.gmail.com/path", "::1", "imap gmail.com", "-bad.example.com", "imap..gmail.com",
	} {
		host := host
		t.Run(host, func(t *testing.T) {
			fake := newImapTestServer(t)
			_, err := fake.channel().Connect("user@gmail.com", "app-pass", host)
			if err == nil {
				t.Fatalf("host %q 应被拒绝", host)
			}
			if !strings.Contains(err.Error(), "IMAP 服务器地址无效") {
				t.Fatalf("错误文案异常：%q", err.Error())
			}
			if dialed := fake.dialedAddresses(); len(dialed) != 0 {
				t.Fatalf("非法 host 仍发起了连接：%v", dialed)
			}
		})
	}
}

func TestMailImapHostMapping(t *testing.T) {
	cases := map[string]string{
		"gmail.com":      "imap.gmail.com",
		"googlemail.com": "imap.gmail.com",
		"outlook.com":    "outlook.office365.com",
		"hotmail.com":    "outlook.office365.com",
		"live.com":       "outlook.office365.com",
		"msn.com":        "outlook.office365.com",
		"qq.com":         "imap.qq.com",
		"foxmail.com":    "imap.qq.com",
		"163.com":        "imap.163.com",
		"126.com":        "imap.126.com",
		"yeah.net":       "imap.yeah.net",
		"icloud.com":     "imap.mail.me.com",
		"me.com":         "imap.mail.me.com",
		"mac.com":        "imap.mail.me.com",
		"yahoo.com":      "imap.mail.yahoo.com",
		"example.test":   "",
	}
	for domain, want := range cases {
		if got := mailImapHostFor(domain); got != want {
			t.Errorf("域名 %s 映射为 %q，期望 %q", domain, got, want)
		}
	}
	if got := mailImapHostFor("Gmail.com"); got != "imap.gmail.com" {
		t.Errorf("域名映射应大小写不敏感：%q", got)
	}
}

func TestMailImapBodyCache(t *testing.T) {
	header := "From: a@b.test\nSubject: Cached\nContent-Type: text/plain; charset=utf-8\n"
	fake := newImapTestServer(t, imapTestMessage{uid: 7, header: header, body: "first body 111111\n"})
	c := fake.channel()
	in := imapTestInbox("user@gmail.com")

	first, err := c.Message(in, "7")
	if err != nil {
		t.Fatalf("首次取正文失败：%v", err)
	}
	if !strings.Contains(first.Text, "111111") {
		t.Fatalf("首次正文异常：%q", first.Text)
	}
	fake.setMessages(imapTestMessage{uid: 7, header: header, body: "second body 222222\n"})
	fake.mu.Lock()
	before := len(fake.commands)
	fake.mu.Unlock()
	second, err := c.Message(in, "7")
	if err != nil {
		t.Fatalf("缓存命中取正文失败：%v", err)
	}
	if second.Text != first.Text || strings.Contains(second.Text, "222222") {
		t.Fatalf("正文缓存未命中：%q", second.Text)
	}
	if count := fake.commandCount("BODY.PEEK[TEXT]"); count != 1 {
		t.Fatalf("60 秒内重复拉正文 %d 次", count)
	}
	fake.mu.Lock()
	after := len(fake.commands)
	fake.mu.Unlock()
	if after != before {
		t.Fatalf("缓存命中仍新建了 IMAP 连接（多出 %d 条命令）", after-before)
	}
}

func TestMailImapLoginQuotesCredentials(t *testing.T) {
	fake := newImapTestServer(t)
	if _, err := fake.channel().Connect("user@example.com", `pa"ss\word`, "imap.example.com"); err != nil {
		t.Fatalf("Connect 失败：%v", err)
	}
	fake.mu.Lock()
	commands := append([]string(nil), fake.commands...)
	fake.mu.Unlock()
	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, `LOGIN "user@example.com" "pa\"ss\\word"`) {
		t.Fatalf("LOGIN 未转义引号或反斜杠：\n%s", joined)
	}
	if len(commands) < 3 || !strings.HasPrefix(commands[0], "A001 ID ") ||
		!strings.HasPrefix(commands[1], "A002 LOGIN ") || commands[2] != "A003 SELECT INBOX" {
		t.Fatalf("命令顺序异常：%v", commands)
	}
}