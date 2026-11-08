# 未配置邀请链接的平台核查

核查日期：2026-11-08（Asia/Shanghai）。对照 `data/catalog.json` 的 22 个平台和本机 `admin-panel/data/referrals.json`，12 个已配置、10 个未配置。只记录配置有无，不公开个人邀请 URL。未修改邀请配置、未注册或加入任何计划。

| 平台 | 结论 | 奖励及下一步 | 官方依据 |
| --- | --- | --- | --- |
| Quackr | 已确认公开邀请返佣 | 标准 15%，被邀请人注册后 12 个月内的合资格消费；可兑换余额或 USDT/USDC。邮箱验证后加入，获得专属链接。 | [Partner Program](https://quackr.io/partners) |
| SMSOnline | 已确认 Premium 邀请返佣 | 20% 消费佣金，可转站内余额或 USDT；页面称无推荐期限及消费金额上限。登录推广页创建代码；新用户可获首次加密货币充值九折券。 | [Premium 推广页](https://premium.smsonline.cloud/referral-program)（已用浏览器加载动态正文） |
| SMSAK | 官网提及邀请与奖金，具体规则待核实 | 官方文章明确提到 referral program and bonuses，但没有比例、奖励类型、提现门槛或生成链接方法；需登录后台或询问客服确认。 | [官方文章](https://www.sms-acktiwator.ru/ru/articles/47/) |
| SMS4Sats | API 推荐字段已确认，现行奖励规则待核实 | 新版接码/租号建单 API 有可选 `referral` 字段；旧索引文档曾写 Lightning address 和 10% 推荐付款，但新版未说明比例或结算办法。不能据旧文档承诺当前 10%，也未确认普通网页邀请 URL 格式。 | [新版建单文档](https://docs.sms4sats.com/api-reference/create-order.md)、[新版简介](https://docs.sms4sats.com/) |
| 5SIM | 未找到现行公开邀请奖励规则 | 已查首页、FAQ、API 文档及站内搜索结果；搜索出现的 `sid` 参数不足以证明返佣。需后台或客服确认。 | [官网](https://5sim.net/)、[FAQ](https://5sim.net/faq)、[API](https://5sim.net/docs) |
| Textverified | 未找到公开邀请奖励规则 | 首页和 FAQ 未给出邀请计划；credits 是充值余额，不能视为邀请赠额。第三方服务的推荐奖励介绍不算本站计划。 | [首页](https://www.textverified.com/)、[FAQ](https://www.textverified.com/faq) |
| GoInSMS | 未找到公开邀请奖励规则 | 公开页为无需注册的免费公共接码，未找到个人邀请代码、返佣或赠额入口。 | [官网](https://www.goinsms.xyz/) |
| SMS24.me | 未找到公开邀请奖励规则 | 无需注册的免费公共接码；站内有关 Airbnb 等平台奖励的文字不代表 SMS24 自身有推荐计划。 | [官网](https://sms24.me/en) |
| Receive-SMSS | 未找到公开邀请奖励规则 | 公开免费接码页未找到邀请或赠额规则；租号广告入口本身也不是邀请计划证据。 | [官网](https://receive-smss.com/) |
| Online SMS Box | 未找到公开邀请奖励规则 | 已检查免费站及私有号商城；条款提到 affiliate/advertising cookies 属通用表述，未给出用户邀请奖励办法。 | [官网](https://onlinesmsbox.com/)、[商城](https://buy.onlinesmsbox.com/)、[条款](https://onlinesmsbox.com/page/terms-and-conditions) |

Quackr 重要限制：90 天点击归因期；佣金延迟 30 天可用，兑换门槛 $50；加密货币提现需要身份验证。按官网说明，OTP codes、余额充值、自购不计佣，部分低毛利产品降低比例或不计佣；每个被邀请人每月佣金上限 $250。其 `/partnership` 是供号合作，邀请计划应使用 `/partners`。

SMSOnline 的奖励属于 Premium 付费业务，不代表免费公共短信页产生佣金。当前目录只收录 `smsonline-free`，域名白名单也只有 `www.smsonline.cloud`，尚未包含 `premium.smsonline.cloud`；因此不能直接把现有免费入口替换成 Premium 推广链接。若后续接入，应单独核实付费产品并调整白名单。Quackr 当前已收录免费与租号两类产品，须确认用户落到计佣产品。

替换入口前应从本人登录后台取得真实专属链接；不要拼造代码或采用搜索结果中的他人推广链接。

限制：未登录各平台后台、未联系第三方、未完成实际消费/奖励结算。未找到公开证据不等于平台一定没有该功能；上述条款也可能调整。
