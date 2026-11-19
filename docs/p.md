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

CLI 默认选择 HTTPS 802。`p status` 直接调用 `online_list`，`p config` 直接调用 `page/loadConfig`。两项查询只携带 `callback`。服务器从连接识别接入地址，内核按固定源 IPv4 匹配返回会话。JSONP 回调名使用 `dr` 加序号，响应必须匹配本次回调。

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
| `err_code/loadErrorPrompt` | 翻译已知错误码 | `p error-prompt --code CODE` |
| `notice/get_notice_content` | 查询门户公告 | `p notice` |
| `self` | 获取自助服务入口 | `p self --type 0\|1\|2` |
| `captcha` | 获取验证码图片 | `p captcha --output PATH`，也由 `p password` 获取 |
| `captcha/check` | 校验本会话验证码答案 | `p captcha-check --answer ANSWER` |
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

需要显式启用 Self 归属读回时可传 `--verify-with-self`。门户已接受且在线账号脱敏时，CLI 使用所选账号建立或复用 Self 管理会话，要求同一 IP 和 MAC 对应唯一 Self 在线连接，并重新确认门户会话未改变。结果用 `account_verified` 和 `operator_verified` 区分证据。只确认归属时命令可成功返回，`verified` 仍为 `false`。

所有 p 命令接受 `--terminal pc|mobile|hipad|vipad`，默认 `pc`，分别对应终端类型 1 至 4。四种终端通过同一套请求流程传入不同参数。

所有 p 命令也接受 `--port 801|802|803|804`。同一 CLI 会话分别保存各端口、终端类型的 Portal 上下文。

`self_type`：`0` 自助登录入口，`1` 当前在线账号的免登录桥，`2` 密码页面入口。`p self` 直接提交回调名、类型、账号与密码四个字段并返回地址。类型 1 的签名身份由当前终端的在线账号决定；交给 zfw 后仍须核对期望账号，使用方法见 [管理会话](zfw.md#管理会话)。

改密命令从单行文件读取新密码，在同一 Portal 上下文获取图片、读取标准输入答案并提交一次：

```text
njupt-net --interface "Ethernet" --account default p password --new-password-file new-password.txt --captcha-output captcha.png
```

独立校验使用连续命令会话，同一端口与终端类型先执行 `p captcha --output PATH`，随后执行一次 `p captcha-check --answer ANSWER`。下一次校验需要重新取图。独立校验与改密分别使用各自的验证码上下文。

## 状态与结果

`p error` 以 `ip` 和 `mac` 查询本终端的 Radius 错误。已有原生错误码时，可以单独执行 `p error-prompt --code AC999` 查询提示。

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

指定 `p login --verify-with-self` 后，若门户接受认证但脱敏账号，CLI 通过目标账号的 Self 会话按 IP 和 MAC 核对唯一连接，再确认门户终端未变化。结果分别报告 `account_verified` 和 `operator_verified`。账号确认成功时命令成功；门户省略运营商时，`operator_verified` 和总体 `verified` 仍为 `false`。

服务器返回 `ret_code` 时，修改结果保留该字段的原始 JSON 值与类型；未返回时省略。它与 `message` 一起提供原生响应信息，`outcome` 仍由服务器的 `result` 判定。门户前端的错误码对照将 `ret_code=2` 解释为“终端 IP 已经在线”；例如 `result=0`、`msg=AC999`、`ret_code=2` 会输出 `outcome:rejected`、`message:AC999` 和 `ret_code:2`。

提交响应丢失或无法解析时，命令立即返回 `unknown`。调用者可显式执行 `p status` 观察当前终端；这次观察与原提交结果分别保存。

注销前读取当前终端状态与 MAC，并按需读取原生配置；接受后复核离线状态。改密返回服务器处理结果，新密码认证由使用者另行发起。

## 当前部署行为

| 功能 | 当前结果 |
|---|---|
| 门户服务端口 | 有线 `801` 与 `802` 的配置及在线状态读取通过。当前有线访问 `803` 与 `804` 时，部署服务器拒绝 TCP 连接 |
| PC 有线认证 | 门户可能脱敏账号。可通过 Self 按 IP 与 MAC 确认连接归属，运营商以门户报告为准 |
| 门户免登录桥 | 签发和消费是两次独立操作。Self 跳回登录表单时报告签名桥失败 |
| 门户注销 | 认证端接受后读取门户确认离线 |
| 补充错误查询 | 带本机 IP 与 MAC 的查询返回“从Radius获取错误代码出现异常！”。独立错误码翻译请求通过 |
| 条件改密 | 请求与响应已实现并有离线测试，真实账号改密待验证 |
| 公告 | 原生响应确认当前为空，命令返回 `available:false` |
| 验证码 | 读取图片已实测，错误答案返回原生拒绝。正确答案分支待专用测试 |

当前 CLI 只注册已确认的服务契约。受部署开关限制的功能以门户配置及服务端响应为准。
