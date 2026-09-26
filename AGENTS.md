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

### 5. CI/CD 与 Docker 构建避坑指南（强制约束）
在维护或扩展自动化流水线时，必须牢记以下历史教训，严禁重犯：
- **精准白名单测试**：在 CI 环境中执行 `go test` 时，绝对禁止使用全局 `./...` 或黑名单排除法。必须显式指定沙盒二开目录，并配合 `-run` 正则参数（如 `go test -run "TestBusiness" ./internal/...`），彻底隔离上游官方可能损坏的同包测试用例。
- **CGO 依赖补齐**：GitHub Actions 等 CI 容器默认无 C 编译环境，执行 `go vet / go test` 前必须显式安装 `gcc` 和 `libsqlite3-dev` 等依赖，并声明 `CGO_ENABLED=1`。
- **Dockerfile 可选复制（容错）**：`COPY` 指令绝不支持 `|| true` 或 `2>/dev/null` 等 Shell 语法。若需实现“文件存在则复制，不存在则忽略”，必须使用通配符（例如 `COPY path/go.mo[d] dest/`）。
- **BuildKit 专属 Ignore**：为了不侵入官方根目录的 `.dockerignore` 且不被其屏蔽构建上下文，必须使用 `Dockerfile.<name>.dockerignore` 机制（如 `Dockerfile.da.dockerignore`）来放行二开所需目录。

---

## 提交前代码审查与自我优化机制（Mandatory Pre-Commit Code Review）

> **触发时机**：在完成功能代码编写、且准备生成 Git Commit 之前，你必须**主动**对自己刚刚编写/修改的所有代码执行一次深度的 Code Review。禁止写完直接提交。

在向我汇报“开发完成”或提供最终 Commit Message 前，你必须严格按以下 4 个维度进行自我审查，并在输出中提供**《交付前 Code Review 报告》**：

### 维度 1：架构与规范一致性（最高优）
- 审视前端：是否私自创建了本可复用的 UI 组件？是否混入了违规的内联样式 (`style="..."`) 或硬编码色值？是否完全遵循了本指南定义的 TDesign 令牌和原项目页面骨架？
- 审视后端：是否破坏了既有的领域模型？是否严格遵守了 API 响应契约（Success/Error 格式）？

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

## 前端 UI 设计规范（强制）

> 本节规则适用于所有前端页面、组件、样式改动。目标：**统一视觉风格、最大化组件复用、彻底消除"AI 味"随机样式，高度贴合 WeKnora 原生风格**。

### 1. 设计令牌（Design Tokens）——单一事实来源
所有颜色、间距、字体、圆角、阴影、动效时长必须使用项目定义的设计令牌，**严禁使用硬编码和内联样式**。
* **令牌文件位置**：`frontend/src/assets/theme/theme.css`（基于 TDesign CSS 变量，主色已定制为绿色 `#07c05f`）
* **禁止**在组件中直接写入 `#3b82f6`、`16px`、`8px`、`border-radius: 12px` 等具体值。

### 2. 页面骨架与高频组件复用（原项目基准）
在实现任何功能前，必须优先复用原项目已被验证的布局与组件体系：
- **标准页面骨架**：列表页必须沿用原项目的嵌套层级：
  `.kb-list-container` -> `.kb-list-content` -> `.header` (含 `.title-row` 和 `.header-subtitle`) -> `ResourceListToolbar` -> `.kb-list-main` (含 `EmptyState` 与网格/列表)。
- **强制复用白名单**（禁止重新手搓原生实现）：
  1. `SettingDrawer` (右侧抽屉，支持拖拽调宽/全屏)
  2. `ResourceIcon` (资源类型统一图标)
  3. `EmptyState` (空数据状态)
  4. `UploadTasksPanel` (上传进度浮层)
  5. `ResourceListToolbar` (列表通用工具栏，含搜索/筛选)
  6. `MessagePlugin` (全局提示，来自 tdesign-vue-next)

### 3. 严格禁止项（Negative Constraints）
* **禁止内联样式**：绝对禁止使用 `style="..."`。必须通过 `<style scoped>` + CSS 变量实现。
* **禁止私造布局**：禁止随意改变页面级布局间距、字体大小层级；必须沿用项目已有的排版比例。
* **禁止乱用第三方库**：禁止引入新的 UI 库、图标库或 CSS-in-JS 方案。
* **绝对禁用自造核心组件**：严禁手搓网格列表（禁止使用类似 `display: grid` 模拟表格）、严禁手搓抽屉和拖拽手柄。必须强制使用 TDesign 的 `t-table`（配合具名插槽处理复杂渲染）和官方定制的 `SettingDrawer`。
* **状态桥接规范**：在处理 `t-table` 的复杂交互（如动态列、多选 `selectedRowKeys`、展开行 `expandedRowKeys`）时，必须使用 `computed` 或拦截函数（如 `@select-change`）将 TDesign 的数组状态与业务底层的 `Set` 数据结构进行无缝桥接，严禁破坏原有业务逻辑。
* **设计令牌优先**：除了 ECharts Canvas 或极个别 JS 动态注入节点的合理例外，严禁在 Vue 文件中使用硬编码色值（`#xxxxxx`）和违规内联样式（`style="..."`）。渐变色也必须使用 TDesign 的语义色阶（如 `var(--td-brand-color-3)`）进行映射。

### 4. 业务流水线与防御性编程（S4 重构后铁律）

1. **统一业务流水线（五步闭环）**：日常事务与档案管理类模块，必须严格遵循"上传摄入（Wizard/Dialog）→ 规则配置（Panel）→ 核心列表展示（t-table）→ 详情交互（SettingDrawer）→ 批量操作（操作栏）"的标准开发流，严禁私造同类轮子。
2. **t-table 动态列 colKey 禁用点路径**：动态列的 `colKey` 禁止直接使用 `data.工单号` 这类带点深层路径——TDesign t-table 内部会按点路径自动取值，当 `row.data` 为 undefined 时直接崩溃。必须用安全字符（如 `data__工单号`），并在 `cell` 渲染函数里 try/catch 兜底返回 `-`。
3. **SettingDrawer 响应式对象必须预初始化空壳**：所有传入 `SettingDrawer` 或表单的响应式对象，定义时必须初始化空壳（如 `reactive({ data: {} })`），杜绝组件隐藏态 slot 仍被渲染时访问 `form.data[key]` 导致白屏崩溃。
4. **多选/展开行状态桥接**：`t-table` 的多选 `selectedRowKeys`、展开行 `expandedRowKeys` 必须通过 `computed` 配合 `@select-change` 拦截，无缝桥接到业务底层的 `Set` 数据结构，禁止破坏原有业务逻辑。

---

## 核心底座保护与“无侵入式”二开规范（强制拦截级）

> **核心宗旨**：绝对禁止为了实现新业务而篡改、破坏或污染 WeKnora 的原有核心链路与底层逻辑。

### 1. 核心代码“黑盒/只读”原则
- 将 WeKnora 原有的核心模块（如 `KnowledgeHandler`、全局组件、基础 Utils）视为第三方开源库。
- **严禁篡改原函数签名**：禁止直接修改原有核心函数或 API 的入参出参，禁止在基础组件中硬编码注入业务判断。

### 2. 沙盒适配器模式（Sandbox Adapter Pattern）
- **Handler 隔离**：新业务 Controller 必须新建独立结构体（如 `BusinessExtractHandler`），通过结构体嵌套复用核心底座，严禁将业务路由直接挂载在核心基类上。
- **Service 层下沉**：数据库交互、结构体组装、大模型调用等复杂逻辑，必须剥离到沙盒专属 Service 中。
- **路由追加**：在注册二开路由时，必须通过 `if handler != nil` 门控，以非侵入式追加至官方路由组末尾，并保留上游鉴权中间件。

### 3. 沙盒 API 响应契约（强制）
新写的后端接口必须与原项目官方接口在风格上“以假乱真”，严格遵守以下约定：
- **成功响应格式**：必须输出标准的 `c.JSON(http.StatusOK, gin.H{"success": true, "data": ...})`，禁止自造 JSON 结构。
- **异常处理规范**：绝对禁止在 Handler 中使用 `c.JSON(400, ...)` 硬编码抛出错误。必须使用 `errors.NewNotFoundError()`, `errors.NewForbiddenError()`, `errors.NewInternalServerError()` 等构造 `AppError`，交由全局中间件统一拦截与序列化。
- **标准状态码**：成功返回 200，参数错误 400，权限不足 403，资源不存在 404，状态冲突 409。

---

## UX 文案与微交互用语规范 (强制)

> **核心痛点**：禁止在 UI 面板、按钮、占位符中生成“对话式、说明书式”的冗长文案。文案必须极简、克制、专业，符合现代企业级中后台产品的标准。

### 1. 对齐基准与反面案例 (AI 味 vs 原项目风格)
在生成任何文案前，请默念以下 3 组对齐案例：
- ❌ AI 味：`在这里您可以管理所有车队的车辆档案信息，点击右侧按钮可新增记录`
  ✅ 原项目：`车辆档案` (主标题) + `管理车队车辆基础档案与证照` (副标题)
- ❌ AI 味：`请在此输入您的车牌号码以便快速查询`
  ✅ 原项目：`请输入车牌号` (Placeholder)
- ❌ AI 味：`系统已成功为您删除了该条记录，您可以继续操作`
  ✅ 原项目：`删除成功` (MessagePlugin)

### 2. 详细文案约束
- **主标题**：必须是纯名词或动宾短语，无修饰。
- **副标题**：一句话说明用途，**绝不允许出现“您可以”、“为您”等拟人化词汇**。
- **按钮与动作**：动宾短语，严格控制在 2~4 个字（如 `新建知识库`，`保存`）。空状态动作需极简直接（如 `清除搜索`）。
- **Tooltip (悬浮提示)**：只用于解释非常规名词或严重后果，绝对禁止解释常规按钮。

---

## 后端构建与服务启动规范

- **CGO 环境依赖**：由于引入了 sqlite/duckdb 等依赖，必须在编译环境中配置支持 CGO 的 C 编译器（如 Windows 下必须确保环境变量中已配置 UCRT 版 WinLibs 工具链）。
- **标准编译命令**：
  ```bash
  export CGO_ENABLED=1
  # 若有特定的 CFLAGS（如 sqlite3 头文件路径），请按需配置 CGO_CFLAGS
  go build -o server.exe ./cmd/server

```

* **服务启动约定**：
* 后端：默认端口 `8080`（如：`Start-ScheduledTask -TaskName WeKnoraServer`）。
* 前端：默认端口 `5173`。
* 解析服务 (docreader)：通过 gRPC 占用 `50051`，需在项目虚拟环境独立启动 Python 服务。



---

## 🚗 二开业务特化规范（日常事务与车队模块专属）

> 以下规则是基于历史开发经验总结的硬性约束，在涉及具体业务代码时必须严格遵守，防止历史 Bug 复现。

### 1. 打印/目录生成规则（强制）

1. **所见即所得**：打印/PDF 生成的列必须严格使用当前列表 `visibleColDefs`（用户在字段筛选器里勾选并显示的列），**禁止**在打印函数里另写一份硬编码字段列表、禁止 `.slice(0, N)` 截断、禁止把 `data.*` 列过滤掉再手动补前几列。
2. **列顺序与列表一致**：打印列顺序必须与列表列顺序一致，不得重新排列、不得增删字段。
3. **状态列自动计算**：证照状态/证件状态等由系统根据有效期自动计算的列，打印时同样走 `calcCertStatus` 逻辑，不得依赖模型提取值。
4. **空值安全显示**：未提取到的字段打印为空（`-` 或空白），**不得**从打印输出里隐藏该列；取值必须使用安全可选链（如 `row.data?.[col.key] ?? '-'`），严禁未判空直接访问子属性导致渲染崩溃。
5. **选中行优先**：用户勾选了记录时打印选中行，未勾选时打印当前筛选后的全部行。
6. 复用 `useCatalogPdf.ts**`：目录式打印统一走 `frontend/src/views/dailyAffairs/useCatalogPdf.ts` 的 `generateCatalogPdf`，不要在各页面里重复实现 PDF 渲染。

### 2. 字段配置与筛选器规则（强制）

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
15. **上传弹窗默认类型前缀规范化**：`FleetUploadDialog` 的默认选中值构造必须将 scope 规范化为短前缀（`vehicle-archive → vehicle`, `driver-archive → driver`, `maintain-archive → maintain`），构造方式为 `${normalizedScope}__${defaultType}`，**禁止**硬编码 `'vehicle__'`。否则司机档案弹窗会显示 `vehicle__驾驶证`（在 driver scope 选项里匹配不到，直接露出原始 value）。

### 3. 窄屏自适应规则（强制）

1. **禁止内部滚动兜底**：卡片网格容器**禁止**加 `overflow: auto/scroll` 作为窄屏方案；窄屏时优先调整每行卡片数量和卡片尺寸。
2. **断点自适应**：
* 默认屏宽：`grid-template-columns: repeat(3, 240px)`（或等同宽度）。
* ≤1200px：改为 `repeat(2, 1fr)`，卡片宽度自适应撑满。
* ≤768px：改为 `1fr`，单列堆叠。


3. **卡片尺寸自适应**：卡片用 `1fr` 或 `auto` 宽度，禁止写死 `width: 240px` 在窄屏断点下；用媒体查询切换列数即可。
4. **容器高度自适应**：概览容器高度随内容撑开，禁止写死 `height` 或 `max-height`。
