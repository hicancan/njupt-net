# 开发、测试与发布

Go 内核与 Python 研究项目独立。常规贡献使用公开夹具和本地服务完成测试；校园网集成测试另外指定源地址与账号。

## Go 开发

使用 Go 1.26 或更新版本，在仓库根目录执行：

```text
go test ./...
go vet ./...
go build -trimpath -o njupt-net ./cmd/njupt-net
```

Windows 输出文件名使用 `njupt-net.exe`。生产代码使用 `golang.org/x/net/html` 解析实际表单，使用 `golang.org/x/sys/windows` 查询 Windows 网卡与地址；其余功能使用 Go 标准库。

业务文件包含自己的类型、请求、解析和状态确认。协议测试贴近对应源码，固定夹具保护真实契约与异常边界。新接口应先说明请求来源、必要参数、认证前提和返回语义，再加入实现。

CLI 的单次命令与 `session` 共用业务分发。会话测试检查单行 JSON 请求和响应、账号隔离、配置快照、门户实例复用、错误后的下一条请求、取消与全部管理会话清理。协议见 [连续命令会话](architecture.md#连续命令会话)。

网络测试分别核对只读连接复用与修改请求的新连接，确保具有修改效果的 GET 也保持每次调用单次提交。网卡绑定测试检查源 IPv4 的唯一启用网卡、连接后套接字的实际网卡索引，以及原生绑定失败的错误传播。Windows、Linux 与 macOS 分别使用 `IP_UNICAST_IF`、`SO_BINDTOIFINDEX` 与 `IP_BOUND_IF`；Linux 运行要求内核 5.7 或更新版本。

重定向测试核对普通读取对跳转的拒绝、GET 与 POST 的单次提交、业务声明的结果页导航和跳转后的读取传输。签名登录入口、退出链和页面修改分别覆盖允许的读取目标、返回初始动作、跨站跳转及管理会话过期。

业务操作所需的动态配置、鉴权、令牌和状态确认由对应包测试。门户登录覆盖直接提交、原生拒绝与 `ret_code` 保留、等待目标账号出现、持续错误身份的核验超时，以及未知结果的立即返回。下线测试覆盖提交前目标 session ID 已不存在、前置查询失败和接受后的状态观察。设备列表覆盖真实字符串在线标记到公开整数的转换及直接查询路径；MAC 解绑覆盖前提失败、明确拒绝、未知结果和接受后的分页确认。

运营商关系与消费保护的测试分别核对 POST 直接返回与 POST 跳转到独立 GET 结果页的流程。独立 GET 已取得当前业务字段时复用该响应，POST 返回的字段通过另一次 GET 核验。每项请求的必要性见 [原子动作与请求边界](../research/requests.md)。

## Python 离线研究

研究环境使用 Python 3.12 与 uv。进入 `research` 后创建独立环境：

```text
uv venv --python 3.12
uv sync --locked
uv run --locked python -m unittest discover -p "test_*.py" -v
```

研究项目使用 Python 标准库。离线测试使用自有协议样本与本地回环 HTTP 服务。实际页面发现、原始请求实验和样本说明见 [研究项目](../research/README.md)。

新增协议先从页面、脚本和动态配置恢复请求，再以独立请求核对参数、认证前提、响应和状态变化。确认后将必要样本加入测试，再实现 Go 业务方法。前端路径目录、当前功能开关和服务端实测结果分别记录在协议文档中。

## Windows 示例离线测试

可选 Windows 示例使用 PowerShell 7。在仓库根目录运行：

```powershell
pwsh -NoProfile -File examples/windows/tests/test.ps1 -OutputDirectory <OUTPUT_DIRECTORY>
```

将 `<OUTPUT_DIRECTORY>` 替换为本次测试输出目录。离线测试使用模拟 CLI 会话，核对目标已在线、保留已有绑定、迁移唯一持有人绑定、直接绑定等状态分支，以及身份与绑定前提、绑定和下线的单次提交、失败后的阶段记录与会话清理。目标身份已在线但宽带需要迁移时，完成关系变更后读取门户确认身份与 MAC，实际离线时再认证；其他身份在线时按精确 Self 连接清退，并确认门户离线。门户原本离线时直接处理绑定和认证。

认证测试在新绑定与已有绑定场景中重放原生未绑定、Radius 错误查询失败、繁忙和超时响应，检查首次立即恢复、连续拒绝的有界间隔、恢复窗口耗尽，以及认证阶段只查询门户和提交认证。目标已上线时直接完成；身份、运营商或 MAC 改变时结束。未知结果、凭据和时段拒绝保留原始错误。正常成功使用内核返回的已核对状态。示例脚本的参数与使用方法见 [Windows 示例](../examples/windows/README.md)。

诊断版用同一套模拟 CLI 检查运行日志、步骤与 HTTP 命令标识关联、重复运行的独立目录、配置读取失败、日志写入失败，以及认证拒绝后的只读现场观察。用例核对原始退出码、失败阶段、错误提示和修改次数，并检查密码过滤。历史认证响应重放还比较两个入口的执行顺序，核对恢复原因、原生响应与等待记录。生产版与诊断版共享 `login.ps1`。

## 校园网集成测试

集成测试需明确配置本机实际校园网 IPv4 和凭据文件。以下使用 PowerShell：

```powershell
$env:NJUPT_NET_LIVE = '1'
$env:NJUPT_NET_SOURCE = '<CAMPUS_IPV4>'
$env:NJUPT_NET_CONFIG = (Resolve-Path './config.json').Path
go test ./integration -run '^(TestLiveReadOnly|TestLiveBills)$' -count=1 -v
```

凭据文件采用 CLI 的 `accounts` 结构，集成测试遍历配置中的账号。设置 `NJUPT_NET_LIVE=1` 后执行校园请求，其余情况下跳过。只读业务测试会建立、退出管理会话，包含账号刷新与图片资源上下文初始化。`TestLivePortalBridge` 从在线门户取得签名链接，核对管理身份并退出会话。

设置上述源地址与凭据后，可核对校园服务在默认 DNS 查询不可用时的访问：

```powershell
go test ./integration -run '^TestLiveCampusWithoutDNS$' -count=1 -v
```

`TestLiveCampusWithoutDNS` 仅在测试进程内拒绝默认解析器的 DNS 查询，读取门户配置与状态，核对配置账号的运营商绑定、在线连接及门户签名管理会话，并退出所建管理会话。机器的 DNS 与 TUN 设置保持原值。校园 TCP 连接通过固定 IP 访问服务，同时绑定源地址和所属网卡；该用例验证固定地址访问及会话流程。普通客户端与公网探测的 DNS 查询仍使用操作系统解析路径。

`TestLivePortalCycle` 与 `TestLiveSelfOfflinePortalLogin` 会下线并恢复属于配置账号的终端，另需设置 `NJUPT_NET_CYCLE=1`。执行时使用独立网络保持其他应用连接。测试同时报告业务错误与恢复错误；任一步失败均使测试失败。

Windows 的批量登录验证使用独立 PowerShell 进程运行 `examples/windows/tests/live.ps1`：

```powershell
pwsh -NoProfile -File examples/windows/tests/live.ps1 -Interface Ethernet -FirstAccount FIRST_ACCOUNT -RestoreAccount RESTORE_ACCOUNT -Config ./config.json -Executable njupt-net -LoginScript ./examples/windows/login-test.ps1 -OutputDirectory <OUTPUT_DIRECTORY>
```

执行器先固定校园网卡的唯一 IPv4，再连续运行两轮目标账号与恢复账号登录。每个阶段调用共享的诊断版流程，分别保存子运行目录、退出码与原始错误；阶段前后读取门户、宽带关系、Self 在线记录及物理链路公网状态。成功同时要求目标身份、运营商、宽带持有人和实际有线通达性一致。`-Rounds` 选择一至三轮；`-IndependentInterface` 可记录独立网络的公网观测，`-ProxyConfigPath` 指定代理 Core 配置快照。

批量事件即时写入 `events.jsonl`，总结果保存到 `result.json`；执行前后网络快照分别写入 `network-before.json` 与 `network-after.json`。测试失败时进入 `finally`，按当前状态调用恢复账号的诊断版登录，并单独核对恢复结果。执行器独立运行，各阶段进度与结果持续写入日志。

## 持续集成

GitHub Actions 在 Windows、Linux、macOS 运行 Go 测试、vet 与 Python 离线测试，并在 Windows 运行 PowerShell 示例流程测试。工作流使用固定提交的官方 Actions，常规任务拥有仓库读取权限。校园网实测状态见 [p](p.md) 和 [zfw](zfw.md)。

## 发布

推送形如 `v4.0.0` 的标签触发发布工作流。工作流先运行同一套 CI，再构建六个目标：

| 系统 | 架构 | 归档 |
|---|---|---|
| Windows | amd64、arm64 | ZIP |
| Linux | amd64、arm64 | tar.gz |
| macOS | amd64、arm64 | tar.gz |

归档包含可执行文件、README、许可证与配置示例，并生成 `SHA256SUMS`。Windows ZIP 另含 `examples/windows/` 中的生产登录脚本、诊断入口、诊断采集脚本与使用说明，离线测试文件保留在源码仓库。每个归档的可执行程序仍为对应平台的一个 `njupt-net`。发布工作流创建草稿 Release，维护者核对版本说明后发布。版本通过 `njupt-net version` 查询；发布构建将标签写入可执行文件。

## 仓库内容

`docs/` 保存 Markdown 协议说明。`examples/windows/` 保存可选 PowerShell 流程与离线测试。`research/` 保存可复用实验和必要自有样本；运行输出通过 `--output` 指定位置。公开样本说明来源或构造方式，优先使用表达协议契约的最小内容。账号配置、真实会话、个人账单与临时抓取留在使用者指定的本地位置。
