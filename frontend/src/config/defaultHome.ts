/**
 * 网站默认落地页（第 42 轮）。
 *
 * 需求：打开根路径 / 或 /platform、以及登录/初始化成功后，首屏进入「日常事务」。
 * 知识库菜单入口（stores/menu.ts）与 /platform/knowledge-bases 路由均保留，
 * 点击菜单仍可正常访问知识库；这里只收敛"默认落地/兜底回弹"的目标地址。
 *
 * 用常量收口，避免默认首页字符串散落在 router 守卫、App、Login、Onboarding 多处。
 */
export const DEFAULT_HOME_PATH = '/platform/daily-affairs/energy'
