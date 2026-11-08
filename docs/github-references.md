# GitHub 相似项目调研与技术取舍

核验：2026-11-06（Asia/Shanghai）。搜索范围包括 `sms verification aggregator`、`sms price comparison`、`接码 聚合` 和静态导航项目；通过 GitHub 连接器读取仓库信息、目录、README 和相关源码。以下是局部设计参考，不是对整个项目安全性或生产质量的背书；没有运行这些项目。

后续已补查公开面板、实际供应商注册和更多核心源码，见[面板与源码复核](panel-and-source-audit.md)。该文区分公开演示实看、源码检查、计划功能和待核验的平台。

## 筛选结论

本轮没有确认一个能直接满足“官方价格比对 + 免费/付费 + 号码时长/复用/续费 + 官网购买跳转”的现成项目。短信业务仓库适合参考平台字段和映射，导航仓库适合参考数据维护和页面组织。当前项目仍宜小规模独立实现，不整体 fork 一个交易平台。

GitHub 搜索中的年份价格对比仓库、推广清单和 README-only 项目，仅作为线索，不将其宣传价格当作官方证据。VerifyHub 实际是 Android/Xposed 验证码聚合模块，与本站范围不符，排除。`WebStackPage/WebStack-Hugo` 返回 404，最终核验的是下面真实可读的 `iplaycode/webstack-hugo`，不使用错误地址。

## 有参考价值的 6 个仓库

| 仓库、所读文件 | 值得参考的具体内容 | 本项目应用与边界 |
| --- | --- | --- |
| [bastienwirtz/homer](https://github.com/bastienwirtz/homer)：[配置](https://github.com/bastienwirtz/homer/blob/51bc3f4fc5035e08531e75513ba75dce4409b36a/public/assets/config.yml.dist)、[搜索组件](https://github.com/bastienwirtz/homer/blob/51bc3f4fc5035e08531e75513ba75dce4409b36a/src/components/SearchInput.vue) | 分组和条目由配置驱动；搜索状态写入 URL；有键盘操作；构建结果可静态托管 | 采用数据/模板分离、URL 筛选、清晰外链。短信比价需要另加结构化产品与报价，不能只用普通书签字段；无需引入 Vue/PWA/Smart Cards |
| [soulteary/flare](https://github.com/soulteary/flare)：[README](https://github.com/soulteary/flare/blob/cf98ff32d3226784adc3030de0d91d07333831df/README.md)、[书签模型](https://github.com/soulteary/flare/blob/cf98ff32d3226784adc3030de0d91d07333831df/config/model/bookmark.go)、[go.mod](https://github.com/soulteary/flare/blob/cf98ff32d3226784adc3030de0d91d07333831df/go.mod) | Go、少量原生 JS、无数据库、单可执行文件；书签和分类分开建模；性能文档说明测量口径 | 采用低依赖和可复核测量的思路。本项目优先构建后退出，不必常驻 HTTP 服务；不随之引入 Fiber、编辑器、账号、天气等功能 |
| [iplaycode/webstack-hugo](https://github.com/iplaycode/webstack-hugo)：[数据](https://github.com/iplaycode/webstack-hugo/blob/5e85297db430079c468dd33e78a945556973c73a/exampleSite/data/webstack.yml)、[模板](https://github.com/iplaycode/webstack-hugo/blob/5e85297db430079c468dd33e78a945556973c73a/layouts/index.html) | 独立数据文件生成静态分类导航；维护者修改数据即可增删入口 | 采用静态生成和分组布局。所读模板带旧版 jQuery/Bootstrap、统计脚本和禁缩放设置，本项目不原样搬运；保留浏览器缩放和键盘可访问性 |
| [momenbasel/FastRecvSMS](https://github.com/momenbasel/FastRecvSMS)：[协议边界](https://github.com/momenbasel/FastRecvSMS/blob/5831219b8cc0efea5b2819c62d4a1afa8455c3af/fastrecvsms/providers/base.py)、[5SIM 实现](https://github.com/momenbasel/FastRecvSMS/blob/5831219b8cc0efea5b2819c62d4a1afa8455c3af/fastrecvsms/providers/fivesim.py) | 平台各自实现服务目录、国家映射和响应解析；明确区分供应商 | 以后确需官方只读价格采集时再参考映射边界。它的购买、轮询和 Key 管理不属于本站；所读实现使用 float、缺价默认 0，不能照搬为比价逻辑 |
| [omermask/stroapp-sms](https://github.com/omermask/stroapp-sms)：[基础模型](https://github.com/omermask/stroapp-sms/blob/e507975c01e32b14a50dc4442d114faf904b681d/app/infrastructure/providers/base.py)、[项目说明](https://github.com/omermask/stroapp-sms/blob/e507975c01e32b14a50dc4442d114faf904b681d/README.md) | 服务报价模型区分服务 ID、价格、数量；有 rental 能力标识；README 描述多供应商交易系统 | 只参考产品维度和能力显式声明。其 get_price 在失败时返回 999999，本站改为未知/失败状态；不引入余额、加价、PostgreSQL、Redis 或 Flutter。README 功能表不等于全部实现已验证 |
| [huluobo678/jiema-juhe](https://github.com/huluobo678/jiema-juhe)：[渠道源码](https://github.com/huluobo678/jiema-juhe/blob/867e9a96e673e883d7cf4197d1c7b12b966c363f/channels/base.py)、[架构规划](https://github.com/huluobo678/jiema-juhe/blob/867e9a96e673e883d7cf4197d1c7b12b966c363f/SPEC_ARCH.md) | 支持能力显式说明，默认不支持指定号码；渠道故障状态独立记录；SPEC 提出健康检查和限流 | 采用“未知/不支持不要伪装成功”的边界；SPEC 只视为方案，不能当作实现证据。静态导航不需要渠道调度、取号并发槽位和交易熔断 |

## 维护状态和复用边界

查询时上述 6 仓库均未标 archived。读取的默认分支提交日期分别为：Homer 2026-11-02、Flare 2026-11-01、WebStack-Hugo 2022-11-14、FastRecvSMS 2026-04-10、StroApp 2026-06-27、jiema-juhe 2026-07-13。提交时间不代表上游平台 API 或价格仍然有效，平台事实仍回到官方资料核实。

GitHub 元数据识别到 Homer 为 Apache-2.0、Flare 为 AGPL-3.0、WebStack-Hugo 与 FastRecvSMS 为 MIT；StroApp、jiema-juhe 元数据没有识别到许可证，所读根目录也未见 LICENSE。公开可读不作为可任意复制的依据。本轮只提取设计思路，没有复制源代码或资源；若之后复用代码，逐文件确认许可与署名要求。

## 语言与运行方式比较

以下是针对本项目的工程判断，不是语言性能排名，尚无本项目压测结果。

| 方案 | 与速度/占用相关的特点 | 选择 |
| --- | --- | --- |
| Go 构建工具 + 静态 HTML/CSS/原生 JS | 编译工具完成校验和生成后退出；线上无常驻应用进程。标准库足够处理 JSON、模板、精确价格比较 | **首选**；兼顾实现量、可移植性和部署成本 |
| Rust 构建工具 + 静态页面 | 官方强调无 GC 和内存效率；静态成品的线上资源占用并不会仅因生成器改用 Rust 而降低 | 暂不选择；仅在实测构建瓶颈或明确硬资源上限出现时重新评估 |
| Hugo + 定制主题 | Go 实现的现成静态生成器；适合内容页，支持数据和模板 | 若未来扩展成大量独立平台介绍/教程页再考虑；当前多维价格校验仍需定制，先用 Go 标准库减少工具链 |
| Go 常驻 Web 服务 | 可将资源打包为单文件并做动态读取，但仍有进程和 HTTP 服务维护 | 只有未来需要服务端实时只读报价或特殊自托管要求时再增加 |
| Node/Next.js 或 Python/FastAPI 常驻服务 | 能实现功能，但本站当前没有需要常驻业务后端的流程 | 当前不用；不把它们的“常驻服务开销”误说成所有静态构建工具都必须承担 |

依据：[Go Web 标准库示例](https://go.dev/doc/articles/wiki/)、[html/template](https://pkg.go.dev/html/template)、[encoding/json](https://pkg.go.dev/encoding/json)、[math/big](https://pkg.go.dev/math/big)、[Go GC 说明](https://go.dev/doc/faq#Why_GC)、[Rust 官方性能说明](https://rust-lang.org/)、[Hugo 功能](https://gohugo.io/about/features/)。Go 自身有运行时和 GC，不宣称其绝对内存占用小于 Rust。首选静态部署是由本项目的只读导航范围决定的。

## 转化为本项目的 7 个决策

1. 一个 JSON 目录作为事实来源，Go 强类型结构校验；当前规模不引入 YAML 库或数据库。
2. 平台、产品、报价分开表达，价格必须绑定准确国家/服务/租期组合，平台支持很多国家不代表起价适用于每个国家。
3. Go 生成完整 HTML，原生 JS 只增强筛选、URL 状态和可比组内的排序；无 JS 时仍能浏览和跳转官网。
4. 起价、精确报价、未知价和失效资料分别处理，不用 0 或极大数字伪装报价。
5. 先人工维护已核验资料，自动化只读采集有实际需求且符合官方访问条件时再加；更新失败保留最后成功数据并显示过期。
6. 不因类似项目具有充值/订单/收码而扩展本站范围，也不因它使用某框架就引入该框架。
7. 用相同目录与设备测首屏字节、筛选耗时、构建时间/峰值内存；只有瓶颈明确后再优化或换语言。
