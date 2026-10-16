# scripts/vm — VM（Debian 12 开发测试机）运维脚本

本目录存放 VM `10.10.10.239`（SSH `-p 5522 root@10.10.10.239`）的部署与运维脚本，
2026-10-16 仓库清理时从项目根目录归档至此。

## 核心脚本

| 脚本 | 用途 |
|------|------|
| `vm_install_systemd.sh` | **安装/重装 systemd 服务**（后端 8080 + vite 5173）。内置前置自愈：二进制 `chmod +x`、补写 `JIEBA_DICT_DIR`、词典齐备性检查、直连误配告警 |
| `vm_run_server.sh` | 迁移期遗留的 nohup 启动方式，**已被 systemd 取代**，仅备查 |
| `vm_build_backend.sh` | 在 VM 上编译 `server_linux.exe` |

## 环境配置

| 文件 | 用途 |
|------|------|
| `vm_cargo_config.toml` | Rust/cargo 配置（anydoc 静态库构建用） |

## 探测 / 诊断

`vm_probe_*.sh`、`vm_check_*.sh`、`vm_verify_*.sh`、`vm_test_connect.sh`、
`vm_gen_key.sh`、`vm_setup_*.sh`、`vm_start_services.sh` 均为一次性排查脚本。

## 数据库连接提醒

VM **无 IPv6 出站**，且内网 DNS 把 supabase 域名劫持到 `198.18.x.x` 代理，
因此 VM 只能用**共享池 Session 模式**：

```bash
DB_HOST=aws-0-ap-northeast-2.pooler.supabase.com
DB_USER=postgres.<PROJECT_REF>   # 后缀即 Supavisor 租户标识，不可省
```

直连（`db.*.supabase.co`）与专用池在 VM 上必失败。详见根目录 `AGENTS.md §5.3`。
