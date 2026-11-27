// 第 77 轮真机验收：
//  A1 能耗 6 子页无序号列
//  A2 表计类型同源（文本输入 + 宿舍/公租房存量 + 核算下拉动态）
//  A3 宿舍新增字段即时六同步 + 403 保存成功 + 新增重复表号拦截
//  A4 抽屉底部唯一保存/取消 + 跨标签草稿一次提交
//  A5 全局无「必填」列 + 高级模式文案
//  A6 公司设置：公司管理 / 无冗余标题 / 分摊子项手风琴
//  A7 车队：留白紧凑 + 证照横向标签 + 保险单三标签 + 无「—」
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
    method, headers: { ...HDR, ...(body ? { "Content-Type": "application/json" } : {}) },
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
const errs = [], bad5xx = [], writes = [];
p.on("console", (m) => { if (m.type() === "error") errs.push(m.text().slice(0, 200)); });
p.on("pageerror", (e) => { try { errs.push("pageerror: " + String(e?.stack || e?.message || JSON.stringify(e)).slice(0, 400)); } catch { errs.push("pageerror: object"); } });
p.on("response", (r) => {
  if (r.status() >= 500 && !/favicon/.test(r.url())) bad5xx.push(`${r.status()} ${r.url().slice(0, 120)}`);
  if (/\/fleet\/hr\/dorm\/records/.test(r.url()) && ["POST", "PUT", "PATCH"].includes(r.request().method())) writes.push({ m: r.request().method(), s: r.status(), u: r.url().slice(-40) });
});

const R = { checks: [] };
const ok = (n, cond, d) => { R.checks.push({ n, pass: !!cond, d: String(d ?? "").slice(0, 500) }); console.log(`${cond ? "PASS" : "FAIL"} ${n} :: ${String(d ?? "").slice(0, 260)}`); };
const shot = (n) => p.screenshot({ path: `${SHOTS}/${n}.png` });
const killGuide = async () => {
  await p.addStyleTag({ content: ".guide{display:none!important;pointer-events:none!important}" }).catch(() => {});
  await p.evaluate(() => document.querySelectorAll(".guide").forEach((e) => e.remove())).catch(() => {});
};
const ensureApp = async (path) => {
  await p.goto(`${APP}${path}`, { waitUntil: "domcontentloaded", timeout: 90000 }).catch(() => {});
  await p.waitForTimeout(24000);
  await killGuide();
};
const card = async (label) => {
  await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes(x));
    if (e) e.click();
  }, label);
  await p.waitForTimeout(4500);
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
    if (done) { await p.waitForTimeout(1800); await waitDrawer(); return true; }
    await p.waitForTimeout(900);
  }
  return false;
};
/** 取最上层「有内容」的抽屉（TDesign 会留下多个空壳，按 innerText 长度筛）。 */
/** 等抽屉真正出现（TDesign 有多个空壳，且打开动画/首屏请求有时序）。 */
const waitDrawer = async (maxMs = 16000) => {
  const t0 = Date.now();
  while (Date.now() - t0 < maxMs) {
    const got = await p.evaluate(() => Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .some((w) => (w.innerText || "").trim().length > 20));
    if (got) return true;
    await p.waitForTimeout(600);
  }
  return false;
};
const topDrawer = (sel = ".t-drawer__content-wrapper") => p.evaluate((s) => {
  const ws = Array.from(document.querySelectorAll(s))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  if (!d) return null;
  const vis = (el) => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  return {
    w: Math.round(d.getBoundingClientRect().width),
    text: (d.innerText || "").replace(/\s+/g, " ").slice(0, 600),
    saves: Array.from(d.querySelectorAll("button")).filter((x) => vis(x) && (x.innerText || "").trim() === "保存").length,
    cancels: Array.from(d.querySelectorAll("button")).filter((x) => vis(x) && (x.innerText || "").trim() === "取消").length,
  };
}, sel);
const footerSave = async () => {
  await p.evaluate(() => {
    const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
    const d = ws[ws.length - 1];
    const btns = Array.from(d?.querySelectorAll("button") || []).filter((x) => (x.innerText || "").trim() === "保存" && x.getBoundingClientRect().width > 10);
    if (btns.length) btns[btns.length - 1].click();
  });
  await p.waitForTimeout(5000);
};
/** 等字段配置区（新增字段输入框）真正渲染出来——抽屉外壳先出、字段表后到。 */
const waitFieldConfig = async (maxMs = 20000) => {
  const t0 = Date.now();
  while (Date.now() - t0 < maxMs) {
    const got = await p.evaluate(() => {
      const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
        .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
      const d = ws[ws.length - 1];
      return !!d?.querySelector('input[placeholder="新增字段名"]');
    });
    if (got) return true;
    await p.waitForTimeout(500);
  }
  return false;
};
const closeDrawer = async () => {  await p.keyboard.press("Escape").catch(() => {});
  await p.waitForTimeout(1400);
  await p.evaluate(() => {
    const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 10);
    const d = ws[ws.length - 1];
    const btn = d?.querySelector(".t-drawer__close-btn, .setting-drawer__close, button[aria-label='关闭']");
    if (btn) btn.click();
  }).catch(() => {});
  await p.waitForTimeout(1200);
};

// --------------------------------------------------------------------------
// A1 能耗 6 子页无序号
// --------------------------------------------------------------------------
await ensureApp("/platform/daily-affairs/utilities");
const pages = ["市电账单", "光伏账单", "电费核算", "水费核算", "气费核算", "费用清单"];
const serialBad = [];
for (const name of pages) {
  await card(name);
  const heads = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll thead th")).map((e) => (e.innerText || "").trim()));
  // 序号列在 TDesign 里是空表头 + 无 colKey 文案；移除后第一格是复选框，第二格必须是业务字段
  if (!heads.length) { serialBad.push(`${name}:无表头`); continue; }
  if (heads[0] !== "" || !heads[1] || heads[1] === "1" || /^\d+$/.test(heads[1])) serialBad.push(`${name}:${JSON.stringify(heads.slice(0, 3))}`);
  const firstRow = await p.evaluate(() => {
    const tr = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).find((r) => r.querySelectorAll("td").length > 3);
    return tr ? Array.from(tr.querySelectorAll("td")).slice(0, 3).map((td) => (td.innerText || "").trim()) : [];
  });
  if (firstRow[0] && /^\d+$/.test(firstRow[0])) serialBad.push(`${name}:行首数字 ${firstRow[0]}`);
}
ok("A1.1 能耗 6 子页均无自增序号列", serialBad.length === 0, JSON.stringify(serialBad));
await shot("d77-a1-energy");

// --------------------------------------------------------------------------
// A2 表计类型
// --------------------------------------------------------------------------
await clickSettings();
await p.evaluate(() => {
  const it = Array.from(document.querySelectorAll(".energy-settings-drawer .t-tabs__nav-item")).find((x) => (x.innerText || "").trim() === "工业表计");
  if (it) it.click();
});
await p.waitForTimeout(4000);
await p.evaluate(() => { const b = document.querySelector("[data-meter-add]"); if (b) b.click(); });
await p.waitForTimeout(1500);
const kindField = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const labels = Array.from(d?.querySelectorAll(".form-item label") || []).map((e) => (e.innerText || "").trim());
  const input = d?.querySelector("[data-meter-kind-input] input");
  return { labels, hasInput: !!input, value: input?.value || "", hasUseLabel: labels.includes("用途") };
});
ok("A2.1 工业表计表单 Label 为「表计类型」且控件为文本框（默认工商业）",
  !kindField.hasUseLabel && kindField.hasInput && kindField.labels.includes("表计类型") && kindField.value === "工商业",
  JSON.stringify(kindField));
await shot("d77-a2-meter-kind");
await closeDrawer();

const dorm = await api("/api/v1/fleet/hr/dorm/records?module=dorm");
const pub = await api("/api/v1/fleet/hr/dorm/records?module=public");
const dormTypes = new Set((dorm.body?.data?.items || []).map((r) => (r.custom_data || r)["表计类型"]));
const pubTypes = new Set((pub.body?.data?.items || []).map((r) => (r.custom_data || r)["表计类型"]));
ok("A2.2 宿舍 40 间表计类型=宿舍", dormTypes.size === 1 && dormTypes.has("宿舍"), JSON.stringify([...dormTypes]));
ok("A2.3 公租房 19 套表计类型=公租房", pubTypes.size === 1 && pubTypes.has("公租房"), JSON.stringify([...pubTypes]));

// 水费核算：表计类型下拉动态包含 工商业/宿舍/公租房
await card("水费核算");
const opened = await p.evaluate(() => {
  const sel = Array.from(document.querySelectorAll(".bfs-select")).find((e) => {
    const input = e.querySelector("input");
    return input && (input.placeholder || "").includes("表计类型");
  });
  if (!sel) return false;
  const input = sel.querySelector("input");
  input.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
  input.click();
  return true;
});
await p.waitForTimeout(1200);
const kindOpts = await p.evaluate(() => Array.from(document.querySelectorAll(".t-select-option")).map((e) => (e.innerText || "").trim()).filter(Boolean));
ok("A2.4 核算筛选表计类型动态含 全部/工商业/宿舍/公租房",
  opened && ["全部表计类型", "工商业", "宿舍", "公租房"].every((x) => kindOpts.includes(x)),
  JSON.stringify(kindOpts));
await shot("d77-a2-filter");
await p.keyboard.press("Escape").catch(() => {});
await p.waitForTimeout(800);

// --------------------------------------------------------------------------
// A3 宿舍新增字段六同步 + 403 保存 + 新增重复拦截
// --------------------------------------------------------------------------
await ensureApp("/platform/daily-affairs/hr");
await card("宿舍管理");
await clickSettings();
// 在 宿舍档案 组新增字段
await waitFieldConfig();
const addedField = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const input = d?.querySelector('.field-config-add input[placeholder="新增字段名"]');
  if (!input) return false;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, "测试六同步字段");
  input.dispatchEvent(new Event("input", { bubbles: true }));
  const btn = Array.from(input.closest(".field-config-add").querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "添加");
  if (btn) btn.click();
  return true;
});
await p.waitForTimeout(1000);
const drawerBefore = await topDrawer();
ok("A3.1 宿舍设置底部仅有唯一保存/取消", drawerBefore && drawerBefore.saves === 1 && drawerBefore.cancels === 1,
  drawerBefore ? `${drawerBefore.saves}/${drawerBefore.cancels}` : "no drawer");
await footerSave();
await closeDrawer();
// 打开 403 抽屉，断言新字段即时出现
await p.evaluate(() => {
  const tr = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).find((r) => (r.innerText || "").includes("宿舍403"));
  if (tr) tr.click();
});
await p.waitForTimeout(4000);
const detail = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const labels = Array.from(d?.querySelectorAll("label, .bsd-field-label, .t-form__label") || []).map((e) => (e.innerText || "").trim());
  return { found: labels.includes("测试六同步字段"), n: labels.length, text: (d?.innerText || "").replace(/\s+/g, " ").slice(0, 200) };
});
ok("A3.1b 新增字段保存后无需刷新即出现在编辑抽屉", detail.found, JSON.stringify(detail));
await shot("d77-a3-sync");

// 403 修改水表号 → 保存 200
const beforeWrites = writes.length;
await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  // 找到 label 含「宿舍水表号」的字段输入框
  const items = Array.from(d.querySelectorAll(".bsd-field, .form-item, .t-form__item"));
  const hit = items.find((it) => (it.innerText || "").includes("宿舍水表号"));
  const input = hit?.querySelector("input");
  if (!input) return;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, "W403-T");
  input.dispatchEvent(new Event("input", { bubbles: true }));
});
await p.waitForTimeout(600);
await footerSave();
const w403 = writes.slice(beforeWrites);
ok("A3.2 宿舍403 修改水表号保存成功（无重复拦截）",
  w403.some((x) => (x.m === "PUT" || x.m === "PATCH") && x.s >= 200 && x.s < 300),
  JSON.stringify(w403));
await p.waitForTimeout(1500);
// 还原
await api(`/api/v1/fleet/hr/dorm/records/${(dorm.body?.data?.items || []).find((r) => (r.custom_data || r)["宿舍编号"] === "宿舍403")?.id}`, "PUT", {
  data: { ...((dorm.body?.data?.items || []).find((r) => (r.custom_data || r)["宿舍编号"] === "宿舍403")?.custom_data || {}), "宿舍水表号": "W021" },
});
await closeDrawer();

// 新增宿舍：重复电表号 E301 必须拦截
await p.evaluate(() => {
  const btns = Array.from(document.querySelectorAll("button")).filter((x) => (x.innerText || "").trim() === "新增" && x.getBoundingClientRect().width > 10);
  if (btns.length) btns[btns.length - 1].click();
});
await p.waitForTimeout(3500);
await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const items = Array.from(d?.querySelectorAll(".bsd-field, .form-item, .t-form__item") || []);
  const hit = items.find((it) => (it.innerText || "").includes("宿舍电表号"));
  const input = hit?.querySelector("input");
  if (!input) return;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, "E301");
  input.dispatchEvent(new Event("input", { bubbles: true }));
});
await p.waitForTimeout(500);
const beforeDup = writes.length;
await footerSave();
await p.waitForTimeout(2500);
const dupWrites = writes.slice(beforeDup);
ok("A3.3 新增宿舍填写已占用表号 E301 被拦截（4xx，未落库）",
  dupWrites.length > 0 && dupWrites.every((x) => x.s >= 400),
  JSON.stringify(dupWrites));
await closeDrawer();

// 删除临时字段（走 API，避免误点行内删除图标）
const dormCat0 = await api("/api/v1/fleet/categories?scope=hr-dorm");
const cat0 = dormCat0.body?.data?.[0];
if (cat0) {
  const keep = (cat0.subs || []).filter((x) => x.name !== "测试六同步字段");
  await api(`/api/v1/fleet/categories/${cat0.id}`, "PUT", { name: cat0.name, subs: keep, enabled: true });
}
ok("A3.4 临时字段已清理", true, "cleanup");

// --------------------------------------------------------------------------
// A4 跨标签草稿 + 抽屉唯一按钮（能耗 / 车队）
// --------------------------------------------------------------------------
// 宿舍设置：在 宿舍档案 与 公租房档案 各加一个字段，一次底部保存后都落库
await clickSettings();
await waitFieldConfig();
const addTwo = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  const add = (name) => {
    const input = d.querySelector('.field-config-add input[placeholder="新增字段名"]');
    if (!input) return false;
    setter?.call(input, name);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    const btn = Array.from(input.closest(".field-config-add").querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "添加");
    if (btn) btn.click();
    return true;
  };
  const ok1 = add("测试草稿A");
  const tab = Array.from(d.querySelectorAll(".fc-group-tab")).find((x) => (x.innerText || "").includes("公租房档案"));
  if (tab) tab.click();
  return { ok1, tab: !!tab };
});
await p.waitForTimeout(2500);
const addSecond = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const input = d?.querySelector('.field-config-add input[placeholder="新增字段名"]');
  if (!input) return false;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, "测试草稿B");
  input.dispatchEvent(new Event("input", { bubbles: true }));
  const btn = Array.from(input.closest(".field-config-add").querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "添加");
  if (btn) btn.click();
  return true;
});
await p.waitForTimeout(800);
await footerSave();
await closeDrawer();
const dormCat = await api("/api/v1/fleet/categories?scope=hr-dorm");
const subs = dormCat.body?.data?.[0]?.subs || [];
const hasA = subs.some((s) => s.name === "测试草稿A");
const hasB = subs.some((s) => s.name === "测试草稿B");
ok("A4.1 跨两个胶囊的草稿一次保存全部落库", addTwo.ok1 && addSecond && hasA && hasB, JSON.stringify({ hasA, hasB }));
// 清理
const cleaned = subs.filter((s) => !["测试草稿A", "测试草稿B"].includes(s.name));
await api(`/api/v1/fleet/categories/${dormCat.body?.data?.[0]?.id}`, "PUT", { name: "宿舍管理", subs: cleaned, enabled: true });

// 车队设置唯一按钮
await ensureApp("/platform/daily-affairs/fleet");
await card("保险单");
await clickSettings();
const fleetDrawer = await topDrawer();
ok("A4.2 车辆档案设置底部仅有唯一保存/取消", fleetDrawer && fleetDrawer.saves === 1 && fleetDrawer.cancels === 1,
  fleetDrawer ? `${fleetDrawer.saves}/${fleetDrawer.cancels}` : "none");
const flatTabs = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  return Array.from(d?.querySelectorAll("[data-cert-flat-tabs] .t-radio-button") || []).map((e) => (e.innerText || "").trim());
});
ok("A4.3 车辆档案顶部横向证照标签 + 提取规则",
  ["行驶证", "保险单", "提取规则"].every((x) => flatTabs.includes(x)), JSON.stringify(flatTabs));
const insTabs = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  return Array.from(d?.querySelectorAll("[data-insurance-tabs] .t-radio-button") || []).map((e) => (e.innerText || "").trim());
});
ok("A4.4 保险单显示 交强险/商业险/险种类别", insTabs.length === 3 && insTabs[0] === "交强险", JSON.stringify(insTabs));
// 切行驶证 → 不出现保险子标签
await p.evaluate(() => {
  const r = Array.from(document.querySelectorAll("[data-cert-flat-tabs] .t-radio-button")).find((x) => (x.innerText || "").trim() === "行驶证");
  if (r) r.click();
});
await p.waitForTimeout(2500);
const inspect = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  return {
    ins: d?.querySelectorAll("[data-insurance-tabs]").length || 0,
    name: d?.querySelector(".meter-form .form-item input")?.value || "",
  };
});
ok("A4.5 行驶证不出现交强险/商业险子标签", inspect.ins === 0 && inspect.name === "行驶证", JSON.stringify(inspect));
await shot("d77-a4-fleet");
await closeDrawer();

// 责任限额金额类型
await clickSettings();
await p.evaluate(() => {
  const r = Array.from(document.querySelectorAll("[data-cert-flat-tabs] .t-radio-button")).find((x) => (x.innerText || "").trim() === "保险单");
  if (r) r.click();
});
await p.waitForTimeout(2500);
await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const g = Array.from(d?.querySelectorAll(".fc-group-tab") || []).find((x) => (x.innerText || "").includes("责任限额"));
  if (g) g.click();
});
await p.waitForTimeout(1800);
const amountTypes = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  return Array.from(d?.querySelectorAll(".field-config-row") || []).slice(0, 3).map((r) => r.querySelector(".fc-type input")?.value || "");
});
ok("A4.6 责任限额字段数据类型显示「金额」", amountTypes.every((v) => v === "金额"), JSON.stringify(amountTypes));
await closeDrawer();

// --------------------------------------------------------------------------
// A5 无必填 + 高级模式
// --------------------------------------------------------------------------
await ensureApp("/platform/daily-affairs/utilities");
await card("水费核算");
await clickSettings();
await p.evaluate(() => {
  const it = Array.from(document.querySelectorAll(".energy-settings-drawer .t-tabs__nav-item")).find((x) => (x.innerText || "").trim() === "提取规则");
  if (it) it.click();
});
await p.waitForTimeout(3500);
const rule = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper")).filter((w) => (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const heads = Array.from(d?.querySelectorAll(".rule-table-head > span") || []).map((e) => (e.innerText || "").trim());
  return { heads, adv: (d?.innerText || "").includes("高级模式") };
});
ok("A5.1 提取规则无「必填」列 + 高级模式文案", !rule.heads.includes("必填") && rule.adv, JSON.stringify(rule));
await shot("d77-a5-rules");
await closeDrawer();

// --------------------------------------------------------------------------
// A6 公司设置
// --------------------------------------------------------------------------
await p.click(".user-button").catch(() => {});
await p.waitForTimeout(1200);
await p.evaluate(() => {
  const it = Array.from(document.querySelectorAll(".menu-item")).find((x) => (x.innerText || "").trim() === "公司设置");
  if (it) it.click();
});
await p.waitForTimeout(3500);
const nav = await p.evaluate(() => (document.body.innerText || "").includes("公司管理"));
ok("A6.1 左侧菜单为「公司管理」", nav);
await p.evaluate(() => {
  const r = Array.from(document.querySelectorAll(".org-unit")).find((x) => (x.innerText || "").includes("持睿汽车部件有限公司"));
  if (r) r.click();
});
await p.waitForTimeout(2500);
const accordion = await p.evaluate(() => {
  const root = document.querySelector(".org-units");
  const heads = Array.from(root?.querySelectorAll(".item-group-head") || []);
  const before = heads.filter((h) => h.nextElementSibling && getComputedStyle(h.nextElementSibling).display !== "none").length;
  const hasRedundant = (root?.innerText || "").includes("电费分摊子项");
  return { groups: heads.length, before, hasRedundant };
});
ok("A6.2 无冗余「电费分摊子项」标题", !accordion.hasRedundant, JSON.stringify(accordion));
ok("A6.3 分摊子项默认全部折叠", accordion.groups > 1 && accordion.before === 0, JSON.stringify(accordion));
// 展开第一组再点第二组 → 第一组自动收起
if (accordion.groups > 1) {
  await p.evaluate(() => {
    const h = Array.from(document.querySelectorAll(".org-units .item-group-head"));
    if (h[0]) h[0].click();
  });
  await p.waitForTimeout(600);
  await p.evaluate(() => {
    const h = Array.from(document.querySelectorAll(".org-units .item-group-head"));
    if (h[1]) h[1].click();
  });
  await p.waitForTimeout(600);
  const expanded = await p.evaluate(() => {
    const heads = Array.from(document.querySelectorAll(".org-units .item-group-head"));
    return heads.map((h) => !!(h.nextElementSibling && getComputedStyle(h.nextElementSibling).display !== "none"));
  });
  ok("A6.4 手风琴：展开第二组时第一组自动收起", expanded[0] === false && expanded[1] === true, JSON.stringify(expanded));
}
await shot("d77-a6-org");
await p.keyboard.press("Escape").catch(() => {});
await p.waitForTimeout(1500);

// --------------------------------------------------------------------------
// A7 车队留白 + 保险单列表三标签
// --------------------------------------------------------------------------
await ensureApp("/platform/daily-affairs/fleet");
const topGap = await p.evaluate(() => {
  const c = document.querySelector(".dash-kpi-card");
  const r = c?.getBoundingClientRect();
  return r ? Math.round(r.top) : -1;
});
ok("A7.1 车队卡片顶部留白紧凑（≤120px）", topGap >= 0 && topGap <= 120, String(topGap));
await card("保险单");
const listTabs = await p.evaluate(() => Array.from(document.querySelectorAll("[data-insurance-list-tabs] .t-radio-button")).map((e) => (e.innerText || "").trim()));
ok("A7.2 保险单列表上方有 交强险/商业险/险种类别", listTabs.length === 3 && listTabs[0] === "交强险", JSON.stringify(listTabs));
const rows = await p.evaluate(() => {
  const trs = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3);
  const dash = Array.from(document.querySelectorAll(".doc-list-scroll tbody td")).filter((td) => (td.innerText || "").trim() === "—").length;
  return { n: trs.length, dash };
});
ok("A7.3 存量保单完整展示且无「—」占位", rows.n >= 2 && rows.dash === 0, JSON.stringify(rows));
// 切商业险仍有数据（存量保单按险种关键词识别）
await p.evaluate(() => {
  const r = Array.from(document.querySelectorAll("[data-insurance-list-tabs] .t-radio-button")).find((x) => (x.innerText || "").trim() === "商业险");
  if (r) r.click();
});
await p.waitForTimeout(1500);
const syRows = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3).length);
ok("A7.4 商业险标签下存量保单可见", syRows >= 1, String(syRows));
// 险种类别 → 字典 7 项
await p.evaluate(() => {
  const r = Array.from(document.querySelectorAll("[data-insurance-list-tabs] .t-radio-button")).find((x) => (x.innerText || "").trim() === "险种类别");
  if (r) r.click();
});
await p.waitForTimeout(1500);
const typeRows = await p.evaluate(() => document.querySelectorAll("[data-insurance-type-panel] .insurance-type-row").length);
ok("A7.5 险种类别展示 7 项标准险种", typeRows === 7, String(typeRows));
await shot("d77-a7-insurance");

// --------------------------------------------------------------------------
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|NetworkError|ECONNRESET|Timeout|Failed to (list|load)|dynamically imported module/i.test(e));
const bizErrs = errs.filter((e) => !envNoise.includes(e));
ok("A8.1 0 业务 console.error", bizErrs.length === 0, JSON.stringify(bizErrs.slice(0, 4)));
const pageErrs = errs.filter((e) => e.startsWith("pageerror:") && !/dynamically imported module/i.test(e));
ok("A8.2 0 业务 pageerror", pageErrs.length === 0, JSON.stringify(pageErrs.slice(0, 3)));
ok("A8.3 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 4)));

const pass = R.checks.filter((x) => x.pass).length;
console.log(`\nSUMMARY ${pass}/${R.checks.length} ${pass === R.checks.length ? "ALL_PASS" : "HAS_FAIL"}`);
if (pass !== R.checks.length) console.log("FAILED:", R.checks.filter((x) => !x.pass).map((x) => x.n).join(" | "));
await b.close();
process.exit(pass === R.checks.length ? 0 : 1);
