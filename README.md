# identity

SMS 比价导航与多渠道临时邮箱工作区。平台名单及接入状态见 [平台清单](docs/platforms.md)。项目目录仍为 `D:\sms`，管理端程序改为 `bin/identity-admin.exe`；已安装的 `SMSNav-DailyPrices` 采集任务沿用原名称。

新增：左栏可切换 **SMS 短信 / 邮箱**。邮箱已原生内置于本机管理服务，随 `start.cmd` 启动，支持 9 个免费渠道创建收件箱、接入自己已有的邮箱（Maildrop.cc / Catchmail.io 等免密码「地址即信箱」域名，Gmail / Outlook / QQ / 163 等填 IMAP 授权码）、三栏邮件工作台、搜索与状态筛选、恢复上次选择、按渠道间隔自动收件、一键复制验证码与关闭确认，不再需要 Docker、Node 或 API Key；说明见 [邮箱功能](docs/mail.md)。

> 新对话 / 代码审查 / 内容优化请先读 [docs/PROJECT-GUIDE.md](docs/PROJECT-GUIDE.md)（含架构、命令、审查清单、优化候选）。

两个可独立构建的 Go 标准库项目：公开静态导航与本机邀请链接管理面板。用户在本站筛选资料，购买、收码、退款和续费均在对应官网完成。候选平台仅保留在目录数据中，不在页面展示。

## 界面（参考 hero-sms 风格的接码比价）

公开导航改为清晰的“选择对象 → 比较渠道 → 前往官网”三段式：顶部**可搜索、可分页的完整接码对象列表**（11,000+ 个对象，按覆盖渠道数排序、输入即筛、点击即查），单张表单按对象关键词、对象、国家、渠道、主区、付费栏目、租期、退款与排序筛选；关键词输入框带**实时自动补全**（如输入 we 优先显示 WeChat），国家名统一为**中文**（含“美国（虚拟）/美国（实体）”等线路区分）。结果为卡片式渠道报价，突出价格、可用时长、期限内再收码、同号续费、结束后复用与退款规则，限制与官方来源折叠在卡片内。

**页面为纯静态、前端按需渲染**：紧凑 JSON 内嵌，首批显示 60 张卡片，按钮追加；筛选变化后重置分页。报价卡片与筛选需要 JavaScript；禁用时显示说明及仍可访问的静态待核实平台入口。对象选择与加载更多使用原生按钮，报价弹窗支持键盘和焦点返回。

报价逐条标明具体价、公开起价、未知或需登录。同产品类型、币种、租期、单位、号码类型、适用范围及报价类型内按精确小数升降序排列；另可按渠道与条件分组。非美元报价显示“≈”参考值、原币种原价及汇率日期。“跨国起价”可能由逐国报价汇总，不代表某个国家可用。

## 接码对象列表与渠道报价

这是接码聚合渠道面板：关键词匹配**各平台提供的接码对象**，筛选“接码对象”下拉列表；渠道与国家单独筛选。对象名称从平台资料动态汇总，不再只限四项；没有价格的对象也会列出其来源渠道，同条件已知价格按升序展示。

已继续修复采集失败原因，并新增独立 [平台密钥配置](config/provider-keys.json)；支持 SMS-Man、Tiger SMS、GrizzlySMS 的只读认证价格接入，填写各平台密钥及实际原币种后生效。DaisySMS 已按官网停服公告移除；SMS-Activate、Truverifi 已移出候选，见 [网站复核记录](docs/site-availability.md)。具体见 [排查与修复](docs/collection-repairs.md)。

运行 `update-prices.cmd`：优先调用已核实的官方匿名价格 API，没有可用公开接口或接口失败时直接读取官网页面及内嵌数据。全流程不调用模型、不需要模型凭据。

当前数量、资源测量与本轮检查统一记录在 [最新验收快照](docs/verification.md#当前验收快照2026-11-07)。目录包含历史保留记录，记录总数不代表本轮成功采集数；每个来源的本轮结果见页面顶栏“采集范围与时间”。[采集完整性说明](docs/collection-coverage.md) 保留历史轮次与覆盖策略。

每 24 小时更新的任务名、使用方式、数据证据与限制见 [价格更新说明](docs/price-updates.md)。真实密钥不提交、不复制到公开产物。新增数据来自自动采集时，以每条记录的采集时间为准；下方原始目录数量描述的是采集前的人工资料基线。

## 本地启动（Windows PowerShell）

本工作区已构建导航生成器、管理员和价格更新器三个可执行文件。可直接双击 `start.cmd`，或执行 `D:\sms\start.cmd`。它不依赖 PowerShell 脚本执行策略。打开 <http://127.0.0.1:8787/>，导航位于 <http://127.0.0.1:8787/preview/>。价格采集不在管理界面触发，使用 `update-prices.cmd` 或已安装的每日任务。

重新构建时，如本机允许 PowerShell 脚本：

```powershell
Set-Location 'D:\sms'
.\build.ps1
.\start.ps1
```

构建脚本使用已安装的 Go（本机为 `D:\Go\bin\go.exe`），不下载第三方依赖；二进制输出到 `bin`，静态页面输出到 `dist`。打开管理员 <http://127.0.0.1:8787/>，导航预览为 <http://127.0.0.1:8787/preview/>。终端保持运行，Ctrl+C 停止。脚本按自身位置解析路径，启动不依赖当前目录。源码改变后重新执行 `build.ps1`。

若执行策略限制 `.ps1`，使用下方与管理端 README 的完整 CLI 命令构建，使用 `start.cmd` 启动，无需修改系统策略。

## 邀请链接

邀请管理只在独立工具中使用：运行 `D:\smsinvite\start.cmd` → 顶部“邀请链接配置” → 搜索平台 → 编辑并保存 → **更新本地导航**。需保持 `D:\sms\start.cmd` 服务运行。短信／邮箱用户页面没有管理入口，8787 的直接管理路由已关闭；根地址自动进入 `/preview/`。保存和更新是两步，生成失败保留配置与旧导航，恢复官方链接后也须更新导航。

邀请仅覆盖该平台的购买 / 主入口；产品规则来源、已有账户管理和续费入口保留官方地址。参数顺序、重复参数、编码和 fragment 不重写。域名必须精确匹配目录白名单，不接受短链、未知子域或用户名伪装；不请求输入的 URL。

管理端详情见 [admin-panel/README.md](admin-panel/README.md)。不要把管理服务部署到公网。公开托管只上传 `dist`，并配置 HTML/CSS/JS 正确的内容类型、压缩、`nosniff` 和禁止嵌入策略；公共站没有后台接口。

## 文件与命令

- `data/catalog.json`：唯一公开事实源，分别记录 providers、products、offers、candidates；管理员只读平台资料。
- `web/`：原生 HTML 模板、CSS、JavaScript；资源编译嵌入生成器。
- `admin-panel/data/referrals.json`：首次启动创建的真实私有配置，已忽略提交；只提交空的 `referrals.example.json`。
- `dist/`：仅三个公开文件。生成器拒绝覆盖含其他文件的目录；失败时保留上版。
- `bin/`：三个 Windows 可执行文件，不提交。

```powershell
Set-Location 'D:\sms'
$env:GOCACHE = 'D:\sms\.cache\go-build'
& 'D:\Go\bin\go.exe' build -trimpath -o 'D:\sms\bin\sitegen.exe' ./cmd/sitegen
& 'D:\sms\bin\sitegen.exe' -catalog 'D:\sms\data\catalog.json' -out 'D:\sms\dist'
# 使用已保存邀请配置时，必须明确传入：
& 'D:\sms\bin\sitegen.exe' -catalog 'D:\sms\data\catalog.json' -referrals 'D:\sms\admin-panel\data\referrals.json' -out 'D:\sms\dist'
```

管理员的独立构建、启动 CLI 见其 README。显式提供的邀请文件缺失、损坏或包含未知平台时，构建失败；省略 `-referrals` 才会全部使用官网入口。

发布采用同级暂存目录与备份目录替换，失败尝试回滚。**本机目录切换并非托管方的零停机发布机制**，切换瞬间预览可能暂时不可读。若进程被强制终止或断电后留下 `dist.previous`，先保留并检查该目录：如 `dist` 缺失，将其改回 `dist`；确认新产物完整后再移走旧备份。不要把输出目录设为项目目录或含其他文件的目录。

## 检查

```powershell
Set-Location 'D:\sms'
$env:GOCACHE = 'D:\sms\.cache\go-build'
& 'D:\Go\bin\go.exe' test ./...
& 'D:\Go\bin\go.exe' vet ./...
& 'D:\node.exe' --test web/app.test.cjs
Set-Location 'D:\sms\admin-panel'
& 'D:\Go\bin\go.exe' test ./...
& 'D:\Go\bin\go.exe' vet ./...
```

根模块的测试不会自动覆盖嵌套管理员模块。Node 仅用于前端自检，不是网站运行依赖。测试使用人工邀请 fixture，不验证真实平台归因。

浏览器检查、设备仿真与尚未完成的项目见 [验收记录与限制](docs/verification.md)。真实上游价格、邀请归因和真实手机性能不能由离线测试证明。

## 资料维护

事实日期以各条记录为准，重新生成不会刷新；人工规则超过 7 天、自动价格超过 24 小时显示待更新，前端在打开页面或重新渲染卡片时按当前时间复核，日期核验以 UTC 零点为界。自动采集最低价不代表精确成交价。未知价格、需登录、来源访问失败和规则冲突分开显示。一个月不当成 30 天，1/7/30 天筛选只匹配已有明确套餐；筛选可能为空。不要将免费公开号码视为私人或持续保号号码。

下列文档保留了前期调研与规划；其中“尚未实现”描述的是交接时状态，当前交付以本 README 与验收记录为准：

- [平台调研](docs/platform-research.md)：历史人工调研记录（含已移除的 E码云/D1）；当前为 22 个平台入口、3 个待核验候选。入口数量不等于独立资源池数量。
- [构建方案](docs/build-plan.md)：免费区、付费一次性号码及续费号码栏；1/7/30 天租期筛选、价格排序、官网跳转和验收条件。
- [GitHub 参考与技术取舍](docs/github-references.md)：6 个仓库的源码依据、采用与舍弃内容、许可证状态，以及 Go/Rust/静态部署比较。
- [面板与核心源码复核](docs/panel-and-source-audit.md)：2 个公开演示页面、6 个原有参考项目和 4 个补充项目/模块的检查范围；实际供应商、面板取舍和数据正确性改进。

调研日期：2026-11-06（Asia/Shanghai）。仅展示公开资料，不收集用户上游账户、余额、API Key 或验证码。日/周/月租统一放“续费号码”栏，同时明确标记是否实际支持同号续费；未知不写成支持。
