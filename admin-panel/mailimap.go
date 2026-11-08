package main

// 自定义邮箱（IMAP）：用户已有的地址 + 应用专用密码直接收件。
// 永不删除上游邮件；只支持 implicit TLS 993，且服务器只接受主机名——
// 浏览器传来的 host 不能变成任意 TCP 探测。

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	mailImapTimeout    = 20 * time.Second
	mailImapBodyTTL    = time.Minute
	mailImapBodyMax    = 50
	mailImapListSize   = 20
	mailImapMimeDepth  = 5
	mailImapMaxLiteral = 2 << 20
)

// 统一的中文文案：绝不透出服务器原文（可能回显账号或内部信息）。
var (
	errMailImapConnect = errors.New("无法连接 IMAP 服务器")
	errMailImapLogin   = errors.New("自定义邮箱登录失败，请检查地址与应用专用密码")
	errMailImapSelect  = errors.New("无法打开邮箱收件箱")
	errMailImapList    = errors.New("无法读取邮箱邮件列表")
	errMailImapMessage = errors.New("无法读取邮件内容")
)

// 内置服务商映射，域名小写精确匹配；其余域名必须由用户填 host。
var mailImapHosts = map[string]string{
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
}

// 末段必须是字母开头的域名：同时排除 IP 字面量、localhost、端口与路径。
var mailImapHostRE = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

var mailImapUIDRE = regexp.MustCompile(`\bUID (\d{1,10})\b`)

func mailImapHostFor(domain string) string { return mailImapHosts[strings.ToLower(domain)] }

type mailImapBody struct {
	msg mailMessage
	at  time.Time
}

type mailImapChannel struct {
	dial func(addr string) (net.Conn, error) // 测试注入明文假服务器
	mu   sync.Mutex
	seen map[string]mailImapBody
}

func newMailImapChannel() *mailImapChannel {
	c := &mailImapChannel{seen: map[string]mailImapBody{}}
	c.dial = func(addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		dialer := &net.Dialer{Timeout: mailImapTimeout}
		return tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host})
	}
	return c
}

func (c *mailImapChannel) Meta() mailProviderMeta {
	return mailProviderMeta{
		Name:        "imap",
		DisplayName: "自定义邮箱（IMAP）",
		Tier:        "user",
		Retention:   "由你的邮箱决定",
		Features:    mailFeatures{PollInbox: true},
	}
}

func (c *mailImapChannel) AttachDomains() []string { return nil }

func (c *mailImapChannel) PollInterval() time.Duration { return 10 * time.Second }

func (c *mailImapChannel) Create(string, string) (mailInboxData, error) {
	return mailInboxData{}, mailInputError("自定义邮箱只能按已有邮箱地址接入，请填写邮箱地址与应用专用密码")
}

// Close 只是本地关闭：上游邮件不能被删。
func (c *mailImapChannel) Close(mailInboxData) error { return nil }

func (c *mailImapChannel) Connect(address, password, host string) (mailInboxData, error) {
	address = strings.TrimSpace(address)
	at := strings.LastIndexByte(address, '@')
	if at <= 0 || at == len(address)-1 || len(address) > 254 {
		return mailInboxData{}, mailInputError("请填写完整邮箱地址，例如 name@gmail.com")
	}
	local, domain := address[:at], strings.ToLower(address[at+1:])
	if !mailUsernameRE.MatchString(local) || !mailDomainRE.MatchString(domain) {
		return mailInboxData{}, mailInputError("邮箱地址格式无效")
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		if host = mailImapHostFor(domain); host == "" {
			return mailInboxData{}, mailInputError("该邮箱域名没有内置 IMAP 服务器，请填写服务器地址")
		}
	} else if len(host) > 253 || !mailImapHostRE.MatchString(host) {
		return mailInboxData{}, mailInputError("IMAP 服务器地址无效，请只填写主机名，例如 imap.gmail.com")
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return mailInboxData{}, mailInputError("请填写应用专用密码或授权码")
	}
	username := local + "@" + domain
	s, err := c.open(host, "993")
	if err != nil {
		return mailInboxData{}, err
	}
	defer s.close()
	if err = s.login(username, password); err != nil {
		return mailInboxData{}, err
	}
	if err = s.selectInbox(); err != nil {
		return mailInboxData{}, err
	}
	return mailInboxData{
		Provider: "imap",
		Address:  username,
		AuthData: map[string]string{"host": host, "port": "993", "username": username, "password": password},
	}, nil
}

func (c *mailImapChannel) Messages(in mailInboxData) ([]mailMessage, error) {
	s, err := c.session(in.AuthData)
	if err != nil {
		return nil, err
	}
	defer s.close()
	highest, err := s.highestUID()
	if err != nil {
		return nil, err
	}
	if highest == 0 {
		return []mailMessage{}, nil
	}
	from := uint32(1)
	if highest >= mailImapListSize {
		from = highest - mailImapListSize + 1
	}
	rep, err := s.command("UID FETCH %d:* (UID BODY.PEEK[HEADER.FIELDS (FROM SUBJECT DATE MESSAGE-ID)])", from)
	if err != nil || rep.status != "OK" {
		return nil, errMailImapList
	}
	headers := map[uint32][]byte{}
	for _, literal := range rep.literals {
		if !strings.Contains(literal.line, "BODY[HEADER.FIELDS") {
			continue
		}
		if uid, ok := mailImapUIDFromLine(literal.line); ok {
			headers[uid] = literal.data
		}
	}
	uids := make([]uint32, 0, len(headers))
	for uid := range headers {
		uids = append(uids, uid)
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	if len(uids) > mailImapListSize {
		uids = uids[:mailImapListSize]
	}
	out := make([]mailMessage, 0, len(uids))
	for _, uid := range uids {
		out = append(out, mailImapDecodeMessage(uid, headers[uid]))
	}
	return out, nil
}

func (c *mailImapChannel) Message(in mailInboxData, id string) (mailMessage, error) {
	uid, err := strconv.ParseUint(id, 10, 32)
	if err != nil || uid == 0 {
		return mailMessage{}, errors.New("邮件不存在")
	}
	// 页面每 5 秒调一次 /code：命中时直接返回，避免反复给邮箱登录。
	key := strings.ToLower(in.Address) + "\x00" + id
	if msg, ok := c.cachedBody(key); ok {
		return msg, nil
	}
	s, err := c.session(in.AuthData)
	if err != nil {
		return mailMessage{}, err
	}
	defer s.close()
	// BODY[TEXT] 不带 MIME 头，无法判断边界与传输编码；连同 HEADER 一起取，再接回整封原文。
	rep, err := s.command("UID FETCH %d (BODY.PEEK[HEADER] BODY.PEEK[TEXT])", uid)
	if err != nil || rep.status != "OK" {
		return mailMessage{}, errMailImapMessage
	}
	var header, text []byte
	for _, literal := range rep.literals {
		switch {
		case strings.Contains(literal.line, "BODY[HEADER] {"):
			header = literal.data
		case strings.Contains(literal.line, "BODY[TEXT] {"):
			text = literal.data
		}
	}
	if header == nil || text == nil {
		return mailMessage{}, errMailImapMessage
	}
	raw := make([]byte, 0, len(header)+len(text)+4)
	raw = append(raw, bytes.TrimRight(header, "\r\n")...)
	raw = append(raw, '\r', '\n', '\r', '\n')
	raw = append(raw, text...)
	msg := mailImapDecodeMessage(uint32(uid), raw)
	c.storeBody(key, msg)
	return msg, nil
}

func (c *mailImapChannel) cachedBody(key string) (mailMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.seen[key]
	if !ok {
		return mailMessage{}, false
	}
	if time.Since(cached.at) >= mailImapBodyTTL {
		delete(c.seen, key)
		return mailMessage{}, false
	}
	return cached.msg, true
}

func (c *mailImapChannel) storeBody(key string, msg mailMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil {
		c.seen = map[string]mailImapBody{}
	}
	now := time.Now()
	for k, body := range c.seen {
		if now.Sub(body.at) >= mailImapBodyTTL {
			delete(c.seen, k)
		}
	}
	if len(c.seen) >= mailImapBodyMax {
		var oldestKey string
		oldest := now
		for k, body := range c.seen {
			if body.at.Before(oldest) {
				oldestKey, oldest = k, body.at
			}
		}
		delete(c.seen, oldestKey)
	}
	c.seen[key] = mailImapBody{msg: msg, at: now}
}

func (c *mailImapChannel) session(auth map[string]string) (*mailImapSession, error) {
	if auth == nil {
		return nil, errMailImapConnect
	}
	host := strings.ToLower(strings.TrimSpace(auth["host"]))
	port := strings.TrimSpace(auth["port"])
	if port == "" {
		port = "993"
	}
	if len(host) > 253 || !mailImapHostRE.MatchString(host) {
		return nil, errMailImapConnect
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return nil, errMailImapConnect
	}
	user, password := auth["username"], auth["password"]
	if user == "" || password == "" {
		return nil, errMailImapLogin
	}
	s, err := c.open(host, port)
	if err != nil {
		return nil, err
	}
	if err = s.login(user, password); err != nil {
		s.close()
		return nil, err
	}
	if err = s.selectInbox(); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (c *mailImapChannel) open(host, port string) (*mailImapSession, error) {
	conn, err := c.dial(net.JoinHostPort(host, port))
	if err != nil {
		return nil, errMailImapConnect
	}
	if err = conn.SetDeadline(time.Now().Add(mailImapTimeout)); err != nil {
		conn.Close()
		return nil, errMailImapConnect
	}
	s := &mailImapSession{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn)}
	line, err := s.readLine()
	if err != nil || !(strings.HasPrefix(line, "* OK") || strings.HasPrefix(line, "* PREAUTH")) {
		conn.Close()
		return nil, errMailImapConnect
	}
	return s, nil
}

type mailImapSession struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
	tag  int
}

type mailImapLiteral struct {
	line string
	data []byte
}

type mailImapReply struct {
	status   string
	lines    []string // 未标记响应行，取 UID 用
	literals []mailImapLiteral
}

func (s *mailImapSession) close() {
	_, _ = s.command("LOGOUT")
	_ = s.conn.Close()
}

func (s *mailImapSession) readLine() (string, error) {
	line, err := s.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (s *mailImapSession) command(format string, args ...any) (*mailImapReply, error) {
	s.tag++
	tag := fmt.Sprintf("A%03d", s.tag)
	if _, err := fmt.Fprintf(s.w, "%s %s\r\n", tag, fmt.Sprintf(format, args...)); err != nil {
		return nil, err
	}
	if err := s.w.Flush(); err != nil {
		return nil, err
	}
	rep := &mailImapReply{}
	for {
		line, err := s.readLine()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(line, tag+" ") {
			rest := line[len(tag)+1:]
			if i := strings.IndexByte(rest, ' '); i >= 0 {
				rep.status = rest[:i]
			} else {
				rep.status = rest
			}
			return rep, nil
		}
		if n, ok := mailImapLiteralLen(line); ok {
			data, err := s.readLiteral(n)
			if err != nil {
				return nil, err
			}
			rep.literals = append(rep.literals, mailImapLiteral{line: line, data: data})
			continue
		}
		if strings.HasPrefix(line, "* ") {
			rep.lines = append(rep.lines, line)
		}
		// "+" 续行与 ")" 之类的响应余量无需处理。
	}
}

func (s *mailImapSession) readLiteral(n int) ([]byte, error) {
	if n < 0 || n > mailImapMaxLiteral {
		return nil, errMailImapMessage
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(s.r, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *mailImapSession) login(user, password string) error {
	// 凭据必须是单行：换行会变成下一条 IMAP 命令。
	if strings.ContainsAny(user+password, "\r\n\x00") {
		return errMailImapLogin
	}
	// ID 是可选扩展（Gmail / 网易要求），服务器不支持时忽略失败继续登录。
	_, _ = s.command(`ID ("name" "identity" "version" "1")`)
	rep, err := s.command("LOGIN %s %s", mailImapQuote(user), mailImapQuote(password))
	if err != nil || rep.status != "OK" {
		return errMailImapLogin
	}
	return nil
}

func (s *mailImapSession) selectInbox() error {
	rep, err := s.command("SELECT INBOX")
	if err != nil || rep.status != "OK" {
		return errMailImapSelect
	}
	return nil
}

func (s *mailImapSession) highestUID() (uint32, error) {
	rep, err := s.command("UID FETCH * (UID)")
	if err != nil || rep.status != "OK" {
		return 0, errMailImapList
	}
	var highest uint32
	for _, line := range rep.lines {
		if uid, ok := mailImapUIDFromLine(line); ok && uid > highest {
			highest = uid
		}
	}
	return highest, nil
}

func mailImapQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func mailImapUIDFromLine(line string) (uint32, bool) {
	match := mailImapUIDRE.FindStringSubmatch(line)
	if match == nil {
		return 0, false
	}
	n, err := strconv.ParseUint(match[1], 10, 32)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint32(n), true
}

// mailImapLiteralLen 识别响应行末尾的 {N} 字面量长度标记。
func mailImapLiteralLen(line string) (int, bool) {
	if !strings.HasSuffix(line, "}") {
		return 0, false
	}
	open := strings.LastIndexByte(line, '{')
	if open < 1 || line[open-1] != ' ' || open+1 >= len(line) {
		return 0, false
	}
	digits := strings.TrimSuffix(line[open+1:len(line)-1], "+")
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// mailImapDecodeMessage 用 net/mail 解析整封原文；raw 可以是只有头的列表结果。
func mailImapDecodeMessage(uid uint32, raw []byte) mailMessage {
	msg := mailMessage{ID: strconv.FormatUint(uint64(uid), 10)}
	m, err := mail.ReadMessage(io.MultiReader(bytes.NewReader(raw), strings.NewReader("\r\n\r\n")))
	if err != nil {
		return msg
	}
	decoder := &mime.WordDecoder{}
	msg.Subject = mailImapDecodeHeader(decoder, m.Header.Get("Subject"))
	msg.From = mailImapDecodeHeader(decoder, m.Header.Get("From"))
	if date, err := m.Header.Date(); err == nil {
		msg.ReceivedAt = date.UTC().Format(time.RFC3339)
	}
	var text, html string
	mailImapMimeBodies(textproto.MIMEHeader(m.Header), m.Body, &text, &html, mailImapMimeDepth)
	if text == "" && html != "" {
		text = mailStripHTML(html) // 只有 HTML 时也要能提取验证码
	}
	msg.Text, msg.HTML = text, html
	msg.Excerpt = mailClip(text, 200)
	return msg
}

// mailImapDecodeHeader 只解 encoded-word；不支持的字符集保持原文。
func mailImapDecodeHeader(decoder *mime.WordDecoder, value string) string {
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func mailImapMimeBodies(header textproto.MIMEHeader, body io.Reader, text, html *string, depth int) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		mediaType = ""
	}
	if strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		if depth <= 0 {
			return
		}
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				return
			}
			mailImapMimeBodies(textproto.MIMEHeader(part.Header), part, text, html, depth-1)
		}
	}
	switch strings.ToLower(mediaType) {
	case "", "text/plain":
		if *text == "" {
			*text = mailImapDecodePart(header, body)
		}
	case "text/html":
		if *html == "" {
			*html = mailImapDecodePart(header, body)
		}
	}
}

func mailImapDecodePart(header textproto.MIMEHeader, body io.Reader) string {
	reader := body
	// multipart 已代解 quoted-printable 并删掉该头，这里只处理 base64 与裸文本。
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		reader = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		reader = quotedprintable.NewReader(body)
	}
	data, err := io.ReadAll(io.LimitReader(reader, mailImapMaxLiteral))
	if err != nil && len(data) == 0 {
		return ""
	}
	return string(data)
}