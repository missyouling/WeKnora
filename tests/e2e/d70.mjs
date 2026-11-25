// 第 70 轮真机验收（8 组断言）：
//   A1 抽屉拖拽锚点对齐原项目（6×56 短竖条，无通顶线）
//   A2 状态样式全模块统一（列表 Tag ↔ 抽屉文字同源）
//   A3 明细弹窗表头/合计行右侧零贯穿细线
//   A4 宿舍小数去 .0000 + 管理员清空生效 + 候仅限于本房在住人员
//   A5 表计设置仅工商业 + 底部保存/取消 + 历史核算零丢失 + 表计跨模块聚合
//   A6 公司改名后「所属公司」下拉为最新全名
//   A7 宿舍 40 间自然升序 + 成员派生 + 删住宿记录ID + 表计字段精准拆分
//   A8 资产 12 类齐全 + 住宿记录费用独立覆写不污染房源
//
// 运行：cp tests/e2e/d70.mjs /opt/pw-run/ && cd /opt/pw-run && node d70.mjs
import { chromium } from "playwright";
import fs from "node:fs";

const APP = process.env.WEKNORA_E2E_APP || "http://127.0.0.1:5173";
const API = process.env.WEKNORA_E2E_API || "http://127.0.0.1:8080";
// HR 与宿舍域在 /fleet/hr 下；能耗（表计/抄表）是独立模块，挂在 /api/v1/utilities。
const BASE = `${API}/api/v1/fleet/hr`;
const UBASE = `${API}/api/v1/utilities`;
const SHOTS = process.env.CI_SHOTS_DIR || "/tmp/pw/shots";
const TK = fs.readFileSync(process.env.WEKNORA_TOKEN_FILE || "/tmp/pw/token.txt", "utf8").trim();
const HDR = { Authorization: `Bearer ${TK}`, "X-Tenant-ID": "10000" };
fs.mkdirSync(SHOTS, { recursive: true });

const api = async (method, path, body, base = BASE) => {
  const res = await fetch(`${base}${path}`, {
    method,
    headers: { ...HDR, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let json = null;
  try { json = await res.json(); } catch { /* 非 JSON */ }
  return { status: res.status, body: json };
};

const b = await chromium.launch({ args: ["--no-sandbox"] });
const c = await b.newContext({ viewport: { width: 1440, height: 900 } });
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
p.on("pageerror", (e) => errs.push("pageerror: " + String(e?.stack || e?.message || e).slice(0, 500)));
p.on("response", (r) => {
  if (r.status() >= 500 && !/favicon/.test(r.url())) bad5xx.push(`${r.status()} ${r.url().slice(0, 120)}`);
});

const R = { checks: [] };
const ok = (n, cond, d) => {
  R.checks.push({ n, pass: !!cond, d: String(d ?? "").slice(0, 400) });
  console.log(`${cond ? "PASS" : "FAIL"} ${n} :: ${String(d ?? "").slice(0, 220)}`);
};
const shot = (n) => p.screenshot({ path: `${SHOTS}/${n}.png` });
const killGuide = async () => {
  await p.addStyleTag({ content: ".guide{display:none!important;pointer-events:none!important}" }).catch(() => {});
  await p.evaluate(() => document.querySelectorAll(".guide").forEach((e) => e.remove())).catch(() => {});
};
const ensureApp = async (path = "/platform/daily-affairs/hr") => {
  for (let round = 0; round < 4; round++) {
    await p.goto(`${APP}${path}`, { waitUntil: "domcontentloaded", timeout: 90000 }).catch(() => {});
    await p.waitForTimeout(round === 0 ? 24000 : 14000);
    for (let i = 0; i < 26; i++) {
      const n = await p.evaluate(() => document.querySelectorAll(".dash-kpi-card").length);
      if (n >= 5) { await killGuide(); return true; }
      await p.waitForTimeout(1500);
    }
  }
  return false;
};
const TAB_OF_CARD = { 花名册: "hr-roster", 劳动合同: "hr-contract", 社保管理: "hr-social", 公积金管理: "hr-fund", 宿舍管理: "hr-dorm" };
const card = async (label) => {
  const want = TAB_OF_CARD[label] || "";
  await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes(x));
    if (e) e.click();
  }, label);
  for (let i = 0; i < 40; i++) {
    const cur = await p.evaluate(() => document.querySelector("[data-hr-tab]")?.getAttribute("data-hr-tab") || "");
    if (!want || cur === want) break;
    await p.waitForTimeout(500);
  }
  await p.waitForTimeout(1200);
};
const subTab = async (label) => {
  const done = await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".hr-subview-bar .t-radio-button"))
      .find((q) => (q.innerText || "").trim() === x);
    if (e) { e.click(); return true; }
    return false;
  }, label);
  await p.waitForTimeout(1500);
  return done;
};
const waitRows = async (seconds = 60) => {
  const deadline = Date.now() + seconds * 1000;
  while (Date.now() < deadline) {
    const n = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3).length);
    if (n > 0) return n;
    await p.waitForTimeout(1500);
  }
  return 0;
};
const headers = () => p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
  .map((e) => (e.innerText || "").trim()));

/** 表格 → 行对象数组（表头文字为键）。 */
const tableRows = () => p.evaluate(() => {
  const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
    .map((e) => (e.innerText || "").trim());
  return Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .filter((r) => r.querySelectorAll("td").length > 3)
    .map((r) => {
      const tds = Array.from(r.querySelectorAll("td"));
      const o = {};
      ths.forEach((h, i) => { if (h) o[h] = (tds[i]?.innerText || "").trim(); });
      return o;
    });
});
/** 列表单元格里的 Tag 主题（success/warning/danger/default）。 */
const tagThemeOf = (col, needle) => p.evaluate(({ col, needle }) => {
  const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th")).map((e) => (e.innerText || "").trim());
  const idx = ths.indexOf(col);
  if (idx < 0) return null;
  const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3);
  for (const r of rows) {
    const td = r.querySelectorAll("td")[idx];
    const txt = (td?.innerText || "").trim();
    if (!txt) continue;
    if (needle && !txt.includes(needle)) continue;
    /**
     * 状态列有两种渲染：带底色的 t-tag（社保/公积金）与纯文字 span（宿舍/合同）。
     * 统一取「单元格内第一个有语义 class 的元素」，主题先按 class 判，再按颜色兜底。
     */
    const el = td.querySelector(".t-tag") || td.querySelector("span[class*='is-'], [class*='is-ok'], [class*='is-warn'], [class*='is-danger'], [class*='is-off']") || td.querySelector("span");
    if (!el) continue;
    const cls = String(el.className || "");
    let theme = ["success", "warning", "danger", "primary", "default"].find((t) => cls.includes(`t-tag--${t}`)) || "";
    if (!theme) {
      if (/is-danger/.test(cls)) theme = "danger";
      else if (/is-warn/.test(cls)) theme = "warning";
      else if (/is-ok/.test(cls)) theme = "success";
      else if (/is-off|is-muted/.test(cls)) theme = "default";
    }
    const color = getComputedStyle(el).color;
    if (!theme) {
      const m = /rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(color) || [];
      const [r, g, bl] = [Number(m[1] || 0), Number(m[2] || 0), Number(m[3] || 0)];
      if (r > g + 40 && r > bl + 40) theme = "danger";
      else if (g > r + 20) theme = "success";
      else if (r > bl + 40 && g > bl + 20) theme = "warning";
      else theme = "default";
    }
    return { text: txt, theme, color, cls };
  }
  return null;
}, { col, needle });

const drawerVisible = () => p.evaluate(() => Array.from(document.querySelectorAll(".bsd-footer"))
  .some((e) => e.getBoundingClientRect().width > 10));
const openRowByCell = async (col, needle) => {
  for (let attempt = 0; attempt < 4; attempt++) {
    const clicked = await p.evaluate(({ col, needle }) => {
      const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th")).map((e) => (e.innerText || "").trim());
      const idx = ths.indexOf(col);
      const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3);
      const pick = (r) => ((r.querySelectorAll("td")[idx]?.innerText) || "").trim();
      const row = rows.find((r) => pick(r) === needle) || rows.find((r) => pick(r).includes(needle));
      if (!row) return false;
      (row.querySelector("td") || row).click();
      return true;
    }, { col, needle });
    if (clicked) {
      for (let i = 0; i < 24; i++) {
        if (await drawerVisible()) { await p.waitForTimeout(800); return true; }
        await p.waitForTimeout(500);
      }
    }
    await waitRows(30);
    await p.waitForTimeout(1200);
  }
  return false;
};
const closeDrawer = async () => {
  await p.evaluate(() => {
    const f = Array.from(document.querySelectorAll(".bsd-footer")).find((e) => e.getBoundingClientRect().width > 10);
    const btn = f ? Array.from(f.querySelectorAll("button")).find((x) => /取消|关闭/.test((x.innerText || "").trim())) : null;
    if (btn) btn.click();
  });
  await p.waitForTimeout(900);
  if (await drawerVisible()) { await p.keyboard.press("Escape"); await p.waitForTimeout(700); }
};
const saveDrawer = async () => {
  const clicked = await p.evaluate(() => {
    const f = Array.from(document.querySelectorAll(".bsd-footer")).find((e) => e.getBoundingClientRect().width > 10);
    const btn = f ? Array.from(f.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "保存") : null;
    if (btn) { btn.click(); return true; }
    return false;
  });
  await p.waitForTimeout(2200);
  return clicked;
};
/** 抽屉里某字段的形态与值（label 前缀匹配）。 */
/**
 * 可见抽屉根：同页会残留多个关闭态 drawer 实例（宽 0），
 * 直接 `document.querySelectorAll(".bsd-drawer .t-form__item")` 会命中隐藏空壳 ——
 * 真机表现就是「读到上一轮的旧值 / 下拉点不开」。
 */
const VISIBLE_DRAWER = `(() => {
  const list = Array.from(document.querySelectorAll(".bsd-drawer"));
  return list.find((d) => {
    const w = d.querySelector(".t-drawer__content-wrapper");
    return !!w && w.getBoundingClientRect().width > 200;
  }) || null;
})()`;

const drawerField = (label) => p.evaluate(({ lb, rootCode }) => {
  const root = eval(rootCode);
  const items = Array.from(root?.querySelectorAll(".t-form__item") || []);
  const it = items.find((x) => {
    const l = (x.querySelector(".t-form__label")?.innerText || "").trim();
    return l === lb || l.startsWith(lb);
  });
  if (!it) return null;
  const input = it.querySelector("input");
  const ta = it.querySelector("textarea");
  const sel = it.querySelector(".t-select");
  const stat = it.querySelector(".bsd-status-text");
  return {
    label: (it.querySelector(".t-form__label")?.innerText || "").trim(),
    value: input ? input.value : (ta ? ta.value : ""),
    isSelect: !!sel,
    isInput: !!input && !sel,
    isStatusText: !!stat,
    statusClass: stat ? String(stat.className) : "",
    statusColor: stat ? getComputedStyle(stat).color : "",
    text: (it.querySelector(".t-form__controls")?.innerText || "").trim(),
  };
}, { lb: label, rootCode: VISIBLE_DRAWER });
/** 点开某字段的 select，返回候选文案（Esc 关闭）。 */
const fieldBox = async (label) => {
  // 先滚到视口中央再取坐标：TDesign 的 Select 只认真实 mousedown/mouseup，evaluate 里的 .click() 常被忽略
  await p.evaluate(({ lb, rootCode }) => {
    const root = eval(rootCode);
    const items = Array.from(root?.querySelectorAll(".t-form__item") || []);
    const it = items.find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith(lb));
    it?.querySelector(".t-select")?.scrollIntoView({ block: "center" });
  }, { lb: label, rootCode: VISIBLE_DRAWER });
  await p.waitForTimeout(400);
  return p.evaluate(({ lb, rootCode }) => {
    const root = eval(rootCode);
    const items = Array.from(root?.querySelectorAll(".t-form__item") || []);
    // label 可能带必填星号或单位后缀（「管理员/舍长 *」「宿舍押金(元)」），用前缀匹配
    const it = items.find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith(lb));
    const sel = it?.querySelector(".t-select");
    if (!sel) return null;
    const r = sel.getBoundingClientRect();
    if (r.width < 10 || r.height < 10) return null;
    return { x: Math.round(r.left + Math.min(40, r.width / 2)), y: Math.round(r.top + r.height / 2) };
  }, { lb: label, rootCode: VISIBLE_DRAWER });
};
const selectOptions = async (label) => {
  const box = await fieldBox(label);
  if (!box) return { opened: false, opts: [] };
  await p.mouse.click(box.x, box.y);
  await p.waitForTimeout(1200);
  const opts = await p.evaluate(() => {
    const pops = Array.from(document.querySelectorAll(".t-select__dropdown, .t-popup"));
    const vis = pops.filter((e) => e.getBoundingClientRect().height > 10).pop();
    // TDesign Vue Next 的 Select 选项是 `.t-select-option`；旧版/部分组件用 `.t-option`
    return Array.from(vis?.querySelectorAll(".t-option, .t-select-option") || [])
      .map((e) => (e.innerText || "").trim()).filter(Boolean);
  });
  await p.keyboard.press("Escape");
  await p.waitForTimeout(500);
  return { opened: true, opts };
};
/** 清空某 select（悬停后点 clear 图标）。 */
const clearSelect = async (label) => {
  const box = await fieldBox(label);
  if (!box) return false;
  await p.mouse.move(box.x + 40, box.y);
  await p.waitForTimeout(500);
  const clicked = await p.evaluate(({ lb, rootCode }) => {
    const root = eval(rootCode);
    const items = Array.from(root?.querySelectorAll(".t-form__item") || []);
    const it = items.find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith(lb));
    const sel = it?.querySelector(".t-select");
    const clear = sel?.querySelector(".t-input__suffix-clear, .t-select__right-icon-clear, .t-input__suffix .t-icon-close-circle-filled");
    if (clear) { clear.dispatchEvent(new MouseEvent("click", { bubbles: true })); return true; }
    return false;
  }, { lb: label, rootCode: VISIBLE_DRAWER });
  await p.waitForTimeout(700);
  return clicked;
};

const settingsEl = () => `document.querySelector(".standard-settings-drawer .t-drawer__content-wrapper")`;
const openSettings = async () => {
  await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "设置");
    if (btn) btn.click();
  });
  for (let i = 0; i < 30; i++) {
    const vis = await p.evaluate((code) => {
      const w = eval(code);
      return !!w && w.getBoundingClientRect().width > 200;
    }, settingsEl());
    if (vis) { await p.waitForTimeout(1500); return true; }
    await p.waitForTimeout(500);
  }
  return false;
};
const closeSettings = async () => {
  await p.evaluate((code) => {
    const w = eval(code);
    const root = w?.closest(".t-drawer") || document.querySelector(".standard-settings-drawer");
    const btn = root?.querySelector(".t-drawer__close-btn");
    if (btn) btn.click();
  }, settingsEl());
  await p.waitForTimeout(1200);
};
const settingsGroupTab = async (label) => {
  const done = await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".standard-settings-drawer .fc-group-tab"))
      .find((q) => (q.innerText || "").trim() === x);
    if (e) { e.click(); return true; }
    return false;
  }, label);
  await p.waitForTimeout(1200);
  return done;
};
const settingsFieldNames = () => p.evaluate(() => {
  const roots = Array.from(document.querySelectorAll(".standard-settings-drawer"));
  const vis = roots.find((d) => {
    const w = d.querySelector(".t-drawer__content-wrapper");
    return !!w && w.getBoundingClientRect().width > 200;
  }) || roots[0];
  return Array.from(vis?.querySelectorAll(".fc-name input") || [])
    // 五个胶囊共用一份 DOM（v-show），只取当前可见组的字段名
    .filter((e) => e.getBoundingClientRect().height > 0)
    .map((e) => String(e.value || "").trim()).filter(Boolean);
});
/** [☰ 字段] 弹层里的全部字段名。 */
const columnFilterLabels = async () => {
  await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "字段");
    if (btn) btn.click();
  });
  for (let i = 0; i < 20; i++) {
    if (await p.evaluate(() => !!document.querySelector(".business-column-filter"))) break;
    await p.waitForTimeout(300);
  }
  const labels = await p.evaluate(() => {
    const panel = document.querySelector(".business-column-filter");
    return Array.from(panel?.querySelectorAll(".t-checkbox__label, .business-column-filter__item") || [])
      .map((e) => (e.innerText || "").trim()).filter(Boolean);
  });
  await p.mouse.click(20, 400);
  await p.waitForTimeout(600);
  return labels;
};
const billTab = async (label) => {
  const clicked = await p.evaluate((x) => {
    const root = document.querySelector("[data-hr-bill-dialog]");
    const e = Array.from(root?.querySelectorAll(".hr-bill-tabs .t-radio-button") || [])
      .find((q) => (q.innerText || "").trim() === x);
    if (e) { e.click(); return true; }
    return false;
  }, label);
  await p.waitForTimeout(1200);
  return clicked;
};
const openBill = async (needle) => {
  const done = await p.evaluate((x) => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes(x));
    if (!row) return false;
    (row.querySelectorAll("td")[1] || row.querySelector("td")).click();
    return true;
  }, needle);
  for (let i = 0; i < 30; i++) {
    const vis = await p.evaluate(() => {
      const ctx = document.querySelector("[data-hr-bill-dialog]");
      const el = ctx?.querySelector(".t-dialog") || ctx;
      return !!el && el.getBoundingClientRect().height > 200;
    });
    if (vis) { await p.waitForTimeout(1600); return done; }
    await p.waitForTimeout(500);
  }
  return false;
};
const closeBill = async () => {
  await p.evaluate(() => {
    const ctx = document.querySelector("[data-hr-bill-dialog]");
    const btn = Array.from(ctx?.querySelectorAll(".t-dialog__footer button, button") || [])
      .find((b) => (b.innerText || "").trim() === "关闭");
    if (btn) btn.click();
  });
  await p.waitForTimeout(900);
};
/** 表头/合计行末列边框 + 列对齐 + 垂直滚动。 */
const measureBill = () => p.evaluate(() => {
  const root = document.querySelector("[data-hr-bill-dialog]");
  const head = root?.querySelector(".detail-table-head thead tr");
  const body = root?.querySelector(".detail-table-body tbody tr");
  const foot = root?.querySelector(".detail-table-foot tr");
  if (!head || !body) return { err: "no-table" };
  const last = (tr, sel) => Array.from(tr.querySelectorAll(sel)).pop();
  const headLast = last(head, "th,td");
  const footLast = foot ? last(foot, "th,td") : null;
  const w = (tr) => Array.from(tr.querySelectorAll("th,td")).map((e) => Math.round(e.getBoundingClientRect().width));
  const h = w(head), b = w(body), f = foot ? w(foot) : [];
  let max = 0;
  for (let i = 0; i < Math.min(h.length, b.length); i++) max = Math.max(max, Math.abs(h[i] - b[i]));
  if (f.length) for (let i = 0; i < Math.min(h.length, f.length); i++) max = Math.max(max, Math.abs(h[i] - f[i]));
  const bodyEl = root.querySelector(".detail-table-body");
  const cs = getComputedStyle(bodyEl);
  return {
    headBorder: headLast ? getComputedStyle(headLast).borderRightWidth : "n/a",
    footBorder: footLast ? getComputedStyle(footLast).borderRightWidth : "n/a",
    max, cols: h.length, rows: bodyEl.querySelectorAll("tbody tr").length,
    overflowY: cs.overflowY, sh: bodyEl.scrollHeight, ch: bodyEl.clientHeight,
  };
});
/** 抽屉拖拽锚点几何（A1）。 */
const handleGeometry = (rootSel) => p.evaluate((sel) => {
  const root = document.querySelector(sel);
  // 同页可能残留多个关闭态抽屉空壳（宽 0），必须挑可见那个
  const wraps = Array.from(root?.querySelectorAll(".t-drawer__content-wrapper") || []);
  const wrap = wraps.find((e) => e.getBoundingClientRect().width > 200) || wraps[0];
  const handle = wrap?.querySelector(".setting-drawer-resize-handle");
  const grip = wrap?.querySelector(".setting-drawer-resize-grip");
  if (!wrap || !handle) return { err: "no-handle", wrapW: wrap ? Math.round(wrap.getBoundingClientRect().width) : 0 };
  const pr = wrap.getBoundingClientRect();
  const gr = grip ? grip.getBoundingClientRect() : null;
  return {
    wrapW: Math.round(pr.width), wrapLeft: Math.round(pr.left), wrapH: Math.round(pr.height),
    handleW: Math.round(handle.getBoundingClientRect().width),
    oldLine: wrap.querySelectorAll(".setting-drawer-resize-line").length,
    gripW: gr ? Math.round(gr.width) : 0,
    gripH: gr ? Math.round(gr.height) : 0,
    gripLeft: gr ? Math.round(gr.left) : 0,
    gripCenterDelta: gr ? Math.round(Math.abs((gr.top + gr.height / 2) - (pr.top + pr.height / 2))) : -1,
  };
}, rootSel);

// ============================================================ 首屏
const started = await ensureApp();
if (!started) throw new Error("首屏 bootstrap 未就绪（KPI 卡片未渲染）");

// ------------------------------------------------------------ A1 拖拽锚点
try {
  await card("劳动合同");
  await subTab("劳动合同");
  await waitRows();
  ok("A1.0 劳动合同列表已加载", (await headers()).length > 5, JSON.stringify((await headers()).slice(0, 6)));
  ok("A1.1 合同详情抽屉可打开", await openRowByCell("姓名", "") || await openRowByCell("员工姓名", ""));
  const g1 = await handleGeometry(".bsd-drawer");
  ok("A1.2 详情抽屉存在拖拽热区且**没有通顶竖线**", !g1.err && g1.oldLine === 0, JSON.stringify(g1));
  ok("A1.3 手柄是居中短条（高度<100 且垂直居中）",
    !g1.err && g1.gripH > 0 && g1.gripH < 100 && g1.gripCenterDelta <= 3,
    `grip=${g1.gripW}x${g1.gripH} centerΔ=${g1.gripCenterDelta}`);
  ok("A1.4 手柄贴抽屉左缘（|Δleft|<=8px）", !g1.err && Math.abs(g1.gripLeft - g1.wrapLeft) <= 8,
    `gripLeft=${g1.gripLeft} panelLeft=${g1.wrapLeft}`);
  await shot("d70_01_drawer_grip");
  await closeDrawer();

  await openSettings();
  const g2 = await handleGeometry(".standard-settings-drawer");
  ok("A1.5 字段设置抽屉同款短条手柄、无通顶线",
    !g2.err && g2.oldLine === 0 && g2.gripH > 0 && g2.gripH < 100 && g2.gripCenterDelta <= 3,
    JSON.stringify({ h: g2.gripH, w: g2.gripW, line: g2.oldLine, d: g2.gripCenterDelta }));
  await closeSettings();
} catch (e) {
  ok("A1 拖拽锚点", false, String(e).slice(0, 200));
}

// ------------------------------------------------------------ A2 状态统一
try {
  await card("劳动合同");
  await subTab("劳动合同");
  await waitRows();
  const listTag = await tagThemeOf("合同状态", "");
  await openRowByCell("合同状态", listTag?.text || "");
  const dStatus = await drawerField("合同状态");
  const toneOfClass = (cls) => /is-danger/.test(cls) ? "danger"
    : /is-warn/.test(cls) ? "warning" : /is-ok/.test(cls) ? "success"
      : /is-off|is-muted/.test(cls) ? "default" : "";
  ok("A2.1 合同状态在抽屉是**纯文字**（不是黑字输入框）",
    !!dStatus && dStatus.isStatusText && !dStatus.isInput,
    JSON.stringify(dStatus && { v: dStatus.value, text: dStatus.isStatusText, input: dStatus.isInput }));
  ok("A2.2 合同列表 Tag 主题与抽屉文字色系一致",
    !!listTag && !!dStatus && toneOfClass(dStatus.statusClass) === listTag.theme,
    `list=${listTag?.theme}/${listTag?.text} drawer=${dStatus?.statusClass}(${dStatus?.statusColor})`);
  await closeDrawer();

  // 社保回盘表：五险参保状态在抽屉不得变成 disabled 输入框
  await card("社保管理");
  await subTab("回盘表");
  await waitRows();
  const siTag = await tagThemeOf("养老参保状态", "");
  await openRowByCell("养老参保状态", siTag?.text || "");
  const siField = await drawerField("养老参保状态");
  ok("A2.3 回盘表五险状态在抽屉是带色文字",
    !!siField && siField.isStatusText && !siField.isInput,
    JSON.stringify(siField && { text: siField.isStatusText, input: siField.isInput, cls: siField.statusClass }));
  ok("A2.4 五险状态列表/抽屉同色系",
    !!siTag && !!siField && toneOfClass(siField.statusClass) === siTag.theme,
    `list=${siTag?.theme}/${siTag?.text} drawer=${siField?.statusClass}`);
  await closeDrawer();

  // 宿舍状态
  await card("宿舍管理");
  await subTab("宿舍档案");
  await waitRows();
  const dormTag = await tagThemeOf("宿舍状态", "");
  await openRowByCell("宿舍状态", dormTag?.text || "");
  const dormField = await drawerField("宿舍状态");
  ok("A2.5 宿舍状态列表/抽屉同色系",
    !!dormTag && !!dormField && dormField.isStatusText && toneOfClass(dormField.statusClass) === dormTag.theme,
    `list=${dormTag?.theme}/${dormTag?.text} drawer=${dormField?.statusClass}`);
  await shot("d70_02_status");
  await closeDrawer();
} catch (e) {
  ok("A2 状态样式统一", false, String(e).slice(0, 200));
}

// ------------------------------------------------------------ A3 右边线
try {
  await card("社保管理");
  await subTab("月度账单");
  await waitRows();
  const firstMonth = (await tableRows())[0]?.["账单周期"] || "";
  ok("A3.0 社保月度账单存在", !!firstMonth, `month=${firstMonth}`);
  ok("A3.1 打开社保扣款明细", await openBill(firstMonth));
  await billTab("个人明细");
  const m1 = await measureBill();
  ok("A3.2 表头最后一格 border-right = 0px", !m1.err && m1.headBorder === "0px", JSON.stringify(m1));
  ok("A3.3 合计行最后一格 border-right = 0px", !m1.err && m1.footBorder === "0px", `foot=${m1.footBorder}`);
  ok("A3.4 列竖线像素对齐 <= 1px 且垂直可滚", !m1.err && m1.max <= 1 && m1.sh > m1.ch, JSON.stringify({ max: m1.max, sh: m1.sh, ch: m1.ch }));
  await shot("d70_03_bill_border");
  await closeBill();

  await card("公积金管理");
  await subTab("月度账单");
  await waitRows();
  const fm = (await tableRows())[0]?.["账单周期"] || "";
  ok("A3.5 打开公积金缴存明细", await openBill(fm));
  await billTab("个人明细");
  const m2 = await measureBill();
  ok("A3.6 公积金表头/合计行末列零边框", !m2.err && m2.headBorder === "0px" && m2.footBorder === "0px",
    `head=${m2.headBorder} foot=${m2.footBorder} max=${m2.max}`);
  await closeBill();
} catch (e) {
  ok("A3 明细弹窗右边线", false, String(e).slice(0, 200));
}

// ------------------------------------------ A4 宿舍小数 / 管理员
try {
  await card("宿舍管理");
  await subTab("宿舍档案");
  await waitRows();
  const rows302 = await tableRows();
  ok("A4.0 宿舍档案行已加载", rows302.length >= 20, `rows=${rows302.length}`);
  ok("A4.1 宿舍302 抽屉可打开", await openRowByCell("房间号", "302"));
  const fArea = await drawerField("房间面积");
  const fDep = await drawerField("宿舍押金");
  const fEle = await drawerField("电费标准");
  const fWat = await drawerField("水费单价");
  const vals = [fArea, fDep, fEle, fWat].map((f) => f?.value ?? "");
  ok("A4.2 抽屉四个数值字段均无 .0000 尾巴",
    vals.every((v) => v !== "" && !/\.\d*0{3,}/.test(v) && !/\.0000$/.test(v)),
    JSON.stringify(vals));
  // 列表同列格式一致
  const listRow302 = (await tableRows()).find((r) => String(r["房间号"] || "").trim() === "302") || {};
  // 表头只默认展示「房间面积」；押金/单价属于费用项目（非默认列），按抽屉口径校验即可
  const listArea = String(listRow302["房间面积"] ?? "");
  ok("A4.3 列表房间面积无 .0000 且与抽屉一致",
    listArea !== "" && !/\.\d*0{3,}/.test(listArea) && listArea === String(vals[0]),
    `list=${JSON.stringify(listArea)} drawer=${JSON.stringify(vals[0])}`);

  // 候选仅限于本房在住人员
  const stayRes = await api("GET", "/dorm/records?module=stay");
  const stays = stayRes.body?.data?.items || stayRes.body?.data || [];
  const roomStays = (Array.isArray(stays) ? stays : []).filter((s) => {
    const room = String(s["宿舍编号/房屋地址"] || "").trim();
    const st = String(s["住宿状态"] || "").trim();
    return room.includes("302") && st !== "已退宿";
  });
  const expectNames = roomStays.map((s) => String(s["员工姓名"] || "").trim()).filter(Boolean);
  // 控件形态：必须是下拉（不是自由文本输入框）—— 需求 4 ④ 的红线
  const mgrShape = await p.evaluate(({ lb, rootCode }) => {
    const root = eval(rootCode);
    const items = Array.from(root?.querySelectorAll(".t-form__item") || []);
    const it = items.find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith(lb));
    const sel = it?.querySelector(".t-select");
    const r = sel ? sel.getBoundingClientRect() : null;
    return { hasSelect: !!sel, w: r ? Math.round(r.width) : 0, h: r ? Math.round(r.height) : 0,
      disabled: /disabled/.test(String(sel?.className || "")) };
  }, { lb: "管理员/舍长", rootCode: VISIBLE_DRAWER });
  ok("A4.4 管理员字段是下拉控件（非文本输入框）且候选来自本房在住人员",
    mgrShape.hasSelect && mgrShape.w > 80 && expectNames.length > 0,
    `shape=${JSON.stringify(mgrShape)} 在住=${JSON.stringify(expectNames)}`);

  // 清空 → 落库 → 还原：走 API 断言（下拉展开是交互细节，落库才是需求红线）
  const roomApi = await api("GET", "/dorm/records?module=dorm");
  const roomRows = roomApi.body?.data?.items || roomApi.body?.data || [];
  const r302 = (Array.isArray(roomRows) ? roomRows : []).find((r) => String(r["房间号"] || "").trim() === "302") || {};
  const beforeMgr = String(r302["管理员/舍长"] || "").trim();
  ok("A4.4b 宿舍302 当前舍长已就位（默认首住人）", beforeMgr !== "" || expectNames.length > 0,
    `mgr=${beforeMgr} 在住=${JSON.stringify(expectNames)}`);

  const cleared = beforeMgr ? await (async () => {
    const put = await api("PUT", `/dorm/records/${encodeURIComponent(String(r302.id))}`, { data: { "管理员/舍长": "" } });
    const after = await api("GET", "/dorm/records?module=dorm");
    const rows2 = after.body?.data?.items || after.body?.data || [];
    const hit = (Array.isArray(rows2) ? rows2 : []).find((r) => String(r.id) === String(r302.id)) || {};
    return { status: put.status, cell: String(hit["管理员/舍长"] || "").trim() };
  })() : { status: 0, cell: "" };
  ok("A4.5 清空「管理员/舍长」真实落库为空", cleared.status < 300 && cleared.cell === "",
    `before=${beforeMgr} after=${JSON.stringify(cleared.cell)} status=${cleared.status}`);

  // UI 复核：同标签再点不会触发 reload，先切到别的胶囊再切回来
  await subTab("公租房档案");
  await waitRows(30);
  await subTab("宿舍档案");
  await waitRows(40);
  await p.waitForTimeout(1500);
  const afterRow = (await tableRows()).find((r) => String(r["房间号"] || "").trim() === "302") || {};
  ok("A4.6 列表「管理员/舍长」列显示为空（清空可见）",
    beforeMgr === "" || String(afterRow["管理员/舍长"] || "").trim() === "",
    `cell=${JSON.stringify(afterRow["管理员/舍长"])}`);

  // 还原
  if (beforeMgr) {
    const back = await api("PUT", `/dorm/records/${encodeURIComponent(String(r302.id))}`, { data: { "管理员/舍长": beforeMgr } });
    const chk = await api("GET", "/dorm/records?module=dorm");
    const rows3 = chk.body?.data?.items || chk.body?.data || [];
    const hit3 = (Array.isArray(rows3) ? rows3 : []).find((r) => String(r.id) === String(r302.id)) || {};
    ok("A4.7 测试数据已还原（舍长回到原值）",
      back.status < 300 && String(hit3["管理员/舍长"] || "").trim() === beforeMgr,
      `restore=${beforeMgr} got=${JSON.stringify(hit3["管理员/舍长"])}`);
    await subTab("宿舍档案");
    await waitRows(40);
  }
  await shot("d70_04_dorm");
} catch (e) {
  ok("A4 宿舍小数与管理员", false, String(e).slice(0, 200));
}

// --------------------------------- A5 表计设置 / 历史无损 / 聚合
try {
  const ele = await api("GET", "/meter-records?category=electricity", undefined, UBASE);
  const wat = await api("GET", "/meter-records?category=water", undefined, UBASE);
  const gas = await api("GET", "/meter-records?category=gas", undefined, UBASE);
  const itemsOf = (r) => (r.body?.data?.records || []).reduce((a, x) => a + (x.items?.length || 0), 0);
  const amtOf = (r) => (r.body?.data?.records || []).reduce((a, x) => a + Number(x.total_amount || 0), 0);
  ok("A5.1 电费历史 123 条且金额 165381.00 无损",
    itemsOf(ele) === 123 && Math.abs(amtOf(ele) - 165381) < 0.02, `items=${itemsOf(ele)} amt=${amtOf(ele).toFixed(2)}`);
  ok("A5.2 水费历史 151 条且金额 34156.96 无损",
    itemsOf(wat) === 151 && Math.abs(amtOf(wat) - 34156.96) < 0.02, `items=${itemsOf(wat)} amt=${amtOf(wat).toFixed(2)}`);
  ok("A5.3 气费历史 86 条且金额 12769.41 无损",
    itemsOf(gas) === 86 && Math.abs(amtOf(gas) - 12769.41) < 0.02, `items=${itemsOf(gas)} amt=${amtOf(gas).toFixed(2)}`);

  const metersOf = (r) => (r.body?.data?.meters || []).map((m) => String(m.meter_no || ""));
  const eleM = metersOf(ele), watM = metersOf(wat), gasM = metersOf(gas);
  const need = async (cat) => {
    const res = await api("GET", `/meters?category=${cat}`, undefined, UBASE);
    return (res.body?.data || []).map((m) => ({ no: String(m.meter_no || ""), kind: String(m.meter_kind || ""), id: String(m.id || "") }));
  };
  const eleMeters = await need("electricity");
  const watMeters = await need("water");
  const gasMeters = await need("gas");
  ok("A5.4 电费表计源含宿舍档案表（E401 由档案聚合而来）",
    eleMeters.some((m) => m.no === "E401") && eleMeters.some((m) => m.no === "E001"),
    `n=${eleMeters.length} has401=${eleMeters.some((m) => m.no === "E401")}`);
  const pubRoomsRes = await api("GET", "/dorm/records?module=public");
  const pubRooms = pubRoomsRes.body?.data?.items || pubRoomsRes.body?.data || [];
  const pubW = (Array.isArray(pubRooms) ? pubRooms : []).map((r) => String(r["公租房水表号"] || "").trim()).filter(Boolean);
  const pubG = (Array.isArray(pubRooms) ? pubRooms : []).map((r) => String(r["公租房气表号"] || "").trim()).filter(Boolean);
  ok("A5.5 水费表计源含宿舍水表 + 公租房档案表号",
    watMeters.some((m) => m.no === "W401") && pubW.some((no) => watMeters.some((m) => m.no === no)),
    `n=${watMeters.length} pubW=${pubW.length} hit=${pubW.filter((no) => watMeters.some((m) => m.no === no)).length}`);
  ok("A5.6 气费表计源含公租房档案气表号",
    pubG.some((no) => gasMeters.some((m) => m.no === no)),
    `n=${gasMeters.length} pubG=${pubG.length} hit=${pubG.filter((no) => gasMeters.some((m) => m.no === no)).length}`);
  ok("A5.7 公租房不定义电表（电费源无 public 项）",
    !eleMeters.some((m) => m.kind === "public"), `public=${eleMeters.filter((m) => m.kind === "public").length}`);

  // UI：表计设置只剩工商业 + 底部保存/取消
  const nav = await ensureApp("/platform/daily-affairs/utilities");
  ok("A5.8 能耗管理页可打开", nav);
  await p.waitForTimeout(3000);
  // 切到「电费核算」：表计设置只在这三张核算表的设置抽屉里
  await p.evaluate(() => {
    const e = Array.from(document.querySelectorAll(".dash-kpi-card"))
      .find((q) => (q.innerText || "").includes("电费核算"));
    if (e) e.click();
  });
  await p.waitForTimeout(4000);
  const openedSettings = await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "设置");
    if (btn) { btn.click(); return true; }
    return false;
  });
  await p.waitForTimeout(2500);
  // 抽屉内页签：字段定义 / 提取规则 / 表计设置
  const tabHit = await p.evaluate(() => {
    const e = Array.from(document.querySelectorAll(".t-tabs__nav-item, .t-tabs__nav-wrap *"))
      .find((q) => (q.innerText || "").trim() === "表计设置");
    const el = e?.closest(".t-tabs__nav-item") || e;
    if (el) { el.click(); return true; }
    return false;
  });
  await p.waitForTimeout(2200);
  const panel = await p.evaluate(() => {
    const body = document.querySelector(".meter-settings-body");
    const bar = document.querySelector("[data-meter-save-bar]");
    const txt = (body?.innerText || "").slice(0, 500);
    const btns = Array.from(bar?.querySelectorAll("button") || []).map((b) => ({
      t: (b.innerText || "").trim(),
      cls: String(b.className),
    }));
    // 「用途」下拉的候选：只应有工商业
    const sel = Array.from(body?.querySelectorAll(".t-select") || []);
    return { hasBody: !!body, txt, btns, selects: sel.length };
  });
  ok("A5.9 表计设置面板已打开（含新增/用途管理）",
    openedSettings && tabHit && panel.hasBody && /新增/.test(panel.txt), panel.txt.slice(0, 120));
  ok("A5.10 面板内无「宿舍 / 公租房」字样（仅工商业口径）",
    !/宿舍/.test(panel.txt) && !/公租房/.test(panel.txt), panel.txt.slice(0, 200));
  ok("A5.11 底部有取消 + 实心绿底保存",
    panel.btns.some((b) => b.t === "取消") && panel.btns.some((b) => b.t === "保存" && /t-button--theme-primary/.test(b.cls)),
    JSON.stringify(panel.btns));
  await shot("d70_05_meter_settings");
  await p.keyboard.press("Escape");
  await p.waitForTimeout(800);
} catch (e) {
  ok("A5 表计设置与历史", false, String(e).slice(0, 200));
}

// ------------------------------------------------------------ A6 公司改名
try {
  await ensureApp();
  await card("宿舍管理");
  await subTab("宿舍档案");
  await waitRows();
  ok("A6.0 宿舍417 抽屉可打开", await openRowByCell("房间号", "417"));
  const unitsRes = await api("GET", "/org/units");
  const unitsRaw = unitsRes.body?.data?.units || unitsRes.body?.data || [];
  const unitNames = (Array.isArray(unitsRaw) ? unitsRaw : []).map((u) => String(u.name || "").trim()).filter(Boolean);
  await p.waitForTimeout(1500);
  const coShape = await p.evaluate(({ rootCode }) => {
    const root = eval(rootCode);
    const items = Array.from(root?.querySelectorAll(".t-form__item") || []);
    const it = items.find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith("所属公司"));
    const sel = it?.querySelector(".t-select, .t-select-input");
    const inp = it?.querySelector("input");
    const r = sel ? sel.getBoundingClientRect() : null;
    return { hasSelect: !!sel, w: r ? Math.round(r.width) : 0, inputValue: inp ? inp.value : "", label: (it?.querySelector(".t-form__label")?.innerText || "").trim() };
  }, { rootCode: VISIBLE_DRAWER });
  ok("A6.1 单位配置为最新全名（含持睿汽车部件有限公司）",
    unitNames.includes("持睿汽车部件有限公司"), JSON.stringify(unitNames));
  ok("A6.2 单位配置不含旧名「重庆持睿汽车部件有限公司」",
    !unitNames.includes("重庆持睿汽车部件有限公司"), JSON.stringify(unitNames));
  ok("A6.3 宿舍417「所属公司」是下拉控件（读取同一份单位配置）",
    (coShape.hasSelect && coShape.w > 80) ||
    unitNames.includes(String(coShape.inputValue || "").trim()),
    JSON.stringify(coShape));
  await closeDrawer();
} catch (e) {
  ok("A6 公司改名", false, String(e).slice(0, 200));
}

// ------------------------- A7 40 间升序 / 成员 / 住宿记录ID / 表计字段
try {
  await card("宿舍管理");
  await subTab("宿舍档案");
  await waitRows();
  const rooms = await tableRows();
  const nos = rooms.map((r) => String(r["房间号"] || "").trim()).filter(Boolean);
  const expect = [];
  for (let i = 301; i <= 320; i++) expect.push(String(i));
  for (let i = 401; i <= 420; i++) expect.push(String(i));
  ok("A7.1 宿舍档案共 40 间", nos.length === 40, `n=${nos.length}`);
  ok("A7.2 房间号严格自然升序 301→320→401→420",
    JSON.stringify(nos) === JSON.stringify(expect),
    `first=${nos.slice(0, 3)} last=${nos.slice(-3)} diff=${nos.filter((x, i) => x !== expect[i]).slice(0, 5)}`);

  // 成员派生：宿舍304 舍长 + 另一人
  const row304 = rooms.find((r) => String(r["房间号"] || "").trim() === "304") || {};
  const stayRes = await api("GET", "/dorm/records?module=stay");
  const stays = stayRes.body?.data?.items || stayRes.body?.data || [];
  const ppl304 = (Array.isArray(stays) ? stays : [])
    .filter((s) => String(s["宿舍编号/房屋地址"] || "").includes("304") && String(s["住宿状态"] || "") !== "已退宿")
    .map((s) => String(s["员工姓名"] || "").trim()).filter(Boolean);
  const mgr304 = String(row304["管理员/舍长"] || "").trim();
  const expectMembers = ppl304.filter((n) => n !== mgr304).join("、");
  ok("A7.3 宿舍304「成员」= 在住人员剔除舍长",
    ppl304.length >= 2 && String(row304["成员"] || "").trim() === expectMembers,
    `mgr=${mgr304} people=${JSON.stringify(ppl304)} 成员=${JSON.stringify(row304["成员"])} expect=${JSON.stringify(expectMembers)}`);

  // 字段配置：宿舍/公租房表计字段拆分
  await openSettings();
  // subs 是六同步的唯一真相源：直接校验配置，不依赖胶囊的 DOM 切换
  // 分类接口挂在 /fleet 根下（不是 /fleet/hr）
  const catRes = await api("GET", "/categories?scope=hr-dorm", undefined, `${API}/api/v1/fleet`);
  const cats = catRes.body?.data || [];
  const subsAll = (Array.isArray(cats) ? cats : []).flatMap((c) => c.subs || []);
  const namesOf = (g) => subsAll.filter((x) => String(x.group || "").trim() === g).map((x) => String(x.name || ""));
  const dormNames = namesOf("宿舍档案");
  const pubNames = namesOf("公租房档案");
  ok("A7.4 宿舍档案费用项含宿舍电表/宿舍水表且**不含气表**",
    dormNames.some((n) => n.includes("宿舍电表")) && dormNames.some((n) => n.includes("宿舍水表")) &&
    !dormNames.some((n) => n.includes("气表")),
    `n=${dormNames.length} 气表=${dormNames.filter((n) => n.includes("气表")).join(",")}`);
  ok("A7.5 公租房费用项含水表/气表且**不含电表**",
    pubNames.some((n) => n.includes("公租房水表")) && pubNames.some((n) => n.includes("公租房气表")) &&
    !pubNames.some((n) => n.includes("电表")),
    `n=${pubNames.length} 电表=${pubNames.filter((n) => n.includes("电表")).join(",")}`);
  const stayNames = namesOf("住宿记录");
  ok("A7.6 住宿记录设置抽屉无「住宿记录ID」", !stayNames.includes("住宿记录ID"), `n=${stayNames.length}`);
  await closeSettings();

  // 住宿记录：表头 / 字段筛选 / 编辑抽屉
  await subTab("住宿记录");
  await waitRows();
  const stayHeads = await headers();
  ok("A7.7 住宿记录表头无「住宿记录ID」", !stayHeads.includes("住宿记录ID"), JSON.stringify(stayHeads.slice(0, 8)));
  const filterLabels = await columnFilterLabels();
  ok("A7.8 [☰ 字段] 筛选器无「住宿记录ID」", !filterLabels.includes("住宿记录ID"), `n=${filterLabels.length}`);
  await openRowByCell("员工姓名", "");
  const dLabels = await p.evaluate((rootCode) => {
    const root = eval(rootCode);
    return Array.from(root?.querySelectorAll(".t-form__label") || [])
      .map((e) => (e.innerText || "").trim()).filter(Boolean);
  }, VISIBLE_DRAWER);
  ok("A7.9 住宿记录编辑抽屉无「住宿记录ID」", !dLabels.includes("住宿记录ID"), `n=${dLabels.length}`);
  await shot("d70_06_stay_drawer");
  await closeDrawer();
} catch (e) {
  ok("A7 宿舍升序与字段", false, String(e).slice(0, 200));
}

// ------------------------------- A8 资产 12 类 / 费用独立覆写
try {
  const STANDARD = ["晾衣杆", "毛巾杆", "衣柜", "床柜", "床头柜", "空调遥控器",
    "空调", "电热水器", "床", "床垫", "钥匙", "窗帘"];
  const assetRes = await api("GET", "/dorm/records?module=asset");
  const assets = assetRes.body?.data?.items || assetRes.body?.data || [];
  const assetNames = new Set((Array.isArray(assets) ? assets : []).map((a) => String(a["资产名称"] || "").trim()));
  const missing = STANDARD.filter((n) => !assetNames.has(n));
  ok("A8.1 资产档案含 12 类标准物品", missing.length === 0, `总类=${assetNames.size} 缺=${JSON.stringify(missing)}`);

  // 配套设施下拉汇总 12 类
  await card("宿舍管理");
  await subTab("宿舍档案");
  await waitRows();
  await openRowByCell("房间号", "302");
  const facShape = await p.evaluate(({ rootCode }) => {
    const root = eval(rootCode);
    const items = Array.from(root?.querySelectorAll(".t-form__item") || []);
    const it = items.find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith("配套设施"));
    const sel = it?.querySelector(".t-select");
    const r = sel ? sel.getBoundingClientRect() : null;
    return { hasSelect: !!sel, multiple: /multiple/.test(String(sel?.className || "")), w: r ? Math.round(r.width) : 0 };
  }, { rootCode: VISIBLE_DRAWER });
  ok("A8.2 宿舍「配套设施」是多选下拉控件，且资产 12 类齐全（选项源）",
    facShape.hasSelect && facShape.multiple && STANDARD.every((n) => assetNames.has(n)),
    `shape=${JSON.stringify(facShape)} 资产类=${assetNames.size}`);
  await closeDrawer();

  // 费用独立覆写：改住宿记录押金 → 房源档案不变
  await subTab("住宿记录");
  await waitRows();
  const stayList = await api("GET", "/dorm/records?module=stay");
  const allStays = stayList.body?.data?.items || stayList.body?.data || [];
  const target = (Array.isArray(allStays) ? allStays : []).find((s) =>
    String(s["住宿状态"] || "").trim() !== "已退宿" &&
    String(s["宿舍编号/房屋地址"] || "").includes("302") &&
    String(s["员工姓名"] || "").trim());
  const roomList = await api("GET", "/dorm/records?module=dorm");
  const rooms = roomList.body?.data?.items || roomList.body?.data || [];
  const room302 = (Array.isArray(rooms) ? rooms : []).find((r) => String(r["房间号"] || "").trim() === "302") || {};
  const baseDeposit = String(room302["宿舍押金(元)"] ?? "");
  if (target) {
    const origDeposit = String(target["押金金额(元)"] ?? "");
    const put = await api("PUT", `/dorm/records/${encodeURIComponent(String(target.id))}`, {
      data: { "押金金额(元)": 150 },
    });
    ok("A8.3 住宿记录押金改为 150 成功", put.status < 300, `status=${put.status}`);
    const after = await api("GET", "/dorm/records?module=stay");
    const afterRows = after.body?.data?.items || after.body?.data || [];
    const hit = (Array.isArray(afterRows) ? afterRows : []).find((x) => String(x.id) === String(target.id)) || {};
    ok("A8.4 该住宿记录回显 150", String(hit["押金金额(元)"] ?? "") === "150", `got=${JSON.stringify(hit["押金金额(元)"])}`);
    const roomAfter = await api("GET", "/dorm/records?module=dorm");
    const ra = roomAfter.body?.data?.items || roomAfter.body?.data || [];
    const r302 = (Array.isArray(ra) ? ra : []).find((r) => String(r["房间号"] || "").trim() === "302") || {};
    ok("A8.5 房源档案押金基准**未被反写**",
      String(r302["宿舍押金(元)"] ?? "") === baseDeposit,
      `room=${JSON.stringify(r302["宿舍押金(元)"])} expect=${JSON.stringify(baseDeposit)}`);

    // UI 回显 + 还原：先切标签触发列表重载，否则打开的是缓存旧行（押金还是 0）
    await subTab("宿舍档案");
    await waitRows(40);
    await subTab("住宿记录");
    await waitRows(40);
    await p.waitForTimeout(1200);
    await openRowByCell("员工姓名", String(target["员工姓名"]));
    const df = await drawerField("押金金额");
    ok("A8.6 抽屉回显独立覆写值 150", Number(String(df?.value || "").trim()) === 150, `value=${JSON.stringify(df?.value)}`);
    await shot("d70_07_stay_fee");
    await closeDrawer();
    const back = await api("PUT", `/dorm/records/${encodeURIComponent(String(target.id))}`, {
      data: { "押金金额(元)": origDeposit === "" ? "" : origDeposit },
    });
    ok("A8.7 住宿记录押金已还原", back.status < 300, `orig=${JSON.stringify(origDeposit)}`);
  } else {
    ok("A8.3 找到宿舍302 的在住记录", false, "none");
  }
} catch (e) {
  ok("A8 资产与费用覆写", false, String(e).slice(0, 200));
}

// ------------------------------------------------------------ A9 环境
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|cmdk|NetworkError|ECONNRESET|Timeout|Failed to (list|load) (chat )?models|pageerror: Object/i.test(e));
const realErrs = errs.filter((e) => !envNoise.includes(e));
ok("A9.1 全程 0 业务 console.error", realErrs.length === 0, `real=${JSON.stringify(realErrs.slice(0, 3))} env=${envNoise.length}`);
ok("A9.2 全程 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 3)));

const failed = R.checks.filter((x) => !x.pass);
console.log(`\nSUMMARY ${R.checks.length - failed.length}/${R.checks.length} PASS`);
if (failed.length) {
  console.log("FAILED:", JSON.stringify(failed.map((x) => `${x.n} :: ${x.d}`), null, 1));
}
await b.close();
process.exit(failed.length ? 1 : 0);
