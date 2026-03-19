
# NJUPT-NET CLI 内核唯一真相源逆向规范（Final SSOT）

版本：2026-03-18  
定位：**作为 `njupt-net` Go CLI 内核实现的唯一真相源（Single Source of Truth, SSOT）**  
适用范围：本文件仅基于当前会话中已上传的全部逆向材料、MCP 报告、深爬结果、Python / Shell 原型、最终收口报告综合整理。  
实现原则：**AI/开发者必须优先服从本文件；若与早期报告冲突，以本文件的“最终裁决”与“证据优先级”节为准。**

---

## 0. 使用说明

### 0.1 目标
本文档目标不是记录“所有曾经出现过的猜测”，而是给出一份**可直接驱动 CLI 内核开发**的协议规范、状态机、数据模型和工程边界。

### 0.2 绝对原则
1. 不把 `HTTP 200` 当作业务成功。
2. 不把页面提示文案当作唯一真相。
3. Self 写接口统一遵循：  
   **PreState -> Submit -> ReadBack -> (可选)Restore**
4. Portal 以 **802 为主实现**，801 只作 **fallback / legacy compatibility**。
5. 本文档中 **Blocked / Guarded** 项不能被实现为“确定成功语义”；只能做保守处理。
6. 若实现与本文件冲突，以本文件为准；若运行环境与本文件冲突，应记录证据，而不是擅自改协议语义。

### 0.3 证据优先级（冲突裁决规则）
当不同报告互相冲突时，按以下优先级取舍：

1. **最终收口式深度逆向报告**  
2. **最终深度补全报告 / 最终 summary / endpoints JSON**
3. **mauth 专项深探**
4. **Chrome Dev MCP 全链路报告 / 深爬报告 / 不确定性报告**
5. **早期 api_reverse_engineering.md / Python / Shell 原型**

---

## 1. 系统总览

### 1.1 两个逻辑系统

#### A. ZFW / Self 自助服务系统
- Base URL（主观测）：`http://10.10.244.240:8080/Self`
- 同义主机：`zfw.njupt.edu.cn:8080`（历史材料中出现）
- 负责：
  - Self 登录 / 注销
  - 在线设备查询
  - 上网记录 / 账单
  - 运营商绑定
  - MAC 列表
  - 用户资料页
  - 无感知开关
  - 消费保护

#### B. Portal 上网认证网关
- 主实现路径（802）：`https://p.njupt.edu.cn:802/eportal/portal/*`
- 直连观测主机：`https://10.10.244.11:802/eportal/portal/*`
- 兼容 / 历史路径（801）：`http://p.njupt.edu.cn:801/eportal/?c=ACSetting&a=*`
- 历史 / 旧式入口：`192.168.168.168`（Shell 原型出现）

### 1.2 AI 实现时的系统分层
建议项目实现为四层：

1. `core/domain`
   - 领域接口、数据模型、错误模型
2. `infra/self`
   - Self HTTP client 与 parser
3. `infra/portal`
   - Portal802Client（主）
   - Portal801Client（fallback）
4. `app/workflow`
   - 组合动作：login, relogin, bind, consume, doctor, ensure-online 等

---

## 2. 最终成熟度裁决

### 2.1 Confirmed（已确认，可按确定语义实现）
1. Self 登录校验链：
   - `login page -> randomCode -> verify -> protected readback`
2. `bind-operator` 存在真实成功写路径，且可恢复
3. `changeConsumeProtect` 存在真实成功写路径，且可恢复
4. `oprateMauthAction` 为真 toggle
5. `oprateMauthAction` 会话失效边界明确：登出后跳登录页 / `refreshMauthType` 返回登录页 HTML
6. Portal 802 `ret_code=2 / AC999` 可稳定观察到“已在线 / 重复登录”语义，应作为 guarded success 而非未知错误
7. Portal 801 注销页若 body 含 `Logout succeed.`，可作为稳定成功标记
8. Portal 801 登录返回通用 EPortal 壳页面时，缺乏稳定机器可判定成功信号，只能作为 guarded fallback
9. `refreshaccount` 为正常空响应，不应依赖为结构化数据接口

### 2.2 Guarded（可实现，但必须保守处理）
1. Portal 802 当前稳定可复现 `ret_code=1` 类错误归并
2. `tooffline` 接口存在、参数明确，请求成功后必须依赖有界延迟 readback 判定；在线环境下仍可能真实失败
3. `updateUserSecurity` 已确认两类失败语义：
   - 前端校验拦截（`Not valid!`）
   - 后端拒绝（`403`）
4. `oprateMauthAction` 当前环境**未复现 403 / 明确 rate limit**

### 2.3 Blocked（接口存在，但成功语义 / 边界仍受环境阻塞）
1. Portal 802 `ret_code=3`：未复现
2. Portal 802 `ret_code=8`：未复现
3. `updateUserSecurity` 真实成功写路径：低风险可控正样本未拿到

### 2.4 实现时的硬性处理
- Confirmed：可作为内核确定能力
- Guarded：可做命令，但必须返回“guarded / tentative”语义或保守错误映射
- Blocked：可以暴露接口与 raw 探针，但不能承诺成功语义

---

## 3. Token / Cookie / 状态 / 解析规则

### 3.1 Token / Cookie
#### `checkcode`
- 来源：`GET /Self/login/?302=LI`
- 提取：`input[name='checkcode']`
- 长度：`4`
- 用途：登录预检 token
- 注意：必须使用 fresh page 获取，不可跨会话复用

#### `csrftoken`
- 来源：各写操作表单页（如 `operatorId`, `consumeProtect`, `personList`）
- 提取：`input[name='csrftoken']`
- 长度：`36`
- 用途：Self 写接口 CSRF token
- 注意：每次访问表单页刷新；**不可缓存复用**

#### `JSESSIONID`
- 来源：登录页 / Self 会话创建过程
- 用途：Self 会话 cookie

### 3.2 解析器总原则
Self 接口返回类型不统一，必须采用 **endpoint 级 parser registry**。

允许的 parser 类型：
1. `JSONParser`
2. `JSONArrayParser`
3. `TwoDimensionalArrayParser`
4. `HTMLPageParser`
5. `HTMLFragmentParser`
6. `EmptyBodyParser`
7. `RawFallbackParser`

### 3.3 登录成功判定规则
不能只看：
- `/Self/login/verify` 是否 `302`
- 页面提示文案是否含“成功/失败”

必须综合判断：
1. 请求链是否进入 `/Self/dashboard`
2. 受保护页面是否真实可访问
3. 会话 cookie 是否有效
4. 必要时是否能继续访问一个受保护数据接口

### 3.4 文案降权规则
页面中的 `swal` / 错误提示 / 通用 DOM 文本只作附加诊断信息，不作唯一真值。

### 3.5 写接口统一状态机
所有 Self 写接口必须走：

1. `PreState`: GET 表单页 / 读取业务字段
2. `Submit`: POST / GET 写接口
3. `ReadBack`: 再次读取业务页面或数据接口
4. `Compare`: 比较业务字段变化
5. `Restore`（若为受控实验或 workflow 要求）

---

## 4. 未登录公共页面族（Self）

以下页面已确认存在并可达：

- `GET /Self/`
- `GET /Self/login/?302=LI`
- `GET /Self/unlogin/help`
- `GET /Self/unlogin/agreement`
- `GET /Self/unlogin/helpinfo/0`

实现意义：
- 可用于 health / doctor / discover
- 不用于登录成功判定

---

## 5. Self 登录与注销

## 5.1 登录主链（Final）
### Step 1: 读取登录页
- Method: `GET`
- Path: `/Self/login/?302=LI`
- 作用：
  - 获取 `checkcode`
  - 获取 / 刷新 `JSESSIONID`

### Step 2: 请求 randomCode
- Method: `GET`
- Path: `/Self/login/randomCode`
- Params:
  - `t`: random
- 作用：
  - **必须调用**
- 说明：
  - 现在以最终深度补全结果为准，视其为登录链真实必要前置动作

### Step 3: 执行 verify
- Method: `POST`
- Path: `/Self/login/verify`
- Content-Type: `application/x-www-form-urlencoded`
- Params:
  - `account`: 学号
  - `password`: 密码
  - `checkcode`: 来自登录页
  - `code`: 验证码字段（当前实现中可留空，但字段保留）
  - `foo`: 蜜罐字段，填账号
  - `bar`: 蜜罐字段，填密码

### Step 4: 受保护页面回读
- 建议优先：
  - `GET /Self/dashboard`
  - 或一个受保护数据接口
- 只有回读成功，才算真正登录成功

## 5.2 登录错误语义（最终裁决）
### Confirmed
- 错误密码：应落回登录页
- `skip_randomCode`：应失败
- stale checkcode / stale session：应失败

### 历史冲突的处理
早期报告曾出现“错误验证码 / 空验证码仍进入 dashboard”的样本；后续 fresh session + logout + no-cache 的深度补全结果将其压回失败。  
**最终实现以最终深度补全结论为准：把 `randomCode`、`checkcode`、session freshness 视为真实必要条件。**

## 5.3 注销
- Method: `GET / POST`
- Path: `/Self/login/logout`
- 登录态调用可用
- 最终落地：登录页
- 无登录态调用也会回登录页

---

## 6. Self 页面空间与导航页面

### 6.1 顶层页面
- `GET /Self/dashboard`
- `GET /Self/service`
- `GET /Self/setting`
- `GET /Self/bill`

### 6.2 Dashboard 子页 / 数据
- `GET /Self/dashboard/getOnlineList`
- `GET /Self/dashboard/getLoginHistory`
- `GET /Self/dashboard/refreshaccount`
- `GET /Self/dashboard/refreshMauthType`
- `GET /Self/dashboard/tooffline`
- `GET /Self/dashboard/oprateMauthAction`

### 6.3 Service 子页 / 数据
- `GET /Self/service/operatorId`
- `GET /Self/service/consumeProtect`
- `GET /Self/service/myMac`
- `GET /Self/service/getMacList`
- `POST /Self/service/bind-operator`
- `POST /Self/service/changeConsumeProtect`

### 6.4 Setting 子页 / 数据
- `GET /Self/setting/userSecurity`
- `GET /Self/setting/personList`
- `POST /Self/setting/updateUserSecurity`

### 6.5 Bill 子页 / 数据
- `GET /Self/bill/userOnlineLog`
- `GET /Self/bill/monthPay`
- `GET /Self/bill/operatorLog`
- `GET /Self/bill/getUserOnlineLog`
- `GET /Self/bill/getMonthPay`
- `GET /Self/bill/getOperatorLog`

---

## 7. Self 接口契约总表

## 7.1 Auth

### `GET /Self/login/?302=LI`
- Response: `HTML + Set-Cookie`
- Extract:
  - `checkcode`
  - `JSESSIONID`
- 用途：登录预检页

### `GET /Self/login/randomCode`
- Params:
  - `t`: random
- Response: `image/placeholder`
- Required: `true`
- 用途：登录链必需前置动作

### `POST /Self/login/verify`
- Params:
  - `account`
  - `password`
  - `checkcode`
  - `code`
  - `foo`
  - `bar`
- Response（表征）：
  - 常见表现为 `302 -> /Self/dashboard`
- 真正成功判定：
  - 必须依靠受保护页面回读

### `GET|POST /Self/login/logout`
- Response（表征）：
  - `302 -> /Self/login`
- 语义：注销

---

## 7.2 Dashboard

### `GET /Self/dashboard/getOnlineList`
- Params:
  - `t`: random
  - `order`: `asc|desc`
  - `_`: timestamp
- Response Type: `JSON Array`
- Empty Value: `[]`
- 字段：
  - `brasid`
  - `ip`
  - `loginTime`
  - `mac`
  - `sessionId`
  - `terminalType`
  - `upFlow`
  - `downFlow`
  - `useTime`
  - `userId`

### `GET /Self/dashboard/getLoginHistory`
- Params:
  - `t`: random
  - `order`
  - `_`
- Response Type: `2D Array`
- Columns（按当前观测）：
  - `loginTime`
  - `logoutTime`
  - `ip`
  - `mac`
  - `upFlow?`
  - `downFlow?`
  - `billingType?`
  - `cost?`
  - `null`
  - `terminalFlag`
  - `terminalType`
  - `index`

### `GET /Self/dashboard/refreshaccount`
- Params:
  - `csrftoken`
  - `t`
- Response Type: `empty`
- 当前结论：
  - `200 + contentLength=0`
- 实现建议：
  - 仅保留 raw probe，不要依赖为结构化数据源

### `GET /Self/dashboard/refreshMauthType`
- Params:
  - `t`
- Response Type: `HTML Fragment`
- Example:
  - `<a href='dashboard/oprateMauthAction'>默认</a>`
  - 也可能体现“关闭”
- 用途：
  - 读取无感知状态
- 注意：
  - 未登录时可能返回登录页 HTML，而不是片段

### `GET /Self/dashboard/tooffline`
- Params:
  - `sessionid`
- Response Type:
  - `JSON`
- Example:
  - `{"success":true}`
  - 当前历史样本也见过 `{"success":false}`
- 状态：
  - **Guarded / Blocked**
- 当前结论：
  - 接口存在；请求体里的 `success=true` 不能直接当作业务成功
  - 必须结合后续有界延迟 `getOnlineList` 回读判定
  - 若目标 session 消失且出现新的 follow-up session，视为 guarded success（目标会话已被踢下线，后续已自动重连）
- 实现要求：
  - 只有在 getOnlineList 返回可踢 session 时才尝试
  - 结果需结合后续 getOnlineList 回读判定
  - 当前应暴露为 guarded capability

### `GET /Self/dashboard/oprateMauthAction`
- Response Type: `HTML / Redirected HTML`
- Side Effect: `true`
- 当前结论：
  - 真 toggle
  - 请求链通常表现为：`GET oprateMauthAction -> 302 -> dashboard 200 -> refreshMauthType 200`
  - 受保护态可成功切换
  - 登出后访问会跳登录页
- 实现要求：
  - 先读 `refreshMauthType`
  - 发起 toggle
  - 延迟约 1 秒再回读
  - 判断前后状态是否翻转

---

## 7.3 Service

### `GET /Self/service/operatorId`
- Response Type: `HTML`
- Extract:
  - `csrftoken`
  - `FLDEXTRA1`
  - `FLDEXTRA2`
  - `FLDEXTRA3`
  - `FLDEXTRA4`
- 用途：
  - 运营商绑定状态页
  - bind 写入前置页

### `POST /Self/service/bind-operator`
- Content-Type: `application/x-www-form-urlencoded`
- Params:
  - `csrftoken`
  - `FLDEXTRA1`: 电信账号
  - `FLDEXTRA2`: 电信密码
  - `FLDEXTRA3`: 移动账号
  - `FLDEXTRA4`: 移动密码
- Response Type:
  - `HTML`
- Side Effect: `true`

#### 最终业务判定规则
- 真值字段：`FLDEXTRA1~4`
- 成功判定：
  - 回读后这些业务字段发生目标变化
- 恢复判定：
  - 再次回读后恢复原值
- 禁止：
  - 只看 `HTTP 200`
  - 只看 `swal`
  - 受 `csrftoken` 轮换干扰

#### 当前已确认能力
- 真实成功写路径：**Confirmed**
- 可恢复：**Confirmed**

### `GET /Self/service/consumeProtect`
- Response Type: `HTML`
- Extract:
  - `csrftoken`
  - `currentLimit`
  - `currentUsage`
  - `balance`
  - 以及页面内嵌 user JSON
- 用途：
  - 消费保护写入前置页

### `POST /Self/service/changeConsumeProtect`
- Content-Type: `application/x-www-form-urlencoded`
- Params:
  - `csrftoken`
  - `consumeLimit`: 数值；`999999` 表示不限
- Response Type:
  - `HTML`
- Side Effect: `true`

#### 最终业务判定规则
- 不可只看 200
- 不可只看 swal “更改成功”
- 真值字段：
  - `installmentFlag`（来自页面内嵌 user JSON）
- 已确认样本：
  - `999999 -> 1`
  - `1 -> 0`
  - `0 -> 20`
  - `20 -> 999999`
  - `999999 -> 123`
  - restore：`123 -> 999999`

#### 当前状态
- **Confirmed**
- 可恢复：**Confirmed**

### `GET /Self/service/myMac`
- 页面页，驱动 `getMacList`

### `GET /Self/service/getMacList`
- Params:
  - `pageSize`
  - `pageNumber`
  - `sortName`
  - `sortOrder`
  - `_`
- Response Type:
  - `JSON`
- 用途：
  - MAC 列表查询

---

## 7.4 Setting

### `GET /Self/setting/userSecurity`
- 页面页

### `GET /Self/setting/personList`
- Response Type:
  - `HTML`
- Extract:
  - `csrftoken`
  - 页面基础资料字段
- 用途：
  - `updateUserSecurity` 的前置页

### `POST /Self/setting/updateUserSecurity`
- Content-Type: `application/x-www-form-urlencoded`
- Params（当前可观察）：
  - `csrftoken`
  - 可能的 phone/email/checkCode 等字段
- Response Type:
  - `HTML`
- Side Effect:
  - `true`

#### 当前已确认语义
1. UI 点击提交（不改字段）：
   - 可能出现前端弹窗：`失败 / Not valid!`
2. token-only POST：
   - 可能 `200`，回到 personList，swal `失败`
3. 带 phone/email/checkCode 的直接 POST：
   - 可能 `403`

#### 当前裁决
- **失败路径 Confirmed**
- **真实成功写路径 Blocked**
- 原因：
  - 页面无低风险可控可编辑字段
  - 前端校验与后端策略双重阻断

#### 实现要求
- 可以暴露 raw / guarded 命令
- 不应承诺“修改个人资料成功”
- 标准 JSON 结果不得暴露原始 personList HTML 或密码类字段
- 默认建议不作为 CLI 核心主路径能力

---

## 7.5 Bill

### `GET /Self/bill/getUserOnlineLog`
- Params:
  - `pageSize`
  - `pageNumber`
  - `sortName`
  - `sortOrder`
  - `startTime`
  - `endTime`
  - `_`
- Response Type:
  - `JSON Object`
- 结构（按当前观测）：
  - `summary`
  - `total`
  - `rows[]`
- `summary` 常见字段：
  - `CHINANETDOWNFLOW`
  - `INTERNETUPFLOW`
  - `CHINANETUPFLOW`
  - `COSTMONEY`
  - `COU`
  - `TIME`
  - `INTERNETDOWNFLOW`
  - `FLOW`
- `rows[]` 常见字段：
  - `area`
  - `chinanetDownFlow`
  - `chinanetUpFlow`
  - `costId`
  - `costMoney`
  - `costStyleId`
  - `extend`
  - `flow`
  - `internetDownFlow`
  - `internetUpFlow`
  - `loginTime`
  - `logoutTime`
  - `macAddress`
  - `nasIp`
  - 以及其他计费相关字段

### `GET /Self/bill/getMonthPay`
- Params:
  - `pageSize`
  - `pageNumber`
  - `sortName`
  - `sortOrder`
  - `startTime?`
  - `endTime?`
  - `_`
- Response Type:
  - `JSON`
- 用途：
  - 月消费记录查询

### `GET /Self/bill/getOperatorLog`
- Params:
  - `pageSize`
  - `pageNumber`
  - `sortName`
  - `sortOrder`
  - `startTime?`
  - `endTime?`
  - `_`
- Response Type:
  - `JSON`
- 用途：
  - 运营商日志查询

### 页面页
- `GET /Self/bill/userOnlineLog`
- `GET /Self/bill/monthPay`
- `GET /Self/bill/operatorLog`

---

## 8. Portal 接口与最终裁决

## 8.1 Portal 802（主实现）

### 登录
- Method: `GET`
- Path: `/eportal/portal/login`
- Host:
  - 优先：`p.njupt.edu.cn:802`
  - 观测直连：`10.10.244.11:802`
- 说明：
  - 直连浏览器会受 `ERR_CERT_COMMON_NAME_INVALID` 影响
  - 对 CLI / Go HTTP client，需允许或显式处理证书策略；浏览器类自动化推荐用 `p.njupt.edu.cn:802`

### 典型参数（当前主观察）
- `user_account`
- `user_password`
- `wlan_user_ip`
- `wlan_user_ipv6`（可选）
- `callback`
- `login_method`
- 以及可能的 Portal 固定参数

### 返回形式
- `HTTP 200`
- `application/javascript`
- body 常见为：
  - `dr1003({...})`
  - 或 callback 变体

### 当前稳定可复现的返回
- `ret_code=1`（含部分字符串型 `"1"`）
- 代表性 `msg`：
  - `未绑定运营商账号,请正确绑定运营商账号再试！`
  - `从Radius获取错误代码出现异常！`
  - `认证出现异常！`
  - `无法获取用户认证密码！`
  - `账号或密码错误(ldap校验)`
  - `本账号费用超支，禁止使用`
  - `Radius认证失败！`

### 当前未复现
- `ret_code=3`
- `ret_code=8`

### Portal 802 最终裁决
- **主实现：Confirmed**
- `ret_code=1` 类错误集合：**Guarded but usable**
- `ret_code=3 / 8`：**Blocked**

### 实现要求
1. 把 802 作为默认登录实现
2. 解析 callback / `dr1003(...)` 包裹的 JSON
3. 对所有未知 `ret_code`：
   - 分类为 `ErrPortalUnknownCode`
   - 保留 `ret_code` 与原始 `msg`
   - 允许“logout once + retry once”
4. 对 `ret_code=1`：
   - 不要硬编码成单一业务含义
   - 视为“多来源错误归并”
5. 当前不得把 `ret_code=3 / 8` 映射成确定语义

### 注销
- Method: `GET`
- Path: `/eportal/portal/logout`
- 说明：
  - 作为 802 对应的 logout 路径保留

## 8.2 Portal 801（fallback / legacy）

### 登录
- Method: `GET`
- Path: `/eportal/?c=ACSetting&a=Login`
- Host: `p.njupt.edu.cn:801`

### 注销
- Method: `GET`
- Path: `/eportal/?c=ACSetting&a=Logout`

### 历史参数面
- `DDDDD`
- `upass`
- `ss1`
- `ss5`
- `ss6`
- `timet`
- `mip`
- `v6ip`
- 以及历史 ACSetting 参数

### 最终裁决
多组 801 变体（full / minimal / no_ss / no_ipv6 / no_mip / wrong_ss1）：
- `HTTP 200`
- 标题一致
- `body_hash` 一致

因此：
- `Logout` 若 body 含 `Logout succeed.`，可视为 **Confirmed success**
- `Login` 返回通用 EPortal 壳页面时，**缺乏稳定机器可判定的成功/失败信号**
- 801 不适合作为主工作路径，仍应作为 fallback / legacy compatibility

### 实现要求
- 仅在 802 不可用且用户显式允许时尝试
- 801 Login 返回 raw / guarded 诊断信息，不依赖壳页面 body 判断成功
- 801 Logout 可在稳定成功标记出现时返回 confirmed success

---

## 9. 运行时环境与 Shell / Python 原型保留信息

## 9.1 Shell 原型中涉及的运行时能力
这些能力属于 `env adapter`，不属于协议内核，但与 CLI 工作流有关：

- 检测当前 SSID
- 连接目标 Wi-Fi
- 获取 IPv4
- 获取 IPv6
- 获取 MAC
- 检查连通性
- OS 分支（Darwin / Linux）
- 历史 Portal 入口 `192.168.168.168`

### 实现裁决
这些能力应实现为：
- `env/adapter`
- 而非 `core protocol`

## 9.2 Python 原型中保留的高价值结论
- `operatorId` 页是绑定读写的前置页
- 绑定 workflow 本质是组合动作，不是原子能力
- 应优先按业务字段回读判断 bind 结果

---

## 10. CLI 内核的最终能力边界

## 10.1 建议作为 CLI 核心原子能力（Confirmed/Guarded）
### Self Auth
1. `SelfPreflightLogin`
2. `SelfLogin`
3. `SelfLogout`

### Dashboard
4. `GetOnlineList`
5. `GetLoginHistory`
6. `RefreshAccountRaw`
7. `GetMauthState`
8. `ToggleMauth`
9. `ForceOffline`（Guarded）

### Service
10. `GetOperatorBinding`
11. `BindOperator`
12. `GetConsumeProtect`
13. `ChangeConsumeProtect`
14. `GetMacList`

### Setting
15. `GetPersonList`
16. `UpdateUserSecurity`（Guarded / mostly blocked for success path）

### Bill
17. `GetUserOnlineLog`
18. `GetMonthPay`
19. `GetOperatorLog`

### Portal
20. `Portal802Login`
21. `Portal802Logout`
22. `Portal801LoginFallback`
23. `Portal801LogoutFallback`

## 10.2 不应作为内核原子能力
以下属于 workflow 或 env adapter：
- 自动重连
- 自动迁移绑定
- 自动连接校园网 Wi-Fi
- 自动检测/修复网络
- 定时巡检
- SSID / IP / MAC 发现

---

## 11. 数据模型建议（Go）

## 11.1 基础结果模型
```go
type EvidenceLevel string

const (
    EvidenceConfirmed EvidenceLevel = "confirmed"
    EvidenceGuarded   EvidenceLevel = "guarded"
    EvidenceBlocked   EvidenceLevel = "blocked"
)
```

```go
type OperationResult[T any] struct {
    Level        EvidenceLevel
    Success      bool
    Data         *T
    RawStatus    int
    RawBody      string
    RawHeaders   map[string][]string
    Message      string
    Diagnostics  map[string]any
}
```

## 11.2 Self 登录结果
```go
type SelfLoginResult struct {
    CheckcodeFetched   bool
    RandomCodeCalled   bool
    VerifyStatus       int
    VerifyLocation     string
    DashboardReadable  bool
    SessionAlive       bool
}
```

## 11.3 Mauth 状态
```go
type MauthState string

const (
    MauthUnknown MauthState = "unknown"
    MauthOn      MauthState = "on"      // “默认”可映射为 on
    MauthOff     MauthState = "off"     // “关闭”
)
```

## 11.4 绑定状态
```go
type OperatorBinding struct {
    TelecomAccount string // FLDEXTRA1
    TelecomPass    string // FLDEXTRA2
    MobileAccount  string // FLDEXTRA3
    MobilePass     string // FLDEXTRA4
}
```

## 11.5 消费保护状态
```go
type ConsumeProtectState struct {
    CSRFTOKEN       string
    InstallmentFlag string // 真值字段
    CurrentLimit    string
    CurrentUsage    string
    Balance         string
}
```

## 11.6 Portal 802 返回
```go
type Portal802Response struct {
    RetCode    string
    Msg        string
    RawPayload string
}
```

---

## 12. 错误模型建议（Go）

### 12.1 Self
- `ErrAuth`
- `ErrNeedFreshLoginPage`
- `ErrNeedRandomCode`
- `ErrTokenExpired`
- `ErrGuardedCapability`
- `ErrBlockedCapability`
- `ErrUnexpectedLoginRedirect`

### 12.2 Portal
- `ErrPortalUnknownCode`
- `ErrPortalRetCode1`
- `ErrPortalRetCode3`（保留占位，不赋固定语义）
- `ErrPortalRetCode8`（保留占位，不赋固定语义）
- `ErrPortalTLS`
- `ErrPortalFallbackRequired`

### 12.3 Mauth
- `ErrTokenOrRateLimited`（当前仅作 guarded 建模建议）
- `ErrStateNotFlipped`

### 12.4 WriteBack
- `ErrWriteNotObserved`
- `ErrReadBackMismatch`
- `ErrRestoreFailed`

---

## 13. 实现约束（必须遵守）

### 13.1 Self
1. 登录必须先 GET loginPage，再 GET randomCode，再 POST verify
2. verify 成功必须回读受保护页面
3. 所有写接口必须现取 `csrftoken`
4. 所有写接口必须 readback 比较业务字段
5. `refreshaccount` 不得作为关键业务逻辑输入
6. `refreshMauthType` 解析必须支持：
   - 状态片段
   - 登录页 HTML（表示会话失效）

### 13.2 Portal
1. 默认走 802
2. 802 返回必须先去掉 callback / `dr1003(...)` 外壳再解析 JSON
3. 未知 `ret_code` 不得臆造含义
4. 801 只作 fallback，不作为主成功判定通道

### 13.3 CLI UX
1. `doctor` 命令必须输出诊断链
2. 对 Guarded / Blocked 能力必须明确展示级别
3. 默认危险写操作要求显式确认或 `--yes`
4. bind / consume / mauth 支持 `--readback`（默认开启）
5. 恢复型实验命令支持 `--restore`

---

## 14. 推荐命令树

```text
njupt-net
  self
    login
    logout
    status
    doctor
  dashboard
    online-list
    login-history
    refresh-account-raw
    mauth get
    mauth toggle
    offline <sessionid>         # guarded
  service
    binding get
    binding set
    consume get
    consume set
    mac list
  setting
    person get
    person update               # guarded / experimental
  bill
    online-log
    month-pay
    operator-log
  portal
    login                       # default 802
    logout
    login-801                   # fallback
  raw
    get <path>
    post <path>
```

---

## 15. 最终实现建议（给 AI 的硬约束）

1. 以本文档为唯一真相源，不再回溯旧冲突结论。
2. 先实现 Self，再实现 Portal。
3. 先实现 Confirmed 能力，再实现 Guarded。
4. Blocked 能力只实现接口壳与 raw 诊断，不伪造成功语义。
5. `bind-operator` 与 `changeConsumeProtect` 都必须支持：
   - `read current`
   - `submit desired`
   - `readback verify`
   - `restore original`（可选）
6. `oprateMauthAction` 必须实现为：
   - `get current state`
   - `toggle`
   - `wait ~1s`
   - `get state again`
7. `tooffline` 实现必须先检查 `getOnlineList` 是否有 session。
8. `updateUserSecurity` 默认标注 experimental / guarded。
9. Portal 802 解析器必须允许 `ret_code` 为数字或字符串。
10. 对 `ret_code=1` 只报告原始消息，不做单义解释。

---

## 16. 当前仍然不存在的“最终证明”

以下事项**尚未被彻底证明**，不得在代码或文档中写成“已完全确认”：

1. Portal 802 `ret_code=3` 的触发条件
2. Portal 802 `ret_code=8` 的触发条件
3. `updateUserSecurity` 的真实成功写路径

这些能力的处理策略：
- 可以暴露接口与 raw 诊断
- 可以在有合适环境时再继续补证
- 不得承诺固定成功语义

---

## 17. 一句话交付结论

基于当前会话中全部已上传逆向材料，**本文件已经足够支撑 AI 完成 `njupt-net` Go CLI 内核的全部主体开发**：  
- Self 核心协议与主写路径已足够确定  
- Portal 主实现（802）已可开发，未知码按 guarded / blocked 处理  
- 801 已被正确降级为 fallback  
- 剩余未证实项不会阻塞主体实现，只会影响少数边界能力的“完全证明程度”

**因此，开发应立即以本文件为唯一真相源推进。**
