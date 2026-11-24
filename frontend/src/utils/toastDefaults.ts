// 全局消息提示（Toast）统一位置与时长。
//
// 问题：TDesign 的 MessagePlugin 默认 `placement: 'top'`，提示从页面顶部中央弹出 ——
// 正好压住人事/车队等页面的 KPI 卡片与列表工具栏（真机：一条「已保存」把第一张
// 卡片和「导入 / 生成账单」按钮糊住）。
//
// 口径（第 69 轮）：
//   · 统一落到**右下角**，与分页/浮动栏不重叠，绝不遮挡顶部卡片；
//   · 单条最宽 400px（样式见 assets/theme/toast.less），多条纵向堆叠；
//   · 成功 2.5s 自动消失；警告 / 错误 4s 且可手动关闭（留足阅读时间）。
//
// 关键坑：TDesign 的 `MessagePlugin.success(params, duration, context)` 第二个参数是
// **duration 数字**，不是 options。placement / offset / closeBtn 必须塞进第一个 params
// 对象里；旧写法把 options 传给第二个参数，会得到 `setTimeout(fn, {object})` ——
// 消息瞬间消失（NaN 延时），且 placement 完全没生效。
import type { App, VNode } from 'vue'
import { isVNode } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'

/** 右下角 + 向内收 20px（offset 是单条消息相对容器的 left/top 偏移）。 */
const PLACEMENT = 'bottom-right' as const
const OFFSET: [number, number] = [-20, -20]
/** 统一最大宽度的钩子（具体样式在 toast.less）。 */
const CLASS_NAME = 'app-toast'

const DURATION: Record<string, number> = {
  success: 2500,
  info: 3000,
  warning: 4000,
  error: 4000,
}

type RawParams = string | VNode | Record<string, unknown>

/** TDesign 原生方法签名：warning(params, duration, context)。 */
type RawFn = (params: RawParams, duration?: number, context?: unknown) => unknown

/** 业务侧旧写法可能传 (msg, 3000) 或直接把 options 放第二参。 */
type CallerOptions = number | Record<string, unknown> | undefined

function withDefaults(kind: 'success' | 'info' | 'warning' | 'error', fn: RawFn) {
  return (message: RawParams, second?: CallerOptions) => {
    let extra: Record<string, unknown> = {}
    let duration = DURATION[kind]
    if (typeof second === 'number') {
      duration = second
    } else if (second && typeof second === 'object') {
      extra = second
      if (typeof extra.duration === 'number') duration = extra.duration
    }
    // 字符串 / VNode 走 content；对象形式（{content, onClose…}）逐字段合并，保留调用方语义
    const base: Record<string, unknown> = typeof message === 'object' && !isVNode(message)
      ? { ...(message as Record<string, unknown>) }
      : { content: message }
    return fn({
      ...base,
      ...extra,
      placement: (extra.placement as string) || PLACEMENT,
      offset: (extra.offset as [number, number]) || OFFSET,
      className: [CLASS_NAME, extra.className].filter(Boolean).join(' '),
      duration,
      // 警告/错误允许手动关闭：用户可能马上要继续操作，不想等 4 秒
      closeBtn: extra.closeBtn ?? (kind === 'warning' || kind === 'error'),
    }, undefined, undefined)
  }
}

let installed = false

/**
 * 幂等安装：重复调用（HMR / 多入口）不会叠加包装层。
 *
 * 传 app 时同时把 `$message` 指向包装后的实例 —— TDesign 在 `app.use(TDesign)` 里
 * 注册的 `$message` 是插件原对象，不重指的话模板里 `$message.warning(...)` 仍弹顶部。
 */
export function installToastDefaults(app?: App): void {
  if (!installed) {
    installed = true
    const target = MessagePlugin as unknown as Record<string, RawFn>
    for (const kind of ['success', 'info', 'warning', 'error'] as const) {
      const original = target[kind]
      if (typeof original !== 'function') continue
      target[kind] = withDefaults(kind, original.bind(MessagePlugin) as RawFn)
    }
  }
  if (app) app.config.globalProperties.$message = MessagePlugin
}
