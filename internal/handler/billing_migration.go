// billing_records 的列结构与精度升级（第 22 轮 P0-2 / P1-9）。
package handler

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 账单表的两条合计列（列名 → 口径）。
//
// 口径在落库处与这里各写一份注释，是为了让「为什么是这个数」不必翻代码：
//   - elec_total_fee 电费合计 = 工业电费 + 宿舍电费（不含水费）
//   - water_total_fee 水费合计 = 各水表之和（当前水费不分工业/宿舍档）
var billingRecordTotalColumns = []struct {
	Name    string
	Comment string
}{
	{"elec_total_fee", "电费合计 = 工业电费 + 宿舍电费"},
	{"water_total_fee", "水费合计 = 各水表之和"},
}

// ensurePgNumericScale 把 Postgres 某列的 numeric 精度收敛到 (precision, scale)。
//
// 只在当前 scale 不一致时才 ALTER（`numeric` 无约束时 numeric_scale 为 NULL），
// 避免每次启动都重写整张表。
func (h *BillingHandler) ensurePgNumericScale(
	ctx context.Context, table, column string, precision, scale int,
) error {
	var cur sql.NullInt64
	row := h.db.WithContext(ctx).Raw(
		`SELECT numeric_scale FROM information_schema.columns
		  WHERE table_name = $1 AND column_name = $2`, table, column).Row()
	if err := row.Scan(&cur); err != nil {
		return err
	}
	if cur.Valid && int(cur.Int64) == scale {
		return nil
	}
	ddl := fmt.Sprintf(
		`ALTER TABLE %s ALTER COLUMN %s TYPE numeric(%d,%d)`, table, column, precision, scale)
	if err := h.db.WithContext(ctx).Exec(ddl).Error; err != nil {
		return err
	}
	from := "无约束"
	if cur.Valid {
		from = fmt.Sprintf("numeric(*,%d)", cur.Int64)
	}
	logger.Infof(ctx, "%s.%s 精度已升级：%s → numeric(%d,%d)", table, column, from, precision, scale)
	return nil
}

// EnsureBillingRecordColumns 幂等升级 billing_records 的列结构与精度。
//
// 做三件事：
//  1. `ratio` 由 numeric(18,6) 升级到 numeric(18,10)。
//     6 位精度下 0.21235625 落库成 0.212356，乘 44,000 的基本电费会差 1 分
//     （9,343.66 vs 标准 9,343.67）。
//  2. 补 `elec_total_fee` / `water_total_fee` 两列（numeric(18,2)）。
//     合计口径写在落库处，页面 / PDF / 报表不必各自相加 —— 少加一次宿舍电费
//     就是 930 元的差。
//  3. 回填存量行。条件写成「为空或值不一致才更新」而不是「为 0 才更新」：
//     既能修正历史漂移，也对已正确的行零写入（幂等）。
//
// 返回被回填的行数（0 表示无需改动）。
func (h *BillingHandler) EnsureBillingRecordColumns(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	m := h.db.Migrator()
	if !m.HasTable(&types.BillingRecord{}) {
		return 0, nil
	}
	isPG := h.db.Dialector.Name() == "postgres"

	// 1) ratio 精度：numeric(18,6) → numeric(18,10)
	if isPG {
		if err := h.ensurePgNumericScale(ctx, "billing_records", "ratio", 18, 10); err != nil {
			logger.Warnf(ctx, "升级 billing_records.ratio 精度失败：%v", err)
		}
	}

	// 2) 两个合计列
	for _, col := range billingRecordTotalColumns {
		if isPG {
			// 直接写 DDL 而不是 GORM AddColumn：后者对 float64 字段会退化成
			// 无约束 numeric，与其余金额列 numeric(18,2) 不一致。
			if err := h.db.WithContext(ctx).Exec(
				`ALTER TABLE billing_records ADD COLUMN IF NOT EXISTS ` + col.Name + ` numeric(18,2)`,
			).Error; err != nil {
				return 0, err
			}
			// 上一次部署用 GORM AddColumn 建的列是无约束 numeric，这里补齐精度
			if err := h.ensurePgNumericScale(ctx, "billing_records", col.Name, 18, 2); err != nil {
				logger.Warnf(ctx, "收敛 billing_records.%s 精度失败：%v", col.Name, err)
			}
			continue
		}
		// SQLite / 其它方言：按 model tag 建列
		field := "ElecTotalFee"
		if col.Name == "water_total_fee" {
			field = "WaterTotalFee"
		}
		if m.HasColumn(&types.BillingRecord{}, field) {
			continue
		}
		if err := m.AddColumn(&types.BillingRecord{}, field); err != nil {
			return 0, err
		}
		logger.Infof(ctx, "billing_records 新增列 %s（%s）", col.Name, col.Comment)
	}

	// 3) 回填存量行。
	//
	// 必须有 `IS NULL` 分支：新建列时整列为 NULL，而 `NULL <> 0.00` 在 SQL 里
	// 是 NULL（不为真），只写「值不一致」会让回填一行都不命中（真机踩过：
	// 列建好了但全是 NULL，页面读不到电费合计）。
	res := h.db.WithContext(ctx).Exec(`
		UPDATE billing_records
		   SET elec_total_fee  = ROUND(industrial_fee + dorm_fee, 2),
		       water_total_fee = water_fee
		 WHERE elec_total_fee  IS NULL
		    OR water_total_fee IS NULL
		    OR elec_total_fee  <> ROUND(industrial_fee + dorm_fee, 2)
		    OR water_total_fee <> water_fee`)
	if res.Error != nil {
		return 0, res.Error
	}
	n := int(res.RowsAffected)
	if n > 0 {
		logger.Infof(ctx, "billing_records 合计列已回填 %d 行", n)
	}
	return n, nil
}

// EnsureBillingItemKeyConstraint 给 billing_tenant_items.item_key 加一条 CHECK：
// 禁止「功率因素」写法入库（第 22 轮 P2-6）。
//
// 背景：「功率因数调整电费」（账单原文）与「功率因素调整电费」（配置历史写法）
// 是同一个费用项。两者并存时，字段定义里会出现两行同名、列表只出一列，
// 而取数循环会把力调电费算两遍（第 19 轮真机：工业电费少 598）。
// 前端已做归一（tenantItemSignature），后端再加一道库级约束兜底 ——
// 脚本/历史导入绕过接口写入时同样拦得住。
//
// 注意顺序：先把存量归一，再加约束。归一可能撞唯一索引（同一租户已存在
// 「因数」行），撞了就只记 warn 不阻断启动，约束也随之后移（下次启动重试）。
func (h *BillingHandler) EnsureBillingItemKeyConstraint(ctx context.Context) error {
	if h.db == nil || h.db.Dialector.Name() != "postgres" {
		return nil
	}
	m := h.db.Migrator()
	if !m.HasTable(&types.BillingTenantItem{}) {
		return nil
	}
	const constraint = "chk_billing_item_key_no_alias"

	// 1) 存量归一（幂等；已无脏数据时 0 行）
	if err := h.db.WithContext(ctx).Exec(`
		UPDATE billing_tenant_items
		   SET item_key = REPLACE(item_key, '功率因素', '功率因数')
		 WHERE item_key LIKE '%功率因素%'`).Error; err != nil {
		logger.Warnf(ctx, "billing_tenant_items 同义键归一失败（稍后重试）：%v", err)
		return nil
	}

	// 2) 约束（Postgres 没有 ADD CONSTRAINT IF NOT EXISTS，先查 pg_constraint）
	var exists bool
	row := h.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = $1)`, constraint).Row()
	if err := row.Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := h.db.WithContext(ctx).Exec(
		`ALTER TABLE billing_tenant_items ADD CONSTRAINT ` + constraint +
			` CHECK (item_key NOT LIKE '%功率因素%')`).Error; err != nil {
		logger.Warnf(ctx, "添加 %s 约束失败（存量可能有冲突行）：%v", constraint, err)
		return nil
	}
	logger.Infof(ctx, "billing_tenant_items 已加约束 %s（禁止「功率因素」同义键入库）", constraint)
	return nil
}
