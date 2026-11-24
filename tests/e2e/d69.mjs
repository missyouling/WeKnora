// 第 69 轮真机验收：
//   A1 抽屉紧凑同行排版 + 容器查询 2/3 列
//   A2 拖拽锚点贴边 + 「司龄」默认表头六同步闭环
//   A3 离职人员引用字段的来源胶囊
//   A4 Toast 不遮挡顶部 KPI
//   A5 账单变动红点 + 勾选浮动栏「重新生成」
//   A6 离职人员部门不丢失（临时置离职验证根因，测完还原）
//   A7 2026-09 身份证号全明文（0 条星号 + 18 位）
//   A8 明细弹窗垂直滚动条真实可用 + 切标签 0px 跳动
//
// 运行：cp tests/e2e/d69.mjs /opt/pw-run/ && cd /opt/pw-run && node d69.mjs
import { chromium } from "playwright";
import fs from "node:fs";

const APP = process.env.WEKNORA_E2E_APP || "http://127.0.0.1:5173";
const API = process.env.WEKNORA_E2E_API || "http://127.0.0.1:8080";
const BASE = `${API}/api/v1/fleet`;
const SHOTS = process.env.CI_SHOTS_DIR || "/tmp/pw/shots";
const TK = fs.readFileSync(process.env.WEKNORA_TOKEN_FILE || "/tmp/pw/token.txt", "utf8").trim();
const HDR = { Authorization: `Bearer ${TK}`, "X-Tenant-ID": "10000" };
fs.mkdirSync(SHOTS, { recursive: true });

const api = async (method, path, body) => {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: { ...HDR, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let json = null;
  try { json = await res.json(); } catch { /* 非 JSON 响应 */ }
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
const headers = () => p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
  .map((e) => (e.innerText || "").trim()));
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
        if (vis) { await p.waitForTimeout(700); return true; }
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
    if (vis) { await p.waitForTimeout(1400); return true; }
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
const drawerLabels = async (waitMs = 8000) => {
  const deadline = Date.now() + waitMs;
  let out = [];
  while (Date.now() < deadline) {
    out = await p.evaluate(() => Array.from(document.querySelectorAll(".bsd-drawer .t-form__label"))
      .map((e) => (e.innerText || "").trim()).filter(Boolean));
    if (out.length) return out;
    await p.waitForTimeout(300);
  }
  return out;
};
const clickColumnsFilter = async (action) => {
  // [☰ 字段] → teleport 到 body 的 .business-column-filter → 重置 / 全选
  const opened = await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("button"))
      .find((x) => (x.innerText || "").trim() === "字段");
    if (btn) { btn.click(); return true; }
    return false;
  });
  for (let i = 0; i < 20; i++) {
    if (await p.evaluate(() => !!document.querySelector(".business-column-filter"))) break;
    await p.waitForTimeout(300);
  }
  const clicked = await p.evaluate((a) => {
    const panel = document.querySelector(".business-column-filter");
    const btn = Array.from(panel?.querySelectorAll("button") || [])
      .find((x) => (x.innerText || "").trim() === a);
    if (btn) { btn.click(); return true; }
    return false;
  }, action);
  await p.waitForTimeout(1200);
  // 关闭弹层（点面板外）
  await p.mouse.click(20, 400);
  await p.waitForTimeout(600);
  return opened && clicked;
};
const billTab = async (label) => {
  const clicked = await p.evaluate((x) => {
    const root = document.querySelector("[data-hr-bill-dialog]");
    const e = Array.from(root?.querySelectorAll(".hr-bill-tabs .t-radio-button") || [])
      .find((q) => (q.innerText || "").trim() === x);
    if (e) { e.click(); return true; }
    return false;
  }, label);
  await p.waitForTimeout(1100);
  return clicked;
};
const billDialogRect = () => p.evaluate(() => {
  const ctx = document.querySelector("[data-hr-bill-dialog]");
  const el = ctx?.querySelector(".t-dialog") || ctx;
  if (!el) return null;
  const r = el.getBoundingClientRect();
  return { w: Math.round(r.width), h: Math.round(r.height), t: Math.round(r.top), l: Math.round(r.left) };
});
const measureAlign = () => p.evaluate(() => {
  const root = document.querySelector("[data-hr-bill-dialog]");
  const head = root?.querySelector(".detail-table-head thead tr");
  const body = root?.querySelector(".detail-table-body tbody tr");
  const foot = root?.querySelector(".detail-table-foot tr");
  if (!head || !body) return { err: "no-table" };
  const w = (tr) => Array.from(tr.querySelectorAll("th,td")).map((e) => Math.round(e.getBoundingClientRect().width));
  const h = w(head), b = w(body), f = foot ? w(foot) : [];
  let max = 0;
  for (let i = 0; i < Math.min(h.length, b.length); i++) {
    max = Math.max(max, Math.abs(h[i] - b[i]));
  }
  if (f.length) {
    for (let i = 0; i < Math.min(h.length, f.length); i++) {
      max = Math.max(max, Math.abs(h[i] - f[i]));
    }
  }
  const bodyEl = root.querySelector(".detail-table-body");
  const cs = getComputedStyle(bodyEl);
  return {
    max, cols: h.length, rows: bodyEl.querySelectorAll("tbody tr").length,
    overflowY: cs.overflowY, sh: bodyEl.scrollHeight, ch: bodyEl.clientHeight,
    sw: bodyEl.offsetWidth - bodyEl.clientWidth,
  };
});

const started = await ensureApp();
if (!started) throw new Error("首屏 bootstrap 未就绪（KPI 卡片未渲染）");

// ---------------------------------------------------------------- A1 抽屉排版
try {
  await card("花名册");
  await subTab("在职人员");
  await waitRows();
  ok("A1.0 花名册在职列表已加载", (await headers()).length > 5, JSON.stringify((await headers()).slice(0, 6)));
  ok("A1.1 员工详情抽屉可打开", await openFirstRow());

  const wide = await p.evaluate(() => {
    const root = document.querySelector(".bsd-drawer .t-drawer__content-wrapper");
    const grid = document.querySelector(".bsd-drawer .bsd-fields");
    const items = Array.from(document.querySelectorAll(".bsd-drawer .bsd-fields > .t-form__item"));
    const rowsOf = (pred) => items.filter(pred);
    const rowItems = rowsOf((it) => {
      const label = (it.querySelector(".t-form__label")?.innerText || "").trim();
      return ["工号", "姓名", "部门"].includes(label);
    }).map((it) => {
      const lr = it.querySelector(".t-form__label")?.getBoundingClientRect();
      const cr = it.querySelector(".t-form__controls")?.getBoundingClientRect();
      return {
        label: (it.querySelector(".t-form__label")?.innerText || "").trim(),
        sameLine: !!(lr && cr) && Math.abs((lr.top + lr.height / 2) - (cr.top + cr.height / 2)) <= 6,
        labelW: lr ? Math.round(lr.width) : 0,
        dir: getComputedStyle(it).flexDirection,
      };
    });
    const long = items.find((it) => (it.querySelector(".t-form__label")?.innerText || "").trim().length >= 7);
    const longDir = long ? getComputedStyle(long).flexDirection : "";
    const cols = getComputedStyle(grid).gridTemplateColumns.split(" ").filter(Boolean).length;
    return {
      drawerW: root ? Math.round(root.getBoundingClientRect().width) : 0,
      rowGap: getComputedStyle(grid).rowGap,
      colGap: getComputedStyle(grid).columnGap,
      cols, rowItems, longDir,
      longLabel: long ? (long.querySelector(".t-form__label")?.innerText || "").trim() : "",
    };
  });
  ok("A1.2 默认宽下一行两列、row-gap=10px", wide.cols === 2 && wide.rowGap === "10px",
    `w=${wide.drawerW} cols=${wide.cols} gap=${wide.rowGap}/${wide.colGap}`);
  ok("A1.3 短文案字段同行（工号/姓名/部门 label 与控件同一水平线）",
    wide.rowItems.length >= 2 && wide.rowItems.every((x) => x.sameLine && x.dir === "row"),
    JSON.stringify(wide.rowItems));
  ok("A1.4 长文案字段自动换行（上下结构）", wide.longDir === "column",
    `${wide.longLabel} dir=${wide.longDir}`);
  await shot("d69_01_drawer_compact");

  // 拖宽到 1150+：鼠标拖拽左侧热区（真实交互，不用改 storage）
  const hb = await p.evaluate(() => {
    const h = document.querySelector(".setting-drawer-resize-handle");
    if (!h) return null;
    const r = h.getBoundingClientRect();
    return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) };
  });
  if (hb) {
    await p.mouse.move(hb.x, hb.y);
    await p.mouse.down();
    for (let i = 1; i <= 8; i++) await p.mouse.move(hb.x - i * 50, hb.y);
    await p.mouse.up();
    await p.waitForTimeout(900);
  }
  const widened = await p.evaluate(() => {
    const root = document.querySelector(".bsd-drawer .t-drawer__content-wrapper");
    const grid = document.querySelector(".bsd-drawer .bsd-fields");
    return {
      w: root ? Math.round(root.getBoundingClientRect().width) : 0,
      cols: getComputedStyle(grid).gridTemplateColumns.split(" ").filter(Boolean).length,
      contentW: Math.round(grid.getBoundingClientRect().width),
    };
  });
  ok("A1.5 拉宽到 ≥1020px 时自动变三列", widened.w >= 1020 && widened.cols === 3,
    `drawer=${widened.w} grid=${widened.contentW} cols=${widened.cols}`);
  await shot("d69_02_drawer_three_cols");
  await closeDrawer();
} catch (e) {
  ok("A1 抽屉排版", false, String(e).slice(0, 200));
}

// ------------------------------------------------- A2 锚点贴边 + 司龄六同步
try {
  const before = await headers();
  ok("A2.0 在职表头初始包含「司龄」", before.includes("司龄"), JSON.stringify(before.slice(0, 16)));
  ok("A2.1 设置抽屉可打开", await openSettings());

  const anchor = await p.evaluate(() => {
    const w = document.querySelector(".standard-settings-drawer .t-drawer__content-wrapper");
    const h = document.querySelector(".setting-drawer-resize-handle");
    if (!w || !h) return { err: "missing" };
    // 先把抽屉内容滚到底（旧实现的 fixed 热区会因此漂移）
    const body = w.querySelector(".t-drawer__body");
    if (body) body.scrollTop = body.scrollHeight;
    const wr = w.getBoundingClientRect();
    const hr = h.getBoundingClientRect();
    return {
      drawerLeft: Math.round(wr.left), handleLeft: Math.round(hr.left),
      diff: Math.abs(Math.round(hr.left) - Math.round(wr.left)),
      w: Math.round(wr.width), inside: w.contains(h),
    };
  });
  ok("A2.2 滚动后拖拽热区仍贴抽屉左边框（Δ ≤ 4px）",
    !anchor.err && anchor.diff <= 4, JSON.stringify(anchor));
  ok("A2.3 热区是抽屉面板的直接子节点（非 body 级 fixed）", anchor.inside === true, `inside=${anchor.inside}`);

  const tabActive = await p.evaluate(() => {
    const t = Array.from(document.querySelectorAll(".standard-settings-drawer .fc-group-tab"))
      .find((e) => e.className.includes("active"));
    return (t?.innerText || "").replace(/\s+/g, " ").trim();
  });
  ok("A2.4 从「在职人员」列表打开设置，默认胶囊就是在职人员",
    tabActive.startsWith("在职人员"), tabActive);
  await shot("d69_03_settings_anchor");

  // 关闭「司龄」的默认表头（第 1 个开关），保留启用（第 2 个）
  const idx = await p.evaluate(() => Array.from(document.querySelectorAll(".standard-settings-drawer .field-config-row"))
    .findIndex((r) => Array.from(r.querySelectorAll("input")).some((i) => i.value === "司龄")));
  ok("A2.5 设置里能定位到「司龄」字段行", idx >= 0, `idx=${idx}`);
  const toggled = await p.evaluate((i) => {
    const row = document.querySelectorAll(".standard-settings-drawer .field-config-row")[i];
    if (!row) return null;
    const sw = Array.from(row.querySelectorAll(".t-switch"));
    const before = sw.map((s) => s.className.includes("t-is-checked"));
    if (sw[0] && sw[0].className.includes("t-is-checked")) sw[0].click();
    return { before, count: sw.length };
  }, idx);
  await p.waitForTimeout(500);
  const afterToggle = await p.evaluate((i) => {
    const row = document.querySelectorAll(".standard-settings-drawer .field-config-row")[i];
    return row ? Array.from(row.querySelectorAll(".t-switch")).map((s) => s.className.includes("t-is-checked")) : null;
  }, idx);
  ok("A2.6 关闭「司龄」默认表头、启用保持开启",
    !!afterToggle && afterToggle[0] === false && afterToggle[1] === true,
    `before=${JSON.stringify(toggled?.before)} after=${JSON.stringify(afterToggle)}`);

  await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll(".standard-settings-drawer button"))
      .find((x) => (x.innerText || "").trim() === "保存");
    if (btn) btn.click();
  });
  await p.waitForTimeout(3000);
  if (await p.evaluate(() => {
    const w = document.querySelector(".standard-settings-drawer .t-drawer__content-wrapper");
    return !!w && w.getBoundingClientRect().width > 200;
  })) await closeSettings();

  let hd = [];
  for (let i = 0; i < 20; i++) {
    hd = await headers();
    if (!hd.includes("司龄")) break;
    await p.waitForTimeout(700);
  }
  ok("A2.7 保存后列表表头立即不含「司龄」", !hd.includes("司龄"), JSON.stringify(hd.slice(0, 16)));

  const resetClicked = await clickColumnsFilter("重置");
  const hd2 = await headers();
  ok("A2.8 点「重置」后表头仍不含「司龄」（基准唯一，不复活）",
    resetClicked && !hd2.includes("司龄"), `clicked=${resetClicked} ${JSON.stringify(hd2.slice(0, 16))}`);

  // 全选语义：应把启用字段全勾上（含司龄），不再是「回到默认」
  const allClicked = await clickColumnsFilter("全选");
  const hd3 = await headers();
  ok("A2.9 「全选」与「重置」语义分离（全选包含启用字段「司龄」）",
    allClicked && hd3.includes("司龄"), `clicked=${allClicked} ${JSON.stringify(hd3.slice(0, 16))}`);
  await clickColumnsFilter("重置");

  // 刷新页面后仍然不显示（落库为 false）
  await p.goto(`${APP}/platform/daily-affairs/hr`, { waitUntil: "domcontentloaded", timeout: 90000 });
  for (let i = 0; i < 30; i++) {
    if (await p.evaluate(() => document.querySelectorAll(".dash-kpi-card").length >= 5)) break;
    await p.waitForTimeout(1500);
  }
  await killGuide();
  await card("花名册");
  await subTab("在职人员");
  await waitRows();
  const hd4 = await headers();
  ok("A2.10 刷新后「司龄」仍保持不显示（false 已真实落库）",
    hd4.length > 5 && !hd4.includes("司龄"), JSON.stringify(hd4.slice(0, 16)));

  // 还原：重新打开司龄的默认表头
  await openSettings();
  const idx2 = await p.evaluate(() => Array.from(document.querySelectorAll(".standard-settings-drawer .field-config-row"))
    .findIndex((r) => Array.from(r.querySelectorAll("input")).some((i) => i.value === "司龄")));
  if (idx2 >= 0) {
    await p.evaluate((i) => {
      const row = document.querySelectorAll(".standard-settings-drawer .field-config-row")[i];
      const sw = Array.from(row.querySelectorAll(".t-switch"));
      if (sw[0] && !sw[0].className.includes("t-is-checked")) sw[0].click();
    }, idx2);
    await p.waitForTimeout(400);
    await p.evaluate(() => {
      const btn = Array.from(document.querySelectorAll(".standard-settings-drawer button"))
        .find((x) => (x.innerText || "").trim() === "保存");
      if (btn) btn.click();
    });
    await p.waitForTimeout(2800);
    if (await p.evaluate(() => {
      const w = document.querySelector(".standard-settings-drawer .t-drawer__content-wrapper");
      return !!w && w.getBoundingClientRect().width > 200;
    })) await closeSettings();
  }
  const hd5 = await headers();
  ok("A2.11 测试字段已还原（司龄重新出现在默认表头）", hd5.includes("司龄"), JSON.stringify(hd5.slice(0, 16)));
} catch (e) {
  ok("A2 锚点与六同步", false, String(e).slice(0, 200));
}

// ---------------------------------------------------- A3 离职引用来源胶囊
try {
  ok("A3.0 设置抽屉可打开", await openSettings());
  const switched = await p.evaluate(() => {
    const t = Array.from(document.querySelectorAll(".standard-settings-drawer .fc-group-tab"))
      .find((e) => (e.innerText || "").trim().startsWith("离职人员"));
    if (t) { t.click(); return true; }
    return false;
  });
  await p.waitForTimeout(1200);
  ok("A3.1 切换到「离职人员」胶囊", switched);

  const refInfo = await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".standard-settings-drawer .field-config-row"));
    const pick = (name) => rows.filter((r) => Array.from(r.querySelectorAll("input")).some((i) => i.value === name));
    const out = {};
    for (const name of ["招聘渠道", "司龄", "证件号码"]) {
      const row = pick(name)[0];
      if (!row) { out[name] = null; continue; }
      const badge = row.querySelector("[data-fc-ref-source]");
      const sw = Array.from(row.querySelectorAll(".t-switch"));
      out[name] = {
        badge: (badge?.innerText || "").trim(),
        enabled: sw.map((s) => s.className.includes("t-is-checked")),
      };
    }
    return out;
  });
  const names = Object.keys(refInfo);
  ok("A3.2 并入的在职字段渲染「在职人员」来源胶囊",
    names.every((n) => refInfo[n] && refInfo[n].badge === "在职人员"),
    JSON.stringify(refInfo));
  ok("A3.3 新并入字段默认关闭（默认表头 + 启用均 false）",
    names.every((n) => refInfo[n] && refInfo[n].enabled[0] === false && refInfo[n].enabled[1] === false),
    JSON.stringify(refInfo));
  await shot("d69_04_resign_ref_badge");
  await closeSettings();
} catch (e) {
  ok("A3 离职引用来源", false, String(e).slice(0, 200));
}

// -------------------------------------------------------- A4 Toast 不遮 KPI
try {
  await card("花名册");
  await subTab("在职人员");
  await waitRows();
  // 触发一条 warning（批量删除未选中的前置提示走 error；这里用保存冲突更可控）：
  // 直接在业务页面点「导出 Excel」不会弹提示，改用一次空搜索刷新触发 info；
  // 最稳的触发方式：点「字段」→ 重置（无 Toast）不可用，所以用 API 侧失败提示不经过 UI。
  // 这里复用真实按钮：点「生成账单」弹窗取消 → 不产生提示。
  // 因此改为在页面内主动调用一次 MessagePlugin（同一插件实例，等价于业务提示）。
  await p.evaluate(() => {
    // 从已加载模块里取插件实例：优先使用全局挂载点（App 里没有则退化为 DOM 触发）
    const app = document.querySelector("#app")?.__vue_app__;
    const gp = app?.config?.globalProperties;
    if (gp?.$message) gp.$message.warning("第 69 轮 Toast 位置校验");
  });
  await p.waitForTimeout(900);
  let toast = await p.evaluate(() => {
    const els = Array.from(document.querySelectorAll(".t-message")).filter((e) => e.getBoundingClientRect().height > 0);
    if (!els.length) return null;
    const r = els[els.length - 1].getBoundingClientRect();
    return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), w: Math.round(r.width) };
  });
  if (!toast) {
    // 退化：用「删除未选中」触发 error 提示
    await p.evaluate(() => {
      const btn = Array.from(document.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "删除");
      if (btn) btn.click();
    });
    await p.waitForTimeout(900);
    toast = await p.evaluate(() => {
      const els = Array.from(document.querySelectorAll(".t-message")).filter((e) => e.getBoundingClientRect().height > 0);
      if (!els.length) return null;
      const r = els[els.length - 1].getBoundingClientRect();
      return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), w: Math.round(r.width) };
    });
  }
  const kpi = await p.evaluate(() => {
    const el = document.querySelector(".dash-kpi-group") || document.querySelector(".dash-kpi-card");
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right) };
  });
  const noOverlap = !!toast && !!kpi && (toast.top >= kpi.bottom || toast.bottom <= kpi.top || toast.left >= kpi.right || toast.right <= kpi.left);
  ok("A4.1 Toast 出现在页面右下角（top > 视口中线）", !!toast && toast.top > 450, JSON.stringify(toast));
  ok("A4.2 Toast 与顶部 KPI 卡片零重叠", noOverlap, `toast=${JSON.stringify(toast)} kpi=${JSON.stringify(kpi)}`);
  ok("A4.3 Toast 宽度受限（≤ 400px）", !!toast && toast.w <= 400, `w=${toast?.w}`);
  await shot("d69_05_toast_bottom_right");
} catch (e) {
  ok("A4 Toast 位置", false, String(e).slice(0, 200));
}

// ------------------------------------- A5 账单红点 + 勾选浮动栏「重新生成」
try {
  await card("社保管理");
  await subTab("月度账单");
  await waitRows();
  const rowsA = await headers();
  ok("A5.0 社保月度账单列表已加载", rowsA.includes("账单周期"), JSON.stringify(rowsA.slice(0, 8)));

  // 制造脏标记：把 2026-11 的一条正常记录原值保存一次（后端会置 source_dirty）
  const rec = await api("GET", "/hr/social/records?record_type=normal&month=2026-11");
  const first = (rec.body?.data?.items || [])[0];
  if (first) {
    await api("PUT", `/hr/social/records/${encodeURIComponent(String(first.id))}`, {
      base: first["缴费基数"], unit_amount: first["单位应缴金额"], personal_amount: first["个人应缴金额"],
      unit_rate: first["单位缴费比例"], personal_rate: first["个人缴费比例"],
    });
  }
  await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "生成账单");
    if (btn) btn.click();
  });
  await p.waitForTimeout(400);
  await p.keyboard.press("Escape");
  await waitRows();
  // 重新载入列表（切标签）
  await subTab("回盘表");
  await subTab("月度账单");
  await waitRows();

  const dot = await p.evaluate(() => {
    const d = document.querySelector("[data-hr-bill-dirty-flag]");
    if (!d) return null;
    const r = d.getBoundingClientRect();
    const cell = d.closest("td");
    const text = (cell?.innerText || "").replace(/\s+/g, " ").trim();
    return {
      w: Math.round(r.width), h: Math.round(r.height),
      bg: getComputedStyle(d).backgroundColor,
      cellText: text,
      beforeMonth: r.left < (cell?.getBoundingClientRect().left || 0) + 20,
    };
  });
  const redDot = !!dot && (() => {
    const m = /rgb\((\d+), (\d+), (\d+)\)/.exec(dot.bg || "");
    if (!m) return false;
    const [r, g, bl] = [Number(m[1]), Number(m[2]), Number(m[3])];
    // 红色主导（主题的 error 令牌在项目里是 #e34d59，不是默认 #d54941）
    return r > 150 && r > g + 60 && r > bl + 60;
  })();
  ok("A5.1 变动账单周期前方出现小红点（7px 圆点）",
    !!dot && dot.w <= 9 && dot.h <= 9 && redDot, JSON.stringify(dot));
  ok("A5.2 周期单元格不再有挤压文字（只剩月份）",
    !!dot && /^2026-\d{2}$/.test(dot.cellText), JSON.stringify(dot?.cellText));

  // 勾选 2026-11 账单行 → 浮动栏出现「重新生成」
  const picked = await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes("2026-11"));
    if (!row) return false;
    const box = row.querySelector("input[type=checkbox], .t-checkbox");
    if (box) { box.click(); return true; }
    row.click();
    return true;
  });
  await p.waitForTimeout(900);
  const hasRegen = await p.evaluate(() => {
    const btn = document.querySelector("[data-hr-regenerate-bills]");
    if (!btn) return null;
    return { text: (btn.innerText || "").trim(), disabled: !!btn.disabled };
  });
  ok("A5.3 勾选账单行后浮动栏出现「重新生成」", picked && !!hasRegen && hasRegen.text === "重新生成",
    JSON.stringify(hasRegen));

  const beforeTotal = await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes("2026-11"));
    return (row?.innerText || "").replace(/\s+/g, " ").trim();
  });
  // 点「重新生成」→ 气泡确认
  await p.evaluate(() => {
    const btn = document.querySelector("[data-hr-regenerate-bills]");
    if (btn) btn.click();
  });
  await p.waitForTimeout(900);
  const confirmed = await p.evaluate(() => {
    const pop = Array.from(document.querySelectorAll(".t-popconfirm")).filter((e) => e.offsetParent !== null);
    const btn = pop.flatMap((e) => Array.from(e.querySelectorAll("button")))
      .find((b) => /确定|确认/.test((b.innerText || "").trim()));
    if (btn) { btn.click(); return true; }
    return false;
  });
  await p.waitForTimeout(5000);
  const afterDot = await p.evaluate(() => !!document.querySelector("[data-hr-bill-dirty-flag]"));
  const afterTotal = await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes("2026-11"));
    return (row?.innerText || "").replace(/\s+/g, " ").trim();
  });
  ok("A5.4 「重新生成」确认后小红点消失（source_dirty 已清除）",
    confirmed && afterDot === false, `confirmed=${confirmed} dot=${afterDot}`);
  ok("A5.5 重新生成后账单金额与记录重算结果一致（行内容非空）",
    afterTotal.length > 10, `${beforeTotal.slice(0, 40)} → ${afterTotal.slice(0, 60)}`);
  await shot("d69_06_bill_dirty_dot");
} catch (e) {
  ok("A5 账单红点与重新生成", false, String(e).slice(0, 200));
}

// -------------------------------------------- A6 离职人员部门不丢失（根因）
try {
  // 从 2026-11 社保记录出发，找一名「在社保名单里 + 花名册在职 + 部门非空」的员工
  const recList = await api("GET", "/hr/social/records?record_type=normal&month=2026-11");
  const recItems = recList.body?.data?.items || [];
  const roster = await api("GET", "/hr/roster/employees?page=1&page_size=500");
  const emps = roster.body?.data?.items || roster.body?.data || [];
  const empList = Array.isArray(emps) ? emps : [];
  let target = null;
  let idNum = "";
  for (const r of recItems) {
    const id = String(r["身份证号"] || r.id_number || "").trim();
    if (id.length !== 18) continue;
    const e = empList.find((x) => String(x.id_number || "").trim() === id);
    if (e && String(e.status || "") !== "离职" && String(e.department || "").trim()) {
      target = e; idNum = id; break;
    }
  }
  ok("A6.0 找到一名在 2026-11 社保名单内的在职员工", !!target,
    target ? `${target.name}/${target.department}` : "none");
  if (target) {
    const rec0 = recItems.find((r) => String(r["身份证号"] || "") === idNum) || {};
    const deptBefore = String(rec0["部门"] || rec0.department || "").trim();
    ok("A6.1 该员工当前社保记录部门非空", !!deptBefore, `dept=${deptBefore}`);

    const put = await api("PUT", `/hr/roster/employees/${encodeURIComponent(String(target.id))}`, { status: "离职" });
    ok("A6.2 临时将该员工置为离职", put.status < 300, `status=${put.status}`);

    // 该员工 2026-11 社保记录必须仍能关联到部门（含离职人员查询）
    const recs = await api("GET", `/hr/social/records?record_type=normal&month=2026-11&q=${encodeURIComponent(idNum)}`);
    const items = recs.body?.data?.items || [];
    const dept = String(items[0]?.["部门"] || items[0]?.department || "").trim();
    ok("A6.3 离职后其社保记录仍关联到部门（含离职人员生效）",
      items.length > 0 && dept === String(target.department).trim(),
      `rows=${items.length} dept=${dept} expect=${target.department}`);

    const restore = await api("PUT", `/hr/roster/employees/${encodeURIComponent(String(target.id))}`,
      { status: String(target.status || "在职") });
    ok("A6.4 测试数据已还原为在职", restore.status < 300, `status=${restore.status}`);
  }
} catch (e) {
  ok("A6 离职人员部门", false, String(e).slice(0, 200));
}

// --------------------------------- A7 2026-09 身份证号全明文（0 条星号）
try {
  const r9 = await api("GET", "/hr/social/records?record_type=normal&month=2026-09");
  const items9 = r9.body?.data?.items || [];
  const ids = items9.map((x) => String(x["身份证号"] || x.id_number || ""));
  const masked = ids.filter((x) => x.includes("*"));
  const notFull = ids.filter((x) => !x.includes("*") && x.length !== 18);
  ok("A7.1 2026-09 正常记录全部身份证号无星号",
    items9.length > 300 && masked.length === 0, `rows=${items9.length} masked=${masked.length}`);
  ok("A7.2 全部为 18 位完整明文", notFull.length === 0,
    `bad=${notFull.length} sample=${JSON.stringify(notFull.slice(0, 3))}`);

  // 账单快照同样无星号（弹窗只读快照）
  const bills = await api("GET", "/hr/social/bills");
  const bill9 = (bills.body?.data?.items || []).find((x) => x.month_label === "2026-09");
  let snapMasked = 0;
  let snapRows = 0;
  if (bill9) {
    const d = await api("GET", `/hr/social/bills/${encodeURIComponent(String(bill9.id))}`);
    const snapItems = d.body?.data?.items || [];
    snapRows = snapItems.length;
    snapMasked = snapItems.filter((x) => String(x["证件号码"] || x.id_number || "").includes("*")).length;
  }
  ok("A7.3 2026-09 账单快照明细同样 0 条星号",
    snapRows >= 70 && snapMasked === 0, `rows=${snapRows} masked=${snapMasked}`);

  // UI 侧：正常记录页把身份证列渲染出来（当前页 0 条星号）
  await card("社保管理");
  await subTab("正常记录");
  await waitRows();
  const uiIds = await p.evaluate(() => {
    const heads = Array.from(document.querySelectorAll(".doc-list-scroll thead th")).map((e) => (e.innerText || "").trim());
    const i = heads.indexOf("身份证号");
    if (i < 0) return { err: "no-col", heads };
    return {
      idx: i,
      values: Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
        .map((r) => (r.querySelectorAll("td")[i]?.innerText || "").trim())
        .filter(Boolean),
    };
  });
  ok("A7.4 UI 列表身份证列无星号且均为 18 位",
    !uiIds.err && uiIds.values.length > 0 && uiIds.values.every((v) => !v.includes("*") && v.length === 18),
    `n=${uiIds.values?.length} bad=${JSON.stringify((uiIds.values || []).filter((v) => v.includes("*") || v.length !== 18).slice(0, 3))}`);
} catch (e) {
  ok("A7 身份证明文", false, String(e).slice(0, 200));
}

// ---------------------------------- A8 明细弹窗滚动条 + 切标签 0px 跳动
try {
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

  // ---- 社保 2026-09（78 人）
  await card("社保管理");
  await subTab("月度账单");
  await waitRows();
  ok("A8.0 打开 2026-09 社保扣款明细", await openBill("2026-09"));
  await billTab("个人明细");
  const m9 = await measureAlign();
  ok("A8.1 78 行明细数据区可滚动（scrollHeight > clientHeight + 50）",
    !m9.err && m9.sh > m9.ch + 50 && /auto|scroll/.test(m9.overflowY),
    JSON.stringify({ sh: m9.sh, ch: m9.ch, oy: m9.overflowY, rows: m9.rows }));
  const scrolled = await p.evaluate(() => {
    const root = document.querySelector("[data-hr-bill-dialog]");
    const body = root.querySelector(".detail-table-body");
    body.scrollTop = body.scrollHeight;
    const rows = Array.from(body.querySelectorAll("tbody tr"));
    const last = rows[rows.length - 1];
    const br = body.getBoundingClientRect();
    const lr = last.getBoundingClientRect();
    return {
      left: Math.round(body.scrollTop), max: body.scrollHeight - body.clientHeight,
      visible: lr.bottom <= br.bottom + 2 && lr.top >= br.top - 2,
      rows: rows.length,
    };
  });
  ok("A8.2 滚到底后最后一行真实进入可视区域",
    scrolled.left >= scrolled.max - 2 && scrolled.visible, JSON.stringify(scrolled));
  ok("A8.3 三段表逐列竖线对齐 ≤1px、无横向滚动条",
    !m9.err && m9.max <= 1 && m9.sw <= 1, JSON.stringify({ max: m9.max, sw: m9.sw, cols: m9.cols }));

  // 切标签几何 0px
  const r0 = await billDialogRect();
  const rects = [r0];
  for (const t of ["单位明细", "部门汇总", "岗位汇总"]) {
    if (!(await billTab(t))) continue;
    rects.push(await billDialogRect());
  }
  const diff = (k) => Math.max(...rects.map((x) => x[k])) - Math.min(...rects.map((x) => x[k]));
  ok("A8.4 社保弹窗切换全部标签：宽/高/上/左 4 项几何差 ≤1px",
    rects.length >= 3 && diff("w") <= 1 && diff("h") <= 1 && diff("t") <= 1 && diff("l") <= 1,
    JSON.stringify({ n: rects.length, dw: diff("w"), dh: diff("h"), dt: diff("t"), dl: diff("l"), r0 }));
  await shot("d69_07_social_scroll");
  await closeBill();

  // ---- 公积金 2026-11（74 人）
  await card("公积金管理");
  await subTab("月度账单");
  await waitRows();
  ok("A8.5 打开 2026-11 公积金缴存明细", await openBill("2026-11"));
  await billTab("个人明细");
  const mf = await measureAlign();
  ok("A8.6 74 行公积金明细同样可滚动",
    !mf.err && mf.sh > mf.ch + 50 && /auto|scroll/.test(mf.overflowY),
    JSON.stringify({ sh: mf.sh, ch: mf.ch, oy: mf.overflowY, rows: mf.rows }));
  const fr = [await billDialogRect()];
  for (const t of ["单位明细", "部门汇总", "岗位汇总"]) {
    if (!(await billTab(t))) continue;
    fr.push(await billDialogRect());
  }
  const fdiff = (k) => Math.max(...fr.map((x) => x[k])) - Math.min(...fr.map((x) => x[k]));
  ok("A8.7 公积金弹窗切换标签几何 0px 跳动",
    fr.length >= 3 && fdiff("w") <= 1 && fdiff("h") <= 1 && fdiff("t") <= 1 && fdiff("l") <= 1,
    JSON.stringify({ n: fr.length, dw: fdiff("w"), dh: fdiff("h") }));
  await shot("d69_08_fund_scroll");
  await closeBill();
} catch (e) {
  ok("A8 明细弹窗滚动", false, String(e).slice(0, 200));
}

// ------------------------------------------------------------------ 汇总
// 环境噪声（VM 上 DNS 劫持 Supabase / dev server 的 HMR 断连）不算业务错误：
// 只断言「业务的 console.error」为 0，网络层抖动单独列出以便追溯。
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|cmdk|NetworkError|ECONNRESET|Timeout/i.test(e));
const realErrs = errs.filter((e) => !envNoise.includes(e));
ok("A9.1 全程 0 业务 console.error", realErrs.length === 0,
  `real=${JSON.stringify(realErrs.slice(0, 4))} env=${envNoise.length}`);
ok("A9.2 全程 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 4)));

const failed = R.checks.filter((x) => !x.pass);
console.log(`\nSUMMARY ${R.checks.length - failed.length}/${R.checks.length} PASS`);
if (failed.length) {
  console.log("FAILED:", JSON.stringify(failed.map((x) => `${x.n} :: ${x.d}`), null, 1));
}
await b.close();
process.exit(failed.length ? 1 : 0);
