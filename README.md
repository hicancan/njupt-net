# njupt-net

南京邮电大学校园网 CLI 与 Go 内核。选择校园网接口后，可登录上网、查看连接与账号信息、管理认证策略、查询和导出账单。命令输出结构化 JSON，适合终端使用与程序调用。

- `p`：当前终端的配置、在线状态、账号认证、注销、自助服务入口与条件改密。
- `zfw`：账号资料、在线连接、MAC 绑定、无感知策略、运营商绑定、消费保护与账单。
- `network`：网卡枚举、源地址选择与外网通达性检测。
- `research`：独立 Python 协议研究、公开样本与离线实验。

上网认证提供 801–804 四个入口，CLI 默认使用 `https://p.njupt.edu.cn:804/eportal/portal/`，通过 `--port` 选择端口。账号自助服务使用 `http://zfw.njupt.edu.cn:8080/Self/`。每项命令的请求、参数与结果见 [p](docs/p.md) 和 [zfw](docs/zfw.md)。

## 安装

使用 Go 1.26 或更新版本：

```text
go install github.com/hicancan/njupt-net/v4/cmd/njupt-net@v4.0.0
```

也可以从源码构建：

```text
go build -trimpath -o njupt-net ./cmd/njupt-net
```

Windows 构建输出使用 `njupt-net.exe`。预编译归档面向 Windows、Linux、macOS 的 amd64 与 arm64；Linux 要求内核 5.7 或更新版本。下载入口为 [Releases](https://github.com/hicancan/njupt-net/releases)。

Windows 归档另附 [PowerShell 登录脚本](examples/windows/README.md)：选择目标校园账号，按当前绑定状态完成宽带迁移与登录。脚本使用 PowerShell 7，通过一个 CLI 会话完成操作。

## 快速使用

将 [config.example.json](config.example.json) 复制为 `config.json`，填写校园账号与密码。账号别名由使用者定义：

```json
{
  "accounts": {
    "default": {
      "account": "YOUR_CAMPUS_ACCOUNT",
      "password": "YOUR_CAMPUS_PASSWORD"
    }
  },
  "broadband_account": {
    "operator": "cmcc",
    "account": "YOUR_BROADBAND_ACCOUNT",
    "password": "YOUR_BROADBAND_PASSWORD"
  }
}
```

`broadband_account` 只用于提交运营商绑定；不进行该操作时可以省略。校园账号保持原值，登录运营商通过命令参数选择。

```text
njupt-net version
njupt-net interfaces
njupt-net --interface "Ethernet" p status
njupt-net --interface "Ethernet" --account default p login --operator cmcc
njupt-net --interface "Ethernet" probe
njupt-net --interface "Ethernet" --account default zfw verify
njupt-net --interface "Ethernet" --account default zfw account
njupt-net --interface "Ethernet" --account default zfw online
njupt-net --interface "Ethernet" --account default zfw bills --kind monthly --year 2026
njupt-net --interface "Ethernet" --account default zfw export --kind monthly --year 2026 --all --output month.xls
```

将 `Ethernet` 替换为 `interfaces` 输出中的实际网卡名。所有网络命令必须明确选择 `--interface NAME` 或 `--source IPv4`，二者只能指定一个。网卡有多个 IPv4 时使用 `--source`。`version`、网卡枚举与帮助无需选择源地址；`version` 返回版本、Go 版本、系统与架构。

全局参数放在命令组之前，业务参数放在子命令之后。配置默认从当前目录的 `config.json` 读取，可用 `--config PATH` 指定；私有管理命令和门户认证使用 `--account ALIAS` 选择凭据。

脚本和程序可使用 `session` 连续发送命令，复用门户实例与各账号的管理会话。标准输入、输出均为每行一个 JSON，使用方法见 [连续命令会话](docs/architecture.md#连续命令会话)。

已在线终端还可通过门户签名链接进入自助服务。全局 `--self-url-file PATH` 读取仅含一行完整 URL 的文件，`--account` 选择期望账号；命令使用该链接登录并核对身份，登录失败时返回错误。操作示例见 [管理会话](docs/zfw.md#管理会话)。

## 网络行为

内核按选定源 IPv4 找到唯一启用网卡，TCP 同时绑定源地址和该网卡。p 与 zfw 按[当前部署地址](docs/deployment.md)直拨校园服务 IP，URL 与 HTTP Host 保留服务域名，HTTPS 使用同一域名发送 SNI 并验证证书。所有连接设置仅作用于当前进程。

只读页面和查询复用 HTTP 连接；认证及修改请求使用新连接，每次修改调用提交一次业务请求。需要导航到结果页的操作，由业务接口明确声明允许读取的目标。

`probe` 使用系统 DNS 解析外部域名，TCP 连接绑定同一源 IPv4 和网卡。`p status` 表示门户在线状态，`probe` 单独观测外网响应。Windows 登录脚本默认选择 801，以门户身份与宽带绑定确认作为成功条件；指定 `-Probe` 时另行报告公网探测结果。

双网环境中，校园业务的 TCP 连接固定从所选校园网卡发出，其他应用按各自代理和系统路由连接。校园服务使用固定 IP；普通域名与公网探测的 DNS 查询仍走操作系统的解析路径。

## 命令与结果

```text
njupt-net p --help
njupt-net zfw --help
```

单次命令输出为 JSON：

```json
{"command":"p status","data":{"source":"192.0.2.10","online":false,"terminal":null}}
```

单次命令的错误写入标准错误，结构为 `{command,data,error:{message}}`，保留已经确认的业务结果。退出码：`0` 成功、`1` 操作失败、`2` 参数错误。帮助为文本。输出文件使用新路径创建，路径已存在时返回错误。

单次私有 zfw 命令依次登录、执行业务、退出管理会话；`session` 按账号分别保留会话，在结束时统一退出。上网终端通过 `p logout` 或 `zfw offline` 下线。每次内核修改调用提交一次请求；`outcome` 表达服务器处理结果，`verified` 表达最终状态是否确认。

## 当前服务行为

门户普通登录使用账号、密码与运营商选择，电脑、手机和平板通过 `--terminal` 选择。登录直接提交一次原生认证，服务器接受后查询选定源 IPv4 的目标在线身份；拒绝和未知结果按本次响应返回。运营商关系通过 zfw 绑定与解绑，Windows 登录脚本组合这些原子操作完成目标账号登录。

MAC 绑定列表已读取到真实设备记录。当前门户注销返回拒绝，充值页面显示服务菜单；命令按实际响应返回处理结果与可用状态。各接口的行为列于协议文档。

## 文档与开发

- [部署与地址](docs/deployment.md)
- [p 门户协议与命令](docs/p.md)
- [zfw 自助服务协议与命令](docs/zfw.md)
- [内核架构](docs/architecture.md)
- [开发、测试与发布](docs/development.md)
- [Windows 账号登录](examples/windows/README.md)
- [Python 协议研究](research/README.md)
- [原子动作与请求边界](research/requests.md)

许可证：[MIT](LICENSE)。
