# Supabase MCP 连接配置（PAT 方案）

> 2026-10-16 定稿。MCP 服务现由 **Supabase Personal Access Token（PAT）** 认证，
> 不再使用 OAuth 授权码流程。

## 为什么从 OAuth 换成 PAT

| | OAuth（旧） | PAT（现） |
|---|---|---|
| 有效期 | access_token **24 小时** | 不过期（除非主动轮换） |
| 维护成本 | 需定时跑刷新脚本 | 零维护 |
| 授权方式 | 浏览器授权码 + PKCE + 本地回调服务器 | 直接写 `Authorization` 头 |
| 失败模式 | 过期后静默 401，工具全部失效 | 仅轮换后失效 |

WeKnora 是自用工具，PAT 的不过期特性直接消除了 24h 刷新这一运维负担。

## 配置位置与格式

`C:\Users\koujiang\.workbuddy\mcp.json`（用户级，**不在仓库内**）：

```json
{
  "mcpServers": {
    "supabase": {
      "type": "http",
      "url": "https://mcp.supabase.com/mcp?project_ref=<PROJECT_REF>&features=docs%2Caccount%2Cdatabase%2Cdebugging%2Cdevelopment%2Cfunctions%2Cbranching",
      "headers": { "Authorization": "Bearer <PAT>" }
    }
  }
}
```

- PAT 来源：`supabase.txt`（仓库根目录，**已被 .gitignore 忽略**）
- 获取方式：Supabase Dashboard → Account → Access Tokens

## 凭据存放提醒

`supabase.txt` **同时包含**：
- 数据库连接串（明文密码）
- Personal Access Token

该文件已被 `.gitignore` 忽略，但**不要分享、不要提交、不要写进任何文档或日志**。
轮换 PAT 后需同步更新 `mcp.json` 的 `headers.Authorization`。

## 可用工具（20 个）

- **数据库**：`execute_sql`、`apply_migration`、`list_tables`、`list_extensions`、`list_migrations`
- **项目**：`get_project_url`、`get_publishable_keys`、`get_advisors`、`generate_typescript_types`
- **日志**：`search_docs`、`query_logs`
- **Edge Functions**：`list_edge_functions`、`get_edge_function`、`deploy_edge_function`
- **开发分支**：`create_branch`、`list_branches`、`delete_branch`、`merge_branch`、`reset_branch`、`rebase_branch`

## 脚本

| 脚本 | 用途 |
|---|---|
| `verify_pat.py` | 从 `supabase.txt` 提取 PAT，验证 Management API 权限与项目列表 |
| `setup_pat.py` | 把 PAT 写入 `mcp.json`，并跑 MCP initialize + tools/list 握手 |
| `functional_test.py` | 完整握手后调 `execute_sql` / `get_project_url` 做端到端验证 |

三个脚本均在**运行时**从 `supabase.txt` 读取凭据，无硬编码密钥，可安全归档。

## MCP Streamable HTTP 踩坑

1. **必须带浏览器 User-Agent** —— Python urllib 默认 UA 被 Cloudflare 拦截，报
   `Error 1010: browser_signature_banned`。
2. **`initialize` 响应头里的 `mcp-session-id` 必须回传** —— 后续 `tools/list` /
   `tools/call` 都要带，否则报 `Mcp-Session-Id header is required`。
3. **JSON-RPC 请求必须带 `id`** —— 漏了 `id` 会被当作 notification，服务端返回
   `202 Accepted` + 空 body，看起来像"成功"实则没有结果。
4. **响应可能是 SSE** —— 需解析 `data:` 行而非直接 `json.loads`。

## 历史：OAuth 方案（已废弃）

2026-10-16 曾用完整 OAuth 2.1 + PKCE 流程跑通：动态客户端注册
（`POST https://api.supabase.com/platform/oauth/apps/register`）、本地回调服务器
（`127.0.0.1:8977/callback`）、state 防 CSRF、授权码换令牌。

该方案的相关脚本**含硬编码 client_secret 与令牌，已全部删除，未归档**。
Supabase 侧的 OAuth 应用仍存在（redirect_uri 为 `http://localhost:8977/callback`），
如需恢复 OAuth 需重新注册。
