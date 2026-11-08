# 面板、供应商与核心源码复核

当前目录已清理不可用站点，保留 24 个主渠道、3 个候选；本文原数量为历史资料，以 [最新复核记录](site-availability.md) 为准。

日期：2026-11-06（Asia/Shanghai）。本轮延续官网比价/跳转范围，检查原有 6 个参考项目，并补读 4 个 SDK/相关模块。参考结论已经合入构建方案；未复制第三方代码。

## 检查深度

- **公开页面实看**：[Homer 演示](https://homer-demo.netlify.app/)读取页面结构并查看截图；[WebStack-Hugo 演示](https://iplaycode.github.io/nav/)查看截图。后者文字快照因工具网络错误未完成，截图读取成功。演示部署不保证与本次读取的 GitHub 提交完全一致。
- **面板源码检查**：StroApp 首页、国家报价列表、价格页、新租赁与租赁列表；jiema-juhe 用户项目面板、HeroSMS 服务查询、管理员概况；Homer 通用卡片/搜索/连接状态；Flare 首页模板；WebStack 数据和首页模板。
- **业务源码检查**：供应商注册、报价合并、渠道工厂、豪猪实现、能力接口、E码云读请求、错误处理和产品模型。
- **未检查**：没有安装或运行参考仓库，没有登录私有面板，没有购买号码、充值或执行供应商 API；没有作全仓安全审计。FastRecvSMS、getSMS、sms-toolkit、saucesteals/sms 是 CLI/SDK，不冒充有可评审的 Web 面板。wk 只读取与供应商发现直接有关的文件，不评审其主业务。

## 从实际源码提取的平台

下表区分注册/导出的实现、README 计划和兼容配置，不把“coming soon”当成已接入。

| 参考项目 | 直接证据 | 供应商结果与本站处理 |
| --- | --- | --- |
| StroApp | [router.py](https://github.com/omermask/stroapp-sms/blob/e507975c01e32b14a50dc4442d114faf904b681d/app/infrastructure/providers/router.py)导入并实例化 4 类 | SMS-Man、5SIM、SMS-Activate、SMSPool；前三个现有有效入口/候选状态沿用，不新增重复平台 |
| FastRecvSMS | [providers/__init__.py](https://github.com/momenbasel/FastRecvSMS/blob/5831219b8cc0efea5b2819c62d4a1afa8455c3af/fastrecvsms/providers/__init__.py) | 5SIM、SMS-Activate 两项；并非接入所有兼容 handler API 的平台 |
| jiema-juhe | [factory.py](https://github.com/huluobo678/jiema-juhe/blob/867e9a96e673e883d7cf4197d1c7b12b966c363f/channels/factory.py)、[haozhuma.py](https://github.com/huluobo678/jiema-juhe/blob/867e9a96e673e883d7cf4197d1c7b12b966c363f/channels/haozhuma.py) | HeroSMS、豪猪；豪猪核验官方入口后新增。架构 SPEC 提到的其他备用渠道不算实现 |
| getSMS | [src/index.js](https://github.com/Viiprogrammer/getSMS/blob/8b49bbc4881e2c66a72118a7a4c0ac51b29a54c6/src/index.js)的 supportedMethods 和构造器 | SMSHub、SMS-Activate；SMSHub 官网本次 403，只增待核验线索，不编造价格或可用状态 |
| sms-toolkit | [README](https://github.com/nsigel/sms-toolkit/blob/65b425e2352ee1fa58faffb694ffcffd81669226/README.md)、[导出](https://github.com/nsigel/sms-toolkit/blob/65b425e2352ee1fa58faffb694ffcffd81669226/src/index.ts)、[Truverifi 实现](https://github.com/nsigel/sms-toolkit/blob/65b425e2352ee1fa58faffb694ffcffd81669226/src/truverifi/index.ts) | Truverifi 有实现；Textverified、SMSPVA 在 README 标 coming soon，不能据此确认完整可用。Truverifi 官网资料老旧/访问不稳、政策有冲突，暂列待核验 |
| saucesteals/sms（Go） | [代码目录](https://github.com/saucesteals/sms/tree/f4bd22b4dabe06216ea2f6d08c5800ab77afe6e7)、[sms.go](https://github.com/saucesteals/sms/blob/f4bd22b4dabe06216ea2f6d08c5800ab77afe6e7/sms.go) | 有 DaisySMS、SMS-Man、SMSPool、SMSPVA 包；均已收录。复用能力作为额外接口，适合参考“并非所有供应商都有同一能力” |
| wk 的接码模块 | [ecode.go](https://github.com/gao-gao-zai/wk/blob/849724f6b39aa68f1092b15595821f0eb98ad610/internal/haozhuma/ecode.go)、[README 供应商说明](https://github.com/gao-gao-zai/wk/blob/849724f6b39aa68f1092b15595821f0eb98ad610/README.md) | 豪猪、E码云有对应实现；D1 是同模板/可配置端点说明，不是第三套已独立验证的适配器。E码云与 D1 经官方入口核验后新增导航条目，底层资源关系未知 |
| Homer / Flare / WebStack | 导航配置与通用模板 | 不是接码供应商实现，不从其应用图标数量推算可接入短信平台 |

结果：原 22 家导航主清单新增 **豪猪、E码云、D1 三个入口**，现为 **25 个平台入口**；SMSHub、Truverifi 加入待核验，连同原 3 家共 **5 个候选**。入口数量不等于独立号码资源池数量。

## 面板中采用的内容

| 所见面板/源码 | 值得保留的设计 | 本站具体改法 |
| --- | --- | --- |
| Homer 公开演示及 [Generic.vue](https://github.com/bastienwirtz/homer/blob/51bc3f4fc5035e08531e75513ba75dce4409b36a/src/components/services/Generic.vue) | 分组卡片、名称/简述/标签、明确的外部入口；状态与卡片信息分开 | 首页先显示平台和产品关键信息，主按钮写“前往官网购买”。卡片不整块自动跳转，便于选中文字、展开规则及访问证据链接；不把标签点击也做成购买跳转 |
| WebStack 公开演示和[首页模板](https://github.com/iplaycode/webstack-hugo/blob/5e85297db430079c468dd33e78a945556973c73a/layouts/index.html) | 侧栏定位分类，卡片密集但容易扫读 | 桌面按免费/付费定位，付费分一次性/续费；手机改顶部栏目。重要规则完整显示或由 details 展开，不能像普通导航简介一样只剩省略号；减少首屏大横幅和无关搜索引擎 |
| StroApp [国家报价页](https://github.com/omermask/stroapp-sms/blob/e507975c01e32b14a50dc4442d114faf904b681d/stroapp-sms-user/lib/features/sms_purchase/screens/countries_screen.dart) | 服务→国家选择，搜索国家名/代码，价格与数量相邻 | 本站筛选国家/服务/租期后展示各供应商同条件报价；有明确证据的库存才显示数值，未知不能当 0，历史数量带核验时间 |
| StroApp [新租赁页](https://github.com/omermask/stroapp-sms/blob/e507975c01e32b14a50dc4442d114faf904b681d/stroapp-sms-user/lib/features/rentals/screens/new_rental_screen.dart)、[租赁列表](https://github.com/omermask/stroapp-sms/blob/e507975c01e32b14a50dc4442d114faf904b681d/stroapp-sms-user/lib/features/rentals/screens/rentals_list_screen.dart) | 国家变化后清除旧服务选择；租赁单独页面；区分空列表与加载失败 | “续费号码”独立栏目，提供 1/7/30 天和原月单位；联动只提供实际存在组合。保留错误/无匹配的区别；不展示用户订单、实际号码、个人倒计时和可操作自动续费开关 |
| jiema-juhe [HeroSMS 查询页](https://github.com/huluobo678/jiema-juhe/blob/867e9a96e673e883d7cf4197d1c7b12b966c363f/templates/admin/herosms_services.html) | 国家/名称搜索，原价、折算价、库存同表展示 | 原价是主要信息；折算价仅有明确汇率来源/时间时出现。服务代码留在展开资料内，主界面用人可读服务名；不搬管理员“创建项目”动作 |
| jiema-juhe [概况](https://github.com/huluobo678/jiema-juhe/blob/867e9a96e673e883d7cf4197d1c7b12b966c363f/templates/admin/dashboard.html) | 汇总数字和渠道状态表，附时间 | 本站最多显示平台入口数、匹配产品数和资料核验时间；不显示虚构的余额、订单数、实时渠道健康或成功率。价格错误和官网访问异常分别说明 |
| Flare [首页模板](https://github.com/soulteary/flare/blob/cf98ff32d3226784adc3030de0d91d07333831df/embed/templates/home.html) | 搜索有标签、无结果提示、清除入口、分类资料错误提示与文本状态 | 采用原生控件、可见标签、结果计数、清空筛选、键盘焦点；无 JS 仍可浏览外链。静态页面不用 POST 搜索与服务端会话 |

## 从核心代码识别的优化点

以下是针对所读文件的改进依据，不是对参考仓库提交完整代码审查结论。

1. **保留报价来源，不在目录层只保留最低一家。** StroApp router 按 service_id 合并并覆盖为低价记录，这会丢掉其他来源，也不足以表达本站的币种、租期、类型和退款条件。本站保留所有合格 offer，使用自己明确维护的服务映射，组内比较；不能认为不同平台同名或同代码必然同一产品。
2. **失败、缺失、真实零价分开。** StroApp 基类用极大值表示价格失败，部分面板缺价显示 0，国家页报价失败被忽略。本站使用 nullable 金额和显式状态，显示“价格未知/来源更新失败/已过期”，绝不把它们变成免费、无库存或最低价。
3. **退款和能力以官方产品规则为准。** sms-toolkit 的 Truverifi cancel 方法是空实现；saucesteals 的 PhoneNumber.Reuse 只是本地状态复位，都不能证明上游退钱、保号或续费成功。本站保留事实证据级别，SDK 方法名不作为用户承诺。
4. **新增平台不用名称子串猜类型。** jiema-juhe factory 有按名称含 hz 等匹配的分支；本站用稳定平台 ID、已核验域名和明确产品 ID，避免误识别。别名只帮助搜索，不参与猜测渠道/规则。
5. **外部状态不能用浏览器网络状态代替。** Homer ConnectivityChecker 检查本站连通性而非所有外链；本站不据此给供应商显示绿灯。官网 HTTP 可访问、价格成功核验、库存和收码成功率是不同事实，不能互推。
6. **DOM 与事件处理保持安全、简单。** jiema 管理面板通过字符串拼接 innerHTML；本站用模板转义和 textContent 更新公开资料。Homer 搜索源码绑定后的回调与移除时的引用不同，本站如使用事件监听要保存同一引用并避免重复初始化；不复刻框架生命周期。
7. **查询失败保留正确的旧数据。** E码云 Go 实现区分网络/限流错误和业务错误，限制响应读取量；未来只读资料更新可参考该思路，同时显式拒绝超限/截断响应。更新失败不覆盖上次有效快照，不刷新事实核验日期，不自动请求购买接口。
8. **保持静态部署的边界。** 面板轮询余额、自动切换供应商、自动购买和验证码流不属于本站；不会为借鉴界面而引入这些业务或它们的运行时依赖。

## 验收补充

- 主清单 25 个入口，候选 5 个分别计数；同一来源重复域名/别名不增加独立平台统计，E码云/D1 的底层关系标未知。
- 提供无价格、来源访问失败、规则冲突、已过期、未知库存和真实免费产品的示例数据，检查文本和排序不同。
- 切换国家后，明确清除失效服务并告知用户；已选租期无匹配时保留筛选并说明原因，不能悄悄跳到别的天数。URL、页面状态、前进后退一致。
- 桌面和手机都能完整读到退款/时长/复用结论；需要展开的条件可用键盘操作，主要风险不藏在 hover tooltip 中。
- 主外链与规则详情、来源链接各自独立；任何报价状态下都不误触发购买。
- 构建数据校验、精确排序和筛选行为仍由最小必要测试覆盖；本轮只完成检查和规划，没有执行未来实现的验收测试。
