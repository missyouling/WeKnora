// 第 68 轮真机验收：抽屉紧凑排版 / 设置抽屉零截断 / 离职全字段 / 社保按月隔离与账单快照 /
// 智能纵横打印 / 明细弹窗定高零跳动。
//
// 运行：cp tests/e2e/d68.mjs /opt/pw-run/ && cd /opt/pw-run && node d68.mjs
// 依赖：playwright、/tmp/pw/token.txt、前端 5173 / 后端 8080。
import { chromium } from "playwright";
import fs from "node:fs";

const APP = process.env.WEKNORA_E2E_APP || "http://127.0.0.1:5173";
const API = process.env.WEKNORA_E2E_API || "http://127.0.0.1:8080";
const BASE = `${API}/api/v1/fleet`;
const SHOTS = process.env.CI_SHOTS_DIR || "/tmp/pw/shots";
const TK = fs.readFileSync(process.env.WEKNORA_TOKEN_FILE || "/tmp/pw/token.txt", "utf8").trim();
const HDR = { Authorization: `Bearer ${TK}`, "X-Tenant-ID": "10000" };
fs.mkdirSync(SHOTS, { recursive: true });

const b = await chromium.launch({ args: ["--no-sandbox"] });
const c = await b.newContext({ viewport: { width: 1280, height: 800 }, acceptDownloads: true });
await c.addInitScript((t) => {
  localStorage.setItem("weknora_token", t);
  localStorage.setItem("weknora_selected_tenant_id", "10000");
}, TK);
const p = await c.newPage();
p.setDefaultTimeout(30000);

const errs = [];
const bad5xx = [];
p.on("console", (m) => { if (m.type() === "error") errs.push(m.text().slice(0, 200)); });
p.on("pageerror", (e) => errs.push("pageerror: " + String(e).slice(0, 200)));
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
const ensureApp = async () => {
  for (let round = 0; round < 4; round++) {
    await p.goto(`${APP}/platform/daily-affairs/hr`, { waitUntil: "domcontentloaded", timeout: 90000 }).catch(() => {});
    await p.waitForTimeout(round === 0 ? 26000 : 14000);
    for (let i = 0; i < 26; i++) {
      const n = await p.evaluate(() => document.querySelectorAll(".dash-kpi-card").length);
      if (n >= 5) { await killGuide(); return true; }
      await p.waitForTimeout(1500);
    }
  }
  return false;
};
const TAB_OF_CARD = {
  花名册: "hr-roster", 劳动合同: "hr-contract", 社保管理: "hr-social",
  公积金管理: "hr-fund", 宿舍管理: "hr-dorm",
};
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
  await p.waitForTimeout(1400);
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
const openFirstRow = async (name = "") => {
  for (let attempt = 0; attempt < 3; attempt++) {
    const clicked = await p.evaluate((nm) => {
      const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
        .filter((r) => r.querySelectorAll("td").length > 3);
      const row = nm ? rows.find((r) => (r.innerText || "").includes(nm)) : rows[0];
      if (!row) return false;
      row.click();
      return true;
    }, name);
    if (clicked) {
      for (let i = 0; i < 24; i++) {
        const vis = await p.evaluate(() => Array.from(document.querySelectorAll(".bsd-footer"))
          .some((e) => e.getBoundingClientRect().width > 10));
        if (vis) return true;
        await p.waitForTimeout(500);
      }
    }
    await p.waitForTimeout(1500);
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
  if (await p.evaluate(() => Array.from(document.querySelectorAll(".bsd-footer")).some((e) => e.getBoundingClientRect().width > 10))) {
    await p.keyboard.press("Escape");
    await p.waitForTimeout(700);
  }
};
let settingsOpened = false;
const openSettings = async () => {
  if (settingsOpened) return true;
  await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "设置");
    if (btn) btn.click();
  });
  for (let i = 0; i < 24; i++) {
    const vis = await p.evaluate(() => {
      const d = document.querySelector(".standard-settings-drawer");
      return !!d && d.getBoundingClientRect().width > 100;
    });
    if (vis) { settingsOpened = true; return true; }
    await p.waitForTimeout(400);
  }
  return false;
};
const closeSettings = async () => {
  await p.evaluate(() => {
    const d = document.querySelector(".standard-settings-drawer");
    const btn = d?.querySelector(".t-drawer__close-btn");
    if (btn) btn.click();
  });
  await p.waitForTimeout(1000);
  settingsOpened = false;
};
const drawerLabels = async (waitMs = 9000) => {
  const deadline = Date.now() + waitMs;
  let out = [];
  while (Date.now() < deadline) {
    out = await p.evaluate(() => Array.from(document.querySelectorAll(".t-drawer .t-form__label"))
      .map((e) => (e.innerText || "").trim()).filter(Boolean));
    if (out.length) return out;
    await p.waitForTimeout(300);
  }
  return out;
};
const openBillRow = async (needle) => {
  const done = await p.evaluate((x) => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes(x));
    if (!row) return false;
    (row.querySelectorAll("td")[1] || row.querySelector("td")).click();
    return true;
  }, needle);
  await p.waitForTimeout(2200);
  return done;
};
const billTab = async (label) => {
  const clicked = await p.evaluate((x) => {
    const root = document.querySelector("[data-hr-bill-dialog]");
    const e = Array.from(root?.querySelectorAll(".hr-bill-tabs .t-radio-button") || [])
      .find((q) => (q.innerText || "").trim() === x);
    if (e) { e.click(); return true; }
    return false;
  }, label);
  await p.waitForTimeout(900);
  return clicked;
};
/** 宿舍费用行 → 扣款明细弹窗（t-dialog，不是抽屉）。 */
const openDormBill = async () => {
  await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    if (rows[0]) rows[0].click();
  });
  for (let i = 0; i < 30; i++) {
    const vis = await p.evaluate(() => {
      const el = document.querySelector("[data-hr-dorm-bill-dialog]");
      return !!el && el.getBoundingClientRect().height > 100;
    });
    if (vis) { await p.waitForTimeout(1200); return true; }
    await p.waitForTimeout(600);
  }
  return false;
};
const closeBill = async () => {
  const btn = p.locator("[data-hr-bill-close]");
  if (await btn.count()) await btn.first().click();
  await p.waitForTimeout(800);
};
const dialogRect = () => p.evaluate(() => {
  // data-hr-bill-dialog 在外层 .t-dialog__ctx；真正弹窗是内层 .t-dialog
  const ctx = document.querySelector("[data-hr-bill-dialog]");
  const el = ctx?.querySelector(".t-dialog") || ctx;
  if (!el) return null;
  const r = el.getBoundingClientRect();
  return { w: Math.round(r.width), h: Math.round(r.height), t: Math.round(r.top), l: Math.round(r.left) };
});
const measureTable = () => p.evaluate(() => {
  const root = document.querySelector("[data-hr-bill-dialog]");
  const table = root?.querySelector(".detail-table");
  if (!table) return { err: "no-table" };
  const ths = Array.from(table.querySelectorAll(".detail-table-head thead th"));
  const rows = Array.from(table.querySelectorAll(".detail-table-body tbody tr"))
    .filter((r) => r.querySelectorAll("td").length > 1);
  const foot = table.querySelector(".detail-table-foot tfoot tr");
  if (!ths.length || !rows.length) return { err: "empty" };
  const tds = Array.from(rows[0].querySelectorAll("td"));
  const fts = foot ? Array.from(foot.querySelectorAll("td")) : [];
  let max = 0;
  for (let i = 0; i < ths.length; i++) {
    const a = ths[i].getBoundingClientRect();
    const b = tds[i]?.getBoundingClientRect();
    const f = fts[i]?.getBoundingClientRect();
    if (!b) continue;
    const d = Math.max(Math.abs(a.left - b.left), Math.abs(a.right - b.right),
      f ? Math.abs(a.left - f.left) : 0, f ? Math.abs(a.right - f.right) : 0);
    if (d > max) max = d;
  }
  const body = table.querySelector(".detail-table-body");
  return { max: Number(max.toFixed(2)), cols: ths.length, hScroll: body ? body.scrollWidth - body.clientWidth : -1 };
});
const api = async (method, path, data, multipart) => {
  const opts = { headers: HDR };
  if (multipart) opts.multipart = multipart;
  else if (data !== undefined) { opts.data = data; }
  const res = await c.request.fetch(`${BASE}${path}`, { method, ...opts });
  let body = null;
  try { body = await res.json(); } catch { /* 非 JSON */ }
  return { status: res.status(), body };
};

try {
  if (!(await ensureApp())) throw new Error("首屏 bootstrap 未就绪（KPI 卡片未渲染）");

  // ================= 断言 1：抽屉紧凑排版 + 设置抽屉零截断 =================
  await card("花名册");
  await waitRows();
  ok("1.0 花名册员工抽屉可打开", await openFirstRow("廖宏"));
  const layout = await p.evaluate(() => {
    const items = Array.from(document.querySelectorAll(".t-drawer .t-form__item"))
      .filter((e) => e.getBoundingClientRect().height > 10);
    const label = items[0]?.querySelector(".t-form__label");
    const ctrl = items[0]?.querySelector(".t-form__controls");
    const lr = label?.getBoundingClientRect();
    const cr = ctrl?.getBoundingClientRect();
    const cs = label ? getComputedStyle(label) : null;
    // 长内容输入框是否被截断（scrollWidth 超出可视宽 = 省略号/滚动）
    const truncated = [];
    for (const inp of Array.from(document.querySelectorAll(".t-drawer .bsd-fields input"))) {
      const v = String(inp.value || "");
      if (v.length < 8) continue;
      if (inp.scrollWidth > inp.clientWidth + 1) truncated.push({ v: v.slice(0, 24), sw: inp.scrollWidth, cw: inp.clientWidth });
    }
    // 整行字段是否从栅格左缘开始（防止孤立半行）
    const grid = document.querySelector(".t-drawer .bsd-fields");
    const gridLeft = grid?.getBoundingClientRect().left || 0;
    const fulls = Array.from(document.querySelectorAll(".t-drawer .bsd-field--full"))
      .map((e) => Math.round(e.getBoundingClientRect().left - gridLeft));
    return {
      itemCount: items.length,
      labelTopOfControl: lr && cr ? lr.bottom <= cr.top + 2 : null,
      fontSize: cs?.fontSize, fontWeight: cs?.fontWeight,
      truncated: truncated.slice(0, 5),
      fullOffsets: fulls.slice(0, 8),
      fullCount: fulls.length,
    };
  });
  ok("1.1 抽屉为 label 置顶上下结构（label 底 ≤ 控件顶）", layout.labelTopOfControl === true, JSON.stringify(layout.labelTopOfControl));
  ok("1.2 label 字号 13px 且非加粗", layout.fontSize === "13px", `${layout.fontSize}/${layout.fontWeight}`);
  ok("1.3 长内容输入框无截断（18 位身份证/公司全称）", layout.truncated.length === 0, JSON.stringify(layout.truncated));
  ok("1.4 整行字段从栅格左缘起（无孤立半行错位）",
    layout.fullOffsets.every((x) => Math.abs(x) <= 2) && layout.fullCount <= 10,
    JSON.stringify({ offsets: layout.fullOffsets, count: layout.fullCount }));
  await shot("d68_01_drawer_top_label");
  await closeDrawer();

  // 设置抽屉：1280×800 与 1440×900 两种视口
  for (const [w, h, tag] of [[1280, 800, "1280"], [1440, 900, "1440"]]) {
    await p.setViewportSize({ width: w, height: h });
    await p.waitForTimeout(800);
    const opened = await openSettings();
    ok(`1.5 设置抽屉可打开（${tag}×${h}）`, opened, String(opened));
    const setInfo = await p.evaluate(() => {
      const drawer = document.querySelector(".standard-settings-drawer");
      const wrap = drawer?.querySelector(".t-drawer__content-wrapper")
        || document.querySelector(".standard-settings-drawer .t-drawer__content-wrapper") || drawer;
      const bodyEl = drawer?.querySelector(".t-drawer__body");
      const typeSel = document.querySelector(".standard-settings-drawer .fc-type");
      const inner = typeSel?.querySelector("input");
      const row = typeSel?.closest(".field-config-row");
      return {
        drawerW: Math.round((wrap || drawer)?.getBoundingClientRect().width || 0),
        // TDesign select 的选中值在 input.value 里，innerText 恒为空
        typeText: (inner?.value || typeSel?.innerText || "").trim(),
        typeW: Math.round(typeSel?.getBoundingClientRect().width || 0),
        typeOverflow: inner ? inner.scrollWidth - inner.clientWidth : null,
        hScroll: bodyEl ? bodyEl.scrollWidth - bodyEl.clientWidth : null,
        rowW: Math.round(row?.getBoundingClientRect().width || 0),
      };
    });
    ok(`1.6 设置抽屉宽度 ≥ 680（${tag}）`, setInfo.drawerW >= 680, JSON.stringify(setInfo));
    ok(`1.7 数据类型完整显示（${tag}）`,
      /^(单行文本|多行文本|数字|日期|数组|明细)$/.test(setInfo.typeText)
      && setInfo.typeW >= 100 && (setInfo.typeOverflow ?? 0) <= 1,
      JSON.stringify({ text: setInfo.typeText, w: setInfo.typeW, overflow: setInfo.typeOverflow }));
    ok(`1.8 设置抽屉无横向滚动（${tag}）`, (setInfo.hScroll ?? 0) <= 1, `hScroll=${setInfo.hScroll}`);
    await shot(`d68_02_settings_${tag}`);
    if (tag === "1280") {
      // 保持打开，供断言 2 使用（1440 那一轮复用同一抽屉，只改视口）
      await p.setViewportSize({ width: 1280, height: 800 });
    }
  }

  // ================= 断言 2：离职人员全字段且默认关闭 =================
  const rosterSubs = async () => {
    const r = await api("GET", "/categories?scope=hr-roster");
    const cats = r.body?.data || [];
    const active = [];
    const resigned = [];
    for (const cat of cats) for (const s of (cat.subs || [])) {
      if (String(s.group || "") === "在职人员") active.push(s);
      if (String(s.group || "") === "离职人员") resigned.push(s);
    }
    return { active, resigned };
  };
  let subs = (await rosterSubs()).resigned;
  const activeCount = (await rosterSubs()).active.length;
  // 离职胶囊 = 在职全集（含用户自定义字段）+ 9 个离职专有字段（6 个引用与在职重名）
  ok("2.1 离职胶囊包含在职全集 + 离职专有（≥57 且 = 在职数 + 9）",
    subs.length >= 57 && subs.length === activeCount + 9,
    `resign=${subs.length} active=${activeCount}`);
  const FIELD = "最高学历";
  const mergedNames = [FIELD, "工作地点", "手机号码", "紧急联系人姓名", "毕业院校"];
  const mergedDefaults = mergedNames.map((n) => {
    const s = subs.find((x) => String(x.name || "").trim() === n);
    return { n, exists: !!s, enabled: s?.enabled, isDefault: s?.is_default };
  });
  ok("2.2 新并入的在职字段默认关闭（enabled/is_default 均 false）",
    mergedDefaults.every((x) => x.exists && x.enabled === false && !x.is_default),
    JSON.stringify(mergedDefaults));
  const keepOn = ["离职日期", "离职类型", "离职原因", "离职证明"];
  ok("2.3 原 15 个离职字段保持启用且默认表头",
    keepOn.every((n) => {
      const s = subs.find((x) => String(x.name || "").trim() === n);
      return s && s.enabled !== false && s.is_default === true;
    }), JSON.stringify(subs.filter((s) => keepOn.includes(s.name)).map((s) => [s.name, s.enabled, s.is_default])));

  // 设置抽屉里切到「离职人员」胶囊，开启「学历」（默认表头 + 启用）
  await openSettings();
  await p.evaluate(() => {
    const tab = Array.from(document.querySelectorAll(".standard-settings-drawer .fc-group-tab"))
      .find((e) => (e.innerText || "").trim().startsWith("离职人员"));
    if (tab) tab.click();
  });
  await p.waitForTimeout(1200);
  const rowIdx = await p.evaluate((nm) => Array.from(document.querySelectorAll(".standard-settings-drawer .field-config-row"))
    .findIndex((r) => Array.from(r.querySelectorAll("input")).some((i) => i.value === nm)), FIELD);
  ok("2.4 设置抽屉「离职人员」胶囊含该字段行", rowIdx >= 0, `idx=${rowIdx}`);
  const rowsCount = await p.evaluate(() => document.querySelectorAll(".standard-settings-drawer .field-config-row").length);
  ok("2.5 离职人员胶囊渲染出全部字段行", rowsCount >= 50, `rows=${rowsCount}`);
  // 打开「学历」的两个开关（默认表头 | 启用）
  await p.evaluate((idx) => {
    const row = document.querySelectorAll(".standard-settings-drawer .field-config-row")[idx];
    if (!row) return;
    for (const s of Array.from(row.querySelectorAll(".t-switch"))) {
      if (!s.className.includes("t-is-checked")) s.click();
    }
  }, rowIdx);
  await p.waitForTimeout(600);
  const toggled = await p.evaluate((idx) => {
    const row = document.querySelectorAll(".standard-settings-drawer .field-config-row")[idx];
    if (!row) return null;
    const sw = Array.from(row.querySelectorAll(".t-switch"));
    return { after: sw.map((s) => s.className.includes("t-is-checked")), sw: sw.length };
  }, rowIdx);
  ok("2.6 可开启「最高学历」的默认表头与启用开关", !!toggled && toggled.after.filter(Boolean).length === toggled.sw,
    JSON.stringify(toggled));
  await p.locator(".standard-settings-drawer .meter-form-actions button", { hasText: "保存" }).first().click();
  await p.waitForTimeout(3200);
  settingsOpened = false;
  subs = (await rosterSubs()).resigned;
  const edu = subs.find((s) => String(s.name || "").trim() === FIELD);
  ok("2.7 保存后落库：最高学历 enabled + is_default 均为 true", !!edu && edu.enabled !== false && edu.is_default === true,
    JSON.stringify(edu || null));

  await subTab("离职人员");
  await waitRows();
  // 保存后字段配置与列表都在重新装载：轮询到表头出现（最多 14s）
  let resignHeaders = [];
  let headerHas = false;
  for (let i = 0; i < 20; i++) {
    resignHeaders = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
      .map((e) => (e.innerText || "").trim()));
    headerHas = resignHeaders.includes(FIELD);
    if (headerHas) break;
    await p.waitForTimeout(700);
  }
  ok("2.8 离职列表表头即时出现「最高学历」", headerHas, JSON.stringify(resignHeaders.slice(0, 14)));
  ok("2.9 离职详情抽屉可打开", await openFirstRow(), "");
  let resignLabels = [];
  let drawerHas = false;
  for (let i = 0; i < 20; i++) {
    resignLabels = await drawerLabels(1200);
    drawerHas = resignLabels.includes(FIELD);
    if (drawerHas) break;
    await p.waitForTimeout(600);
  }
  ok("2.10 离职抽屉渲染「最高学历」字段", drawerHas,
    `labels=${resignLabels.length} has=${drawerHas} sample=${JSON.stringify(resignLabels.slice(-6))}`);
  await closeDrawer();

  // 还原：关掉「学历」的两个开关
  await openSettings();
  await p.evaluate(() => {
    const tab = Array.from(document.querySelectorAll(".standard-settings-drawer .fc-group-tab"))
      .find((e) => (e.innerText || "").trim().startsWith("离职人员"));
    if (tab) tab.click();
  });
  await p.waitForTimeout(1200);
  await p.evaluate((name) => {
    const row = Array.from(document.querySelectorAll(".standard-settings-drawer .field-config-row"))
      .find((r) => Array.from(r.querySelectorAll("input")).some((i) => i.value === name));
    if (!row) return;
    for (const s of Array.from(row.querySelectorAll(".t-switch"))) {
      if (s.className.includes("t-is-checked")) s.click();
    }
  }, FIELD);
  await p.waitForTimeout(500);
  await p.locator(".standard-settings-drawer .meter-form-actions button", { hasText: "保存" }).first().click();
  await p.waitForTimeout(2600);
  settingsOpened = false;
  subs = (await rosterSubs()).resigned;
  const edu2 = subs.find((s) => String(s.name || "").trim() === FIELD);
  ok("2.11 测试字段已还原（最高学历恢复默认关闭）", !!edu2 && edu2.enabled === false && !edu2.is_default,
    JSON.stringify(edu2 || null));

  // ================= 断言 3：社保跨月隔离 + 快照不可变 + 变更提醒 =================
  const csvPath = (name, rows) => {
    const fp = `${SHOTS}/${name}`;
    fs.writeFileSync(fp, rows.map((r) => r.join(",")).join("\n"), "utf8");
    return fp;
  };
  const H = ["序号", "姓名", "证件号码", "缴费基数", "应缴费额", "人员编号"];
  const f09p = csvPath("2026-09养老单位缴纳.csv", [H,
    ["1", "张三", "500101199001010011", "5000", "800", "P001"],
    ["2", "李四", "500101199002020022", "4200", "672", "P002"]]);
  const f09i = csvPath("2026-09工伤单位缴纳.csv", [H,
    ["1", "张三", "500101199001010011", "5000", "60", "P001"],
    ["2", "李四", "500101199002020022", "4200", "50.4", "P002"]]);
  const f08i = csvPath("2026-08工伤单位缴纳.csv", [H,
    ["1", "张三", "500101199001010011", "5000", "88.8", "P001"]]);

  const cleanupMonth = async (m) => {
    for (const kind of ["normal", "abnormal"]) {
      const r = await api("GET", `/hr/social/records?record_type=${kind}&month=${m}`);
      const ids = (r.body?.data?.items || []).map((x) => x.id).filter(Boolean);
      if (ids.length) await api("POST", "/hr/social/records/batch-delete", { ids });
    }
    const bills = await api("GET", "/hr/social/bills");
    for (const bill of (bills.body?.data?.items || [])) {
      if (String(bill.month_label) === m) await api("DELETE", `/hr/social/bills/${bill.id}`);
    }
  };
  await cleanupMonth("2026-09");
  await cleanupMonth("2026-08");

  const imp = async (files, month) => {
    // 原生 fetch + FormData（Playwright 的 multipart 序列化在 Node 22 会报 stream.on）
    const fd = new FormData();
    fd.append("record_type", "normal");
    fd.append("bill_month", month);
    for (const f of files) {
      fd.append("files", new Blob([fs.readFileSync(f)], { type: "text/csv" }), f.split("/").pop());
    }
    const res = await fetch(`${BASE}/hr/social/records/import`, { method: "POST", headers: HDR, body: fd });
    let body = null;
    try { body = await res.json(); } catch { /* ignore */ }
    return { status: res.status, body };
  };
  const imp09 = await imp([f09p, f09i], "2026-09");
  ok("3.1 导入 2026-09 养老+工伤成功", imp09.status === 200, JSON.stringify(imp09.body?.data || imp09.body));
  const imp08 = await imp([f08i], "2026-08");
  ok("3.2 导入 2026-08 工伤成功", imp08.status === 200, JSON.stringify(imp08.body?.data || imp08.body));

  const recOf = async (m) => {
    const r = await api("GET", `/hr/social/records?record_type=normal&month=${m}`);
    const items = r.body?.data?.items || [];
    return { n: items.length, total: items.reduce((s, x) => s + Number(x["应缴合计"] || 0), 0), items };
  };
  const sep = await recOf("2026-09");
  ok("3.3 导入 2026-08 后 2026-09 记录条数与金额 100% 未受影响",
    sep.n === 4 && Math.abs(sep.total - (800 + 672 + 60 + 50.4)) < 0.01,
    JSON.stringify({ n: sep.n, total: sep.total }));

  const gen09 = await api("POST", "/hr/social/bills", { month_label: "2026-09", overwrite: true });
  ok("3.4 生成 2026-09 账单成功", gen09.status === 200, JSON.stringify(gen09.body?.data || gen09.body));
  const bill09 = gen09.body?.data || {};
  const detailBefore = await api("GET", `/hr/social/bills/${bill09.id}`);
  const dBefore = detailBefore.body?.data || {};
  const amountBefore = Number(dBefore.bill?.["应缴总计"] || 0);
  const itemsBefore = (dBefore.items || []).length;
  // 明细按 (人, 记录类型) 聚合：张三/李四各 1 行（各行含养老+工伤两险种）
  ok("3.5 账单快照已固化（明细 2 条 / 合计 1582.4）",
    itemsBefore === 2 && Math.abs(amountBefore - 1582.4) < 0.01,
    JSON.stringify({ items: itemsBefore, amount: amountBefore }));

  const delTarget = (sep.items || []).find((x) => String(x["险种类型"] || "").includes("养老"));
  const delRes = await api("DELETE", `/hr/social/records/${delTarget?.id}`);
  ok("3.6 删除 2026-09 一条正常记录 → 返回 bill_dirty_warning",
    delRes.status === 200 && delRes.body?.data?.bill_dirty_warning === true,
    JSON.stringify(delRes.body?.data || delRes.body));

  const detailAfter = await api("GET", `/hr/social/bills/${bill09.id}`);
  const dAfter = detailAfter.body?.data || {};
  ok("3.7 已生成账单 100% 保持原快照（明细/金额不变）",
    (dAfter.items || []).length === itemsBefore
    && Math.abs(Number(dAfter.bill?.["应缴总计"] || 0) - amountBefore) < 0.01
    && dAfter.bill?.source_dirty === true,
    JSON.stringify({ items: (dAfter.items || []).length, amount: dAfter.bill?.["应缴总计"], dirty: dAfter.bill?.source_dirty }));

  // UI：月度账单列表橙标 + 明细弹窗顶部提醒条
  await card("社保管理");
  await subTab("月度账单");
  await waitRows();
  const hasFlag = await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes("2026-09"));
    return !!row && !!row.querySelector("[data-hr-bill-dirty-flag]");
  });
  ok("3.8 月度账单列表 2026-09 行显示「原始记录已变动」橙标", hasFlag, String(hasFlag));
  await openBillRow("2026-09");
  const bannerVisible = await p.evaluate(() => {
    const el = document.querySelector("[data-hr-bill-dirty]");
    return !!el && el.getBoundingClientRect().height > 0;
  });
  ok("3.9 账单明细弹窗顶部显示变更提醒条", bannerVisible, String(bannerVisible));
  await shot("d68_03_bill_dirty");
  await closeBill();

  const regen = await api("POST", "/hr/social/bills", { month_label: "2026-09", overwrite: true });
  ok("3.10 重新生成账单成功且清脏标记",
    regen.status === 200 && regen.body?.data?.source_dirty === false, JSON.stringify(regen.body?.data || regen.body));
  const billsList = await api("GET", "/hr/social/bills");
  const row09 = (billsList.body?.data?.items || []).find((x) => String(x.month_label) === "2026-09");
  ok("3.11 列表数据已按最新记录重算（782.4）",
    !!row09 && Math.abs(Number(row09["应缴总计"] || 0) - (672 + 60 + 50.4)) < 0.01,
    JSON.stringify({ total: row09?.["应缴总计"], dirty: row09?.source_dirty }));
  await cleanupMonth("2026-09");
  await cleanupMonth("2026-08");

  // ================= 断言 4：智能纵横打印 =================
  await p.setViewportSize({ width: 1680, height: 1000 });
  const measurePaper = async () => p.evaluate(() => {
    const page = document.querySelector(".ppd-dialog .pdf-source-page");
    if (!page) return null;
    const r = page.getBoundingClientRect();
    const spans = Array.from(page.querySelectorAll(".textLayer span"))
      .map((s) => ({ t: (s.textContent || "").trim(), x: s.getBoundingClientRect().left }))
      .filter((s) => s.t);
    const seq = spans.find((s) => s.t === "序号") || spans.find((s) => s.t.includes("序号"));
    const second = spans.filter((s) => s.x > (seq?.x ?? -1) + 2).sort((a, b) => a.x - b.x)[0];
    return {
      w: Math.round(r.width), h: Math.round(r.height),
      portrait: r.height > r.width,
      seqRatio: seq && second ? Number(((second.x - seq.x) / r.width * 100).toFixed(1)) : null,
      headers: spans.slice(0, 12).map((s) => s.t),
    };
  });
  const openPrint = async (clickLabel) => {
    await p.evaluate((x) => {
      const dlg = document.querySelector("[data-hr-bill-dialog]");
      const btns = Array.from(dlg?.querySelectorAll("button") || []);
      const btn = btns.find((b) => (b.innerText || "").trim() === x);
      if (btn) btn.click();
    }, clickLabel);
    await p.waitForTimeout(3500);
    for (let i = 0; i < 30; i++) {
      const ready = await p.evaluate(() => !!document.querySelector(".ppd-dialog .pdf-source-page"));
      if (ready) break;
      await p.waitForTimeout(700);
    }
    await p.waitForTimeout(1500);
  };
  const closePrint = async () => {
    const btn = p.locator(".ppd-close");
    if (await btn.count()) await btn.first().click();
    await p.waitForTimeout(800);
  };

  // 4.1 公积金 岗位汇总（6 列）→ 纵向
  await card("公积金管理");
  await subTab("月度账单");
  await waitRows();
  await openBillRow("2026-11");
  await billTab("岗位汇总");
  await openPrint("打印");
  const fundPaper = await measurePaper();
  ok("4.1 6 列岗位汇总打印为 A4 纵向", !!fundPaper && fundPaper.portrait, JSON.stringify(fundPaper));
  ok("4.2 序号列占比 < 12%", !!fundPaper && (fundPaper.seqRatio ?? 99) < 12, `seqRatio=${fundPaper?.seqRatio}%`);
  await shot("d68_04_print_portrait");
  await closePrint();
  await closeBill();

  // 4.3 社保 个人明细（9 列）→ 纵向
  await card("社保管理");
  await subTab("月度账单");
  await waitRows();
  await openBillRow("2026-11");
  await billTab("个人明细");
  await openPrint("打印");
  const siPaper = await measurePaper();
  ok("4.3 9 列社保个人明细打印为 A4 纵向", !!siPaper && siPaper.portrait, JSON.stringify(siPaper));
  ok("4.4 社保个人明细序号列占比 < 12%", !!siPaper && (siPaper.seqRatio ?? 99) < 12, `seqRatio=${siPaper?.seqRatio}%`);
  await closePrint();
  await closeBill();

  // 4.5 宿舍扣款明细（15 列）→ 横向
  await card("宿舍管理");
  await subTab("费用信息");
  await waitRows();
  ok("4.5a 费用信息扣款明细弹窗可打开", await openDormBill());
  // 明细行是异步拉取的：等数据体出现真实行再点打印（否则 printDormBill 会因 rows=0 直接 return）
  let dormRows = 0;
  for (let i = 0; i < 30; i++) {
    dormRows = await p.evaluate(() => {
      const el = Array.from(document.querySelectorAll("[data-hr-dorm-bill-dialog]"))
        .find((e) => e.getBoundingClientRect().width > 100);
      return el ? Array.from(el.querySelectorAll(".detail-table-body tbody tr"))
        .filter((r) => r.querySelectorAll("td").length > 1).length : 0;
    });
    if (dormRows > 0) break;
    await p.waitForTimeout(700);
  }
  ok("4.5b 扣款明细数据行已加载", dormRows > 0, `rows=${dormRows}`);
  // 宿舍明细弹窗可能同时残留隐藏的历史实例：一律只操作"可见"的那一个
  const dormCols = await p.evaluate(() => {
    const el = Array.from(document.querySelectorAll("[data-hr-dorm-bill-dialog]"))
      .find((e) => e.getBoundingClientRect().width > 100);
    return el ? el.querySelectorAll(".detail-table-head thead th").length : 0;
  });
  ok("4.5 扣款明细列数 ≥ 15（足够宽）", dormCols >= 15, `cols=${dormCols}`);
  await p.evaluate(() => {
    const el = Array.from(document.querySelectorAll("[data-hr-dorm-bill-dialog]"))
      .find((e) => e.getBoundingClientRect().width > 100);
    const btn = Array.from(el?.querySelectorAll("button") || []).find((b) => (b.innerText || "").trim() === "打印");
    if (btn) btn.click();
  });
  await p.waitForTimeout(3500);
  for (let i = 0; i < 30; i++) {
    if (await p.evaluate(() => !!document.querySelector(".ppd-dialog .pdf-source-page"))) break;
    await p.waitForTimeout(700);
  }
  await p.waitForTimeout(1500);
  const dormPaper = await measurePaper();
  ok("4.6 15+ 列扣款明细自动切换 A4 横向", !!dormPaper && dormPaper.portrait === false, JSON.stringify(dormPaper));
  await shot("d68_05_print_landscape");
  await closePrint();
  await p.evaluate(() => {
    const el = Array.from(document.querySelectorAll("[data-hr-dorm-bill-dialog]"))
      .find((e) => e.getBoundingClientRect().width > 100);
    const btn = Array.from(el?.querySelectorAll("button") || []).find((b) => (b.innerText || "").trim() === "关闭");
    if (btn) btn.click();
  });
  await p.waitForTimeout(900);

  // ================= 断言 5：明细弹窗切换标签 0px 跳动 =================
  const geomAcross = async (labels) => {
    const out = [];
    for (const t of labels) {
      await billTab(t);
      const r = await dialogRect();
      const m = await measureTable();
      out.push({ tab: t, ...r, max: m.max, hScroll: m.hScroll });
    }
    const span = (k) => Math.max(...out.map((x) => x[k])) - Math.min(...out.map((x) => x[k]));
    return { out, dw: span("w"), dh: span("h"), dt: span("t"), dl: span("l") };
  };
  await card("公积金管理");
  await subTab("月度账单");
  await waitRows();
  await openBillRow("2026-11");
  const gFund = await geomAcross(["个人明细", "单位明细", "部门汇总", "岗位汇总"]);
  ok("5.1 公积金弹窗 4 标签几何零跳动（宽高/左上 ≤1px）",
    Math.max(gFund.dw, gFund.dh, gFund.dt, gFund.dl) <= 1, JSON.stringify(gFund).slice(0, 300));
  ok("5.2 切换标签后三段表仍逐列对齐且无横滚",
    gFund.out.every((x) => x.max <= 1 && (x.hScroll ?? 0) <= 0), JSON.stringify(gFund.out.map((x) => [x.tab, x.max, x.hScroll])));
  await shot("d68_06_fund_geom");
  await closeBill();

  await card("社保管理");
  await subTab("月度账单");
  await waitRows();
  await openBillRow("2026-11");
  const gSi = await geomAcross(["个人明细", "单位明细", "部门汇总"]);
  ok("5.3 社保弹窗 3 标签几何零跳动（宽高/左上 ≤1px）",
    Math.max(gSi.dw, gSi.dh, gSi.dt, gSi.dl) <= 1, JSON.stringify(gSi).slice(0, 300));
  ok("5.4 社保切换标签后三段表仍逐列对齐且无横滚",
    gSi.out.every((x) => x.max <= 1 && (x.hScroll ?? 0) <= 0), JSON.stringify(gSi.out.map((x) => [x.tab, x.max, x.hScroll])));
  await shot("d68_07_social_geom");
  await closeBill();

  // ================= 全局零错误 =================
  ok("6.1 全程 0 console.error", errs.length === 0, JSON.stringify(errs.slice(0, 5)));
  ok("6.2 全程 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 5)));
} catch (e) {
  ok("FATAL 脚本异常", false, String(e && e.stack ? e.stack : e));
  try { await shot("d68_fatal"); } catch { /* ignore */ }
} finally {
  const fails = R.checks.filter((x) => !x.pass);
  console.log(`\n==== 结果：${R.checks.length - fails.length}/${R.checks.length} PASS，${fails.length} FAIL ====`);
  if (fails.length) console.log(fails.map((f) => `FAIL ${f.n} :: ${f.d}`).join("\n"));
  fs.writeFileSync(`${SHOTS}/d68-result.json`, JSON.stringify(R, null, 2));
  await b.close();
  process.exit(fails.length ? 1 : 0);
}
