# identity · 项目交接与代码审查指南（新对话请先读本文）

> 用途：在新对话中让接手者（AI 或人）**快速、准确地接手本项目，做代码审查与内容优化**。
> 本文以「当前真实状态」为准；凡是本文没写的，先读代码/`docs/` 再判断，不要臆测。
> 最近更新：2026-11-08（Asia/Shanghai）。工作目录 `D:\sms`。

---

## 1. 这是什么

2026-11-08 邮箱改为原生实现：左栏切换 SMS / 邮箱，本机管理服务内置创建、收件、验证码提取及关闭，不再依赖 Docker / Node / API Key。下列静态导航边界适用于 SMS；邮箱只在本机 `/preview/#mail` 使用。详见 [邮箱功能](mail.md)，服务端实现 `admin-panel/mail.go`、`mailproviders.go`、`mailcode.go`，前端测试 `web/mail.test.cjs`。

一个 **SMS 接码平台比价 / 导航站**：比较各接码平台在「接码对象 × 国家」下的公开报价、规则（可用时长、再收码、同号续费、未收码退款、结束后复用），用户在本站筛选比价，**点击按钮跳到对应平台官网**去注册/充值/取号/收码/退款。

**明确边界（不可改变用户要求）**
- **不接支付**、不做统一钱包、不转售、不代下单、不收集验证码/账户/API Key。
- 业务流程始终是「选择平台 → 跳转对应平台官网」。
- 展示平台公开资源价，不等于到手价（可能加服务费/手续费）；非美元报价同时展示带“≈”的美元参考值、原价及汇率日期。
- 纯本地/静态项目：公开站是**静态 HTML**，管理员是**本机 127.0.0.1:8787** 的单管理员工具，不公网部署。

---

## 2. 两个独立 Go module

| Module | 目录 | 作用 | 二进制 |
|---|---|---|---|
| `identity` | 根 `D:\sms` | 静态导航生成器 + 价格采集器（库代码在根包，两个 `cmd/`） | `bin\sitegen.exe`、`bin\priceupdate.exe` |
| `identityadmin` | `D:\sms\admin-panel` | 本机邀请链接管理面板，读根目录 catalog，调用 sitegen 生成 dist | `bin\identity-admin.exe` |

- Go 版本以本机为准（`go.mod` 写 `go 1.26`）。**纯标准库，不新增第三方依赖。**
- 两个 module 各自 `go test ./...`；根 `go test ./...` **不覆盖** 嵌套的 `admin-panel`。

## 3. 目录与关键文件

```
D:\sms\
  data\catalog.json          ★唯一公开事实源：providers/products/offers/services/service_listings/price_update
  web\index.gohtml           ★公开站页面模板（结构、顶部提示、表单、内嵌 JSON）
  web\app.js                 ★公开站前端（按需渲染卡片、筛选/排序、对象浏览、自动补全、渠道报价预览弹窗）
  web\styles.css             公开站样式
  web\app.test.cjs           前端自检（node:test，喂 JSON 数据跑真实 app.js）
  generate.go                ★生成器：加载 catalog→合并 umbrella→排序→序列化紧凑 JSON→渲染→原子发布 dist
  catalog.go                 Catalog/Provider/Product/Offer/Service 结构 + 严格 JSON + Validate()
  prices.go                  ★价格采集主流程 UpdatePrices + 分析回退 + 证据校验 + 原子写
  public_prices.go           ★公开 API 采集分派（5SIM/SMSPool/SMSAK/... 各 collectXxx）
  public_onlinesim.go / public_smspva.go / public_sats.go / public_svnumber.go / public_smsman.go
                             各渠道专用只读采集器
  authenticated_prices.go    认证(密钥)价格 API（sms-man/tiger-sms/grizzlysms/herosms/sv-number）
  page_prices.go / page_channels.go   页面抓取 + 页面兜底解析 + 失败原因文案
  targets.go                 接码对象名归一/别名/ID 稳定
  countries.go               ISO码/英文别名 → 中文国家名
  currency.go                非 USD 报价按快照汇率折 USD（原价写进证据）
  message_filter.go          短信验证码示例识别（防把收件箱内容当证据）
  cmd\sitegen\main.go        生成器 CLI
  cmd\priceupdate\main.go    采集器 CLI
  config\price-update.json   采集来源/分析API/密钥文件路径（私有，gitignore）
  config\provider-keys.json  各平台只读 API key（私有，gitignore）
  admin-panel\               管理面板（见其 README）
  dist\                      公开产物：index.html + app.js + styles.css（生成，勿手改）
  bin\                       三个 exe（生成，勿提交）
  docs\                      历史与验收文档（见 §9）
  update-prices.cmd / start.cmd / build.ps1 / watch-prices.cmd / enable-price-updates.ps1
```

## 4. 构建与运行（Windows PowerShell）

```powershell
Set-Location 'D:\sms'
$env:GOCACHE = 'D:\sms\.cache\go-build'   # 如默认 cache 不可写

# 构建三个二进制
& 'D:\Go\bin\go.exe' build -trimpath -o 'D:\sms\bin\sitegen.exe' ./cmd/sitegen
& 'D:\Go\bin\go.exe' build -trimpath -o 'D:\sms\bin\priceupdate.exe' ./cmd/priceupdate
Set-Location 'D:\sms\admin-panel'
& 'D:\Go\bin\go.exe' build -trimpath -o 'D:\sms\bin\identity-admin.exe' .

# 生成公开站（不加 -referrals 则全部用官方链接）
Set-Location 'D:\sms'
& 'D:\sms\bin\sitegen.exe' -catalog 'D:\sms\data\catalog.json' -out 'D:\sms\dist'
# 带管理员邀请配置：
& 'D:\sms\bin\sitegen.exe' -catalog 'D:\sms\data\catalog.json' -referrals 'D:\sms\admin-panel\data\referrals.json' -out 'D:\sms\dist'

# 采集价格（约 25 分钟预算，写回 catalog，再自动/手动重新生成）
& 'D:\sms\bin\priceupdate.exe' -config 'D:\sms\config\price-update.json' -catalog 'D:\sms\data\catalog.json' -out 'D:\sms\dist'

# 启动管理面板 + 本地预览
& 'D:\sms\bin\identity-admin.exe' -catalog 'D:\sms\data\catalog.json' -referrals 'D:\sms\admin-panel\data\referrals.json' -builder 'D:\sms\bin\sitegen.exe' -site-out 'D:\sms\dist'
# 打开 http://127.0.0.1:8787/  预览：http://127.0.0.1:8787/preview/
```

**测试**
```powershell
Set-Location 'D:\sms'
& 'D:\Go\bin\go.exe' test ./...     # 根模块
& 'D:\Go\bin\go.exe' vet ./...
& 'D:\node.exe' --test web/app.test.cjs web/mail.test.cjs   # 前端自检
Set-Location 'D:\sms\admin-panel'
& 'D:\Go\bin\go.exe' test ./... ; & 'D:\Go\bin\go.exe' vet ./...
# 可选联网冒烟（真实创建匿名收件箱）： & 'D:\Go\bin\go.exe' test -tags live -run TestLiveMail -v
```
`build.ps1`/`start.cmd` 是便捷脚本；本机若禁 `.ps1`，直接用上面的完整 CLI + `start.cmd`。

---

## 5. 数据模型（`data/catalog.json`）

顶层键：`schema_version, providers, products, offers, candidates, services, service_listings, price_update`。

- **Provider**：`id, name, aliases, url, hosts[], api_hosts[], note, sources[], verified`。`hosts/api_hosts` 是邀请链接与采集的**域名白名单**（精确匹配，不放子域）。
- **Product**：某平台的一种产品类型（`kind`: `free|once|rental`）。**规则字段**：`duration, starts, repeat, reuse, renew, auto_renew, restore, online`（后六个取值集合 `supported|unsupported|conditional|unknown|conflicting`）、`refund`（`refundable|nonrefundable|conditional|unknown|conflicting|na`）、`conditions`、`sources[]`、`verified`。
- **Offer**：一条报价，绑定 `product` + `service_id` + `country` + `term` + `number_type` + `scope` + `unit` + `currency` + `amount`(十进制字符串或 null) + `kind`(`exact|starting|unknown|login`) + `status`(`verified|conflicting|unavailable`) + `source` + `verified` + `checked_at` + `evidence`(原文片段) + `method`。
- **Service**：接码对象（如 Telegram），`id, name, aliases[]`。**对象 ID 稳定**（按名字哈希），改名不能失配。
- **ServiceListing**：某渠道"列出了该对象"但可能无价的记录。
- **Candidate**：待核验候选（当前 3 个：SMSBower/VAK-SMS/SMSHub）。**页面已不展示候选区**，数据保留。
- **price_update**：最近一次采集的统计/失败/警告（顶栏「采集范围与时间」展示）。

**当前规模和测量**：统一见 [最新验收快照](verification.md#当前验收快照2026-11-07)。原始服务数、别名归一后的展示对象数、报价记录数与本轮成功采集数是不同口径。

`catalog.Validate()` 是硬约束（重复键、类型、关联、URL 白名单、金额与 kind 一致性、精确报价缺条件等）。**改 catalog 后必须过 Validate。**

---

## 6. 生成管线（`generate.go / Generate`）与前端

1. `LoadCatalog` → `mergeUmbrellaTargets`（把 `AnyOther/Any other/AnyOther/Другой/Любой другой/Not Listed⋯/other/OTHER (no guarantee)/Service Not Listed` 等**同一兜底对象的不同叫法**合并为一个「其他/未列出 (Any Other)」）。
2. 按 service 名/别名索引，回填 offer 的 service_id/service。
3. `unifyCurrency` → 非 USD 报价按快照汇率折 USD（原价与原币种写进 evidence）。
4. `SortOffers`：产品类型 → 有价优先 → 币种 → 单位/租期/报价类型/号码类型/适用范围 → 精确有理数比价（`math/big.Rat`）。前端使用精确十进制字符串比较；按渠道分组时另以渠道和完整条件为先。
5. 构造 `Page`：`Rows`、`Targets`（对象下拉）、`Pending`。热门直达固定为 Google、Telegram、WeChat、GPT（筛选 OpenAI）、Claude。
6. **序列化紧凑 JSON**（`page.RowsJSON`，内嵌 `<script id="rows-data">`）：
   - `products`：产品级重复字段（时长/条件/管理URL/来源/核验日等）查表；
   - `pool`：字符串池（term/号码类型/适用范围/采购URL/读取方式/报价来源/核验日/采集时间 用整数索引）；
   - `rows`：每条报价的短键记录；`k` 是产品类型，`ok` 是逐条报价类型；`n/sc` 为号码类型/适用范围的池索引；`oa/oc/ora` 为换算前的原价/币种/续费价，仅换算行存在。
   - `rate_date`：美元换算使用的汇率快照日期，卡片和报价弹窗显示。
   - **证据原文 `ev` 已不再内嵌**（卡片不展示，省 ~2.9MB）。
7. 渲染模板 → 原子发布（同级暂存目录 + `.previous` 备份替换；输出目录只允许 3 个文件，防止误删）。

**前端 `web/app.js`（无框架）**
- 读 `#rows-data` JSON，展开 pool + products，**只在 `#ledger` 渲染当前视图前 60 张卡**（"点击加载更多"追加）——避免数万 DOM 节点导致卡顿。
- 筛选：关键词(多词与)、对象、国家、渠道、主区、栏目、租期、退款；排序（价升/降/分组）。URL 参数恢复 + 前进后退。
- 对象浏览器（`#obj-data`，可搜、可分页）；关键词自动补全（按前缀→词首→包含，组内按覆盖渠道数降序）。
- 每卡「本渠道全部 N 个报价」按钮 → 原生 `dialog` 可搜列表，支持 Enter 打开、Tab 循环、Esc 关闭及焦点返回。
- **0 条且选了国家**时，若放宽国家有结果，说明本站未收录该国的匹配记录，并提供清除国家筛选按钮；不将本站覆盖范围等同源站能力。
- 国家名中文；`country=未知` 且 `kind=starting` 显示「跨国起价（国家未指定）」，其他未知国家显示「国家未注明」。
- 对象选择和两处加载更多均为原生按钮；卡片与筛选需要 JavaScript，禁用时有明确提示。

---

## 7. 采集（`prices.go / UpdatePrices`）

- 读 `config/price-update.json`（`sources[]` 每项 `provider/product/url/currency`；`provider_keys` 指向 `config/provider-keys.json`）。
- 每来源：优先官方匿名/认证只读 API（`public_*.go` / `authenticated_prices.go`），失败再抓价格页/内嵌数据（`page_*.go`）。全流程不调用模型。
- **证据校验**：金额、国家、单位、term、币种必须能在同一原文片段里找到，否则丢弃；短信验证码示例不得作证据；截断的大响应整批丢弃。
- 按「渠道×对象」保留各国最低价；5000+ 对象的渠道只对重点对象（Google/Telegram/WhatsApp/OpenAI，见 `focus`）拉逐国价，其余只记**全球起价**（country=未知）——**这是设计取舍，不是缺数据**（约 96% 对象只有全球起价）。例外：「其他/未列出 (Any Other)」始终保留逐国价，它没有具体服务可比，只能按国家比较。
- 写回：`catalog.Validate()` → 变更检测（内容变了才写）→ 备份 `.backup` → 原子替换；失败保留旧文件。

---

## 8. 管理面板（`admin-panel/`，独立 module）

- 默认 `127.0.0.1:8787` 提供短信／邮箱用户工作区，根地址跳转 `/preview/`。邀请管理入口已移出用户界面，仅 `D:\smsinvite` 工具的 `/links/` 私有通道可访问，8787 上不带私有凭据的管理路由返回 404（**不远程部署**）。
- 安全：Host 校验、POST-only、CSRF、同源 Origin、请求体大小限制、邀请 URL 必须 HTTPS + 精确域名白名单、预览只读公开产物、拒绝 `..`。
- 邀请配置 `data/referrals.json`（`provider_id → 完整HTTPS URL`，私有，gitignore）。保存与「更新本地导航」是两步；生成失败不丢已存配置。购买入口用邀请，规则/管理入口保持官方。
- **邮箱（mail）原生内置**：`mailproviders.go` 声明式渠道配置 + 通用 REST/GraphQL 引擎（9 个免费渠道，固定顺序即自动调度顺序），`mailcode.go` 提取验证码，`mailimap.go` 是自定义邮箱（IMAP，implicit TLS 993、只读 INBOX、正文缓存），`mail.go` 提供 `/mail-api/*` 与 `data/mail.json` 持久化（私有，gitignore）。接入自己已有的邮箱有两条路：`Create.Skip` 且静态域名的渠道（Maildrop.cc / Catchmail.io）通过 `AttachDomains()` 暴露本域任意地址，免密码；其他域名要求 IMAP 授权码，凭据只存本机、关闭记录时清除。收件箱列表带 `state`（active/expired/closed）供前端按状态或渠道分组。原独立 mail-hub 服务已不再需要；未移植账号池/配额/统计，见 [邮箱功能](mail.md)。邮箱接口沿用 Host/Origin/CSRF/请求大小校验，只提供白名单路径，不是通用代理。
- 管理端主页面只管理邀请链接与本地导航，不启动价格采集。价格采集仍由 `update-prices.cmd` 或每日任务运行（约 10–25 分钟预算）；采集失败保留旧报价和核验时间。更新器非 watch 模式退出码：0 全部完成，1 未完成，2 参数错误，3 目录已保存但生成失败，4 全来源失败状态已保存且导航已生成。
- 详见 `admin-panel/README.md`。

---

## 9. 已有文档（`docs/`，历史 + 验收）

- `verification.md`：**逐轮验收记录**（含最近改动：hero-sms 风格改版、国家中文化、自动补全、对象浏览、按需渲染、规则归一、原始价声明、删候选/采集上顶栏/删证据、umbrella 合并、0 条引导、字符串池减负、邮箱原生化）。**审查时以它为准看"最近做了什么/怎么验的"。**
- `mail.md`：邮箱原生实现（9 渠道、调度顺序、`data/mail.json`、本地 API、未移植项、许可与验收）。
- `site-availability.md`：站点可用性/移除记录（DaisySMS、SMS-Activate、Truverifi 已移除；E码云、D1 按用户要求移除）。
- `collection-repairs.md`：采集修复史（含 SMS-Man 公开站点 key 绕过 $13 门槛）。
- `price-updates.md`：采集配置、密钥获取入口、24h 任务。
- `collection-coverage.md / public-price-sources.md / platform-research.md / build-plan.md / panel-and-source-audit.md / github-references.md`：调研与方案（多为历史，现状以 README+verification 为准）。
- 根 `README.md`：项目总入口、界面说明、启动与检查命令。
- 历史交接：`HANDOFF.md`（早期规划，部分已过时）。

---

## 10. 代码审查重点清单（建议按此审）

**正确性**
- [ ] `catalog.Validate()` 是否真的拦住脏数据；`mergeUmbrellaTargets` 是否会误合并真实对象（如 Anycoin/Anygo/Hanya 必须独立）。
- [ ] 价格一致性：显示金额 == evidence 金额；`kind`（exact/starting）与金额是否匹配；`unifyCurrency` 折算是否把原价/汇率写进证据、是否误混币种排序。
- [ ] `SortOffers` 精确比价（`big.Rat`）、币种/单位不混排；前端 `priceCmp` 与后端排序口径一致。
- [ ] 生成管线序列化（products 查表 + pool 索引）**展开无损**：前端 `t/h/me/os/ov/ca` 索引还原正确；缺字段不崩。
- [ ] 按需渲染：`appendPage` 分页正确、加载更多不重复、筛选变化重置；空态与「0 条引导」逻辑正确。

**安全**
- [ ] 邀请/采集 URL 一律走 `ValidateURL`（HTTPS、无用户名、精确 host、端口、控制字符）；页面 `html/template` 转义 + 前端 `esc()`；内嵌 JSON 经 `json.Marshal`（`<>&` 转义）不可注入 `</script>`。
- [ ] 管理面板 Host/Origin/CSRF/大小/预览路径限制仍在；不把密钥写入 dist/日志。
- [ ] 采集不发密钥给第三方来源；不跟随不可控重定向。

**健壮性/性能**
- [ ] 采集失败保留旧数据、不刷新事实日期、不半覆盖；并发上限、超时、截断丢弃。
- [ ] `dist` 体积和 DOM 数见最新验收快照；首批 60 卡；压缩改动须同时比较原始大小与 gzip。
- [ ] 前端在 ~2.3 万行数据下的筛选/排序耗时；内存。

**内容/产品**
- [ ] 文案是否诚实（原始价 disclaimer、全球起价含义、未核实/资料冲突标注、不宣称最低价）。
- [ ] 规则字段（退款/续费/时长）是否与官方页面一致；`conflicting` 是否恰当保留而非硬猜。
- [ ] 无障碍（键盘可操作、`details/summary`、焦点可见）、手机宽度可读。

---

## 11. 内容优化候选（已知可改进点，先判断价值再动）

1. **体积再压缩**：已离线模拟将 `service/country` 加入池，原始 JSON 减少约 241KB，但 gzip 反增约 22KB，本轮不采用。改变分组折叠交互前应先证明实际收益。
2. **逐国覆盖**：现仅 4 个热门对象与「其他/未列出 (Any Other)」逐国采集；如需更多，需指定重点对象清单（全量逐国不现实、会拖慢/变大）。
3. **文档维护**：README 为入口，verification 顶部维护唯一带日期的验收快照，其下保留历史，避免重复宣称“当前/最新”。
4. **未验证项**：真实浏览器手机/低配滚动帧率、部分渠道实时价与官网一致性（采集有 403/需登录，未逐站复查）、邀请归因（无真实邀请码）。

---

## 12. 常见操作备忘

- **只重新生成、不采集**：跑 `sitegen.exe -catalog data\catalog.json -out dist`。
- **改页面**：改 `web/*` → 重建 `sitegen.exe`（web 资源是 `//go:embed`，改完必须重编二进制再生成）→ 再 `sitegen`。
- **改数据**：改 `data/catalog.json`（走 Validate）→ 重生成。**别手改 `dist/`。**
- **加采集渠道**：在 `public_prices.go` 的 `collectPublicAPI` 分派 + 对应 `collectXxx`，或在 `authenticated_prices.go` 白名单加平台；来源要在 `config/price-update.json` 登记且 provider 的 `api_hosts` 含该域名。
- **跑测试前**：确认 `GOCACHE` 可写；前端测试用 `D:\node.exe`。

---

## 13. 给新对话的开场建议指令

> 先读 `docs/PROJECT-GUIDE.md`（本文），再按需读 `README.md`、`docs/verification.md`、相关源码。遵守工作区 AGENTS/Ponytail（最小改动、别过度设计）。做**代码审查**时按 §10 清单，输出按严重度排序、每条给文件:行与理由、标明"已验证/未验证"。做**内容优化**时按 §11，先说明价值与风险，改动后跑 §4 的测试与生成并核对 `dist`。不要引入支付、不要改"选平台跳官网"的业务、不要新增第三方依赖、不要把管理员部署到公网、不要编造价格或邀请码。

---

*本文是给"新对话"的入口。若代码与本文冲突，以代码 + `docs/verification.md` 为准，并顺手更新本文。*
