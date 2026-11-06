import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

/**
 * 第 42 轮：默认首页由「知识库」切换为「日常事务」。
 * 只改默认落地/登录成功跳转，不删知识库菜单与路由。
 */
const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
const router = read('../../router/index.ts')
const constant = read('../../config/defaultHome.ts')
const app = read('../../App.vue')
const login = read('../auth/Login.vue')
const onboarding = read('../auth/WorkspaceOnboarding.vue')
const menuStore = read('../../stores/menu.ts')

test('默认首页常量指向日常事务入口页', () => {
  assert.match(constant, /export const DEFAULT_HOME_PATH = '\/platform\/daily-affairs'/)
  assert.doesNotMatch(constant, /daily-affairs\/(energy|utilities|invoices|fleet)/,
    '默认首页必须落在入口页，不得绑到某个子模块')
})

test('默认首页指向已注册的入口路由', () => {
  assert.match(router, /path: "daily-affairs",\s*\r?\n\s*name: "dailyAffairs"/)
})

test('根路径 / 与 /platform 都重定向到默认首页', () => {
  assert.match(router, /import \{ DEFAULT_HOME_PATH \} from '@\/config\/defaultHome'/)
  assert.match(router, /\/\/ 默认首屏：日常事务[\s\S]{0,160}path: "\/",\s*\r?\n\s*redirect: DEFAULT_HOME_PATH/)
  assert.match(router, /name: "Platform",\s*\r?\n\s*redirect: DEFAULT_HOME_PATH/)
  assert.doesNotMatch(router, /redirect: "\/platform\/knowledge-bases"/)
})

test('路由守卫的兜底回弹不再把用户送回知识库列表', () => {
  assert.doesNotMatch(router, /next\('\/platform\/knowledge-bases'\)/)
  assert.ok((router.match(/next\(DEFAULT_HOME_PATH\)/g) || []).length >= 3)
  assert.match(router, /authStore\.hasValidTenant \? DEFAULT_HOME_PATH/)
})

test('登录 / 邀请兑换 / 工作空间创建完成后落地默认首页', () => {
  for (const [name, src] of [['App.vue', app], ['Login.vue', login], ['WorkspaceOnboarding.vue', onboarding]] as const) {
    assert.match(src, /import \{ DEFAULT_HOME_PATH \} from '@\/config\/defaultHome'/, `${name} 必须引用统一常量`)
    assert.match(src, /DEFAULT_HOME_PATH/, `${name} 必须使用默认首页`)
    assert.doesNotMatch(src, /router\.replace\('\/platform\/knowledge-bases'\)/, `${name} 不得再硬编码知识库落地页`)
  }
})

test('知识库菜单入口与路由仍然保留', () => {
  assert.match(menuStore, /titleKey: 'menu\.knowledgeBase'[\s\S]{0,120}path: 'knowledge-bases'/)
  assert.match(router, /path: "knowledge-bases",\s*\n\s*name: "knowledgeBaseList"/)
})
