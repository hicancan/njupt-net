# p 门户

上网认证门户位于 `https://p.njupt.edu.cn/`，API 路径为 `/eportal/portal/`。p 命令直接查询或修改选定源 IPv4 的终端状态。

## 原生接口与配置

四个 API 入口使用相同业务路径，通过 `--port` 明确选择：

| 端口 | 协议 | 基址 |
|---:|---|---|
| 801 | HTTP | `http://p.njupt.edu.cn:801/eportal/portal/` |
| 802 | HTTPS | `https://p.njupt.edu.cn:802/eportal/portal/` |
| 803 | HTTP | `http://p.njupt.edu.cn:803/eportal/portal/` |
| 804 | HTTPS | `https://p.njupt.edu.cn:804/eportal/portal/` |

CLI 默认选择 804。`p status` 直接调用 `online_list`，`p config` 直接调用 `page/loadConfig`；两项查询只携带 `callback`。服务器从连接识别接入地址，内核按固定源 IPv4 匹配返回会话。JSONP 回调名使用 `dr` 加序号，响应必须匹配本次回调。

认证使用 `login_method=1`。`page/loadConfig` 独立提供门户功能配置。HTTPS 使用服务域名验证证书。

## 接口与命令

以下路径均相对 API 基址；当前协议使用 GET，JSONP 结果或图片资源。

| 路径 | 用途 | CLI |
|---|---|---|
| `page/loadConfig` | 获取动态门户配置 | `p config` |
| `online_list` | 查询目标接入 IP 对应会话 | `p status` |
| `login` | 提交账号密码与终端上下文 | `p login` |
| `logout` | 注销当前终端 | `p logout` |
| `err_code` | 获取认证补充错误 | `p error` |
| `self` | 获取自助服务入口 | `p self --type 0\|1\|2` |
| `captcha` | 获取改密图片资源 | `p password` 内部步骤；核心 `Captcha` |
| `change_pass` | 提交条件改密 | `p password` |

普通认证使用账号、密码与运营商选择。`campus` 使用校园账号，`njxy` 为电信，`cmcc` 为移动。配置保存原始账号，内核按终端类型添加账号前缀，再附加运营商后缀。登录直接提交以下八个字段：

| 字段 | 值 |
|---|---|
| `callback` | 本次 JSONP 回调名 |
| `enable_r3` | `0` |
| `login_method` | `1` |
| `terminal_type` | 所选终端类型 `1..4` |
| `user_account` | `,0,账号`；手机使用 `,1,账号`，运营商后缀附在账号后 |
| `user_password` | 原始密码，由 URL 编码传输 |
| `wlan_user_ip` | 选定源 IPv4 |
| `wlan_user_ipv6` | 空字符串，保留字段 |

`p login` 完成本地参数校验后直接提交一次认证。服务器接受后，内核查询目标在线身份；服务器拒绝时，命令返回该次原生拒绝。

所有 p 命令接受 `--terminal pc|mobile|hipad|vipad`，默认 `pc`，分别对应终端类型 1 至 4。四种终端通过同一套请求流程传入不同参数。

所有 p 命令也接受 `--port 801|802|803|804`。同一 CLI 会话分别保存各端口、终端类型的 Portal 上下文。

`self_type`：`0` 自助登录入口，`1` 当前在线账号的免登录桥，`2` 密码页面入口。`p self` 直接提交回调名、类型、账号与密码四个字段并返回地址。类型 1 的签名身份由当前终端的在线账号决定；交给 zfw 后仍须核对期望账号，使用方法见 [管理会话](zfw.md#管理会话)。

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
| `not_submitted` | 修改前的必要检查已确定无需提交，例如注销时终端已经离线 |

`verified` 表示最终目标状态是否确认。登录接受后立即查询在线状态，连续查询的起始时间至少相隔 50 毫秒，最长观察十秒。只有选定源 IPv4 的在线账号与提交的账号、运营商后缀一致，才设为 `verified:true`；观察持续至目标身份出现。观察超时保留服务器的 `accepted`、`verified:false` 和最后读到的状态，并返回核验错误。

终端已经在线时，认证请求仍由服务器处理，命令保留实际响应中的 `result`、`msg` 和 `ret_code` 所表达的结果。外层登录流程先观察当前身份，决定是否需要发起认证。

服务器返回 `ret_code` 时，修改结果保留该字段的原始 JSON 值与类型；未返回时省略。它与 `message` 一起提供原生响应信息，`outcome` 仍由服务器的 `result` 判定。门户前端的错误码对照将 `ret_code=2` 解释为“终端 IP 已经在线”；例如 `result=0`、`msg=AC999`、`ret_code=2` 会输出 `outcome:rejected`、`message:AC999` 和 `ret_code:2`。

提交响应丢失或无法解析时，命令立即返回 `unknown`。调用者可显式执行 `p status` 观察当前终端；这次观察与原提交结果分别保存。

注销前读取当前终端状态与 MAC，并按需读取原生配置；接受后复核离线状态。改密返回服务器处理结果，新密码认证由使用者另行发起。每项动作的请求前提与确认职责见 [原子动作与请求边界](../research/requests.md)。

## 当前部署行为

| 功能 | 当前结果 |
|---|---|
| PC 有线认证 | 服务器接受，按源 IP 读回目标账号在线，同源外网通达 |
| 四种终端参数 | 同一有线接入使用四种终端参数均被接受，并核对到目标在线账号 |
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
