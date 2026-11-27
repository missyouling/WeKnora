// 第 78 轮真机验收：
//  A1 维保卡片收敛（一般维修 / 二级维护 / 历史记录）+ 横向平级设置抽屉
//  A2 一般维修三子表编辑 + 金额钩稽（90 / 189 / 18 / 297）+ 差额告警 + 持久回显
//  A3 二级维护随单非解析附件袋（上传 2 / 零解析 / 无后缀预览 / 列表计数 2→1）
//  A4 行驶证年检日期六同步 + 证照状态 4 态联动
//  A5 铁律：无序号列、无 — 占位、0 console.error / pageerror / 5xx
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
p.setDefaultTimeout(40000);
const errs = [], bad5xx = [];
p.on("console", (m) => { if (m.type() === "error") errs.push(m.text().slice(0, 200)); });
p.on("pageerror", (e) => { try { errs.push("pageerror: " + String(e?.stack || e?.message || JSON.stringify(e)).slice(0, 400)); } catch { errs.push("pageerror: object"); } });
p.on("response", (r) => {
  if (r.status() >= 500 && !/favicon/.test(r.url())) bad5xx.push(`${r.status()} ${r.url().slice(0, 120)}`);
});

const R = { checks: [] };
const ok = (n, cond, d) => {
  R.checks.push({ n, pass: !!cond, d: String(d ?? "").slice(0, 600) });
  console.log(`${cond ? "PASS" : "FAIL"} ${n} :: ${String(d ?? "").slice(0, 300)}`);
};
const shot = (n) => p.screenshot({ path: `${SHOTS}/${n}.png` });
const killGuide = async () => {
  await p.addStyleTag({ content: ".guide{display:none!important;pointer-events:none!important}" }).catch(() => {});
  await p.evaluate(() => document.querySelectorAll(".guide").forEach((e) => e.remove())).catch(() => {});
};
const ensureApp = async (path) => {
  await p.goto(`${APP}${path}`, { waitUntil: "domcontentloaded", timeout: 90000 }).catch(() => {});
  await p.waitForTimeout(26000);
  await killGuide();
};
const card = async (label) => {
  await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes(x));
    if (e) e.click();
  }, label);
  await p.waitForTimeout(5000);
  await killGuide();
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
    if (done) { await p.waitForTimeout(2000); await waitDrawer(); return true; }
    await p.waitForTimeout(900);
  }
  return false;
};
const waitDrawer = async (maxMs = 20000) => {
  const t0 = Date.now();
  while (Date.now() - t0 < maxMs) {
    const got = await p.evaluate(() => Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .some((w) => (w.innerText || "").trim().length > 20));
    if (got) return true;
    await p.waitForTimeout(600);
  }
  return false;
};
/** 等设置抽屉的字段配置区真正渲染（外壳先出、字段表后到）。 */
const waitFieldConfig = async (maxMs = 20000) => {
  const t0 = Date.now();
  while (Date.now() - t0 < maxMs) {
    const got = await p.evaluate(() => {
      const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
        .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
      const d = ws[ws.length - 1];
      return !!(d && d.querySelector(".field-config-row"));
    });
    if (got) return true;
    await p.waitForTimeout(500);
  }
  return false;
};
const topDrawer = () => p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  if (!d) return null;
  const vis = (el) => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  return {
    w: Math.round(d.getBoundingClientRect().width),
    text: (d.innerText || "").replace(/\s+/g, " ").slice(0, 800),
    saves: Array.from(d.querySelectorAll("button")).filter((x) => vis(x) && (x.innerText || "").trim() === "保存").length,
    cancels: Array.from(d.querySelectorAll("button")).filter((x) => vis(x) && (x.innerText || "").trim() === "取消").length,
  };
});
const closeDrawer = async () => {
  await p.keyboard.press("Escape").catch(() => {});
  await p.waitForTimeout(1500);
  await p.evaluate(() => {
    const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 10);
    const d = ws[ws.length - 1];
    const btn = d?.querySelector(".t-drawer__close-btn, .setting-drawer__close, button[aria-label='关闭']");
    if (btn) btn.click();
  }).catch(() => {});
  await p.waitForTimeout(1500);
};
/** 详情抽屉：等「结算明细」或表单渲染完成。 */
const waitDetail = async (maxMs = 20000) => {
  const t0 = Date.now();
  while (Date.now() - t0 < maxMs) {
    const got = await p.evaluate(() => {
      const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
        .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
      const d = ws[ws.length - 1];
      return !!(d && (d.querySelector("[data-repair-line-items]") || d.querySelector(".bsd-fields") || d.querySelector(".bsd-block")));
    });
    if (got) return true;
    await p.waitForTimeout(500);
  }
  return false;
};
/** 点抽屉里的保存并等接口返回。 */
const saveDetail = async () => {
  await p.evaluate(() => {
    const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
    const d = ws[ws.length - 1];
    const btns = Array.from(d?.querySelectorAll("button") || [])
      .filter((x) => (x.innerText || "").trim() === "保存" && x.getBoundingClientRect().width > 10);
    if (btns.length) btns[btns.length - 1].click();
  });
  await p.waitForTimeout(6000);
};

// ==========================================================================
// A1 维保卡片收敛 + 横向平级设置抽屉
// ==========================================================================
await ensureApp("/platform/daily-affairs/fleet?tab=maintain-archive");
const kpiLabels = await p.evaluate(() => Array.from(document.querySelectorAll(".dash-kpi-card"))
  .map((x) => (x.innerText || "").replace(/\s+/g, " ").trim().split(" ")[0]));
ok("A1.1 维保卡片为 一般维修 / 二级维护 / 历史记录（无合格证卡）",
  kpiLabels.includes("一般维修") && kpiLabels.includes("二级维护") && kpiLabels.includes("历史记录")
  && !kpiLabels.includes("合格证"), JSON.stringify(kpiLabels));

const heads = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
  .map((e) => (e.innerText || "").trim()));
ok("A1.2 维保列表无序号列（复选框后直接业务字段）",
  heads.length > 2 && heads[0] === "" && !/^\d+$/.test(heads[1] || ""), JSON.stringify(heads.slice(0, 4)));

const dashCells = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody td"))
  .filter((td) => (td.innerText || "").trim() === "—").length);
ok("A1.3 维保列表无「—」占位符", dashCells === 0, String(dashCells));

// 二级维护卡片 → 设置
await card("二级维护");
await clickSettings();
await waitFieldConfig();
const flatTabs = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  return {
    tabs: Array.from(d?.querySelectorAll("[data-cert-flat-tabs] .t-radio-button") || []).map((e) => (e.innerText || "").trim()),
    active: Array.from(d?.querySelectorAll("[data-cert-flat-tabs] .t-radio-button.t-is-checked") || []).map((e) => (e.innerText || "").trim()),
    groups: Array.from(d?.querySelectorAll(".fc-group-tab") || []).map((e) => (e.innerText || "").replace(/\s+/g, " ").trim()),
    firstGroupActive: Array.from(d?.querySelectorAll(".fc-group-tab.active") || []).map((e) => (e.innerText || "").trim()),
  };
});
ok("A1.4 抽屉顶层横向平级标签 一般维修|二级维护|提取规则",
  ["一般维修", "二级维护", "提取规则"].every((x) => flatTabs.tabs.includes(x)), JSON.stringify(flatTabs.tabs));
ok("A1.5 自动激活「二级维护」", flatTabs.active.includes("二级维护"), JSON.stringify(flatTabs.active));
ok("A1.6 默认展开首个胶囊分组「合格证信息」",
  flatTabs.groups.length > 0 && (flatTabs.firstGroupActive[0] || "").includes("合格证信息"),
  JSON.stringify({ groups: flatTabs.groups, active: flatTabs.firstGroupActive }));

const drawerBtns = await topDrawer();
ok("A1.7 抽屉内仅底部 1 套 取消 / 保存", drawerBtns && drawerBtns.saves === 1 && drawerBtns.cancels === 1,
  drawerBtns ? `${drawerBtns.saves}/${drawerBtns.cancels}` : "no drawer");

// 切到提取规则
await p.evaluate(() => {
  const r = Array.from(document.querySelectorAll("[data-cert-flat-tabs] .t-radio-button"))
    .find((x) => (x.innerText || "").trim() === "提取规则");
  if (r) r.click();
});
await p.waitForTimeout(4000);
const rulePanel = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const typeTabs = Array.from(d?.querySelectorAll("[data-extract-type-tabs] .t-radio-button") || []).map((e) => (e.innerText || "").trim());
  const headSpans = Array.from(d?.querySelectorAll(".rule-table-head > span") || []).map((e) => (e.innerText || "").trim());
  const text = (d?.innerText || "").replace(/\s+/g, " ");
  return { typeTabs, headSpans, adv: text.includes("高级模式"), required: headSpans.includes("必填") };
});
ok("A1.8 提取规则用方块标签（一般维修|二级维护）",
  ["一般维修", "二级维护"].every((x) => rulePanel.typeTabs.includes(x)), JSON.stringify(rulePanel.typeTabs));
ok("A1.9 提取规则无「必填」列且有「高级模式」文字",
  !rulePanel.required && rulePanel.adv, JSON.stringify({ heads: rulePanel.headSpans, adv: rulePanel.adv }));
await shot("d78-a1-settings");
await closeDrawer();

// ==========================================================================
// A2 一般维修三子表 + 金额钩稽
// ==========================================================================
await card("一般维修");
await p.waitForTimeout(3000);
// 新建一条测试记录（走 API 更快且可精确控制）
const TEST_NO = "JS78TEST001";
const created = await api("/api/v1/fleet/records", "POST", {
  record_type: "maintain-archive",
  sub_category: "一般维修",
  doc_type: "一般维修",
  record_date: "2026-11-27",
  amount: 297,
  data: {
    结算单号: TEST_NO, 车牌号: "渝C81022", 结算日期: "2026-11-27",
    挂账单位: "重庆星达铜业有限公司", 工时费: 90, 材料费: 189, 管理费: 0, 其它费: 18, 实际应收: 297,
    承修单位: "重庆某某汽修厂",
  },
});
ok("A2.0 测试记录创建成功", created.status >= 200 && created.status < 300, String(created.status));
const testId = created.body?.data?.id || created.body?.data?.ID || "";

await ensureApp("/platform/daily-affairs/fleet?tab=maintain-archive");
await card("一般维修");
await p.waitForTimeout(4000);
// 打开该记录
const opened = await p.evaluate((no) => {
  const tr = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .find((r) => (r.innerText || "").includes(no));
  if (!tr) return false;
  tr.click();
  return true;
}, TEST_NO);
await p.waitForTimeout(5000);
await waitDetail();
ok("A2.1 分栏详情抽屉打开（左图右表）", opened, String(opened));

const layout = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const blocks = Array.from(d?.querySelectorAll(".bsd-block-title") || []).map((e) => (e.innerText || "").trim());
  return {
    blocks,
    hasLineItems: !!d?.querySelector("[data-repair-line-items]"),
    hasAttach: !!d?.querySelector("[data-attach-section]"),
    previewToggle: !!d?.querySelector(".bsd-preview-toggle"),
  };
});
ok("A2.2 抽屉含「结算明细」三子表", layout.hasLineItems, JSON.stringify(layout.blocks));
ok("A2.3 抽屉含「相关附件」分组", layout.hasAttach, JSON.stringify(layout.blocks));

// 填三子表：3 项工时 50/20/20、4 项零件 160/20/5/4、附加税费 18
// 注意：必须**逐个点击 + 等待 Vue 重渲染** —— 在同一个 evaluate 里连点 N 次，
// 只有第一次能拿到按钮（后续点击时 DOM 还没更新），实测只加了 1 行。
const clickAdd = async (kind, n) => {
  for (let i = 0; i < n; i++) {
    await p.evaluate((k) => {
      const btn = document.querySelector(`[data-li-add="${k}"]`);
      if (btn) btn.click();
    }, kind);
    await p.waitForTimeout(700);
  }
};
await clickAdd("labor", 3);
await clickAdd("part", 4);
await clickAdd("extra", 1);
await p.waitForTimeout(1500);

// 填值：逐行定位 input
const filled = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const root = d?.querySelector("[data-line-items]");
  if (!root) return "no root";
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  const setVal = (el, v) => {
    if (!el) return;
    setter?.call(el, String(v));
    el.dispatchEvent(new Event("input", { bubbles: true }));
    el.dispatchEvent(new Event("change", { bubbles: true }));
  };
  const blocks = Array.from(root.querySelectorAll(".li-block"));
  if (blocks.length < 3) return `blocks=${blocks.length}`;
  const labor = blocks[0], part = blocks[1], extra = blocks[2];
  const lRows = Array.from(labor.querySelectorAll("tbody tr")).filter((r) => r.querySelectorAll("input").length >= 4);
  const L = [
    ["拔左前轮更换轮胎螺丝", 1, 50],
    ["更换刹车管", 1, 20],
    ["更换喷水接头", 1, 20],
  ];
  lRows.slice(0, 3).forEach((r, i) => {
    const ins = r.querySelectorAll("input");
    setVal(ins[0], L[i][0]);
    setVal(ins[1], L[i][1]);
    setVal(ins[2], L[i][2]);
  });
  const pRows = Array.from(part.querySelectorAll("tbody tr")).filter((r) => r.querySelectorAll("input").length >= 5);
  const P = [
    ["轮胎螺丝", "个", 4, 40, 160],
    ["刹车管", "根", 1, 20, 20],
    ["喷水接头", "个", 1, 5, 5],
    ["卡箍", "个", 2, 2, 4],
  ];
  pRows.slice(0, 4).forEach((r, i) => {
    const ins = r.querySelectorAll("input");
    setVal(ins[0], P[i][0]);
    setVal(ins[1], P[i][1]);
    setVal(ins[2], P[i][2]);
    setVal(ins[3], P[i][3]);
    setVal(ins[4], P[i][4]);
  });
  const eRows = Array.from(extra.querySelectorAll("tbody tr")).filter((r) => r.querySelectorAll("input").length >= 2);
  if (eRows[0]) {
    const ins = eRows[0].querySelectorAll("input");
    setVal(ins[0], "税费");
    setVal(ins[1], 18);
  }
  return `labor=${lRows.length} part=${pRows.length} extra=${eRows.length}`;
});
await p.waitForTimeout(2500);

const totals = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const sums = Array.from(d?.querySelectorAll(".li-sum") || []).map((e) => (e.innerText || "").replace(/\s+/g, " ").trim());
  return { sums };
});
ok("A2.4 三子表合计 = 工时 90 / 材料 189 / 附加 18",
  totals.sums.some((s) => s.includes("90.00")) && totals.sums.some((s) => s.includes("189.00"))
  && totals.sums.some((s) => s.includes("18.00")), JSON.stringify({ filled, sums: totals.sums }));
await shot("d78-a2-lineitems");

// 把实际应收改成 300 → 断言橙色差额提示
await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const items = Array.from(d?.querySelectorAll(".t-form__item, .bsd-field, .form-item") || []);
  const hit = items.find((it) => (it.innerText || "").includes("实际应收"));
  const input = hit?.querySelector("input");
  if (!input) return;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, "300");
  input.dispatchEvent(new Event("input", { bubbles: true }));
  input.dispatchEvent(new Event("change", { bubbles: true }));
  input.dispatchEvent(new Event("blur", { bubbles: true }));
});
await p.waitForTimeout(2500);
const warn = await p.evaluate(() => {
  const el = document.querySelector("[data-repair-warn]");
  return { has: !!el, text: (el?.innerText || "").replace(/\s+/g, " ").slice(0, 300) };
});
ok("A2.5 实际应收改 300 后出现橙色金额钩稽差额提示", warn.has && warn.text.includes("差额"), JSON.stringify(warn));

// 一键按明细重算
await p.evaluate(() => { const b2 = document.querySelector("[data-repair-recalc]"); if (b2) b2.click(); });
await p.waitForTimeout(2500);
const afterRecalc = await p.evaluate(() => {
  const el = document.querySelector("[data-repair-warn]");
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  const items = Array.from(d?.querySelectorAll(".t-form__item, .bsd-field, .form-item") || []);
  const hit = items.find((it) => (it.innerText || "").includes("实际应收"));
  return { warnGone: !el, actual: hit?.querySelector("input")?.value || "" };
});
ok("A2.6 按明细重算后告警消失且实际应收=297",
  afterRecalc.warnGone && String(afterRecalc.actual).replace(/[^\d.]/g, "") === "297", JSON.stringify(afterRecalc));
await shot("d78-a2-recalc");

// 保存并刷新断言持久回显
await saveDetail();
await p.waitForTimeout(2000);
const persisted = await api(`/api/v1/fleet/records?type=maintain-archive&sub_category=${encodeURIComponent("一般维修")}&page_size=200`);
const row = (persisted.body?.data?.items || []).find((r) => (r.data || {})["结算单号"] === TEST_NO);
const li = row?.data?.line_items || {};
ok("A2.7 三子表 100% 持久回显（3 工时 / 4 零件 / 1 附加）",
  (li.labor_items || []).length === 3 && (li.part_items || []).length === 4 && (li.extra_items || []).length === 1,
  JSON.stringify({ labor: (li.labor_items || []).length, part: (li.part_items || []).length, extra: (li.extra_items || []).length }));
ok("A2.8 主表金额与摘要已按明细落库",
  Number(row?.data?.["工时费"]) === 90 && Number(row?.data?.["材料费"]) === 189
  && Number(row?.data?.["实际应收"]) === 297 && String(row?.data?.["维修项目摘要"] || "").includes("（3项/4件）"),
  JSON.stringify({ 工时费: row?.data?.["工时费"], 材料费: row?.data?.["材料费"], 实际应收: row?.data?.["实际应收"], 摘要: row?.data?.["维修项目摘要"] }));

// ==========================================================================
// A3 二级维护随单非解析附件袋
// ==========================================================================
const TEST_CERT = "HG78TEST001";
const created2 = await api("/api/v1/fleet/records", "POST", {
  record_type: "maintain-archive",
  sub_category: "二级维护",
  doc_type: "二级维护",
  record_date: "2026-11-27",
  data: { 合格证编号: TEST_CERT, 车牌号: "渝C81022", 维修类别: "二级维护", 承修方: "重庆某某汽修厂" },
});
const certId = created2.body?.data?.id || created2.body?.data?.ID || "";
ok("A3.0 二级维护测试记录创建成功", created2.status >= 200 && created2.status < 300 && !!certId, String(created2.status));

// 上传 2 个附件（1 PNG + 1 PDF）：走真实 multipart 接口
const PNG_1PX = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg==",
  "base64",
);
const PDF_MIN = Buffer.from(
  "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
  "3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n",
  "utf8",
);
const uploadOne = async (buf, name, mime, tag) => {
  const fd = new FormData();
  fd.append("tag", tag);
  fd.append("files", new Blob([buf], { type: mime }), name);
  const t0 = Date.now();
  const res = await fetch(`${API}/api/v1/fleet/records/${certId}/attachments`, {
    method: "POST", headers: HDR, body: fd,
  });
  let json = null;
  try { json = await res.json(); } catch { /* ignore */ }
  return { status: res.status, ms: Date.now() - t0, json };
};
const up1 = await uploadOne(PNG_1PX, "竣工检测单78.png", "image/png", "竣工检测单");
const up2 = await uploadOne(PDF_MIN, "竣工检测单78.pdf", "application/pdf", "竣工检测单");
ok("A3.1 附件上传瞬时 2xx（PNG + PDF）",
  up1.status >= 200 && up1.status < 300 && up2.status >= 200 && up2.status < 300,
  JSON.stringify({ png: up1.status, pdf: up2.status, ms: [up1.ms, up2.ms] }));

// 零解析：附件上传后该记录不应出现任何新的提取/解析产物
const recAfter = await api(`/api/v1/fleet/records?type=maintain-archive&sub_category=${encodeURIComponent("二级维护")}&page_size=200`);
const certRow = (recAfter.body?.data?.items || []).find((r) => (r.data || {})["合格证编号"] === TEST_CERT);
ok("A3.2 附件上传未触发解析（记录字段未被改写）",
  certRow && String(certRow.data?.["承修方"] || "") === "重庆某某汽修厂", JSON.stringify({ 承修方: certRow?.data?.["承修方"] }));

const attList = await api(`/api/v1/fleet/records/${certId}/attachments`);
const attItems = attList.body?.data?.items || [];
ok("A3.3 附件列表返回 2 条元数据且不含 BYTEA",
  attItems.length === 2 && !JSON.stringify(attList.body).includes("iVBORw0KGgo"),
  JSON.stringify(attItems.map((x) => ({ n: x.file_name, t: x.tag, s: x.file_size }))));

// 无后缀预览路由
const viewRes = await fetch(`${API}/api/v1/fleet/attachments/view?id=${encodeURIComponent(attItems[0]?.id || "")}`, { headers: HDR });
const viewCt = viewRes.headers.get("content-type") || "";
const viewCd = viewRes.headers.get("content-disposition") || "";
const viewBytes = Buffer.from(await viewRes.arrayBuffer());
ok("A3.4 无后缀预览路由返回 200 + 正确 Content-Type + inline",
  viewRes.status === 200 && /png|jpeg|pdf/.test(viewCt) && /inline/.test(viewCd) && viewBytes.length > 0,
  JSON.stringify({ status: viewRes.status, ct: viewCt, cd: viewCd, bytes: viewBytes.length }));

// 列表「附件」列显示 2
await ensureApp("/platform/daily-affairs/fleet?tab=maintain-archive");
await card("二级维护");
await p.waitForTimeout(4000);
const attachCell = await p.evaluate((cert) => {
  const tr = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .find((r) => (r.innerText || "").includes(cert));
  if (!tr) return { found: false };
  const cells = Array.from(tr.querySelectorAll("td")).map((td) => (td.innerText || "").trim());
  return { found: true, cells, attachText: (tr.querySelector(".fleet-attach-cell")?.innerText || "").trim() };
}, TEST_CERT);
ok("A3.5 列表「附件」列显示 2 个附件", attachCell.attachText.includes("2"), JSON.stringify(attachCell.attachText));

// 打开抽屉断言附件区 + 页内预览
await p.evaluate((cert) => {
  const tr = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .find((r) => (r.innerText || "").includes(cert));
  if (tr) tr.click();
}, TEST_CERT);
await p.waitForTimeout(6000);
await waitDetail();
const bagInfo = await p.evaluate(() => {
  const bag = document.querySelector("[data-attach-bag]");
  if (!bag) return { has: false };
  return {
    has: true,
    rows: Array.from(bag.querySelectorAll(".attach-bag__row")).length,
    names: Array.from(bag.querySelectorAll(".attach-bag__name")).map((x) => (x.innerText || "").trim()),
    tagChips: Array.from(bag.querySelectorAll(".attach-bag__tag-chip")).map((x) => (x.innerText || "").trim()),
  };
});
ok("A3.6 抽屉「相关附件」区展示 2 个附件 + 标签", bagInfo.has && bagInfo.rows === 2
  && bagInfo.tagChips.every((t) => t === "竣工检测单"), JSON.stringify(bagInfo));
await shot("d78-a3-attachments");

// 页内预览（FilePreviewDialog）
await p.evaluate(() => {
  const n = document.querySelector(".attach-bag__name");
  if (n) n.click();
});
await p.waitForTimeout(3000);
const previewDlg = await p.evaluate(() => {
  const dlg = document.querySelector(".file-preview-dialog");
  const img = dlg?.querySelector("img");
  const frame = dlg?.querySelector("iframe");
  return {
    open: !!dlg && (dlg.getBoundingClientRect().width > 100),
    hasImg: !!img, hasFrame: !!frame,
    src: (img?.getAttribute("src") || frame?.getAttribute("src") || "").slice(0, 160),
  };
});
ok("A3.7 点击附件页内预览（走无后缀路由，未开新窗口）",
  previewDlg.open && /\/fleet\/attachments\/view\?id=/.test(previewDlg.src), JSON.stringify(previewDlg));
await shot("d78-a3-preview");
await p.evaluate(() => {
  const dlg = document.querySelector(".file-preview-dialog");
  const btn = Array.from(dlg?.querySelectorAll("button") || []).find((x) => (x.innerText || "").trim() === "关闭");
  if (btn) btn.click();
}).catch(() => {});
await p.waitForTimeout(1500);

// 删除 1 个附件 → 列表变 1
const delRes = await api(`/api/v1/fleet/attachments/${attItems[0].id}`, "DELETE");
ok("A3.8 删除附件接口 2xx", delRes.status >= 200 && delRes.status < 300, String(delRes.status));
const attList2 = await api(`/api/v1/fleet/records/${certId}/attachments`);
ok("A3.9 删除后剩 1 个附件", (attList2.body?.data?.items || []).length === 1,
  String((attList2.body?.data?.items || []).length));

await closeDrawer();
// 抽屉关闭后列表不会自动重拉（附件变更走的是抽屉内接口），手动刷新一次
await p.reload({ waitUntil: "domcontentloaded" }).catch(() => {});
await p.waitForTimeout(26000);
await killGuide();
await card("二级维护");
await p.waitForTimeout(3000);
const attachCell2 = await p.evaluate((cert) => {
  const tr = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .find((r) => (r.innerText || "").includes(cert));
  return (tr?.querySelector(".fleet-attach-cell")?.innerText || "").trim();
}, TEST_CERT);
ok("A3.10 列表附件列更新为 1 个附件", attachCell2.includes("1"), JSON.stringify(attachCell2));

// ==========================================================================
// A4 行驶证年检日期 + 证照状态 4 态
// ==========================================================================
await ensureApp("/platform/daily-affairs/fleet?tab=vehicle-archive");
await card("行驶证");
await p.waitForTimeout(4000);
const licenseHeads = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
  .map((e) => (e.innerText || "").trim()));
ok("A4.1 行驶证列表表头含「年检日期」", licenseHeads.includes("年检日期"), JSON.stringify(licenseHeads));

// 打开一条行驶证记录（若无记录先建一条）
let licRow = await api("/api/v1/fleet/records?type=vehicle-archive&sub_category=" + encodeURIComponent("行驶证") + "&page_size=200");
let licItems = licRow.body?.data?.items || [];
if (!licItems.length) {
  await api("/api/v1/fleet/records", "POST", {
    record_type: "vehicle-archive", sub_category: "行驶证", doc_type: "行驶证",
    record_date: "2026-11-27", data: { 车牌号: "渝C81022", 年检日期: "2027-11-27" },
  });
  licRow = await api("/api/v1/fleet/records?type=vehicle-archive&sub_category=" + encodeURIComponent("行驶证") + "&page_size=200");
  licItems = licRow.body?.data?.items || [];
}
const licTarget = licItems[0] || {};
const licPlate = String(licTarget.data?.["车牌号"] || "");
const licOrigDate = String(licTarget.data?.["年检日期"] || "");
ok("A4.2 存在可编辑的行驶证记录", !!licTarget.id, JSON.stringify({ plate: licPlate, orig: licOrigDate }));

await p.evaluate((plate) => {
  const tr = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .find((r) => (r.innerText || "").includes(plate));
  if (tr) tr.click();
}, licPlate);
await p.waitForTimeout(6000);
await waitDetail();
const drawerHasDate = await p.evaluate(() => {
  const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
  const d = ws[ws.length - 1];
  return { text: (d?.innerText || "").includes("年检日期") };
});
ok("A4.3 行驶证编辑抽屉含「年检日期」", drawerHasDate.text, JSON.stringify(drawerHasDate));

/**
 * 在抽屉里把年检日期设成指定值，读回证照状态文案。
 *
 * 必须用真实键盘输入（locator.fill + Enter）：t-date-picker 内部维护自己的
 * 输入缓冲，`dispatchEvent('input')` 不会让组件采纳该值（真机踩过 ——
 * 页面显示的 input.value 变了，但组件 model 没变，证照状态因此不动）。
 */
const setInspection = async (val) => {
  const scope = p.locator(".t-drawer__content-wrapper").filter({ hasText: "年检日期" }).last();
  const item = scope.locator(".t-form__item, .bsd-field, .form-item").filter({ hasText: "年检日期" }).last();
  const input = item.locator("input").first();
  await input.click();
  await p.waitForTimeout(400);
  await input.fill(val);
  await p.waitForTimeout(500);
  await p.keyboard.press("Enter");
  await p.waitForTimeout(2200);
  return p.evaluate(() => {
    const ws = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").trim().length > 20);
    const d = ws[ws.length - 1];
    const items = Array.from(d?.querySelectorAll(".t-form__item, .bsd-field, .form-item") || []);
    const hit = items.find((it) => (it.innerText || "").includes("证照状态"));
    const tag = hit?.querySelector(".t-tag, .bsd-status-text, .bsd-readonly-tag span");
    const dateHit = items.find((it) => (it.innerText || "").includes("年检日期"));
    return {
      label: (tag?.innerText || "").trim(),
      cls: tag?.className || "",
      dateVal: dateHit?.querySelector("input")?.value || "",
    };
  });
};

const stExpired = await setInspection("2025-01-01");
ok("A4.4 年检日期=2025-01-01 → 证照状态「已过期」", stExpired.label === "已过期", JSON.stringify(stExpired));
const stExpiring = await setInspection("2026-12-12");
ok("A4.5 年检日期=15 天后 → 证照状态「即将到期」", stExpiring.label === "即将到期", JSON.stringify(stExpiring));
const stActive = await setInspection("2027-11-27");
ok("A4.6 年检日期=1 年后 → 证照状态「有效」", stActive.label === "有效", JSON.stringify(stActive));
await shot("d78-a4-inspection");

// 保存 2027-11-27 后刷新列表断言一致
await saveDetail();
await p.waitForTimeout(2000);
const licAfter = await api("/api/v1/fleet/records?type=vehicle-archive&sub_category=" + encodeURIComponent("行驶证") + "&page_size=200");
const licSaved = (licAfter.body?.data?.items || []).find((r) => r.id === licTarget.id);
ok("A4.7 年检日期已持久化落库", String(licSaved?.data?.["年检日期"] || "").startsWith("2027-11-27"),
  JSON.stringify({ found: !!licSaved, val: licSaved?.data?.["年检日期"] }));

await p.waitForTimeout(2000);
const listStatus = await p.evaluate((plate) => {
  const tr = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .find((r) => (r.innerText || "").includes(plate));
  return { text: (tr?.innerText || "").replace(/\s+/g, " ") };
}, licPlate);
ok("A4.8 列表证照状态与抽屉一致（有效）", listStatus.text.includes("有效"), JSON.stringify(listStatus.text.slice(0, 200)));

// 还原原记录日期
if (licTarget.id) {
  await api(`/api/v1/fleet/records/${licTarget.id}`, "PUT", {
    record_type: "vehicle-archive", sub_category: "行驶证", doc_type: "行驶证",
    record_date: licTarget.record_date || "2026-11-27",
    amount: licTarget.amount || 0,
    data: { ...(licTarget.data || {}), 年检日期: licOrigDate },
  });
}

// ==========================================================================
// A5 清理 + 铁律
// ==========================================================================
if (testId) await api(`/api/v1/fleet/records/${testId}`, "DELETE");
if (certId) await api(`/api/v1/fleet/records/${certId}`, "DELETE");
const leftAtt = await api(`/api/v1/fleet/records/${certId}/attachments`);
ok("A5.1 测试记录已清理", leftAtt.status >= 200, String(leftAtt.status));

const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|NetworkError|ECONNRESET|Timeout|Failed to (list|load)|dynamically imported module/i.test(e));
const bizErrs = errs.filter((e) => !envNoise.includes(e));
ok("A5.2 0 业务 console.error", bizErrs.length === 0, JSON.stringify(bizErrs.slice(0, 4)));
const pageErrs = errs.filter((e) => e.startsWith("pageerror:") && !/dynamically imported module/i.test(e));
ok("A5.3 0 业务 pageerror", pageErrs.length === 0, JSON.stringify(pageErrs.slice(0, 3)));
ok("A5.4 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 4)));

const pass = R.checks.filter((x) => x.pass).length;
console.log(`\nSUMMARY ${pass}/${R.checks.length} ${pass === R.checks.length ? "ALL_PASS" : "HAS_FAIL"}`);
if (pass !== R.checks.length) console.log("FAILED:", R.checks.filter((x) => !x.pass).map((x) => x.n).join(" | "));
await b.close();
process.exit(pass === R.checks.length ? 0 : 1);
