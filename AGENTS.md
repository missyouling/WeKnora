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
