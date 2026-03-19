# njupt-net

中文 | [English](README.en.md)

[![Verify](https://github.com/hicancan/njupt-net-cli/actions/workflows/release.yml/badge.svg)](https://github.com/hicancan/njupt-net-cli/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/hicancan/njupt-net-cli)](https://github.com/hicancan/njupt-net-cli/blob/main/go.mod)
[![Latest Release](https://img.shields.io/github/v/release/hicancan/njupt-net-cli)](https://github.com/hicancan/njupt-net-cli/releases)
[![Repo Stars](https://img.shields.io/github/stars/hicancan/njupt-net-cli?style=social)](https://github.com/hicancan/njupt-net-cli/stargazers)

`njupt-net` 是一个面向 NJUPT 自助服务系统与 Portal 网关的 Go 终端系统。

它不是“几个接口的命令行封装”，而是一套带类型内核的终端系统，核心目标是：

- 把逆向出来的协议真相显式固化下来
- 把 `confirmed / guarded / blocked` 变成运行时语义
- 把写操作统一为 readback-first 的可验证工作流
- 把 `--output json` 变成稳定的机器接口
- 把守护逻辑做成跨平台 Go runtime，而不是脚本拼装

仓库名仍然是 `njupt-net-cli`，但正式产品面是 `njupt-net` 命令。

## 目录

- [项目定位](#项目定位)
- [核心亮点](#核心亮点)
- [架构设计](#架构设计)
- [命令功能清单](#命令功能清单)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [机器可读契约](#机器可读契约)
- [Go 守护运行时](#go-守护运行时)
- [证据级别模型](#证据级别模型)
- [质量门禁](#质量门禁)
- [项目结构](#项目结构)
- [相关文档](#相关文档)

## 项目定位

NJUPT 的联网环境并不是一个单一、稳定、统一的 API，而是由以下部分组合而成：

- Self 自助服务系统
- Portal 认证网关
- HTML 页面中的嵌入状态
- JSON / JSONP 接口
- 可确认、可保守实现、仍被环境阻塞的不同能力层级

`njupt-net` 的目标不是把复杂性藏起来，而是把复杂性结构化：

- 已确认的能力，按确定语义实现
- 仍有风险的能力，按 guarded 暴露
- 仍受环境阻塞的能力，按 blocked 处理

## 核心亮点

- Go-first 的 typed kernel
- `confirmed / guarded / blocked` 证据模型
- Self 登录主链的受保护页面回读校验
- 绑定、消费保护、mauth 的 readback-first 写入模型
- Portal 802 为主实现，801 仅作 guarded fallback
- 跨平台 Go 守护 runtime：
  - 白天守 `B`
  - 夜间守 `W`
  - 不主动 `logout`
  - 高频探测与激进恢复
  - 结构化状态与事件流
- AST 架构护栏测试，防止未来回退成“命令层偷做协议”

## 架构设计

项目采用克制的模块化单体架构，而不是过度设计的多仓或插件系统。

```text
cmd/njupt-net
  -> internal/app
      -> internal/kernel
      -> internal/selfservice
      -> internal/portal
      -> internal/workflow
      -> internal/runtime/guard
```

### 各层职责

- `cmd/njupt-net`
  - 只负责命令装配和 flag 解析
- `internal/app`
  - lazy config
  - renderer/output
  - client/session factory
  - confirmation policy
- `internal/kernel`
  - evidence level
  - typed result
  - typed problem catalog
  - transport contract
  - writeflow 语义
- `internal/selfservice`
  - Self 协议真相
  - endpoint/request 组织
  - parser/helper
  - typed model mapping
- `internal/portal`
  - Portal 请求构建
  - 原始响应/JSONP 解析
  - `ret_code` 分类
  - typed model mapping
- `internal/workflow`
  - 纯 use-case 组合动作
  - 不直接构造具体 transport
- `internal/runtime/guard`
  - 调度、探测、runner、supervisor、state store
  - typed runtime state machine

## 命令功能清单

当前 CLI 一共有 **8 个功能域，32 个叶子命令**。

### 顶层功能域

- `self`
- `dashboard`
- `service`
- `setting`
- `bill`
- `portal`
- `raw`
- `guard`

### 完整命令树

```text
njupt-net
  self
    login
    logout
    status
    doctor
  dashboard
    online-list
    login-history
    refresh-account-raw
    offline
    mauth get
    mauth toggle
  service
    binding get
    binding set
    consume get
    consume set
    mac list
    migrate
  setting
    person get
    person update
  bill
    month-pay
    online-log
    operator-log
  portal
    login
    logout
    login-801
    logout-801
  raw
    get
    post
  guard
    run
    start
    stop
    status
    once
```

### 功能域说明

| 功能域 | 典型命令 | 作用 |
| --- | --- | --- |
| `self` | `login`, `logout`, `status`, `doctor` | Self 登录与诊断主路径 |
| `dashboard` | `online-list`, `login-history`, `mauth`, `offline` | 在线会话、历史记录、无感知与 guarded 操作 |
| `service` | `binding`, `consume`, `mac`, `migrate` | 宽带绑定、消费保护、MAC、迁移工作流 |
| `setting` | `person get/update` | guarded 的个人资料面 |
| `bill` | `month-pay`, `online-log`, `operator-log` | 账单与记录查询 |
| `portal` | `login`, `logout`, `login-801`, `logout-801` | Portal 802 主链与 801 fallback |
| `raw` | `get`, `post` | 低层调试探针 |
| `guard` | `start`, `status`, `stop`, `once`, `run` | Go 守护运行时 |

## 快速开始

### 1. 编译

```bash
go build ./...
```

跨平台发布构建：

```bash
bash ./scripts/build.sh all
```

```powershell
.\scripts\build.ps1 -Mode all
```

### 2. 准备 `credentials.json`

最小示例：

```json
{
  "accounts": {
    "B": {
      "username": "你的学号",
      "password": "你的密码"
    },
    "W": {
      "username": "你的学号",
      "password": "你的密码"
    }
  },
  "cmcc": {
    "account": "你的移动宽带账号",
    "password": "你的移动宽带密码"
  },
  "self": {
    "baseURL": "http://10.10.244.240:8080",
    "timeoutSeconds": 10
  },
  "portal": {
    "baseURL": "https://10.10.244.11:802/eportal/portal",
    "fallbackBaseURLs": [
      "https://p.njupt.edu.cn:802/eportal/portal"
    ],
    "isp": "mobile",
    "timeoutSeconds": 8,
    "insecureTLS": true
  },
  "guard": {
    "stateDir": "dist/guard",
    "probeIntervalSeconds": 3,
    "bindingCheckIntervalSeconds": 180,
    "timezone": "Asia/Shanghai",
    "schedule": {
      "dayProfile": "B",
      "nightProfile": "W",
      "nightStart": "23:30",
      "nightEnd": "07:00"
    }
  }
}
```

### 3. 常用命令示例

```bash
njupt-net self login --profile B
njupt-net self status --profile B
njupt-net service binding get --profile B
njupt-net portal login --profile B --ip 10.163.177.138
njupt-net guard start --replace
njupt-net guard status --output json
```

### 4. 本地 smoke 测试

```powershell
.\scripts\test-local.ps1
```

只读模式：

```powershell
.\scripts\test-local.ps1 -ReadOnly -SkipPortal
```

## 配置说明

配置查找顺序：

1. `--config <path>`
2. `NJUPT_NET_CONFIG`
3. `credentials.json`

支持的环境变量覆盖包括：

- `NJUPT_NET_OUTPUT`
- `NJUPT_NET_SELF_BASE_URL`
- `NJUPT_NET_PORTAL_BASE_URL`
- `NJUPT_NET_PORTAL_ISP`
- `NJUPT_NET_INSECURE_TLS`
- `NJUPT_NET_SELF_TIMEOUT`
- `NJUPT_NET_PORTAL_TIMEOUT`

下面这些命令不应依赖成功加载配置文件：

- `njupt-net --help`
- `njupt-net completion ...`
- `njupt-net guard status --state-dir ...`
- `njupt-net guard stop --state-dir ...`

## 机器可读契约

`--output json` 是正式支持的机器接口，而不是调试附属品。

### 操作结果

所有命令都返回 typed `OperationResult`：

- `level`
- `success`
- `message`
- `data`
- `problems`
- `raw`

### Problems

问题输出是版本化的稳定契约。

每个问题对象都包含：

- `code`
- `message`
- `details`

当前重点的 typed family 包括：

- Portal 问题
- readback / restore / state-comparison 问题
- invalid-config 问题
- guarded / blocked capability 问题

### Guard Status

`guard status --output json` 使用嵌套结构：

- `running`
- `health`
- `desiredProfile`
- `scheduleWindow`
- `binding`
- `connectivity`
- `portal`
- `cycle`
- `timing`
- `log`

### Guard Events

守护运行时事件采用 JSONL 输出，记录稳定的 `kind + typed details`。

支持的事件种类：

- `startup`
- `schedule-switch`
- `binding-audit`
- `portal-login`
- `binding-repair`
- `degraded`
- `shutdown`
- `fatal`

相比之下，人类可读输出允许更自由地演进。

## Go 守护运行时

Go guard 是正式支持的长运行模型。

### 运行策略

- 白天守 `B`
- 夜间守 `W`
- 默认 `3s` 高频探测
- 不主动 `logout`
- 探测失败后立即走恢复链
- `stop` 优先发 graceful stop request，超时再 kill

### 状态目录

默认状态目录：

```text
dist/guard
```

关键文件：

- `status.json`
- `current-log.txt`
- `current-events.txt`
- `logs/*.log`
- `logs/*.events.jsonl`
- `launcher.pid`
- `worker.pid`

### 健康状态

- `healthy`
  - 目标 profile 正确
  - 绑定正确
  - 最终连通
- `degraded`
  - 一整轮恢复链结束后仍未恢复到目标状态
- `stopped`
  - 守护未运行

## 证据级别模型

逆向确定性模型是运行时 API 的一部分。

### `confirmed`

可以按正式能力实现。

例如：

- Self 登录主链
- 宽带绑定写入与回读验证
- 消费保护写入与回读验证
- mauth toggle
- Portal 802 主登录链

### `guarded`

可以暴露，但必须保守。

例如：

- force offline
- Portal 801 fallback
- 某些环境敏感的写路径

### `blocked`

已知接口存在，但成功语义还不足以承诺为正式确定能力。

例如：

- 仍被环境阻塞的用户资料更新路径
- 仍未拿到充分样本的 Portal 某些 ret_code 分支

## 质量门禁

本地质量门禁：

```bash
go test ./...
go test -cover ./...
go vet ./...
```

```powershell
.\scripts\build.ps1 -Mode all
.\scripts\test-local.ps1 -ReadOnly -SkipPortal
```

GitHub Actions 会继续强制：

- 格式检查
- 单元测试
- 覆盖率运行
- `go vet`
- `staticcheck`
- 多平台构建

## 项目结构

```text
.
├── cmd/njupt-net
├── doc
├── internal/app
├── internal/kernel
├── internal/portal
├── internal/runtime/guard
├── internal/selfservice
├── internal/workflow
└── scripts
```

## 相关文档

- [doc/FINAL-SSOT.md](doc/FINAL-SSOT.md)
- [doc/IMPLEMENTATION-TASK.md](doc/IMPLEMENTATION-TASK.md)
- [doc/ARCHITECTURE-REVIEW.md](doc/ARCHITECTURE-REVIEW.md)
- [doc/CAPABILITY-MATRIX.md](doc/CAPABILITY-MATRIX.md)

## 仓库说明

- 仓库名是 `njupt-net-cli`，正式产品名是 `njupt-net`
- 历史 Python/PowerShell 守护脚本不再是正式支持路径
- 当前主线产品是：Go CLI、typed kernel、Go guard runtime
