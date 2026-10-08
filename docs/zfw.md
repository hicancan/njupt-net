# zfw 自助服务

自助服务基址为 `http://zfw.njupt.edu.cn:8080/Self/`。它管理账号资源，使用独立 Cookie 会话；管理登录与终端上网认证是两个生命周期。

## 管理会话

登录直接读取 `login/` 表单、隐藏字段与动态 action，初始化页面所需的 `login/randomCode` 图片上下文，然后 POST 到登录校验地址。普通账号登录输入账号与密码，图片上下文由内核初始化。

登录后核对 `/Self/dashboard` 与服务器账号身份。单次私有 CLI 命令登录、执行业务、退出 `login/logout`；[连续命令会话](architecture.md#连续命令会话)按账号别名保留独立 Cookie 与管理身份，在 `close` 或标准输入 EOF 时统一退出。上网终端通过 `p logout` 或 `zfw offline` 下线。

门户 `self_type=1` 返回当前终端在线账号的签名地址，可建立同一种管理会话。核心 `LoginBridge` 限定已确认的 zfw 主机、HTTP 8080、`/Self/login/eportalLogin` 及 `params/timestamp/sign` 三个参数，随后核对 dashboard 身份。CLI 用全局 `--self-url-file PATH` 明确选择该方式；文件仅含一行完整 URL，可带一个末尾换行。

```powershell
$bridge = njupt-net --interface 'Ethernet' --account default p self --type 1 | ConvertFrom-Json
$bridge.data.url | Set-Content -LiteralPath bridge-url.txt -NoNewline -Encoding utf8
njupt-net --interface 'Ethernet' --account default --self-url-file bridge-url.txt zfw account
```

URL 文件保存示例中的 `.data.url` 字符串。`--account` 选择期望账号；桥登录成功后 `verify` 返回 `identity_verified:true` 与 `login_method:portal_bridge`，密码登录成功后返回 `credentials_valid:true`。任一方法登录失败时返回错误。

| 相对路径 | 方法 | 用途 |
|---|---|---|
| `login/` | GET | 登录表单 |
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
| `verify` | 管理鉴权 | 按所选方法核对账号密码或门户签名身份 |
| `account` | GET `dashboard` | 账号概览 |
| `refresh` | GET `dashboard/refreshaccount` | 使用专用令牌刷新账号 |
| `profile` | GET `setting/personList` | 只读资料 |
| `online` | GET `dashboard/getOnlineList` | 当前账号在线连接 |
| `history` | GET `dashboard/getLoginHistory` | 近期登录记录 |
| `offline --session ID` | GET `dashboard/tooffline` | 提交前核对精确 session ID，接受后复核该 ID 消失 |
| `devices` | GET `service/getMacList` | 分页 MAC 绑定列表 |
| `unbind --mac ADDRESS` | GET `service/unbindmac` | 使用 MAC 页面专用令牌提交解绑 |

在线连接以 session ID 标识，MAC 绑定以 MAC 标识。下线连接与解除持久绑定分别执行。`offline` 提交前重新读取当前账号的在线列表，精确匹配指定 session ID；查询失败或该 ID 已不存在时，返回 `not_submitted`、`verified:false` 和错误。下线提交被接受后，只读查询目标 session ID，连续查询的起始时间至少相隔 50 毫秒，在十秒内观察该会话消失，确认后返回 `accepted`、`verified:true`。`devices` 接受页码及 `10/25/50/100` 页大小；`unbind` 使用十二位十六进制 MAC。

账号刷新返回 HTTP 200 空正文，`refresh` 成功时返回 `data:null`。概览通过 `account` 单独读取。Self 页面加载时也会调用刷新接口。运营商绑定的实测显示，执行账号刷新后，门户仍可能继续返回“未绑定运营商账号”；新绑定的认证生效时间由登录流程处理。

`getMacList` 直接返回 `total` 与 `rows`，已实测读回非空设备列表。每行的五列依次表示在线标记、MAC、终端类型、最近登录时间和最近 IP；服务器在线标记为字符串 `"0"` 或 `"1"`，公开模型的 `online` 为对应整数。读取列表直接调用该接口；`service/myMac` 用于 MAC 解绑前读取专用令牌。

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

当前部署中，同一移动宽带账号仍绑定旧校园账号时，直接绑定新校园账号会返回“已存在该运营商账号”。关系迁移依次解除旧校园账号的绑定、在目标校园账号下绑定同一宽带。切换当前终端的上网身份还需完成旧连接下线与目标账号认证。

以下示例组合这两类动作。`OLD_ACCOUNT`、`NEW_ACCOUNT` 为配置中的账号键；在旧账号的 `online` 结果中按终端 IP 与 MAC 选定对应的 `SESSION_ID`。配置中的 `broadband_account.operator` 设为 `cmcc`。

```powershell
njupt-net --interface 'Ethernet' --account OLD_ACCOUNT zfw online
njupt-net --interface 'Ethernet' --account OLD_ACCOUNT zfw offline --session SESSION_ID
njupt-net --interface 'Ethernet' --account OLD_ACCOUNT zfw operator --unbind cmcc
njupt-net --interface 'Ethernet' --account NEW_ACCOUNT zfw operator --bind
njupt-net --interface 'Ethernet' --account NEW_ACCOUNT p login --operator cmcc
njupt-net --interface 'Ethernet' p status
njupt-net --interface 'Ethernet' probe
```

运营商解绑改变校园账号的宽带关系；终端下线使用 `offline --session`，MAC 解绑使用 `unbind --mac`。绑定提交后通过 Self 表单核对账号关系，登录提交后通过 p 在线列表核对终端身份。

Windows 用户可通过 [可选 PowerShell 7 登录示例](../examples/windows/README.md) 指定目标账号：`login.ps1 -Account ACCOUNT_ALIAS`。目标已绑定配置中的宽带且密码已设置时保留；目标绑定完全为空时，查询其他配置账号，存在唯一持有人便迁移，无持有人便直接绑定。当前终端登录账号与宽带持有人分别确定。

脚本分别判断门户身份与目标宽带关系。门户已经是目标账号和运营商时，保留当前连接，仅处理尚需迁移或绑定的宽带关系；改绑后重新读取门户，核对目标身份与 MAC，实际离线时再认证。其他账号或运营商在线时，按本机 IP 与 MAC 选择唯一 Self 会话，下线前重新观察绑定及在线身份，清退后确认门户离线。门户原本离线时，继续处理目标绑定和认证。

绑定操作准确读回账号与密码，登录操作返回核对后的终端身份；指定 `-Probe` 时另外观测公网通达性。宽带迁移改变校园账号的账号级关系，所需上网认证由门户当前身份决定。

需要认证时，脚本在新绑定后立即登录；仅本次新绑定且门户准确返回 `rejected`、`verified:false` 与完整提示“未绑定运营商账号,请正确绑定运营商账号再试！”时，收到完整拒绝便立即串行发起下一次认证，成功即结束这段观察。`BindingTimeoutSeconds` 默认 45 秒，从绑定完成起计时。每次内核登录调用提交一次请求，认证次数与生效实际用时随流程结果返回。

CSRF 令牌按操作读取：刷新使用 dashboard 内联调用令牌；消费保护与运营商绑定使用各自隐藏表单令牌；MAC 解绑使用 MAC 页面内联令牌。无感知查询返回包含 HTML 操作链接的 JSON 字符串，内核从链接恢复下一次动作。

修改操作区分 `accepted`、`rejected`、`unknown`、`not_submitted`，`verified` 单独表示最终状态是否确认。消费限额提交后通过独立 GET 读回实际限额；运营商绑定或解绑通过独立 GET 读取表单，绑定核对选定账号与密码一致，解绑核对两字段均为空，同时确认另一运营商字段保持原值。

POST 跳转到对应结果页时，跳转中的 GET 已完成独立读取，内核直接解析该页完成核验；POST 直接返回页面时，内核另行 GET 当前业务页核验。结果输出账号与密码是否已设置等字段。无感知操作重新查询策略，指定连接下线只读核对目标会话消失。

MAC 解绑先从专用页面取得令牌，再分页定位目标。目标已不存在时返回 `not_submitted`、`verified:true`；前置列表读取失败时返回 `not_submitted` 和具体错误。服务器结构化 `state:true/false` 分别映射为 `accepted/rejected`，缺少该字段则为 `unknown`。明确接受后，内核分页确认目标消失，再设为 `verified:true`；明确拒绝或结果未知时直接返回服务器处理结果。证明目标不存在需要读完全部页，分页期间总数、行数及 MAC 唯一性保持一致。

## 账单

| `--kind` | 页面 | 查询 | 导出 |
|---|---|---|---|
| `online` | `bill/userOnlineLog` | `bill/getUserOnlineLog` | `bill/exportUserOnlineLog` |
| `monthly` | `bill/monthPay` | `bill/getMonthPay` | `bill/exportMonthPay` |
| `operations` | `bill/operatorLog` | `bill/getOperatorLog` | `bill/exportOperatorLog` |

以上均使用 GET。`bills` 返回对应类别的账单模型；上网记录、业务记录和指定年份的月账单直接调用查询接口。月账单省略年份时，先从 `bill/monthPay` 的年份选择框读取服务器默认年份，再查询。

`total`、`rows` 与 `summary` 分别保留服务器返回的总数、记录与汇总。各字段表达各自的服务器统计含义，内核按原值返回。

`export --output PATH` 创建 XLS 文件。`--all` 直接导出所选时间或年份范围；当前页导出先在同一管理会话查询目标页，建立服务器当前页导出上下文。

| 参数 | 规则 |
|---|---|
| `--start`、`--end` | 上网与业务记录使用 `YYYY-MM-DD`；日期差不超过 60 天，结束不晚于上海当天 |
| `--year` | 月账单年份，范围 `1..9999`；省略时使用服务器页面的默认年份 |
| `--page` | 正整数，默认 1 |
| `--size` | 上网记录 `10/20/50/100`；另两类 `10/25/50/100` |
| `--sort`、`--order` | 上网默认 `loginTime/DESC`；月账单 `0..7`；业务记录 `0/1/2/4`；方向 `ASC/DESC` |

当前服务器附件使用未加引号的中文 `.xls` 文件名与 `file;charset=UTF-8` 类型。内核核对附件契约和 OLE 文件头后保存文件；导出数据由所选账号、时间范围与服务器记录决定。

## 公开页面

`notice`、`help`、`agreement` 分别 GET `unlogin/notice`、`unlogin/help`、`unlogin/agreement`，使用公开页面。公告与协议直接读取；帮助先按需通过登录页建立站点 Cookie，再从帮助外框取得实际 iframe 地址。当前嵌入地址为 `/Self/unlogin/helpinfo/0`，内容页返回 HTTP 200 与“暂无使用帮助信息”。结果表达 `available:false`，并提供实际内容地址与 HTTP 状态。

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
| MAC 列表 | 分页读取成功，非空设备记录已按实际五列契约解析 |
| 运营商绑定与解绑 | 通过专用表单提交，独立读回选定运营商字段与另一运营商字段 |
| MAC 解绑 | 请求和离线测试已实现，真实账号修改待验证 |
| 充值 | 入口显示服务菜单，未提供付款表单，返回不可用状态 |

网页相对充值链接会拼接成 `service/service/userRecharge`；CLI 直接读取表中确认的 `service/userRecharge`。部署与账号权限变化时，可通过 [research](../research/README.md) 重新读取页面和复核接口。

全部命令的请求前提、提交与确认见 [原子动作与请求边界](../research/requests.md)。
