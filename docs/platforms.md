# identity 平台清单

核对日期：2026-11-07。SMS 名单来自本项目 `data/catalog.json`；邮箱为本机管理服务原生内置的 9 个免费渠道。

补充核查：LongSMS 长期套餐已接入官网匿名价格 API；短期 OTP 通过网页登录快照接入 179 条服务级起价。豪猪通过登录后的 `/v1/projects?keyword=…` 接入 6,702 个项目报价；官方 `/sms/` API 仍只提供余额与取号。详见 [两平台价格 API 核查](longsms-haozhuma-price-api.md)。下表仍反映当前目录实际接入状态。

## SMS：22 个平台入口

本站提供公开资料、报价比较和官网跳转；没有短信下单或收码 API。

| 平台 | 当前目录状态 |
| --- | --- |
| SMS-BUS | 已收录产品及报价记录 |
| HeroSMS | 已收录产品及报价记录 |
| 5SIM | 已收录产品及报价记录 |
| LongSMS | 已收录长期套餐和短期服务级起价；国家短期起价与部分项目映射待核实 |
| Textverified | 已收录产品及报价记录 |
| GoInSMS | 已收录产品及报价记录 |
| SMS24.me | 已收录产品及报价记录 |
| OnlineSIM | 已收录产品及报价记录 |
| SMSPool | 已收录产品及报价记录 |
| SMS-Man | 已收录产品及报价记录 |
| Tiger SMS | 已收录产品及报价记录 |
| GrizzlySMS | 已收录产品及报价记录 |
| Quackr | 已收录产品及报价记录 |
| Receive-SMSS | 已收录产品及报价记录 |
| SMSPVA | 已收录产品及报价记录 |
| SMSAK | 已收录产品及报价记录 |
| SV Number | 已收录产品及报价记录 |
| SMSFAST | 已收录产品及报价记录 |
| SMSOnline | 已收录产品及报价记录 |
| Online SMS Box | 已收录产品及报价记录 |
| SMS4Sats | 已收录产品及报价记录 |
| 豪猪 | 已收录网页项目报价；国家/号码类型与超长项目名仍需完善 |

“报价记录”包括免费、具体报价、起价和价格待核实等类型，不表示所有平台均有实时准确价格，也不表示本轮采集全部成功。另有 SMSBower、VAK-SMS、SMSHub 三个候选保留在数据中，不计入已展示的 22 个入口。

## 充值门槛与支付方式（2026-11-08 核对）

22 个渠道的最低充值金额与可选充值方式记录在 `data/catalog.json` 的 `deposit_note`、`payment_methods`、`payment_verified` 字段，并作为小字标注在「渠道报价」卡片右下角。免费公共号码标注“无需充值 / 免费公共号码”；起充金额只在登录后的充值页显示时标注“最低充值金额需登录查看”，不臆测数值。**起充金额需登录不等于支付方式需登录**：支付方式只在没找到官网依据时才留空，卡片这时显示“支付方式以官网注册登录后的充值页为准”并给出官网链接，不再笼统写“需登录查看”。已公开核实的关键值：5SIM 1 USD、SMS-BUS 3 USD（前端充值组件默认值）、GrizzlySMS 2.5–3 USD（按方式）、LongSMS 2 USDT、SMSOnline €10、Online SMS Box 10 TL。主要官方依据：GrizzlySMS FAQ（最低 $3，银行卡/电子钱包/加密货币）、5SIM 支付页（金额不少于 1 USD）、SMS-BUS 充值页脚本（minAmount 3）、LongSMS `/api/site-content`（min_usdt 2）、SMS4Sats FAQ（仅 Lightning、无账户余额）。

2026-11-08 复核（本轮修复“部分平台最低充值与支付方式都写需登录查看”）：逐个打开官网重查，只有起充金额需要登录，支付方式多数可以公开核实，已按官网改写并补充来源。

| 平台 | 最低充值 | 支付方式（官网公开依据） |
| --- | --- | --- |
| SMS-Man | 登录后查看 | [支付政策](https://sms-man.com/payment-policy)只写银行卡（含 Verified by Visa / SecureCode 验证）与退款原路退回；首页与 `/payment` 公开页均为登录页（403/跳转），不列支付组合 |
| Tiger SMS | 登录后查看 | [`/payments`](https://tiger-sms.com/payments)公开列出 PayPal、Alipay、CoinBase、WeChat pay、UnitPay、USDT、AdvCash、Payeer、银行卡、SBP、CryptoCloud、FreeKassa；[FAQ](https://tiger-sms.com/faq)称充值方式以该页为准 |
| SV Number | 登录后查看 | [FAQ](https://sms-verification-number.com/en/faq)说明充值页自选支付方式、加密货币走 Heleket / 2328、支付宝付款后需激活优惠码；首页写 Visa / Mastercard / Crypto |
| SMSFAST | 登录后查看 | 首页公开列 Visa / Mastercard / Crypto（Bitcoin）；[FAQ](https://smsfast.com/faq)只讲到账处理，不提支付宝，已移除原“支付宝” |
| SMSPVA | 登录后查看 | [FAQ](https://smspva.com/help.html)写 Bitcoin、Webmoney、Yandex money、信用卡，余额页列出全部方式；原“Verve”无官网依据，已移除 |
| SMSAK | 登录后查看 | 官网只写 USDT（TRC20 / ERC20），其余为登录后界面，不补充 |

Textverified、SMSPool 与豪猪仍标“需登录”：前两者公开 FAQ 只有支付方式没有起充金额，豪猪官网没有任何公开充值/支付说明。

OnlineSIM 复核：已移除未找到官网依据的“手机话费余额”。[官网使用说明](https://onlinesim.io/ru/free_numbers/georgia)列出 Visa/Mastercard、Alipay、GCash、Payeer、Perfect Money；[支付条款第 4.1 节](https://onlinesim.io/documentation/offer)另说明银行转账和平台提供的数字资产支付。[充值操作说明](https://onlinesim.io/instructions/how-to-create-wechat-account-without-phone-number-onlinesimio)要求登录后从 Balance → Refill 选择方式和金额。未查到公开的最低充值数值；访问 `/v2/en/payment` 跳转到登录页后返回 403，因此页面明确标为“最低充值金额未公开核实”，不把推广提现门槛或号码单价当作充值门槛。已将这三个官方依据加入渠道 `sources`。

## 邮箱：本机管理服务原生提供的 9 个免费渠道

创建收件箱时可指定渠道与域名；自动调度按下列固定顺序尝试，渠道不可用时切换到下一个。

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

渠道可用性由上游决定，可能随时变化。使用方式、本地 API 与限制见 [邮箱功能](mail.md)。

另有「自定义邮箱（IMAP）」：不创建临时地址，而是把**自己已有的邮箱**（Gmail、Outlook、QQ、163、iCloud 等）按地址 + 应用专用密码接入收件，不参与上面的自动调度。
