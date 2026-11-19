# 部署与地址

| 服务 | 域名 | 校园地址 | 入口 |
|---|---|---|---|
| p | p.njupt.edu.cn | 10.10.244.11 | HTTPS 802，/eportal/portal/ |
| Self | zfw.njupt.edu.cn | 10.10.244.240 | HTTP 8080，/Self/ |

p 默认使用 HTTPS 802。参数还支持 HTTP 801、803 和 HTTPS 804，实际可用端口由校园部署决定。Self 管理会话使用独立的 Cookie。

## 网络约束

内核使用 IPv4，根据选定源地址找到唯一启用网卡，TCP 同时绑定该地址与网卡。校园服务按当前部署映射直拨 IP，URL、HTTP Host 与 TLS SNI 保留服务域名，证书按域名验证。`interfaces` 显示本机地址，`p status` 查询该源地址的门户会话。

Windows 使用 `IP_UNICAST_IF` 选择 TCP 出口网卡；Linux 使用 `SO_BINDTOIFINDEX`，最低内核版本为 5.7；macOS 使用 `IP_BOUND_IF`。这些选项在连接前应用于每个 TCP 套接字，校园请求固定从选定网卡发出。

`probe` 使用普通客户端，通过系统 DNS 解析 `www.msftconnecttest.com`，再以同一源 IPv4 和网卡请求 `http://www.msftconnecttest.com/connecttest.txt`。HTTP 200 且正文为 `Microsoft Connect Test` 时，确认本次探测端点可达。DNS 查询使用操作系统的解析路径，返回地址后的 TCP 连接使用选定网卡。

门户身份、宽带绑定和外网探测分别报告。Windows 登录脚本默认选择门户 802，在身份与绑定确认后返回登录成功。指定 `-Probe` 时另行记录公网探测结果。双网环境下选择校园 IPv4 执行业务，其他应用继续按自身代理和系统路由连接外网。
