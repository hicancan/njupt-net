# p 门户

上网认证门户位于 `https://p.njupt.edu.cn/`，当前 API 基址为 `https://p.njupt.edu.cn:804/eportal/portal/`。p 命令围绕选定源 IPv4 的当前终端工作：先恢复网页认证上下文，再查询或修改该终端的上网状态。

## 初始化与认证上下文

初始化按当前页面恢复终端上下文：

1. 读取 `/a79.htm` 的页面版本、源 IP、MAC 与 VLAN。
2. 读取 `/a41.js`，确认 HTTPS 804 与当前协议开关。
3. 调用 `page/loadConfig`，取得动态 program、page、认证方式及功能配置。
4. 从 `/a40.js` 读取当前 JavaScript 协议版本。

门户目标 IP 必须与选定源地址一致。配置请求中的 IP 等字段按当前网页方式编码。JSONP 使用 `dr` 加序号的回调名；内核核对回调名后解析其中的 JSON 数据。

当前支持 `login_method=1`、`check_online_method=1`，保留域名与 TLS 证书验证。部署切换到其他协议时明确报错。

## 接口与命令

以下路径均相对 API 基址；当前协议使用 GET，JSONP 结果或图片资源。

| 路径 | 用途 | CLI |
|---|---|---|
| `page/loadConfig` | 获取动态门户配置 | `p config`；其他操作按需初始化 |
| `online_list` | 查询目标接入 IP 对应会话 | `p status` |
| `login` | 提交账号密码与终端上下文 | `p login` |
| `logout` | 注销当前终端 | `p logout` |
| `err_code` | 获取认证补充错误 | `p error` |
| `self` | 获取自助服务入口 | `p self --type 0\|1\|2` |
| `captcha` | 获取改密图片资源 | `p password` 内部步骤；核心 `Captcha` |
| `change_pass` | 提交条件改密 | `p password` |

普通认证使用账号、密码与运营商选择。`campus` 使用校园账号原值，`njxy` 为电信，`cmcc` 为移动。配置保存原始账号，内核按动态配置添加终端前缀、运营商后缀并编码请求。

所有 p 命令接受 `--terminal pc|mobile|hipad|vipad`，默认 `pc`，分别对应终端类型 1 至 4。四种终端通过同一套请求流程传入不同参数。

`self_type`：`0` 自助登录入口，`1` 在线免登录桥，`2` 密码页面入口。CLI 返回地址；类型 1 的签名 URL 可交给 zfw 建立管理会话，使用方法见 [管理会话](zfw.md#管理会话)。

改密命令从单行文件读取新密码，在同一 Portal 上下文获取图片、读取标准输入答案并提交一次：

```text
njupt-net --interface "Ethernet" --account default p password --new-password-file new-password.txt --captcha-output captcha.png
```

## 状态与结果

`online_list` 返回会话列表与总数。内核核对 `list/total`，按 `online_ip` 匹配选定源 IP，并读取实际在线账号与 MAC；匹配结果决定 `online`。

离线响应为 `result=0`、`msg=获取用户在线信息数据为空！`，且省略 `list/total`。该响应映射为 `online:false`；其他拒绝或畸形响应返回具体错误。

| `outcome` | 含义 |
|---|---|
| `accepted` | 服务器接受修改请求 |
| `rejected` | 服务器明确拒绝 |
| `unknown` | 服务器处理结果尚未确认 |
| `not_submitted` | 没有提交修改请求，例如终端已经在线 |

`verified` 表示最终目标状态是否确认。登录接受后最多十秒、每五百毫秒只读查询在线状态，并核对 IP 与账号。目标账号已经在线时返回 `not_submitted`，此次调用仅查询当前会话。认证请求提交一次，之后通过只读查询确认状态。

注销接受后复核离线状态。改密返回服务器处理结果，新密码认证由使用者另行发起。

## 当前部署行为

| 功能 | 当前结果 |
|---|---|
| PC 有线认证 | 服务器接受，按源 IP 读回目标账号在线，同源外网通达 |
| 手机与平板参数 | 离线协议测试通过，实际接入待验证 |
| 门户免登录桥 | 类型 1 返回签名链接，zfw 登录后读回目标账号身份 |
| 指定连接下线后重新认证 | zfw 下线连接后由 p 登录，恢复目标账号在线 |
| 门户注销 | 在线时返回 `获取用户在线信息数据为空！`，命令返回拒绝结果 |
| 补充错误查询 | 服务端返回 Radius 错误查询失败 |
| 条件改密 | 请求与响应已实现并有离线测试，真实账号改密待验证 |

当前网页还携带以下产品模块，其动态配置状态如下：

| 模块 | 当前范围 |
|---|---|
| 门户用量与记录 `page/loadUserInfo` 等 | 当前相关展示未启用；实际账号与账单由 zfw 提供 |
| 门户 MAC、并发终端管理 | 当前解绑与并发验证开关关闭；与 zfw MAC 接口分开 |
| 感知登录、公共临时账号 | 当前相关开关关闭 |
| 短信、扫码、第三方认证 | 普通账号页面没有对应认证入口 |
| 访客 `visitor/*`、普教 `educae_join/*` | 当前 `register_mode=1`；访客、普教分别对应模式 3/4 |
| IPv6 辅助记录 | 当前 IPv6 功能未启用，脚本包含辅助日志路径 |
| 场景、日历、APG 联动 | 当前相关开关关闭 |

动态配置与前端路径可通过 [research](../research/README.md) 分别复现。新增功能研究从对应开关、模板和请求参数开始，再核对服务端响应及状态变化。

## 前端可见路径目录

从 `a41.js` 递归取得 25 份脚本与 20 个模板。四个主脚本 `a40.js`、`a41.js`、`a43.js`、`a44.js` 包含 67 条不同路径：62 条 JSONP、2 条资源、3 条导航。扩展脚本 `a64.js` 另含 2 条 OAuth 路径，合计 69 条。下表路径相对 `/eportal/portal/`；当前 CLI 调用见上文“接口与命令”，各模块配置见“当前部署行为”。

| 模块 | 路径 | 用途 |
|---|---|---|
| 普通认证 | `page/loadConfig` | 动态配置 |
| 普通认证 | `online_list` | 在线状态 |
| 普通认证 | `login` | 账号认证 |
| 普通认证 | `logout` | 终端注销 |
| 信息 | `page/loadUserInfo` | 门户用户信息 |
| 信息 | `page/loadOnlineRecord` | 在线记录 |
| 信息 | `page/loadLogonRecord` | 上下线记录 |
| 信息 | `page/loadRechargeRecord` | 收费或充值记录 |
| 错误 | `err_code` | 认证错误 |
| 错误 | `err_code/loadErrorPrompt` | 错误提示翻译 |
| 公告 | `notice/get_notice_content` | 公告内容 |
| 终端上下文 | `index/getClientIp` | 接入 IP |
| MAC | `mac/find` | 绑定或在线 MAC 查询 |
| MAC | `mac/unbind` | 注销并解除绑定 |
| 并发终端 | `login_verify/unbind` | 解除并发终端限制 |
| 感知 | `perceive` | 感知认证 |
| 临时账号 | `login/short_login` | 临时账号认证 |
| 短信 | `sms` | 短信验证码 |
| 场景认证 | `sms/check_hotel` | 酒店账号校验 |
| 图片校验 | `captcha/check` | 校验图片答案 |
| 密码 | `change_pass` | 条件改密 |
| 扫码 | `qrcode/create` | 生成二维码 |
| 扫码 | `qrcode` | 扫码端授权 |
| 第三方 | `dingtalk/create` | 钉钉入口 |
| 第三方 | `wechat_work/create` | 企业微信入口 |
| 第三方 | `miniprogram/create` | 小程序二维码 |
| 自助服务 | `self` | 自助入口与免登录桥 |
| 访客 | `visitor/authByQRCode` | 二维码登记 |
| 访客 | `visitor/authByRemote` | 远程审核登记 |
| 访客 | `visitor/checkAuditQRcode` | 审核二维码状态 |
| 访客 | `visitor/checkUnlogin` | 免登录参数校验 |
| 访客 | `visitor/checkUserAuditState` | 账号审核状态 |
| 访客 | `visitor/checkUserStateByIP` | 按 IP 查询开通状态 |
| 访客 | `visitor/getAuditRegVcode` | 团体注册校验码 |
| 访客 | `visitor/groupScanQRCodeAuth` | 团体扫码授权 |
| 访客 | `visitor/loadLoginRecord` | 登录记录 |
| 访客 | `visitor/loadOnlineTerm` | 在线终端 |
| 访客 | `visitor/loadTopupRecord` | 充值记录 |
| 访客 | `visitor/loadUserInfo` | 账号信息 |
| 访客 | `visitor/registerByQrcode` | 二维码注册 |
| 访客 | `visitor/sendUrlToPhone` | 发送充值链接 |
| 访客 | `visitor/sms` | 登记验证码 |
| 普教 | `educae_join/authByQRCode` | 二维码登记 |
| 普教 | `educae_join/authByRemote` | 远程审核登记 |
| 普教 | `educae_join/checkAuditQRcode` | 审核二维码状态 |
| 普教 | `educae_join/checkUnlogin` | 免登录参数校验 |
| 普教 | `educae_join/checkUserAuditState` | 账号审核状态 |
| 普教 | `educae_join/getAuditRegVcode` | 团体注册校验码 |
| 普教 | `educae_join/groupScanQRCodeAuth` | 团体扫码授权 |
| 普教 | `educae_join/loadLoginRecord` | 登录记录 |
| 普教 | `educae_join/loadOnlineTerm` | 在线终端 |
| 普教 | `educae_join/loadTopupRecord` | 充值记录 |
| 普教 | `educae_join/loadUserInfo` | 账号信息 |
| 普教 | `educae_join/registerByQrcode` | 二维码注册 |
| 普教 | `educae_join/sendUrlToPhone` | 发送充值链接 |
| 普教 | `educae_join/sms` | 登记验证码 |
| IPv6 | `ipv6/unloadLog` | IPv6 联动失败记录 |
| IPv6 | `ipv6Log` | IPv6 辅助记录 |
| 页面组件 | `scene/index` | 场景配置 |
| 页面组件 | `calendar/index` | 日历数据 |
| 导航 | `index` | 统一门户入口 |
| 资源 | `captcha` | 图片验证码 |
| 资源 | `lang` | 国际化资源 |
| 导航 | `dingtalk` | 钉钉授权结果导航 |
| 导航 | `miniprogram` | 小程序导航 |
| APG | `apg/get_active_conf` | 客户端检查配置 |
| APG | `apg/queryPageSet` | 页面与认证策略 |
| OAuth | `oauth/create` | OAuth 授权入口 |
| OAuth | `oauth/logout` | OAuth 退出 |

目录研究先定位每条路径对应的模板、功能开关、认证前提与参数。查询操作读取响应，认证和修改操作另外核对账号或终端状态。源码入口：[a40](https://p.njupt.edu.cn/a40.js)、[a41](https://p.njupt.edu.cn/a41.js)、[a43](https://p.njupt.edu.cn/a43.js)、[a44](https://p.njupt.edu.cn/a44.js)、[a64](https://p.njupt.edu.cn/a64.js)。
