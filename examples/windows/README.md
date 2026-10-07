# Windows 登录示例

`login.ps1` 使用 PowerShell 7。指定目标校园账号后，脚本根据当前宽带绑定决定是否迁移，再登录并核对门户身份与公网通达性。所需调用函数均包含在该脚本内。`njupt-net.exe` 可独立运行，使用这个可选脚本时才需要 PowerShell 7。

先将 `njupt-net` 放入 PATH，按照 [配置示例](../../config.example.json) 准备当前目录的 `config.json`。账号参数使用配置 `accounts` 中的键。通过 `njupt-net interfaces` 查看网卡名与 IPv4；以下命令中的 `Ethernet` 替换为校园网网卡名。

以下命令从仓库根目录运行；使用 Windows ZIP 时，从解压根目录运行。`-Config` 的相对路径按当前工作目录解析，`-Executable` 可指定解压得到的可执行文件。

## 使用

在 `config.json` 的 `accounts` 中填写校园账号与密码，在 `broadband_account` 中填写宽带账号、密码，以及 `njxy` 或 `cmcc` 运营商。`-Account` 选择要登录的校园账号键；运营商直接取自宽带配置。

```powershell
pwsh -NoProfile -File examples/windows/login.ps1 -Interface Ethernet -Account ACCOUNT_ALIAS
```

网卡有多个 IPv4 时，通过 `-Source` 选择具体地址：

```powershell
pwsh -NoProfile -File examples/windows/login.ps1 -Source SOURCE_IPV4 -Account ACCOUNT_ALIAS
```

## 参数

| 参数 | 含义 | 默认值 |
|---|---|---|
| `-Account ALIAS` | 目标校园账号在配置中的键 | 必填 |
| `-Interface NAME` | 选择校园网网卡，与 `-Source` 二选一 | 必须选择一种 |
| `-Source IPv4` | 选择校园网网卡上的源 IPv4，与 `-Interface` 二选一 | 必须选择一种 |
| `-Config FILE` | 校园账号与宽带配置文件 | 当前目录的 `config.json` |
| `-Executable COMMAND` | `njupt-net` 可执行文件路径或命令名 | PATH 中的 `njupt-net` |
| `-TimeoutSeconds SECONDS` | 传给 CLI 的单次网络请求超时 | `15` |

一次执行固定使用最初选定的源 IPv4。同一源 IPv4 已有流程运行时，新流程报告正在使用并结束。执行期间配置文件保持只读，每次调用 CLI 前再次核对内容；配置发生变化时结束当前流程。

## 目标状态

脚本以目标校园账号持有配置中的宽带、当前终端使用该账号和运营商在线且公网通达为完成状态。先读取目标账号在所选运营商下的绑定，再决定处理方式：

| 目标账号当前绑定 | 处理方式 |
|---|---|
| 账号等于配置中的宽带账号，且密码已设置 | 保留现有绑定 |
| 账号、密码均为空 | 查询其他配置账号，寻找这条宽带的当前持有人 |
| 已绑定另一条宽带，或账号、密码只有一项为空 | 报告当前绑定不符合要求并结束 |

目标绑定为空时，其他配置账号中有唯一持有人便迁移到目标；没有持有人便直接绑定目标；发现多个持有人时结束。宽带持有账号与终端当前登录账号分别确定，两者可以不同。

当前终端已经使用目标账号、正确运营商在线，且目标绑定符合配置时，只检查公网通达性。当前终端在线但尚未达到这一状态时，脚本通过 Self 按本机校园网 IPv4 与 MAC 选定唯一会话，下线前再次核对目标与持有人的绑定、原身份与 MAC，再完成必要的宽带迁移或绑定，使用目标账号认证。当前在线身份须能唯一对应配置中的校园账号；未知或歧义身份会返回错误。

门户显示离线时，脚本读取目标账号及已找到的宽带持有人在线连接，确认所选源 IPv4 没有会话后，继续所需的迁移、绑定和登录。修改前重新读取目标及已找到持有人的绑定，完成后核对门户目标身份、同一源地址的公网通达性及绑定结果。

下线操作选定本机会话；宽带绑定属于校园账号级关系，迁移后由目标校园账号持有。

每次修改只提交一次；任一步失败时停止，并保留已经完成的步骤。再次手动运行时，脚本重新读取当前状态，决定还需要哪些操作。

## 桌面入口

本机桌面入口调用同一个 `login.ps1`，通过 `-Account` 选择目标账号，并使用绝对路径指定脚本、`-Config` 和 `-Executable`。这样从桌面启动时仍读取指定配置。不同账号的入口只需改变目标账号参数。

## 结果

成功时，脚本向标准输出写入一份 JSON，退出码为 `0`；失败时，向标准错误写入一份 JSON，退出码为 `1`。`data.stage` 标明执行到的阶段，`data.steps` 记录已执行步骤及其结果；失败结果同样保留这些信息，便于查看当前终端与绑定状态。

结果的 `command` 为 `login`。`data.account_alias` 是目标账号，`data.previous_account_alias` 记录原在线账号，`data.binding_from` 记录迁移前的宽带持有人，`data.binding_moved` 表示本次宽带迁移是否已完成。

校园账号和宽带密码由 CLI 从配置文件读取。脚本通过 CLI 的 JSON 结果核对状态，密码保留在配置文件中。
