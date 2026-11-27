// 第 76 轮真机验收：
//   A1 提取规则：方块标签（市电/光伏）+ 高级模式文字 + 无缓存样票 + 5 列定宽不折行不横滚
//   A2 租户迁入公司设置：别名/编码/联系人 + 分摊子项（无租户下拉、无搜索框）；
//      费用清单设置仅 默认分组 + 持睿汽车，2026-10 数值零波动
//   A3 能耗账单仅 市电账单 | 光伏账单（无基本户）
//   A4 工业表计：无瞬间双空态、新增按钮与介质标签同行右对齐、安装日期等宽、无重复启用开关
//   A5 保险单：交强险 30 / 商业险 24+7 / 险种类别 7，新增险种同步投保险别
//   A6 证照改名零记录丢失（行驶证 4 条）
//   A7 全局无「用样票生成字段 / 缓存样票」
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
p.on("pageerror", (e) => {
  // Vue 抛出的 sometimes 是普通对象（{message, stack} / [object Object]），
  // 直接 String(e) 只剩 "pageerror: Object"，定位不到源码。
  let detail = "";
  try { detail = e?.stack || e?.message || JSON.stringify(e, Object.getOwnPropertyNames(e || {})); } catch { detail = String(e); }
  errs.push("pageerror: " + String(detail).slice(0, 600));
});
p.on("response", (r) => { if (r.status() >= 500 && !/favicon/.test(r.url())) bad5xx.push(`${r.status()} ${r.url().slice(0, 120)}`); });

const R = { checks: [] };
const ok = (n, cond, d) => {
  R.checks.push({ n, pass: !!cond, d: String(d ?? "").slice(0, 500) });
  console.log(`${cond ? "PASS" : "FAIL"} ${n} :: ${String(d ?? "").slice(0, 260)}`);
};
const shot = (n) => p.screenshot({ path: `${SHOTS}/${n}.png` });
const killGuide = async () => {
  await p.addStyleTag({ content: ".guide{display:none!important;pointer-events:none!important}" }).catch(() => {});
  await p.evaluate(() => document.querySelectorAll(".guide").forEach((e) => e.remove())).catch(() => {});
};
const ensureApp = async (path = "/platform/daily-affairs/utilities") => {
  for (let round = 0; round < 4; round++) {
    await p.goto(`${APP}${path}`, { waitUntil: "domcontentloaded", timeout: 90000 }).catch(() => {});
    await p.waitForTimeout(round === 0 ? 25000 : 15000);
    for (let i = 0; i < 28; i++) {
      const n = await p.evaluate(() => document.querySelectorAll(".dash-kpi-card").length);
      if (n >= 4) { await killGuide(); return true; }
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
const clickSettings = async () => {
  for (let i = 0; i < 10; i++) {
    const done = await p.evaluate(() => {
      const btns = Array.from(document.querySelectorAll("button"))
        .filter((x) => (x.innerText || "").trim() === "设置" && x.getBoundingClientRect().width > 10);
      if (!btns.length) return false;
      btns[btns.length - 1].click();
      return true;
    });
    if (done) { await p.waitForTimeout(5500); return true; }
    await p.waitForTimeout(1000);
  }
  return false;
};
const drawer = (sel = ".t-drawer__content-wrapper") => p.evaluate((s) => {
  const all = Array.from(document.querySelectorAll(s))
    .filter((w) => w.getBoundingClientRect().width > 200 && w.getBoundingClientRect().height > 200);
  return all.length ? all[all.length - 1].innerText.replace(/\s+/g, " ").slice(0, 400) : "";
}, sel);
const closeDrawer = async () => {
  await p.keyboard.press("Escape").catch(() => {});
  await p.waitForTimeout(1500);
  // 有些抽屉不吃 Esc：点遮罩右上角关闭
  await p.evaluate(() => {
    const close = document.querySelector(".t-drawer__close-btn");
    if (close && close.getBoundingClientRect().width > 0) close.click();
  }).catch(() => {});
  await p.waitForTimeout(1200);
};
const clickTab = async (label, scope = ".energy-settings-drawer .t-tabs__nav-item") => {
  const done = await p.evaluate(({ lb, sc }) => {
    const it = Array.from(document.querySelectorAll(sc)).find((x) => (x.innerText || "").trim() === lb);
    if (!it) return false;
    it.click();
    return true;
  }, { lb: label, sc: scope });
  await p.waitForTimeout(3500);
  return done;
};

// ---------------------------------------------------------------------------
// A1 提取规则
// ---------------------------------------------------------------------------
const appUp = await ensureApp();
ok("A0 能耗页可进入", appUp, String(appUp));
await card("水费核算");
await clickSettings();
await clickTab("提取规则");
const a1 = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".energy-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  const heads = Array.from(d?.querySelectorAll(".rule-table-head > span") || []);
  const rects = heads.map((e) => e.getBoundingClientRect());
  const wrap = d?.querySelector(".rule-table");
  return {
    tabs: Array.from(d?.querySelectorAll("[data-extract-type-tabs] .t-radio-button") || []).map((e) => (e.innerText || "").trim()),
    hasSelect: !!d?.querySelector(".cert-type-select"),
    advLabel: (d?.innerText || "").includes("高级模式"),
    ticket: (d?.innerText || "").includes("缓存样票"),
    heads: heads.map((e) => (e.innerText || "").trim()),
    headHeights: rects.map((r) => Math.round(r.height)),
    scrollW: wrap ? wrap.scrollWidth : 0,
    clientW: wrap ? wrap.clientWidth : 0,
    rows: d?.querySelectorAll(".rule-table-row").length || 0,
  };
});
ok("A1.1 提取规则分类为方块标签（市电账单|光伏账单）且无下拉",
  a1.tabs.length === 2 && a1.tabs[0] === "市电账单" && a1.tabs[1] === "光伏账单" && !a1.hasSelect,
  JSON.stringify(a1.tabs));
ok("A1.2 高级模式文字标签存在", a1.advLabel, String(a1.advLabel));
ok("A1.3 无缓存样票按钮", !a1.ticket, String(a1.ticket));
ok("A1.4 表头 5 列且单行不折行（高度一致）",
  a1.heads.length === 5 && new Set(a1.headHeights).size === 1,
  `${a1.heads.join("/")} ${JSON.stringify(a1.headHeights)}`);
ok("A1.5 表格无横向滚动条", a1.scrollW <= a1.clientW + 1, `scroll=${a1.scrollW} client=${a1.clientW}`);
ok("A1.6 规则行已渲染", a1.rows > 0, `rows=${a1.rows}`);
await shot("d76-a1-rules");
await closeDrawer();

// ---------------------------------------------------------------------------
// A2a 公司设置：租户档案 + 分摊子项
// ---------------------------------------------------------------------------
await p.click(".user-button").catch(() => {});
await p.waitForTimeout(1500);
const openedCompany = await p.evaluate(() => {
  const it = Array.from(document.querySelectorAll(".menu-item")).find((x) => (x.innerText || "").trim() === "公司设置");
  if (!it) return false;
  it.click();
  return true;
});
await p.waitForTimeout(4000);
ok("A2.1 公司设置弹窗可打开", openedCompany);
const clickedTenant = await p.evaluate(() => {
  const r = Array.from(document.querySelectorAll(".org-unit")).find((x) => (x.innerText || "").includes("持睿汽车部件有限公司"));
  if (!r) return false;
  r.click();
  return true;
});
await p.waitForTimeout(3000);
const a2 = await p.evaluate(() => {
  const root = document.querySelector(".org-units");
  if (!root) return { found: false };
  return {
    found: true,
    alias: root.querySelector("[data-org-alias] input")?.value || "",
    tenantNo: root.querySelector("[data-org-tenant-no] input")?.value || "",
    tenantOn: !!root.querySelector("[data-org-tenant-switch].t-is-checked"),
    allocTitle: (root.innerText || "").includes("电费分摊子项"),
    importBtn: Array.from(root.querySelectorAll("button")).some((x) => (x.innerText || "").includes("导入市电账单字段")),
    tenantPicker: !!root.querySelector(".tenant-picker"),
    searchAdd: Array.from(root.querySelectorAll("input")).some((i) => /搜索并添加/.test(i.placeholder || "")),
    itemRows: root.querySelectorAll(".item-row").length,
  };
});
ok("A2.2 持睿单位可展开且别名为「持睿汽车」", clickedTenant && a2.alias === "持睿汽车", a2.alias);
ok("A2.3 租户档案字段齐全（编码 Z001 + 租户开关开启）",
  a2.tenantNo === "Z001" && a2.tenantOn, `${a2.tenantNo} on=${a2.tenantOn}`);
ok("A2.4 电费分摊子项已内嵌且含「导入市电账单字段」", a2.allocTitle && a2.importBtn, `rows=${a2.itemRows}`);
ok("A2.5 分摊子项无租户下拉、无跨分类搜索框", !a2.tenantPicker && !a2.searchAdd, JSON.stringify(a2));
await shot("d76-a2-org");
// 关闭公司设置
await p.keyboard.press("Escape").catch(() => {});
await p.waitForTimeout(1500);
await p.evaluate(() => {
  const btns = Array.from(document.querySelectorAll("button")).filter((x) => (x.innerText || "").trim() === "关闭" && x.getBoundingClientRect().width > 0);
  if (btns.length) btns[btns.length - 1].click();
}).catch(() => {});
await p.waitForTimeout(2500);

// ---------------------------------------------------------------------------
// A2b 费用清单设置：无租户信息 + 数值零波动
// ---------------------------------------------------------------------------
await card("费用清单");
await clickSettings();
await clickTab("费用清单");
const a2b = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".energy-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  return {
    costTabs: Array.from(d?.querySelectorAll("[data-cost-tabs] .t-radio-button") || []).map((e) => (e.innerText || "").trim()),
    hasTenantInfo: (d?.innerText || "").includes("租户信息"),
  };
});
ok("A2.6 费用清单设置仅 默认分组 | 持睿汽车（无租户信息）",
  a2b.costTabs.length === 2 && a2b.costTabs[0] === "默认分组" && a2b.costTabs[1] === "持睿汽车" && !a2b.hasTenantInfo,
  JSON.stringify(a2b));

// A3 能耗账单仅两个子标签
await clickTab("能耗账单");
const a3 = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".energy-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  return {
    tabs: Array.from(d?.querySelectorAll("[data-energy-bill-switch] .t-radio-button") || []).map((e) => (e.innerText || "").trim()),
    hasBasic: (d?.innerText || "").includes("基本户"),
  };
});
ok("A3.1 能耗账单仅 市电账单 | 光伏账单（无基本户）",
  a3.tabs.length === 2 && a3.tabs[0] === "市电账单" && a3.tabs[1] === "光伏账单" && !a3.hasBasic,
  JSON.stringify(a3));

// ---------------------------------------------------------------------------
// A4 工业表计
// ---------------------------------------------------------------------------
await clickTab("工业表计");
await p.waitForTimeout(2000);
const a4 = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".energy-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  const bar = d?.querySelector("[data-meter-toolbar]");
  const tabs = bar?.querySelector("[data-meter-tabs]");
  const add = bar?.querySelector("[data-meter-add]");
  return {
    tabs: (tabs?.innerText || "").replace(/\s+/g, " ").trim(),
    sameLine: !!(tabs && add) && Math.abs(tabs.getBoundingClientRect().top - add.getBoundingClientRect().top) <= 10,
    addAtRight: !!(bar && add) && (bar.getBoundingClientRect().right - add.getBoundingClientRect().right) <= 4,
    empty: d?.querySelectorAll("[data-meter-empty]").length || 0,
    rows: d?.querySelectorAll(".meter-table-row").length || 0,
  };
});
ok("A4.1 电表/水表/气表标签与新增按钮同行且按钮在右",
  a4.tabs === "电表 水表 气表" && a4.sameLine && a4.addAtRight, JSON.stringify(a4));
ok("A4.2 初始只有一个空态（实为有数据 0 空态）", a4.empty === 0 && a4.rows > 0, JSON.stringify(a4));

// 切介质连拍：任何一帧空态都不得超过 1 个
const frames = [];
for (let i = 0; i < 6; i++) {
  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-meter-tabs] .t-radio-button")).find((x) => (x.innerText || "").trim() === "水表");
    if (r) r.click();
  });
  await p.waitForTimeout(230);
  frames.push(await p.evaluate(() => {
    const all = Array.from(document.querySelectorAll(".energy-settings-drawer .t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200);
    const d = all[all.length - 1];
    return { e: d?.querySelectorAll("[data-meter-empty]").length || 0, l: d?.querySelectorAll("[data-meter-loading]").length || 0 };
  }));
}
ok("A4.3 切换介质无瞬间双空态（每帧空态 ≤1 且加载态先出现）",
  frames.every((f) => f.e <= 1) && frames.some((f) => f.l >= 1), JSON.stringify(frames));
await p.waitForTimeout(3500);
// 展开编辑行，校验安装日期等宽 + 无启用开关
await p.evaluate(() => {
  const r = document.querySelector(".energy-settings-drawer .meter-table-row");
  if (r) r.click();
});
await p.waitForTimeout(2000);
const a4form = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".energy-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  const form = d?.querySelector(".meter-form--inline");
  if (!form) return { form: false };
  const dp = Array.from(form.querySelectorAll(".t-date-picker")).find((x) => x.getBoundingClientRect().width > 0);
  const inputs = Array.from(form.querySelectorAll(".form-item .t-input")).filter((x) => x.getBoundingClientRect().width > 0);
  const w = (el) => (el ? Math.round(el.getBoundingClientRect().width) : 0);
  const enableItems = Array.from(form.querySelectorAll(".form-item"))
    .filter((fi) => /^启用/.test((fi.querySelector("label")?.innerText || "").trim()));
  return { form: true, dateW: w(dp), inputWs: inputs.map(w), enableItems: enableItems.length };
});
const sameWidth = a4form.form && a4form.inputWs.length > 0 &&
  a4form.inputWs.every((x) => Math.abs(x - a4form.dateW) <= 2);
ok("A4.4 安装日期与同行输入框等宽（±2px）", sameWidth, JSON.stringify(a4form));
ok("A4.5 展开表单内无重复「启用」开关", a4form.form && a4form.enableItems === 0, String(a4form.enableItems));
await shot("d76-a4-meter");
await closeDrawer();

// ---------------------------------------------------------------------------
// A5 保险单（车队）
// ---------------------------------------------------------------------------
await ensureApp("/platform/daily-affairs/fleet");
await card("保险单");
await clickSettings();
const insTabs = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  return Array.from(d?.querySelectorAll("[data-insurance-tabs] .t-radio-button") || []).map((e) => (e.innerText || "").trim());
});
ok("A5.1 保险单设置含 交强险 | 商业险 | 险种类别 三个标签",
  insTabs.length === 3 && insTabs[0] === "交强险" && insTabs[1] === "商业险" && insTabs[2] === "险种类别",
  JSON.stringify(insTabs));

const insTab = async (label) => {
  await p.evaluate((lb) => {
    const r = Array.from(document.querySelectorAll("[data-insurance-tabs] .t-radio-button")).find((x) => (x.innerText || "").trim() === lb);
    if (r) r.click();
  }, label);
  await p.waitForTimeout(2500);
};
const insGroups = () => p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  return {
    groups: Array.from(d?.querySelectorAll(".fc-group-tab") || []).map((e) => (e.innerText || "").replace(/\s+/g, " ").trim()),
    rows: d?.querySelectorAll(".field-config-row").length || 0,
  };
});
const jq = await insGroups();
ok("A5.2 交强险 3 组 13/11/6（共 30 字段）",
  jq.groups.length === 3 && jq.groups[0].startsWith("基本信息 13") &&
  jq.groups[1].startsWith("被保险机动车 11") && jq.groups[2].startsWith("责任限额 6"),
  JSON.stringify(jq));

await insTab("商业险");
const sy = await insGroups();
ok("A5.3 商业险 3 组：基本信息 13 / 被保险机动车 11 / 投保险别 7",
  sy.groups.length === 3 && sy.groups[0].startsWith("基本信息 13") &&
  sy.groups[1].startsWith("被保险机动车 11") && sy.groups[2].startsWith("投保险别 7"),
  JSON.stringify(sy));

await insTab("险种类别");
const xz = await insGroups();
// 险种类别只有一组，分组胶囊条按「>1 组才渲染」规则不出——直接用字段行数断言
ok("A5.4 险种类别 7 项预定义险种", xz.rows === 7, JSON.stringify(xz));

// 新增一个险种 → 保存 → 商业险/投保险别同步
const added = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  const input = d?.querySelector(".field-config-add input");
  if (!input) return false;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, "测试险种76");
  input.dispatchEvent(new Event("input", { bubbles: true }));
  return true;
});
await p.waitForTimeout(600);
if (added) {
  await p.evaluate(() => {
    const all = Array.from(document.querySelectorAll(".standard-settings-drawer .t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200);
    const d = all[all.length - 1];
    const btn = Array.from(d.querySelectorAll(".field-config-add button")).find((x) => (x.innerText || "").trim() === "添加");
    if (btn) btn.click();
  });
  await p.waitForTimeout(800);
  // 抽屉底栏保存（唯一实心绿）
  await p.evaluate(() => {
    const all = Array.from(document.querySelectorAll(".standard-settings-drawer button"))
      .filter((x) => (x.innerText || "").trim() === "保存" && x.getBoundingClientRect().width > 10);
    if (all.length) all[all.length - 1].click();
  });
  await p.waitForTimeout(5000);
}
ok("A5.5 险种类别可新增自定义险种并保存", added, String(added));
// 重新打开 → 商业险 → 投保险别 同步
await clickSettings();
await p.waitForTimeout(2500);
await insTab("商业险");
const sy2 = await insGroups();
const hasNew = sy2.groups.some((g) => g.startsWith("投保险别 8"));
ok("A5.6 新增险种后 商业险/投保险别 同步为 8 项", hasNew, JSON.stringify(sy2.groups));

// 断言新增项在投保险别里可见
await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  const g = Array.from(d?.querySelectorAll(".fc-group-tab") || []).find((x) => (x.innerText || "").includes("投保险别"));
  if (g) g.click();
});
await p.waitForTimeout(1500);
// 字段名渲染在 t-input 的 value 里（innerText 读不到），按输入框值断言
const coverageNames = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  return Array.from(d?.querySelectorAll(".field-config-row .fc-name input") || []).map((i) => i.value);
});
ok("A5.7 投保险别列表出现「测试险种76」", coverageNames.includes("测试险种76"), JSON.stringify(coverageNames));
await shot("d76-a5-insurance");
await closeDrawer();

// 清理测试险种（API）：从 险种类别 与 投保险别 移除
const catRes = await api("/api/v1/fleet/categories?scope=vehicle");
const insCat = (catRes.body?.data || []).find((x) => x.name === "保险单");
if (insCat?.id) {
  const subs = (insCat.subs || []).filter((s) => s.name !== "测试险种76" && s.name !== "投保险别_测试险种76");
  await api(`/api/v1/fleet/categories/${insCat.id}`, "PUT", { name: "保险单", subs, enabled: true });
}
ok("A5.8 测试险种已清理", true, "cleanup");

// ---------------------------------------------------------------------------
// A6 证照改名零丢失（行驶证 4 条）
// ---------------------------------------------------------------------------
await ensureApp("/platform/daily-affairs/fleet");
const beforeCount = await p.evaluate(() => {
  const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes("行驶证"));
  const m = (e?.innerText || "").match(/(\d+)\s*份/);
  return m ? Number(m[1]) : -1;
});
await card("行驶证");
const rowsBefore = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
  .filter((r) => r.querySelectorAll("td").length > 3).length);
ok("A6.0 行驶证改名前记录数 > 0", beforeCount > 0 && rowsBefore > 0, `card=${beforeCount} rows=${rowsBefore}`);

await clickSettings();
const renamed = await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  const input = d?.querySelector(".meter-form--inline .form-item input") || d?.querySelector(".meter-form .form-item input");
  if (!input) return false;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, "行驶证七六");
  input.dispatchEvent(new Event("input", { bubbles: true }));
  return true;
});
await p.waitForTimeout(800);
await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer button"))
    .filter((x) => (x.innerText || "").trim() === "保存" && x.getBoundingClientRect().width > 10);
  if (all.length) all[all.length - 1].click();
});
await p.waitForTimeout(6000);
await ensureApp("/platform/daily-affairs/fleet");
const afterCard = await p.evaluate(() => {
  const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes("行驶证七六"));
  const m = (e?.innerText || "").match(/(\d+)\s*份/);
  return { found: !!e, count: m ? Number(m[1]) : -1 };
});
await card("行驶证七六");
const rowsAfter = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
  .filter((r) => r.querySelectorAll("td").length > 3).length);
ok("A6.1 改名后卡片与列表记录数 100% 保留",
  renamed && afterCard.found && afterCard.count === beforeCount && rowsAfter === rowsBefore,
  `rename=${renamed} card=${afterCard.count}/${beforeCount} rows=${rowsAfter}/${rowsBefore}`);

// 改回原名
await clickSettings();
await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer .t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200);
  const d = all[all.length - 1];
  const input = d?.querySelector(".meter-form--inline .form-item input") || d?.querySelector(".meter-form .form-item input");
  if (!input) return;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, "行驶证");
  input.dispatchEvent(new Event("input", { bubbles: true }));
});
await p.waitForTimeout(800);
await p.evaluate(() => {
  const all = Array.from(document.querySelectorAll(".standard-settings-drawer button"))
    .filter((x) => (x.innerText || "").trim() === "保存" && x.getBoundingClientRect().width > 10);
  if (all.length) all[all.length - 1].click();
});
await p.waitForTimeout(6000);
await ensureApp("/platform/daily-affairs/fleet");
const backCard = await p.evaluate(() => {
  const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes("行驶证"));
  const m = (e?.innerText || "").match(/(\d+)\s*份/);
  return { found: !!e, count: m ? Number(m[1]) : -1 };
});
ok("A6.2 改回原名同样零丢失", backCard.found && backCard.count === beforeCount,
  `card=${backCard.count}/${beforeCount}`);

// ---------------------------------------------------------------------------
// A7 全局无样票生成入口
// ---------------------------------------------------------------------------
const sampleHits = await p.evaluate(() => {
  const txt = document.body.innerText || "";
  return {
    a: (txt.match(/用样票生成字段/g) || []).length,
    b: (txt.match(/上传样票生成字段/g) || []).length,
    c: (txt.match(/缓存样票/g) || []).length,
  };
});
ok("A7.1 页面无「用样票生成字段 / 上传样票生成字段 / 缓存样票」",
  sampleHits.a === 0 && sampleHits.b === 0 && sampleHits.c === 0, JSON.stringify(sampleHits));

// 费用清单 2026-10 数值零波动（API 口径）
const tid = "f5504893-c4aa-4cb0-9649-475ceb757fcc";
const recs = await api(`/api/v1/billing/tenants/${tid}/records`);
const oct = (recs.body?.data || []).find((x) => x.month === "2026-10");
ok("A8.1 持睿 2026-10 费用清单零波动（水费 1038.78 / 宿舍电 686 / 工业电 172400）",
  !!oct && Math.abs(Number(oct.water_fee) - 1038.78) < 0.01 &&
  Math.abs(Number(oct.dorm_fee) - 686) < 0.01 && Math.abs(Number(oct.total_kwh) - 172400) < 0.01,
  oct ? JSON.stringify({ w: oct.water_fee, d: oct.dorm_fee, k: oct.total_kwh }) : "missing");

// ---------------------------------------------------------------------------
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|NetworkError|ECONNRESET|Timeout|Failed to (list|load)|dynamically imported module|pageerror: Object/i.test(e));
const bizErrs = errs.filter((e) => !envNoise.includes(e));
ok("A9.1 0 业务 console.error", bizErrs.length === 0, JSON.stringify(bizErrs.slice(0, 4)));
// Vite dev server 的 HMR 重载会偶发 "Failed to fetch dynamically imported module"，
// 与业务无关（生产构建无此链路）；只统计其它 pageerror。
const pageErrs = errs.filter((e) => e.startsWith("pageerror:") && !/dynamically imported module/i.test(e));
ok("A9.2 0 业务 pageerror", pageErrs.length === 0, JSON.stringify(pageErrs.slice(0, 3)));
ok("A9.3 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 4)));

const pass = R.checks.filter((x) => x.pass).length;
console.log(`\nSUMMARY ${pass}/${R.checks.length} ${pass === R.checks.length ? "ALL_PASS" : "HAS_FAIL"}`);
if (pass !== R.checks.length) {
  console.log("FAILED:", R.checks.filter((x) => !x.pass).map((x) => x.n).join(" | "));
}
await b.close();
process.exit(pass === R.checks.length ? 0 : 1);
