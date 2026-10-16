import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import { defineConfig, vueTsConfigs } from '@vue/eslint-config-typescript'
import globals from 'globals'

// 用 @vue/eslint-config-typescript 的 defineConfig 包裹：
// 它负责把 tseslint 配置的文件匹配扩展到 .vue、注入 vue-eslint-parser，
// 并按类型检查能力重排配置顺序。
export default defineConfig(
  // ---- 忽略目录 ----
  {
    ignores: [
      'dist/**',
      'node_modules/**',
      '*.d.ts',
      'src/wailsjs/**', // Wails 自动生成产物
      'scripts/**',
    ],
  },

  // ---- 基础 JS 规则 ----
  js.configs.recommended,

  // ---- Vue 必需规则 ----
  // 用 flat/essential 而非 flat/recommended：后者含大量格式类规则
  // （属性排序、缩进等），存量 254 个 .vue 会被刷屏，丧失信号价值。
  ...pluginVue.configs['flat/essential'],

  // ---- TypeScript 推荐集（非类型检查版，保证速度） ----
  ...vueTsConfigs.recommended,

  // ---- 全局变量 ----
  {
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
        ...globals.es2022,
      },
    },
  },

  // ---- 项目定制规则 ----
  {
    rules: {
      // ===== error：真实缺陷，一旦触发必须修 =====

      // JS 基础
      'no-dupe-keys': 'error',
      'no-dupe-args': 'error',
      'no-unreachable': 'error',
      'no-func-assign': 'error',
      'no-self-assign': 'error',
      'no-self-compare': 'error',
      'no-empty': ['error', { allowEmptyCatch: true }],
      'no-global-assign': 'error',
      'no-debugger': 'error',

      // Vue 模板与 SFC 正确性
      'vue/no-dupe-keys': 'error',
      'vue/no-duplicate-attributes': 'error',
      'vue/no-template-key': 'error',
      'vue/no-textarea-mustache': 'error',
      'vue/valid-template-root': 'error',
      'vue/valid-v-for': 'error',
      // <script> 只允许显式 'ts'，或省略 lang（默认 js）。
      // 说明：本规则会把「默认语言」从允许集合中剔除（script 的默认语言就是 js），
      // 因此无法把 'js' 列进 lang 数组，写成 lang="js" 反而会被判为多余。
      // 仓库存量有若干刻意保持 plain JS 的 SFC（chat 目录下几个组件），
      // 故保留 allowNoLang；待它们迁移为 TS 后可去掉该开关，强制所有 SFC 声明 lang。
      // 注意：options 按标签名顶层分组，不要再套一层 block；未列出的块不做检查。
      'vue/block-lang': ['error', { script: { lang: ['ts'], allowNoLang: true } }],
      'vue/valid-v-model': 'error',
      'vue/valid-v-once': 'error',
      'vue/valid-v-show': 'error',
      'vue/valid-v-text': 'error',
      'vue/no-use-v-if-with-v-for': 'error',
      'vue/require-v-for-key': 'error',
      'vue/no-unused-components': 'error',
      'vue/no-unused-vars': 'error',
      // NOTE: no-mutating-props 降级为 warn（见下方 warn 区的说明）。
      'vue/return-in-computed-property': 'error',
      'vue/no-side-effects-in-computed-properties': 'error',
      'vue/no-reserved-component-names': 'error',
      'vue/no-shared-component-data': 'error',

      // TypeScript：未使用变量既是噪音也是真问题，用 error + 下划线豁免
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
          ignoreRestSiblings: true,
        },
      ],
      '@typescript-eslint/no-duplicate-enum-values': 'error',
      '@typescript-eslint/no-extra-non-null-assertion': 'error',
      '@typescript-eslint/no-non-null-asserted-optional-chain': 'error',
      '@typescript-eslint/triple-slash-reference': 'error',
      '@typescript-eslint/no-this-alias': 'error',

      // ===== warn：风格与严格度，存量多，分批治理 =====
      // 分级：error / warn 保留（线上诊断有实际价值），只禁止 log / debug 这类调试残留。
      // 存量 console.error 213 处、console.warn 33 处均属前者，不要为了清零而删掉它们。
      'no-console': ['warn', { allow: ['warn', 'error', 'info'] }],
      'no-var': 'warn',
      'prefer-const': 'warn',
      'eqeqeq': ['warn', 'smart'],
      'no-extra-semi': 'warn',
      'no-useless-escape': 'warn',
      'no-prototype-builtins': 'warn',
      'no-control-regex': 'warn',
      'vue/no-v-html': 'warn',
      // 降级为 warn 的原因：本项目存在「父组件把共享 reactive 对象整块传给子组件」
      // 的既定模式（如 doc-content.vue 的 details，父组件 KnowledgeBase.vue:1845
      // 自身也在直接写该对象）。子组件写 props.details.* 是有意为之——父组件的
      // summaryStateChange 只回写 cardList、不回写 details，去掉这处写入会让
      // 抽屉 UI 不刷新，属于功能回归。彻底改造需连同父子组件重构为 emit/v-model，
      // 已登记为 P2 架构项，此处先作为提示保留。
      'vue/no-mutating-props': 'warn',
      'vue/this-in-template': 'warn',
      'vue/component-definition-name-casing': 'warn',
      'vue/match-component-file-name': 'warn',
      'vue/no-unused-properties': ['warn', { groups: ['props', 'data', 'computed'] }],
      'vue/no-unused-refs': 'warn',
      'vue/require-prop-types': 'warn',
      'vue/require-explicit-emits': 'warn',
      'vue/v-on-event-hyphenation': 'warn',
      'vue/attribute-hyphenation': 'warn',
      'vue/no-static-inline-styles': 'warn',
      'vue/prefer-separate-static-class': 'warn',
      'vue/no-potential-component-option-typo': 'warn',
      'vue/no-reserved-keys': 'warn',
      'vue/require-render-return': 'warn',

      // ===== off：存量过大或与项目现状冲突，暂不开启 =====
      // any 用量 2363 处，先关；AGENTS.md 已明令「严禁滥用 any」，
      // 待分批治理后再从 warn 起步。
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-non-null-assertion': 'off',
      '@typescript-eslint/ban-ts-comment': 'off',
      '@typescript-eslint/no-empty-function': 'off',
      '@typescript-eslint/no-empty-object-type': 'off',
      '@typescript-eslint/no-unsafe-function-type': 'off',
      'vue/attributes-order': 'off',
      'vue/require-default-prop': 'off',
      'vue/multi-word-component-names': 'off',
      'vue/no-v-text-v-html-on-component': 'off',
      'vue/singleline-html-element-content-newline': 'off',
      'vue/max-attributes-per-line': 'off',
      'vue/html-closing-bracket-newline': 'off',
      'vue/html-indent': 'off',
      'vue/html-self-closing': 'off',
      'vue/first-attribute-linebreak': 'off',
      'vue/block-order': 'off',
      'vue/block-tag-newline': 'off',
    },
  },

  // ---- 测试与配置文件放宽 ----
  {
    files: [
      '**/*.test.ts',
      '**/*.test.mjs',
      '**/*.spec.ts',
      '**/tests/**',
      '**/*.config.ts',
      '**/*.config.js',
      'vite.config.ts',
    ],
    rules: {
      'no-console': 'off',
      '@typescript-eslint/no-unused-vars': 'off',
    },
  },

  // ---- CLI 工具脚本：console.log 就是它们的输出方式 ----
  // 这些由 npm scripts 直接执行（scan-i18n-gaps / regenerate-i18n-locales），
  // 打印到 stdout 是功能本身，不能当调试残留删掉（曾误删并恢复过一次）。
  {
    files: [
      'src/i18n/localeGapScan.ts',
      'src/i18n/regeneratePrunedLocales.ts',
      'scripts/**',
    ],
    rules: {
      'no-console': 'off',
    },
  },
)
