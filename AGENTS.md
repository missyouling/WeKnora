# AGENTS.md

## 研发工作流与 CI/CD 规范（强制执行优先级）

作为 WeKnora 项目的研发 Agent，你必须严格遵循项目既有的 CI/CD 标准。**“代码能跑”不是终点，“无警告通过本地所有检查并符合流水线标准”才是交付的最低门槛。** 在执行任何开发任务时，必须严格遵守以下规范：

### 1. 编码与测试先行
- **测试同步**：任何核心逻辑的修改（特别是后端核心业务、数据提取规则、工具类函数），必须同步更新或新增对应的单元测试。
- **禁止破坏性修改**：修改现有代码前，先运行相关测试；修改后，必须保证原有测试不被破坏。

### 2. 交付前本地验证（强制校验）
在宣布“任务完成”或向用户展示最终代码前，你**必须主动通过终端执行**以下检查，禁止“脑补”测试结果：
- **前端检查**：
  - 必须执行 `npm run type-check`，绝不允许存在 TypeScript 类型报错（严禁滥用 `any` 或 `@ts-ignore` 逃避检查）。
  - 必须执行 `npm run lint`（或对应脚本），修复所有格式和规范警告。
- **后端检查**：
  - 必须执行 `go test ./...` 确保所有后端测试通过。
  - 必须执行 `go vet` 等静态检查，修复所有潜在隐患。
- **未通过处理**：如果检查报错，必须自行分析报错信息，修改代码并重新运行检查，直到完全通过，才能向用户报告完成。

### 3. Git 提交规范
每次改动完成后，必须创建一个对应的 Git commit，以便后续追踪和回滚。必须按 Conventional Commits 规范生成 Message：
- 格式：`<type>(<scope>): <subject>`（Type 包含 feat, fix, docs, style, refactor, test, chore）。
- 描述必须精准，说明“解决了什么问题”或“实现了什么功能”。

### 4. 故障修复第一原则
- 每次交付给用户前，确保所有测试和验证全部通过。
- 如果用户反馈了 CI/CD 流水线的报错日志，必须**放下手头新功能的开发，将优先级提升至最高**，精准定位修复，而不是盲目重构。

## 提交前代码审查与自我优化机制（Mandatory Pre-Commit Code Review）

> **触发时机**：在完成功能代码编写、且准备生成 Git Commit 之前，你必须**主动**对自己刚刚编写/修改的所有代码执行一次深度的 Code Review。禁止写完直接提交。

在向我汇报“开发完成”或提供最终 Commit Message 前，你必须严格按以下 4 个维度进行自我审查，并在输出中提供**《交付前 Code Review 报告》**：

### 维度 1：架构与规范一致性（最高优）
- 审视前端：是否私自创建了本可复用的 UI 组件？是否混入了违规的内联样式 (`style="..."`) 或硬编码色值？是否完全遵循了本指南定义的 TDesign 令牌？
- 审视后端：是否破坏了既有的领域模型？新建的函数和结构体是否符合 WeKnora 的 Go 项目规范？

### 维度 2：健壮性与边缘状态 (Robustness & Edge Cases)
- 审查可能导致 Panic 或渲染崩溃的代码。
- 前端：是否对异步请求的数据做了判空与可选链（`?.`）处理？列表/表格是否考虑了 Empty（空数据）和 Loading 状态？
- 后端：是否忽略了 error 检查（`_ = err`）？是否存在协程（goroutine）泄漏、切片越界、或者未关闭的数据库/文件连接？

### 维度 3：性能与代码重构 (Performance & Code Smell)
- 是否有过于冗长的 `if-else` 嵌套可以提前 return 优化？
- 前端 Vue3 代码中是否存在不必要的响应式追踪？是否存在可以被提取为纯函数的重复逻辑？
- 后端是否在 for 循环中频繁查询数据库？是否可以改为批量查询？

### 维度 4：审查结果输出 SOP
在执行完上述思考后，你必须按以下格式向我汇报：
1. **拦截项 (Blockers)**：如果发现维度 1 和 2 的严重问题，你必须**直接重写并修复该代码**，而不是仅仅口头指出。
2. **优化建议 (Suggestions)**：如果发现维度 3 的可优化点，列出“原代码 vs 优化后代码”供我选择是否采纳。
3. **Commit 准备**：确认所有致命问题都已修复后，最后再根据 Conventional Commits 规范，输出准备执行的 Git Commit Message。

---

## 后端构建（Windows）

- **必须使用 UCRT 版 WinLibs 工具链**，MSVCRT 版无法链接 duckdb 静态库（duckdb 是 MSVC 编译的，MSVCRT 工具链会报 `__stdio_common_vsnprintf_s` 等未定义符号）。
  - 工具链目录：`C:\Users\koujiang\AppData\Local\Microsoft\WinGet\Packages\BrechtSanders.WinLibs.POSIX.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\mingw64\bin`
- 编译命令（在项目根目录执行）：
  ```powershell
  $ucrt = "C:\Users\koujiang\AppData\Local\Microsoft\WinGet\Packages\BrechtSanders.WinLibs.POSIX.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\mingw64\bin"
  $env:PATH = "$ucrt;$env:PATH"
  $env:CC = "$ucrt\gcc.exe"
  $env:CXX = "$ucrt\g++.exe"
  $env:CGO_ENABLED = "1"
  $env:CGO_CFLAGS = "-O2 -g -ID:\Downloads\WeKnora-main\third_party\sqlite3"  # sqlite-vec 编译需要 sqlite3.h
  go build -o server.exe ./cmd/server

```

* 构建产物必须输出到项目根目录 `server.exe`（计划任务 `WeKnoraServer` 执行 `scripts\start_server.bat` 运行的就是它）。

## 服务启动（Windows 计划任务）

* 已有计划任务：`WeKnoraServer`（后端）、`WeKnoraDocreader`（解析服务）、`WeKnoraVite`（前端）。
* 启动后端：`Start-ScheduledTask -TaskName WeKnoraServer`
* 端口约定：后端 8080（注意：`scripts\start_vite.bat` 曾写死 VITE_DEV_PROXY_TARGET=8088 导致 5173 代理 500，已改为 8080）、前端 5173、docreader gRPC 50051。
* docreader 可手动启动（当前 python 解释器版本不符时用项目虚拟环境）：
```powershell
$env:PYTHONPATH = "D:\Downloads\WeKnora-main"
D:\Downloads\WeKnora-main\docreader\.venv\Scripts\python.exe -m docreader.main

```



---

## 前端 UI 设计规范（强制）

> 本节规则适用于所有前端页面、组件、样式改动。目标：**统一视觉风格、最大化组件复用、彻底消除"AI 味"随机样式**。
> **AI 动作强制约束**：在生成任何 UI 组件代码前，你必须先在思考过程中检索 `frontend/src/components/` 目录，并列出即将复用的已有业务组件和 TDesign 组件名称，严禁直接手写原生 HTML 结构。

### 1. 设计令牌（Design Tokens）——单一事实来源

所有颜色、间距、字体、圆角、阴影、动效时长必须使用项目定义的设计令牌，**严禁使用硬编码和内联样式**。

* **令牌文件位置**：`frontend/src/assets/theme/theme.css`（基于 TDesign CSS 变量，主色已定制为绿色 `#07c05f`）
* **禁止**在组件中直接写入 `#3b82f6`、`16px`、`8px`、`border-radius: 12px` 等具体值。
* 新增令牌前，必须先在现有令牌中查找是否有可复用的语义化变量。

**核心语义化令牌参考：**

| 用途 | 令牌 |
| --- | --- |
| 主色 | `var(--td-brand-color)`（项目定制绿色 `#07c05f`） |
| 主色悬停 | `var(--td-brand-color-hover)` |
| 正文文字 | `var(--td-text-color-primary)` |
| 次要文字 | `var(--td-text-color-secondary)` |
| 边框 | `var(--td-component-stroke)` |
| 容器背景 | `var(--td-bg-color-container)` |
| 页面背景 | `var(--td-bg-color-page)` |
| 圆角默认 | `var(--td-radius-default)`（3px） |
| 圆角中 | `var(--td-radius-medium)`（6px） |
| 圆角大 | `var(--td-radius-large)`（9px） |
| 警告色 | `var(--td-warning-color)` / `var(--td-warning-color-1)` |
| 错误色 | `var(--td-error-color)` / `var(--td-error-color-1)` |
| 正文字号中 | `var(--td-font-size-body-medium)`（14px） |
| 标题字号中 | `var(--td-font-size-title-medium)`（16px） |
| 间距小 | `var(--td-comp-margin-s)` / `8px` 标准模数 |
| 间距中 (默认) | `var(--td-comp-margin-m)` / `16px` 标准模数 |
| 间距大 | `var(--td-comp-margin-l)` / `24px` 标准模数 |

### 2. 组件复用（避免重复造轮子）

1. **第一优先级（WeKnora 本地业务组件）**：在实现任何功能（如弹窗、表单、页面头部、列表卡片）前，必须先检索 `frontend/src/components/` 目录，复用项目已有的二次封装组件。
2. **第二优先级（TDesign 基础组件）**：本地无业务组件时，必须使用 TDesign Vue Next（`^1.19.2`）的基础组件。
3. **禁止行为**：
* 禁止修改 `node_modules` 下的 TDesign 源码。
* 禁止绕过现有封装，直接用原生 div 和基础 CSS 重新手搓一个类似的弹窗或卡片。
* 如需改变组件外观，优先通过组件的 `variant` / `size` / `theme` 属性调整，其次通过 `className` 配合全局令牌调整。



**TDesign 可复用基础组件速查：**

* 交互：`t-button`, `t-input`, `t-select`, `t-form`, `t-table`
* 容器：`t-dialog`, `t-drawer`, `t-tabs`
* 反馈：`t-tooltip`, `t-popconfirm`, `t-message`, `t-empty`, `t-loading`
* 图标：统一从 `tdesign-icons-vue-next` 引入，严禁使用 Emoji 或第三方图标包。

### 3. 严格禁止项（Negative Constraints）

* **禁止内联样式**：绝对禁止使用 `style="..."` 编写业务组件的外观样式（如 `<div style="margin: 16px; color: #333;">`），必须通过 `<style scoped>` + CSS 变量实现。
* **禁止硬编码色值**：严禁出现 `#fff`, `#3b82f6`, `rgba(0,0,0,0.5)` 等硬编码颜色，必须映射到 TDesign 令牌。
* **禁止私造布局**：禁止随意改变页面级布局间距、字体大小层级；必须沿用项目已有的排版比例和网格间距。
* **禁止乱用第三方库**：禁止引入新的 UI 库、图标库或 CSS-in-JS 方案（如未配置的 Tailwind 默认类名）。
* **禁止滥用 scoped 逃逸**：`<style scoped>` 仅允许用于承载**页面的 Flex/Grid 结构与业务特化间距/尺寸**，严禁在里面重置基础 UI 组件颜色或破坏全局主题。

### 4. 前端改动验收清单（每次交付前必须自检）

* [ ] 代码中**不存在** `style="..."` 这种写死颜色、间距的内联样式。
* [ ] 所有颜色、间距、字体、圆角、阴影均来自 TDesign 设计令牌，**无任何硬编码像素值/色值**。
* [ ] 所有基础 UI 元素均已复用 WeKnora 本地封装组件或 TDesign 组件，**未新建重复的平行组件**。
* [ ] 页面在视觉上与项目现有页面风格完全一致，未引入新的 UI 库或图标。
* [ ] 相关测试已更新并通过，且已创建对应的 Git commit。

### 5. 前端技术栈约定

* 框架：Vue 3（^3.5.34）+ Composition API + `<script setup>`
* 构建工具：Vite（^7.3.5）
* 语言：TypeScript（~6.0.3）
* UI 组件库：TDesign Vue Next（^1.19.2）
* 图标库：tdesign-icons-vue-next（0.4.4）
* 状态管理：Pinia（^3.0.4）
* 路由：Vue Router（^4.5.0）
* 样式方案：TDesign CSS 变量 + 组件内 `<style scoped>`（仅限布局结构/尺寸，外观视觉强绑定 TDesign 令牌）
* 代码检查：`npm run type-check`（vue-tsc --build）

## 核心底座保护与“无侵入式”二开规范（强制拦截级）

> **背景定义**：WeKnora 原项目是一个稳定、完整的企业级知识库系统。我们当前的所有二开需求（如车队管理、维保档案等），均属于**“外挂旁路功能”**。
> **核心宗旨**：**绝对禁止为了实现新业务而篡改、破坏或污染 WeKnora 的原有核心链路与底层逻辑。** 在执行任务时，必须严格遵守以下 5 条红线：

### 1. 核心代码“黑盒/只读”原则
- 将 WeKnora 原有的核心模块（如权限认证、核心知识库 CRUD、全局通用 UI 组件、基础 Utils 工具类）视为**第三方开源库 (Black-box)**。
- **严禁篡改原函数签名**：如果不满足你的需求，禁止直接修改原有核心函数或 API 接口的入参和出参结构，禁止在通用基础组件中硬编码注入你的特化业务判断（如 `if (isVehicle) { ... }`）。
- **允许的姿势**：你应该直接调用它们，或者在你的业务目录中对其进行高阶封装（Wrapper/Adapter/Decorator）。

### 2. 数据库“旁路扩展”原则
- **严禁直接污染核心表**：如果需要给原系统的核心实体（如用户、部门、文档）增加字段，**严禁**直接执行 `ALTER TABLE` 在原表加字段。
- **允许的姿势**：必须创建独立的扩展表（如 `ext_user_profiles`、`fleet_vehicle_archives`），通过外键（ID）与核心表进行关联。确保即使把你的扩展表删光，原系统依然能正常运行。

### 3. 文件与目录的物理隔离
- 新增的外挂业务模块必须与 WeKnora 核心模块在目录上保持物理隔离。
- 前端：新业务页面必须建立在独立的文件夹中（如 `src/views/FleetManagement/`），禁止与原知识库页面混杂。
- 后端：新业务逻辑必须建立在独立的包/模块中（如 `internal/fleet/`），并通过注册独立的路由组（Router Group）对外暴露，禁止直接在原知识库的 Controller/Service 中续写代码。

### 4. 依赖复用与状态隔离
- **复用但不共享状态**：前端新页面必须复用 WeKnora 现有的全局状态（如当前登录用户信息 `useUserStore`），但严禁在原有的核心 Store 中塞入你的外挂业务状态（如在 `userStore` 里放 `currentVehicleId`）。新业务必须新建专属的 Store。
- **复用核心 API**：原项目提供的文件上传接口、人员查询接口、字典接口等，能直接复用的必须直接复用，绝对禁止重复写一套“车队专用文件上传”接口。

### 5. 核心侵入“熔断审批”机制
如果你在开发过程中发现：**如果不修改原有的 WeKnora 核心底层代码，这个新功能就绝对无法实现**（例如需要在核心中间件、生命周期钩子中打洞）。
- **强制熔断**：你必须立即停止写代码！
- **申报格式**：向我输出《核心侵入风险报告》，说明“为什么必须改核心”、“打算怎么改”、“对原有知识库功能的爆炸半径（Blast Radius）影响评估”。
- **等待指令**：只有在我明确回复“同意修改核心”后，你才能继续编码。

### 7. UX 文案与微交互用语规范 (强制)
> **核心痛点**：禁止在 UI 面板、按钮、占位符中生成“对话式、说明书式”的冗长文案。文案必须极简、克制、专业，符合现代企业级中后台产品的标准。

在生成界面的任何中文文本时，必须严格遵守以下 4 条红线：

#### 7.1 按钮与动作标签（动作极简原则）
- **字数限制**：按钮文本严格控制在 **2~4 个中文字符**，禁止出现短句。
- **动词统一**：
  - [标准] `保存` / [禁止] `保存数据`、`确定保存`、`保存配置`
  - [标准] `新增` / [禁止] `添加新记录`、`创建数据`
  - [标准] `确定` / [禁止] `确认执行`、`好的`
  - [标准] `删除` / [禁止] `删除选中项`、`移除数据`
  - [标准] `导出` / [禁止] `导出到Excel`

#### 7.2 标题与副标题（消除废话原则）
- **主标题**：必须是纯名词或动宾短语（如 `车辆档案`、`维保记录`），禁止写成完整的句子（如 `车辆档案管理列表`）。
- **副标题/说明文字**：**非必要不说明**。如果界面的用途一目了然，直接不写副标题。如必须写，严格限制在 **15 个字以内**，且直接陈述结果。
  - [禁止（AI 味废话）]：“在这里您可以查看和管理所有车队的车辆基础信息，点击右侧可以新增。”
  - [标准（极简）]：“管理车队车辆的基础档案与证照。”

#### 7.3 占位符与表单提示 (Placeholder & Tooltip)
- **输入框占位符**：使用 `请输入 + 字段名`，禁止啰嗦。如 `请输入车牌号`，禁止写 `请在此输入您的车牌号码以便查询`。
- **下拉框占位符**：使用 `请选择 + 字段名`。
- **Tooltip (悬浮提示)**：只用于解释非常规名词或严重后果，绝对禁止解释常规按钮（如“编辑”按钮上加个 tooltip 写“点击编辑此行数据”，这是严令禁止的视觉垃圾）。

#### 7.4 人称与语气（去拟人化）
- 界面文案必须客观、冷静。
- **绝对禁止**使用对话式代词：“您”、“我们”、“为你”。
- **绝对禁止**使用说明书式句式：“在这里可以...”、“系统会自动...”、“请注意...”。
- 举例：将“系统已成功为您删除了该条记录”改为极简的 `删除成功`。

---

## 打印/目录生成规则（强制）

> 适用于所有列表页的"打印""目录打印""打印预览"功能。

1. **所见即所得**：打印/PDF 生成的列必须严格使用当前列表 `visibleColDefs`（用户在字段筛选器里勾选并显示的列），**禁止**在打印函数里另写一份硬编码字段列表、禁止 `.slice(0, N)` 截断、禁止把 `data.*` 列过滤掉再手动补前几列。
2. **列顺序与列表一致**：打印列顺序必须与列表列顺序一致，不得重新排列、不得增删字段。
3. **状态列自动计算**：证照状态/证件状态等由系统根据有效期自动计算的列，打印时同样走 `calcCertStatus` 逻辑，不得依赖模型提取值。
4. **空值安全显示**：未提取到的字段打印为空（`-` 或空白），**不得**从打印输出里隐藏该列；取值必须使用安全可选链（如 `row.data?.[col.key] ?? '-'`），严禁未判空直接访问子属性导致渲染崩溃。
5. **选中行优先**：用户勾选了记录时打印选中行，未勾选时打印当前筛选后的全部行。
6. **复用 `useCatalogPdf.ts**`：目录式打印统一走 `frontend/src/views/dailyAffairs/useCatalogPdf.ts` 的 `generateCatalogPdf`，不要在各页面里重复实现 PDF 渲染。

---

## 字段配置与筛选器规则（强制）

> 适用于车队管理「车辆档案」「司机档案」「维保管理」等证照列表页的字段筛选器、设置抽屉、重置按钮。

1. **base/detail 划分是权威来源**：每个证照类型在 `BUILTIN_CERTS` 里定义的 `base`（基础字段）和 `detail`（详细字段）是唯一事实来源，**禁止**在运行时用用户启用状态去覆盖 base/detail 划分。
2. **base 字段始终显示**：`base` 字段不受用户启用/禁用状态影响，始终出现在字段筛选器和列表中；重置后必须全部勾选。
3. **detail 字段按启用状态显示**：`detail` 字段根据用户在设置抽屉里的启用状态决定是否出现在筛选器和列表中；未启用的 detail 字段不显示。
4. **用户新增字段归入 detail**：用户在内置字段之外新增的字段，一律归入 `detail`，默认不勾选。
5. **重置按钮 = 恢复 base**：点击"重置"时，`visibleKeys` 必须等于 `columnDefs` 中 `def=true` 的列（即 base 字段），不得混入 detail 字段。
6. **持久化存档校验**：localStorage 里的字段存档必须完整包含全部默认 base 列才采用，否则自动重置为新默认，避免列定义升级后旧存档导致缺列。
7. **列表/打印/编辑抽屉字段同源**：列表显示列、打印列、编辑抽屉字段必须来自同一份 `columnDefs`/`visibleColDefs`，不得在不同地方各自硬编码一份字段列表。
8. **字段三方同步**：字段筛选器勾选项、列表显示列、编辑抽屉字段必须三方同步——用户在设置抽屉里增删改字段后，字段筛选器、列表、编辑抽屉必须同时更新；禁止编辑抽屉自行加列或漏列。
9. **状态列自动计算**：证照状态/证件状态/保单状态等由系统根据有效期自动计算的列，不依赖模型提取值，编辑抽屉里可手动修改但有默认值。
10. **列顺序以字段配置 subs 数组为唯一权威**：列表列、字段筛选器、编辑抽屉三处的字段排列顺序，必须按该证照类型在 `categories[scope].subs` 里的数组顺序输出；base/detail 只决定「默认是否勾选显示」（def 判定用 baseSet），不得用内置 `BUILTIN_CERTS.base/detail` 的硬编码顺序覆盖用户在字段配置里拖动后的顺序。用户在字段配置里调整 subs 顺序后，列表列、筛选器、编辑抽屉必须立即同步。无 subs 时回退内置顺序兜底；用户新增字段追加末尾。
11. **字段开关「四同步」**：字段配置里某个字段的状态开关关闭后，该字段在「列表表头、字段筛选器、编辑抽屉、打印预览」四处必须同时隐藏；四处字段均派生自同一份 `archiveTypeFields`（源头已 `.filter(s => s.enabled !== false)`），禁止在任一处各自维护一份独立字段列表或绕过该源头。分类开关关闭后，该分类的类型卡片、筛选下拉选项、上传类型选项均不显示（内置未入库分类默认启用）。
12. **「启用表头」开关与字段筛选器勾选双向同步**：字段配置里的「启用表头」开关（后端 `FleetCategorySub.is_default`）= 该字段在字段筛选器里默认勾选/在列表表头显示。用户在字段配置里切换并保存后，列表页必须按最新 `is_default` 重建筛选器勾选（`resetColumns()`），**不得**继续沿用旧的 localStorage 勾选存档。实现：字段配置保存后 `dispatchEvent('fleet-categories-changed')`，列表页 `reloadCategories()` 末尾在 `nextTick` 里 `resetColumns()`。后端 `is_default` 必须随 subs 一起 JSON 透传持久化（`FleetCategorySub` 结构体必须含 `IsDefault bool json:"is_default"`，否则会被序列化丢弃导致开关回读丢失）。
13. **分类名称、顺序、别名全局六同步**：用户在「字段配置」里对证照分类的改名、拖拽排序、启用/禁用，必须同时同步到以下六处，任何一处硬编码或用本地兜底数据都会导致闪烁/不一致：
* ① 设置抽屉内分类列表（权威源，`categories[scope]` 数组顺序即排序）；
* ② 主页面概览卡片分组标题（取 `fleet_group_aliases.name`，key=`${scope}:${group_key}`，builtin group_key=`company/driver/maintain`）；
* ③ 主页面概览类型卡片顺序（按 `categories[scope]` 的 name 顺序 sort，禁止按 `overviewStats.by_doc_type` 的返回顺序渲染）；
* ④ 工具栏证照类型筛选下拉；
* ⑤ 提取规则面板证照类型下拉；
* ⑥ 上传弹窗证照类型下拉。
* **反闪烁门控**：设置抽屉首次打开时，`loadCategories()`、`loadCustomGroups()`、`loadGroupAliases()` 三个异步必须 `await Promise.all` 全部完成后才 `ready=true` 渲染内容，禁止先用硬编码 GROUPS label + 本地 BUILTIN 字段数渲染再异步跳变。
* **import 必须显式**：新用到的 API（如 `listFleetGroupAliases`）必须在文件头 import 行里显式列出，调用时被 catch 吞掉会静默失效。


14. **主页面类型卡片以 enabled categories 为唯一权威**：概览卡片页必须遍历后端返回的 `categories[scope]`（且 `enabled !== false`）来出类型卡片，**禁止**遍历 `overviewStats.by_doc_type` 出卡——后者用记录里存的旧名（如"维修工单"），且 0 记录分类不显示。计数按 `name → builtin_key` 匹配 `overviewStats.by_doc_type`，匹配不到缺省 0。**整个删除内置未入库兜底段**：否则改名内置分类（如"维修工单"→"一般维修"）会同时出现"一般维修"（来自 categories）和"维修工单"（来自 by_doc_type）两张重复卡。
15. **上传弹窗默认类型前缀规范化**：`FleetUploadDialog` 的默认选中值构造必须将 scope 规范化为短前缀（`vehicle-archive → vehicle`, `driver-archive → driver`, `maintain-archive → maintain`），构造方式为 ``${normalizedScope}__${defaultType}``，**禁止**硬编码 `'vehicle__'`。否则司机档案弹窗会显示 `vehicle__驾驶证`（在 driver scope 选项里匹配不到，直接露出原始 value）。

---

## 窄屏自适应规则（强制）

> 适用于车队管理概览卡片、状态卡片、证照类型卡片等网格布局。

1. **禁止内部滚动兜底**：卡片网格容器**禁止**加 `overflow: auto/scroll` 作为窄屏方案；窄屏时优先调整每行卡片数量和卡片尺寸。
2. **断点自适应**：
* 默认屏宽：`grid-template-columns: repeat(3, 240px)`（或等同宽度）。
* ≤1200px：改为 `repeat(2, 1fr)`，卡片宽度自适应撑满。
* ≤768px：改为 `1fr`，单列堆叠。


3. **卡片尺寸自适应**：卡片用 `1fr` 或 `auto` 宽度，禁止写死 `width: 240px` 在窄屏断点下；用媒体查询切换列数即可。
4. **容器高度自适应**：概览容器高度随内容撑开，禁止写死 `height` 或 `max-height`。

---

## AOCI 仓库认知

AOCI 为本仓库维护一个稳定、可版本化、可增量更新的仓库级认知层，供模型跨任务复用对系统的理解。

`aoci.txt` 是面向模型的结构化认知索引。它以每个受管理文件、数据库表或其他受管理对象一条独立 Entry 的方式，用符号标签与 F/R/A/S 语义表达对象的核心职责、重要关系、对外契约，以及理解或修改系统时必须知道的非显然约束和设计决策。

Header、目录段和全部 Entry 共同组成完整仓库索引，可以覆盖前端、后端、配置、数据库结构及其他受管理内容。受管理内容发生变化时，通常只需维护受影响的认知条目，不需要重新生成整个索引。

AOCI 提供系统架构、对象职责、重要关系、对外契约和关键约束的高密度视图。

### 工作原理

AOCI 采用“模型生成、模型读取”的认知闭环。

Header、Entry 和 Curation 语义的创作只按当前机器签发的 Plan 与实时 Guide 执行；由 Host 模型基于当前绑定证据独立完成。

Entry 的语义必须来自模型对真实证据的理解。不得仅依据路径、文件名、扩展名、AST、符号列表、依赖扫描、正则、固定模板或规则引擎推导、预填、拼接或改写索引语义。

对 Fresh Bootstrap，只按当前机器签发的 Plan 和实时 Guide 执行。当它们要求创作时，Host 模型创作 Root、Meta、Tag 和 F/R/A/S，提供 authoring-run 声明，并把它绑定到 Plan、Evidence 与完整 Candidate。不得要求 AOCI 填写 `origin=host_model`、制造 Receipt 或把程序生成的 Framework 当作语义。本文件不自行重建 Onboarding 流程。内部批次不是用户决策；只有遇到既有批准边界或真实的安全、漂移、CAS、Recovery 条件才停止。

### 最小使用入口

* `aoci_rules`：取得当前AOCI版本的会话运行合同。
* `aoci_overview`：建立或恢复本仓库的完整认知。
* `aoci_maintain`：受管理对象达到最终稳定状态后检查认知是否需要维护。
* `aoci_update_entry`：提交与当前证据和源码摘要绑定的完整语义更新批次。
* `aoci_report`：仅当当前布局和工具状态支持时，在证据不足、无法可靠生成语义时登记待办，不猜写。

其他MCP工具、CLI命令、参数和专项流程，以当前工具说明、Guide和 `--help` 返回内容为准，不在本文件中重复完整手册。

本区块只规定仓库接入、认知使用和收尾原则。`aoci_rules` 承载当前会话合同，Guide实时输出承载当前Plan的执行顺序与停点，工具Schema、Spec和Validator承载机器结构与判据；Prompt、Description、README和静态文档不能覆盖这些机器事实。

### 建立、生成和恢复认知

1. 每个新的 Agent Run 开始时，应先判断：
* 本仓库是否已经存在可用的完整AOCI索引；
* 当前上下文中是否已有与本仓库根、当前索引版本和当前AOCI服务相匹配，并且模型仍可可靠使用的完整仓库认知。


2. 仓库已经存在可用的完整索引，但当前Run没有可靠完整认知时，先调用 `aoci_rules`，再调用 `aoci_overview`。
完整认知仍可靠时直接复用。局部不确定本身不要求机械重读系统全貌。
本Run从已知Host上下文压缩恢复时（包括宿主注入的压缩摘要），必须把此前模型认知视为不可靠。压缩handoff不得保留或摘要正式Whole-Index，也不得保留或摘要任何Overview Header、Entry、Chunk、Challenge或Attestation正文；只能保留安全续接所需的receipt身份、未完成write或Recovery状态，以及立即重载指令。复制进handoff的Whole-Index语义或receipt不能证明恢复后模型的当前认知可靠。若当前上下文已无法可靠保留运行合同，先调用 `aoci_rules`。继续业务任务前，使用 `refresh_reasons=["context_compaction"]` 和新的 `refresh_event_id` 调用普通完整Whole-Index `aoci_overview`（不设置 `check_only` 或设为false）；不得使用 `check_only` 或认知probe。原样跟随每个 `next_cursor` 直到 `completed=true`，确认交付，并且只基于新交付正文提交一次Attestation。完成这次新的完整传输后，即使Attestation为partial或fail也消费该generation，并按既有合同继续source-bound任务，不再自动调用第二次Overview。
AOCI可以针对 `context_compaction`、项目 `cognition_refresh_threshold` 下的机器 `semantic_threshold` 或主要 `phase_transition` 提供checkpoint与认知状态事实。只需要这些紧凑事实时使用 `check_only=true`；这些事实只向Agent提供建议，不替模型决定是否需要系统全貌。
Agent显式调用普通 `aoci_overview`（未设置 `check_only` 或为false）时，只要能形成一致的CognitionSet，AOCI必须完整交付请求scope。不得因为已有receipt、阈值未达到或没有待处理刷新原因而抑制正文。正式认知Dirty或Stale时仍交付正文，但必须标记不可靠。存在未决恢复或无法形成一致snapshot时失败关闭，不返回混合正文。
普通Overview返回 `continuation_required=true` 时，必须原样提交 `next_cursor` 并自动继续到 `completed=true`。不得询问用户、开始业务任务或给出阶段性系统结论。Host截断、缺块、重复、乱序、cursor失败、Index变化或`chunk_tokens`变化时停止本次认知链。Attestation完成前不得用Memory、源码、Spec、`aoci.txt`、历史会话、scope、search或Entry读取修补或补充Whole-Index认知。Challenge ordinal是正式Entry序列中的1-based位置；Header内容、注释、空行、Section/Overview/Chunk Marker、Receipt与Metadata均不计数，Chunk Receipt ordinal使用同一序列。Attestation必须原样回绑本次Challenge发布的当前`index_sha256`、`entry_sequence_sha256`与`entry_count`；旧Index、旧Entry序列、旧数量或旧Attestation均无效。完整链结束后只正式提交一次既有模型认知Attestation；同一响应只允许一次不改变语义答案的JSON Schema或字段格式修正。对象、Tag或F不匹配即失败且认知吸收不确定，不得语义重试或旁路补答。首次认知失败时还不得执行Root/Meta、Migration、全局布局或其他未重新绑定的系统级决策。上下文压缩刷新若传输完整、认知身份不变、治理对齐且没有Recovery或第三方冲突，即使Attestation为partial或fail也消耗该refresh generation，并继续原任务，不再自动重读Overview。`system_mastery_percent`只自评系统框架——架构、职责、强关系、稳定外部契约以及高熵安全和维护约束——不表示完整实现或运行实况知识；机器索引覆盖率必须分开。默认只向用户输出由本次真实覆盖率、Challenge、块数、Token和掌握度生成的规定成功或失败一句话。Host截断时提示用户把 `overview_delivery.chunk_tokens` 设置为更小的合法值后重新开始，不得自动修改。
加法认知等级必须与严格证明字段分开解释。`delivery_verified`表示已加载Index且Host交付已确认，但完整认知验证仍未完成；应表达为“已加载且交付已验证”，不得描述为“没有认知”或“没有理解系统”。`cognition_verified`要求Attestation通过（Challenge至少80%的ordinal完全正确且对象身份至多失手一处），`cognition_governed`还要求治理对齐。通用完整读取失败句只用于真实交付故障。
当Overview响应包含可选`cognition-state/v2`投影时，必须分别解释各维度。其Level止于`model_cognition_usable`；`strict_attestation_verified`、`governance_aligned`与`current_system_cognition_reliable`都是独立状态，绝不参与该Level。ordinal、对象身份、Tag或核心F不匹配可以导致严格Attestation失败，而模型认知仍然可用；不得仅凭这种不匹配就宣称模型没有理解系统。只有`current_system_cognition_reliable=true`允许无保留地声称当前完整系统认知可靠。投影缺失时继续使用上述Legacy解释。
普通的只读审计、分析、检查、不修改代码或不提交、不push，不自动等于严格零写入，也不改变上述认知有效性判断。Codex Memory和历史Skill只能辅助恢复经验、用户偏好与调查方向，不能替代与当前仓库根、索引摘要、AOCI服务身份和认知范围匹配的当前认知收据；项目AGENTS和当前AOCI身份在AOCI状态上优先于历史Memory。
只有用户明确禁止Ledger、元数据、`.aoci`运行资产及任何文件写入时，才按严格零写入处理。若必要的认知建立与该边界冲突，必须报告冲突并请求用户裁决或建议使用隔离副本，不得静默以Memory替代当前仓库认知。
3. 仓库没有可用的完整索引，或当前只有最小骨架、Header不完整、Entries未完成、必要Curation尚未裁决时，如果需要建立正式完整AOCI索引，先取得 `aoci_rules`，然后进入当前AOCI Guide。由Guide依据仓库真实状态决定下一阶段并完成必要安全步骤。
`aoci_maintain` 不替代索引建立流程。
不在本文件中自行重建或硬编码完整索引生成状态机。
4. 在长程任务中，模型负责保留当前认知收据并正确使用刷新门禁：
* Host报告上下文压缩或模型已知系统全貌丢失时，执行上述强制 `context_compaction` 重载规则；AOCI不能自行推断Host事件；
* 进入真正的主要阶段时声明 `phase_transition`，不得把函数、测试运行或小步骤当作阶段；
* 在有用的稳定检查点通过 `check_only=true` 取得机器语义计数；
* 除已知压缩的强制重载外，由Agent判断当前任务是否需要再次显式获取指定scope或完整Overview；
* 在维护和对齐完成前，保留AOCI报告的Dirty或Stale可靠性状态。



### 任务收尾与认知维护

5. 纯只读问答、分析、版本核验，或没有产生受AOCI管理对象变化的任务，不需要调用维护工具。当前AOCI版本是任意`aoci_overview` check_only或`aoci_maintain`响应里的`cognition_receipt.mcp_service_version`；二进制路径是项目`.mcp.json`里的`command`，CLI不必在PATH上。
6. 发生受AOCI管理对象变化时，待其达到本次任务的最终稳定状态后，只调用一次 `aoci_maintain`。不要在每次中间修改后逐文件维护。
7. 若维护结果返回真实语义候选，Host 模型必须基于每个候选绑定的对象和必要证据，独立创作完整标签与F/R/A/S更新。通过 `aoci_update_entry` 一次提交当前机器签发批次的完整候选集合，同时原样保留每项 `source_sha256`、`candidate_id` 与对应domain批次身份。`max_entries`只限制单次请求和原子事务，不限制logical plan、Whole-Index或Managed Scope。`remaining`非零时，在当前批次成功Apply后重新调用Maintain并从新preimage继续；绝不能为满足transport上限缩减Index覆盖或自行截取返回批次。
没有足够证据且当前布局支持 `aoci_report` 时，使用它而不猜测、套用模板或为消除待办而生成缺乏证据的认知。
8. 必须遵守工具返回的结构化状态和安全边界：
* `repair_required`：只修复明确命中的候选，再重新提交当前机器签发的完整批次；
* `stopped`：结束当前写入尝试并检查 `failed_step`、错误、正式写入证据与Recovery。auto模式下，已证明零写入则记录closure并重新Plan；完整Intent和可证明postimage则Resume；策略要求Rollback且preimage可证明则精确恢复后重新Plan。只有证据不足、第三方正式字节冲突、需要审批或外部动作，或命中其他真实安全边界时，才停止整个用户任务；
* 冲突、审批、人工裁决、权限和安全信号不得忽略；
* 已经对齐后不得重复维护或重复写入；`refresh_ready_for_overview` 是checkpoint事实，由Agent决定是否为下一阶段请求普通完整Overview。


维护完成后如果又修改了任何受管理对象，之前的维护结果失效，应在新的最终稳定状态重新完成收尾。
9. 用户只限制业务文件范围，但没有明确禁止仓库托管资产时，AOCI托管资产可以在收尾阶段为保持认知一致而更新，并应在审计和提交中与业务文件区分。
用户明确禁止修改 `aoci.txt`、`.aoci`、元数据或任何额外文件时，以用户限制为准，不得写入，并如实报告剩余不一致。

### 专项流程

初始化、完整索引生成、Header生成、Entries生成、数据库结构索引、Curation、人工评审和故障恢复，只按当前AOCI Guide或工具在对应阶段返回的指令、命令和安全停点执行。

不预加载、不猜测，也不自行重建这些专项流程。平台调用方式、请求格式、批次上限、审批规则、索引格式细节和恢复步骤由对应Guide、工具说明、模型Prompt和CLI帮助按需提供。