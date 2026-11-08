# SMS 平台调研：官网比价与跳转

最新复核已移除 DaisySMS、SMS-Activate、Truverifi，当前 24 个主入口和 3 个候选；下文原调研数量为历史过程，现状见 [网站复核记录](site-availability.md)。

核验日期：2026-11-06（Asia/Shanghai）。前两轮主清单为 22 家；本轮从参考项目源码发现并核验 3 个新入口，现为 **25 个平台入口、5 个待核验候选**。入口数量不代表独立资源池、全部字段已验证或最低价。

原始 `C:/Users/27874/Desktop/sms.txt` 的 8 行链接去重为 7 家；附件只作参考。本项目按最新要求仅展示信息并跳转官网，API 可用性不再是收录门槛，不在本项目购买、付款、收码、退款或续费。

## 上轮新增低价入口与补充来源（7 家）

以下均为查询日官方公开资料；宣传起价不是同一国家/服务/号码类型下的成交排名。

| 平台与官方入口 | 公开价格证据 | 可用时长与后续使用 | 退款与收录结论 |
| --- | --- | --- | --- |
| [SMSPVA](https://smspva.com/) | 官网标单次号码 **USD 0.05 起**，具体国家/服务另选 | 有追加短信功能；短期确切时长按产品。另有[租赁页](https://smspva.com/rent)，支持同号延长，具体可选租期需登录核实 | 单次未收码退款窗口尚未核实；长租按具体问题处理，不能统一标自动退。新增低价单次入口及续费号码来源 |
| [SMSAK](https://sms-acktiwator.ru/ru/) | smsak.org 跳转至此；官方标 **USD 0.05/OTP 起**；邮箱验证码价格不计入 SMS 价 | 官方 OTP 窗口 **20 分钟**；单次产品，结束后能否复用同号未确认 | 官网说明未收码可取消退余额；具体窗口按产品。只录短信产品，不把其邮箱租赁归为续费号码 |
| [SV Number](https://sms-verification-number.com/en/) | 原 sms-activation-service.com 跳转至此；价目介绍 **USD 0.10/单次起**；同页 FAQ 又出现 USD 0.04 与 USD 1 起，存在冲突，最低价暂不采信 | 单次 **20 分钟**、不可延长；期限内同服务可多次短信的说明也有计费差异，按套餐确认。[租赁入口](https://rent.sms-activation-service.com/en)另收录到续费栏 | 单次官方称未收码不扣费/可取消返还；长租规则有矛盾，不套用。activation.pw 同样重定向到这一入口，不重复计数 |
| [SMSFAST](https://smsfast.com/) | [官方介绍](https://smsfast.com/about-us)标 **USD 0.21/激活起**，不是所有国家/服务统一价 | 激活 **20–90 分钟**，不可延长；长租 **1 天–1 年**，官网称可保号续费，具体 7/30 天套餐仍需确认 | 官网承诺未收码可取消退余额，但同页对“两分钟内/两分钟后可取消”描述冲突；[退款页](https://smsfast.com/refund-policy)本次仅占位内容，取消时限待核实 |
| [SMSOnline](https://www.smsonline.cloud/country/United%20States) | 官方美国页面明确免费 **0 元**；页面上的私人号 USD 0.10 起链接不当作其自营已核验报价 | 免费公共号码，在线时长/未来同号使用不保证 | 免费区官网入口；未确认固定保号或续费 |
| [Online SMS Box](https://onlinesmsbox.com/) | 官网 FAQ 说明免费 **0 元** | 共用临时号，无固定保号承诺，离线后不能恢复；**48 小时是短信保存时间，不是号码寿命** | 免费区官网入口；不标可续费 |
| [SMS4Sats](https://sms4sats.com/) | 需选国家/服务后报价；**未核验可比较的低价**，仅补充支付模式不同的候选 | [官方 FAQ](https://sms4sats.com/faq)称历史已用号码目前不能召回；单次与租赁分开，租赁时长和同号续费未核实 | 通过 Bitcoin Lightning 支付；未收到码时 hold invoice 取消释放资金，FAQ 称最多约 20 分钟；此时间是付款释放窗口，不当作号码可用时长 |

## 本轮从项目源码新增的官网入口（3 个）

详细发现链路见[面板与源码复核](panel-and-source-audit.md)。以下价格均未核实，不标“低价推荐”，也不进入精确价格排序。

| 新增入口 | 项目证据与官方核验 | 时长/复用/续费 | 退款、报价和展示 |
| --- | --- | --- | --- |
| 豪猪 | jiema-juhe 有独立适配器；[官方主页](https://my.haozhuma.com/)直接提供[网页版](https://h5.haozhuma.com/login.php)和[API 文档入口](https://www.showdoc.com.cn/haozhuma)；动态文档本次仅占位内容 | 有指定号码能力的源码线索，但官方保号时长、期限内复用、结束后复用和续费保证均未核实 | 价格/退款未知；官网导航条目，类型待核实，不因为有 getPhone 方法就宣称已可交易或保证复用 |
| E码云（eomsg） | wk 有独立 Ecode 实现；[官方主页](https://www.eomsg.com/)、[教程](https://www.eomsg.com/tutor.html)、[API](https://www.eomsg.com/api.html)可读 | 官方教程明确可以指定号码二次/多次接码；在线寿命、库存可用性、到期恢复和同号续费未知，不等于永久保号 | 价格/未收码退款未知；API 文档另有请求计费说明，不能宣传查询完全免费。教程称一般三分钟内收短信，这是到码等待说明，不是号码可用时长 |
| D1（d1jiema） | wk README 提及 D1 同模板/可配置端点，不是第三个独立适配器；[主页](https://www.d1jiema.com/)及[官方教程](https://www.d1jiema.com/tutor.html)可读 | 教程明确可以指定号码二次/多次接码；保号时长、结束后原号可用、续费、到期恢复未知 | 价格/退款未知；先作官网入口。与 E码云页面模板相近且存在兼容线索，底层供应关系未证实，不能宣称是独立号码池或直接合并为同一主体 |

## 原有 15 家资料与当前定位

| 平台 | 官方来源与当前定位 | 退款资料 |
| --- | --- | --- |
| SMS-BUS | [官网](https://sms-bus.com/)、[API](https://sms-bus.com/docs)；单次/租赁分别跳转 | [帮助](https://sms-bus.com/blog/help)：未收码退账户余额；结束后普通订单不能再次收码，特殊可复用号需另确认 |
| HeroSMS | [官网](https://hero-sms.com/cn)、[API 入口](https://hero-sms.com/api)；本项目只需官网产品信息 | [未收码取消返还政策](https://cdn.hero-sms.com/assets/pdf/return_policy_en.pdf)；不外推至所有长租 |
| 5SIM | [官网](https://5sim.net/)、[文档](https://5sim.net/docs)、[规则](https://5sim.net/rules)；已有低价来源，页面多个服务标 USD 0.01 起，国家未固定 | 取消/未完成可按规则返还余额；已收码但注册失败不等于未收码 |
| LongSMS | [官网](https://longsms.net/)、[用户入口](https://longsms.net/app)；保留信息入口，页面本次不可读 | 时长、复用、续费、退款均待核实，不因名称含 long 就认定可长租 |
| Textverified（.com） | [单次/API 介绍](https://www.textverified.com/automate)、[租赁](https://www.textverified.com/rentals)、[价格](https://www.textverified.com/services)；单次与租赁分栏 | 单次介绍承诺无短信/来电返还 credits；长租规则另核实。textverified.org 不当作同一服务 |
| GoInSMS | [官网](https://www.goinsms.xyz/)免费公开号码 | 无购买退款；不保证持续可用 |
| SMS24.me | [官网](https://sms24.me/en)免费公开收件箱 | 无购买退款；不保证之后仍能使用同号 |
| OnlineSIM | [官网](https://onlinesim.io/)、[API 总览](https://onlinesim.io/openapi_docs/Onlinesim-API-UN/info/)；免费、临时号、租赁分别展示 | 官网 FAQ：临时号未收码可关闭不扣费/余额返还；不套用长租 |
| SMSPool（.net） | [官网](https://www.smspool.net/)、[月租 FAQ](https://www.smspool.net/article/long-term-rental-frequently-asked-questions-a9841ece9324)；单次和月租分栏 | [单次指南](https://www.smspool.net/article/smspool-api-order-view-and-cancel-numbers-9883b6969fad)说明未收码返还，取消初期有锁定窗口；月租另核实；smspool.org 不混用 |
| SMS-Man | [单次](https://sms-man.com/)、[租赁](https://sms-man.com/rent)；租赁 24 小时/周/月进入续费号码栏 | [单次政策](https://sms-man.com/payment-policy)支持未收码取消返还；**租赁页明确购买后不可取消、退款或提前终止**，按租赁产品标不可退并提示最终官网确认 |
| Tiger SMS | [官网客户 API](https://tiger-sms.com/api)含报价与产品资料入口；只展示已核实产品 | [退款政策](https://tiger-sms.com/refund_policy.pdf)称未收码取消后返还；文档较旧，当前具体条件待核实 |
| GrizzlySMS | [官网文档](https://grizzlysms.com/docs)、[开发者页](https://grizzlysms.com/pt/developer-api)；保留比价与官网入口 | 原则上未送达可退，产品窗口未核实，显示有条件/待核实 |
| Quackr | [免费页](https://quackr.io/temporary-numbers)、[租号入口](https://quackr.io/rent-sms-numbers)、[API 权限](https://quackr.io/api)；免费与长租分栏 | [规则](https://quackr.io/refund-policy)：48 小时内反馈、两次更换仍失败才符合退款；已收 OTP 不退换；48 小时不是租期 |
| Receive-SMSS | [官网](https://receive-smss.com/)免费公开号码 | 无购买退款，固定时长/复用无保证 |

## 号码时长、再次使用和续费矩阵

以下按产品而非平台打标签；没有官方证据的字段明确为“未知”。同平台可在多个栏目出现，但平台数量只计一次。

| 平台/产品 | 可用时长 | 期限内再次收码 | 结束后同号再次使用 | 同号续费/到期恢复 | 栏目 |
| --- | --- | --- | --- | --- | --- |
| SMS-BUS 普通单次 | 固定窗口未确认 | 特殊复用产品另核实 | 普通订单结束后不支持 | 未确认 | 一次性 |
| SMS-BUS 租赁 | 具体天数待核实 | 未确认 | 未确认 | 未确认 | 续费号码，待核实标签 |
| HeroSMS 临时号 | 官网称从 20 分钟起，按套餐 | 未确认 | 未确认 | 未确认 | 一次性 |
| HeroSMS Full Rent | 具体天数待核实 | 长期接收能力官方有介绍 | 到期后未确认 | 未确认 | 续费号码，待核实标签 |
| 5SIM 激活 | 规则为 5–30 分钟 | 一般支持同服务，部分组合例外 | 文档有 re-buy 能力，不保证同号随时可买 | re-buy 不等于保号续期 | 一次性 |
| LongSMS | 未知 | 未知 | 未知 | 未知 | 类型待核实，不自动归续费 |
| Textverified 单次 | 准确窗口按产品，当前页面未确认统一时长 | 未确认 | 未确认，不承诺之后可复用 | 不等于长期保号 | 一次性 |
| Textverified Rentals | 官方列日/周/月等；月不当作固定 30 天 | 租期内可持续接收，按服务范围 | 到期后恢复未确认 | 页面有自动/手动/不续的选项，具体产品核实 | 续费号码 |
| OnlineSIM 临时号 | 主产品标 15 分钟，FAQ 称 10 分钟–1 小时，按实际套餐 | 同服务期限内多次 | 不保证到期后同号 | 不推断可续 | 一次性 |
| OnlineSIM 租赁 | 按实际套餐；未核实 1/7/30 天各档 | 租期内接收 | 到期恢复未确认 | 官网称可延长，具体窗口未确认 | 续费号码 |
| SMSPool 单次 | 实际订单窗口决定，统一时长未确认 | 按产品核实 | 未确认 | 未确认 | 一次性 |
| SMSPool 月租 | 月租，天数按购买页 | FAQ 限 25 条/天；Always On 持续在线，Single service 需等就绪 | 不续费到期后通常无法恢复 | 可自动续费/提前数月续费，依余额 | 续费号码 |
| SMS-Man 单次 | 官网称最长 20 分钟 | 页面标 1 条 | 未确认 | 未确认 | 一次性 |
| SMS-Man 租赁 | 24 小时/周/月；其他上限文案不同，按套餐 | 租期内不限条数的官方说明 | 到期恢复未确认 | 可购买固定租期不等于确认能保号续费，续费字段未知 | 续费号码；租赁不可退标签 |
| Tiger SMS | 时长未核实 | 未核实 | 未核实 | 未核实 | 产品类型核实后分栏 |
| GrizzlySMS | 时长未核实 | 未核实 | 未核实 | 未核实 | 产品类型核实后分栏 |
| Quackr 付费租赁 | 租号页列 1 周、1 个月、6 个月、12 个月 | 页面称租期内不限条数 | 到期恢复未确认 | 同号续费条件未确认 | 续费号码，未知续费标签 |
| SMSPVA 租赁 | 官网称最长保证 90 天，购买档位需登录确认 | 可能先激活排队；每次激活在线 10–30 分钟，不等于整个租期一直在线 | Archive 仅技术允许时恢复 | 剩余不足 7 天才可延长；恢复需补逾期天数+至少 30 天预付 | 续费号码 |
| SMSPVA 单次 | 未确认 | 有 Additional SMS 功能，范围按套餐 | 未确认 | 未确认 | 一次性 |
| SMSAK OTP | 20 分钟 | 未确认 | 未确认 | 未确认 | 一次性 |
| SV Number 单次 | 20 分钟，不可延长 | 同服务可多条但官方计费文案不一致 | 多数单次结束后不能复用 | 不支持延长该临时产品 | 一次性 |
| SV Number 租赁 | 主站称 1 天–3 个月；租赁页另称 1 周起，资料冲突 | 按租赁产品 | 过期恢复说明互相矛盾 | 支持续期的产品存在；续费窗口有 3 天/7 天矛盾，不能统一承诺 | 续费号码，资料冲突标签 |
| SMSFAST 激活 | 20–90 分钟 | 未确认 | 未确认，不承诺结束后保号 | 明确不可延长 | 一次性 |
| SMSFAST 长租 | 1 天–1 年，具体档位按官网 | 租期内持续接收能力有说明 | 到期恢复未确认 | 官方称可续费，窗口与价格待核实 | 续费号码 |
| SMS4Sats 单次 | 号码寿命未确认；20 分钟仅退款释放时限 | 按单次产品核实 | FAQ 明确目前不能召回已用号码 | 不支持召回不代表所有租赁政策 | 一次性 |
| SMS4Sats 租赁 | 官方有 Rent 模式，确切期限待核实 | FAQ 称短租可多条 | 未确认 | 未确认 | 续费号码，待核实标签 |
| 豪猪 | 官方时长未确认 | 未确认，源码指定取号不是官方复用保证 | 未确认 | 未确认 | 类型待核实 |
| E码云 | 未知；三分钟是到码等待说明 | 官方称可指定同号多次接码，取决于号码仍可用 | 未承诺结束后一定可用 | 同号续费/到期恢复未知 | 付费信息入口，租赁类型未核实 |
| D1 | 未知；三分钟是到码等待说明 | 官方称可指定同号多次接码，取决于号码仍可用 | 未承诺结束后一定可用 | 同号续费/到期恢复未知 | 付费信息入口，租赁类型未核实 |
| GoInSMS / SMS24 / Quackr 免费 / Receive-SMSS / SMSOnline / Online SMS Box / OnlineSIM 免费 | 无固定保号期限保证 | 在线时可能收到多条，但非独占 | 不保证之后仍可访问或归自己使用 | 不提供可核验的个人保号续费 | 免费区 |

矩阵的 Textverified 来源于[单次产品页](https://www.textverified.com/verifications)和[租赁页](https://www.textverified.com/rentals)；SMSPVA 租赁细节来源于[租赁 FAQ](https://smspva.com/rent)；DaisySMS 来源于[官方文档](https://daisysms.com/docs/api)；Quackr 租期来源于[租号页](https://quackr.io/rent-sms-numbers)；其余来源见各行平台的官网和规则链接。Textverified 搜索摘要曾提及短期复用，但原链接现已重定向且当前页面未能复核，故相应字段保留未知。

## 低价资料的使用边界

新确认带明确宣传起价的候选为 SMSPVA、SMSAK、SMSFAST；SV Number 虽有低起价，但同页数值冲突，暂只作价格线索。SMSOnline、Online SMS Box 作为免费来源增加；SMS4Sats 尚未证明低价，不标低价推荐。赠金和初次优惠不作为免费来源或常规价格。

续费号码展示租期总价、续费价是否已知和原币单位，不用“最低日价×天数”伪造套餐。未公开价格显示“官网选择后查看”；没有账户或 API Key 仍可收录和跳转。官方来源同页出现矛盾时标记冲突，不能通过猜测消除。

## 待核验的 3 家（不计入 24 个主入口）

| 平台 | 结果 | 处理 |
| --- | --- | --- |
| [SMSBower](https://smsbower.com/) | 首页 403，API 路径重定向异常 | 等可读官方价格与产品规则 |
| [VAK-SMS](https://vak-sms.com/) | 旧文档 404，已修正为官网，规则和价格待核实 | 等官方域名、价格和产品信息核验 |
| [SMSHub](https://smshub.org/) | getSMS 源码明确支持；官网本次首页和 /en 均 403 | 暂只作线索，不把第三方宣传起价、兼容 API 名称当作当前官方价格/规则 |

主清单收录不构成可用率或最低价保证。免费短信公开；号码租期、收码窗口、短信保存时长、付款冻结时间与验证码有效期分别记录，不能互相替代。
