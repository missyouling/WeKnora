---
name: upstream-sync-merge
description: 把上游 WeKnora 的 origin/main 同步并合入二开分支 daily-affairs-v4 的可复用流程。当用户要求「同步上游 / 合并 main / merge origin/main / upstream sync / 上游同步后适配」，或需要在上游同步后做冲突消解、代码边界核查与门禁验证时使用。覆盖分支模型、触发条件、输入输出、合并规则、冲突处理原则、保留范围与覆盖边界、验证标准、禁止事项。
agent_created: true
---

# 上游同步后合并（Upstream Sync Merge）

## 目的与适用场景

用于「上游项目发布新版本 → 合并进二开分支」这一固定场景。目标：上游核心代码与 `origin/main` 保持 100% 一致；二开能力以新增独立文件与最小挂载点存活；整个合并可追溯、可回滚、可复验。

触发条件（满足任一即启用本流程）：
- `origin/main` 出现新提交，用户要求同步上游、合并 main、执行 upstream sync。
- 合并产生冲突，需要按统一原则消解而不是临时发挥。
- 合并完成后需要核查代码边界、跑门禁、补齐开发机环境差异。

不适用场景：日常功能开发；仅改二开文件的提交；把二开分支回写 `main`；单纯更新文档或基线数字。这些场景不执行合并流程。

## 输入与输出

| 类别 | 内容 |
|---|---|
| 输入 | ① `origin/main`（fetch 后的上游镜像）② 当前二开分支 `daily-affairs-v4` ③ `scripts/upstream-touch-check.sh` 输出的「本分支改过的上游文件 ∩ 上游新改动文件」交集 ④ 二开允许改动目录白名单与最小挂载点清单（见 AGENTS.md §5.9 / §5.11）⑤ 可执行门禁的 VM 开发环境 |
| 输出 | ① 一个 `--no-ff` 合并提交，只含合并消解相关改动 ② 上游核心与 `origin/main` 零差异、二开挂载点带 `[upstream-touch]` 标记 ③ 祖先关系与 diff 边界证据 ④ 全量门禁与真机验收结果 ⑤ 更新后的规范基线说明（不固化一次性快照） |

## 前置条件

- 工作区干净或改动已提交；确认当前分支是二开分支而非 `main`。
- 合并前创建可回滚备份分支（如 `backup-v4-before-r<N>`）并回显短 hash。
- 合并提交内不得夹带功能开发、重构、格式化等无关改动。

## 合并规则（SOP）

1. **侦察**：执行 `git fetch origin main`；用 `git merge-base` 与 `git rev-list --left-right --count` 评估两侧提交数与文件数，判断改动规模；列出上游新增与删除的文件。
2. **交集预判**：运行 `scripts/upstream-touch-check.sh`，得到必须人工确认的上游文件交集；交集为空通常可安全合入。
3. **备份**：创建备份分支或 tag。
4. **合并**：执行 `git merge --no-ff origin/main -m "merge: 同步上游 origin/main 到 <迭代> 基线"`，保留合并提交以便回滚与审计。
5. **冲突枚举**：用 `git diff --name-only --diff-filter=U` 列出全部冲突文件；逐文件统计双方改动量与冲突块数，按下方原则分类消解。
6. **消解**：同类批量处理，但每类处理后立刻核对残留冲突标记与关键片段（结构体字段、函数签名、路由注册、字段声明）。
7. **提交**：执行 `git add -A` 与 `git commit --no-edit`，保留 merge message。
8. **边界验证**：核查祖先关系与 `git diff origin/main...HEAD`（见「验证标准」）。
9. **同步与门禁**：把合并结果同步到 VM，执行编译/静态/单测/构建门禁；处理环境差异（行尾、迁移、服务重启）。
10. **真机验收**：涉及前端或接口行为时跑 Playwright 逐项断言与 console error 检查；全部通过后按项目提交规范记账。

## 冲突处理原则

按「文件归属」三分类，禁止凭手感二选一：

| 类别 | 判据 | 处理 |
|---|---|---|
| A 纯上游文件 | 二开没有任何必要改动，仅因历史重构产生冲突 | 整体取 `origin/main`（`git checkout --theirs`），恢复与上游 100% 一致 |
| B 上游核心 + 二开最小挂载点 | 上游主体 + 少量二开调用点/字段/保护逻辑 | 取上游主体，再以最小 diff 补回挂载点，逐处标注 `[upstream-touch]`；不把二开业务逻辑写进上游实现 |
| C 二开特有文件 | 只存在于二开分支的新增文件 | 全量保留；若与上游同名，把二开实现迁到独立新文件，上游文件回到 main 版本 |

保持上游主体不变的兼容手法：
- **签名/类型变化**：用可变参数、类型别名、兼容解析函数吸收新旧调用形态，避免修改上游调用方与上游测试。
- **上游组件能力扩展**：恢复上游版本后，用 prop + computed 的最小补丁接入二开能力；禁止重写模板或搬回二开旧实现。
- **上游自带静态检查噪音**：用 `eslint.config.js` 的文件级 override 把存量噪音降级为 warn，不改上游源码、不关闭真实缺陷规则。
- **上游已删除文件**：同步删除，不留孤儿文件。
- **行尾**：合并/同步后统一 LF。上游源码扫描类测试按 `\n` 断言，CRLF 会造成大面积假失败；开发机上的备份目录若被测试框架自动发现，先移出工作区。
- **迁移与环境**：上游新增表/列导致接口 5xx 时，幂等补 DDL 或补列，属于环境修复；不改迁移版本号、不改上游迁移文件。

## 保留范围与覆盖边界

| 边界 | 范围 | 合并时行为 |
|---|---|---|
| 覆盖（上游可取 main） | docreader 与解析管线、核心 knowledge 管线、通用 `internal/types/` 与 models、migrations、上游前端页面/组件/工具、上游测试、根配置 | 与 `origin/main` 完全一致；不得残留二开业务 diff |
| 保留（二开必须存活） | `frontend/src/views/dailyAffairs/`、`frontend/src/components/business/`、`internal/service/knowledge_*`、`internal/handler/` 下的二开文件，以及所有新增独立文件 | 全量保留，合并不得覆盖或删除 |
| 最小挂载点（B 类） | 路由注册与参数兼容、静态前端托管无条件注册、类型结构字段补充、鉴权 5xx 与网络抖动保护、文档预览页码定位、连接池/菜单高亮等基础设施开关 | 仅在冲突时以最小 diff 补回，并标 `[upstream-touch]` |

**边界判据**：对任一上游核心路径执行 `git diff origin/main...HEAD -- <路径>`，结果只允许出现调用点、字段、开关级的最小差异；出现业务分支、业务字段组装或 UI 重写即判定越界，必须抽到独立文件后重做。

## 验证标准（全部必须通过）

1. `git merge-base --is-ancestor origin/main HEAD` 返回 0。
2. `git diff origin/main...HEAD` 的改动只落在「保留范围 + 最小挂载点」；上游核心零业务 diff。
3. 工作区无残留冲突标记，上游已删除文件已删除。
4. Go 编译与静态门禁：`go vet ./...`、`go build -tags anydoc ... ./cmd/server`。
5. 后端单测：按 AGENTS.md §8.5 的**逐包白名单 + `-run` 正则**执行，禁止 `./...` 全跑。
6. 前端门禁：`npm run type-check`、`npm run lint`（0 error）、`npm test`（全绿）、`npm run build`。
7. VM 环境门禁：LF 行尾、上游新增表/列已幂等补齐、服务重启后 `/health` 正常、无 5xx 风暴。
8. 真机验收：涉及 UI/接口时 Playwright 逐项断言且 console error 为 0。
9. 证据留存：门禁输出、边界核查结果、真机断言计数写入交付报告；合并与后续适配分别记账。

## 禁止事项

- 禁止把二开提交写进 `main`，禁止把 `daily-affairs-v4` 回写 `origin/main`。
- 禁止 `git merge -X ours` / `--strategy=ours` 整体吞掉上游，禁止 `git checkout --ours` 覆盖上游核心。
- 禁止为消解冲突而重写、重构、格式化上游实现；禁止在上游核心文件里新增业务逻辑。
- 禁止 `git push --force`、跳过 hook、跳过门禁提交合并结果。
- 禁止在合并提交里夹带无关功能改动或顺手「优化」。
- 禁止修改 `schema_migrations` 版本号或上游迁移文件来「修复」迁移错位。
- 禁止在 AGENTS.md 或本技能里固化一次性 commit hash、冲突数、文件数作为规则。
- 禁止删除 `.workbuddy` 等 Agent 数据目录，或批量删除未确认的上游文件。

## 关联规范与资源

- AGENTS.md §5.9：分支模型与「无侵入式二开」原则；§5.11：本项目合并约束的权威条目（冲突时以 §5.11 为准）。
- AGENTS.md §8.5 / §8.5.4：Go 测试白名单与 ESLint 规则；§1：交付前验证与提交规范。
- AGENTS.md 按项目约定仅本地/VM 维护、不进远端仓库（见 `.gitignore`）；新环境若没有该文件，直接以本技能规则为准，不必等待 §5.11。
- `scripts/upstream-touch-check.sh`：交集预判脚本，每次合并前先运行。
- `git diff origin/main...HEAD`：边界核查的唯一判据。
