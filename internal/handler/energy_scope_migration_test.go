package handler

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// P2 迁移的验收口径（这是全流程最容易翻车的一段）：
//
//	✅ 21 个默认分组 × 全量字段必须全部落进 subs
//	✅ DB 里用户改过的 label / type / 默认显示 必须覆盖默认值
//	✅ DB 里用户自建的字段必须追加，不能被丢
//	✅ 幂等：跑两遍结果一致
//
//	❌ 只迁 DB 的 56 条 → line/sys/pf 等 7 个分组变空 = 事故

func newMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.UtilityFieldConfig{}, &types.FleetCategory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestBuildEnergySubsIncludesInMemoryDefaultGroups(t *testing.T) {
	db := newMigrationTestDB(t)
	h := &UtilityHandler{db: db}

	subs := h.buildEnergySubs(t.Context(), 10000)
	if len(subs) == 0 {
		t.Fatal("迁移结果为空")
	}

	// DB 里一条记录都没有（全新环境）时，也必须产出全部 21 组默认字段。
	// 这是「不能只迁 DB」的核心断言。
	byGroup := map[string]int{}
	for _, s := range subs {
		byGroup[s.Group]++
	}
	for _, g := range energyMigratedGroups {
		if byGroup[g] == 0 {
			t.Errorf("分组 %q 在迁移结果里 0 个字段——内存默认值没被合并进来（只迁 DB 就会这样）", g)
		}
	}
	// 12 个费用分项的主分组必须都在
	for _, g := range []string{"overview", "market", "line", "trans", "sys",
		"gov-industrial", "catalog", "gov-residential", "capacity", "pf", "meter", "resident-meter"} {
		if byGroup[g] == 0 {
			t.Errorf("费用分项分组 %q 缺失", g)
		}
	}
}

func TestBuildEnergySubsOverridesDBValues(t *testing.T) {
	db := newMigrationTestDB(t)
	now := time.Now()
	rows := []types.UtilityFieldConfig{
		{ID: "1", TenantID: 10000, Category: "electricity", Group: "overview",
			FieldKey: "account_no", Label: "用电户号（改过）", FieldType: "text",
			DefaultVisible: false, SortOrder: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "2", TenantID: 10000, Category: "electricity", Group: "sys",
			FieldKey: "custom_1", Label: "尖峰附加费", FieldType: "amount",
			DefaultVisible: true, SortOrder: 99, IsCustom: true, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	h := &UtilityHandler{db: db}
	subs := h.buildEnergySubs(t.Context(), 10000)

	var gotAccount, gotCustom *types.FleetCategorySub
	for i := range subs {
		if subs[i].Group == "overview" && subs[i].Name == "account_no" {
			gotAccount = &subs[i]
		}
		if subs[i].Group == "sys" && subs[i].Name == "custom_1" {
			gotCustom = &subs[i]
		}
	}
	if gotAccount == nil {
		t.Fatal("overview.account_no 未产出")
	}
	if gotAccount.Label != "用电户号（改过）" {
		t.Errorf("用户改过的 label 未覆盖默认值，得到 %q", gotAccount.Label)
	}
	if gotAccount.IsDefault {
		t.Error("用户关掉的 default_visible 未生效")
	}
	if gotCustom == nil {
		t.Fatal("用户自建字段 custom_1 被丢弃——迁移必须保留自建字段")
	}
	if gotCustom.Label != "尖峰附加费" || gotCustom.DataType != "number" {
		t.Errorf("自建字段属性不对：%+v", gotCustom)
	}
	// amount 必须归一成 number（中台没有 amount 类型）
	if gotCustom.DataType == "amount" {
		t.Error("amount 必须归一为 number，否则前端 dataType 分支匹配不到")
	}
}

func TestBuildEnergySubsGroupLabelsPresent(t *testing.T) {
	db := newMigrationTestDB(t)
	h := &UtilityHandler{db: db}
	subs := h.buildEnergySubs(t.Context(), 10000)
	for _, s := range subs {
		if s.Group == "" {
			t.Errorf("字段 %q 没有 group 标记——迁移后无法按分项归组", s.Name)
		}
		if s.GroupLabel == "" {
			t.Errorf("分组 %q 没有中文名，胶囊条会显示英文键", s.Group)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := newMigrationTestDB(t)
	h := &UtilityHandler{db: db}
	ctx := t.Context()

	w1, f1, err := h.MigrateUtilityFieldConfigsToEnergyScope(ctx, 10000, false)
	if err != nil {
		t.Fatal(err)
	}
	if w1 == 0 || f1 == 0 {
		t.Fatalf("首次迁移应写入分类与字段，得到 written=%d fields=%d", w1, f1)
	}

	var cat types.FleetCategory
	if err := db.Where("scope = ? AND name = ?", EnergyScope, EnergyBillTypeUtility).First(&cat).Error; err != nil {
		t.Fatalf("市电账单分类未创建：%v", err)
	}
	firstLen := len(cat.Subs)

	// 第二遍：subs 非空 → 跳过（幂等保护），不重复插入
	w2, f2, err := h.MigrateUtilityFieldConfigsToEnergyScope(ctx, 10000, false)
	if err != nil {
		t.Fatal(err)
	}
	if w2 != 0 || f2 != 0 {
		t.Fatalf("第二次迁移应跳过，实际 written=%d fields=%d", w2, f2)
	}

	var cat2 types.FleetCategory
	if err := db.Where("scope = ? AND name = ?", EnergyScope, EnergyBillTypeUtility).First(&cat2).Error; err != nil {
		t.Fatal(err)
	}
	if len(cat2.Subs) != firstLen {
		t.Fatalf("幂等性破坏：字段数从 %d 变成 %d", firstLen, len(cat2.Subs))
	}
}

func TestMigrateCreatesThreeCategories(t *testing.T) {
	db := newMigrationTestDB(t)
	h := &UtilityHandler{db: db}
	if _, _, err := h.MigrateUtilityFieldConfigsToEnergyScope(t.Context(), 10000, false); err != nil {
		t.Fatal(err)
	}
	var cats []types.FleetCategory
	if err := db.Where("scope = ? AND deleted_at IS NULL", EnergyScope).Order("sort_order ASC").Find(&cats).Error; err != nil {
		t.Fatal(err)
	}
	if len(cats) != 3 {
		t.Fatalf("应创建 3 个分类（市电/光伏/能耗单），实际 %d", len(cats))
	}
	names := []string{cats[0].Name, cats[1].Name, cats[2].Name}
	want := []string{EnergyBillTypeUtility, EnergyBillTypeSolar, EnergyBillTypeLedger}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("分类顺序/名称不对：%v，期望 %v", names, want)
		}
	}
	// 光伏与能耗单没有旧配置，subs 留空由各自目录补默认值
	if len(cats[1].Subs) != 0 || len(cats[2].Subs) != 0 {
		t.Error("光伏/能耗单不应从旧表迁入字段")
	}
}

func TestNormalizeUtilityDataType(t *testing.T) {
	cases := map[string]string{
		"amount":   "number",
		"number":   "number",
		"date":     "date",
		"textarea": "textarea",
		"array":    "array",
		"text":     "text",
		"":         "text",
		"unknown":  "text",
	}
	for in, want := range cases {
		if got := normalizeUtilityDataType(in); got != want {
			t.Errorf("normalizeUtilityDataType(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- 顶部 6 张卡片 ↔ 设置分类 1:1 ----

// TestEnergyViewCategoriesMatchCards 锁定「卡片分类」清单与顺序。
//
// 顺序即顶部卡片顺序：市电账单 / 光伏账单 / 电费核算 / 水费核算 / 气费核算 / 费用清单。
// 前两个由旧表迁移与目录负责，后四个由 EnsureEnergyViewCategories 补齐；
// 这里锁后四个的名字与字段集，防止改名或漏配（漏配的后果是点那张卡片的「设置」
// 打开后分类列表里找不到自己，用户改不了列）。
func TestEnergyViewCategoriesMatchCards(t *testing.T) {
	want := []string{"电费核算", "水费核算", "气费核算", "费用清单"}
	if len(energyViewCategorySpecs) != len(want) {
		t.Fatalf("卡片分类数量应为 %d，实际 %d", len(want), len(energyViewCategorySpecs))
	}
	for i, name := range want {
		if energyViewCategorySpecs[i].Name != name {
			t.Fatalf("第 %d 个分类应为 %q，实际 %q", i+1, name, energyViewCategorySpecs[i].Name)
		}
		if len(energyViewCategorySpecs[i].Fields) == 0 {
			t.Fatalf("分类 %q 没有默认字段——设置里会是一片空白", name)
		}
	}
}

// TestWaterCategoryHasWaterOnlyFees 水费核算必须带三项水费附加费。
//
// 电费/气费没有垃圾处置费、二次供水费、污水处理费；水费少了这三列，
// 用户在设置里就配不出水费的实际账单口径。
func TestWaterCategoryHasWaterOnlyFees(t *testing.T) {
	keys := map[string]bool{}
	for _, f := range energyWaterFields {
		keys[f.Key] = true
	}
	for _, k := range []string{"garbage_fee", "secondary_water_fee", "sewage_fee"} {
		if !keys[k] {
			t.Fatalf("水费核算缺少字段 %q", k)
		}
	}
	// 电费核算不该带这三列（否则会多出永远为空的列）
	for _, f := range energyMeterFields {
		switch f.Key {
		case "garbage_fee", "secondary_water_fee", "sewage_fee":
			t.Fatalf("电费核算不应包含水费专属字段 %q", f.Key)
		}
	}
}

// TestMeterFieldsHaveUniqueKeys 字段 key 必须唯一（重复会让 subs 里出现两条同名，
// 前端按 name 去重时会把后一条吞掉，表现为「某个列在设置里消失」）。
func TestMeterFieldsHaveUniqueKeys(t *testing.T) {
	for _, group := range [][]energyViewField{energyMeterFields, energyWaterFields, energyBillingFields} {
		seen := map[string]bool{}
		for _, f := range group {
			if f.Key == "" || f.Label == "" {
				t.Fatalf("字段 key/label 不能为空：%+v", f)
			}
			if seen[f.Key] {
				t.Fatalf("字段 key 重复：%q", f.Key)
			}
			seen[f.Key] = true
		}
	}
}
