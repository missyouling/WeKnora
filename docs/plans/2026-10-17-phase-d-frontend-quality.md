# Phase D：前端双轨规则面板 + 6 大业务质量可观测性 + 能耗设置交互整改

日期：2026-10-17　分支：`daily-affairs-v4`

## 一、目标与边界

后端 Phase A/B/C 已把 `extract_issues` / `patched_by_regex` / `regex_patches` 写入六个业务的
`custom_metadata`，`ExtractFieldConfig` 也已支持 `regex` / `required`。本轮只做前端消费层，
外加一处**必要的后端补齐**（见 §五）。

三个目标：
1. 规则面板可编辑 `required` / `regex`，且对「自定义扩展中文 key」明确置灰 + 提示；
2. 六个业务详情抽屉 + 提取测试弹窗统一展示质量与溯源（无问题静默、补洞蓝色徽章、问题分级 Alert）；
3. 消除「能耗设置」抽屉套抽屉。

不做：
- 不改提取器行为、不改 Prompt 拼装；
- 不为明细子列（`items[].extra` / `fee_items`）做字段级规则（Phase B/C 既定边界）。

## 二、数据流

```
后端 custom_metadata ──┐
                       ├─→ parseExtractQuality(meta, index?) ─→ ExtractQuality
测试接口 data          ──┘                                        │
                                                                  ├─→ ExtractQualityPanel（徽章 + Alert）
                                                                  └─→ TestResultPanel / 6 抽屉
```

`index` 语义：与后端 `ExtractFieldIssue.Index` 一致（结果数组下标，`-1` = 全局/非行级）。
抽屉只展示**当前记录**的问题，`index < 0` 的全局问题始终展示；列表/弹窗不传 `index`，全量展示。

## 三、文件清单

| 文件 | 动作 | 说明 |
|---|---|---|
| `src/api/fleet.ts` | 改 | `ExtractFieldConfig` 增 `regex?` / `required?` |
| `src/views/dailyAffairs/extractQuality.ts` | 新增 | 纯函数解析层（可单测，零 Vue 依赖） |
| `src/views/dailyAffairs/extractQuality.test.ts` | 新增 | 空值防御 / index 过滤 / severity 归一 / 补洞溯源 |
| `src/views/dailyAffairs/extractFieldMeta.ts` | 新增 | 各 scope 标准契约键集合 + 内置正则预设 |
| `src/views/dailyAffairs/ExtractQualityPanel.vue` | 新增 | 徽章 + 分级 Alert，静默优先 |
| `src/views/dailyAffairs/ExtractRulePanel.vue` | 改 | 必填 Switch 列、正则 Input 列、展开区预设与提示 |
| `src/views/dailyAffairs/TestResultPanel.vue` | 改 | 结果区顶部接入质量面板 |
| `ExtractTestDialog.test.ts` | 改 | 新增质量渲染断言 |
| `ExtractRulePanel.test.ts` | 新增 | 新列、置灰规则、预设断言 |
| `Invoice/Contract/Regulation/AwardPunish/Solar/Utilities Management.vue` | 改 | 抽屉顶部接入质量面板 |
| `UtilitySettingsDrawer.vue` | 改 | 去嵌套：单层抽屉 + 步骤化面板 |
| `UtilitySettingsDrawer.test.ts` | 新增 | 断言模板中仅一个 `t-drawer` |
| `internal/handler/extract_config.go` | 改 | 测试接口返回质量元数据（否则弹窗永远为空） |
| `internal/application/service/extract_generic_validate.go` | 改 | 扁平 map 记录 + camelCase↔snake_case 别名 |

## 四、关键设计决策

| 决策 | 理由 |
|---|---|
| 前端解析层独立成 `extractQuality.ts` 纯模块 | 6 个抽屉 + 弹窗共用；纯函数才能被 `node:test` 直接覆盖，避免只能靠源码字符串断言 |
| 组件静默优先（无问题不渲染任何东西） | 用户明确要求"不增加视觉噪音"；正常文档占多数，徽章常驻会让抽屉变成告警墙 |
| 徽章按**字段**聚合，不按补洞条数 | 一张票补 3 个字段显示 3 个徽章比"正则兜底: 3 处"有用；详情走 hover 弹层 |
| Alert 按 severity 合并为 2 条（error / warn），标题走 `#title`、明细走默认插槽 | TDesign Alert 不支持列表插槽，多条 Alert 每张票一条会撑爆抽屉；合并后仍可定位 `#index field`。注意：tdesign-vue-next 1.19.2 的 Alert **没有 `#description` 插槽**（那是 prop 名），E2E 曾抓出明细整块静默丢失，已加源码回归断言 |
| 自定义 key 的 regex **禁用而非隐藏** | 禁用 + Tooltip 才能让用户理解"为什么这个字段没有兜底"，隐藏会让人误以为功能缺失 |
| 必填/正则在**行内展开区**做完整编辑，表格列只做快速开关与状态 | 表格 6 列在 654px 抽屉内必然挤压；完整编辑放展开区可容纳预设、校验、说明 |
| 能耗设置改「单抽屉 + 左列分组 + 右列编辑」 | 与发票设置（单抽屉 + Tabs）同范式；不引入第二层抽屉，也不需要额外 Dialog 生命周期 |

## 五、后端必要补齐（为什么本轮必须动）

`TestExtractConfig` 走的是 `ExtractFleetDocumentFromContent`（通用 fleet 沙箱），**从未**跑通用校验，
返回体里没有 `extract_issues`。若前端只做防御解析，用户在"测试规则"里永远看不到告警——
Phase D 的验收面直接失效。因此：

1. `GenericExtractRecord` 支持**扁平 map** 视图（`NewGenericRecordFromMap`），写入回写 map；
2. 记录字段查找增加 `camelCase ↔ snake_case` 别名（`signDate` → `sign_date`）。
   否则合同/制度/奖惩的配置键（camelCase）永远匹配不到结构体 json tag（snake_case），
   `Required` 静默失效；
3. 测试接口在校验/补洞后**重新计算 `missing`**（补洞成功的字段不应再报缺失），
   并把 `extract_issues` / `patched_by_regex` / `regex_patches` 一起返回。

## 六、TDD 任务拆分

1. `extractQuality.test.ts`：先红（模块不存在）→ 实现 `parseExtractQuality`。
2. `ExtractRulePanel.test.ts`：先红（无 required/regex 列）→ 改面板。
3. `ExtractTestDialog.test.ts`：加断言（结果区含质量面板、徽章/Alert 文案）→ 改 `TestResultPanel`。
4. `UtilitySettingsDrawer.test.ts`：先红（当前模板有 2 个 `t-drawer`）→ 改单层。
5. Go：`TestExtractGenericValidateMatchesCamelCaseConfigKeys` / `TestExtractGenericValidateFlatMapRecord`
   先红 → 实现 → 跑白名单。

## 七、门禁

- 前端 `npm run type-check`：0 error
- 前端 `npm run lint`：0 error（存量 warning 不计）
- 前端单测：新增 4 个测试文件全绿 + 存量 `ExtractTestDialog.test.ts` 17 条不回归
- Go：`go vet`、CI 白名单（`TestBusiness|TestExtract|TestDailyAffairs|TestGolden`）、
  全量 service+handler 基线对照「零新增失败」
- VM 冒烟：发票/合同抽屉 + 测试弹窗人工可见（截图或日志证据）
