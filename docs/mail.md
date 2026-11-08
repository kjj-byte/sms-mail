# 邮箱（原生）

左栏「SMS 短信 / 邮箱」切换。邮箱功能由本机管理服务 `admin-panel` 原生实现（Go 标准库），随 `start.cmd` 一起启动，不需要 Docker、Node.js、API Key 或任何环境变量。入口：<http://127.0.0.1:8787/preview/#mail>。

## 功能

- 9 个免费模板渠道，创建时可指定渠道与域名
- **接入自己已有的邮箱**：`maildrop.cc` / `catchmail.io` 这类「地址即信箱」的域名免密码直接接入；Gmail / Outlook / QQ / 163 等自定义域名用 IMAP 授权码接入
- 三栏工作区：邮箱列表、收件列表、邮件阅读；手机上纵向排列
- 创建临时邮箱、接入已有邮箱使用独立表单，不会混发字段
- 按地址 / 渠道 / 目标服务搜索，按状态筛选，按状态或渠道分组
- 恢复上次选中的邮箱；首次使用默认选中最近的有效邮箱
- 单次请求获取邮件与最新验证码，按渠道间隔轮询（至少 5 秒）；切换 SMS 或隐藏浏览器标签页时暂停
- 纯文本阅读正文，一键复制地址、验证码与验证链接；关闭前提示影响，关闭后清除本机凭据
- 已过期 / 已关闭的邮箱可以查看记录，不再请求上游收件

## 渠道与调度顺序

自动调度按下列固定顺序尝试；某个渠道不可用时切换到下一个，全部失败则报错。未指定渠道或域名的自动创建失败时，该渠道冷却 60 秒，后续自动调度与调度预览会跳过它；显式指定渠道可立即重试，成功后解除冷却。冷却状态只保存在当前进程中。创建时也可显式指定渠道，域名是否被接受由上游决定。

| 顺序 | 渠道 ID | 渠道 |
| ---: | --- | --- |
| 1 | `mailtm` | Mail.tm |
| 2 | `mailgw` | Mail.gw |
| 3 | `tempmail-lol` | TempMail.lol |
| 4 | `tempmail-ing` | TempMail.ing |
| 5 | `guerrillamail` | Guerrilla Mail |
| 6 | `maildrop` | Maildrop.cc |
| 7 | `tempmail-io` | Temp-Mail.io |
| 8 | `catchmail` | Catchmail.io |
| 9 | `nimail-cn` | NiMail.cn |

## 启动与使用

构建并启动根目录的 `start.cmd`（或在 PowerShell 执行 `.\start.cmd`），打开 <http://127.0.0.1:8787/preview/#mail>。首次创建前填写真实的目标服务域名；创建后页面自动轮询收件。不需要额外安装或填写密钥。构建与端口说明见 [admin-panel/README.md](../admin-panel/README.md)。

## 接入自己已有的邮箱

展开「新建 / 接入邮箱」，切换到「接入已有邮箱」。有两种接入方式，都不需要新增渠道：

**1. 地址即信箱（免密码）**：`maildrop.cc`、`catchmail.io`（含 `mailistry.com`、`zeppost.com`）这类渠道不注册也能读本域任意地址，填 `name@maildrop.cc` 即可接入。可接入域名由 `GET /mail-api/status` 的 `attachDomains` 返回，页面会直接列出来。

**2. 自定义域名（IMAP 授权码）**：Gmail、Outlook、QQ、163、iCloud、Yahoo 等自己的邮箱，填地址 + **应用专用密码/授权码**后，程序用 IMAP 只读收件。

| 域名 | 自动识别的 IMAP 服务器 |
| --- | --- |
| `gmail.com` / `googlemail.com` | `imap.gmail.com` |
| `outlook.com` / `hotmail.com` / `live.com` / `msn.com` | `outlook.office365.com` |
| `qq.com` / `foxmail.com` | `imap.qq.com` |
| `163.com` / `126.com` / `yeah.net` | `imap.163.com` / `imap.126.com` / `imap.yeah.net` |
| `icloud.com` / `me.com` / `mac.com` | `imap.mail.me.com` |
| `yahoo.com` | `imap.mail.yahoo.com` |

其他域名需要在「IMAP 服务器（可选）」里自己填。只支持 **implicit TLS（993）**。

### 拿授权码（不是登录密码）

- **Gmail**：Google 账号 → 安全性 → 两步验证 → 应用专用密码。没有两步验证就拿不到，普通密码也不被 IMAP 接受。
- **QQ / 163 / 126**：邮箱设置里先开启 IMAP 服务，再生成授权码。
- 授权码等同于该邮箱的收件权限，只填在页面输入框里，不要贴进聊天、文档或命令。

### 行为与边界

- **只读 INBOX，不删除、不标记已读**（`BODY.PEEK`）；关闭邮箱不会删你的邮件，只是停止轮询。
- 轮询间隔 10 秒（页面 5 秒刷新会命中缓存），每次只拉最近 20 封的头部；正文按需拉取并缓存 60 秒。
- `+` 子地址（如 `me+shop@gmail.com`）按基础账号接入，子地址的邮件本来就在同一个收件箱里。
- 还没做：IMAP STARTTLS（143）、多文件夹、附件下载、GBK 等非 UTF-8 邮件转码。
- 授权码以明文保存在 `data/mail.json`（权限 0600）；关闭邮箱时会清掉该记录的凭据。
- 地址即信箱的记录按 24 小时有效期显示（实际取决于上游保留期）；自定义 IMAP 邮箱不会过期。

## 数据与备份

收件箱记录与渠道凭据保存在 `admin-panel/data/mail.json`。该文件已忽略提交，含私有数据，请自行备份；删除后现有收件箱与凭据不再保留。不要把它复制进 `dist` 或公开仓库。

列表保留策略：已关闭记录保留 24 小时后清理，总记录上限 200 条（超出时丢弃最旧的）。

## 本地 API

仅由本机管理服务提供，公开静态站（`dist`）不提供。仍受 Host、Origin、CSRF、POST 方法与请求大小校验。

| 方法与路径 | 作用 |
| --- | --- |
| `GET /mail-api/status` | 服务状态 |
| `GET /mail-api/providers` | 渠道列表 |
| `GET /mail-api/dispatch/preview` | 自动调度预览 |
| `GET /mail-api/inboxes` | 收件箱列表，每条带 `state`（`active`/`expired`/`closed`）；`status=active` 只看有效的 |
| `POST /mail-api/inbox` | 创建收件箱（`provider` / `domain`）；带 `address` 时改为接入已有地址，自定义域名再加 `password`（授权码）与可选 `imapHost` |
| `GET /mail-api/inbox/{id}/messages` | 收件列表与 `pollAfter`（秒）；加 `includeCode=true` 同时返回 `verification` 验证码结果 |
| `GET /mail-api/inbox/{id}/messages/{mid}` | 邮件正文 |
| `GET /mail-api/inbox/{id}/code` | 提取验证码 |
| `DELETE /mail-api/inbox/{id}` | 关闭收件箱 |

`GET /mail-api/status` 返回 `{"csrf":"…","attachDomains":["catchmail.io","maildrop.cc","mailistry.com","zeppost.com"]}`，`attachDomains` 即可直接接入的域名，可能为空。

## 行为边界与限制

- 上游保留期与限流由各渠道决定，本机不延长也不保证。
- 页面按 `pollAfter` 轮询（最少 5 秒）；离开邮箱、隐藏标签页、过期或关闭后停止。上游失败时暂停自动收件，可手动重试。
- 收件列表请求按邮箱串行合并并缓存；最新邮件 ID 未变化时复用验证码结果，正文读取失败会提示并允许下次重试。
- 过期邮箱的邮件与验证码接口返回 HTTP 410；仍可关闭以释放资源。已有 `/code` 接口与原有 JSON 记录格式保留。
- 临时邮箱可能公开可读，勿用于接收敏感邮件。
- 渠道可用性随时可能变化；不可用时自动切换到下一渠道或报错。
- 仅限本机单管理员使用，管理端没有远程身份认证，不能改为公网监听；公开托管只上传 `dist`。

## 未移植项

原服务的以下能力没有移植，原因如下：

- Outlook / YYDS Mail / IMAP 域名邮箱 / iCloud 别名的**账号池管理**（批量导入、按租期分配、1:1 借出回收、自动补货）：原生版只支持把**已有邮箱**按地址 + 授权码接入单账号，不做池化分配。iCloud 隐藏地址的签发也不做。
- API Key 分级与配额、封禁表与自动封禁、统计：原服务面向多用户实例；原生版只有本机单管理员，也不再有「上报成功 / 失败」按钮，因为没有统计后台。
- 自定义渠道编辑器：不支持自己新增 API 模板渠道；只能在「地址即信箱」的内置渠道上直接接入已有地址（见上）。
- 备份调度、更新检查：由用户自行备份文件和更新程序。
- SQLite 数据库：原生版改用本地 JSON 文件。
- Dropmail.me：需要用户自己的 token。
- 别名（`+` 子地址）不当作独立邮箱：填 `me+tag@` 时按基础账号 `me@` 接入，邮件本来就在同一个收件箱。

## 来源与许可

行为与渠道定义参考 <https://github.com/ydddp/mail-hub>（AGPL-3.0-or-later），本项目用 Go 重写。公开分发本项目须遵守 AGPL-3.0-or-later 的相应义务。

## 验证

### 2026-11-08 工作台重设计

本轮核对了 mail-hub 的 [dispatcher](https://github.com/ydddp/mail-hub/blob/master/src/dispatcher.ts)、[收件接口](https://github.com/ydddp/mail-hub/blob/master/src/routes/inbox.ts) 与前端结构，重点采用失败冷却、生命周期约束与清晰的收件流程；继续使用本项目已有的 Go / JSON 结构，不增加服务或依赖。

原实现的问题与本轮改动：

- 创建与已有邮箱字段混用 → 两种模式分别提交，成功接入后清空授权码输入框。
- 重开页面失去选择 → 仅在浏览器保存所选邮箱 ID，重新连接时恢复，不在浏览器存凭据。
- 邮件与验证码并发拉取导致重复请求 → 工作台使用合并接口，服务端按邮箱合并并发轮询，最新邮件 ID 不变时复用验证码结果；原 `/code` 接口保留。
- 过期仍然取件、后台继续轮询 → 前后端同时处理过期状态；隐藏标签页、切换服务、关闭后停止自动请求。
- 正文读取失败被当成没有验证码 → 提示正文失败，保留摘要候选，并允许下次重试。
- HTML 实体里的数字丢失 → 用标准库解码实体；验证链接校验 HTTP(S) 协议和主机。HTML 邮件在服务端转换成文本供页面阅读，不插入邮件 HTML。
- 邮件列表和正文混排 → 三栏工作区；手机纵向排列，选择邮箱或邮件后滚动到对应内容。

验收：两个 Go module 的常规测试与 `vet` 通过；前端 29 项测试通过；浏览器检查了 1440px 桌面三栏与 390px 手机布局、正文读取与验证码展示，没有横向溢出。浏览器有邮件场景使用隔离的模拟数据，不改动用户邮箱记录。

本轮联网测试：7 / 9 个渠道完成创建 → 合并收件 → 验证码接口 → 关闭。通过：Mail.tm、TempMail.ing、Guerrilla Mail、Maildrop.cc、Temp-Mail.io、Catchmail.io、NiMail.cn；失败：Mail.gw 无可用域名（域名请求 15 秒超时），TempMail.lol 返回 HTTP 403。这是本次测试时的上游状态，不代表长期可用性。测试邮箱使用临时存储并自动关闭。

未验证真实私人邮箱登录或第三方验证码送达；`-race` 因当前 Go 环境未启用 CGO 无法执行，新增并发逻辑有普通并发测试覆盖。

```powershell
$env:GOCACHE = 'D:\sms\.cache\go-build'
go test ./...
go -C admin-panel test ./...
node --test web/app.test.cjs web/mail.test.cjs
# 可选：联网冒烟测试，会在各渠道真实创建匿名收件箱后立刻关闭
go -C admin-panel test -tags live -run TestLiveMail -v
```

测试覆盖收件箱生命周期、CSRF / 同源 / Host / 请求大小拦截、轮询节流与前端轮询，以及 IMAP 会话（脚本化假服务器：encoded-word 主题、multipart 正文、认证失败、host 校验、缓存）；渠道真实可用性、真实邮箱登录与邮件送达取决于上游，测试不保证。

2026-11-08 验收：两个 Go module 的测试、vet，以及前端 19 项测试通过。联网冒烟测试按渠道逐个创建→取件→关闭：9 个渠道中 8 个可用（Mail.tm、TempMail.lol、TempMail.ing、Guerrilla Mail、Maildrop.cc、Temp-Mail.io、Catchmail.io、NiMail.cn）；Mail.gw 当时其域名接口返回 HTTP 521，属上游停机，自动调度会跳过它。没有邮件注入接口，未验证真实验证码送达；验证码提取用单元测试覆盖。

同日追加「直接接入 + 分类列出」：联网测试对 `catchmail.io`、`maildrop.cc`、`mailistry.com`、`zeppost.com` 四个域名各接入一次并成功取件、关闭；同一个地址重复接入复用原记录。

同日再追加「自定义域名（IMAP）」：管理端 40 个测试（另有 3 个 `-tags live`）、前端 23 项通过。联网测试对 `imap.gmail.com` 与 `imap.qq.com` 完成 TLS 握手与协议会话，并在 LOGIN 阶段以「自定义邮箱登录失败」拒绝错误凭据——**没有真实邮箱账号，未验证成功登录与真实取件**。期间 `imap.gmail.com:993` 出现过一次 20 秒超时（随后恢复），属网络或 Google 侧临时限制。
