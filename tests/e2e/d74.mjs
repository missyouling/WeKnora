// 第 74 轮真机验收：
//   A1 设置 · 能耗：7 个平级标签 / 市电分组默认折叠 / 光伏字段平铺 / 唯一底部保存 /
//      能耗历史零波动（电 123·165,381.00 / 水 151·34,156.96 / 气 86·12,769.41）
//   A2 当前页列表全字段子串关键字筛选（花名册 / 劳动合同 / 社保回盘表 /
//      公积金职工明细 / 宿舍档案 / 能耗电费核算），清空后恢复全量
//   A3 抽屉拖拽锚点与知识库文档详情抽屉 1:1（宽高 / 左缘偏移 / 圆角 / 悬停色）
//
// 运行：cat tests/e2e/d74.mjs | ssh weknora-dev 'cat > /opt/pw-run/d74.mjs'
//       ssh weknora-dev 'cd /opt/pw-run && node d74.mjs'
import { chromium } from "playwright";
import fs from "node:fs";

const APP = process.env.WEKNORA_E2E_APP || "http://127.0.0.1:5173";
const API = process.env.WEKNORA_E2E_API || "http://127.0.0.1:8080";
const SHOTS = process.env.CI_SHOTS_DIR || "/tmp/pw/shots";
const TK = fs.readFileSync(process.env.WEKNORA_TOKEN_FILE || "/tmp/pw/token.txt", "utf8").trim();
const HDR = { Authorization: `Bearer ${TK}`, "X-Tenant-ID": "10000" };
const ENERGY_KB = process.env.WEKNORA_ENERGY_KB || "270af1db-4fd8-43f2-a0e4-dcb823232b95";
fs.mkdirSync(SHOTS, { recursive: true });

const api = async (path, base = `${API}/api/v1/utilities`) => {
  const res = await fetch(`${base}${path}`, { headers: HDR });
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
p.on("pageerror", (e) => errs.push("pageerror: " + String(e?.stack || e?.message || e).slice(0, 400)));
p.on("response", (r) => {
  if (r.status() >= 500 && !/favicon/.test(r.url())) bad5xx.push(`${r.status()} ${r.url().slice(0, 120)}`);
});

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
const card = async (label) => {
  await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes(x));
    if (e) e.click();
  }, label);
  await p.waitForTimeout(2500);
};
const subTab = async (label) => {
  const done = await p.evaluate((x) => {
    const e = Array.from(document.querySelectorAll(".hr-subview-bar .t-radio-button"))
      .find((q) => (q.innerText || "").trim() === x);
    if (e) { e.click(); return true; }
    return false;
  }, label);
  await p.waitForTimeout(1800);
  return done;
};
const rowSel = ".doc-list-scroll tbody tr";
const rowCount = () => p.evaluate((sel) => Array.from(document.querySelectorAll(sel))
  .filter((r) => r.querySelectorAll("td").length > 3).length, rowSel);
const rowsText = () => p.evaluate((sel) => Array.from(document.querySelectorAll(sel))
  .filter((r) => r.querySelectorAll("td").length > 3)
  .map((r) => (r.innerText || "").replace(/\s+/g, " ").trim()), rowSel);
const waitRows = async (seconds = 60) => {
  const deadline = Date.now() + seconds * 1000;
  while (Date.now() < deadline) {
    const n = await rowCount();
    if (n > 0) return n;
    await p.waitForTimeout(1500);
  }
  return 0;
};
/** 行数稳定后再断言：搜索走 300ms 防抖 + 一次列表请求，固定 sleep 会偶发读到上一帧。 */
const waitStableCount = async (maxMs = 16000) => {
  const t0 = Date.now();
  let last = -1;
  let stable = 0;
  while (Date.now() - t0 < maxMs) {
    const n = await rowCount();
    if (n === last) stable += 1; else { stable = 0; last = n; }
    if (stable >= 2) return n;
    await p.waitForTimeout(700);
  }
  return last;
};
/**
 * 输入关键字并等待"列表真的换了数据"。
 *
 * 防抖 300ms + 一次列表请求的延迟在不同接口上差异很大（花名册 86 行带联表最慢），
 * 固定 sleep 或"计数稳定两帧就返回"都会偶发读到上一帧的旧行 —— 因此按**期望值**
 * 轮询：narrowing 用例等到条数与基线不同，清空用例等到回到全量，再做稳定收敛。
 */
const fillSearch = async (kw, baseline = null) => {
  const input = p.locator(".doc-search-field input").first();
  await input.click();
  await input.fill(kw);
  await p.waitForTimeout(500);
  if (baseline !== null) {
    const deadline = Date.now() + 20000;
    while (Date.now() < deadline) {
      const n = await rowCount();
      if (n !== baseline) break;
      await p.waitForTimeout(600);
    }
  }
  return waitStableCount(8000);
};
const clearSearch = async (expectFull = null) => {
  const input = p.locator(".doc-search-field input").first();
  await input.fill("");
  await p.waitForTimeout(500);
  if (expectFull !== null) {
    const deadline = Date.now() + 20000;
    while (Date.now() < deadline) {
      const n = await rowCount();
      if (n === expectFull) break;
      await p.waitForTimeout(600);
    }
  }
  return waitStableCount(8000);
};
const drawerOpen = () => p.evaluate(() => Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
  .some((w) => w.getBoundingClientRect().width > 200 && w.getBoundingClientRect().height > 200));
const clickSettings = async () => {
  // 等工具栏渲染完成再点（切换卡片后子视图是异步挂载的）
  let clicked = false;
  for (let i = 0; i < 10 && !clicked; i++) {
    clicked = await p.evaluate(() => {
      const btns = Array.from(document.querySelectorAll("button"))
        .filter((x) => (x.innerText || "").trim() === "设置" && x.getBoundingClientRect().width > 10);
      if (!btns.length) return false;
      btns[btns.length - 1].click();
      return true;
    });
    if (!clicked) await p.waitForTimeout(1000);
  }
  for (let i = 0; i < 14; i++) {
    if (await drawerOpen()) { await p.waitForTimeout(1800); return true; }
    await p.waitForTimeout(900);
  }
  return false;
};
const closeTopDrawer = async () => {
  await p.evaluate(() => {
    const wrappers = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200);
    const w = wrappers[wrappers.length - 1];
    if (!w) return;
    const btn = Array.from(w.querySelectorAll("button"))
      .find((x) => /^(取消|关闭)$/.test((x.innerText || "").trim()) && x.getBoundingClientRect().width > 10);
    if (btn) btn.click();
  });
  await p.waitForTimeout(1200);
  if (await p.evaluate(() => Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .some((w) => w.getBoundingClientRect().width > 200))) {
    await p.keyboard.press("Escape");
    await p.waitForTimeout(900);
  }
};

// ============================================================ 首屏
const booted = await ensureApp("/platform/daily-affairs/hr");
if (!booted) throw new Error("首屏 bootstrap 未就绪");

// ============================================================ A1 设置 · 能耗
try {
  await ensureApp("/platform/daily-affairs/utilities");
  await card("水费核算");
  await waitRows(40);
  const opened = await clickSettings();
  ok("A1.1 水费核算「设置」可打开", opened);

  const energyTabs = ["能耗账单", "提取规则", "电费核算", "水费核算", "气费核算", "费用清单", "工业表计"];
  const tabs = await p.evaluate(() => Array.from(document.querySelectorAll(".energy-settings-drawer .t-tabs__nav-item"))
    .map((e) => (e.innerText || "").trim()).filter(Boolean));
  ok("A1.2 顶层同一行严格 7 个平级标签", JSON.stringify(tabs) === JSON.stringify(energyTabs), JSON.stringify(tabs));
  const activeTab = await p.evaluate(() => (document.querySelector(".energy-settings-drawer .t-tabs__nav-item.t-is-active")?.innerText || "").trim());
  ok("A1.3 从水费核算进入自动激活「水费核算」标签", activeTab === "水费核算", activeTab);

  // 能耗账单 → 市电账单
  await p.evaluate(() => {
    const it = Array.from(document.querySelectorAll(".energy-settings-drawer .t-tabs__nav-item"))
      .find((x) => (x.innerText || "").trim() === "能耗账单");
    if (it) it.click();
  });
  await p.waitForTimeout(3500);
  const bill = await p.evaluate(() => {
    const d = document.querySelector(".energy-settings-drawer .t-drawer__content-wrapper");
    if (!d) return null;
    return {
      hasTypeName: (d.innerText || "").includes("能耗类型名称"),
      hasSample: (d.innerText || "").includes("样票生成字段"),
      hasAddCategory: (d.innerText || "").includes("新增能耗账单"),
      rows: d.querySelectorAll(".field-config-row").length,
      pills: Array.from(d.querySelectorAll(".fc-group-tab")).length,
      collapsed: !!d.querySelector("[data-fc-groups-collapsed]"),
      activePill: (d.querySelector(".fc-group-tab.active")?.innerText || "").trim(),
    };
  });
  ok("A1.4 市电账单：无能耗类型名称 / 无样票生成字段 / 无新增能耗账单",
    bill && !bill.hasTypeName && !bill.hasSample && !bill.hasAddCategory, JSON.stringify(bill));
  ok("A1.5 市电账单：分组胶囊存在且默认全部折叠（0 行字段 + 折叠提示）",
    bill && bill.pills >= 4 && bill.rows === 0 && bill.collapsed && !bill.activePill, JSON.stringify(bill));

  // 点第一个分组 → 按需展开
  await p.evaluate(() => {
    const pill = document.querySelector(".energy-settings-drawer .fc-group-tab");
    if (pill) pill.click();
  });
  await p.waitForTimeout(1600);
  const expandedRows = await p.evaluate(() => document.querySelectorAll(".energy-settings-drawer .field-config-row").length);
  ok("A1.6 点击分组头按需展开字段", expandedRows > 0, `rows=${expandedRows}`);

  // 光伏账单：无派生字段/账单概况胶囊，字段平铺
  await p.evaluate(() => {
    const r = Array.from(document.querySelectorAll("[data-energy-bill-switch] .t-radio-button"))
      .find((x) => (x.innerText || "").trim() === "光伏账单");
    if (r) r.click();
  });
  await p.waitForTimeout(3500);
  const solar = await p.evaluate(() => {
    const d = document.querySelector(".energy-settings-drawer .t-drawer__content-wrapper");
    if (!d) return null;
    return {
      pills: Array.from(d.querySelectorAll(".fc-group-tab")).map((e) => (e.innerText || "").trim()),
      rows: d.querySelectorAll(".field-config-row").length,
      text: (d.innerText || "").replace(/\s+/g, " "),
    };
  });
  ok("A1.7 光伏账单：无「派生字段（不可提取）」与「账单概况」分组胶囊",
    solar && !solar.pills.some((x) => /派生字段|账单概况/.test(x)), JSON.stringify(solar?.pills || []));
  ok("A1.8 光伏账单：字段直接平铺展示", solar && solar.rows >= 10, `rows=${solar?.rows}`);
  await shot("d74-a1-solar");

  // 遍历 7 个标签：可见「保存」严格 === 1
  const saveCounts = {};
  for (const label of energyTabs) {
    await p.evaluate((lb) => {
      const it = Array.from(document.querySelectorAll(".energy-settings-drawer .t-tabs__nav-item"))
        .find((x) => (x.innerText || "").trim() === lb);
      if (it) it.click();
    }, label);
    await p.waitForTimeout(1800);
    saveCounts[label] = await p.evaluate(() => {
      const d = document.querySelector(".energy-settings-drawer .t-drawer__content-wrapper");
      if (!d) return -1;
      return Array.from(d.querySelectorAll("button"))
        .filter((x) => (x.innerText || "").trim() === "保存" && x.getBoundingClientRect().width > 10).length;
    });
  }
  ok("A1.9 7 个标签均只保留唯一一套「保存」按钮",
    Object.values(saveCounts).every((n) => n === 1), JSON.stringify(saveCounts));
  await closeTopDrawer();

  // 页面联动：从「光伏账单」卡片点设置 → 能耗账单 + 光伏账单二级切换
  await card("光伏账单");
  await clickSettings();
  for (let i = 0; i < 10; i++) {
    const ready = await p.evaluate(() => !!document.querySelector(".energy-settings-drawer .t-tabs__nav-item"));
    if (ready) break;
    await p.waitForTimeout(900);
  }
  const link = await p.evaluate(() => {
    const d = document.querySelector(".energy-settings-drawer .t-drawer__content-wrapper");
    const tab = (document.querySelector(".energy-settings-drawer .t-tabs__nav-item.t-is-active")?.innerText || "").trim();
    const radio = Array.from(d?.querySelectorAll("[data-energy-bill-switch] .t-radio-button") || [])
      .find((x) => x.classList.contains("t-is-checked"));
    return { tab, bill: (radio?.innerText || "").trim() };
  });
  ok("A1.12 光伏账单「设置」联动到能耗账单 → 光伏账单", link.tab === "能耗账单" && link.bill === "光伏账单", JSON.stringify(link));
  await closeTopDrawer();

  // 能耗历史零波动
  const expect = { electricity: [123, 165381], water: [151, 34156.96], gas: [86, 12769.41] };
  const hist = {};
  for (const [cat, [cnt, amt]] of Object.entries(expect)) {
    const r = await api(`/meter-records?category=${cat}`);
    const recs = r.body?.data?.records || [];
    const items = recs.reduce((a, x) => a + (x.items?.length || 0), 0);
    const sum = recs.reduce((a, x) => a + Number(x.total_amount || 0), 0);
    hist[cat] = { items, sum: Math.round(sum * 100) / 100, want: [cnt, amt] };
    ok(`A1.10 能耗历史零波动 · ${cat}`, items === cnt && Math.abs(sum - amt) < 0.02, `${items} / ${sum.toFixed(2)}`);
  }
  ok("A1.11 三类历史全部命中基准", Object.values(hist).every((h) => h.want[0] === h.items), JSON.stringify(hist));
} catch (e) {
  ok("A1 设置 · 能耗重构", false, String(e).slice(0, 300));
}

// ============================================================ A2 全字段关键字筛选
const keywordCases = [
  { name: "花名册", kw: "大拉", nav: async () => { await card("花名册"); await waitRows(40); } },
  { name: "劳动合同", kw: "生产部", nav: async () => { await card("劳动合同"); await subTab("劳动合同"); await waitRows(40); } },
  { name: "社保回盘表", kw: "生产部", nav: async () => { await card("社保管理"); await subTab("回盘表"); await waitRows(40); } },
  { name: "公积金职工明细", kw: "生产部", noDomHit: true, nav: async () => { await card("公积金管理"); await subTab("职工明细"); await waitRows(40); } },
  { name: "宿舍档案", kw: "重庆星", nav: async () => { await card("宿舍管理"); await subTab("宿舍档案"); await waitRows(40); } },
];
try {
  await ensureApp("/platform/daily-affairs/hr");
  for (const kase of keywordCases) {
    await kase.nav();
    const full = await waitStableCount();
    const filtered = await fillSearch(kase.kw, full);
    const texts = await rowsText();
    const hit = texts.filter((t) => t.includes(kase.kw)).length;
    ok(`A2 ${kase.name}：搜索「${kase.kw}」精准收敛`, full > 0 && filtered > 0 && filtered < full, `full=${full} filtered=${filtered}`);
    if (!kase.noDomHit) ok(`A2 ${kase.name}：命中行确实包含关键字`, hit > 0, `hit=${hit}/${filtered}`);
    const restored = await clearSearch(full);
    ok(`A2 ${kase.name}：清空后恢复全量`, restored === full, `full=${full} restored=${restored}`);
  }
  // 花名册：单位名任意连续子串（重庆星 / 星达 / 铜业）
  await card("花名册");
  await waitRows(40);
  for (const kw of ["重庆星", "星达", "铜业"]) {
    const n = await fillSearch(kw);
    ok(`A2 花名册：单位名局部子串「${kw}」可命中`, n > 0, `rows=${n}`);
  }
  await clearSearch();
  // 公积金职工明细默认表头不含「部门」列，DOM 里看不到关键字本身：
  // 用接口侧同口径（JSON 全字段子串）核对收敛条数，避免把「列被隐藏」误判成过滤失败。
  // 页面默认按「正常」状态加载，接口口径必须带同一个 status 才能与列表基数对齐
  const fundApi = await api(`/fund/records?status=${encodeURIComponent("正常")}`, `${API}/api/v1/fleet/hr`);
  const fundData = fundApi.body?.data;
  const fundRows = Array.isArray(fundData) ? fundData : (fundData?.items || []);
  const fundExpect = fundRows.filter((r) => JSON.stringify(r).includes("生产部")).length;
  await card("公积金管理");
  await subTab("职工明细");
  await waitRows(40);
  const fundFiltered = await fillSearch("生产部");
  ok("A2 公积金职工明细：收敛条数与接口全字段子串一致", fundFiltered === fundExpect, `ui=${fundFiltered} api=${fundExpect}`);
  await clearSearch();

  // 能耗 · 电费核算（表计/使用单位/表号 全字段）
  await ensureApp("/platform/daily-affairs/utilities");
  await card("电费核算");
  const energyFull = await waitRows(40);
  const energyFiltered = await fillSearch("宿舍301", energyFull);
  ok("A2 能耗电费核算：搜索「宿舍301」精准收敛", energyFull > 0 && energyFiltered > 0 && energyFiltered < energyFull,
    `full=${energyFull} filtered=${energyFiltered}`);
  const energyRestored = await clearSearch(energyFull);
  ok("A2 能耗电费核算：清空后恢复全量", energyRestored === energyFull, `full=${energyFull} restored=${energyRestored}`);
} catch (e) {
  ok("A2 全字段关键字筛选", false, String(e).slice(0, 300));
}

// ============================================================ A3 抽屉拖拽锚点
const parkMouse = async () => { await p.mouse.move(4, 4); await p.waitForTimeout(350); };
const measureSetting = (title) => p.evaluate((t) => {
  const panels = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
    .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").includes(t));
  const panel = panels[panels.length - 1];
  if (!panel) return { found: false };
  const h = panel.querySelector("[data-setting-drawer-resize]");
  if (!h) return { found: false, reason: "no-handle" };
  const hr = h.getBoundingClientRect();
  const grip = h.querySelector(".setting-drawer-resize-grip");
  const gs = grip ? getComputedStyle(grip) : null;
  const gr = grip ? grip.getBoundingClientRect() : null;
  const pr = panel.getBoundingClientRect();
  return {
    found: true,
    handleW: Math.round(hr.width),
    handleH: Math.round(hr.height),
    leftOffset: Math.round(hr.left - pr.left),
    centerDev: Math.round((hr.top + hr.height / 2) - window.innerHeight / 2),
    gripW: gr ? +gr.width.toFixed(1) : null,
    gripH: gr ? +gr.height.toFixed(1) : null,
    radius: gs?.borderRadius || "",
    opacity: gs?.opacity || "",
    bg: gs?.backgroundColor || "",
  };
}, title);
const hoverGrip = async (title) => {
  await p.evaluate((t) => {
    const panels = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").includes(t));
    const panel = panels[panels.length - 1];
    const h = panel?.querySelector("[data-setting-drawer-resize]");
    // 热区 12px 以抽屉边缘为轴：取靠内侧 3px 处的点，确保落在抽屉这一侧
    if (h) { const r = h.getBoundingClientRect(); window.__hoverTarget = [r.left + r.width - 3, r.top + r.height / 2]; }
  }, title);
  const pt = await p.evaluate(() => window.__hoverTarget || null);
  if (!pt) return null;
  await p.mouse.move(pt[0], pt[1], { steps: 6 });
  await p.waitForTimeout(1000);
  return p.evaluate((t) => {
    const panels = Array.from(document.querySelectorAll(".t-drawer__content-wrapper"))
      .filter((w) => w.getBoundingClientRect().width > 200 && (w.innerText || "").includes(t));
    const panel = panels[panels.length - 1];
    const grip = panel?.querySelector(".setting-drawer-resize-grip");
    return grip ? getComputedStyle(grip).backgroundColor : null;
  }, title);
};

try {
  // 基准：知识库文档详情抽屉（上游原生实现）。
  // 用独立 page 打开，避免前面 ensureApp 的多次 reload 留下隐藏抽屉实例干扰 hover。
  const bp = await c.newPage();
  bp.setDefaultTimeout(30000);
  await bp.goto(`${APP}/platform/knowledge-bases/${ENERGY_KB}`, { waitUntil: "domcontentloaded", timeout: 90000 }).catch(() => {});
  await bp.waitForTimeout(26000);
  await bp.evaluate(() => {
    const guide = document.querySelectorAll(".guide");
    guide.forEach((e) => e.remove());
    const btn = document.querySelectorAll(".row-file-name")[0];
    if (btn) btn.click();
  });
  await bp.waitForTimeout(7000);
  const base = await bp.evaluate(() => {
    const all = Array.from(document.querySelectorAll(".doc-drawer-resize-handle"))
      .filter((x) => { const r = x.getBoundingClientRect(); return r.width > 5 && r.height > 100; });
    const h = all[all.length - 1];
    if (!h) return { found: false };
    const line = h.querySelector(".doc-drawer-resize-line");
    const ls = line ? getComputedStyle(line) : null;
    const lr = line ? line.getBoundingClientRect() : null;
    const hr = h.getBoundingClientRect();
    const panel = document.querySelector(".doc-main-drawer .t-drawer__content-wrapper")
      || document.querySelector(".doc-main-drawer");
    const pr = panel ? panel.getBoundingClientRect() : { left: 0 };
    return {
      found: true,
      handleW: Math.round(hr.width),
      handleH: Math.round(hr.height),
      leftOffset: Math.round(hr.left - pr.left),
      centerDev: Math.round((hr.top + hr.height / 2) - window.innerHeight / 2),
      gripW: lr ? +lr.width.toFixed(1) : null,
      gripH: lr ? +lr.height.toFixed(1) : null,
      radius: ls?.borderRadius || "",
      opacity: ls?.opacity || "",
      bg: ls?.backgroundColor || "",
    };
  });
  ok("A3.0 知识库文档详情抽屉可打开并采集基准", base.found, JSON.stringify(base));
  let baseHover = null;
  if (base.found) {
    const baseHandle = bp.locator(".doc-drawer-resize-handle").last();
    await baseHandle.hover({ timeout: 8000 }).catch(() => {});
    await bp.waitForTimeout(1200);
    baseHover = await baseHandle.evaluate((el) => {
      const line = el.querySelector(".doc-drawer-resize-line");
      return line ? getComputedStyle(line).backgroundColor : null;
    }).catch(() => null);
  }
  await bp.screenshot({ path: `${SHOTS}/d74-a3-doc.png` }).catch(() => {});
  await bp.close();
  ok("A3.0b 基准手柄悬停高亮色已采集", !!baseHover && baseHover !== base.bg, `${baseHover} vs 常态 ${base.bg}`);

  const cmp = (name, m) => {
    if (!base.found || !m.found) { ok(`A3 ${name}`, false, JSON.stringify(m)); return; }
    const okW = Math.abs(m.handleW - base.handleW) <= 1;
    const okH = Math.abs(m.handleH - base.handleH) <= 1;
    const okGripW = Math.abs((m.gripW || 0) - (base.gripW || 0)) <= 1;
    const okGripH = Math.abs((m.gripH || 0) - (base.gripH || 0)) <= 1;
    const okLeft = Math.abs(m.leftOffset - base.leftOffset) <= 1;
    const okRadius = m.radius === base.radius;
    ok(`A3 ${name}：手柄几何与知识库 1:1`, okW && okH && okGripW && okGripH && okLeft && okRadius,
      `${JSON.stringify(m)} vs ${JSON.stringify(base)}`);
  };

  // 花名册详情
  await ensureApp("/platform/daily-affairs/hr");
  await card("花名册");
  await waitRows(40);
  await p.evaluate(() => { const r = document.querySelector(".doc-list-scroll tbody tr"); if (r) (r.querySelector("td") || r).click(); });
  await p.waitForTimeout(4500);
  await parkMouse();
  let m = await measureSetting("员工详情");
  ok("A3.a 花名册详情抽屉已打开", m.found, JSON.stringify(m));
  cmp("花名册详情", m);
  const hittable = await p.evaluate(() => {
    const h = Array.from(document.querySelectorAll("[data-setting-drawer-resize]"))
      .filter((x) => x.getBoundingClientRect().height > 100).pop();
    if (!h) return false;
    const r = h.getBoundingClientRect();
    const el = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return !!el && (el === h || h.contains(el));
  });
  ok("A3.a1 拖拽热区真实可命中（无遮挡裁剪）", hittable, String(hittable));
  const hover1 = await hoverGrip("员工详情");
  ok("A3.a2 花名册详情悬停高亮色与知识库一致", hover1 && baseHover && hover1 === baseHover, `${hover1} vs ${baseHover}`);
  await closeTopDrawer();

  // 合同详情
  await card("劳动合同");
  await waitRows(40);
  await p.evaluate(() => { const r = document.querySelector(".doc-list-scroll tbody tr"); if (r) (r.querySelector("td") || r).click(); });
  await p.waitForTimeout(4500);
  await parkMouse();
  m = await measureSetting("合同详情");
  ok("A3.b 合同详情抽屉已打开", m.found, JSON.stringify(m));
  cmp("合同详情", m);
  await closeTopDrawer();

  // 宿舍详情
  await card("宿舍管理");
  await subTab("宿舍档案");
  await waitRows(40);
  await p.evaluate(() => { const r = document.querySelector(".doc-list-scroll tbody tr"); if (r) (r.querySelector("td") || r).click(); });
  await p.waitForTimeout(4500);
  await parkMouse();
  m = await measureSetting("宿舍");
  ok("A3.c 宿舍详情抽屉已打开", m.found, JSON.stringify(m));
  cmp("宿舍详情", m);
  await closeTopDrawer();

  // 宿舍管理设置（中台 StandardSettingDrawer）
  const hrSettings = await clickSettings();
  await parkMouse();
  m = await measureSetting("宿舍管理设置");
  ok("A3.d 宿舍管理设置抽屉已打开", hrSettings && m.found, JSON.stringify(m));
  cmp("宿舍管理设置", m);
  await closeTopDrawer();

  // 能耗 · 电费核算编辑抽屉（MeterRecordDrawer）
  await ensureApp("/platform/daily-affairs/utilities");
  await card("电费核算");
  await waitRows(40);
  await p.evaluate(() => { const r = document.querySelector(".doc-list-scroll tbody tr"); if (r) (r.querySelector("td") || r).click(); });
  await p.waitForTimeout(4500);
  await parkMouse();
  m = await measureSetting("记录");
  ok("A3.e 能耗电费编辑抽屉已打开", m.found, JSON.stringify(m));
  cmp("能耗电费编辑", m);
  await closeTopDrawer();

  // 设置 · 能耗
  await clickSettings();
  await parkMouse();
  m = await measureSetting("设置 · 能耗");
  ok("A3.f 设置 · 能耗抽屉已打开", m.found, JSON.stringify(m));
  cmp("设置 · 能耗", m);
  await closeTopDrawer();
  await shot("d74-a3-done");
} catch (e) {
  ok("A3 抽屉拖拽锚点 1:1", false, String(e).slice(0, 300));
}

// ============================================================ A4 环境
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|NetworkError|ECONNRESET|Timeout|Failed to (list|load)|dynamically imported module|pageerror: Object/i.test(e));
const realErrs = errs.filter((e) => !envNoise.includes(e));
ok("A4.1 全程 0 业务 console.error", realErrs.length === 0,
  `real=${JSON.stringify(realErrs.slice(0, 3))} env=${envNoise.length}`);
ok("A4.2 全程 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 3)));

const failed = R.checks.filter((x) => !x.pass);
console.log(`\nSUMMARY ${R.checks.length - failed.length}/${R.checks.length} PASS`);
if (failed.length) {
  console.log("FAILED:", JSON.stringify(failed.map((x) => `${x.n} :: ${x.d}`), null, 1));
}
await b.close();
process.exit(failed.length ? 1 : 0);
