# 原子动作与请求边界

内核按一个业务动作组织请求：取得必要的身份、令牌或当前值，提交一次动作，再确认该动作对应的目标状态。CLI 选择动作与参数，外层流程选择账号迁移顺序、再次认证和恢复步骤。

本文覆盖当前 CLI 命令，以及直接供 Go 调用的图片与管理语言操作。p 路径相对 `/eportal/portal/`，zfw 路径相对 `/Self/`；部署地址见 [网络部署](../docs/deployment.md)，字段契约见 [p](../docs/p.md) 与 [zfw](../docs/zfw.md)。

## 请求承担的职责

| 职责 | 取得或改变的内容 | 结果的使用方式 |
|---|---|---|
| 本地选择 | 参数、账号凭据、源 IPv4 与唯一启用网卡 | 固定本次调用的对象和接入链路 |
| 管理鉴权 | 独立 Cookie 会话与服务器确认的账号身份 | 同一账号的后续私有请求复用该身份 |
| 操作前提 | 专用令牌、待保留字段、精确连接或绑定 | 确定这次提交的内容与目标 |
| 动作提交 | 一次原生认证、修改或下线请求 | 保留该次服务器接受、拒绝或未知的结果 |
| 状态确认 | 提交后的在线身份、绑定、限额或策略 | 确定对应目标是否已经出现 |

读取承担具体职责时才进入调用链。已有响应已经提供同一职责所需的数据时，直接使用该响应；操作所需的当前状态和令牌在相应动作发生前取得。

提交结果与状态确认分别保存。明确接受后可以只读观察目标变化；提交响应丢失或无法解析时，修改结果立即返回 `unknown`。后续显式查询提供新的状态观察，原提交结果仍为未知。

## 链路、传输与进程

源 IPv4 属于唯一启用网卡，TCP 同时绑定该地址与网卡。p 与 zfw 直拨当前部署 IP，保留 URL、Host 和 HTTPS 证书身份。显式 `--source` 在创建 Link 时解析所属网卡；`--interface` 先完成网卡名称与地址选择。

HTTP 传输依据操作后果选择：

| 入口 | 请求规则 | 导航规则 |
|---|---|---|
| `network.Read` | 已确认的只读 GET，复用连接 | 重定向返回错误 |
| `network.Request` | 一次 GET 或表单 POST，使用新连接 | 重定向返回错误 |
| `network.Navigate` | 初始动作提交一次 | 只跟随业务声明的同源 GET 结果页；307/308 返回错误 |

认证与修改使用新连接，保证具有修改效果的 GET 也按本次调用提交一次。只读客户端复用固定站点的连接；各账号拥有独立 CookieJar。HTTPS 新连接共享 TLS 会话缓存并继续验证证书。

| CLI | 请求前提 | 执行与结果 |
|---|---|---|
| `version` | 本地构建信息 | 返回版本、Go 版本、系统与架构 |
| `interfaces` | 本机网络接口 | 枚举名称、索引、启用状态与 IPv4 |
| 帮助 | 命令与参数定义 | 输出命令用法 |
| `probe` | 已选定 Link | 通过系统 DNS 解析 NCSI 域名，以所选网卡请求 `/connecttest.txt`，核对 HTTP 响应与正文 |
| `session` | 固定源地址、配置路径与超时 | 串行处理原有 CLI 命令；按账号复用管理会话，按端口与终端类型复用 Portal |
| 会话 `close` / 输入 EOF | 该进程建立的管理会话 | 逐一退出管理会话，关闭空闲连接，报告清理结果 |

`session` 只管理命令传输与资源生命周期。配置首次读取后形成进程内快照，Link 和私有管理会话按首次使用建立。单次私有 zfw 命令在业务结束后退出管理会话；连续命令在关闭时统一退出。

## p 原子动作

| CLI / Go 动作 | 必要前提 | 原生请求 | 结果确认 |
|---|---|---|---|
| `p config` / `Configure` | 源地址、终端类型、端口 | GET `page/loadConfig`，只带 JSONP 回调名 | 解析原生配置与当前部署字段 |
| `p status` / `Status` | 已选定 Link | GET `online_list`，只带 JSONP 回调名 | 核对 `list/total`，按源 IP 选出账号、MAC 与会话 |
| `p login` / `Login` | 本地校验账号、密码与运营商 | 直接 GET `login`，提交原生八字段 | 明确接受后只读查询，直至源 IP 的目标账号与运营商身份出现 |
| `p logout` / `Logout` | 查询当前终端及 MAC；按需读取注销所需配置 | GET `logout` 一次 | 明确接受后只读确认门户离线；原本离线返回 `not_submitted` |
| `p error` / `ErrorInfo` | 源 IP 与 Portal 当前 MAC | GET `err_code` | 解析原生错误提示与处理结果 |
| `p self --type 0\|1\|2` / `SelfURL` | 账号、密码、所选入口类型 | GET `self`，提交回调名、类型、账号与密码 | 返回服务器给出的地址与处理结果 |
| `Captcha` | 当前 Portal | GET `captcha`，携带图片随机参数 | 核对图片类型与非空内容 |
| `p password` / `ChangePassword` | 新密码、同一 Portal 的图片答案；按需读取专用配置 | GET `change_pass` 一次 | 保留服务器处理结果；新密码认证由调用者另行发起 |

`p login` 的终端类型与运营商都是请求参数。认证直接提交原生字段；配置、网页模板和启动脚本各自属于独立查询。

登录接受后的状态确认只做读取，连续查询起始时间至少相隔 50 毫秒，最长观察十秒。核验超时保留 `accepted`、`verified:false` 与最后状态。原生拒绝保留 `message` 和 `ret_code` 的值与类型；已有在线会话也由服务器处理这次认证请求。

`p password` 的 CLI 入口读取新密码文件、保存图片、接收答案，再调用改密。`Captcha` 与 `ChangePassword` 在 Go API 中分别可用，调用者负责交互和文件。

门户 `self` 的类型 1 使用当前终端在线身份生成签名地址。zfw 消费该地址时核对期望账号，签名生成与管理登录分别作为动作处理。

## zfw 管理身份

每项私有业务共用下面的一次鉴权。后续业务表格从已确认的管理身份开始列出请求。

| 动作 | 必要前提 | 请求顺序 | 身份确认 |
|---|---|---|---|
| 密码登录 / `Session.Login` | 独立 CookieJar，账号与密码 | GET `login/` 取动态表单；GET `login/randomCode` 初始化图片上下文；POST 当前 `login/verify` action；GET 声明的 dashboard 结果页 | 在结果页核对实际账号 |
| 签名桥 / `LoginBridge` | 调用者提供当前签名 URL 与期望账号 | GET `login/eportalLogin` 一次；GET 声明的 dashboard 结果页 | 在结果页核对期望账号 |
| 管理退出 / `Logout` | 本会话的鉴权状态 | GET `login/logout` 一次，跟随声明的站点入口或登录结果页 | 核对实际登录页与登录表单 |
| 管理语言 / `Language` | 同一 Cookie 会话，`English` 或 `zh_cn` | GET `login/changeLanguage` 一次；读取登录页 | 核对语言按钮的实际文本 |

登录页提供隐藏字段与动态 action；图片初始化影响当前会话，两者承担不同前提。dashboard 导航结果已经提供身份信息，管理鉴权直接解析该响应。

业务令牌从各自页面读取。账号刷新、消费保护、运营商关系和 MAC 解绑使用对应专用令牌；这些令牌与已确认的管理身份分别管理。

## zfw 查询

| CLI | 必要前提 | 业务请求 | 返回内容 |
|---|---|---|---|
| `zfw verify` | 管理鉴权完成 | 使用已确认的管理身份 | 登录方式、身份核对结果；密码登录另返回凭据有效 |
| `zfw account` | 管理身份 | GET `dashboard` | 账号状态、套餐、计费、余额、时长与消费保护 |
| `zfw profile` | 管理身份 | GET `setting/personList` | 当前资料标签、值与只读状态 |
| `zfw online` | 管理身份 | GET `dashboard/getOnlineList` | 当前账号的精确连接 ID、IP、MAC 与计费字段 |
| `zfw history` | 管理身份 | GET `dashboard/getLoginHistory` | 近期登录记录 |
| `zfw devices` | 管理身份、合法分页参数 | GET `service/getMacList` | MAC 绑定分页与总数 |
| `zfw consume` | 管理身份 | GET `service/consumeProtect` | 实际消费限额与不限状态 |
| `zfw operator` | 管理身份 | GET `service/operatorId` | 两类运营商账号与密码是否已设置 |
| `zfw mauth` | 管理身份 | GET `dashboard/refreshMauthType` | 当前策略及其动作链接表达的状态 |
| `zfw recharge` | 管理身份 | GET `service/userRecharge` | 充值页面实际可用状态 |

设备列表直接查询 `getMacList`。新管理会话中的直接查询与先打开 MAC 页面再查询已经得到相同列表；页面另为解绑提供令牌。

账号概览、资料、策略和绑定查询各自读取当前业务值。管理登录取得的账号身份在同一会话中复用，业务页面与列表保持各自的读取语义。

## zfw 修改与连接下线

| CLI | 必要前提 | 一次动作提交 | 明确接受后的确认 |
|---|---|---|---|
| `zfw refresh` | GET `dashboard` 取得专用令牌 | GET `dashboard/refreshaccount` | 核对 HTTP 200 空正文，返回 `data:null` |
| `zfw offline --session ID` | GET 当前账号在线列表，找到精确 ID | GET `dashboard/tooffline` | 只读观察该 ID 从列表消失 |
| `zfw unbind --mac MAC` | MAC 页面专用令牌；分页定位目标绑定 | GET `service/unbindmac` | 服务器明确接受后，分页确认目标 MAC 消失 |
| `zfw consume --limit AMOUNT` | GET 当前消费保护表单与令牌 | POST `service/changeConsumeProtect` | 通过独立 GET 的实际限额核对目标金额 |
| `zfw operator --bind` | GET 当前运营商表单、令牌与另一运营商字段 | POST `service/bind-operator` | 通过独立 GET 核对选定账号、完整密码及另一组原值 |
| `zfw operator --unbind OPERATOR` | 同一运营商表单与字段 | POST `service/bind-operator` | 通过独立 GET 确认选定账号、密码均为空，另一组保持原值 |
| `zfw mauth --change` | 读取当前无感知策略与动作 | GET `dashboard/oprateMauthAction` | 重新查询策略，核对显示状态已改变 |

运营商和消费保护 POST 跳转至已声明的业务页时，导航中的 GET 已经是独立读取。内核使用该响应完成确认。POST 直接返回 HTML 时，另行 GET 当前业务页，取得持久化后的值。

消费保护按十进制金额比较，`999999` 表示不限。运营商修改保持另一运营商的账号与密码原值；解绑目标两字段原本均为空时，返回 `not_submitted`、`verified:true`。

连接下线的精确 session ID 必须存在于当前账号列表。提交前查询失败或目标已不存在时，返回 `not_submitted`；接受后以只读查询确认该 ID 消失。门户离线状态由外层流程另行观察。

MAC 查询找到目标即可确定存在；证明不存在需要读完全部预期记录，并核对分页总数、行数及 MAC 唯一性。前置查询失败返回 `not_submitted`，目标原本不存在返回 `not_submitted`、`verified:true`。服务器明确拒绝或结果未知时直接返回；明确接受后才确认列表变化。

## 账单与导出

| CLI 选择 | 必要前提 | 业务请求 | 确认职责 |
|---|---|---|---|
| `zfw bills --kind online` | 管理身份、日期与分页筛选 | GET `bill/getUserOnlineLog` | 解析行、总数与汇总 |
| `zfw bills --kind monthly --year YEAR` | 管理身份、明确年份 | GET `bill/getMonthPay` | 解析行、总数与月汇总 |
| `zfw bills --kind monthly` | GET `bill/monthPay` 取得服务器默认年份 | GET `bill/getMonthPay` | 使用所取年份解析结果 |
| `zfw bills --kind operations` | 管理身份、日期与分页筛选 | GET `bill/getOperatorLog` | 解析业务记录与总数 |
| `zfw export` | 同一管理会话先查询目标页，建立当前页上下文 | GET 相应 `export*`，`type=1` | 核对附件头、XLS 文件结构，创建输出文件 |
| `zfw export --all` | 管理身份、明确日期或年份范围 | GET 相应 `export*`，`type=2` 与范围参数 | 核对附件并创建输出文件 |

三类导出路径分别为 `bill/exportUserOnlineLog`、`bill/exportMonthPay`、`bill/exportOperatorLog`。月账单省略年份时，查询与导出都先取得服务器默认年份。当前页导出依赖同一会话的表格查询状态；全部导出直接使用范围参数。

账单查询虽然使用 GET，也会建立服务器当前页导出上下文，调用按单次提交传输处理。页面导航只用于确实需要的默认年份，其余筛选直接提交查询接口。

## 公开页面

| CLI | 必要前提 | 请求顺序 | 内容确认 |
|---|---|---|---|
| `zfw notice` | 公开访问 | GET `unlogin/notice` | 解析实际公告正文 |
| `zfw agreement` | 公开访问 | GET `unlogin/agreement` | 解析实际协议正文 |
| `zfw help` | 站点 Cookie；首次按需 GET `login/` 建立 | GET `unlogin/help`；GET 页面中确认的同源 iframe | 返回实际帮助内容、地址、HTTP 状态与可用状态 |

帮助外框与 iframe 分别读取，实际内容地址由当前页面提供。公告与协议直接读取各自页面。

## 已有结果的复用与反事实检查

每次调整请求链，都同时检查参数来源、操作目标和结果所能证明的状态。下表记录当前各项精简成立的依据。

| 原有附加动作 | 当前处理 | 成立依据 |
|---|---|---|
| p 登录前自动查询状态 | 直接提交原生认证，外层按需观察 | 原生认证接收完整参数并返回已有会话等处理结果 |
| p 登录前读取配置、模板或脚本 | 登录直接使用原生八字段 | 认证接口已经独立验证，终端与运营商是参数差异 |
| 未知提交后自动查询状态 | 立即返回未知，查询由调用者发起 | 一次状态观察不能证明丢失响应的提交结果 |
| POST 已导航至独立 GET 结果页后再次 GET 同页 | 解析已取得的独立结果页 | 该 GET 已提供持久化业务字段 |
| 设备查询前打开 MAC 页面 | 直接请求列表 | 独立会话直接查询与页面初始化后的结果一致 |
| 明确年份账单查询前读取账单页 | 直接提交年份 | 查询参数已经完整，页面只提供默认年份选择 |
| 全部账单导出前查询当前页 | 直接提交导出范围 | `type=2` 使用日期或年份范围；`type=1` 使用当前页上下文 |
| 所有公开页面先初始化 Cookie | 只有帮助按需建立 | 帮助 iframe 的生成依赖站点 Cookie，公告与协议直接可读 |
| 每项业务启动独立 CLI 并重新鉴权 | 连续命令复用进程和各账号会话 | 同一会话内已经确认的管理身份可以用于后续私有请求 |
| 目标门户身份已符合要求，仅因宽带关系待迁移便下线重登 | 只迁移关系，读回门户确认身份与 MAC；实际离线时再认证 | 关系迁移实验和对应脚本分支均保持原会话及物理网卡外网访问 |
| 门户离线后再用 Self 列表判断能否认证 | 由门户终端状态决定后续步骤 | 门户与 Self 属于两个会话空间，实际在线身份由门户观察 |
| 登录成功后再查一次相同门户状态 | 使用登录返回的已核验状态 | 内核已确认源地址、目标账号与运营商 |
| 需要认证时，新绑定后固定等待再认证 | 立即认证，按完整明确拒绝串行决定下一次认证 | 管理端保存与认证侧生效分别观察，认证结果提供下一步依据 |

动态表单、专用令牌、精确 session ID、完整绑定字段和明确接受后的目标状态各自保留。它们分别确定提交内容、操作归属与实际变化，验证时应逐项观察相应职责。

外层 Windows 流程在下线前重新观察绑定与身份，改绑前重新读取相关账号的绑定。门户目标身份已经达标时，完成所需关系迁移后核对身份与 MAC，保持在线便继续使用当前连接；实际离线时再认证。其他身份在线时，通过精确 Self 连接清退后完成目标认证。

需要认证的新绑定流程中，只有完整的“未绑定运营商账号,请正确绑定运营商账号再试！”拒绝才在生效窗口内触发下一次串行认证；其余拒绝、未知结果及接受后核验失败保留本次结果并结束。相关部署观察见 [运营商绑定与认证生效](zfw/binding.md) 和 [门户会话与自助在线记录](p/sessions.md)。
