# identity · 本机短信与邮箱服务

导航的邮箱工作区由本服务原生提供，随本服务启动，访问 `/preview/#mail`。功能、渠道、数据与接口边界见 [邮箱功能](../docs/mail.md)。不需要 Docker、Node 或环境变量。

独立 Go module，仅使用标准库；读取根项目 `data/catalog.json` 中的平台，不维护第二份平台名单。只监听 `127.0.0.1:8787`，请使用该地址，`localhost` 不在 Host 白名单。

## 启动

已构建时直接双击根目录 `start.cmd`，或在 PowerShell 执行 `D:\sms\start.cmd`。当前目录不影响文件落点。

若本机允许 PowerShell 脚本，也可执行根项目脚本：

```powershell
& 'D:\sms\build.ps1'
& 'D:\sms\start.ps1'
```

打开 <http://127.0.0.1:8787/> 自动进入 `/preview/` 短信／邮箱工作区。终端保持运行，Ctrl+C 停止。

邀请管理已从用户界面关闭。链接编辑、恢复、备份和生成导航仅通过 `D:\smsinvite\start.cmd` 启动的私有工具进入；点击顶部“邀请链接配置”。原 8787 的 `/save`、`/restore`、`/build`、`/backup` 等路由没有私有通道凭据时返回 404。服务启动后在 `admin-panel/data/.invite-admin.key` 保存本次通道密钥，仅独立工具读取并转发；每次重启轮换，不写入公开页面。该文件不能发布。

也可独立构建：

```powershell
Set-Location 'D:\sms\admin-panel'
$env:GOCACHE = 'D:\sms\.cache\go-build'
& 'D:\Go\bin\go.exe' test ./...
& 'D:\Go\bin\go.exe' vet ./...
& 'D:\Go\bin\go.exe' build -trimpath -o 'D:\sms\bin\identity-admin.exe' .
& 'D:\sms\bin\identity-admin.exe' -catalog 'D:\sms\data\catalog.json' -referrals 'D:\sms\admin-panel\data\referrals.json' -builder 'D:\sms\bin\sitegen.exe' -site-out 'D:\sms\dist'
```

四个路径参数必填，启动时转为绝对路径。浏览器不能修改路径或命令。首次启动仅在邀请文件不存在时创建空配置；损坏文件直接报错，不自动重置。若修改了目录，请重启管理端以加载新的平台资料。价格采集不由管理端启动，继续使用 `update-prices.cmd` 或每日任务，见 [价格更新说明](../docs/price-updates.md)。

## 使用

以下步骤均在独立邀请工具的“邀请链接配置”页面中操作，先保持本短信／邮箱服务运行。

1. 搜索平台，或筛选已配置 / 未配置。
2. 展开“编辑邀请链接”，粘贴完整 HTTPS URL。校验只检查链接格式和精确域名白名单，不访问链接。
3. 保存后，链接写入 `admin-panel/data/referrals.json`。**此时导航尚未更新。**
4. 点击“更新本地导航”，成功后打开 `/preview/` 核对购买链接。
5. 恢复官方链接后同样需要更新导航。其他平台保持不变。

未配置时，产品优先使用自己的官方入口。邀请覆盖购买按钮及待核实平台的主入口，不影响产品规则来源、已有账户续费与管理按钮。URL 的重复参数、顺序、百分号编码及 fragment 原样保存。

可下载 JSON 备份；备份恢复暂不提供上传接口。手工恢复前停服务、另存原文件，再校验和启动。真实配置、备份、日志不应提交或部署。

## 失败与边界

- 多页面同时编辑时使用文件摘要发现过期修订，冲突须刷新后重试，不覆盖新配置。
- 写入使用同目录临时文件、同步、关闭、重命名替换；测试包含 Windows 上对已有文件的连续覆盖。
- 生成失败保留已保存配置和上版导航，修复生成器后可重试。导航更新任务限制为一个，30 秒超时。
- 限制 Host、Origin、随机 CSRF、POST、请求大小；不开放 CORS。预览仅允许三个公开文件，使用受限目录 API，拒绝符号链接与私有路径。
- 本工具没有远程身份认证，不能改成公网监听部署。公开托管仅上传 `dist`。
- 此版不防本机其他进程直接改写文件；正常使用应只运行一个管理端。保存期间不要用其他程序并发修改邀请文件。

尚未验证真实平台邀请归因或注册行为；只验证保存的链接出现在正确的购买入口。
