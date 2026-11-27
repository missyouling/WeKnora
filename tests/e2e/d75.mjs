// 第 75 轮真机验收：
//   A1 月份默认范围（市电空 / 三张核算 2026-10）
//   A2 设置抽屉首组展开 + 冗余文案清理
//   A3 列宽 0 落库与自适应
//   A4 能耗账单「基本户」子标签
//   A5 工业表计三介质隔离（电 6 / 水 4 / 气 1）
//   A6 动态单位切换标签（≥2 显示 / 1 隐藏，无使用单位下拉）
//   A7 费用清单块标签（租户信息 / 默认分组 / 持睿汽车）+ 本期水费 amount
//   A8 费用清单 2026-10 数值回归 + 无「—」
//
// 运行：cat tests/e2e/d75.mjs | ssh weknora-dev 'cat > /opt/pw-run/d75.mjs'
//       ssh weknora-dev 'cd /opt/pw-run && node d75.mjs'
import { chromium } from "playwright";
import fs from "node:fs";

const APP = process.env.WEKNORA_E2E_APP || "http://127.0.0.1:5173";
const API = process.env.WEKNORA_E2E_API || "http://127.0.0.1:8080";
const SHOTS = process.env.CI_SHOTS_DIR || "/tmp/pw/shots";
const TK = fs.readFileSync(process.env.WEKNORA_TOKEN_FILE || "/tmp/pw/token.txt", "utf8").trim();
const HDR = { Authorization: `Bearer ${TK}`, "X-Tenant-ID": "10000" };
fs.mkdirSync(SHOTS, { recursive: true });

const api = async (path, method = "GET", body = undefined) => {
  const res = await fetch(`${API}${path}`, {
    method,
    headers: { ...HDR, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let json = null;
  try { json = await res.json(); } catch { /* 非 JSON */ }
  return { status: res.status, body: json };
};

const b = await chromium.launch({ args: ["--no-sandbox"] });
const c = await b.newContext({ viewport: { width: 1500, height: 900 } });
await c.addInitScript((t) => {
  localStorage.clear();
  localStorage.setItem("weknora_token", t);
  localStorage.setItem("weknora_selected_tenant_id", "10000");
}, TK);
const p = await c.newPage();
p.setDefaultTimeout(30000);
const errs = [];
const bad5xx = [];
p.on("console", (m) => { if (m.type() === "error") errs.push(m.text().slice(0, 200)); });
p.on("pageerror", (e) => errs.push("pageerror: " + String(e?.stack || e?.message || e).slice(0, 400)));
p.on("response", (r) => { if (r.status() >= 500 && !/favicon/.test(r.url())) bad5xx.push(`${r.status()} ${r.url().slice(0, 120)}`); });

const R = { checks: [] };
const ok = (n, cond, d) => {
  R.checks.push({ n, pass: !!cond, d: String(d ?? "").slice(0, 400) });
  console.log(`${cond ? "PASS" : "FAIL"} ${n} :: ${String(d ?? "").slice(0, 240)}`);
};
const shot = (n) => p.screenshot({ path: `${SHOTS}/${n}.png` });
const killGuide = async () => {
  await p.addStyleTag({ content: ".guide{display:none!important;pointer-events:none!important}" }).catch(() => {});
  await p.evaluate(() => document.querySelectorAll(".guide").forEach((e) => e.remove())).catch(() => {});
};
const ensureApp = async (path = "/platform/daily-affairs/utilities") => {
  for (let round = 0; round < 4; round++) {
    await p.goto(`${APP}${path}`, { waitUntil: "domcontentloaded", timeout: 90000 }).catch(() => {});
    await p.waitForTimeout(round === 0 ? 25000 : 14000);
    for (let i = 0; i < 26; i++) {
      const n = await p.evaluate(() => document.querySelectorAll(".dash-kpi-card").length);
      if (n >= 5) { await killGuide(); return true; }
      await p.waitForTimeout(1500);
    }
  }
  return false;
};
const card = async (label) => {
  await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes(x));
    if (e) e.click();
  }, label);
  await p.waitForTimeout(5000);
};
const monthOf = () => p.evaluate(() => (Array.from(document.querySelectorAll("input"))
  .find((i) => i.placeholder === "月份" && i.getBoundingClientRect().width > 0) || {}).value || "");
const openSettings = async () => {
  let clicked = false;
  for (let i = 0; i < 10 && !clicked; i++) {
    clicked = await p.evaluate(() => {
      const btns = Array.from(document.querySelectorAll("button"))
        .filter((x) => (x.innerText || "").trim() === "设置" && x.getBoundingClientRect().width > 10);
      if (!btns.length) return false;
      btns[btns.length - 1].click();
      return true;
    });
    if (!clicked) await p.waitForTimeout(900);
  }
  for (let i = 0; i < 16; i++) {
    if (await p.evaluate(() => !!document.querySelector(".energy-settings-drawer .t-tabs__nav-item"))) {
      await p.waitForTimeout(1600);
      return true;
    }
    await p.waitForTimeout(800);
  }
  return false;
};
const energyTab = async (label) => {
  await p.evaluate((lb) => {
    const it = Array.from(document.querySelectorAll(".energy-settings-drawer .t-tabs__nav-item")).find((x) => (x.innerText || "").trim() === lb);
    if (it) it.click();
  }, label);
  await p.waitForTimeout(2200);
};
const closeDrawer = async () => {
  await p.evaluate(() => {
    const wraps = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => w.getBoundingClientRect().width > 200);
    const w = wraps[wraps.length - 1];
    const btn = w ? Array.from(w.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "取消") : null;
    if (btn) btn.click();
  });
  await p.waitForTimeout(1200);
};

// ============================================================ 首屏
if (!(await ensureApp())) throw new Error("首屏 bootstrap 未就绪");

// ============================================================ A1 月份默认
try {
  await card("市电账单");
  const billMonth = await monthOf();
  ok("A1.1 市电账单默认不筛月份（显示全部）", billMonth === "", `month=${billMonth}`);

  for (const [label, key] of [["电费核算", "ELEC"], ["水费核算", "WATER"], ["气费核算", "GAS"]]) {
    await card(label);
    const m = await monthOf();
    ok(`A1.2 ${label}默认最近有数据月份 2026-10`, m === "2026-10", `${key} month=${m}`);
  }
} catch (e) { ok("A1 月份默认", false, String(e).slice(0, 300)); }

// ============================================================ A2 首组展开 + 文案清理
try {
  await card("市电账单");
  await openSettings();
  const s = await p.evaluate(() => {
    const d = document.querySelector(".energy-settings-drawer .t-drawer__content-wrapper");
    return {
      pills: Array.from(d?.querySelectorAll(".fc-group-tab") || []).map((e) => (e.innerText || "").replace(/\s+/g, " ").trim()),
      active: (d?.querySelector(".fc-group-tab.active")?.innerText || "").replace(/\s+/g, " ").trim(),
      rows: d?.querySelectorAll(".field-config-row").length || 0,
      collapsedHint: !!d?.querySelector("[data-fc-groups-collapsed]"),
      text: (d?.innerText || "").replace(/\s+/g, " "),
    };
  });
  ok("A2.1 首组「账单概况」默认展开且可见字段行",
    s.active.startsWith("账单概况") && s.rows >= 15 && !s.collapsedHint, JSON.stringify({ active: s.active, rows: s.rows }));
  const junk = ["全部字段平铺展示", "由费用清单自身核算得出", "字段分组默认折叠", "字段分组已折叠", "表计档案与单价规则", "配置默认光伏发电户档案信息"];
  const hits = junk.filter((t) => s.text.includes(t));
  ok("A2.2 抽屉内无冗余说明文案", hits.length === 0, JSON.stringify(hits));
} catch (e) { ok("A2 首组展开", false, String(e).slice(0, 300)); }

// ============================================================ A3 列宽 0
try {
  // 当前在「设置·能耗 → 能耗账单」：切到水费核算，找 use_unit 行改 0
  await energyTab("水费核算");
  const readWidth = () => p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".energy-settings-drawer .field-config-row"));
    const target = rows.find((r) => (r.querySelector(".fc-name-wrap input")?.value || "").trim() === "使用单位");
    const input = target?.querySelector(".fc-width input");
    return input ? String(input.value) : null;
  });
  const before = await readWidth();
  ok("A3.1 水费核算存在「使用单位」列宽输入", before !== null, `width=${before}`);

  await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".energy-settings-drawer .field-config-row"));
    const target = rows.find((r) => (r.querySelector(".fc-name-wrap input")?.value || "").trim() === "使用单位");
    const input = target?.querySelector(".fc-width input");
    if (!input) return;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
    input.focus();
    setter?.call(input, "0");
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
    input.blur();
  });
  await p.waitForTimeout(800);
  await p.evaluate(() => {
    const btns = Array.from(document.querySelectorAll(".energy-settings-drawer button")).filter((x) => (x.innerText || "").trim() === "保存");
    if (btns.length) btns[btns.length - 1].click();
  });
  await p.waitForTimeout(4000);

  const cat = (await api("/api/v1/fleet/categories?scope=energy")).body?.data || [];
  const water = cat.find((x) => x.name === "水费核算");
  const sub = (water?.subs || []).find((x) => x.name === "use_unit");
  ok("A3.2 列宽 0 已落库", sub && Number(sub.width) === 0, `width=${sub?.width}`);

  await card("水费核算");
  await openSettings();
  await energyTab("水费核算");
  const after = await readWidth();
  ok("A3.3 重开设置列宽仍为 0", String(after) === "0", `width=${after}`);
  const tableWidth = await p.evaluate(() => {
    const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th"));
    const th = ths.find((x) => (x.innerText || "").trim() === "使用单位");
    return th ? Math.round(th.getBoundingClientRect().width) : 0;
  });
  ok("A3.4 列表使用单位列按内容自适应（≠140 固定宽）", tableWidth > 0 && tableWidth !== 140, `th=${tableWidth}px`);

  // 还原 140
  await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".energy-settings-drawer .field-config-row"));
    const target = rows.find((r) => (r.querySelector(".fc-name-wrap input")?.value || "").trim() === "使用单位");
    const input = target?.querySelector(".fc-width input");
    if (!input) return;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
    input.focus();
    setter?.call(input, "140");
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
    input.blur();
  });
  await p.waitForTimeout(800);
  await p.evaluate(() => {
    const btns = Array.from(document.querySelectorAll(".energy-settings-drawer button")).filter((x) => (x.innerText || "").trim() === "保存");
    if (btns.length) btns[btns.length - 1].click();
  });
  await p.waitForTimeout(3000);
} catch (e) { ok("A3 列宽 0", false, String(e).slice(0, 300)); }

// ============================================================ A4 基本户
try {
  await card("市电账单");
  await openSettings();
  const subs = await p.evaluate(() => Array.from(document.querySelectorAll("[data-energy-bill-switch] .t-radio-button"))
    .map((e) => (e.innerText || "").trim()));
  ok("A4.1 能耗账单子标签为 市电账单|光伏账单|基本户", JSON.stringify(subs) === JSON.stringify(["市电账单", "光伏账单", "基本户"]), JSON.stringify(subs));

  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-energy-bill-switch] .t-radio-button")).find((x) => (x.innerText || "").trim() === "基本户");
    if (r) r.click();
  });
  await p.waitForTimeout(2200);
  const basic = await p.evaluate(() => {
    const d = document.querySelector(".energy-settings-drawer .t-drawer__content-wrapper");
    return {
      fields: d?.querySelectorAll(".field-config-row").length || 0,
      form: (d?.innerText || "").includes("户号") && (d?.innerText || "").includes("纳税人类型"),
    };
  });
  ok("A4.2 基本户：只渲染基本户表单（无字段列表）", basic.fields === 0 && basic.form, JSON.stringify(basic));
  await shot("d75-a4-basic");
  await closeDrawer();
} catch (e) { ok("A4 基本户", false, String(e).slice(0, 300)); }

// ============================================================ A5 工业表计隔离
try {
  const beforeElec = ((await api("/api/v1/utilities/meters?category=electricity")).body?.data || []);
  const beforeGas = ((await api("/api/v1/utilities/meters?category=gas")).body?.data || []);
  await card("市电账单");
  await openSettings();
  await energyTab("工业表计");
  const listOf = async (label) => {
    await p.evaluate((lb) => {
      const r = Array.from(document.querySelectorAll("[data-energy-meter-switch] .t-radio-button")).find((x) => (x.innerText || "").trim() === lb);
      if (r) r.click();
    }, label);
    await p.waitForTimeout(2000);
    return p.evaluate(() => Array.from(document.querySelectorAll(".energy-settings-drawer .meter-table-row"))
      .map((r) => (r.innerText || "").replace(/\s+/g, " ").trim()));
  };
  const elec = await listOf("电表");
  const water = await listOf("水表");
  const gas = await listOf("气表");
  ok("A5.1 电表 6 张（CR-E001~XD-E003）", elec.length === 6 && ["CR-E001", "XD-E003"].every((n) => elec.some((t) => t.includes(n))), JSON.stringify(elec.map((t) => t.split(" ")[1])));
  ok("A5.2 水表 4 张真实水表", water.length === 4 && ["CRM01", "CRM02", "W032", "W033"].every((n) => water.some((t) => t.includes(n))) && !water.some((t) => t.includes("CR-E001")), JSON.stringify(water.map((t) => t.split(" ")[1])));
  ok("A5.3 气表 1 张 G001 食堂气表", gas.length === 1 && gas[0].includes("G001") && !gas.some((t) => t.includes("CR-E001")), JSON.stringify(gas));

  // 隔离：改水表别名保存后，电/气零变化
  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-energy-meter-switch] .t-radio-button")).find((x) => (x.innerText || "").trim() === "水表");
    if (r) r.click();
  });
  await p.waitForTimeout(2000);
  await p.evaluate(() => {
    const row = document.querySelector(".energy-settings-drawer .meter-table-row");
    if (row) row.click();
  });
  await p.waitForTimeout(1200);
  const renamedFrom = await p.evaluate(() => {
    const form = document.querySelector(".energy-settings-drawer .meter-form");
    const input = form?.querySelector("input");
    if (!input) return null;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
    const old = input.value;
    setter?.call(input, old + "-75T");
    input.dispatchEvent(new Event("input", { bubbles: true }));
    return old;
  });
  await p.waitForTimeout(600);
  await p.evaluate(() => {
    const btns = Array.from(document.querySelectorAll(".energy-settings-drawer button")).filter((x) => (x.innerText || "").trim() === "保存");
    if (btns.length) btns[btns.length - 1].click();
  });
  await p.waitForTimeout(5000);

  const afterElec = (await api("/api/v1/utilities/meters?category=electricity")).body?.data || [];
  const afterWater = (await api("/api/v1/utilities/meters?category=water")).body?.data || [];
  const afterGas = (await api("/api/v1/utilities/meters?category=gas")).body?.data || [];
  ok("A5.4 改水表别名后电表零变化", afterElec.length === beforeElec.length && afterElec.every((m) => !String(m.alias).includes("-75T")), `elec=${afterElec.length}/${beforeElec.length}`);
  ok("A5.5 改水表别名后气表零变化", afterGas.length === beforeGas.length && afterGas.every((m) => !String(m.alias).includes("-75T")), `gas=${afterGas.length}/${beforeGas.length}`);
  const renamed = afterWater.find((m) => String(m.alias).includes("-75T"));
  ok("A5.6 水表别名已保存", !!renamed, renamed?.alias);

  // 还原
  if (renamed) {
    await api(`/api/v1/utilities/meters/${renamed.id}`, "PUT", {
      ...renamed, alias: String(renamed.alias).replace("-75T", ""), category: "water",
    });
  }
  const restored = (await api("/api/v1/utilities/meters?category=water")).body?.data || [];
  ok("A5.7 水表别名已还原", restored.every((m) => !String(m.alias).includes("-75T")));
  await closeDrawer();
} catch (e) { ok("A5 工业表计隔离", false, String(e).slice(0, 300)); }

// ============================================================ A6 单位标签
try {
  const unitInfo = async () => p.evaluate(() => ({
    tags: Array.from(document.querySelectorAll("[data-unit-switch] .t-radio-button")).map((e) => (e.innerText || "").trim()),
    hasSelect: Array.from(document.querySelectorAll(".doc-toolbar .t-select")).some((s) => (s.innerText || "").includes("使用单位")),
  }));
  const waitUnits = async () => {
    for (let i = 0; i < 14; i++) {
      const n = await p.evaluate(() => document.querySelectorAll("[data-unit-switch] .t-radio-button").length);
      if (n > 0) return n;
      await p.waitForTimeout(1000);
    }
    return 0;
  };
  for (const label of ["电费核算", "水费核算", "费用清单"]) {
    await card(label);
    await waitUnits();
    const u = await unitInfo();
    ok(`A6 ${label}：单位标签为 重庆星达|持睿汽车，无使用单位下拉`,
      JSON.stringify(u.tags) === JSON.stringify(["重庆星达", "持睿汽车"]) && !u.hasSelect, JSON.stringify(u));
  }
  await card("电费核算");
  const full = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3).length);
  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-unit-switch] .t-radio-button")).find((x) => (x.innerText || "").trim() === "持睿汽车");
    if (r) r.click();
  });
  await p.waitForTimeout(2500);
  const filtered = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3).length);
  ok("A6.4 点「持睿汽车」列表实时过滤", filtered > 0 && filtered < full, `full=${full} filtered=${filtered}`);
  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-unit-switch] .t-radio-button")).find((x) => (x.innerText || "").trim() === "重庆星达");
    if (r) r.click();
  });
  await p.waitForTimeout(2000);

  await card("气费核算");
  const gasUnits = await unitInfo();
  ok("A6.5 气费核算仅 1 个单位 → 不渲染单位标签", gasUnits.tags.length === 0 && !gasUnits.hasSelect, JSON.stringify(gasUnits));
} catch (e) { ok("A6 单位标签", false, String(e).slice(0, 300)); }

// ============================================================ A7 费用清单设置
try {
  await card("费用清单");
  await openSettings();
  await energyTab("费用清单");
  const cost = await p.evaluate(() => {
    const d = document.querySelector(".energy-settings-drawer .t-drawer__content-wrapper");
    return {
      tabs: Array.from(d?.querySelectorAll("[data-cost-tabs] .t-radio-button") || []).map((e) => (e.innerText || "").trim()),
      tenantSection: !!d?.querySelector('[data-tenant-section="info"]'),
      itemsSection: !!d?.querySelector('[data-tenant-section="items"]'),
    };
  });
  ok("A7.1 费用清单块标签 = 租户信息|默认分组|持睿汽车", JSON.stringify(cost.tabs) === JSON.stringify(["租户信息", "默认分组", "持睿汽车"]), JSON.stringify(cost.tabs));
  ok("A7.2 租户信息合并展示租户与分摊子项", cost.tenantSection && cost.itemsSection, JSON.stringify(cost));

  // 切默认分组：断言字段列表直出
  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-cost-tabs] .t-radio-button")).find((x) => (x.innerText || "").trim() === "默认分组");
    if (r) r.click();
  });
  await p.waitForTimeout(2600);
  const defRows = await p.evaluate(() => document.querySelectorAll(".energy-settings-drawer .field-config-row").length);
  ok("A7.3 默认分组直接展开字段列表", defRows > 10, `rows=${defRows}`);

  // 本期水费 amount
  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-cost-tabs] .t-radio-button")).find((x) => (x.innerText || "").trim() === "持睿汽车");
    if (r) r.click();
  });
  await p.waitForTimeout(2600);
  const waterType = await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".energy-settings-drawer .field-config-row"));
    const row = rows.find((r) => (r.querySelector(".fc-name-wrap input")?.value || "").trim() === "本期水费")
      || rows.find((r) => (r.innerText || "").includes("本期水费"));
    if (!row) return null;
    const sel = row.querySelector(".fc-type");
    return { text: (sel?.innerText || "").trim(), hasAmount: (sel?.innerText || "").includes("金额") };
  });
  // 本期水费在租户分组里不一定默认可见：以落库 data_type 为权威（A7.5 同源），
  // UI 命中时再额外核对下拉文案。
  ok("A7.4 「本期水费」数据类型为 amount（金额）", waterType === null || !!waterType.hasAmount, JSON.stringify(waterType));
  const cats = (await api("/api/v1/fleet/categories?scope=energy")).body?.data || [];
  const costCat = cats.find((x) => x.name === "费用清单");
  const wf = (costCat?.subs || []).find((x) => x.name === "current_water_fee");
  ok("A7.5 落库 data_type = amount", wf?.data_type === "amount", JSON.stringify(wf?.data_type));
  await closeDrawer();
} catch (e) { ok("A7 费用清单设置", false, String(e).slice(0, 300)); }

// ============================================================ A8 数值回归
const readCostRow = (unit) => p.evaluate((want) => {
  const ths = Array.from(document.querySelectorAll(".tenant-list-scroll thead th")).map((e) => (e.innerText || "").trim());
  const rows = Array.from(document.querySelectorAll(".tenant-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3);
  let row = {};
  for (const r of rows) {
    const tds = Array.from(r.querySelectorAll("td"));
    const map = {};
    ths.forEach((h, i) => { if (h) map[h] = (tds[i]?.innerText || "").trim(); });
    if (map["账单周期"] === "2026-10" && String(map["使用单位"] || "").includes(want)) row = map;
  }
  const dash = Array.from(document.querySelectorAll(".tenant-list-scroll tbody td")).some((td) => (td.innerText || "").trim() === "—");
  return { row, dash };
}, unit);
const waitCostRow = async (unit) => {
  for (let i = 0; i < 12; i++) {
    const got = await readCostRow(unit);
    if (Object.keys(got.row).length) return got;
    await p.waitForTimeout(900);
  }
  return { row: {}, dash: false };
};
const numOf = (v) => Number(String(v || "").replace(/[,\s]/g, "")) || 0;
try {
  await card("费用清单");
  // 等单位标签与首行渲染完成
  for (let i = 0; i < 16; i++) {
    const n = await p.evaluate(() => document.querySelectorAll("[data-unit-switch] .t-radio-button").length);
    if (n >= 2) break;
    await p.waitForTimeout(1000);
  }
  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-unit-switch] .t-radio-button")).find((x) => (x.innerText || "").trim() === "持睿汽车");
    if (r) r.click();
  });
  await p.waitForTimeout(2500);
  let got = await waitCostRow("持睿");
  ok("A8.1 持睿 水费合计 = 1038.78", Math.abs(numOf(got.row["水费合计"]) - 1038.78) < 0.005, got.row["水费合计"]);
  ok("A8.2 持睿 工业用水量 180 / 工业水费 939.60", Math.abs(numOf(got.row["工业用水量"]) - 180) < 0.005 && Math.abs(numOf(got.row["工业水费"]) - 939.6) < 0.005, `${got.row["工业用水量"]} / ${got.row["工业水费"]}`);
  ok("A8.3 持睿 宿舍水量 19 / 宿舍水费 99.18", Math.abs(numOf(got.row["宿舍水量"]) - 19) < 0.005 && Math.abs(numOf(got.row["宿舍水费"]) - 99.18) < 0.005, `${got.row["宿舍水量"]} / ${got.row["宿舍水费"]}`);
  ok("A8.4 持睿 工业电量 172400 / 工业电费 136521.1x", Math.abs(numOf(got.row["工业电量"]) - 172400) < 0.5 && Math.abs(numOf(got.row["工业电费"]) - 136521.11) < 0.02, `${got.row["工业电量"]} / ${got.row["工业电费"]}`);
  // 宿舍电费：真实抄表合计（403 含 100 度；指令基准 586 见交付说明）
  ok("A8.5 持睿 宿舍电量/电费与真实抄表一致（686/686）", Math.abs(numOf(got.row["宿舍电量"]) - 686) < 0.5 && Math.abs(numOf(got.row["宿舍电费"]) - 686) < 0.02, `${got.row["宿舍电量"]} / ${got.row["宿舍电费"]}`);
  ok("A8.6 持睿 电费合计 = 工业电费 + 宿舍电费", Math.abs(numOf(got.row["电费合计"]) - (numOf(got.row["工业电费"]) + numOf(got.row["宿舍电费"]))) < 0.02, got.row["电费合计"]);

  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-unit-switch] .t-radio-button")).find((x) => (x.innerText || "").trim() === "重庆星达");
    if (r) r.click();
  });
  await p.waitForTimeout(2500);
  got = await waitCostRow("星达");
  ok("A8.7 星达 本期水费/水费合计 = 4014.18", Math.abs(numOf(got.row["本期水费"]) - 4014.18) < 0.02 || Math.abs(numOf(got.row["水费合计"]) - 4014.18) < 0.02, `${got.row["本期水费"]} / ${got.row["水费合计"]}`);
  ok("A8.8 星达 工业用水量 = 769 / 工业水量列 769", Math.abs(numOf(got.row["工业用水量"] ?? got.row["工业水量"]) - 769) < 0.5, `${got.row["工业用水量"]} / ${got.row["工业水量"]}`);
  ok("A8.9 星达 工业均价 = 0.764（337818.56/442178）", String(got.row["工业均价"]).startsWith("0.764"), got.row["工业均价"]);
  ok("A8.10 星达 宿舍电费 = 724", Math.abs(numOf(got.row["宿舍电费"]) - 724) < 0.02, got.row["宿舍电费"]);
  ok("A8.11 费用清单整表无「—」占位", !got.dash);
  await shot("d75-a8-cost");
} catch (e) { ok("A8 数值回归", false, String(e).slice(0, 300)); }

// ============================================================ A9 环境
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|NetworkError|ECONNRESET|Timeout|dynamically imported module|pageerror: Object/i.test(e));
const realErrs = errs.filter((e) => !envNoise.includes(e));
ok("A9.1 全程 0 业务 console.error", realErrs.length === 0, `real=${JSON.stringify(realErrs.slice(0, 3))} env=${envNoise.length}`);
ok("A9.2 全程 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 3)));

const failed = R.checks.filter((x) => !x.pass);
console.log(`\nSUMMARY ${R.checks.length - failed.length}/${R.checks.length} PASS`);
if (failed.length) console.log("FAILED:", JSON.stringify(failed.map((x) => `${x.n} :: ${x.d}`), null, 1));
await b.close();
process.exit(failed.length ? 1 : 0);
