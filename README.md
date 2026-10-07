# njupt-net

南京邮电大学校园网 CLI 与 Go 内核。选择校园网接口后，可登录上网、查看连接与账号信息、管理认证策略、查询和导出账单。命令输出结构化 JSON，适合终端使用与程序调用。

- `p`：当前终端的配置、在线状态、账号认证、注销、自助服务入口与条件改密。
- `zfw`：账号资料、在线连接、MAC 绑定、无感知策略、运营商绑定、消费保护与账单。
- `network`：网卡枚举、源地址选择与外网通达性检测。
- `research`：独立 Python 协议研究、公开样本与离线实验。

上网认证使用 `https://p.njupt.edu.cn:804/eportal/portal/`，账号自助服务使用 `http://zfw.njupt.edu.cn:8080/Self/`。每项命令的请求、参数与结果见 [p](docs/p.md) 和 [zfw](docs/zfw.md)。

## 安装

使用 Go 1.26 或更新版本：

```text
go install github.com/hicancan/njupt-net/v3/cmd/njupt-net@v3.1.0
```

也可以从源码构建：

```text
go build -trimpath -o njupt-net ./cmd/njupt-net
```

Windows 构建输出使用 `njupt-net.exe`。预编译归档面向 Windows、Linux、macOS 的 amd64 与 arm64；下载入口为 [Releases](https://github.com/hicancan/njupt-net/releases)。

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

已在线终端还可通过门户签名链接进入自助服务。全局 `--self-url-file PATH` 读取仅含一行完整 URL 的文件，`--account` 选择期望账号；命令使用该链接登录并核对身份，登录失败时返回错误。操作示例见 [管理会话](docs/zfw.md#管理会话)。

## 网络行为

校园请求绑定选定源 IPv4，直接连接校园服务。DNS 使用系统解析器，HTTPS 验证域名与证书。所有连接设置仅作用于当前进程。

双网环境中，可选择校园接口执行业务，让另一条独立网络承担其他应用流量。源地址绑定仍受操作系统路由和校园网络可达性约束；`p status` 表示门户在线状态，`probe` 独立检验外网响应。

## 命令与结果

```text
njupt-net p --help
njupt-net zfw --help
```

普通输出为 JSON：

```json
{"command":"p status","data":{"source":"192.0.2.10","online":false,"terminal":null}}
```

错误写入标准错误，结构为 `{command,data,error:{message}}`，保留已经确认的业务结果。退出码：`0` 成功、`1` 操作失败、`2` 参数错误。帮助为文本。输出文件使用新路径创建，路径已存在时返回错误。

私有 zfw 命令依次登录、执行业务、退出管理会话。上网终端通过 `p logout` 或 `zfw offline` 下线。修改请求只提交一次；`outcome` 表达服务器处理结果，`verified` 表达最终状态是否确认。

## 当前服务行为

门户普通登录使用账号、密码与运营商选择，电脑、手机和平板通过 `--terminal` 选择。当前 PC 有线登录、门户签名登录、账号与账单读取、指定连接下线后重新认证均已有校园网实测结果。移动宽带从一个校园账号解绑、绑定到另一个校园账号并登录上网的流程也已实测通过。

当前门户注销返回拒绝，MAC 列表接口返回空正文，充值页面显示服务菜单。对应命令返回具体服务端结果或协议错误。各接口的当前行为与待验证项列于协议文档。

## 文档与开发

- [部署与地址](docs/deployment.md)
- [p 门户协议与命令](docs/p.md)
- [zfw 自助服务协议与命令](docs/zfw.md)
- [内核架构](docs/architecture.md)
- [开发、测试与发布](docs/development.md)
- [Python 协议研究](research/README.md)

许可证：[MIT](LICENSE)。
