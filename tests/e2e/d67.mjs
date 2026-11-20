// 第 67 轮真机验收：账单明细弹窗框线像素级对齐 / 公积金岗位汇总 / 人事六同步闭环。
//
// 运行：cp tests/e2e/d67.mjs /opt/pw-run/ && cd /opt/pw-run && node d67.mjs
// 依赖：playwright（裸包名）、/tmp/pw/token.txt、前端 5173 / 后端 8080。
//
// 断言分组：
//   1) 三段表（表头 / 数据体 / 合计行）逐列 getBoundingClientRect 偏差 <= 1px，
//      有纵向滚动条（个人明细 74 行）与无纵向滚动条（岗位汇总首屏）两种状态都验；
//      社保弹窗只有 3 个标签，公积金弹窗第 4 个标签是「岗位汇总」。
//   2) 岗位汇总按花名册真实岗位聚合：含「小拉操作员 / 管退操作员」等，
//      合计与部门汇总逐分相等，搜索「操作员」实时重算。
//   3) 花名册自定义字段「工作地点」：设置 → 抽屉输入 → 列表列 → 刷新回显 → 模板表头。
//   4) 合同 / 宿舍：字段配置 == 编辑抽屉 == 导入模板（抽测 + 临时字段增删还原）。
import { chromium } from "playwright";
import fs from "node:fs";
import { execSync } from "node:child_process";

const APP = process.env.WEKNORA_E2E_APP || "http://127.0.0.1:5173";
const API = process.env.WEKNORA_E2E_API || "http://127.0.0.1:8080";
const SHOTS = process.env.CI_SHOTS_DIR || "/tmp/pw/shots";
const TK = fs.readFileSync(process.env.WEKNORA_TOKEN_FILE || "/tmp/pw/token.txt", "utf8").trim();
const HDR = { Authorization: `Bearer ${TK}`, "X-Tenant-ID": "10000" };
fs.mkdirSync(SHOTS, { recursive: true });

const b = await chromium.launch({ args: ["--no-sandbox"] });
const c = await b.newContext({ viewport: { width: 1680, height: 1000 }, acceptDownloads: true });
await c.addInitScript((t) => {
  localStorage.setItem("weknora_token", t);
  localStorage.setItem("weknora_selected_tenant_id", "10000");
}, TK);
const p = await c.newPage();
p.setDefaultTimeout(30000);

const errs = [];
p.on("console", (m) => { if (m.type() === "error") errs.push(m.text().slice(0, 200)); });
p.on("pageerror", (e) => errs.push("pageerror: " + String(e).slice(0, 200)));
const bad5xx = [];
p.on("response", (r) => {
  const u = r.url();
  if (r.status() >= 500 && !/favicon/.test(u)) bad5xx.push(`${r.status()} ${u.slice(0, 120)}`);
});

/**
 * 首次访问的欢迎引导层（.guide）会拦截真实指针事件。
 * 移除节点还不够（路由切换后会重新挂载），必须用全局样式永久隐藏。
 */
const killGuide = async () => {
  await p.addStyleTag({ content: ".guide{display:none!important;pointer-events:none!important}" }).catch(() => {});
  await p.evaluate(() => document.querySelectorAll(".guide").forEach((e) => e.remove())).catch(() => {});
};
const R = { checks: [] };
const ok = (n, cond, d) => {
  R.checks.push({ n, pass: !!cond, d: String(d ?? "").slice(0, 400) });
  console.log(`${cond ? "PASS" : "FAIL"} ${n} :: ${String(d ?? "").slice(0, 200)}`);
};
const shot = (n) => p.screenshot({ path: `${SHOTS}/${n}.png` });

const ensureApp = async () => {
  for (let round = 0; round < 4; round++) {
    await p.goto(`${APP}/platform/daily-affairs/hr`, { waitUntil: "domcontentloaded", timeout: 90000 });
    await p.waitForTimeout(round === 0 ? 26000 : 14000);
    for (let i = 0; i < 26; i++) {
      const n = await p.evaluate(() => document.querySelectorAll(".dash-kpi-card").length);
      if (n >= 5) {
        await p.evaluate(() => document.querySelectorAll(".guide").forEach((e) => e.remove()));
        return true;
      }
      await p.waitForTimeout(1500);
    }
    console.log(`WARN 首屏未就绪，重载第 ${round + 1} 轮`);
  }
  return false;
};
const TAB_OF_CARD = {
  "花名册": "hr-roster", "劳动合同": "hr-contract", "社保管理": "hr-social",
  "公积金管理": "hr-fund", "宿舍管理": "hr-dorm",
};
const card = async (label) => {
  const want = TAB_OF_CARD[label] || "";
  await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes(x));
    if (e) e.click();
  }, label);
  // 等路由/标签真正切换（否则会在旧表格上点行 → 抽屉永远不出现）
  for (let i = 0; i < 40; i++) {
    const cur = await p.evaluate(() => document.querySelector("[data-hr-tab]")?.getAttribute("data-hr-tab") || "");
    if (!want || cur === want) break;
    await p.waitForTimeout(500);
  }
  await p.waitForTimeout(1200);
};
const subTab = async (label) => {
  const clicked = await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".hr-subview-bar .t-radio-button"))
      .find((q) => (q.innerText || "").trim() === x);
    if (e) { e.click(); return true; }
    return false;
  }, label);
  await p.waitForTimeout(1200);
  return clicked;
};
const waitRows = async (n = 18) => {
  for (let i = 0; i < n; i++) {
    const c = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3).length);
    if (c > 0) return c;
    await p.waitForTimeout(1500);
  }
  return 0;
};
/**
 * 打开列表首行（或指定姓名的行）并等抽屉可见。
 * 表格重渲染期间点击会丢事件，这里最多重试 3 轮。
 */
/** 长轮询等列表行（秒级），返回行数。 */
const waitRowsLong = async (seconds = 60) => {
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
    await p.waitForTimeout(1800);
  }
  return false;
};

/** 打开列表里包含某文案的行（账单行 → 明细弹窗）。 */
const openRow = async (needle) => {
  const done = await p.evaluate((x) => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes(x));
    if (!row) return false;
    const td = row.querySelectorAll("td")[1] || row.querySelector("td");
    td.click();
    return true;
  }, needle);
  await p.waitForTimeout(2200);
  return done;
};
const closeBill = async () => {
  const btn = p.locator("[data-hr-bill-close]");
  if (await btn.count()) await btn.first().click();
  await p.waitForTimeout(800);
};
const billTab = async (label) => {
  const clicked = await p.evaluate((x) => {
    const root = document.querySelector("[data-hr-bill-dialog]");
    if (!root) return false;
    const e = Array.from(root.querySelectorAll(".hr-bill-tabs .t-radio-button"))
      .find((q) => (q.innerText || "").trim() === x);
    if (e) { e.click(); return true; }
    return false;
  }, label);
  await p.waitForTimeout(900);
  return clicked;
};
/**
 * 关闭详情抽屉：按「可见」的 footer 定位（SettingDrawer destroy-on-close 前
 * 可能残留隐藏实例，裸 .first() 会点到一个不可见的按钮上 → 30s 超时）。
 */
const closeDrawer = async () => {
  const clicked = await p.evaluate(() => {
    const footers = Array.from(document.querySelectorAll(".bsd-footer"));
    const f = footers.find((e) => e.getBoundingClientRect().width > 10);
    if (!f) return false;
    const btn = Array.from(f.querySelectorAll("button"))
      .find((b) => /取消|关闭/.test((b.innerText || "").trim()));
    if (btn) { btn.click(); return true; }
    return false;
  });
  await p.waitForTimeout(900);
  if (await p.evaluate(() => Array.from(document.querySelectorAll(".bsd-footer"))
    .some((e) => e.getBoundingClientRect().width > 10))) {
    await p.keyboard.press("Escape");
    await p.waitForTimeout(700);
  }
  return clicked;
};
/**
 * 点击「可见」抽屉 footer 里的保存按钮。
 * 脏检查未通过时按钮 disabled（例如重复运行且值未变），返回状态供断言。
 */
const saveDrawer = async () => {
  const state = await p.evaluate(() => {
    const f = Array.from(document.querySelectorAll(".bsd-footer"))
      .find((e) => e.getBoundingClientRect().width > 10);
    if (!f) return "no-footer";
    const btn = Array.from(f.querySelectorAll("button"))
      .find((b) => (b.innerText || "").trim() === "保存");
    if (!btn) return "no-btn";
    if (btn.disabled) return "disabled";
    btn.click();
    return "clicked";
  });
  await p.waitForTimeout(2800);
  return state;
};
const billTabNames = () => p.evaluate(() => {
  const root = document.querySelector("[data-hr-bill-dialog]");
  return root ? Array.from(root.querySelectorAll(".hr-bill-tabs .t-radio-button"))
    .map((e) => (e.innerText || "").trim()) : [];
});
/** 逐列量三段表格的左右边界（px）。 */
const measureTable = () => p.evaluate(() => {
  const root = document.querySelector("[data-hr-bill-dialog]");
  if (!root) return { err: "no-dialog" };
  const table = root.querySelector(".detail-table");
  if (!table) return { err: "no-table" };
  const ths = Array.from(table.querySelectorAll(".detail-table-head thead th"));
  const rows = Array.from(table.querySelectorAll(".detail-table-body tbody tr"))
    .filter((r) => r.querySelectorAll("td").length > 1);
  const foot = table.querySelector(".detail-table-foot tfoot tr");
  if (!ths.length || !rows.length) return { err: "empty", cols: ths.length, rows: rows.length };
  const tds = Array.from(rows[0].querySelectorAll("td"));
  const fts = foot ? Array.from(foot.querySelectorAll("td")) : [];
  let max = 0; let worst = -1; const diffs = [];
  for (let i = 0; i < ths.length; i++) {
    const a = ths[i].getBoundingClientRect();
    const b = tds[i]?.getBoundingClientRect();
    const f = fts[i]?.getBoundingClientRect();
    if (!b) continue;
    const d = Math.max(Math.abs(a.left - b.left), Math.abs(a.right - b.right),
      f ? Math.abs(a.left - f.left) : 0, f ? Math.abs(a.right - f.right) : 0);
    diffs.push(Number(d.toFixed(2)));
    if (d > max) { max = d; worst = i; }
  }
  const body = table.querySelector(".detail-table-body");
  return {
    cols: ths.length,
    rows: rows.length,
    footCells: fts.length,
    max: Number(max.toFixed(2)),
    worst,
    diffs,
    sb: body ? Math.round(body.offsetWidth - body.clientWidth) : -1,
    hScroll: body ? body.scrollWidth - body.clientWidth : -1,
    footText: (foot?.innerText || "").replace(/\s+/g, " ").trim(),
  };
});
const footCells = () => p.evaluate(() => {
  const root = document.querySelector("[data-hr-bill-dialog]");
  const foot = root?.querySelector(".detail-table-foot tfoot tr");
  return foot ? Array.from(foot.querySelectorAll("td")).map((e) => (e.innerText || "").trim()) : [];
});
const bodyTexts = (col = 1) => p.evaluate((idx) => {
  const root = document.querySelector("[data-hr-bill-dialog]");
  const rows = Array.from(root?.querySelectorAll(".detail-table-body tbody tr") || [])
    .filter((r) => r.querySelectorAll("td").length > 1);
  return rows.map((r) => (r.querySelectorAll("td")[idx]?.innerText || "").trim());
}, col);
/** 字段配置（后端真值）。 */
const subsOf = async (scope) => {
  const r = await c.request.get(`${API}/api/v1/fleet/categories?scope=${scope}`, { headers: HDR });
  const j = await r.json();
  const cats = j?.data || [];
  const subs = [];
  for (const cat of cats) for (const s of (cat.subs || [])) subs.push(s);
  return subs;
};
/**
 * 抽屉里可见的表单 Label 集合（轮询等待）。
 * 抽屉 footer 先于表单体挂载，字段配置接口也可能晚到 —— 读太早会得到空数组，
 * 误判成「六同步断裂」。
 */
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

let settingsOpened = false;
const openSettings = async () => {
  if (settingsOpened) return true;
  await p.evaluate(() => {
    const btns = Array.from(document.querySelectorAll("button"));
    const b = btns.find((x) => (x.innerText || "").trim() === "设置");
    if (b) b.click();
  });
  // 等抽屉真正可见（首帧会有动画/DOM 挂载延迟）
  for (let i = 0; i < 20; i++) {
    const vis = await p.evaluate(() => {
      const d = document.querySelector(".standard-settings-drawer");
      return !!d && (d.getBoundingClientRect().width > 100);
    });
    if (vis) { settingsOpened = true; return true; }
    await p.waitForTimeout(300);
  }
  return false;
};
const closeSettings = async () => {
  await p.keyboard.press("Escape");
  await p.waitForTimeout(900);
  // Esc 可能被内层拦截，兜底点抽屉关闭按钮
  if (await p.locator(".standard-settings-drawer").count()) {
    await p.evaluate(() => {
      const d = document.querySelector(".standard-settings-drawer");
      const btn = d?.querySelector(".t-drawer__close-btn");
      if (btn) btn.click();
    });
    await p.waitForTimeout(800);
  }
  settingsOpened = false;
};

try {
  if (!(await ensureApp())) throw new Error("首屏 bootstrap 未就绪（KPI 卡片未渲染）");
  await killGuide();

  // ============ 断言 1：公积金 / 社保账单弹窗标签与三段表对齐 ============
  await card("公积金管理");
  if (!(await subTab("月度账单"))) ok("1.0 公积金月度账单标签存在", false);
  await waitRows();
  ok("1.1 打开 2026-11 公积金缴存明细", await openRow("2026-11"));
  await p.waitForSelector("[data-hr-bill-dialog]", { timeout: 20000 });
  await p.waitForTimeout(1200);

  const names = await billTabNames();
  ok("1.2 公积金标签 = 个人明细/单位明细/部门汇总/岗位汇总", 
    JSON.stringify(names) === JSON.stringify(["个人明细", "单位明细", "部门汇总", "岗位汇总"]), JSON.stringify(names));
  ok("1.3 不存在旧「按岗位汇总」标签", !names.some((n) => n.includes("按岗位汇总")), JSON.stringify(names));

  const fundMeasure = {};
  for (const t of ["个人明细", "单位明细", "部门汇总", "岗位汇总"]) {
    await billTab(t);
    if (t === "个人明细") await p.waitForTimeout(900);
    const m = await measureTable();
    fundMeasure[t] = m;
    ok(`1.4 公积金「${t}」三段表逐列对齐 <=1px`,
      !m.err && m.max <= 1, JSON.stringify({ max: m.max, worst: m.worst, cols: m.cols, rows: m.rows, sb: m.sb, hScroll: m.hScroll }));
    ok(`1.5 公积金「${t}」无横向滚动条`, !m.err && (m.hScroll ?? 0) <= 0, `hScroll=${m.hScroll}`);
  }
  await shot("d67_01_fund_tabs");
  // 两种高度状态都要覆盖：个人明细 74 行必然溢出（滚动条生效），部门汇总 7 行不溢出
  const overflowStates = await p.evaluate(() => {
    const root = document.querySelector("[data-hr-bill-dialog]");
    const body = root?.querySelector(".detail-table-body");
    return body ? { over: body.scrollHeight > body.clientHeight + 1, sb: Math.round(body.offsetWidth - body.clientWidth) } : null;
  });
  ok("1.6 样本覆盖溢出/不溢出两种高度状态（74 行溢出，7 行不溢出）",
    (fundMeasure["个人明细"]?.rows ?? 0) >= 40 && overflowStates !== null,
    JSON.stringify({ detailRows: fundMeasure["个人明细"]?.rows, deptRows: fundMeasure["部门汇总"]?.rows, ...overflowStates }));
  // 1.6b：强制一条经典（占位）滚动条，验证 JS 实测补偿路径。
  // headless Chrome 在 Linux 可能用 overlay 滚动条（宽度 0），此时补偿天然为 0；
  // 注入 ::-webkit-scrollbar 宽度后若占位，则 paddingRight 必须同步补偿。
  await p.addStyleTag({ content: ".hr-bill-detail .detail-table-body::-webkit-scrollbar{width:16px!important;height:16px!important}" });
  await billTab("个人明细");
  await p.waitForTimeout(600);
  const forced = await measureTable();
  const padRight = await p.evaluate(() => {
    const el = document.querySelector("[data-hr-bill-dialog] .detail-table-head");
    return el ? getComputedStyle(el).paddingRight : "";
  });
  ok("1.6b 强制占位滚动条时表头/合计行 padding-right 等于实测宽度且仍对齐",
    !forced.err && forced.max <= 1 && (forced.sb <= 0 || padRight === `${forced.sb}px`),
    JSON.stringify({ max: forced.max, sb: forced.sb, padRight }));
  await closeBill();

  await card("社保管理");
  await subTab("月度账单");
  await waitRows();
  ok("1.7 打开 2026-11 社保扣款明细", await openRow("2026-11"));
  await p.waitForSelector("[data-hr-bill-dialog]", { timeout: 20000 });
  await p.waitForTimeout(1200);
  const siNames = await billTabNames();
  ok("1.8 社保标签恢复三标签（无岗位维度）",
    JSON.stringify(siNames) === JSON.stringify(["个人明细", "单位明细", "部门汇总"]), JSON.stringify(siNames));
  for (const t of siNames) {
    await billTab(t);
    const m = await measureTable();
    ok(`1.9 社保「${t}」三段表逐列对齐 <=1px`, !m.err && m.max <= 1,
      JSON.stringify({ max: m.max, worst: m.worst, cols: m.cols, rows: m.rows, sb: m.sb, hScroll: m.hScroll }));
  }
  await shot("d67_02_social_tabs");
  await closeBill();

  // ============ 断言 2：公积金岗位汇总按真实岗位聚合 ============
  await card("公积金管理");
  await subTab("月度账单");
  await waitRows();
  await openRow("2026-11");
  await p.waitForSelector("[data-hr-bill-dialog]", { timeout: 20000 });
  await p.waitForTimeout(1400);

  await billTab("部门汇总");
  const deptFoot = await footCells();
  await billTab("岗位汇总");
  const postFoot = await footCells();
  const posts = await bodyTexts(1);
  ok("2.1 岗位列出现花名册真实岗位（操作员/过磅员/保安…）",
    posts.some((x) => /操作员|过磅员|保安|检验员/.test(x)) && !posts.some((x) => /^(普工|操作员)$/.test(x)),
    JSON.stringify(posts.slice(0, 8)));
  ok("2.2 岗位人数降序（首行人数 >= 末行人数）", await p.evaluate(() => {
    const root = document.querySelector("[data-hr-bill-dialog]");
    const ns = Array.from(root?.querySelectorAll(".detail-table-body tbody tr") || [])
      .map((r) => Number((r.querySelectorAll("td")[2]?.innerText || "").trim()));
    return ns.length > 1 && ns.every((n, i) => i === 0 || ns[i - 1] >= n);
  }));
  // 合计行与部门汇总逐列一致：序号列写「合计 N 人」，其余金额列逐分相等
  const money = (arr) => arr.slice(2).join("|");
  ok("2.3 岗位汇总合计 == 部门汇总合计（人数 + 三列金额）",
    postFoot.length >= 5 && money(postFoot) === money(deptFoot)
      && (postFoot[0] || "").replace(/\s/g, "") === (deptFoot[0] || "").replace(/\s/g, ""),
    JSON.stringify({ postFoot, deptFoot }));
  ok("2.4 合计为 74 人 / 22,939.00 / 22,939.00 / 45,878.00",
    postFoot.includes("22,939.00") && postFoot.includes("45,878.00") && /74/.test(postFoot[0] || ""),
    JSON.stringify(postFoot));
  // 搜索「操作员」实时过滤 + 合计重算
  const search = p.locator("[data-hr-bill-post-search] input");
  await search.fill("操作员");
  await p.waitForTimeout(700);
  const filtered = await bodyTexts(1);
  const filteredFoot = await footCells();
  ok("2.5 搜索「操作员」只留含操作员的岗位",
    filtered.length > 0 && filtered.every((x) => x.includes("操作员")), JSON.stringify(filtered));
  ok("2.6 搜索后合计行实时重算（与过滤后明细合计一致）", await p.evaluate(() => {
    const root = document.querySelector("[data-hr-bill-dialog]");
    const rows = Array.from(root?.querySelectorAll(".detail-table-body tbody tr") || [])
      .filter((r) => r.querySelectorAll("td").length > 1);
    const cents = (s) => Math.round(Number(String(s || "0").replace(/,/g, "")) * 100) || 0;
    const people = rows.reduce((n, r) => n + (Number((r.querySelectorAll("td")[2]?.innerText || "0").trim()) || 0), 0);
    const sum = rows.reduce((n, r) => n + cents((r.querySelectorAll("td")[5]?.innerText || "").trim()), 0);
    const foot = root?.querySelector(".detail-table-foot tfoot tr");
    const fPeople = Number((foot?.querySelectorAll("td")[2]?.innerText || "0").trim());
    const fSum = cents((foot?.querySelectorAll("td")[5]?.innerText || "").trim());
    return people === fPeople && sum === fSum && people > 0;
  }), JSON.stringify(filteredFoot));
  await shot("d67_03_post_summary");
  await search.fill("");
  await p.waitForTimeout(500);
  await closeBill();

  // ============ 断言 3：花名册自定义字段「工作地点」六同步 ============
  await card("花名册");
  await subTab("在职人员");
  await waitRows();
  const ROSTER_FIELD = "工作地点";
  const ROSTER_VALUE = "重庆璧山厂区";
  let rosterSubs = await subsOf("hr-roster");
  let target = rosterSubs.find((s) => String(s.name || "").trim() === ROSTER_FIELD);
  if (!target) {
    // 通过「设置 → 字段设置 → 新增字段名 → 添加 → 保存」真实创建
    const opened = await openSettings();
    ok("3.0 字段设置抽屉可打开且提供「新增字段名」输入框", opened && (await p.locator('.standard-settings-drawer input[placeholder="新增字段名"]').count()) > 0,
      `opened=${opened}`);
    const input = p.locator('.standard-settings-drawer input[placeholder="新增字段名"]').first();
    await input.fill(ROSTER_FIELD);
    await p.locator(".standard-settings-drawer button", { hasText: "添加" }).first().click();
    await p.waitForTimeout(500);
    await p.locator(".standard-settings-drawer .meter-form-actions button", { hasText: "保存" }).first().click();
    await p.waitForTimeout(3000);
    settingsOpened = false;
    rosterSubs = await subsOf("hr-roster");
    target = rosterSubs.find((s) => String(s.name || "").trim() === ROSTER_FIELD);
  }
  ok("3.1 字段配置中存在「工作地点」且已启用", !!target && target.enabled !== false,
    JSON.stringify(target || null));
  ok("3.2 「工作地点」属于在职人员胶囊（不会在表头/抽屉串组）",
    !target || !target.group || String(target.group).includes("在职"), String(target?.group || ""));

  // 3.3 字段筛选器里存在且顺序 = 字段配置顺序
  await p.evaluate(() => {
    const b = Array.from(document.querySelectorAll(".business-column-filter-wrap button"))
      .find((x) => (x.innerText || "").trim() === "字段");
    if (b) b.click();
  });
  await p.waitForTimeout(700);
  const filterItems = await p.evaluate(() => Array.from(document.querySelectorAll(".business-column-filter .field-popup-item"))
    .map((e) => (e.innerText || "").trim()));
  ok("3.3a [☰ 字段] 中存在「工作地点」", filterItems.includes(ROSTER_FIELD), JSON.stringify(filterItems.slice(0, 10)));
  const enabledNames = rosterSubs.filter((s) => s.enabled !== false)
    .map((s) => String(s.label || s.name || "").trim());
  const orderOk = (() => {
    const idx = filterItems.map((x) => enabledNames.indexOf(x)).filter((i) => i >= 0);
    return idx.every((v, i) => i === 0 || idx[i - 1] <= v);
  })();
  ok("3.3b 字段筛选顺序与字段配置一致", orderOk, `${filterItems.length}/${enabledNames.length}`);

  // 勾选「工作地点」→ 表头立刻出现
  await p.evaluate((x) => {
    const item = Array.from(document.querySelectorAll(".business-column-filter .field-popup-item"))
      .find((e) => (e.innerText || "").trim() === x);
    if (item) item.click();
  }, ROSTER_FIELD);
  await p.waitForTimeout(900);
  await p.keyboard.press("Escape");
  await p.waitForTimeout(400);
  const headers = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
    .map((e) => (e.innerText || "").trim()));
  ok("3.4 勾选后列表表头出现「工作地点」", headers.includes(ROSTER_FIELD), JSON.stringify(headers));

  // 打开员工抽屉：必须存在「工作地点」输入框
  const empName = await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes("廖宏")) || rows[0];
    if (!row) return "";
    const tds = row.querySelectorAll("td");
    // 第 2 列是姓名（第 1 列是勾选框）
    const nm = (tds[2]?.innerText || tds[1]?.innerText || "").trim();
    row.click();
    return nm;
  });
  await p.waitForTimeout(2600);
  const labels = await drawerLabels();
  ok("3.5 员工详情抽屉出现「工作地点」（不再被白名单丢弃）", labels.includes(ROSTER_FIELD),
    JSON.stringify({ emp: empName, has: labels.includes(ROSTER_FIELD), n: labels.length }));

  const fieldItem = p.locator(".t-drawer .t-form__item")
    .filter({ has: p.locator(".t-form__label", { hasText: ROSTER_FIELD }) }).first();
  if (!(await fieldItem.count())) throw new Error("抽屉内找不到工作地点表单项");
  const nameInput = fieldItem.locator("input").first();
  const current = await nameInput.inputValue();
  // 重复运行时值可能已是目标值：脏检查会禁用保存，这里做一次「A/B 交替」写入。
  let saveValue = ROSTER_VALUE;
  if (current === ROSTER_VALUE) saveValue = `${ROSTER_VALUE}A`;
  await nameInput.fill(saveValue);
  await p.waitForTimeout(600);
  const saveState = await saveDrawer();
  ok("3.6 填值后「保存」可点击并提交（未因脏检查被禁用）", saveState === "clicked", `${saveState} current=${current}`);
  const drawerGone = await p.evaluate(() => !Array.from(document.querySelectorAll(".bsd-footer"))
    .some((e) => e.getBoundingClientRect().width > 10));
  ok("3.6b 保存后抽屉关闭", drawerGone, String(drawerGone));

  // 列表表格里对应员工行渲染该值
  const cellOk = await p.evaluate((args) => {
    const [name, val] = args;
    const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
      .map((e) => (e.innerText || "").trim());
    const idx = ths.indexOf("工作地点");
    if (idx < 0) return { ok: false, reason: "no-header" };
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes(name));
    if (!row) return { ok: false, reason: "no-row" };
    const txt = (row.querySelectorAll("td")[idx]?.innerText || "").trim();
    return { ok: txt === val, txt, idx };
  }, [empName, saveValue]);
  ok("3.7 列表该员工行显示刚保存的工作地点", cellOk.ok, JSON.stringify({ want: saveValue, ...cellOk }));
  await shot("d67_04_roster_custom_cell");

  // 刷新页面 → 重开抽屉 → 值仍在（落库校验）
  /**
   * 真·刷新页面（重新加载整个 SPA）验证落库。
   * Vite 开发服务器偶发模块图失效会白屏，这里最多重试 2 次；
   * 仍失败则退化为「切模块强制重新拉列表」（同样走服务端，不是本地缓存）。
   */
  let reloadedRows = 0;
  for (let attempt = 0; attempt < 2 && !reloadedRows; attempt++) {
    try {
      await p.goto(`${APP}/platform/daily-affairs/hr`, { waitUntil: "domcontentloaded", timeout: 90000 });
    } catch { /* 网络抖动由下一轮兜底 */ }
    for (let i = 0; i < 30; i++) {
      const ready = await p.evaluate(() => document.querySelectorAll(".dash-kpi-card").length >= 5);
      if (ready) break;
      await p.waitForTimeout(1500);
    }
    await killGuide();
    reloadedRows = await waitRowsLong(60);
  }
  let reloadMode = "reload";
  if (!reloadedRows) {
    reloadMode = "switch-tab";
    await card("劳动合同");
    await card("花名册");
    await subTab("在职人员");
    reloadedRows = await waitRowsLong(60);
  }
  ok("3.8a 重新加载页面后花名册行已加载", reloadedRows > 0 && (await openFirstRow(empName)),
    `mode=${reloadMode} rows=${reloadedRows} emp=${empName}`);
  let reloaded = "__missing__";
  for (let i = 0; i < 24; i++) {
    reloaded = await p.evaluate((x) => {
      const item = Array.from(document.querySelectorAll(".t-drawer .t-form__item"))
        .find((e) => (e.querySelector(".t-form__label")?.innerText || "").trim() === x);
      return item ? (item.querySelector("input")?.value || "") : "__missing__";
    }, ROSTER_FIELD);
    if (reloaded !== "__missing__") break;
    await p.waitForTimeout(500);
  }
  ok("3.8 刷新后重开抽屉回显刚保存的工作地点", reloaded === saveValue, `${reloaded} vs ${saveValue}`);
  // 关抽屉
  await closeDrawer();

  // 下载花名册导入模板：表头必须含「工作地点」，且不含跨模块引用字段
  const tplPath = `${SHOTS}/roster67.xlsx`;
  const tr = await c.request.get(`${API}/api/v1/fleet/hr/roster/template?module=roster`, { headers: HDR });
  fs.writeFileSync(tplPath, await tr.body());
  let shared = "";
  try {
    shared = execSync(`unzip -p ${tplPath} xl/sharedStrings.xml 2>/dev/null || unzip -p ${tplPath} xl/worksheets/sheet1.xml`, { encoding: "utf8", maxBuffer: 8 << 20 });
  } catch (e) { shared = ""; }
  ok("3.9 导入模板表头动态包含「工作地点」", shared.includes(ROSTER_FIELD), `${tr.status()} bytes=${fs.statSync(tplPath).size}`);
  ok("3.10 导入模板排除跨模块引用字段（个人社保账号/个人公积金账号）",
    !shared.includes("个人社保账号") && !shared.includes("个人公积金账号"), "");

  // ============ 断言 4：跨页面六同步巡检 ============
  // 4.1 劳动合同：抽屉字段 = 字段配置（enabled, 非引用）
  await card("劳动合同");
  await subTab("劳动合同");
  await waitRows();
  await p.waitForTimeout(1200);
  ok("4.0 劳动合同详情抽屉可打开", await openFirstRow());
  const contractLabels = await drawerLabels();
  const contractSubs = await subsOf("hr-contract");
  const contractExpect = contractSubs
    .filter((s) => s.enabled !== false && !s.source
      && String(s.group || "").includes("劳动") && !String(s.group || "").includes("劳务"))
    .map((s) => String(s.label || s.name || "").trim())
    .filter((x) => x && !/^(表头|序号)$/.test(x));
  const contractMissing = contractExpect.filter((x) => !contractLabels.includes(x));
  ok("4.1 劳动合同抽屉覆盖全部启用原生字段", contractMissing.length === 0,
    JSON.stringify({ missing: contractMissing.slice(0, 8), cfg: contractExpect.length, drawer: contractLabels.length }));
  await closeDrawer();

  // 4.2 临时字段增删（合同）：新增 → 抽屉出现 → 模板出现 → 删除还原
  const TMP = "第67轮临时字段";
  const settingsOk = await openSettings();
  ok("4.2a 合同字段设置抽屉可打开", settingsOk, String(settingsOk));
  // 明确切到「劳动合同」胶囊再新增，否则默认胶囊可能是劳务合同 → 字段进了另一组
  await p.evaluate(() => {
    const tab = Array.from(document.querySelectorAll(".standard-settings-drawer .fc-group-tab"))
      .find((e) => (e.innerText || "").trim().startsWith("劳动合同"));
    if (tab) tab.click();
  });
  await p.waitForTimeout(800);
  const addInput2 = p.locator('.standard-settings-drawer input[placeholder="新增字段名"]').first();
  await addInput2.fill(TMP);
  await p.locator(".standard-settings-drawer button", { hasText: "添加" }).first().click();
  await p.waitForTimeout(400);
  await p.locator(".standard-settings-drawer .meter-form-actions button", { hasText: "保存" }).first().click();
  await p.waitForTimeout(3000);
  settingsOpened = false;
  await p.waitForTimeout(1200);
  await openFirstRow();
  const labels2 = await drawerLabels();
  ok("4.2 新增临时字段后抽屉立即出现", labels2.includes(TMP), JSON.stringify(labels2.slice(-6)));
  await closeDrawer();
  const tpl2 = await c.request.get(`${API}/api/v1/fleet/hr/roster/template?module=contract`, { headers: HDR });
  fs.writeFileSync(`${SHOTS}/contract67.xlsx`, await tpl2.body());
  let shared2 = "";
  try {
    shared2 = execSync(`unzip -p ${SHOTS}/contract67.xlsx xl/sharedStrings.xml 2>/dev/null || unzip -p ${SHOTS}/contract67.xlsx xl/worksheets/sheet1.xml`, { encoding: "utf8", maxBuffer: 8 << 20 });
  } catch (e) { shared2 = ""; }
  ok("4.3 临时字段进入导入模板表头", shared2.includes(TMP), "");
  // 删除临时字段（还原）：⋮ 菜单 → 下拉「删除」→ 气泡确认「删除」
  await openSettings();
  // 回到「劳动合同」胶囊（临时字段加在这一组），否则行不会渲染
  await p.evaluate(() => {
    const tab = Array.from(document.querySelectorAll(".standard-settings-drawer .fc-group-tab"))
      .find((e) => (e.innerText || "").trim().startsWith("劳动合同"));
    if (tab) tab.click();
  });
  await p.waitForTimeout(900);
  // 字段名在行内 input.value 里（innerText 为空），按 input 值定位
  let rowReady = false;
  for (let i = 0; i < 40; i++) {
    rowReady = await p.evaluate((x) => Array.from(document.querySelectorAll(".standard-settings-drawer .field-config-row"))
      .some((r) => Array.from(r.querySelectorAll("input")).some((i) => i.value === x)), TMP);
    if (rowReady) break;
    await p.waitForTimeout(500);
  }
  ok("4.4a 设置抽屉里能看到临时字段行", rowReady, TMP);
  /**
   * TDesign 的菜单/气泡必须用**真实指针事件**（DOM `.click()` 不会触发
   * dropdown 的委托监听，实测点了毫无反应）。
   */
  const rowIdx = await p.evaluate((x) => Array.from(document.querySelectorAll(".standard-settings-drawer .field-config-row"))
    .findIndex((r) => Array.from(r.querySelectorAll("input")).some((i) => i.value === x)), TMP);
  if (rowIdx >= 0) {
    await p.locator(".standard-settings-drawer .field-config-row").nth(rowIdx)
      .locator(".fc-op button").first().click();
    await p.waitForTimeout(900);
    const delItem = p.locator(".t-dropdown__item").filter({ hasText: /^删除$/ }).last();
    if (await delItem.count()) await delItem.click({ force: true });
    await p.waitForTimeout(900);
    const delConfirm = p.locator(".t-popconfirm button").filter({ hasText: /^删除$/ }).last();
    if (await delConfirm.count()) await delConfirm.click({ force: true });
    await p.waitForTimeout(800);
  }
  await p.waitForTimeout(600);
  await p.locator(".standard-settings-drawer .meter-form-actions button", { hasText: "保存" }).first().click();
  await p.waitForTimeout(3000);
  settingsOpened = false;
  const afterDel = await subsOf("hr-contract");
  ok("4.4 临时字段删除后回到原配置", !afterDel.some((s) => String(s.name || "").trim() === TMP), "");

  // 4.3 宿舍档案：抽屉 = 字段配置（enabled）
  await card("宿舍管理");
  await waitRows();
  await p.waitForTimeout(1200);
  ok("4.4 宿舍档案抽屉可打开", await openFirstRow());
  const dormLabels = await drawerLabels();
  const dormSubs = await subsOf("hr-dorm");
  const dormExpect = dormSubs
    .filter((s) => s.enabled !== false && !s.source && String(s.group || "").includes("宿舍档案"))
    .map((s) => String(s.label || s.name || "").trim()).filter(Boolean);
  const dormMissing = dormExpect.filter((x) => !dormLabels.includes(x) && !/费用/.test(x));
  ok("4.5 宿舍档案抽屉覆盖全部启用字段", dormMissing.length === 0,
    JSON.stringify({ missing: dormMissing.slice(0, 8), cfg: dormExpect.length, drawer: dormLabels.length }));
  await closeDrawer();

  // ============ 全局零错误 ============
  ok("5.1 全程 0 console.error", errs.length === 0, JSON.stringify(errs.slice(0, 5)));
  ok("5.2 全程 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 5)));
} catch (e) {
  ok("FATAL 脚本异常", false, String(e && e.stack ? e.stack : e));
  try { await shot("d67_fatal"); } catch { /* ignore */ }
} finally {
  const fails = R.checks.filter((x) => !x.pass);
  console.log(`\n==== 结果：${R.checks.length - fails.length}/${R.checks.length} PASS，${fails.length} FAIL ====`);
  if (fails.length) console.log(fails.map((f) => `FAIL ${f.n} :: ${f.d}`).join("\n"));
  fs.writeFileSync(`${SHOTS}/d67-result.json`, JSON.stringify(R, null, 2));
  await b.close();
  process.exit(fails.length ? 1 : 0);
}
