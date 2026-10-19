package container

import "testing"

// fakeEnv 用 map 模拟 os.Getenv，便于表驱动测试。
func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// TestDBPoolSettingsDefaults 锁定默认值必须**低于**上游 Supabase session
// pooler 的 pool_size(15)。
//
// 事故背景：database/sql 的 MaxOpenConns 默认是 0（无限），而本项目 PostgreSQL
// 分支历史上从未调用 SetMaxOpenConns。提取高峰时应用并发抢连接直接超过上游
// 15 条上限，pooler 回 FATAL(EMAXCONNSESSION)，鉴权 token 查询失败被映射成
// 503/401，前端整页报错。默认值必须留出余量给 psql / 迁移 / 其它客户端。
func TestDBPoolSettingsDefaults(t *testing.T) {
	maxOpen, maxIdle := dbPoolSettings(fakeEnv(nil))
	if maxOpen <= 0 {
		t.Fatalf("maxOpen must be capped above zero, got %d", maxOpen)
	}
	if maxOpen >= 15 {
		t.Fatalf("maxOpen=%d must stay below upstream pool_size 15", maxOpen)
	}
	if maxIdle > maxOpen {
		t.Fatalf("maxIdle=%d must not exceed maxOpen=%d", maxIdle, maxOpen)
	}
}

// TestDBPoolSettingsEnvOverride 允许运维不改代码调整上限。
func TestDBPoolSettingsEnvOverride(t *testing.T) {
	maxOpen, maxIdle := dbPoolSettings(fakeEnv(map[string]string{
		"DB_MAX_OPEN_CONNS": "12",
		"DB_MAX_IDLE_CONNS": "3",
	}))
	if maxOpen != 12 || maxIdle != 3 {
		t.Fatalf("want 12/3, got %d/%d", maxOpen, maxIdle)
	}
}

// TestDBPoolSettingsIdleClamped 闲置连接数不得超过上限，否则
// SetMaxIdleConns 会静默提到 MaxOpenConns，失去「低闲置」的意义。
func TestDBPoolSettingsIdleClamped(t *testing.T) {
	maxOpen, maxIdle := dbPoolSettings(fakeEnv(map[string]string{
		"DB_MAX_OPEN_CONNS": "2",
		"DB_MAX_IDLE_CONNS": "10",
	}))
	if maxOpen != 2 {
		t.Fatalf("maxOpen want 2, got %d", maxOpen)
	}
	if maxIdle != 2 {
		t.Fatalf("maxIdle must be clamped to maxOpen, got %d", maxIdle)
	}
}

// TestDBPoolSettingsInvalidFallsBack 非法值不得把池压成 0（= 无限）。
func TestDBPoolSettingsInvalidFallsBack(t *testing.T) {
	for _, v := range []string{"abc", "-1", "0", " ", "1e9"} {
		maxOpen, _ := dbPoolSettings(fakeEnv(map[string]string{"DB_MAX_OPEN_CONNS": v}))
		if maxOpen <= 0 || maxOpen >= 15 {
			t.Fatalf("invalid value %q must fall back to safe default, got %d", v, maxOpen)
		}
	}
}
