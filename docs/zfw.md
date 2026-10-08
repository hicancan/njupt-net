# zfw 自助服务

自助服务基址为 `http://zfw.njupt.edu.cn:8080/Self/`。它管理账号资源，使用独立 Cookie 会话；管理登录与终端上网认证是两个生命周期。

## 管理会话

登录读取实际表单、隐藏字段与动态 action，初始化页面所需的 `login/randomCode` 图片上下文，然后 POST 到登录校验地址。普通账号登录输入账号与密码，图片上下文由内核初始化。

登录后核对 `/Self/dashboard` 与服务器账号身份。每条私有 CLI 命令独立登录、执行业务、退出 `login/logout`，Cookie 仅存在于该命令的管理会话。上网终端通过 `p logout` 或 `zfw offline` 下线。

门户 `self_type=1` 返回的签名地址可建立同一种管理会话。核心 `LoginBridge` 限定已确认的 zfw 主机、HTTP 8080、`/Self/login/eportalLogin` 及 `params/timestamp/sign` 三个参数，随后核对 dashboard 身份。CLI 用全局 `--self-url-file PATH` 明确选择该方式；文件仅含一行完整 URL，可带一个末尾换行。

```powershell
$bridge = njupt-net --interface 'Ethernet' --account default p self --type 1 | ConvertFrom-Json
$bridge.data.url | Set-Content -LiteralPath bridge-url.txt -NoNewline -Encoding utf8
njupt-net --interface 'Ethernet' --account default --self-url-file bridge-url.txt zfw account
```

URL 文件保存示例中的 `.data.url` 字符串。`--account` 选择期望账号；桥登录成功后 `verify` 返回 `identity_verified:true` 与 `login_method:portal_bridge`，密码登录成功后返回 `credentials_valid:true`。任一方法登录失败时返回错误。

| 相对路径 | 方法 | 用途 |
|---|---|---|
| `login` | GET | 登录表单 |
| `login/randomCode` | GET | 初始化图片上下文 |
| `login/verify` | POST | 提交当前动态表单 |
| `login/eportalLogin` | GET | 消费门户签名桥并核对身份 |
| `login/logout` | GET | 退出管理会话 |
| `login/changeLanguage` | GET | 修改当前会话语言 |

语言的实际值为 `English`、`zh_cn`，两种语言均已通过服务端请求切换并读回。核心 `Session.Language` 设置并核对当前会话，适合持续使用同一会话的 Go 调用者。公开页面语言按钮出现 `ctx` 未定义的 JavaScript 错误，点击后未发出请求。

## 菜单与路径目录

当前页面、表单、业务脚本与嵌入内容共发现 39 条 Self 路径，包含业务调用、导航和图片资源，按不同路径去重。本文各节列出完整目录，路径均相对 `/Self/`。

| 相对路径 | 方法 | 用途 |
|---|---|---|
| `bill` | GET | 账单菜单 |
| `service` | GET | 服务菜单 |
| `setting` | GET | 设置菜单 |

三个菜单用于发现业务页面。CLI 按账号、连接、策略和账单组织命令，直接返回相应业务结果。

## 账号与连接

| CLI | 相对路径与方法 | 行为 |
|---|---|---|
| `verify` | 管理登录与退出 | 按所选方法核对账号密码或门户签名身份 |
| `account` | GET `dashboard` | 账号概览 |
| `refresh` | GET `dashboard/refreshaccount` | 使用专用令牌刷新，再读取概览 |
| `profile` | GET `setting/personList` | 只读资料 |
| `online` | GET `dashboard/getOnlineList` | 当前账号在线连接 |
| `history` | GET `dashboard/getLoginHistory` | 近期登录记录 |
| `offline --session ID` | GET `dashboard/tooffline` | 下线指定连接，并复核该 session ID 消失 |
| `devices` | GET `service/myMac`、`service/getMacList` | 分页 MAC 绑定列表 |
| `unbind --mac ADDRESS` | GET `service/unbindmac` | 使用 MAC 页面专用令牌提交解绑 |

在线连接以 session ID 标识，MAC 绑定以 MAC 标识。下线连接与解除持久绑定分别执行。`devices` 接受页码及 `10/25/50/100` 页大小；`unbind` 使用十二位十六进制 MAC。

账号刷新返回 HTTP 200 空正文，内核随后读取概览。Self 页面加载时也会调用该接口。运营商绑定的实测显示，执行账号刷新后，门户仍可能继续返回“未绑定运营商账号”；新绑定的认证生效时间由登录流程处理。`getMacList` 约定返回 JSON 列表，当前服务器返回 HTTP 200 空正文，命令因此返回协议错误。

当前资料页呈现只读标签和值，`setting/updateUserSecurity` 表单只有令牌，提交后显示 `Not valid!`。`profile` 读取该页面的资料，结果包含 `read_only:true`。

## 策略与账号关系

| CLI | 相对路径 | 行为 |
|---|---|---|
| `consume` | `service/consumeProtect` | 读取消费保护 |
| `consume --limit AMOUNT` | POST `service/changeConsumeProtect` | 提交专用表单并核对实际限额 |
| `operator` | `service/operatorId` | 读取运营商绑定 |
| `operator --bind` | POST `service/bind-operator` | 提交配置中选定的运营商账号，保留另一运营商字段 |
| `operator --unbind njxy\|cmcc` | POST `service/bind-operator` | 清空选定运营商的账号与密码，保留另一运营商字段 |
| `mauth` | `dashboard/refreshMauthType` | 读取无感知策略 |
| `mauth --change` | GET `dashboard/oprateMauthAction` | 执行一次当前策略动作，再核对变化 |
| `recharge` | `service/userRecharge` | 检查实际充值入口 |

消费限额为非负十进制金额，最多三位小数，`999999` 表示不限。运营商表单包含电信 `FLDEXTRA1/2` 和移动 `FLDEXTRA3/4`；`operator --bind` 修改配置选定的运营商字段，`operator --unbind njxy|cmcc` 将选定的一组账号与密码同时置空。两种操作互斥，均先 GET 当前表单，使用其中的 `csrftoken`，保留另一组字段，再 POST 到同一 `service/bind-operator` 地址。解绑目标的账号和密码均已为空时，返回 `not_submitted`、`verified:true`。

当前部署中，同一移动宽带账号仍绑定旧校园账号时，直接绑定新校园账号会返回“已存在该运营商账号”。转移时，先下线待切换终端的旧会话，解除旧校园账号的移动绑定，再在新校园账号下绑定并认证。以下命令中的 `OLD_ACCOUNT`、`NEW_ACCOUNT` 为配置中的账号键；在旧账号的 `online` 结果中按终端 IP 与 MAC 选定对应的 `SESSION_ID`。配置中的 `broadband_account.operator` 设为 `cmcc`。

```powershell
njupt-net --interface 'Ethernet' --account OLD_ACCOUNT zfw online
njupt-net --interface 'Ethernet' --account OLD_ACCOUNT zfw offline --session SESSION_ID
njupt-net --interface 'Ethernet' --account OLD_ACCOUNT zfw operator --unbind cmcc
njupt-net --interface 'Ethernet' --account NEW_ACCOUNT zfw operator --bind
njupt-net --interface 'Ethernet' --account NEW_ACCOUNT p login --operator cmcc
njupt-net --interface 'Ethernet' p status
njupt-net --interface 'Ethernet' probe
```

运营商解绑改变校园账号的宽带关系；终端下线使用 `offline --session`，MAC 解绑使用 `unbind --mac`。以上移动宽带迁移流程已完成校园网实测，解绑、重新绑定和新账号认证均通过独立状态核对，同一源地址的公网检查成功。

Windows 用户可通过 [可选 PowerShell 7 登录示例](../examples/windows/README.md) 指定目标账号：`login.ps1 -Account ACCOUNT_ALIAS`。目标已绑定配置中的宽带且密码已设置时保留；目标绑定完全为空时，查询其他配置账号，存在唯一持有人便迁移，无持有人便直接绑定。当前终端登录账号与宽带持有人分别确定。需要下线时，脚本内部按本机 IP 与 MAC 选择唯一 Self 会话；门户已离线时，先确认目标账号及宽带持有人在该源地址上没有会话。最终核对目标身份、公网与绑定状态。宽带迁移改变校园账号的账号级关系。

CSRF 令牌按操作读取：刷新使用 dashboard 内联调用令牌；消费保护与运营商绑定使用各自隐藏表单令牌；MAC 解绑使用 MAC 页面内联令牌。无感知查询返回包含 HTML 操作链接的 JSON 字符串，内核从链接恢复下一次动作。

修改操作区分 `accepted`、`rejected`、`unknown`、`not_submitted`，`verified` 单独表示最终状态是否确认。消费限额提交后独立 GET 读回限额；运营商绑定或解绑提交后独立 GET 读取表单，绑定核对选定账号与密码一致，解绑核对两字段均为空，同时确认另一运营商字段保持原值。结果仅输出账号与密码是否已设置等非密码字段。无感知操作重新查询策略，指定连接下线只读核对目标会话消失。

MAC 解绑先完整读取当前绑定。目标已不存在时返回 `not_submitted`、`verified:true`；列表异常时，明确指定的 MAC 仍可提交一次。服务器结构化 `state:true/false` 分别映射为 `accepted/rejected`，缺少该字段则为 `unknown`。提交后完整读回各页，目标消失时设为 `verified:true`，同时保留原 `outcome`。提交后的成功结果需要 `accepted` 与 `verified:true` 同时成立；列表为空正文、不完整或重复页时，返回已知结果与协议错误。

## 账单

| `--kind` | 页面 | 查询 | 导出 |
|---|---|---|---|
| `online` | `bill/userOnlineLog` | `bill/getUserOnlineLog` | `bill/exportUserOnlineLog` |
| `monthly` | `bill/monthPay` | `bill/getMonthPay` | `bill/exportMonthPay` |
| `operations` | `bill/operatorLog` | `bill/getOperatorLog` | `bill/exportOperatorLog` |

以上均使用 GET。`bills` 返回明确账单模型；`export --output PATH` 创建 XLS 文件。`--all` 导出所选时间或年份范围，否则先在同一管理会话查询目标页，建立服务器当前页导出上下文。

| 参数 | 规则 |
|---|---|
| `--start`、`--end` | 上网与业务记录使用 `YYYY-MM-DD`；日期差不超过 60 天，结束不晚于上海当天 |
| `--year` | 月账单年份，且必须存在于实际页面 |
| `--page` | 正整数，默认 1 |
| `--size` | 上网记录 `10/20/50/100`；另两类 `10/25/50/100` |
| `--sort`、`--order` | 上网默认 `loginTime/DESC`；月账单 `0..7`；业务记录 `0/1/2/4`；方向 `ASC/DESC` |

当前服务器附件使用未加引号的中文 `.xls` 文件名与 `file;charset=UTF-8` 类型。内核核对附件契约和 OLE 文件头后保存文件；导出数据由所选账号、时间范围与服务器记录决定。

## 公开页面

`notice`、`help`、`agreement` 分别 GET `unlogin/notice`、`unlogin/help`、`unlogin/agreement`，不要求账号。匿名读取先建立站点所需 Cookie。帮助页实际嵌入 `/Self/unlogin/helpinfo/0`，该内容页返回 HTTP 200 与“暂无使用帮助信息”；读取帮助包含实际嵌入内容，结果表达 `available:false`，并提供实际内容地址与 HTTP 状态。

## 当前服务行为

| 功能 | 当前结果 |
|---|---|
| 账号、资料、在线连接、近期记录、策略查询、刷新 | 校园请求成功，按具体业务模型返回 |
| 三类账单与 XLS 导出 | 查询与导出成功 |
| 门户签名桥 | 建立管理会话，读回目标账号身份并退出 |
| 指定连接下线 | 读回目标 session ID 消失，再由 p 恢复认证成功 |
| 无感知策略 | 修改后读回变化，并恢复原策略 |
| 管理会话语言 | `English`、`zh_cn` 切换与读回成功 |
| 消费保护 | 以原值 `999999` 提交，服务器接受且读回一致；其他限额修改待验证 |
| MAC 列表 | HTTP 200 空正文，命令返回协议错误 |
| 移动宽带解绑与跨校园账号迁移 | 旧账号解绑、新账号绑定均被接受且读回一致，新账号门户认证及同源公网检查成功 |
| MAC 解绑 | 请求和离线测试已实现，真实账号修改待验证 |
| 充值 | 入口显示服务菜单，未提供付款表单，返回不可用状态 |

网页相对充值链接会拼接成 `service/service/userRecharge`；CLI 直接读取表中确认的 `service/userRecharge`。部署与账号权限变化时，可通过 [research](../research/README.md) 重新读取页面和复核接口。
