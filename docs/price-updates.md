# 接码目录、公开价格与每日更新

当前目录为 **22 个主渠道、3 个候选**（2026-11-07 按用户要求移除 E码云、D1）；详情见 [网站可用性复核](site-availability.md)。下文保留此前排查过程。

运行 `D:\sms\update-prices.cmd`。更新器先读已核实的官方匿名 API；没有可用接口、接口出错或没有价格时，抓取官网页面及内嵌 JSON。对象进入下拉列表，目录与报价分别统计。

最新一轮核验 **29,095 条报价**，目录共 29,570 条报价、30,489 条渠道对象记录、17,394 个归一对象。网页登录快照贡献 6,881 条（LongSMS 短期服务级起价 179 条，豪猪项目报价 6,702 条）。OnlineSIM 完成 98/98 个国家，SMSPVA 完成 69/69 个国家，LongSMS 长期当前 24 条有库存套餐。SMS4Sats 全目录 802 项中，本轮完成询价 310 项，277 项有价、33 项暂无库存，492 项仍未完成；其余缺口没有伪装成已完成。见 [采集范围说明](collection-coverage.md)。

最新各来源排查、停服公告及认证渠道填写方式见 [采集修复说明](collection-repairs.md)。

## 配置

编辑 `config/price-update.json`，保留现有 `sources`。真实配置不提交、不复制到 `dist`。

来源顺序按“快速官方 API 优先、慢或部分来源最后”排列（5SIM、SMS-BUS、HeroSMS、Textverified、Tiger、Grizzly、OnlineSIM、SMSPool、SMS-Man、SMSPVA、SMSAK、SV Number、LongSMS、SMSFAST、SMS4Sats）。SMS4Sats 逐对象询价且常被限流，放最后，避免挤占其它来源的 25 分钟预算。

LongSMS 已接入官网匿名长期套餐 API：读取 `/api/countries`，再按国家读取 `/api/pricing?country_id=…&product_line=long`，只发布有库存国家的 `active` 套餐。报价金额已加上接口返回的 `first_activation_fee`；项目代码 `other`、`hosting_other`、`hosting_full`、`full_pack` 原样保留，不映射成 Google 等具体对象；最终租期截止和续费规则仍待核实。`/app/shop` 的短期 OTP 目录与报价需要网页登录态，当前未接入，因此 36 条长期套餐不等于整个 shop 的完整报价。

共享公开收件箱（GoInSMS、SMS24、Quackr、Receive-SMSS、SMSOnline、Online SMS Box）与豪猪不放入付费价格采集 `sources`；豪猪官方 API 只有登录、余额和取号，没有价格查询接口。它们仍在目录与免费/待核实区展示，不重复显示为“价格采集失败”。

SMSFAST 没有匿名价格 API：采集器读取官网公开起价（当前 “From $0.21”）作为产品级起价，写入证据与采集时间；这不是逐对象价格。

价格采集不在管理界面触发；使用 `update-prices.cmd`、`watch-prices.cmd` 或已安装的每日任务。公开部署的 `dist` 与管理端预览都不含采集按钮。

- `provider_keys`：独立平台密钥文件，相对本配置目录解析；真实文件不提交。
- `web_prices`：网页登录快照路径；不存在或超过 48 小时时跳过，不阻塞公开 API 采集。
- `price_targets`：重点对象保留分国家报价；SMS4Sats 对全目录询价，该字段仅影响优先顺序。Textverified 的详情页暂仍按重点对象读取。「其他/未列出 (Any Other)」不受该列表限制，始终保留分国家报价。
- `sources`：已核实的官方来源及原币种。不同币种不换算排序，不猜汇率。

公开价格请求不携带任何模型密钥，不登录、充值、购买或拉取短信内容；网页快照脚本只复用人工登录的本地 profile，不代取号、不代付款。

## 平台密钥获取入口（只读价格）

认证价格接入支持 5 个平台，密钥填入 `config/provider-keys.json`（示例 `config/provider-keys.example.json`），同时必须填写该平台实际报价原币种（如 USD / RUB），程序不猜币种。

> **SMS-Man 无需付费密钥**：本轮已改为读取官网公开站点的只读价格接口（见 [采集修复](collection-repairs.md)），无需账户余额 >13$，直接从 180 个国家取 6000+ 条 USD 报价。`config/provider-keys.json` 里的 `sms-man` 留空即可。

| 平台 | 在哪里获取 API Key | 官方 API 文档 |
|---|---|---|
| SMS-Man | 无需付费密钥（读取官网公开站点接口）；旧账户 control Token 亦可 | [api 介绍](https://sms-man.com/blog/how-to-use-our-api-introduction) |
| Tiger SMS | 注册/登录后在个人中心 API 区 | https://tiger-sms.com/api |
| GrizzlySMS（灰熊） | 登录后在设置 → API | https://grizzlysms.com/cn/docs/partner |
| HeroSMS | 登录后在个人中心 API 区 | https://hero-sms.com/api |
| SV Number（SmsVerificationNumber） | 登录后在 API 设置页（兼容 sms-activate 协议） | https://sms-verification-number.com/en/api-sms-activate/ |

另有 **SMSFAST** 支持 API 但程序尚未接入认证采集，其密钥在「个人中心 → API」获取，文档见 https://smsfast.com/faq/api 。Textverified 需登录，本轮未接入价格。

以上外链为各平台官方页面；注册、充值、取号仍在对应平台官网完成，本站只读取目录与价格。填入真实密钥后先运行 `update-prices.cmd` 验证，再依赖每日任务。

## 数据口径

报价保存来源、时间、金额、证据和读取方式。金额用十进制精确比较，排除明确无库存的价格。对象只按明确同名或已维护别名合并，不按名称相似猜测。

聚合保存每个对象/渠道的最低公开价，标为起价；号池、号码类型未提供时显示未知，不当作锁价或相同号码成交价。5SIM、SMSPool 保存全局起价和重点对象（及「其他/未列出 (Any Other)」）的国家最低价；OnlineSIM 已按公开启用国家目录逐国分页；SMSPVA 也按公开国家目录读取价格与库存。SMS-BUS 使用零售 `cost`，不使用批发 `ws_cost`。

页面文字仅匹配本渠道有可靠来源的对象，不把国家起价套给其它渠道同名对象。短信示例不得作为目录或价格证据。截断 JSON 的价格整批丢弃，独立取得的完整目录可以保留。单独登记的 `api_hosts` 不放宽邀请链接白名单。

## 每 24 小时更新

现有任务 **SMSNav-DailyPrices** 已安装，周期 P1D，使用更新后的 `bin/priceupdate.exe`；当前用户需登录。查看：

```powershell
Get-ScheduledTask -TaskName 'SMSNav-DailyPrices'
Get-ScheduledTaskInfo -TaskName 'SMSNav-DailyPrices'
```

立即更新运行 `update-prices.cmd`。没有系统任务时可用 `watch-prices.cmd`，两者不要同时启用。安装脚本是 `enable-price-updates.ps1`。

## 网页登录价格快照

LongSMS 的 `/app/shop` 短期 OTP 和豪猪的 `/v1/projects?keyword=…` 需要网页登录态，公开 API 取不到。`tools/web-prices.mjs` 用本机 Chrome 的独立 profile 保存登录态，不保存密码或 Cookie 到仓库：

```powershell
# 首次：弹出可见 Chrome，手动登录两个平台（含验证码）
D:\node.exe tools\web-prices.mjs --login
# 看到“已写入”后再关闭 Chrome；之后 priceupdate 会自动以 --auto 复用 .cache/web-prices-profile
```

脚本把规范化报价写入 `data/web-prices.json`（已 gitignore）；`priceupdate` 读取 `web_prices` 配置并在公开 API 采集前合并。LongSMS 短期当前只取 `browse/services.price_usd` 的**服务级起价**（179 条有价服务），国家起价尚未接入自动流程。豪猪用 `/v1/projects?keyword=…` 的两位字母/数字关键词扫描去重（当前 6,702 条），中文-only 名称可能缺失。快照超过 48 小时不合并；没有登录态时脚本直接跳过，不影响公开 API 采集。验证码和 Turnstile 必须由人完成，脚本不绕过。

任务上限 30 分钟，网络总预算 25 分钟，页面请求超时 25 秒；普通 API 流程上限 3 分钟；OnlineSIM、SMSPVA、SMS4Sats 批量采集上限 6 分钟；SMSAK 的完整大响应单独使用 HTTP 6 分钟、流程 7 分钟。失败保留历史数据及原时间，不伪装成新报价。用户取消在提交前再次检查，不提交新目录；预算耗尽仍保存已取得的结果及未完成状态。写入前生成 `data/catalog.json.backup`，检查并发修改；目录上限 32 MiB。目录已保存但导航生成失败时，修复输出问题后单独重生成，不回滚新价格。非 watch 模式退出码 3 表示该阶段失败，4 表示全来源失败状态已保存且导航已生成，其他失败为 1 或参数错误 2。进程意外中止留下锁时，确认没有更新/生成进程，再移走锁。

## 构建与验证

```powershell
$env:GOCACHE = 'D:\sms\.cache\go-build'
& 'D:\Go\bin\go.exe' -C 'D:\sms' build -trimpath -o 'D:\sms\bin\priceupdate.exe' ./cmd/priceupdate
```

完整构建见根目录 `build.ps1`。根模块和管理员分别运行 `go test ./...`、`go vet ./...`，前端运行 `D:\node.exe --test web/app.test.cjs`。

已实际拉取公开数据并运行自动测试。认证接口已用模拟 transport 验证，尚未用真实平台密钥联调；浏览器、移动端和大目录性能未实测。静态 HTML 约 37 MB，屏幕外卡片延后绘制；公网托管需启用压缩。
