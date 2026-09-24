# AGENTS.md

## 注意事项

- 每次改动完成后，都必须创建一个对应的 Git commit，以便后续追踪和回滚。
- 每次改动后，都必须编写或更新相关测试，并在交付给用户前，确保所有测试和验证全部通过。
- **前端 UI 改动必须遵守本文档的「前端 UI 设计规范（强制）」章节，优先复用现有组件与设计令牌，禁止随意引入新样式方案。**

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
- 构建产物必须输出到项目根目录 `server.exe`（计划任务 `WeKnoraServer` 执行 `scripts\start_server.bat` 运行的就是它）。

## 服务启动（Windows 计划任务）

- 已有计划任务：`WeKnoraServer`（后端）、`WeKnoraDocreader`（解析服务）、`WeKnoraVite`（前端）。
- 启动后端：`Start-ScheduledTask -TaskName WeKnoraServer`
- 端口约定：后端 8080（注意：`scripts\start_vite.bat` 曾写死 VITE_DEV_PROXY_TARGET=8088 导致 5173 代理 500，已改为 8080）、前端 5173、docreader gRPC 50051。
- docreader 可手动启动（当前 python 解释器版本不符时用项目虚拟环境）：
  ```powershell
  $env:PYTHONPATH = "D:\Downloads\WeKnora-main"
  D:\Downloads\WeKnora-main\docreader\.venv\Scripts\python.exe -m docreader.main
  ```

---

## 前端 UI 设计规范（强制）

> 本节规则适用于所有前端页面、组件、样式改动。目标：**统一视觉风格、最大化组件复用、消除"AI 味"随机样式**。

### 1. 设计令牌（Design Tokens）——单一事实来源

所有颜色、间距、字体、圆角、阴影、动效时长必须使用项目定义的设计令牌，**禁止硬编码**。

- **令牌文件位置**：`frontend/src/assets/theme/theme.css`（基于 TDesign CSS 变量，主色已定制为绿色 `#07c05f`）
- **禁止**在组件中直接写入 `#3b82f6`、`16px`、`8px`、`border-radius: 12px` 等具体值。
- 新增令牌前，必须先在现有令牌中查找是否有可复用的语义化变量。
- 若必须新增令牌，需在 commit message 中说明原因，并同步更新令牌文件。

**核心语义化令牌参考（TDesign 变量，项目已覆盖）：**

| 用途 | 令牌 |
|---|---|
| 主色 | `var(--td-brand-color)`（项目定制绿色 `#07c05f`） |
| 主色浅底 | `var(--td-brand-color-1)` |
| 主色悬停 | `var(--td-brand-color-hover)` |
| 正文文字 | `var(--td-text-color-primary)` |
| 次要文字 | `var(--td-text-color-secondary)` |
| 占位文字 | `var(--td-text-color-placeholder)` |
| 边框 | `var(--td-component-stroke)` |
| 容器背景 | `var(--td-bg-color-container)` |
| 次级容器背景 | `var(--td-bg-color-secondarycontainer)` |
| 页面背景 | `var(--td-bg-color-page)` |
| 圆角小 | `var(--td-radius-small)`（2px） |
| 圆角默认 | `var(--td-radius-default)`（3px） |
| 圆角中 | `var(--td-radius-medium)`（6px） |
| 圆角大 | `var(--td-radius-large)`（9px） |
| 圆角超大 | `var(--td-radius-extraLarge)`（12px） |
| 阴影 1 | `var(--td-shadow-1)` |
| 阴影 2 | `var(--td-shadow-2)` |
| 警告色 | `var(--td-warning-color)` / `var(--td-warning-color-1)` |
| 错误色 | `var(--td-error-color)` / `var(--td-error-color-1)` |
| 成功色 | `var(--td-success-color)` / `var(--td-success-color-1)` |
| 字体族 | `var(--td-font-family)` |
| 正文字号小 | `var(--td-font-size-body-small)`（12px） |
| 正文字号中 | `var(--td-font-size-body-medium)`（14px） |
| 正文字号大 | `var(--td-font-size-body-large)`（16px） |
| 标题字号小 | `var(--td-font-size-title-small)`（14px） |
| 标题字号中 | `var(--td-font-size-title-medium)`（16px） |
| 标题字号大 | `var(--td-font-size-title-large)`（20px） |

### 2. 组件复用——先查找，后创建

- **项目 UI 组件库**：TDesign Vue Next（`tdesign-vue-next` ^1.19.2），图标库 `tdesign-icons-vue-next`。
- 在创建**任何**新组件前，必须先确认 TDesign 是否已有对应组件（Button/Input/Select/Table/Dialog/Drawer/Form/Tabs/Tooltip/Popconfirm/Message/Tag/Badge/Empty/Loading 等）。
- **禁止**修改 `node_modules` 下的 TDesign 组件源码；样式调整应通过传入 `className`、`style`、CSS 变量覆盖或设计令牌实现。
- 如需新变体，优先扩展现有组件的 `variant` / `size` / `theme` 等 props，而不是新建一个平行组件。
- 若确实需要新建基础组件，必须先向用户说明理由并获得确认。

**可复用组件速查：**

| 组件 | 说明 |
|---|---|
| `t-button` | variant: outline/dashed/text; theme: default/primary/danger |
| `t-input` | 默认/error/disabled，支持 autosize |
| `t-select` | 下拉选择，支持 filterable |
| `t-table` | 表格，支持排序/分页/自定义列 |
| `t-dialog` | 模态弹窗，支持 header/body/footer slot |
| `t-drawer` | 抽屉，支持宽度调整 |
| `t-form` / `t-form-item` | 表单 |
| `t-tabs` | 标签页 |
| `t-tooltip` | 文字提示 |
| `t-popconfirm` | 删除/危险操作二次确认 |
| `t-message` / `t-dialog` (MessagePlugin/DialogPlugin) | 轻提示/确认框 |
| `t-tag` | 标签/状态标识 |
| `t-empty` | 空状态 |
| `t-loading` | 加载态 |
| `t-icon` | 图标，统一从 `tdesign-icons-vue-next` 引入 |

> 如需其他组件，请先查阅 TDesign 文档，不要直接创建新的基础组件。

### 3. 禁止项（Negative Constraints）

- **禁止**引入新的 UI 库、图标库、CSS-in-JS 方案（除非用户明确要求）。
- **禁止**使用 Tailwind 默认色板（如 `bg-blue-500`、`text-gray-700`），必须使用 TDesign 设计令牌映射的颜色。
- **禁止**在组件中创建独立的、一次性的 `<style scoped>` 或行内样式方案；所有样式应通过全局令牌和共享组件实现。
- **禁止**使用 Emoji 作为功能图标。
- **禁止**在未检查现有组件的情况下，直接生成新的按钮、输入框、卡片等基础 UI 元素。
- **禁止**随意改变页面级布局间距、字体大小层级；必须沿用项目已有的排版比例。
- **禁止**硬编码颜色值（如 `#e37318`、`#d54941`）；警告/错误状态应使用 `var(--td-warning-color-1)`、`var(--td-error-color-1)` 等 TDesign 令牌。

### 4. 与 `ui-design` 技能协同

- 本环境已安装 `ui-design` 技能（位于用户技能目录 `.user_skills/ui-design/`），其中包含设计方向、组件灵感、验收清单等参考文档。
- 在进行 UI 开发时，请**主动读取并应用** `ui-design` 技能中的设计原则与验收清单。
- 当 `ui-design` 技能的建议与本文档中的设计令牌 / 组件复用规则冲突时，**以本文档规则为准**。
- 每次完成前端 UI 改动后，需对照 `ui-design` 技能中的验收清单进行自检。

### 5. 前端改动验收清单（每次交付前必须自检）

- [ ] 所有颜色、间距、字体、圆角、阴影均来自 TDesign 设计令牌，无硬编码值。
- [ ] 所有基础 UI 元素（按钮、输入框、卡片、弹窗等）均复用 TDesign 组件，未新建平行组件。
- [ ] 未修改 `node_modules` 下的 TDesign 组件源码。
- [ ] 未引入新的 UI 库、图标库或 CSS 方案。
- [ ] 页面在视觉上与项目现有页面风格一致（颜色、间距、圆角、字体层级）。
- [ ] 已对照 `ui-design` 技能的验收清单完成自检。
- [ ] 相关测试已更新并通过。
- [ ] 已创建对应的 Git commit。

### 6. 前端技术栈约定

- 框架：Vue 3（^3.5.34）+ Composition API + `<script setup>`
- 构建工具：Vite（^7.3.5）
- 语言：TypeScript（~6.0.3）
- UI 组件库：TDesign Vue Next（^1.19.2）
- 图标库：tdesign-icons-vue-next（0.4.4）
- 状态管理：Pinia（^3.0.4）
- 路由：Vue Router（^4.5.0）
- 样式方案：TDesign CSS 变量 + 组件内 scoped style（仅限布局/间距，颜色必须用令牌）
- 代码检查：`npm run type-check`（vue-tsc --build）

> 若需偏离以上技术栈，必须先向用户说明并获得确认。

---

## 打印/目录生成规则（强制）

> 适用于所有列表页的"打印""目录打印""打印预览"功能。

1. **所见即所得**：打印/PDF 生成的列必须严格使用当前列表 `visibleColDefs`（用户在字段筛选器里勾选并显示的列），**禁止**在打印函数里另写一份硬编码字段列表、禁止 `.slice(0, N)` 截断、禁止把 `data.*` 列过滤掉再手动补前几列。
2. **列顺序与列表一致**：打印列顺序必须与列表列顺序一致，不得重新排列、不得增删字段。
3. **状态列自动计算**：证照状态/证件状态等由系统根据有效期自动计算的列，打印时同样走 `calcCertStatus` 逻辑，不得依赖模型提取值。
4. **空值显示**：未提取到的字段打印为空（`-` 或空白），**不得**从打印输出里隐藏该列。
5. **选中行优先**：用户勾选了记录时打印选中行，未勾选时打印当前筛选后的全部行。
6. **复用 `useCatalogPdf.ts`**：目录式打印统一走 `frontend/src/views/dailyAffairs/useCatalogPdf.ts` 的 `generateCatalogPdf`，不要在各页面里重复实现 PDF 渲染。

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
10. **列顺序以字段配置 subs 数组为唯一权威**：列表列、字段筛选器、编辑抽屉三处的字段排列顺序，必须按该证照类型在 categories[scope].subs 里的数组顺序输出；base/detail 只决定「默认是否勾选显示」（def 判定用 baseSet），不得用内置 BUILTIN_CERTS.base/detail 的硬编码顺序覆盖用户在字段配置里拖动后的顺序。用户在字段配置里调整 subs 顺序后，列表列、筛选器、编辑抽屉必须立即同步。无 subs 时回退内置顺序兜底；用户新增字段追加末尾。
11. **字段开关「四同步」**：字段配置里某个字段的状态开关关闭后，该字段在「列表表头、字段筛选器、编辑抽屉、打印预览」四处必须同时隐藏；四处字段均派生自同一份 rchiveTypeFields（源头已 .filter(s => s.enabled !== false)），禁止在任一处各自维护一份独立字段列表或绕过该源头。分类开关关闭后，该分类的类型卡片、筛选下拉选项、上传类型选项均不显示（内置未入库分类默认启用）。

12. **「启用表头」开关与字段筛选器勾选双向同步**：字段配置里的「启用表头」开关（后端 `FleetCategorySub.is_default`）= 该字段在字段筛选器里默认勾选/在列表表头显示。用户在字段配置里切换并保存后，列表页必须按最新 `is_default` 重建筛选器勾选（`resetColumns()`），**不得**继续沿用旧的 localStorage 勾选存档。实现：字段配置保存后 `dispatchEvent('fleet-categories-changed')`，列表页 `reloadCategories()` 末尾在 `nextTick` 里 `resetColumns()`。后端 `is_default` 必须随 subs 一起 JSON 透传持久化（`FleetCategorySub` 结构体必须含 `IsDefault bool json:"is_default"`，否则会被序列化丢弃导致开关回读丢失）。

---

## 窄屏自适应规则（强制）

> 适用于车队管理概览卡片、状态卡片、证照类型卡片等网格布局。

1. **禁止内部滚动兜底**：卡片网格容器**禁止**加 `overflow: auto/scroll` 作为窄屏方案；窄屏时优先调整每行卡片数量和卡片尺寸。
2. **断点自适应**：
   - 默认屏宽：`grid-template-columns: repeat(3, 240px)`（或等同宽度）。
   - ≤1200px：改为 `repeat(2, 1fr)`，卡片宽度自适应撑满。
   - ≤768px：改为 `1fr`，单列堆叠。
3. **卡片尺寸自适应**：卡片用 `1fr` 或 `auto` 宽度，禁止写死 `width: 240px` 在窄屏断点下；用媒体查询切换列数即可。
4. **容器高度自适应**：概览容器高度随内容撑开，禁止写死 `height` 或 `max-height`。

<!-- aoci:begin -->
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

- `aoci_rules`：取得当前AOCI版本的会话运行合同。
- `aoci_overview`：建立或恢复本仓库的完整认知。
- `aoci_maintain`：受管理对象达到最终稳定状态后检查认知是否需要维护。
- `aoci_update_entry`：提交与当前证据和源码摘要绑定的完整语义更新批次。
- `aoci_report`：仅当当前布局和工具状态支持时，在证据不足、无法可靠生成语义时登记待办，不猜写。

其他MCP工具、CLI命令、参数和专项流程，以当前工具说明、Guide和 `--help` 返回内容为准，不在本文件中重复完整手册。

本区块只规定仓库接入、认知使用和收尾原则。`aoci_rules` 承载当前会话合同，Guide实时输出承载当前Plan的执行顺序与停点，工具Schema、Spec和Validator承载机器结构与判据；Prompt、Description、README和静态文档不能覆盖这些机器事实。

### 建立、生成和恢复认知

1. 每个新的 Agent Run 开始时，应先判断：

   - 本仓库是否已经存在可用的完整AOCI索引；
   - 当前上下文中是否已有与本仓库根、当前索引版本和当前AOCI服务相匹配，并且模型仍可可靠使用的完整仓库认知。

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

   - Host报告上下文压缩或模型已知系统全貌丢失时，执行上述强制 `context_compaction` 重载规则；AOCI不能自行推断Host事件；
   - 进入真正的主要阶段时声明 `phase_transition`，不得把函数、测试运行或小步骤当作阶段；
   - 在有用的稳定检查点通过 `check_only=true` 取得机器语义计数；
   - 除已知压缩的强制重载外，由Agent判断当前任务是否需要再次显式获取指定scope或完整Overview；
   - 在维护和对齐完成前，保留AOCI报告的Dirty或Stale可靠性状态。

### 任务收尾与认知维护

5. 纯只读问答、分析、版本核验，或没有产生受AOCI管理对象变化的任务，不需要调用维护工具。当前AOCI版本是任意`aoci_overview` check_only或`aoci_maintain`响应里的`cognition_receipt.mcp_service_version`；二进制路径是项目`.mcp.json`里的`command`，CLI不必在PATH上。

6. 发生受AOCI管理对象变化时，待其达到本次任务的最终稳定状态后，只调用一次 `aoci_maintain`。不要在每次中间修改后逐文件维护。

7. 若维护结果返回真实语义候选，Host 模型必须基于每个候选绑定的对象和必要证据，独立创作完整标签与F/R/A/S更新。通过 `aoci_update_entry` 一次提交当前机器签发批次的完整候选集合，同时原样保留每项 `source_sha256`、`candidate_id` 与对应domain批次身份。`max_entries`只限制单次请求和原子事务，不限制logical plan、Whole-Index或Managed Scope。`remaining`非零时，在当前批次成功Apply后重新调用Maintain并从新preimage继续；绝不能为满足transport上限缩减Index覆盖或自行截取返回批次。

   没有足够证据且当前布局支持 `aoci_report` 时，使用它而不猜测、套用模板或为消除待办而生成缺乏证据的认知。

8. 必须遵守工具返回的结构化状态和安全边界：

   - `repair_required`：只修复明确命中的候选，再重新提交当前机器签发的完整批次；
   - `stopped`：结束当前写入尝试并检查 `failed_step`、错误、正式写入证据与Recovery。auto模式下，已证明零写入则记录closure并重新Plan；完整Intent和可证明postimage则Resume；策略要求Rollback且preimage可证明则精确恢复后重新Plan。只有证据不足、第三方正式字节冲突、需要审批或外部动作，或命中其他真实安全边界时，才停止整个用户任务；
   - 冲突、审批、人工裁决、权限和安全信号不得忽略；
   - 已经对齐后不得重复维护或重复写入；`refresh_ready_for_overview` 是checkpoint事实，由Agent决定是否为下一阶段请求普通完整Overview。

   维护完成后如果又修改了任何受管理对象，之前的维护结果失效，应在新的最终稳定状态重新完成收尾。

9. 用户只限制业务文件范围，但没有明确禁止仓库托管资产时，AOCI托管资产可以在收尾阶段为保持认知一致而更新，并应在审计和提交中与业务文件区分。

   用户明确禁止修改 `aoci.txt`、`.aoci`、元数据或任何额外文件时，以用户限制为准，不得写入，并如实报告剩余不一致。

### 专项流程

初始化、完整索引生成、Header生成、Entries生成、数据库结构索引、Curation、人工评审和故障恢复，只按当前AOCI Guide或工具在对应阶段返回的指令、命令和安全停点执行。

不预加载、不猜测，也不自行重建这些专项流程。平台调用方式、请求格式、批次上限、审批规则、索引格式细节和恢复步骤由对应Guide、工具说明、模型Prompt和CLI帮助按需提供。
<!-- aoci:end -->

13. **分类名称、顺序、别名全局六同步**：用户在「字段配置」里对证照分类的改名、拖拽排序、启用/禁用，必须同时同步到以下六处，任何一处硬编码或用本地兜底数据都会导致闪烁/不一致：
    - ① 设置抽屉内分类列表（权威源，`categories[scope]` 数组顺序即排序）；
    - ② 主页面概览卡片分组标题（取 `fleet_group_aliases.name`，key=`${scope}:${group_key}`，builtin group_key=`company/driver/maintain`）；
    - ③ 主页面概览类型卡片顺序（按 `categories[scope]` 的 name 顺序 sort，禁止按 overviewStats.by_doc_type 的返回顺序渲染）；
    - ④ 工具栏证照类型筛选下拉；
    - ⑤ 提取规则面板证照类型下拉；
    - ⑥ 上传弹窗证照类型下拉。
    - **反闪烁门控**：设置抽屉首次打开时，`loadCategories()`、`loadCustomGroups()`、`loadGroupAliases()` 三个异步必须 `await Promise.all` 全部完成后才 `ready=true` 渲染内容，禁止先用硬编码 GROUPS label + 本地 BUILTIN 字段数渲染再异步跳变。
    - **import 必须显式**：新用到的 API（如 `listFleetGroupAliases`）必须在文件头 import 行里显式列出，调用时被 catch 吞掉会静默失效。

