# `njupt-net` 逆向覆盖差异报告（2026-03）

## 1. 结论先行

这份报告对比三套对象：

1. 新零基文档：[doc/REVERSE-ENGINEERING-2026-03.md](/D:/code/github/hicancan/njupt-net-cli/doc/REVERSE-ENGINEERING-2026-03.md)
2. 旧唯一真相源：[doc/FINAL-SSOT.md](/D:/code/github/hicancan/njupt-net-cli/doc/FINAL-SSOT.md)
3. 现有能力矩阵：[doc/CAPABILITY-MATRIX.md](/D:/code/github/hicancan/njupt-net-cli/doc/CAPABILITY-MATRIX.md)

要直接回答的两个问题是：

### 1.1 目前是不是“所有功能都已逆向”

**是，如果“已逆向”指当前命令树覆盖的每个业务页面/业务接口都已被映射到 `confirmed` / `guarded` / `blocked`。**

**否，如果“已逆向”要求所有能力都达到 `confirmed` 成功语义。**

**也不是，如果“所有功能”指这次发现式扫描扫出来的整站暴露面。**

新一轮发现式审计确认：Portal 801 / 802 对外还暴露了一整套管理后台接口族，不属于当前 32 个叶子命令，也没有完成逐条业务语义逆向。因此，“命令树范围内全部逆向”与“整站全部暴露面都逆向完”不是一回事。

当前仍必须保留非 `confirmed` 的项：

- `setting person update`：`blocked`
- `portal login-801`：`blocked`

### 1.2 目前是不是“所有逆向出来的功能都已实现”

**是，如果“已实现”指 32 个叶子命令都存在，且每个命令都映射到了已知协议面。**

**否，如果“已实现”要求所有命令都必须是 `confirmed` 正式能力。**

**也不是，如果“已实现”指发现式审计扫出来的所有站点能力都已经产品化成 CLI。**

当前不是“缺命令”或“漏实现”，而是部分命令根据现场证据应继续保持 `guarded` / `blocked`。

## 2. 新零基文档 vs 旧 `FINAL-SSOT`

## 2.1 已被新零基审计补强或再确认的点

| 条目 | 旧 SSOT 状态 | 新零基审计结果 | 结论标签 |
| --- | --- | --- | --- |
| `/Self/` 根路径 | 作为公共页列出 | 已现场确认未登录时会落到 `/Self/login/?302=LI` | `已逆向且已实现` |
| `/Self/unlogin/help` / `helpinfo/0` | 已列出 | 已现场确认当前只有“暂无使用帮助信息”占位内容 | `已逆向且已实现` |
| `userOnlineLog` 页面与 `getUserOnlineLog` | 旧 SSOT 已列接口 | 已现场确认页面结构、日期范围控件、汇总区和 XHR 参数面 | `已逆向且已实现` |
| `monthPay` 页面与 `getMonthPay` | 旧 SSOT 已列接口 | 已现场确认年份下拉、表格列和 XHR 参数面 | `已逆向且已实现` |
| `operatorLog` 页面与 `getOperatorLog` | 旧 SSOT 已列接口 | 已现场确认页面结构和 XHR 参数面 | `已逆向且已实现` |
| `operatorId` 表单字段 | 旧 SSOT 已列出 `FLDEXTRA*` | 已现场确认 `form action=/Self/service/bind-operator` 与 4 个字段映射 | `已逆向且已实现` |
| `consumeProtect` 表单字段 | 旧 SSOT 已描述 | 已现场确认 `form action=/Self/service/changeConsumeProtect` 与 `999999=不限制` 文案 | `已逆向且已实现` |
| `personList` 敏感内容 | 旧 SSOT 已强调安全边界 | 已现场再次确认页面脚本里存在密码型信息，标准 JSON 不能暴露原始 HTML | `已逆向且已实现` |
| Portal 802 `AC999` | 旧 SSOT 认为是已在线 / 重复登录 guarded success | 已通过“先 logout 再 login”的现场补证再次确认 | `已逆向且已实现` |
| Portal 801 logout | 旧 SSOT 认为 `Logout succeed.` 可作为 confirmed 标记 | 已现场再次确认 | `已逆向且已实现` |
| Portal 801 login | 旧 SSOT 认为返回通用壳页，仅能 guarded | 已现场补证真实 `/admin/login/login` JSON API；当前校园网用户凭据未返回 token，应提升为明确 blocked | `应保留 guarded/blocked` |
| `/Self/service/userRecharge` | 旧 SSOT 基本未强调 | 发现式审计确认入口存在，但页面退回服务壳页，未观察到独立充值能力 | `已逆向但未实现` |
| Portal 801/802 管理后台面 | 旧 SSOT 只覆盖登录/登出及少量入口 | 新审计在前端资源中发现 `admin/user/*`、`admin/role/*`、`portal/page/*`、`portal/program/*`、`portal/settings/*` 等完整后台面，但语义尚未逐条逆向 | `已实现但逆向证据不足` |

## 2.2 新零基审计没有推翻旧 SSOT 的点

总体上，旧 `FINAL-SSOT` 的结构与结论是可靠的，本轮没有发现“旧 SSOT 错了但实现一直照错做”的大类问题。真正发生变化的是：

- 页面层与网络层证据更完整了
- 旧文档里的若干 guarded/blocked 结论得到了新的现场补证
- 一些以前偏实现侧的判断，现在获得了浏览器侧的直接佐证
- 发现式方法额外暴露了**旧 SSOT 没有系统列出的站点后台面**

## 3. 新零基文档 vs `CAPABILITY-MATRIX`

能力矩阵当前与实现整体是对齐的。新零基审计对它的结论是：

| 能力矩阵条目 | 新零基审计结论 | 标签 |
| --- | --- | --- |
| `self login/logout/status/doctor` | 与现场一致 | `已逆向且已实现` |
| `dashboard online-list/login-history/refresh-account-raw/mauth get/mauth toggle` | 与现场一致 | `已逆向且已实现` |
| `dashboard offline` guarded | 新现场补证已把它提升为 bounded-readback confirmed 成功 | `已逆向且已实现` |
| `service binding/consume/mac/migrate` | 与现场一致 | `已逆向且已实现` |
| `setting person get` guarded | 新现场补证已把它提升为 confirmed 脱敏读取能力 | `已逆向且已实现` |
| `setting person update` blocked shell | 与现场一致 | `应保留 guarded/blocked` |
| `bill online-log/month-pay/operator-log` | 与现场一致 | `已逆向且已实现` |
| `portal login/logout` | 与现场一致；`AC999` 的 already-online 语义再次确认 | `已逆向且已实现` |
| `portal login-801` blocked | 与现场一致；真实管理端 JSON API 已补证 | `应保留 guarded/blocked` |
| `portal logout-801` confirmed | 与现场一致 | `已逆向且已实现` |
| `guard` runtime commands | 与现场和路由器运行状态一致 | `已逆向且已实现` |

没有发现“能力矩阵宣称已实现，但现场证据不足以支撑”的新增问题。

但有一个重要边界需要写清楚：

- 能力矩阵描述的是 **`njupt-net` 当前产品面**
- 不是整站所有被发现出来的后台能力面

## 4. 新零基文档 vs 当前 32 个 CLI 命令实现

下面这张表按命令树逐项给出最终标签。

| 命令 | 协议面 | 最终标签 | 说明 |
| --- | --- | --- | --- |
| `self login` | `/Self/login/*` | `已逆向且已实现` | 登录链已现场确认 |
| `self logout` | `/Self/login/logout` | `已逆向且已实现` | 注销链已验证 |
| `self status` | `/Self/dashboard` + `/Self/service` | `已逆向且已实现` | 受保护页可读性检查 |
| `self doctor` | 组合 workflow | `已逆向且已实现` | 组合能力，无新协议面 |
| `dashboard online-list` | `getOnlineList` | `已逆向且已实现` | |
| `dashboard login-history` | `getLoginHistory` | `已逆向且已实现` | |
| `dashboard refresh-account-raw` | `refreshaccount` | `已逆向且已实现` | 原始探针 |
| `dashboard mauth get` | `refreshMauthType` | `已逆向且已实现` | |
| `dashboard mauth toggle` | `oprateMauthAction` | `已逆向且已实现` | 通过前后状态翻转确认 |
| `dashboard offline` | `tooffline` | `已逆向且已实现` | bounded readback 证明目标会话移除即可 confirmed；后续自动重连不否定成功 |
| `service binding get` | `operatorId` | `已逆向且已实现` | |
| `service binding set` | `bind-operator` | `已逆向且已实现` | 业务失败优先于通用读回错误 |
| `service consume get` | `consumeProtect` | `已逆向且已实现` | |
| `service consume set` | `changeConsumeProtect` | `已逆向且已实现` | |
| `service mac list` | `myMac` + `getMacList` | `已逆向且已实现` | |
| `service migrate` | 组合 `bind-operator` 流程 | `已逆向且已实现` | 组合能力 |
| `setting person get` | `personList` | `已逆向且已实现` | 页面存在敏感原始内容，但标准 JSON 已稳定输出脱敏字段 |
| `setting person update` | `updateUserSecurity` | `应保留 guarded/blocked` | 成功语义仍不足以 confirmed |
| `bill online-log` | `getUserOnlineLog` | `已逆向且已实现` | |
| `bill month-pay` | `getMonthPay` | `已逆向且已实现` | |
| `bill operator-log` | `getOperatorLog` | `已逆向且已实现` | |
| `portal login` | 802 login | `已逆向且已实现` | `AC999` 已按 guarded success 归类 |
| `portal logout` | 802 logout | `已逆向且已实现` | |
| `portal login-801` | 801 login | `应保留 guarded/blocked` | 已确认是 `/admin/login/login` 管理端 JSON API；当前校园网用户凭据未返回 token |
| `portal logout-801` | 801 logout | `已逆向且已实现` | `Logout succeed.` 已确认 |
| `raw get` | 任意 Self GET | `已逆向且已实现` | 观察入口，无新增协议面 |
| `raw post` | 任意 Self POST | `已逆向且已实现` | 观察入口，无新增协议面 |
| `guard run` | 运行时组合链 | `已逆向且已实现` | |
| `guard start` | 运行时组合链 | `已逆向且已实现` | |
| `guard stop` | 运行时组合链 | `已逆向且已实现` | |
| `guard status` | 状态文件读取 | `已逆向且已实现` | |
| `guard once` | 单轮组合链 | `已逆向且已实现` | |

## 4.1 发现式审计新增、但当前 CLI 不覆盖的站点暴露面

这轮真正的新差异，不是某个旧命令写错了，而是发现了**更大的站点能力图谱**。至少包括：

- 旧式门户根页：
  - `/eportal/?c=ACSetting&a=Login`
  - `/eportal/?c=ACSetting&a=Logout&ver=1.0`
  - `/eportal/portal/page/loadConfig`
  - `/eportal/portal/online_list`
- 801 / 802 管理 SPA：
  - `admin/login/login`
  - `admin/login/logout`
  - `admin/user/info`
  - `admin/user/getList`
  - `admin/user/saveUser`
  - `admin/user/deleteUser`
  - `admin/user/changePassword`
  - `admin/role/getList`
  - `admin/module_auth/getInfo`
  - `admin/module_auth/saveInfo`
  - `admin/dashboard/getList`
  - `admin/dashboard/getStoreInfo`
  - `admin/dashboard/getSiteInfo`
  - `portal/page/*`
  - `portal/program/*`
  - `portal/settings/*`
  - `portal/visit_blacklist/*`
  - `portal/custom_error/*`

这些路径的**存在性**已被浏览器资源和网络请求证实，但它们不等于：

1. 已经全部完成业务语义逆向
2. 已经全部纳入 CLI 设计范围

因此它们不能被算成“当前所有功能都已实现”。

## 5. 仍然不是 “全部 confirmed” 的空白点

本轮没有留下“未逆向”的命令面空白，但仍有 2 个命令能力不应被说成 fully confirmed：

1. `setting person update`
2. `portal login-801`

它们不是没实现，而是**根据现场证据必须保守表达**。

与此同时，发现式审计还留下了一组**站点面空白**：

- Portal 801 / 802 管理后台接口族已被发现，但尚未逐项语义逆向
- `/Self/service/userRecharge` 入口存在，但尚未观察到独立充值功能

所以“命令面空白”与“站点面空白”要分开看。

## 6. 最终结论

### 6.1 关于“所有功能都逆向了”

- **按命令树覆盖的业务面来讲：是。**
- **按“全部都已经 confirmed”来讲：不是。**
- **按这次发现式扫描出来的整站暴露面来讲：也不是。**

### 6.2 关于“所有功能都实现了”

- **按 CLI 命令树来讲：是，32 个叶子命令全部已实现。**
- **按“全部都已成为 confirmed 成熟能力”来讲：不是。**
- **按“所有被发现到的站点功能都已经产品化成 CLI”来讲：不是。**

### 6.3 最终一句话

`njupt-net` 当前已经完成了**命令树覆盖范围内**的业务逆向闭环和实现闭环；但发现式审计进一步证明，整站对外暴露的能力面比当前 CLI 更大，尤其是 Portal 801/802 的管理后台面仍有大量“已发现、未逐条逆向、未产品化”的接口。因此，命令树范围内可以说“已逆向、已实现”，整站范围内则还不能这么说。
