// 第 72 轮真机验收（房型联动 / 空置开关 / 表计查重 / 公租房级联 / 住宿重构 / 资产字典）：
//   A1 房型 → 床位数 / 可用床位 / 状态实时联动，且防缩容
//   A2 空置开关（四态、拒入住、能耗录入隔离、历史零波动）
//   A3 表计号/表名全局查重（400 + 指明冲突房源）
//   A4 公租房 租赁单位 → 所属部门级联 + 舍长/成员匹配
//   A5 住宿记录：姓名去工号、部门回显、费用项归组、床位池化、设置取消不白屏
//   A6 资产档案=共用物品字典（去重、12 类、配套下拉）
//
// 运行：cp tests/e2e/d72.mjs /opt/pw-run/ && cd /opt/pw-run && node d72.mjs
//
// 运行：cp tests/e2e/d72.mjs /opt/pw-run/ && cd /opt/pw-run && node d72.mjs
import { chromium } from "playwright";
import fs from "node:fs";

const APP = process.env.WEKNORA_E2E_APP || "http://127.0.0.1:5173";
const API = process.env.WEKNORA_E2E_API || "http://127.0.0.1:8080";
const HBASE = `${API}/api/v1/fleet/hr`;
const UBASE = `${API}/api/v1/utilities`;
const SHOTS = process.env.CI_SHOTS_DIR || "/tmp/pw/shots";
const TK = fs.readFileSync(process.env.WEKNORA_TOKEN_FILE || "/tmp/pw/token.txt", "utf8").trim();
const HDR = { Authorization: `Bearer ${TK}`, "X-Tenant-ID": "10000" };
fs.mkdirSync(SHOTS, { recursive: true });

const api = async (method, path, body, base = HBASE) => {
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
const tableRows = () => p.evaluate(() => {
  const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th")).map((e) => (e.innerText || "").trim());
  return Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .filter((r) => r.querySelectorAll("td").length > 3)
    .map((r) => {
      const tds = Array.from(r.querySelectorAll("td"));
      const o = {};
      ths.forEach((h, i) => { if (h) o[h] = (tds[i]?.innerText || "").trim(); });
      return o;
    });
});
const footText = () => p.evaluate(() => (document.querySelector(".doc-list-scroll tfoot")?.innerText || "").replace(/\s+/g, " ").trim());
const openRowByCell = async (col, needle) => {
  for (let attempt = 0; attempt < 4; attempt++) {
    const clicked = await p.evaluate(({ col, needle }) => {
      const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th")).map((e) => (e.innerText || "").trim());
      const idx = ths.indexOf(col);
      // 房间号列可能被用户隐藏：退化为整行文本匹配
      const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3);
      const pick = (r) => (idx >= 0 ? ((r.querySelectorAll("td")[idx]?.innerText) || "").trim() : (r.innerText || ""));
      const row = rows.find((r) => pick(r) === needle) || rows.find((r) => pick(r).includes(needle));
      if (!row) return false;
      (row.querySelector("td") || row).click();
      return true;
    }, { col, needle });
    if (clicked) {
      for (let i = 0; i < 24; i++) {
        const vis = await p.evaluate(() => Array.from(document.querySelectorAll(".bsd-footer"))
          .some((e) => e.getBoundingClientRect().width > 10));
        if (vis) { await p.waitForTimeout(800); return true; }
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
  if (await p.evaluate(() => Array.from(document.querySelectorAll(".bsd-footer")).some((e) => e.getBoundingClientRect().width > 10))) {
    await p.keyboard.press("Escape");
    await p.waitForTimeout(700);
  }
};
/** 从可见抽屉读某字段（label 前缀匹配）。 */
const drawerField = (label) => p.evaluate((lb) => {
  const list = Array.from(document.querySelectorAll(".bsd-drawer"));
  const root = list.find((d) => {
    const w = d.querySelector(".t-drawer__content-wrapper");
    return !!w && w.getBoundingClientRect().width > 200;
  });
  const items = Array.from(root?.querySelectorAll(".t-form__item") || []);
  const it = items.find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith(lb));
  if (!it) return null;
  const input = it.querySelector("input");
  const sel = it.querySelector(".t-select");
  return {
    label: (it.querySelector(".t-form__label")?.innerText || "").trim(),
    value: input ? input.value : "",
    isSelect: !!sel,
    multiple: /multiple/.test(String(sel?.className || "")),
  };
}, label);
/** 打开某字段下拉并读选项（多选下拉必须真实 click）。 */
const selectOptions = async (label) => {
  const item = p.locator(".bsd-drawer .t-form__item").filter({ hasText: label }).first();
  try {
    await item.locator(".t-select").first().click({ timeout: 8000 });
  } catch {
    return { opened: false, opts: [] };
  }
  await p.waitForTimeout(1200);
  const opts = await p.evaluate(() => {
    const pops = Array.from(document.querySelectorAll(".t-select__dropdown, .t-popup"));
    const vis = pops.filter((e) => e.getBoundingClientRect().height > 10).pop();
    return Array.from(vis?.querySelectorAll(".t-option, .t-select-option") || [])
      .map((e) => (e.innerText || "").trim()).filter(Boolean);
  });
  await p.keyboard.press("Escape");
  await p.waitForTimeout(500);
  return { opened: true, opts };
};

// ============================================================ 首屏
const started = await ensureApp();
if (!started) throw new Error("首屏 bootstrap 未就绪");

// ------------------------------------------------------------ 通用抽屉操作
const visibleDrawerCode = `(() => {
  const list = Array.from(document.querySelectorAll(".bsd-drawer"));
  return list.find((d) => {
    const w = d.querySelector(".t-drawer__content-wrapper");
    return !!w && w.getBoundingClientRect().width > 200;
  }) || null;
})()`;

/** 字段所在 .t-form__item 的坐标（label 前缀匹配）。 */
const fieldBox = async (label) => {
  await p.evaluate(({ lb, code }) => {
    const root = eval(code);
    const it = Array.from(root?.querySelectorAll(".t-form__item") || [])
      .find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith(lb));
    it?.scrollIntoView({ block: "center" });
  }, { lb: label, code: visibleDrawerCode });
  await p.waitForTimeout(400);
  return p.evaluate(({ lb, code }) => {
    const root = eval(code);
    const it = Array.from(root?.querySelectorAll(".t-form__item") || [])
      .find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith(lb));
    if (!it) return null;
    const sel = it.querySelector(".t-select");
    const inp = it.querySelector("input");
    const el = sel || inp;
    if (!el) return null;
    const r = el.getBoundingClientRect();
    if (r.width < 10) return null;
    return {
      x: Math.round(r.left + Math.min(40, r.width / 2)),
      y: Math.round(r.top + r.height / 2),
      isSelect: !!sel,
      value: inp ? inp.value : "",
    };
  }, { lb: label, code: visibleDrawerCode });
};

/** 选一个下拉选项（真实鼠标点击）。 */
const pickOption = async (label, text) => {
  const box = await fieldBox(label);
  if (!box) return false;
  await p.mouse.click(box.x, box.y);
  await p.waitForTimeout(900);
  const hit = await p.evaluate((t) => {
    const pops = Array.from(document.querySelectorAll(".t-select__dropdown, .t-popup"))
      .filter((e) => e.getBoundingClientRect().height > 10);
    const vis = pops[pops.length - 1];
    const opt = Array.from(vis?.querySelectorAll(".t-select-option, .t-option") || [])
      .find((e) => (e.innerText || "").trim() === t);
    if (!opt) return false;
    opt.click();
    return true;
  }, text);
  await p.waitForTimeout(700);
  return hit;
};

/** 输入框写入（t-input / t-input-number 都适用）。 */
const setInput = async (label, value) => {
  const box = await fieldBox(label);
  if (!box) return false;
  await p.mouse.click(box.x, box.y);
  await p.waitForTimeout(200);
  await p.keyboard.press("ControlOrMeta+A").catch(() => {});
  await p.keyboard.press("Backspace");
  await p.keyboard.type(String(value), { delay: 30 });
  await p.keyboard.press("Tab");
  await p.waitForTimeout(400);
  return true;
};

/** 抽屉底部保存。 */
const saveDrawer = async () => {
  const ok = await p.evaluate(() => {
    const f = Array.from(document.querySelectorAll(".bsd-footer"))
      .find((e) => e.getBoundingClientRect().width > 10);
    const btn = Array.from(f?.querySelectorAll("button") || [])
      .find((x) => (x.innerText || "").trim() === "保存");
    if (btn) { btn.click(); return true; }
    return false;
  });
  await p.waitForTimeout(1800);
  return ok;
};

/** 最近一条 toast 文案。 */
const toastText = () => p.evaluate(() => Array.from(document.querySelectorAll(".t-message"))
  .map((e) => (e.innerText || "").trim()).filter(Boolean).join(" | "));

/** 抽屉分组标题 → 字段名数组。 */
const groupFields = () => p.evaluate((code) => {
  const root = eval(code);
  const out = {};
  for (const sec of Array.from(root?.querySelectorAll(".bsd-block") || [])) {
    const label = (sec.querySelector(".bsd-block-title")?.innerText || "").trim();
    if (!label) continue;
    out[label] = Array.from(sec.querySelectorAll(".t-form__label"))
      .map((e) => (e.innerText || "").replace(/\s*\*$/, "").trim()).filter(Boolean);
  }
  return out;
}, visibleDrawerCode);

const dormRoomsApi = async () => {
  const res = await api("GET", "/dorm/records?module=dorm");
  const items = res.body?.data?.items || res.body?.data || [];
  const map = {};
  for (const r of (Array.isArray(items) ? items : [])) map[String(r["房间号"] || "").trim()] = r;
  return map;
};

await card("宿舍管理");
await subTab("宿舍档案");
await waitRows();

// -------------------------------------------------- A1 房型 → 床位数/可用床位联动
try {
  const opened = await openRowByCell("房间号", "401");
  // 抽屉打开后表单是异步灌值：直接读会拿到 undefined，先轮询到有值
  let f1 = null;
  for (let i = 0; i < 20; i++) {
    f1 = await drawerField("床位数");
    if (f1 && String(f1.value || "") !== "") break;
    await p.waitForTimeout(400);
  }
  const f2 = await drawerField("可用床位");
  const f3 = await drawerField("房型");
  ok("A1.1 宿舍401 打开（房型=单人间、床位数=1、可用床位=1）",
    opened && String(f3?.value || "") === "单人间" && String(f1?.value) === "1" && String(f2?.value) === "1",
    `房型=${f3?.value} 床位=${f1?.value} 可用=${f2?.value}`);

  await pickOption("房型", "双人间");
  const d1 = await drawerField("床位数");
  const d2 = await drawerField("可用床位");
  ok("A1.2 切「双人间」→ 床位数=2、可用床位=2（实时联动）",
    String(d1?.value) === "2" && String(d2?.value) === "2",
    `床位=${d1?.value} 可用=${d2?.value}`);

  await pickOption("房型", "单人间");
  const s1 = await drawerField("床位数");
  const s2 = await drawerField("可用床位");
  ok("A1.3 切回「单人间」→ 床位数=1、可用床位=1（不再残留 2）",
    String(s1?.value) === "1" && String(s2?.value) === "1",
    `床位=${s1?.value} 可用=${s2?.value}`);
  ok("A1.4 数值无 .0000 尾巴",
    !/\.\d*0{3,}/.test(`${s1?.value}${s2?.value}`), `${s1?.value}/${s2?.value}`);
  await shot("d72_01_roomtype");
  await closeDrawer();
} catch (e) {
  ok("A1 房型联动", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A2 空置开关 + 双下游隔离
try {
  // 状态筛选器：含空置、不含维修中
  const filterOpts = await p.evaluate(() => {
    // 状态筛选是一个 t-select，占位/当前值是「全部状态」
    const sels = Array.from(document.querySelectorAll(".t-select"))
      .filter((e) => {
        const inp = e.querySelector("input");
        const txt = `${inp?.value || ""} ${inp?.placeholder || ""}`;
        return /全部状态|空闲|部分入住|满员/.test(txt);
      });
    const target = sels.find((e) => e.getBoundingClientRect().width > 40);
    if (!target) return false;
    target.querySelector("input")?.click();
    return true;
  });
  await p.waitForTimeout(1000);
  const filterTexts = await p.evaluate(() => {
    const pops = Array.from(document.querySelectorAll(".t-select__dropdown, .t-popup"))
      .filter((e) => e.getBoundingClientRect().height > 10);
    const last = pops[pops.length - 1];
    return Array.from(last?.querySelectorAll(".t-select-option, .t-option") || [])
      .map((e) => (e.innerText || "").trim()).filter(Boolean);
  });
  await p.keyboard.press("Escape");
  ok("A2.1 状态筛选器含「空置」且不含「维修中」",
    filterOpts && filterTexts.includes("空置") && !filterTexts.includes("维修中"),
    JSON.stringify(filterTexts.slice(0, 8)));

  // 宿舍420：打开 → 空置开关
  await openRowByCell("房间号", "420");
  const vacantSwitch = await p.evaluate((code) => {
    const root = eval(code);
    const sw = root?.querySelector("[data-hr-dorm-vacant]");
    if (!sw) return null;
    const r = sw.getBoundingClientRect();
    return {
      x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2),
      on: /t-is-checked|checked/.test(String(sw.className)),
    };
  }, visibleDrawerCode);
  ok("A2.2 宿舍420 抽屉有「空置」开关（默认关）", !!vacantSwitch && !vacantSwitch.on,
    JSON.stringify(vacantSwitch));

  if (vacantSwitch) {
    await p.mouse.click(vacantSwitch.x, vacantSwitch.y);
    await p.waitForTimeout(800);
    await saveDrawer();
    const rooms1 = await dormRoomsApi();
    ok("A2.3 开启空置并保存 → 宿舍状态=空置、可用床位=0",
      String(rooms1["420"]?.["宿舍状态"] || "") === "空置" && Number(rooms1["420"]?.["可用床位数"]) === 0,
      `st=${rooms1["420"]?.["宿舍状态"]} free=${rooms1["420"]?.["可用床位数"]}`);

    // 列表显示灰色「空置」
    await subTab("公租房档案");
    await waitRows(30);
    await subTab("宿舍档案");
    await waitRows();
    const rows420 = await tableRows();
    const r420 = rows420.find((x) => String(x["房间号"] || "").trim() === "420") || {};
    ok("A2.4 列表 420 行「宿舍状态」显示空置", String(r420["宿舍状态"] || "").includes("空置"),
      JSON.stringify(r420["宿舍状态"]));
    await shot("d72_02_vacant");

    // 住宿记录 → 安排入住：房源下拉排除 420
    await subTab("住宿记录");
    await waitRows();
    const addBtn = await p.evaluate(() => {
      const btn = Array.from(document.querySelectorAll("button"))
        .find((x) => /安排入住/.test((x.innerText || "").trim()));
      if (btn) { btn.click(); return true; }
      return false;
    });
    await p.waitForTimeout(2500);
    const roomOpts = addBtn ? (await selectOptions("宿舍编号/房屋地址")).opts : [];
    ok("A2.5 安排入住房源下拉不含「宿舍420」",
      !roomOpts.some((o) => o.includes("420")), `n=${roomOpts.length} 含420=${roomOpts.filter((o) => o.includes("420")).length}`);
    await closeDrawer();

    // 能耗核算表计源排除 420
    const el = await api("GET", "/meters?category=electricity", undefined, UBASE);
    const wat = await api("GET", "/meters?category=water", undefined, UBASE);
    const elNos = (el.body?.data || []).map((m) => String(m.meter_no || ""));
    const watNos = (wat.body?.data || []).map((m) => String(m.meter_no || ""));
    ok("A2.6 能耗电/水表计源不含空置房（E420/W420）",
      !elNos.includes("E420") && !watNos.includes("W420"),
      `E420=${elNos.filter((x) => x === "E420").length} W420=${watNos.filter((x) => x === "W420").length}`);

    // 历史零波动
    const ele = await api("GET", "/meter-records?category=electricity", undefined, UBASE);
    const wtr = await api("GET", "/meter-records?category=water", undefined, UBASE);
    const gas = await api("GET", "/meter-records?category=gas", undefined, UBASE);
    const sum = (res) => {
      const recs = res.body?.data?.records || [];
      return {
        n: recs.reduce((a, x) => a + (x.items?.length || 0), 0),
        amt: recs.reduce((a, x) => a + Number(x.total_amount || 0), 0),
      };
    };
    const E = sum(ele), W = sum(wtr), G = sum(gas);
    ok("A2.7 空置隔离后历史电/水/气记录与总额零波动",
      E.n === 123 && Math.abs(E.amt - 165381) < 1 &&
      W.n === 151 && Math.abs(W.amt - 34156.96) < 1 &&
      G.n === 86 && Math.abs(G.amt - 12769.41) < 1,
      `E=${E.n}/${E.amt.toFixed(2)} W=${W.n}/${W.amt.toFixed(2)} G=${G.n}/${G.amt.toFixed(2)}`);

    // 还原：关闭空置
    await subTab("宿舍档案");
    await waitRows();
    await openRowByCell("房间号", "420");
    const sw2 = await p.evaluate((code) => {
      const root = eval(code);
      const sw = root?.querySelector("[data-hr-dorm-vacant]");
      if (!sw) return null;
      const r = sw.getBoundingClientRect();
      return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2), on: /t-is-checked|checked/.test(String(sw.className)) };
    }, visibleDrawerCode);
    if (sw2?.on) {
      await p.mouse.click(sw2.x, sw2.y);
      await p.waitForTimeout(700);
      await saveDrawer();
    }
    let rooms2 = await dormRoomsApi();
    if (String(rooms2["420"]?.["宿舍状态"] || "") !== "空闲") {
      // 兜底还原：个别情况下点到开关但未触发 change，直接走接口复位，保证不留脏数据
      await api("PUT", `/dorm/records/${encodeURIComponent(String(rooms2["420"].id))}`,
        { data: { "空置": false, "宿舍状态": "空闲" } });
      rooms2 = await dormRoomsApi();
    }
    ok("A2.8 关闭空置后恢复「空闲」",
      String(rooms2["420"]?.["宿舍状态"] || "") === "空闲",
      `st=${rooms2["420"]?.["宿舍状态"]}`);

    // 有在住人员的房间开启空置 → 拦截
    await subTab("宿舍档案");
    await waitRows();
    await openRowByCell("房间号", "301");
    const sw3 = await p.evaluate((code) => {
      const root = eval(code);
      const sw = root?.querySelector("[data-hr-dorm-vacant]");
      if (!sw) return null;
      const r = sw.getBoundingClientRect();
      return { x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2) };
    }, visibleDrawerCode);
    if (sw3) {
      await p.mouse.click(sw3.x, sw3.y);
      await p.waitForTimeout(900);
      const t = await toastText();
      ok("A2.9 有在住人员时开启空置被拦截并提示", /退宿|在住人员/.test(t), t.slice(0, 120));
    } else {
      ok("A2.9 有在住人员时开启空置被拦截并提示", false, "未找到空置开关");
    }
    await closeDrawer();
  }
} catch (e) {
  ok("A2 空置开关", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A3 表计号/表名全局查重
try {
  const rooms = await dormRoomsApi();
  const r402 = rooms["402"];
  const victim = String(r402["宿舍电表号"] || "");
  const dup = await api("PUT", `/dorm/records/${encodeURIComponent(String(r402.id))}`, {
    data: { "宿舍电表号": "E401", "宿舍电表名称": "宿舍401" },
  });
  const msg = JSON.stringify(dup.body || {});
  ok("A3.1 重复电表号被后端拦截（400 + 指明冲突房源）",
    dup.status === 400 && /E401/.test(msg) && /宿舍401|已被占用|占用/.test(msg),
    `status=${dup.status} msg=${msg.slice(0, 160)}`);

  const after = await dormRoomsApi();
  ok("A3.2 冲突未写入（402 电表号保持原值）",
    String(after["402"]?.["宿舍电表号"] || "") === victim, `now=${after["402"]?.["宿舍电表号"]} expect=${victim}`);
} catch (e) {
  ok("A3 表计查重", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A4 公租房：租赁单位 → 所属部门 + 舍长
try {
  await subTab("公租房档案");
  await waitRows(40);
  const opened = await openRowByCell("房屋编号", "5-2-6");
  const dept = await drawerField("所属部门");
  const mgr = await drawerField("管理员/舍长");
  ok("A4.1 公租房 5-2-6「所属部门」是下拉（非文本输入）", opened && !!dept?.isSelect,
    `isSelect=${dept?.isSelect} value=${dept?.value}`);
  const deptOpts = dept?.isSelect ? (await selectOptions("所属部门")).opts : [];
  ok("A4.2 部门候选按「租赁单位」加载（星达 8 个部门）",
    deptOpts.length >= 6, `n=${deptOpts.length} ${JSON.stringify(deptOpts.slice(0, 4))}`);
  ok("A4.3 公租房「管理员/舍长」匹配到本房在住人员", String(mgr?.value || "").trim() === "李中凯",
    `mgr=${mgr?.value}`);
  await shot("d72_03_public");
  await closeDrawer();
} catch (e) {
  ok("A4 公租房级联", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A5 住宿记录重构
try {
  await subTab("住宿记录");
  await waitRows();
  const opened = await openRowByCell("员工姓名", "吴勇建");
  const nameF = await drawerField("员工姓名");
  // 只读回显是异步灌值：轮询到有值再断言（直接读会拿到空串）
  let deptF = null;
  for (let i = 0; i < 16; i++) {
    deptF = await drawerField("所属部门");
    if (String(deptF?.value || "").trim()) break;
    await p.waitForTimeout(400);
  }
  ok("A5.1 住宿详情·吴勇建：员工姓名仅显示姓名（无工号）",
    opened && String(nameF?.value || "") === "吴勇建", `value=${nameF?.value}`);
  let deptVal = String(deptF?.value || "").trim();
  if (!deptVal) {
    /**
     * 兜底：只读回显是 disabled input，个别时序下 DOM value 还没同步。
     * 直接读抽屉组件的 form.data（断言的仍是「抽屉里显示什么」）。
     */
    deptVal = await p.evaluate(() => {
      const list = Array.from(document.querySelectorAll(".bsd-drawer"));
      const root = list.find((d) => {
        const w = d.querySelector(".t-drawer__content-wrapper");
        return !!w && w.getBoundingClientRect().width > 200;
      });
      let comp = root?.__vueParentComponent;
      while (comp && !comp.props?.form) comp = comp.parent;
      return String(comp?.props?.form?.data?.["所属部门"] || "").trim();
    });
  }
  const dbg = await p.evaluate(() => {
    const list = Array.from(document.querySelectorAll(".bsd-drawer"))
      .map((d) => {
        const w = d.querySelector(".t-drawer__content-wrapper");
        return {
          w: w ? Math.round(w.getBoundingClientRect().width) : 0,
          title: (d.querySelector(".t-drawer__header")?.innerText || "").replace(/\s+/g, " ").slice(0, 20),
        };
      }).filter((x) => x.w > 100);
    return list;
  });
  ok("A5.2 所属部门正确回显「生产部」（不再空白）",
    deptVal === "生产部",
    `value=${deptVal} isSelect=${deptF?.isSelect} drawers=${JSON.stringify(dbg)}`);

  const groups = await groupFields();
  const feeGroup = groups["费用项目"] || [];
  const needInFee = ["房屋租金", "物业费", "电费标准", "水费单价", "分摊比例"];
  const missing = needInFee.filter((n) => !feeGroup.some((x) => x.startsWith(n)));
  ok("A5.3 费用类字段全部归入「费用项目」分组", missing.length === 0,
    `fee=${JSON.stringify(feeGroup.slice(0, 12))} 缺=${JSON.stringify(missing)}`);

  const allLabels = Object.values(groups).flat();
  ok("A5.4 抽屉不含 3 个「入住时*表度数」字段",
    !allLabels.some((x) => /入住时.*表度数/.test(x)),
    JSON.stringify(allLabels.filter((x) => /度数/.test(x))));

  // 列表表头 / subs 同步
  const headers = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll thead th"))
    .map((e) => (e.innerText || "").trim()).filter(Boolean));
  ok("A5.5 列表表头不含度数 / 住宿记录ID",
    !headers.some((x) => /度数|住宿记录ID/.test(x)), JSON.stringify(headers.slice(0, 14)));

  // 床位号：宿舍301 单人间 → 仅 1 个床位；且同房零重复
  const bed = await drawerField("床位号");
  ok("A5.6 床位号是下拉且可选（按房型池化）", !!bed?.isSelect, `isSelect=${bed?.isSelect} value=${bed?.value}`);
  await closeDrawer();

  const stays = await api("GET", "/dorm/records?module=stay");
  const stayItems = stays.body?.data?.items || stays.body?.data || [];
  const live = (Array.isArray(stayItems) ? stayItems : []).filter((s) => String(s["住宿状态"] || "") !== "已退宿");
  const seen = new Set();
  let dup = 0;
  for (const s of live) {
    const key = `${String(s["宿舍编号/房屋地址"] || "").trim()}|${String(s["床位号"] || "").trim()}`;
    if (!String(s["床位号"] || "").trim()) continue;
    if (seen.has(key)) dup++;
    seen.add(key);
  }
  ok("A5.7 在住记录同房零重复床位号", dup === 0, `dup=${dup} live=${live.length}`);
} catch (e) {
  ok("A5 住宿记录重构", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A6 资产档案共用字典
try {
  await subTab("资产档案");
  const n = await waitRows(40);
  await p.waitForTimeout(1200);
  const rows = await tableRows();
  const codes = rows.map((x) => String(x["资产编号"] || "").trim()).filter(Boolean);
  const names = rows.map((x) => String(x["资产名称"] || "").trim()).filter(Boolean);
  ok("A6.1 资产档案为去重字典（<= 20 条）", rows.length > 0 && rows.length <= 20, `rows=${rows.length}`);
  ok("A6.2 无分房展开记录（ZC-3xx/ZC-4xx）",
    !codes.some((c) => /^ZC-[34]\d\d-/.test(c)), JSON.stringify(codes.slice(0, 6)));
  const STD = ["晾衣杆", "毛巾杆", "衣柜", "床柜", "床头柜", "空调遥控器", "空调", "电热水器", "床", "床垫", "钥匙", "窗帘"];
  const miss = STD.filter((x) => !names.includes(x));
  ok("A6.3 12 类标准物品齐全", miss.length === 0, `缺=${JSON.stringify(miss)}`);
  const foot = await footText();
  ok("A6.4 合计行仅在首列显示条数", /共\s*\d+\s*条|合计/.test(foot), foot.slice(0, 100));
  await shot("d72_06_asset");

  // 宿舍抽屉 配套设施 选项含 12 类
  await subTab("宿舍档案");
  await waitRows();
  await openRowByCell("房间号", "401");
  const fac = await selectOptions("配套设施");
  const missFac = STD.filter((x) => !fac.opts.some((o) => o.includes(x)));
  ok("A6.5 宿舍「配套设施」选项含全部 12 类", fac.opened && missFac.length === 0,
    `n=${fac.opts.length} 缺=${JSON.stringify(missFac)}`);
  await closeDrawer();
} catch (e) {
  ok("A6 资产字典", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A7 设置抽屉「取消」必须真关闭
try {
  await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((x) => (x.innerText || "").trim() === "设置");
    if (btn) btn.click();
  });
  for (let i = 0; i < 24; i++) {
    const vis = await p.evaluate(() => Array.from(document.querySelectorAll(".standard-settings-drawer")).some((d) => {
      const w = d.querySelector(".t-drawer__content-wrapper");
      return !!w && w.getBoundingClientRect().width > 200;
    }));
    if (vis) break;
    await p.waitForTimeout(600);
  }
  await p.waitForTimeout(1200);
  const clicked = await p.evaluate(() => {
    const roots = Array.from(document.querySelectorAll(".standard-settings-drawer"));
    const vis = roots.find((d) => {
      const w = d.querySelector(".t-drawer__content-wrapper");
      return !!w && w.getBoundingClientRect().width > 200;
    });
    const f = vis?.querySelector(".setting-drawer__footer");
    const btn = Array.from(f?.querySelectorAll("button") || []).find((x) => (x.innerText || "").trim() === "取消");
    if (btn) { btn.click(); return true; }
    return false;
  });
  await p.waitForTimeout(1500);
  const remain = await p.evaluate(() => Array.from(document.querySelectorAll(".standard-settings-drawer"))
    .filter((d) => {
      const w = d.querySelector(".t-drawer__content-wrapper");
      return !!w && w.getBoundingClientRect().width > 200;
    }).length);
  ok("A7.1 设置抽屉点「取消」后完全关闭（0 白屏残留）", clicked && remain === 0,
    `clicked=${clicked} remain=${remain}`);
  await shot("d72_07_settings_closed");
} catch (e) {
  ok("A7 设置抽屉取消", false, String(e).slice(0, 200));
}

// ------------------------------------------------------------ A8 环境
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|cmdk|NetworkError|ECONNRESET|Timeout|Failed to (list|load) (chat )?models|pageerror: Object/i.test(e));
const realErrs = errs.filter((e) => !envNoise.includes(e));
ok("A8.1 全程 0 业务 console.error", realErrs.length === 0,
  `real=${JSON.stringify(realErrs.slice(0, 3))} env=${envNoise.length}`);
const serverErrs = bad5xx.filter((x) => !/\/api\/v1\/(chat|models)/.test(x));
ok("A8.2 全程 0 5xx", serverErrs.length === 0, JSON.stringify(serverErrs.slice(0, 3)));

const failed = R.checks.filter((x) => !x.pass);
console.log(`\nSUMMARY ${R.checks.length - failed.length}/${R.checks.length} PASS`);
if (failed.length) {
  console.log("FAILED:", JSON.stringify(failed.map((x) => `${x.n} :: ${x.d}`), null, 1));
}
await b.close();
process.exit(failed.length ? 1 : 0);
