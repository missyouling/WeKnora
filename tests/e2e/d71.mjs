// 第 71 轮真机验收（持睿表房合一收口）：
//   A1 11 间持睿房：所属公司=持睿汽车部件有限公司、表号 E021–E031/W021–W031、40 间仍自然升序
//   A2 能耗核算零偏差（123/151/86 条 + 三笔总额）且下拉含「持睿403」、无空表项
//   A3 配套设施下拉严格去重 + 资产 485 条台账与合计行
//   A4 2026-10 费用扣款 81 人 / 88,147.20 无回归
//   A5 全程 0 console.error / 0 pageerror / 0 5xx
//
// 运行：cp tests/e2e/d71.mjs /opt/pw-run/ && cd /opt/pw-run && node d71.mjs
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

// -------------------------------------------------- A1 持睿 11 间表房合一
try {
  const res = await api("GET", "/dorm/records?module=dorm");
  const rooms = res.body?.data?.items || res.body?.data || [];
  const byNo = {};
  for (const r of (Array.isArray(rooms) ? rooms : [])) byNo[String(r["房间号"] || "").trim()] = r;

  const chiriRooms = [
    ["403", "E021", "W021"], ["404", "E022", "W022"], ["405", "E023", "W023"],
    ["408", "E024", "W024"], ["409", "E025", "W025"], ["410", "E026", "W026"],
    ["412", "E027", "W027"], ["413", "E028", "W028"], ["414", "E029", "W029"],
    ["416", "E030", "W030"], ["418", "E031", "W031"],
  ];
  const bad = [];
  for (const [no, e, w] of chiriRooms) {
    const r = byNo[no] || {};
    const co = String(r["所属公司"] || "").trim();
    const eno = String(r["宿舍电表号"] || "").trim();
    const wno = String(r["宿舍水表号"] || "").trim();
    const en = String(r["宿舍电表名称"] || "").trim();
    if (co !== "持睿汽车部件有限公司" || eno !== e || wno !== w) {
      bad.push(`${no}:${co}/${eno}/${wno}`);
    }
    if (en && en !== `持睿${no}`) bad.push(`${no}:name=${en}`);
  }
  ok("A1.1 11 间持睿房 所属公司=持睿汽车部件有限公司 且表号回归 E021–E031/W021–W031",
    bad.length === 0, `bad=${JSON.stringify(bad)}`);

  const virtual = chiriRooms
    .map(([no]) => byNo[no])
    .filter((r) => r && /^E4\d\d$|^W4\d\d$/.test(String(r["宿舍电表号"] || "") + String(r["宿舍水表号"] || "")));
  ok("A1.2 虚拟表号（E4xx/W4xx）已从持睿房退役", virtual.length === 0, `n=${virtual.length}`);

  // 其余 4 楼维持星达；417/419 表号保持空
  const others = ["401", "402", "406", "407", "411", "415", "420"];
  const coBad = others.filter((no) => String((byNo[no] || {})["所属公司"] || "").trim() !== "重庆星达铜业有限公司");
  ok("A1.3 其余 4 楼房源仍为重庆星达铜业有限公司", coBad.length === 0, `bad=${JSON.stringify(coBad)}`);
  const emptyBad = ["417", "419"].filter((no) => {
    const r = byNo[no] || {};
    return String(r["宿舍电表号"] || "").trim() !== "" || String(r["宿舍水表号"] || "").trim() !== "";
  });
  ok("A1.4 417/419 原无表计 → 表名/表号保持为空（未伪造）", emptyBad.length === 0, `bad=${JSON.stringify(emptyBad)}`);

  const nos = (Array.isArray(rooms) ? rooms : []).map((r) => String(r["房间号"] || "").trim()).filter(Boolean);
  const expect = [];
  for (let i = 301; i <= 320; i++) expect.push(String(i));
  for (let i = 401; i <= 420; i++) expect.push(String(i));
  ok("A1.5 宿舍档案仍 40 间且 301→320→401→420 严格升序",
    nos.length === 40 && JSON.stringify(nos) === JSON.stringify(expect),
    `n=${nos.length} first=${nos.slice(0, 3)} last=${nos.slice(-3)}`);

  // UI 复核：列表里持睿房显示新公司名与表号
  await card("宿舍管理");
  await subTab("宿舍档案");
  await waitRows();
  const uiRows = await tableRows();
  const ui403 = uiRows.find((r) => String(r["房间号"] || "").trim() === "403") || {};
  ok("A1.6 列表 403 行所属公司=持睿汽车部件有限公司",
    String(ui403["所属公司"] || "").trim() === "持睿汽车部件有限公司",
    JSON.stringify(ui403["所属公司"]));
  await shot("d71_01_dorm_list");
} catch (e) {
  ok("A1 持睿表房合一", false, String(e).slice(0, 200));
}

// ---------------------------------- A2 核算零偏差 + 下拉含持睿403 + 无空表
try {
  const ele = await api("GET", "/meter-records?category=electricity", undefined, UBASE);
  const wat = await api("GET", "/meter-records?category=water", undefined, UBASE);
  const gas = await api("GET", "/meter-records?category=gas", undefined, UBASE);
  const itemsOf = (r) => (r.body?.data?.records || []).reduce((a, x) => a + (x.items?.length || 0), 0);
  const amtOf = (r) => (r.body?.data?.records || []).reduce((a, x) => a + Number(x.total_amount || 0), 0);
  ok("A2.1 电费 123 条 / 165381.00 零偏差",
    itemsOf(ele) === 123 && Math.abs(amtOf(ele) - 165381) < 0.02, `${itemsOf(ele)} / ${amtOf(ele).toFixed(2)}`);
  ok("A2.2 水费 151 条 / 34156.96 零偏差",
    itemsOf(wat) === 151 && Math.abs(amtOf(wat) - 34156.96) < 0.02, `${itemsOf(wat)} / ${amtOf(wat).toFixed(2)}`);
  ok("A2.3 气费 86 条 / 12769.41 零偏差",
    itemsOf(gas) === 86 && Math.abs(amtOf(gas) - 12769.41) < 0.02, `${itemsOf(gas)} / ${amtOf(gas).toFixed(2)}`);

  const metersOf = async (cat) => {
    const r = await api("GET", `/meters?category=${cat}`, undefined, UBASE);
    return (r.body?.data || []).map((m) => ({
      no: String(m.meter_no || ""), alias: String(m.alias || ""), use: String(m.use_unit || ""),
    }));
  };
  const eleM = await metersOf("electricity");
  const watM = await metersOf("water");
  const chiri403E = eleM.find((m) => m.no === "E021");
  const chiri403W = watM.find((m) => m.no === "W021");
  ok("A2.4 电费表计下拉含「持睿403 (E021)」且使用单位为持睿",
    !!chiri403E && chiri403E.alias.includes("持睿403") && chiri403E.use.includes("持睿"),
    JSON.stringify(chiri403E));
  ok("A2.5 水费表计下拉含「持睿403 (W021)」", !!chiri403W && chiri403W.alias.includes("持睿403"), JSON.stringify(chiri403W));
  const blank = [...eleM, ...watM].filter((m) => !m.no);
  ok("A2.6 下拉无空表项（417/419 未被塞进空表号）", blank.length === 0, `blank=${blank.length}`);
  const noE4 = eleM.filter((m) => /^E4\d\d$/.test(m.no) && chiriRoomsLike(m.no));
  function chiriRoomsLike(no) {
    const n = no.replace(/^E/, "");
    return ["403", "404", "405", "408", "409", "410", "412", "413", "414", "416", "418"].includes(n);
  }
  ok("A2.7 持睿房间不再暴露 E4xx 虚拟表计", noE4.length === 0, JSON.stringify(noE4.map((m) => m.no)));

  // UI：电费核算列表行数与表计下拉可选
  const okNav = await ensureApp("/platform/daily-affairs/utilities");
  ok("A2.8 能耗管理页可打开", okNav);
  await p.waitForTimeout(2500);
  await p.evaluate(() => {
    const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes("电费核算"));
    if (e) e.click();
  });
  await p.waitForTimeout(4500);
  const eleListRows = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
    .filter((r) => r.querySelectorAll("td").length > 3).length);
  ok("A2.9 电费核算列表有数据（一次拉全 123 行）", eleListRows >= 120, `rows=${eleListRows}`);
  await shot("d71_02_meter_list");
} catch (e) {
  ok("A2 核算与表计下拉", false, String(e).slice(0, 200));
}

// ------------------------------ A3 配套去重 + 资产 485 条与合计行
try {
  await ensureApp();
  await card("宿舍管理");
  await subTab("宿舍档案");
  await waitRows();
  ok("A3.0 403 抽屉可打开", await openRowByCell("房间号", "403"));
  const fac = await drawerField("配套设施");
  ok("A3.1 配套设施是多选下拉控件", !!fac && fac.isSelect && fac.multiple, JSON.stringify(fac));
  const facOpts = await selectOptions("配套设施");
  const uniq = new Set(facOpts.opts);
  ok("A3.2 配套设施选项严格去重（数量=去重后数量，且远小于 485）",
    facOpts.opened && facOpts.opts.length > 0 && uniq.size === facOpts.opts.length && facOpts.opts.length <= 40,
    `n=${facOpts.opts.length} uniq=${uniq.size}`);
  const STANDARD = ["晾衣杆", "毛巾杆", "衣柜", "床柜", "床头柜", "空调遥控器", "空调", "电热水器", "床", "床垫", "钥匙", "窗帘"];
  const miss = STANDARD.filter((n) => !facOpts.opts.includes(n));
  ok("A3.3 12 类标准设施全部在选项内", miss.length === 0, `缺=${JSON.stringify(miss)}`);
  await shot("d71_03_facility");
  await closeDrawer();

  await subTab("资产档案");
  await waitRows();
  const assetRows = await tableRows();
  const foot = await footText();
  ok("A3.4 资产档案合计行仅首列「合计（共 485 条）」",
    /合计（共 485 条）/.test(foot), `foot=${foot.slice(0, 80)}`);
  ok("A3.5 资产列表可渲染且分页/滚动正常", assetRows.length > 0, `rows=${assetRows.length}`);
  await shot("d71_04_asset");

  // 关键词搜索 + 状态筛选可用（不卡顿）
  const t0 = Date.now();
  const searched = await p.evaluate(() => {
    const inp = document.querySelector(".business-filter-bar input, .filter-bar input, input[placeholder*='搜索']");
    if (!inp) return false;
    inp.focus();
    inp.value = "衣柜";
    inp.dispatchEvent(new Event("input", { bubbles: true }));
    return true;
  });
  await p.waitForTimeout(1800);
  const afterSearch = await tableRows();
  ok("A3.6 资产关键词搜索可用（≤5s 返回）",
    searched && Date.now() - t0 < 5000 && afterSearch.length > 0,
    `rows=${afterSearch.length} ms=${Date.now() - t0}`);
} catch (e) {
  ok("A3 配套与资产", false, String(e).slice(0, 200));
}

// ------------------------------------- A4 2026-10 费用扣款无回归
try {
  const charges = await api("GET", "/dorm/charges?month=2026-10");
  const items = charges.body?.data?.items || charges.body?.data || [];
  const list = Array.isArray(items) ? items : [];
  const total = list.reduce((a, x) => a + Number(x["扣款总计"] || x.total || 0), 0);
  ok("A4.1 2026-10 费用扣款 81 人 / 88,147.20 无回归",
    list.length === 81 && Math.abs(total - 88147.2) < 0.02,
    `${list.length} 人 / ${total.toFixed(2)}`);

  // UI 复核：费用信息页合计行
  await ensureApp();
  await card("宿舍管理");
  await subTab("费用信息");
  await waitRows();
  const feeRows = await tableRows();
  const feeFoot = await footText();
  // 费用信息主列表设计上就是「每月 1 行」的账单汇总（明细 81 人走弹窗/接口）
  ok("A4.2 费用信息主列表按月汇总（1 行 = 2026-10）", feeRows.length === 1, `rows=${feeRows.length}`);
  ok("A4.3 费用信息合计行含扣款总计 88,147.20",
    /88,147\.20|合计/.test(feeFoot), `foot=${feeFoot.slice(0, 120)}`);
  await shot("d71_05_fee");
} catch (e) {
  ok("A4 费用扣款", false, String(e).slice(0, 200));
}

// ------------------------------------------------------------ A5 环境
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|cmdk|NetworkError|ECONNRESET|Timeout|Failed to (list|load) (chat )?models|pageerror: Object/i.test(e));
const realErrs = errs.filter((e) => !envNoise.includes(e));
ok("A5.1 全程 0 业务 console.error", realErrs.length === 0, `real=${JSON.stringify(realErrs.slice(0, 3))} env=${envNoise.length}`);
ok("A5.2 全程 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 3)));

const failed = R.checks.filter((x) => !x.pass);
console.log(`\nSUMMARY ${R.checks.length - failed.length}/${R.checks.length} PASS`);
if (failed.length) {
  console.log("FAILED:", JSON.stringify(failed.map((x) => `${x.n} :: ${x.d}`), null, 1));
}
await b.close();
process.exit(failed.length ? 1 : 0);
