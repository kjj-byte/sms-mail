# 官方公开价格来源核查

日期：2026-11-06。DaisySMS 已停服移除，当前 24 个主渠道；见 [网站复核记录](site-availability.md)。最新排查、认证密钥填写与修复见 [本轮排查记录](collection-repairs.md)。“有 API”与“匿名能取得价格”分别核查。未确认不等于不存在；不把模型密钥用于平台 API。

| 渠道 | 接口 / 页面与结果 |
|---|---|
| 5SIM | [官方文档](https://5sim.net/docs)：匿名 guest/products、guest/prices、guest/countries；1,783 条报价。 |
| SMSPool | [官方 OpenAPI](https://api.smspool.net/openapi.json)：service/retrieve_all、request/pricing 无需密钥；2,082 条报价。 |
| OnlineSIM | [官方 getTariffs 文档](https://onlinesim.io/openapi_docs/Onlinesim-API-UN/Onlinesim-API-UN-get-api-gettariffs-php/)：按公开国家目录读取 98/98 个国家，本轮 2,556 条聚合报价。 |
| SMS-BUS | [官方 API](https://sms-bus.com/docs/) 需 token；改抓 [官网](https://sms-bus.com/) Nuxt 零售数据，262 条报价。 |
| SMSAK | [官方 API 文档](https://sms-acktiwator.ru/ru/otp/api-docs/)：目录与价格公开，完整价格下载实测约 4 分钟，已单独延长超时；本轮成功取得 2,462 条报价。 |
| Textverified | API 需认证；已从[公开目录](https://www.textverified.com/services)发现详情链接并读取重点对象的 SMS 单次价格，排除语音和租赁。 |
| SMS-Man | [官方 API](https://sms-man.com/es/api) 需 token，认证接入已完成；匿名页面 HTTP 403。 |
| SMSPVA | 开发者 API 需密钥；官网访客接口匿名取得 69/69 个国家的价格与库存，本轮 349 条聚合报价。 |
| Tiger SMS | [官方 API](https://tiger-sms.com/cn/api) 需平台密钥；只读认证接入已完成，等待本地填写密钥和原币种。 |
| SV Number | [官方 API](https://sms-verification-number.com/en/api-sms-activate/) 需 api_key；页面未取得合格价格。 |
| HeroSMS | 页面 Nuxt 中直接取得服务目录和最低可用价，排除租赁产品和国家名称。 |
| GrizzlySMS | 从同一官网服务卡片取得对象、国家、价格；另提供认证 API 接入。 |
| SMS4Sats | [官方 v2 文档](https://docs.sms4sats.com/)：全目录 802 项已纳入只读询价；本轮完成 301 项，274 项有价，剩余项继续补抓。 |
| SMSFAST | 已将 about-us 改为首页，当前源站连接失败；API 需独立密钥，尚未接入。 |
| LongSMS | 已接入官网匿名 `/api/countries` 与 `/api/pricing?product_line=long`；只读取长期套餐，短期目录需网页登录。 |
| GoInSMS | 共享号码页面，未取得可验证的接码对象目录。 |
| SMS24.me、Quackr、Receive-SMSS、SMSOnline、Online SMS Box | 公开号码内容不能当作付费接码价格；页面兜底本轮没有有效目录/价格。 |
| 豪猪 | 官方 `/sms/` API 账号/密码可登录并读取余额，但只提供余额、取号、验证码，没有项目价格查询；`/v1/projects` 需网页登录态。 |

最新本轮统计见用户页面与 [验收记录](verification.md)。保留历史完整记录，所以当前目录记录总数与本轮更新数不同。

这是最低公开报价，库存随时变化。没有明确别名的不同对象名独立保留；未将所有国家、所有号池全部展开，不宣称完整覆盖所有平台。
