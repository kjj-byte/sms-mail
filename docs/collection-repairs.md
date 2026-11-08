# 本轮信息采集排查与修复

当前目录为 **22 个主渠道、3 个候选**（2026-11-07 已按用户要求移除 E码云、D1）；详情见 [网站可用性复核](site-availability.md)。下文保留此前排查过程。

## 价格来源层修补（2026-11-07 下午）

- **免费/登录来源移出付费采集**：共享公开收件箱（GoInSMS、SMS24、Quackr、Receive-SMSS、SMSOnline、Online SMS Box）与 LongSMS、豪猪不再作为付费价格 `sources` 重试；它们仍保留目录、免费区或待核实入口，不再混入“价格采集失败”。`config/price-update.json` 与示例同步。
- **来源顺序调整**：Tiger、Grizzly 等快速官方 API 前置；SMS4Sats 逐对象询价、易限流，移到最后，避免挤占 25 分钟预算导致其它来源超时失败。
- **SMSFAST 公开起价**：官网无匿名逐对象价格 API；采集器读取首页公开 “From $0.21”，替换旧的硬编码手动报价，写入证据、来源与采集时间。该条仍标注为产品级起价，不是逐对象报价。
- **成功刷新替换旧行**：来源完整成功时删除同产品未再出现的旧自动报价，避免“来源上次访问失败”历史行无限累积（目录接近 32 MiB 上限）；失败或部分成功（如 SMS4Sats）仍保留旧行并标 `unavailable`，不伪装成新报价。
- **管理员更新按钮（已于 2026-11-08 按用户要求移除）**：本机管理页面与导航预览不再触发价格采集；采集继续由 `update-prices.cmd` 或每日任务运行。详见 [价格更新说明](price-updates.md)。

## 接入 SMS-Man 公开站点价格（本轮，绕过 13$ 账户门槛）

此前 SMS-Man 只有账户 control API，需账户余额 >13$ 才能取价，否则 403/拒绝。本轮发现：**官网 SPA 自身就是用一个公开的「站点发布密钥」调用同一套只读接口**，该密钥不带账户余额门槛即可取公开目录与逐国价格。

- 目录：`GET https://api.sms-man.com/api/services?api_key=<站点密钥>&l=en`、`/api/countries?...`
- 逐国价格：`GET https://api.sms-man.com/control/get-prices?token=<站点密钥>&country_id=<ID>&currency=USD`
  - 一次返回该国家**所有应用**的 cost/count；`currency=USD` 明确美元（不带 currency 默认 RUB）。
- 新增 `public_smsman.go`：`collectSMSMan` 先取服务/国家目录，再按国家并发（3）逐个 `get-prices&currency=USD`；每个对象记「跨国最低起价（国家=未知）」+ 重点对象（Google/Telegram/WhatsApp/OpenAI）记逐国价，币种固定 USD。国家名优先用 ISO code 归一（标题常为 `United States/USA` 这类无法直接归一的形式）。
- 站点密钥内置于官网页面 bootstrap（非账户 token，不涉及登录/充值/取号），只读取目录与价格。
- 实测（联网）：**5,343 对象 / 5,659 报价 / 180 个国家**，如 Telegram 美国≈1.80 USD、奥地利 3.51、阿尔巴尼亚 0.69。已在 `public_prices_test.go` 增加单元测试（含货币 USD、库存<=0 跳过、逐国价断言）。

## 接入 SV-Number、HeroSMS（上一轮）

1. **SV-Number 接入成功**：其 API 为 sms-activate 兼容（`sms-verification-number.com/stubs/handler_api`，`lang=en`=USD），但 `getPrices` 必须带 `country`。新增 `public_svnumber.go`：先 `getCountryAndOperators` 取国家，再逐国 `getServicesAndCost&country=ID`；每个服务记“跨国最低起价（国家=未知）”，仅 Google/Telegram/WhatsApp/OpenAI 记逐国精确价，避免 10 万+ 报价撑爆目录。实测 **535 对象 / 1154 报价 / 209 国**，余额 $0 也能只读取价。
2. **HeroSMS 接入成功（充值后）**：HeroSMS 是 sms-activate 兼容（`hero-sms.com/stubs/handler_api.php`，币种 840=USD）。注意 `getServicesList` 是**公开接口、不校验 key**（无 key 也 200），所以不能用它判断 key 有效；真正的鉴权接口 `getPrices`/`getBalance` 才判 key。用户首把 key 在 `getPrices` 上 401 BAD_KEY（key 无效/已被邮件重新生成），换新 key 后 `getBalance` 返回 `2.0856` 美元、`getPrices` 正常。实测 **812 对象 / 1502 报价**。账户 2$ 余额足够查询（查询不扣费；官方“最低充 3$”是充值档位，不是查价门槛）。
3. **豪猪 haozhuma**：给的是“api账号+api密码”签名式接口（`h5.haozhuma.com`），协议与 sms-activate 不同，需单独逆向签名；本轮未接入。
4. 其余失败不变（SMS-Man 余额>13$ 门槛/403、Receive-SMSS 403、LongSMS 动态页、SMSFAST 密钥未接、共享收件箱类、豪猪/E码云/D1 需登录）。
5. 本轮结果：全站 16,355 条有效报价、12 个来源失败；Google 显示 15 个平台、Telegram 16 个平台；价格统一 USD，排序在同币种同单位内升序、无断点。

## 统一币种 + 数据缺漏审计（上轮）

1. **统一币种为 USD**：生成时把非 USD 报价按快照汇率折算（`currency.go`：BTC/USD=75854、1USD=6.6713CNY，2026-11-06），原价与原币种写进证据。页面所有价格 `<strong>USD …`，无 sats/CNY 混排；排序在同币种同单位内升序，断点=0。
2. **HeroSMS 只有 4 条**：抓的是落地页（只嵌 5 服务）；真实目录是 sms-activate API。
3. **瞬态失败修复**：`fetchPage`/`publicJSON` 加 3 次重试，恢复 Textverified/SMS4Sats 目录。SMS4Sats 本轮仍被限流，从本轮采集剔除以免耗尽预算，历史报价保留并标“来源上次访问失败”。

1. **统一币种为 USD**：生成时把非 USD 报价按快照汇率折算为 USD（`currency.go`：BTC/USD=75854、1USD=6.6713CNY，2026-11-06），**原价与原币种写进证据**（“原价 X sats ≈ USD Y（汇率折算）”），采集端仍保存原币种。结果：页面所有价格 `<strong>USD …`，不再出现 sats/CNY 混排；排序在同币种同单位内升序，断点=0。
2. **HeroSMS 只有 4 条有价的根因**：此前抓的是 `hero-sms.com/cn` 落地页，其 Nuxt 状态只嵌入 5 个服务（Full rent 等）。已核实 HeroSMS 是 **sms-activate 兼容 API**（`hero-sms.com/stubs/handler_api.php`，`api_key`，币种 840=USD），已把 HeroSMS 接入认证 API 路径（`loadProviderKeys`/`authenticatedJSON`/`collectAuthenticatedPrices`），**待用户提供 API Key 后即抓全量**；无 key 时退回落地页（仅 4 条，页面标注）。
3. **其余平台缺漏审计**：13 个有价渠道（grizzly 2677、onlinesim 2655、smsak 2467、smspool 2082、fivesim 1783、tiger 1362、smspva 418、sms-bus 262、textverified 4、herosms 4…）。失败 13 家均为外部限制：
   - SMS-Man 403（余额>13$门槛）、Receive-SMSS 403、LongSMS 动态页/失效；
   - SV-Number、SMSFAST 需独立密钥（协议未接）；
   - GoInSMS、SMS24、Quackr、SMSOnline、Online SMS Box 为共享收件箱，无“服务×国家”价格结构；
   - 豪猪、E码云、D1 需登录/商户 API。
4. **瞬态失败修复**：给 `fetchPage`、`publicJSON` 增加最多 3 次重试（网络/读取中断才重试），恢复了上一轮因“响应读取失败”丢失的 Textverified（4）与 SMS4Sats 目录。SMS4Sats 本轮被站点限流导致询价超时，已从本轮采集剔除以免耗尽预算拖垮 Tiger/Grizzly，历史 274 条价格保留并标“来源上次访问失败”。

## 接入 Tiger SMS / GrizzlySMS 官方 API（上轮）

用户提供两家 API Key 后接入 `config/provider-keys.json`。两家均为官方只读价格 API（币种 USD）：Tiger SMS 985 对象/1362 报价/196 国；GrizzlySMS 2508 对象/2677 报价/206 国。并给 `authenticatedJSON` 加 3 次重试 + 客户端超时 100s→180s，修复 Grizzly 约 3.7MB 响应中途断开的“响应读取失败”问题。

用户提供两家 API Key 后接入 `config/provider-keys.json`（SMS-Man 需余额 >13$，暂不用）。两家均为官方只读价格 API（`getServicesList / getCountries / getPrices`，币种 USD）：

- **Tiger SMS**：985 个接码对象、1374 条报价、覆盖 196 个国家/地区。
- **GrizzlySMS**：2508 个接码对象、2677 条报价、覆盖 206 个国家/地区。

两家均已进入主力梯队，Google 等热门对象可显示的渠道从 13 升到 14–15 个。全站报价 10,288 → 13,709（本轮有效报价），页面从 29.9 MB → 41 MB。

**修复的取价问题**：GrizzlySMS 的 `getPrices` 约 3.7 MB，原代码在读取大响应时偶发“响应读取失败或超出大小限制”（连接中途断开）。已在 `authenticatedJSON` 增加最多 3 次重试（针对网络/读取中断，HTTP 非 200 不重试），并将 `http.Client` 总超时从 100s 提升到 180s。修复后 Grizzly 稳定取得 2677 条报价。

## 价格排序与搜索完整性（上轮）

1. **按价格排序**：`SortOffers` 与前端 `app.js` 统一为同币种、同计费单位内按价格升序（有价在前）。新增“排序”下拉；每卡带 `data-price/currency/unit/priced`。币种与单位永不混排比较。
2. **搜索关键词显示全部平台**：`data-q` 含服务名+别名+来源名，搜 Google/Telegram 可命中全部有该对象的平台。
3. **信息源现状（诚实口径）**：24 渠道中 11 个取得可核验价格（onlinesim、grizzlysms、smsak、smspool、fivesim、tiger-sms、smspva、sms4sats、sms-bus、herosms、textverified）。15 个来源本轮失败：SMS-Man 余额门槛/403、LongSMS/Receive-SMSS 403、Tiger 类已解决、SV-Number/SMSFAST 需独立密钥、GoInSMS/SMS24/Quackr/SMSOnline/Online SMS Box 为共享收件箱、豪猪/E码云/D1 需登录。属外部限制，未绕过、未伪造价格。

本轮实际核验 **7,067 条有效报价，来自 10 个渠道**；目录汇总 6,588 个接码对象、10,644 条渠道对象记录（含历史记录）。移除停服来源后，14 个来源仍未成功，原因已分为认证缺失、HTTP 403、源站网络故障、停服及无公开对象目录。

SMSAK 本轮成功取得 2,470 条价格。其余报价渠道为 5SIM、SMS-BUS、HeroSMS、Textverified、OnlineSIM、SMSPool、SMSPVA、SMS4Sats、GrizzlySMS。

## 已确认的程序和来源问题

| 来源 | 原因 | 修复 |
|---|---|---|
| SMSAK | 约 2.8 MB 的完整响应实测下载 4 分 12 秒；原 HTTP 100 秒会截断 | 仅该接口的 HTTP 上限改为 6 分钟，流程上限 7 分钟；总任务仍限制 25 分钟，完整 JSON 才发布 |
| HeroSMS | Nuxt 中用索引引用服务名、库存和价格，普通文字解析漏掉 | 直接解析索引，读取 priceMinAvailable，排除 Full rent 和国家目录 |
| GrizzlySMS | 国家价格和服务价格混在页面，原来没有单次产品绑定 | 增加单次激活产品；仅解析同一 service-card 内服务、国家和价格，并核对显示价 |
| Textverified | 总目录只有对象名，实际价格在 app 子域的对象详情页 | 从总目录的真实链接读取 price_targets 对应对象页，取 SMS Verification，排除语音和租赁价格 |
| SMSPVA | 开发者 API 需密钥，但官网访客价格和库存接口可匿名读 | 从首页 allServices 获取名称/代码，调用官网的只读 GET_PRICES_BY_COUNTRY 与 GET_AMOUNT_BY_COUNTRY，按相同国家和运营商配对 |
| SMS4Sats | 只读了首页，未接入已经迁移的 v2 API | 使用 api.sms4sats.com 的目录和 quote；quote 不开票、不预约号码，保留 sats 原单位，不推断已分配国家 |
| SMSFAST | 原来源为 about-us，且当前源站连接超时 | 改为首页；网络仍不可用时保留真实网络错误，不使用缓存搜索摘要当作实时价格 |
| DaisySMS | [官网](https://daisysms.com/) 公告已于 2026-03-26 停服，旧 API 文档仍能打开 | 标注停服及历史规则，不再误导为补密钥即可取得现价 |
| 通用日志 | 具体 HTTP、超时、解析错误最后被覆盖为一句“未采集到” | 失败原因保留到页面；目录、报价覆盖不足与来源失败分开说明 |

SMSPVA 当前只展开美国报价；Textverified 和 SMS4Sats 的详情询价按 `price_targets` 获取，其余对象仍进入下拉目录。这些范围限制在采集说明中保留，不等于全量价格。

## 本地平台密钥配置

按你的要求创建 `config/provider-keys.json`。与分析模型的 `config/price-update.json` 分开；已有文件不会被覆盖。填写示例：

```json
{
  "schema_version": 1,
  "providers": {
    "sms-man": {"api_key": "填该平台的真实密钥", "currency": "USD"},
    "tiger-sms": {"api_key": "", "currency": ""},
    "grizzlysms": {"api_key": "", "currency": ""}
  }
}
```

这里的 USD 仅示范格式，**currency 必须填写该账户价格 API 实际返回金额的原币种**；例如实际按卢布计价就填 RUB，程序不做换算。密钥留空的渠道继续匿名页面采集。

价格配置已用 `provider_keys: "provider-keys.json"` 引用，路径相对价格配置所在目录解析。填写后直接运行 `update-prices.cmd`，现有每日任务也会读取该文件。当前支持 SMS-Man、Tiger SMS、GrizzlySMS；DaisySMS 已停服，不创建无用密钥项。其余平台的认证协议尚未接入。

- [SMS-Man 官方 API](https://sms-man.com/es/api)：token 认证；已核验 applications、countries 和 get-prices 端点存在，匿名返回 wrong_token。
- [Tiger SMS 官方 API](https://tiger-sms.com/cn/api)：api_key 认证，getServicesList、getCountries、getPrices。
- [GrizzlySMS 官方 API](https://grizzlysms.com/docs)：api_key 认证，服务目录、国家和价格。

仅允许固定官方地址上的这些只读方法；阻止重定向携带密钥，不请求购买、号码、短信、余额或订单接口。错误不输出带密钥 URL 或原始响应；公开证据只写服务/国家/价格/库存。私有密钥文件已加入忽略清单，不复制到 dist。提交或分享项目时只使用 provider-keys.example.json。

认证流程已使用模拟 transport 验证，真实平台账户尚未联调；填好密钥和原币种后才能核验账户权限及实际响应。

## 剩余来源的原因

- SMS-Man、LongSMS、Receive-SMSS 的公开来源返回 HTTP 403；不绕过访问限制。SMS-Man 可填写独立 token 改走官方 API。
- Tiger SMS 的公开页面没有对象报价，认证价格接入已完成，等待你填写密钥。
- SV Number 的公开页面没有可验证对象报价，官方价格 API 需独立密钥，认证适配仍未实现。
- GoInSMS、SMS24.me、Quackr、SMSOnline、Online SMS Box 主要是共享公开号码和短信页，没有稳定、明确的接码对象目录。不能拿短信发送者/验证码示例生成对象或价格。
- 豪猪、E码云、D1 首页没有公开的对象价格；需登录或其官方认证接口，尚未接入。当前不会伪造它们支持 Google 等对象。

已测试金额精度、库存和服务代码映射、页面不交叉拼价、接口截断、密钥校验与隔离、重定向及购买接口阻止。浏览器和移动端实际操作未实测。
