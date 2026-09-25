import { createRouter, createWebHistory } from 'vue-router'
import type { RouteLocationNormalized } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useDeploymentCapabilitiesStore } from '@/stores/deploymentCapabilities'
import { autoSetup, getCurrentUser, userInfoFromApi } from '@/api/auth'
import type { DeploymentCapabilityKey } from '@/config/deploymentCapabilities'
import { MessagePlugin } from 'tdesign-vue-next'
import i18n from '@/i18n'
import { normalizeSettingsSection } from '@/config/settingsRoute'
import { isToolboxSection, toolboxLocation } from '@/config/toolbox'

/** Lite /妗岄潰 WebView 纭埛鏂版椂鍙兘鍙墦寮€ `/`锛岀敤 session 璁颁綇涓婃椤甸潰浠ヤ究鎭㈠ */
const LITE_LAST_PATH_KEY = 'weknora_lite_last_path'

function isLiteEdition(authStore: ReturnType<typeof useAuthStore>) {
  return authStore.isLiteMode || localStorage.getItem('weknora_lite_mode') === 'true'
}

function isLiteSpaDefaultEntry(to: RouteLocationNormalized) {
  return (
    to.path === '/' ||
    to.path === '/platform' ||
    to.path === '/platform/knowledge-bases' ||
    to.name === 'knowledgeBaseList'
  )
}

function isSafeLiteRestoreTarget(path: string) {
  return path.startsWith('/platform/') && !path.startsWith('/platform/organizations')
}

function hasPendingOIDCCallback() {
  if (typeof window === 'undefined') return false
  const hash = window.location.hash || ''
  return hash.includes('oidc_result=') || hash.includes('oidc_error=')
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: "/",
      redirect: "/platform/knowledge-bases",
    },
    {
      path: "/login",
      name: "login",
      component: () => import("../views/auth/Login.vue"),
      meta: { requiresAuth: false, requiresInit: false }
    },
    // Embed chat is a separate entry (embed.html + embed-main.ts), not this SPA.
    {
      path: "/register",
      name: "registerByInvite",
      // Share-link landing page reuses the Login form: the same Vue
      // component renders both modes and detects ?token=xxx on mount
      // to switch into invite-register flow. Avoids a parallel page
      // that would duplicate the OIDC / language-switch / styling
      // surface for one extra field.
      component: () => import("../views/auth/Login.vue"),
      meta: { requiresAuth: false, requiresInit: false }
    },
    {
      path: "/onboarding/workspace",
      name: "workspaceOnboarding",
      component: () => import("../views/auth/WorkspaceOnboarding.vue"),
      meta: { requiresAuth: true, requiresInit: false, requiresTenant: false }
    },
    {
      path: "/join",
      name: "joinOrganization",
      // 閲嶅畾鍚戝埌缁勭粐鍒楄〃椤碉紝骞跺皢 code 鍙傛暟杞崲涓?invite_code
      redirect: (to) => {
        const code = to.query.code as string
        return {
          path: '/platform/organizations',
          query: code ? { invite_code: code } : {}
        }
      },
      meta: { requiresInit: true, requiresAuth: true }
    },
    {
      path: "/knowledgeBase",
      name: "home",
      component: () => import("../views/knowledge/KnowledgeBase.vue"),
      meta: { requiresInit: true, requiresAuth: true }
    },
    {
      path: "/platform",
      name: "Platform",
      redirect: "/platform/knowledge-bases",
      component: () => import("../views/platform/index.vue"),
      meta: { requiresInit: true, requiresAuth: true },
      children: [
        {
          path: "tenant",
          redirect: "/platform/settings"
        },
        {
          path: "settings",
          name: "settings",
          component: () => import("../views/settings/Settings.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "knowledge-bases",
          name: "knowledgeBaseList",
          component: () => import("../views/knowledge/KnowledgeBaseList.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "knowledge-bases/:kbId",
          name: "knowledgeBaseDetail",
          component: () => import("../views/knowledge/KnowledgeBase.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "knowledge-search",
          // 鏃ц矾寰勪繚鐣欎负閲嶅畾鍚戯紝鎵撳紑鍏ㄥ眬鍛戒护闈㈡澘锛堚寴K锛夛紝甯︿笂鍙€夌殑 q 鍙傛暟
          redirect: (to) => {
            const q = to.query.q
            return {
              path: '/platform/knowledge-bases',
              query: typeof q === 'string' ? { cmdk: q } : { cmdk: '' },
            }
          },
        },
        {
          path: "artifacts",
          name: "artifactLibrary",
          component: () => import("../views/artifacts/ArtifactLibrary.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiredCapability: 'settings.sandbox' }
        },
        {
          path: "toolbox/:section?",
          name: "toolbox",
          component: () => import("../views/toolbox/Toolbox.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "agents",
          name: "agentList",
          component: () => import("../views/agent/AgentList.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiredCapability: 'agents' }
        },
        {
          path: "integrations",
          redirect: (to) => {
            const tab = typeof to.query.tab === 'string' ? to.query.tab : undefined
            const incoming = typeof to.query.section === 'string' ? to.query.section : 'integrations'
            const rest = { ...to.query }
            delete rest.tab
            return {
              path: '/platform/settings',
              query: {
                ...rest,
                section: normalizeSettingsSection(incoming, tab),
              },
            }
          },
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "creatChat",
          name: "globalCreatChat",
          component: () => import("../views/creatChat/creatChat.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "knowledge-bases/:kbId/creatChat",
          name: "kbCreatChat",
          component: () => import("../views/creatChat/creatChat.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "chat/:chatid",
          name: "chat",
          component: () => import("../views/chat/index.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "organizations",
          name: "organizationList",
          component: () => import("../views/organization/OrganizationList.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiredCapability: 'organizations' }
        },
        // Compatibility redirects for /platform/system/* URLs. System
        // administration surfaces live as dedicated sections inside the
        // standard Settings modal; keep stable URLs for bookmarks and
        // external links.
        {
          path: "system",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        {
          path: "system/settings",
          name: "systemSettings",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        {
          path: "system/admins",
          name: "systemAdmins",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        {
          path: "system/queues",
          name: "systemQueues",
          redirect: { path: "/platform/settings", query: { section: "runtime-queues" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        // === 日常事务沙盒路由（非侵入式追加） ===
        {
          path: "daily-affairs",
          name: "dailyAffairs",
          component: () => import("../views/dailyAffairs/DailyAffairsHome.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "daily-affairs/invoices",
          name: "invoiceManagement",
          component: () => import("../views/dailyAffairs/InvoiceManagement.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "daily-affairs/contracts",
          name: "contractManagement",
          component: () => import("../views/dailyAffairs/ContractManagement.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "daily-affairs/regulations",
          name: "regulationManagement",
          component: () => import("../views/dailyAffairs/RegulationManagement.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "daily-affairs/award-punish",
          name: "awardPunishManagement",
          component: () => import("../views/dailyAffairs/AwardPunishManagement.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "daily-affairs/utilities",
          name: "utilitiesManagement",
          component: () => import("../views/dailyAffairs/UtilitiesManagement.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "daily-affairs/fleet",
          name: "fleetManagement",
          component: () => import("../views/dailyAffairs/FleetManagement.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        }
      ],
    },
    // Dev-only markdown rendering test page
    ...(import.meta.env.DEV ? [{
      path: '/platform/dev/markdown',
      name: 'markdownTest',
      component: () => import('../views/dev/MarkdownTestPage.vue'),
      meta: { requiresAuth: false, requiresInit: false }
    }] : []),
  ],
});

// 鎸佷箙鍖?auto-setup / login 杩斿洖鐨勮璇佷俊鎭埌 store
function persistLoginResponse(authStore: ReturnType<typeof useAuthStore>, response: any) {
  const activeTenant = response.active_tenant || response.tenant
  if (response.user && response.token) {
    const homeTenantId = response.user.tenant_id ?? activeTenant?.id ?? ''
    authStore.setUser(userInfoFromApi(response.user, homeTenantId))
    authStore.setToken(response.token)
    if (response.refresh_token) {
      authStore.setRefreshToken(response.refresh_token)
    }
    if (activeTenant) {
      authStore.setTenant({
        id: String(activeTenant.id) || '',
        name: activeTenant.name || '',
        owner_id: response.user.id || '',
        created_at: activeTenant.created_at || new Date().toISOString(),
        updated_at: activeTenant.updated_at || new Date().toISOString()
      })
    } else {
      authStore.setTenant(null)
    }
    if (Array.isArray(response.memberships)) {
      authStore.setMemberships(response.memberships)
    }
  }
}

async function hydrateSessionFromToken(authStore: ReturnType<typeof useAuthStore>) {
  const token = localStorage.getItem('weknora_token')
  if (!token) return false

  if (!authStore.token) {
    authStore.setToken(token)
  }

  const storedRefreshToken = localStorage.getItem('weknora_refresh_token')
  if (storedRefreshToken && !authStore.refreshToken) {
    authStore.setRefreshToken(storedRefreshToken)
  }

  try {
    const response = await getCurrentUser()
    const user = response.data?.user
    if (!response.success || !user) {
      return false
    }

    authStore.setUser(userInfoFromApi(user, response.data?.tenant?.id))

    const tenant = response.data?.tenant
    if (tenant) {
      authStore.setTenant({
        id: String(tenant.id) || '',
        name: tenant.name || '',
        owner_id: tenant.owner_id || user.id || '',
        description: tenant.description,
        status: tenant.status,
        business: tenant.business,
        storage_quota: tenant.storage_quota,
        storage_used: tenant.storage_used,
        created_at: tenant.created_at || new Date().toISOString(),
        updated_at: tenant.updated_at || new Date().toISOString(),
      })
    } else {
      authStore.setTenant(null)
    }

    // Refresh memberships on every page load 鈥?same reason as
    // App.vue's syncOIDCUserContext: without this the auth store
    // would only ever see the snapshot from the original /auth/login
    // call, so role changes (and tenant-switch role lookups) would
    // be silently stale until the user logged out and back in.
    const memberships = response.data?.memberships
    if (Array.isArray(memberships)) {
      authStore.setMemberships(memberships)
    }

    const canCreateTenant = response.data?.capabilities?.can_create_tenant
    if (typeof canCreateTenant === 'boolean') {
      authStore.setCanCreateTenant(canCreateTenant)
    }

    authStore.setAutoAcceptInvitation(
      response.data?.capabilities?.auto_accept_invitation === true,
    )

    return true
  } catch {
    return false
  }
}

let autoSetupAttempted = false
let liteDeepLinkRestoreDone = false

// 璺敱瀹堝崼锛氭鏌ヨ璇佺姸鎬佸拰绯荤粺鍒濆鍖栫姸鎬?
router.beforeEach(async (to, from, next) => {
  const authStore = useAuthStore()

  // OIDC 鍥炶烦鐧诲綍缁撴灉渚濊禆 App.vue 鍦ㄦ寕杞藉悗娑堣垂 URL hash銆?
  // 濡傛灉杩欓噷鍏堟寜鈥滄湭鐧诲綍鈥濇嫤鎴埌 /login锛屼細瀵艰嚧鍥炶皟缁撴灉娌℃湁鏈轰細钀界洏銆?
  if (hasPendingOIDCCallback()) {
    next()
    return
  }

  // Preserve bookmarks for tools that have moved out of Settings.
  if (to.path === '/platform/settings' && isToolboxSection(to.query.section)) {
    next({ ...toolboxLocation(to.query.section,
      typeof to.query.sandboxId === 'string' ? to.query.sandboxId : undefined), replace: true })
    return
  }

  // Lite锛氱‖鍒锋柊鍚庤嫢钀藉湪榛樿棣栭〉锛屾仮澶嶆湰娆′細璇濅腑鏈€鍚庤闂殑 /platform 瀛愯矾寰?
  if (!liteDeepLinkRestoreDone) {
    liteDeepLinkRestoreDone = true
    if (isLiteEdition(authStore)) {
      const saved = sessionStorage.getItem(LITE_LAST_PATH_KEY)
      if (saved && isSafeLiteRestoreTarget(saved) && isLiteSpaDefaultEntry(to)) {
        if (saved !== to.fullPath) {
          next(saved)
          return
        }
      }
    }
  }

  // Tenantless onboarding still requires a valid user token even though it
  // deliberately skips the normal tenant/system-initialization gates.
  if (to.path === '/onboarding/workspace') {
    if (!authStore.isLoggedIn) {
      const restored = await hydrateSessionFromToken(authStore)
      if (!restored) {
        next('/login')
        return
      }
    }
    if (authStore.hasValidTenant) {
      next('/platform/knowledge-bases')
    } else {
      next()
    }
    return
  }

  // 濡傛灉璁块棶鐨勬槸鐧诲綍椤甸潰鎴栧垵濮嬪寲椤甸潰锛岀洿鎺ユ斁琛?
  if (to.meta.requiresAuth === false || to.meta.requiresInit === false) {
    // 濡傛灉宸茬櫥褰曠敤鎴疯闂櫥褰曢〉闈紝閲嶅畾鍚戝埌鐭ヨ瘑搴撳垪琛ㄩ〉闈?
    if (to.path === '/login' && authStore.isLoggedIn) {
      next(authStore.hasValidTenant ? '/platform/knowledge-bases' : '/onboarding/workspace')
      return
    }
    next()
    return
  }

  // 妫€鏌ョ敤鎴疯璇佺姸鎬?
  if (to.meta.requiresAuth !== false) {
    if (!authStore.isLoggedIn) {
      const restored = await hydrateSessionFromToken(authStore)
      if (restored) {
        next(
          !authStore.hasValidTenant && to.meta.requiresTenant !== false
            ? '/onboarding/workspace'
            : to.fullPath,
        )
        return
      }

      if (!autoSetupAttempted) {
        autoSetupAttempted = true
        localStorage.removeItem('weknora_auto_setup_failed')
        try {
          const response = await autoSetup()
          if (response.success) {
            persistLoginResponse(authStore, response)
            authStore.setLiteMode(true)
            next(to.fullPath)
            return
          }
        } catch {
          // Auto-setup may be unavailable outside the native Lite shell.
        }
      }
      next('/login')
      return
    }
  }

  if (to.meta.requiresTenant !== false && !authStore.hasValidTenant) {
    next('/onboarding/workspace')
    return
  }

  // 閮ㄧ讲鑳藉姏鍙弿杩扳€滃悗绔槸鍚︽彁渚涜鍔熻兘鈥濓紝涓嶅弽鏄犳湇鍔″仴搴锋垨鏄惁宸查厤缃€?
  // 鎺㈡祴澶辫触鏃?Store 浼?fail-open锛岀湡姝ｇ殑鏉冮檺鍜屽彲鐢ㄦ€т粛鐢卞悗绔帴鍙ｆ牎楠屻€?
  const deploymentCapabilities = useDeploymentCapabilitiesStore()
  await deploymentCapabilities.ensureLoaded()
  const requiredCapability = to.meta.requiredCapability as DeploymentCapabilityKey | undefined
  if (requiredCapability && !deploymentCapabilities.isSupported(requiredCapability)) {
    MessagePlugin.warning(i18n.global.t('settings.capabilityUnavailable'))
    next('/platform/knowledge-bases')
    return
  }

  // SystemAdmin gate 鈥?checked AFTER auth so a non-admin who's logged
  // out gets redirected to /login first (consistent with how the rest
  // of the auth flow works), and only an authenticated non-admin sees
  // the bounce. This is UI-only; the server enforces the real check.
  if (to.meta.requiresSystemAdmin === true) {
    if (!authStore.isSystemAdmin) {
      next('/platform/knowledge-bases')
      return
    }
  }

  next()
})

router.afterEach((to) => {
  if (!isLiteEdition(useAuthStore())) return
  if (to.path === '/login') return
  if (!to.path.startsWith('/platform')) return
  sessionStorage.setItem(LITE_LAST_PATH_KEY, to.fullPath)
})

export default router
