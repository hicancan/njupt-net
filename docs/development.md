# 开发、测试与发布

Go 内核与 Python 研究项目独立。常规贡献使用公开夹具和本地服务完成测试；校园网集成测试另外指定源地址与账号。

## Go 开发

使用 Go 1.26 或更新版本，在仓库根目录执行：

```text
go test ./...
go vet ./...
go build -trimpath -o njupt-net ./cmd/njupt-net
```

Windows 输出文件名使用 `njupt-net.exe`。生产代码直接依赖 `golang.org/x/net/html` 解析实际表单；其他网络与 CLI 功能使用 Go 标准库。

业务文件包含自己的类型、请求、解析和状态确认。协议测试贴近对应源码，固定夹具保护真实契约与异常边界。新接口应先说明请求来源、必要参数、认证前提和返回语义，再加入实现。

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

将 `<OUTPUT_DIRECTORY>` 替换为本次测试输出目录。离线测试使用模拟 CLI JSON，核对目标已在线、保留已有绑定、迁移唯一持有人绑定、直接绑定等状态分支，以及身份与绑定前提、单次修改和失败后的阶段记录。示例脚本的参数与使用方法见 [Windows 示例](../examples/windows/README.md)。

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

`TestLiveCampusWithoutDNS` 仅在测试进程内拒绝默认解析器的 DNS 查询，读取门户配置与状态，核对配置账号的运营商绑定、在线连接及门户签名管理会话，并退出所建管理会话。机器的 DNS 与 TUN 设置保持原值。该用例验证固定地址访问及会话流程。

`TestLivePortalCycle` 与 `TestLiveSelfOfflinePortalLogin` 会下线并恢复属于配置账号的终端，另需设置 `NJUPT_NET_CYCLE=1`。执行时使用独立网络保持其他应用连接。测试同时报告业务错误与恢复错误；任一步失败均使测试失败。

## 持续集成

GitHub Actions 在 Windows、Linux、macOS 运行 Go 测试、vet 与 Python 离线测试，并在 Windows 运行 PowerShell 示例流程测试。工作流使用固定提交的官方 Actions，常规任务拥有仓库读取权限。校园网实测状态见 [p](p.md) 和 [zfw](zfw.md)。

## 发布

推送形如 `v3.0.0` 的标签触发发布工作流。工作流先运行同一套 CI，再构建六个目标：

| 系统 | 架构 | 归档 |
|---|---|---|
| Windows | amd64、arm64 | ZIP |
| Linux | amd64、arm64 | tar.gz |
| macOS | amd64、arm64 | tar.gz |

归档包含可执行文件、README、许可证与配置示例，并生成 `SHA256SUMS`。Windows ZIP 另含 `examples/windows/login.ps1` 与使用说明，测试文件保留在源码仓库。每个归档的可执行程序仍为对应平台的一个 `njupt-net`。发布工作流创建草稿 Release，维护者核对版本说明后发布。版本通过 `njupt-net version` 查询；发布构建将标签写入可执行文件。

## 仓库内容

`docs/` 保存 Markdown 协议说明。`examples/windows/` 保存可选 PowerShell 流程与离线测试。`research/` 保存可复用实验和必要自有样本；运行输出通过 `--output` 指定位置。公开样本说明来源或构造方式，优先使用表达协议契约的最小内容。账号配置、真实会话、个人账单与临时抓取留在使用者指定的本地位置。
