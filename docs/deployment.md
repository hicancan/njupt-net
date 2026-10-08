# 部署与地址

当前上网认证由 p 提供，账号管理由 zfw 提供。下表区分域名、部署地址、服务端口与页面入口；后文给出全端口扫描和应用响应。

## 服务边界

| 服务 | 地址观测 | 用途 | 内核范围 |
|---|---|---|---|
| `p.njupt.edu.cn` | `10.10.244.11` | 上网认证门户 | HTTPS 443 页面与 HTTPS 804 EPortal |
| `zfw.njupt.edu.cn` | `10.10.244.240` | 账号自助管理 | HTTP 8080 `/Self/` |
| `3a.njupt.edu.cn` | `192.168.168.168` | 历史认证入口 | 未纳入当前协议 |
| `10.168.6.10` | 历史代码中的地址 | 旧 ACSetting 线索 | 未确认当前适用性 |

地址记录以 2026-10-07 的校园接入观测为依据。p 与 zfw 协议包分别维护 `p.njupt.edu.cn → 10.10.244.11`、`zfw.njupt.edu.cn → 10.10.244.240` 的部署映射，运行时直拨对应 IP。URL、HTTP Host 与 HTTPS 证书验证保留原域名。

2026-10-08 的固定 IP 直连复核确认：p 的 HTTPS 443 页面与 HTTPS 804 配置接口可访问，域名证书验证成功；zfw 的 HTTP 8080 登录入口可访问。校园认证与自助服务访问使用这些部署地址，无需在请求前解析服务域名。

`192.168.168.168` 是私有地址，历史记录将它作为认证服务器地址。排查可达性时依次检查域名解析、目标路由、出口网卡和 HTTP 响应。热点与校园网使用重叠网段时，系统按路由前缀和优先级选择出口，可用实际路由表定位冲突。该历史入口的当前服务用途待重新确认。

## 网页与认证 API 端口

| 主机 | TCP 端口 | 协议 | 已观察用途 |
|---|---:|---|---|
| p | 80 | HTTP | 上网登录页面 |
| p | 443 | HTTPS | 当前网页入口，如 `/a79.htm` |
| p | 801 | HTTP | Portal 配置 API，另有旧 ACSetting 路径 |
| p | 802 | HTTPS | Portal 配置 API |
| p | 803 | HTTP | 当前脚本指定的 HTTP API 端口 |
| p | **804** | **HTTPS** | **当前脚本指定的 HTTPS API 端口** |
| zfw | 8080 | HTTP | 独立 Self 管理服务 |

`a41.js` 的当前初始化配置选择 HTTPS 804，内核使用 `https://p.njupt.edu.cn:804/eportal/portal/` 认证。801 至 804 均能读取配置，当前账号认证实验使用 804。[门户初始化脚本](https://p.njupt.edu.cn/a41.js)

Self 的完整入口是 `http://zfw.njupt.edu.cn:8080/Self/`，主机地址为 `10.10.244.240`。

## TCP 全端口结果

2026-10-07 对两个目标分别扫描 TCP 1–65535，随后对开放端口进行服务探测和 HTTP 请求核对。扫描通过校园接入链路完成。

| 主机 | open | closed | filtered | 合计 |
|---|---:|---:|---:|---:|
| p `10.10.244.11` | 14 | 63254 | 2267 | 65535 |
| zfw `10.10.244.240` | 12 | 63298 | 2225 | 65535 |

`open` 表示连接建立成功，`closed` 表示明确拒绝，`filtered` 表示扫描期间未取得可判定开放或关闭的响应。

### p 开放端口

| TCP 端口 | 实际响应 |
|---|---|
| 22 | SSH，OpenSSH 8.4 banner |
| 23 | Telnet 样式协商，曾返回 Drcom 控制台 banner |
| 80 | HTTP，`Server: DrcomServer1.0`，认证网页 |
| 82 | 连接建立后关闭 |
| 85 | HTTP，`Server: DrcomServer1.0`，返回 203 与错误页面 |
| 443 | HTTPS，认证网页 |
| 801、803 | HTTP，nginx，Portal API |
| 802、804 | HTTPS，nginx，Portal API |
| 901 | 连接建立后关闭 |
| 1723 | TCP 可连接，应用协议待确认 |
| 9002 | HTTP 跳转 |
| 61671 | 连接建立后关闭 |

### zfw 开放端口

| TCP 端口 | 实际响应 |
|---|---|
| 110 | POP3 banner |
| 111 | RPCbind |
| 1158、3938 | TLS 响应，应用协议待确认 |
| 5003 | 连接建立后关闭 |
| 5520 | Oracle Remote Method Invocation |
| 8080 | HTTP，Dr.COM Server，跳转至 Self |
| 8088 | HTTP 404，正文为“URL请求路径不存在” |
| 8089 | 正文为 `INVALID REQUEST` |
| 10000 | HTTP，MiniServ 1.780 / Webmin httpd |
| 31042、46397 | TCP 可连接，应用协议待确认 |

上表服务名称来自实际握手、banner 或应用响应。Nmap 的端口号默认名称可用于寻找下一步探测线索，应用协议由对应响应确认。

## UDP 全端口结果

同日扫描两个目标的 UDP 1–65535，结果如下：

| 主机 | 收到 UDP 应答的端口 | open\|filtered，无应答 | closed，收到端口不可达 |
|---|---|---:|---:|
| p | 123、161 | 65201 | 332 |
| zfw | 111、34151 | 65197 | 336 |

UDP 应答端口还需针对具体协议发送请求并解析响应。`open|filtered` 表示本次探测没有收到响应；UDP 服务可能仅回答符合协议的报文。[Nmap UDP 扫描说明](https://nmap.org/book/man-port-scanning-techniques.html)

随后对 p 发送两个定向协议请求，各发送一个数据报，均收到同一目标端口应答：

| UDP 端口 | 请求与响应 | 协议结果 |
|---|---|---|
| 123 | 48 字节 NTP 请求；48 字节应答，头部 `1c 03 0a fa` | NTPv3，server mode 4，stratum 3 |
| 61440 | 请求 `07 01 08 00 01 00 00 00`；32 字节应答，头部 `07 01 10 00 02 00 00 00` | 与 Drcom PPPoE 挑战响应结构一致，code 7、request ID 1、type 2 |

NTP 字段按 [RFC 5905](https://www.rfc-editor.org/rfc/rfc5905.html) 解析，Drcom 请求与响应布局参考 [drcom-generic 客户端](https://github.com/drcoms/drcom-generic/blob/master/latest-pppoe.py)。61440 在全端口通用探测中没有应答，符合协议的挑战请求取得了响应。该协议的账号认证与持续保活留待独立实验；当前 CLI 使用 HTTPS 804 认证。

## 复现

在能够访问校园目标的网络中使用 Nmap。UDP 扫描需要支持原始套接字的运行环境与相应权限：

```text
nmap -sT -Pn -p1-65535 -oX tcp.xml 10.10.244.11 10.10.244.240
nmap -sU -Pn -p1-65535 -oX udp.xml 10.10.244.11 10.10.244.240
```

`-sV` 可进一步探测已发现端口的服务。将 XML 交给研究项目可统一读取端口范围、状态和服务属性：

```text
uv run --project research python research/deployment.py --nmap-xml tcp.xml
```

源地址绑定的定向 UDP 实验：

```text
uv run --project research python research/deployment.py --source <CAMPUS_IPV4> --udp ntp
uv run --project research python research/deployment.py --source <CAMPUS_IPV4> --udp drcom-challenge
```

应用请求与独立协议实验见 [research](../research/README.md)。重新扫描时保存时间、接入路径与原始 XML，便于比较部署变化。

## 网络约束

内核使用 IPv4，TCP 绑定选定源地址，按当前部署映射直接连接校园服务，并保留 TLS 域名验证。系统路由与 TUN 策略决定数据包如何到达目标。`interfaces` 显示本机地址，`p status` 查询该源地址的门户会话。

`probe` 使用普通客户端，通过系统 DNS 解析 `www.msftconnecttest.com`，再以同一源 IPv4 请求 `http://www.msftconnecttest.com/connecttest.txt`。HTTP 200 且正文为 `Microsoft Connect Test` 时，确认本次探测端点可达。DNS 查询使用操作系统的解析路径，TCP 源地址绑定约束该连接。

门户身份、宽带绑定和外网探测分别报告。Windows 登录脚本在身份与绑定确认后返回登录成功，公网探测失败时保留该次观测的错误信息。双网环境下选择校园 IPv4 执行业务，其他应用继续按自身代理和系统路由连接外网。
