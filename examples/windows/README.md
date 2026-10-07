# Windows 流程示例

这三个可选脚本使用 PowerShell 7，将连接、下线和宽带迁移组合为一次命令。`njupt-net.exe` 可独立运行，使用这些脚本时才需要 PowerShell 7。

| 脚本 | 用途 |
|---|---|
| `connect.ps1` | 使用指定校园账号连接，并检查所选链路的公网通达性 |
| `disconnect.ps1` | 下线当前终端，保留校园账号的宽带绑定 |
| `switch.ps1` | 将配置中的电信或移动宽带从旧校园账号迁移到新校园账号，并连接上网 |

先将 `njupt-net` 放入 PATH，按照 [配置示例](../../config.example.json) 准备当前目录的 `config.json`。账号参数使用配置 `accounts` 中的键。通过 `njupt-net interfaces` 查看网卡名与 IPv4；以下命令中的 `Ethernet` 替换为校园网网卡名。

以下命令从仓库根目录运行；使用 Windows ZIP 时，从解压根目录运行。`-Config` 的相对路径按当前工作目录解析，`-Executable` 可指定解压得到的可执行文件。

## 公共参数

| 参数 | 含义 | 默认值 |
|---|---|---|
| `-Interface NAME` | 选择校园网网卡，与 `-Source` 二选一 | 必须选择一种 |
| `-Source IPv4` | 选择校园网网卡上的源 IPv4，与 `-Interface` 二选一 | 必须选择一种 |
| `-Config FILE` | 校园账号与宽带配置文件 | 当前目录的 `config.json` |
| `-Executable COMMAND` | `njupt-net` 可执行文件路径或命令名 | PATH 中的 `njupt-net` |
| `-TimeoutSeconds SECONDS` | 传给 CLI 的单次网络请求超时 | `15` |

一次执行固定使用最初选定的源 IPv4。同一源 IPv4 已有流程运行时，新流程报告正在使用并结束。执行期间配置文件保持只读，每次调用 CLI 前再次核对内容；配置发生变化时结束当前流程。

## 连接

`-Account` 选择校园账号，`-Operator` 选择 `campus`、`njxy` 或 `cmcc`，分别对应校园网、电信和移动。

```powershell
pwsh -NoProfile -File examples/windows/connect.ps1 -Interface Ethernet -Account ACCOUNT_ALIAS -Operator cmcc
```

当前终端已用所选账号和运营商在线时，脚本检查公网通达性；离线时提交一次登录，核对账号和运营商，再检查公网通达性。当前在线账号或运营商不一致时，脚本返回对应错误并结束。

网卡有多个 IPv4 时，通过 `-Source` 选择具体地址：

```powershell
pwsh -NoProfile -File examples/windows/connect.ps1 -Source SOURCE_IPV4 -Account ACCOUNT_ALIAS -Operator cmcc
```

## 下线

```powershell
pwsh -NoProfile -File examples/windows/disconnect.ps1 -Interface Ethernet -Account ACCOUNT_ALIAS
```

脚本读取所选账号的在线连接，按本机校园网 IPv4 与 MAC 选定唯一对应会话，使用 `zfw offline` 下线，并确认门户离线。门户已显示离线时，脚本继续查询该账号的 Self 在线连接，确认所选源 IPv4 没有会话后返回成功。宽带绑定保留在该校园账号下，下次可以再次连接。

## 迁移宽带并连接

在 `config.json` 的 `broadband_account` 中填写要迁移的宽带账号、密码，以及 `njxy` 或 `cmcc` 运营商。`-From` 和 `-To` 分别选择配置中的旧、新校园账号：

```powershell
pwsh -NoProfile -File examples/windows/switch.ps1 -Interface Ethernet -From OLD_ACCOUNT -To NEW_ACCOUNT
```

旧、新账号需为不同校园账号。当前终端须使用 `-From` 账号及配置中的运营商在线；旧账号需绑定配置中的宽带账号且已设置密码，新账号对应运营商的账号与密码均为空。脚本核对这些状态和两账号凭据，再依次执行：

1. 下线本机在旧校园账号下的会话。
2. 再次核对两账号的绑定，确认旧绑定符合配置、新绑定仍为空，解除旧账号的指定运营商绑定。
3. 再次读取新账号绑定，确认仍为空，将配置中的宽带绑定到新校园账号。
4. 使用新校园账号和该运营商登录。
5. 核对门户身份与同一终端 MAC、公网通达性，以及旧、新校园账号的绑定状态。

该流程使用 `broadband_account` 指定的运营商和宽带账号。每次修改只提交一次；任一步失败时停止，并保留已经完成的步骤。

下线操作选定本机会话；宽带绑定属于校园账号级关系，迁移后由新校园账号持有。当前终端已经切到 `-To` 时，再次使用相同的 `-From`、`-To` 会在旧身份核对阶段结束。

## 结果

成功时，脚本向标准输出写入一份 JSON，退出码为 `0`；失败时，向标准错误写入一份 JSON，退出码为 `1`。`data.stage` 标明执行到的阶段，`data.steps` 记录已执行步骤及其结果；失败结果同样保留这些信息，便于查看当前终端与绑定状态。

校园账号和宽带密码由 CLI 从配置文件读取。脚本通过 CLI 的 JSON 结果核对状态，密码保留在配置文件中。
