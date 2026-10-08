# 校园网协议研究

研究程序直接访问当前部署，与 Go 内核独立。浏览器观察用户操作和真实请求；Python 固定变量、重现请求、检查响应及操作后的状态。研究从页面、配置和脚本发现候选地址，再通过真实请求确认部署入口、参数和响应。

[原子动作与请求边界](requests.md) 汇总全部 CLI 命令与 Go 管理操作的请求前提、提交和确认职责；[门户会话](p/sessions.md)、[运营商关系](zfw/binding.md) 与 [MAC 列表](zfw/mac.md) 记录各自的部署观察。

使用 Python 3.12、uv 和标准库，无第三方依赖。在本目录创建正式环境并运行离线测试：

```text
uv venv --python 3.12
uv sync --locked
uv run --locked python -m unittest discover -p "test_*.py" -v
```

离线测试使用人工样本和回环 HTTP 服务器，验证解析、Cookie 隔离、源地址、压缩、重定向和查询约束。真实业务通过下面的在线实验验证，需要校园网源地址及相应账号。

## 文件边界

| 文件 | 研究对象 |
|---|---|
| `network.py` | 显式 IPv4 源地址、HTTP/HTTPS、独立 Cookie、响应观察 |
| `pages.py` | HTML 控件、导航、脚本引用和静态路由候选的解析 |
| `credentials.py` | 从指定配置读取一个账号 |
| `deployment.py` | 域名解析、已知 HTTP 入口、单次定向 UDP 探测、独立 Nmap XML 扫描结果 |
| `p/portal.py` | 启动页、运行配置、启动脚本引用的模块、四类终端模板 |
| `p/native.py` | 801–804 原生状态与配置接口，仅携带 JSONP 回调名 |
| `p/access.py` | 状态、登录、退出、错误信息、操作后观察 |
| `p/password.py` | 条件改密图片资源与提交 |
| `p/selfservice.py` | 三种管理跳转请求及授权地址文件 |
| `zfw/session.py` | 登录前提、Cookie、身份、退出、语言、菜单和脚本清单 |
| `zfw/account.py` | 账号模型、资料、刷新和资料提交能力 |
| `zfw/connections.py` | 在线连接、历史记录、选定会话下线 |
| `zfw/mac.py` | MAC 列表原始响应、页面列含义、解绑检查 |
| `zfw/mauth.py` | 无感知状态与单次切换 |
| `zfw/operator.py` | 运营商表单、绑定、清空绑定及独立读回检查 |
| `zfw/consume.py` | 消费限额、表单和修改后检查 |
| `zfw/bills.py` | 三类账单、筛选、分页、排序、两类 XLS 导出 |
| `zfw/recharge.py` | 实际充值入口与付款表单 |
| `zfw/public.py` | 公告、帮助、协议及页面引用的 iframe |
| `access_cycle.py` | 显式的下线、登录及同源外网连通性实验 |

各业务目录的 `samples/` 保存解析测试所用的人工样本，并说明来源与用途。研究程序从当前部署获取页面与脚本，原始观察通过 `--output` 保存。

## 在线实验

以下命令在本目录运行。将 `SOURCE_IPV4` 替换为校园网网卡 IPv4，`ACCOUNT_ALIAS` 替换为配置中的账号键。配置采用产品的 `accounts` 结构。

```text
uv run --locked python -m deployment --source SOURCE_IPV4
uv run --locked python -m deployment --source SOURCE_IPV4 --udp ntp
uv run --locked python -m deployment --source SOURCE_IPV4 --udp drcom-challenge
uv run --locked python -m p.portal --source SOURCE_IPV4
uv run --locked python -m p.native --source SOURCE_IPV4 --port 804 --route online_list
uv run --locked python -m p.native --source SOURCE_IPV4 --port 804 --route page/loadConfig
uv run --locked python -m p.access --source SOURCE_IPV4
uv run --locked python -m zfw.session --source SOURCE_IPV4 --config ../config.json --account ACCOUNT_ALIAS
uv run --locked python -m zfw.mac --source SOURCE_IPV4 --config ../config.json --account ACCOUNT_ALIAS
uv run --locked python -m zfw.mac --source SOURCE_IPV4 --config ../config.json --account ACCOUNT_ALIAS --compare-initialization
uv run --locked python -m zfw.operator --source SOURCE_IPV4 --config ../config.json --account ACCOUNT_ALIAS
uv run --locked python -m zfw.operator --source SOURCE_IPV4 --config ../config.json --account ACCOUNT_ALIAS --bind cmcc --operator-account BROADBAND_ACCOUNT
uv run --locked python -m zfw.operator --source SOURCE_IPV4 --config ../config.json --account ACCOUNT_ALIAS --unbind cmcc
uv run --locked python -m zfw.bills --source SOURCE_IPV4 --config ../config.json --account ACCOUNT_ALIAS --kind monthly --export page
uv run --locked python -m zfw.public --source SOURCE_IPV4
```

其他业务模块用相同方式运行；`python -m 模块名 --help` 说明其参数。HTTP 连接直接绑定选定源地址。每个管理实验持有独立 Cookie 会话，结束时退出该管理会话；终端上网状态由接入接口单独管理。

桥接登录实验先通过 `p.selfservice --type 1 --url-file URL_FILE` 独占创建授权地址文件，再以 `zfw.session --self-url-file URL_FILE` 验证实际入口及配置账号身份。授权地址文件包含短期授权信息，保存在受版本控制忽略的本地实验目录。桥接入口按当前已发现的管理主机、端口及路径校验。

默认输出响应状态、类型、长度、哈希和 JSON 结构。`--output OUTPUT_DIRECTORY` 将解压后的响应正文保存到本次新建的目录，文件采用独占创建。正文包含实际账号、用户模型、令牌或账单，作为本地实验数据保存；公开样本使用下述人工构造格式。

状态改变通过显式参数选择：`p.access --action login|logout`、`p.password --action change`、`zfw.connections --offline-session`、`zfw.mac --unbind`、`zfw.mauth --toggle`、`zfw.operator --bind|--unbind`、`zfw.consume --set-limit`、`zfw.account --refresh|--submit-profile`。新密码通过交互输入。每次动作对应一次提交，认证端口和协议由指定入口确定。

`zfw.operator --bind njxy|cmcc` 和 `--unbind njxy|cmcc` 互斥。绑定通过 `--operator-account` 指定宽带账号，并交互输入密码；解绑只选择运营商。程序 GET `service/operatorId` 获取当前表单和 `csrftoken`，修改电信 `FLDEXTRA1/2` 或移动 `FLDEXTRA3/4`，保留另一组字段，向 `service/bind-operator` POST 一次。解绑提交的选定账号与密码均为空。提交后独立 GET 读取表单，输出响应摘要，以及账号、密码是否符合目标和另一运营商是否保持原值的布尔结果；解绑结果分别使用 `account_cleared`、`password_cleared`、`other_operator_unchanged`。

Python 实验分别保留提交响应和后续独立读取，用于核对服务器字段的变化。Go 操作可复用 POST 跳转中已经取得的独立 GET 结果页，其边界见 [原子动作与请求边界](requests.md)。绑定读回与门户认证的状态边界见 [运营商绑定与认证生效](zfw/binding.md)。

`access_cycle` 必须指定 `--execute` 和 `--operator`，会改变选定终端的上网状态。实验前核对当前账号、源地址和待用配置；如选用 `--restore-on-failure`，恢复属于显式实验清理，并单独报告结果。账单当前页导出先在同一管理会话查询表格；全部导出携带页面提供的筛选条件。

## 研究流程与结果解释

先从当前页面、配置、模板、菜单、表单和浏览器请求发现操作，再用独立会话复现前提、字段和状态，最后把已确认事实写入协议文档并落实内核。待确认项记录具体响应和所缺证据；人工样本用于解析测试，真实观察保留请求前提与结果。

- Portal 的原生状态、配置查询只需 `callback=dr序号`，服务器从连接识别接入地址。普通认证使用账号、密码、终端类型、源 IPv4、空 IPv6 与 `enable_r3=0`；四种终端及三种运营商是同一接口的参数差异。网页研究单独保留前端脚本的编码与初始化顺序。
- `p.portal` 递归获取启动脚本引用的业务脚本及二十份当前终端模板；静态目录同时标记已启用、条件启用和未启用模块。all.js、hls.js、layer 等通用资源库列为外部依赖，目录来源随调查结果记录。
- Self 登录页的图片初始化请求影响会话，默认流程保留该请求；`--skip-image` 用于比较初始化前提。每个操作分别取得自己的令牌。
- 语言实验已验证服务器端 Cookie 会话偏好。公开帮助页的语言按钮存在未定义 `ctx` 的脚本错误，该按钮点击后请求未发出。
- 匿名帮助实验先访问登录页建立 `/Self` Cookie，再从帮助外框读取实际 iframe。这一顺序使服务器输出可用的 `/Self/unlogin/helpinfo/0` 地址，整个过程使用公开页面。
- MAC 列表已取得有效非空 JSON，五列值为字符串，在线状态严格使用 `"0"`、`"1"`。新管理会话直接查询与先打开页面后查询的结果一致；解绑另从页面取得操作令牌。
- 运营商绑定通过专用表单提交并独立读回。旧绑定仍存在时，新校园账号直接绑定同一移动宽带会收到“已存在该运营商账号”；迁移先解除旧关系，再绑定目标账号。管理端绑定与门户认证分别确认，命令见 [Self 协议](../docs/zfw.md)。
- 资料表单仅包含 `csrftoken`，当前提交返回 `Not valid!`。充值入口返回服务菜单，付款表单及请求需要可用页面继续确认。
- `deployment --nmap-xml SCAN_XML` 读取独立扫描的范围和状态。TCP 服务枚举说明监听端口，页面与脚本调查说明 HTTP 路径；UDP 无响应按扫描器给出的状态记录。
- `deployment --udp ntp|drcom-challenge` 向 `--host` 指定目标发送一个报文，默认目标为 p。NTP 请求为首字节 `1b` 的 48 字节报文；Drcom 请求为 `07 01 08 00 01 00 00 00`。结果记录应答对端、长度、协议头及结构字段；`--output` 保存同一份汇总。Drcom 请求依据参考客户端的 [`_make_challenge`](https://github.com/drcoms/drcom-generic/blob/master/latest-pppoe.py)，应答中的挑战种子留在进程内存。

2026-10-07 的单报文复核收到 p 的两类 UDP 应答：123 返回 48 字节的 NTPv3 服务端报文，stratum 为 3；61440 返回 32 字节的 Drcom 挑战应答，前八字节为 `07 01 10 00 02 00 00 00`。挑战头中的长度字段为 16，实际 UDP 正文为 32 字节，两者分别记录。这个实验确认服务端的挑战交换能力，账号认证与会话保活属于后续独立协议研究。

后续研究从新增账号权限、接入区域或页面前提出发，复用这些实验记录请求、响应和状态变化，再扩展协议目录。

完整路径目录与部署结论分别维护于 [Portal 协议](../docs/p.md)、[Self 协议](../docs/zfw.md) 和 [网络部署](../docs/deployment.md)。这些文档统一维护接口表，研究程序输出用于重新核对目录和部署结果。
