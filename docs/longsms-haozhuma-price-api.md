# LongSMS / 豪猪价格 API 核查

核查时间：2026-11-07（Asia/Shanghai）。本次读取官方公开页面、官方前端脚本、只读接口和用户手动登录后的网页会话；未取号、未下单、未绕过验证码。LongSMS 采集器、网页快照与目录已接入。

## 当前接入状态（2026-11-07）

- **LongSMS 已接入**：公开长期套餐由 `collectLongSMS` 读取；`/app/shop` 短期 OTP 由网页登录快照读取。当前目录有 179 条有价短期服务的**服务级起价**（`browse/services.price_usd`）；国家起价尚未接入自动流程。长期当前 24 条有库存套餐（美国/英国），金额包含 `first_activation_fee`。
- **豪猪已接入网页项目报价**：`/v1/projects?keyword=…` 需要登录 token；脚本用两位字母/数字关键词扫描并去重，当前合并 6,702 个项目报价（0.120 CNY/单次），项目名重复或超长时合并/跳过。官方 `/sms/` API 仍只有登录、余额、取号，没有价格接口。

## 网页登录快照

`tools/web-prices.mjs --login` 会启动可见 Chrome，由人完成 LongSMS/豪猪登录和验证码；登录态保存在 `.cache/web-prices-profile`。之后 `--auto` 读取 LongSMS `browse/services` 的 `price_usd` 服务级起价，并读取豪猪 `api.zsnixp.cn/v1/projects?keyword=…`（两位字母/数字扫描去重），写入 `data/web-prices.json`。`priceupdate` 自动合并该快照。密码、Cookie 不写入仓库，验证码不绕过；没有会话时脚本跳过，不影响公开 API。

## LongSMS：确认存在可匿名访问的长期价格接口

官网 `https://www.longsms.net/app` 可访问并跳转登录页。前端脚本 `/assets/index-BeNgS1WI.js` 明确使用 `/api` 为基础地址，并调用 `/countries`、`/pricing` 等查询。以下请求通过浏览器同源 fetch，以 `credentials: 'omit'` 实测：

| GET 地址 | 返回 | 用途 |
| --- | --- | --- |
| `https://www.longsms.net/api/countries` | HTTP 200，`success: true` | 国家 ID、国家代码/名称、号码类型、库存等 |
| `https://www.longsms.net/api/pricing?country_id=1&product_line=long` | HTTP 200，12 条价格记录 | 美国长期套餐价格 |
| `https://www.longsms.net/api/pricing?country_id=1&product_line=short` | HTTP 200，空数组 | 此条件没有返回短期价格，不能推断所有短期产品都没有价格 |
| `https://www.longsms.net/api/user/short-term/browse/services?lang=en` | HTTP 401，`UNAUTHORIZED` | 短期对象目录，需要认证 |
| `https://www.longsms.net/api/user/short-term/browse/countries?service_code=tg` | HTTP 401，`UNAUTHORIZED` | 短期对象对应国家/报价，需要认证；参数 `tg` 仅用于验证认证要求 |
| `https://www.longsms.net/api/user/long-projects/catalog` | HTTP 401，`UNAUTHORIZED` | 长期项目代码的目录，需要认证 |

长期价格响应包含 `id`、`days`、`minutes`、`price`、`original_price`、`first_activation_fee`、`active`、`product_line`、`service_code`、`long_project_code`，顶层还有 `lease_cutoff_at`。本次返回的项目代码包括 `other`、`hosting_other`、`hosting_full`、`full_pack`，不能未核实就映射成 Google 等具体对象。

官网前端还展示面向账号的 `/api/client/v1/handler` 文档，以 `X-Api-Key` 鉴权；它不能与网页使用的 Bearer Token 混用。本次实测该 Key 可调用 `getBalance`，但没有 `getPrices`/目录动作；`previewNumber` 只返回号码和 `pick_token`，不返回价格。

接入判断：公开长期套餐采集已接入，报价金额包含 `first_activation_fee`；仍需核实项目代码含义、最终租期截止和续费规则。`/app/shop` 的短期 OTP 目录和报价要网页登录态，现有 LongSMS 报价不代表整个 shop。匿名返回价格不等于服务承诺稳定开放 API；直连采集已验证可用。

## 豪猪：有官方业务 API，未找到官方独立价格查询 API

[官网](https://my.haozhuma.com/)直接链接到[官方 ShowDoc 文档](https://www.showdoc.com.cn/haozhuma)。通过正常浏览器读取文档目录和正文，确认官方服务器为 `https://api.haozhuma.com` / `https://api.haozhuyun.com`，公布了：

- [获取 API 账号和 API 密码](https://www.showdoc.com.cn/haozhuma/11417140388167061)：在官方网页版获取。
- [登录](https://www.showdoc.com.cn/haozhuma/9627394833027598)：`/sms/?api=login&user=...&pass=...`，换取 token。
- [查询余额](https://www.showdoc.com.cn/haozhuma/9627395504455779)：`/sms/?api=getSummary&token=...`；返回 `money` 是账户余额，不能作为项目单价。
- [获取号码](https://www.showdoc.com.cn/haozhuma/9627395408482771)：`/sms/?api=getPhone&token=...&sid=...`；官方示例没有价格字段。
- 指定号码、获取验证码、释放号码、拉黑号码及参数代码表。

当前官方目录没有项目列表/价格查询页面，也没有已核实的 `getPrices` 一类接口。没有证据证明它不存在，但不能把“有接码 API”当作“有价格 API”。

官方网页版 `https://h5.haozhuma.com/login.php` 目前加载新前端，其代码有价格显示字段；页面运行时检查 `https://api.haozhuma.com/v1/ping` 和 `https://api.zsnixp.cn/v1/ping`。`/v1/projects` 存在但返回 401，网页登录接口 `/v1/auth/login` 还要求验证码；这些只能证明有新版 Web API，不足以证明价格查询地址、参数及认证协议已确认。本次未登录网页版，未取得具体项目价格请求，也未绕过验证码。

更正旧记录：`docs/collection-repairs.md` 中“豪猪需逆向签名”的历史判断不适用于已读到的官方 `/sms/` 文档；该文档直接规定账号/密码换 token，未要求这里所述的额外签名。新版网页接口与官方 `/sms/` 接口应分别核实。

接入判断：暂不把豪猪加入自动价格采集；下一步需要官方补充价格 API 文档，或在有授权登录状态时检查网页的只读项目/价格请求。无需通过取号或下单试探价格。
