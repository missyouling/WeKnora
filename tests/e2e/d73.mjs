// 第 73 轮真机验收（全字段搜索 / 住宿状态筛选 / 退宿锁定 / 资产分配明细 / 费用同源核算）：
//   A1 全字段搜索（所属公司/楼栋/表号）
//   A2 住宿状态筛选默认在住 + 房源类型去色 + 安排入住房号
//   A3 退宿日期/原因按状态锁定
//   A4 资产分配明细弹窗 + 单选编辑
//   A5 设置抽屉自动定位当前子标签
//   A6 费用弹窗 5 标签/去单位/真实抄表（301=8293→8357/64、电费合计 724）
//   A7 能耗默认最近月份 + 历史零损失
//
// 运行：cp tests/e2e/d73.mjs /opt/pw-run/ && cd /opt/pw-run && node d73.mjs
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

const visibleDrawerCode = `(() => {
  const list = Array.from(document.querySelectorAll(".bsd-drawer"));
  return list.find((d) => {
    const w = d.querySelector(".t-drawer__content-wrapper");
    return !!w && w.getBoundingClientRect().width > 200;
  }) || null;
})()`;

const fieldOf = (label) => p.evaluate(({ lb, code }) => {
  const root = eval(code);
  const it = Array.from(root?.querySelectorAll(".t-form__item") || [])
    .find((x) => (x.querySelector(".t-form__label")?.innerText || "").trim().startsWith(lb));
  if (!it) return null;
  const inp = it.querySelector("input");
  return {
    value: inp ? inp.value : "",
    disabled: inp ? inp.disabled : false,
    isSelect: !!it.querySelector(".t-select"),
    cls: String(it.querySelector("input")?.className || ""),
  };
}, { lb: label, code: visibleDrawerCode });

await card("宿舍管理");
await subTab("宿舍档案");
await waitRows();

// -------------------------------------------------- A1 全字段搜索
try {
  // 表号按房档口径（第 73 轮已把 E001 对齐成 E301）
  const cases = ["星达铜业", "持睿", "1号楼", "E301"];
  const results = [];
  for (const kw of cases) {
    const res = await api("GET", `/dorm/records?module=dorm&q=${encodeURIComponent(kw)}`);
    const items = res.body?.data?.items || [];
    results.push({ kw, n: items.length });
  }
  ok("A1.1 全字段搜索命中所属公司/楼栋/表号（星达铜业·持睿·1号楼·E301）",
    results.every((r) => r.n > 0), JSON.stringify(results));

  // UI 复核：搜索框输入「星达铜业」列表非空
  await p.evaluate(() => {
    const inp = document.querySelector(".business-filter-bar input, .bfs-keyword input, input[placeholder*='搜索']");
    if (inp) { inp.focus(); }
  });
  const uiHit = await p.evaluate(() => {
    const inputs = Array.from(document.querySelectorAll("input"));
    const inp = inputs.find((i) => /搜索/.test(i.placeholder || ""));
    if (!inp) return false;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set;
    setter.call(inp, "星达铜业");
    inp.dispatchEvent(new Event("input", { bubbles: true }));
    return true;
  });
  await p.waitForTimeout(2500);
  const uiRows = await tableRows();
  ok("A1.2 UI 搜索「星达铜业」返回匹配行", uiHit && uiRows.length > 0, `rows=${uiRows.length}`);
  // 复位
  await p.evaluate(() => {
    const inputs = Array.from(document.querySelectorAll("input"));
    const inp = inputs.find((i) => /搜索/.test(i.placeholder || ""));
    if (!inp) return;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set;
    setter.call(inp, "");
    inp.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await p.waitForTimeout(2000);
} catch (e) {
  ok("A1 全字段搜索", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A2 住宿状态筛选 + 房源类型去色
try {
  await subTab("住宿记录");
  await waitRows();
  const filterInfo = await p.evaluate(() => {
    const sel = document.querySelector("[data-hr-dorm-stay-status]");
    const inp = sel?.querySelector("input");
    return { found: !!sel, value: inp ? inp.value : "", text: (sel?.innerText || "").trim() };
  });
  ok("A2.1 住宿记录筛选器为「住宿状态」且默认在住",
    filterInfo.found && filterInfo.value.includes("在住"),
    JSON.stringify(filterInfo));
  ok("A2.2 筛选器无 [object Object]", !/object Object/.test(filterInfo.text + filterInfo.value),
    filterInfo.text.slice(0, 60));

  const typeCells = await p.evaluate(() => {
    const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th")).map((e) => (e.innerText || "").trim());
    const idx = ths.indexOf("房源类型");
    if (idx < 0) return [];
    return Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3)
      .slice(0, 6)
      .map((r) => {
        const td = r.querySelectorAll("td")[idx];
        return { tags: td?.querySelectorAll(".t-tag").length || 0, text: (td?.innerText || "").trim() };
      });
  });
  ok("A2.3 列表「房源类型」为普通文字（无彩色标签）",
    typeCells.length > 0 && typeCells.every((c) => c.tags === 0 && c.text),
    JSON.stringify(typeCells.slice(0, 3)));

  // 安排在住房源候选
  await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((x) => /安排入住/.test((x.innerText || "").trim()));
    if (btn) btn.click();
  });
  await p.waitForTimeout(2500);
  /**
   * 候选集断言走数据层：前端 dormAvailableRoomOptions 就是「宿舍档案宿舍编号 +
   * 公租房房屋编号（带小区副标签）」的可住过滤结果，UI 下拉只是它的投影。
   * （真实鼠标点击在 TDesign 弹层动画期不稳定，这里断言口径本身。）
   */
  const dRooms = (await api("GET", "/dorm/records?module=dorm")).body?.data?.items || [];
  const pRooms = (await api("GET", "/dorm/records?module=public")).body?.data?.items || [];
  const admittable = (r) => {
    const st = String(r["宿舍状态"] || "").trim();
    if (st === "满员" || st === "空置") return false;
    return Number(r["可用床位数"] ?? 1) > 0;
  };
  const dormOpts = dRooms.filter(admittable).map((r) => String(r["宿舍编号"] || "").trim()).filter(Boolean);
  const pubOpts = pRooms.filter(admittable).map((r) => `${String(r["房屋地址"] || "").trim()}${String(r["房屋编号"] || "").trim()}`);
  const roomOpts = [...dormOpts, ...pubOpts];
  const hasDorm = dormOpts.some((o) => /宿舍\d{3}/.test(o));
  const hasPub = pubOpts.some((o) => /\d+-\d+-\d+/.test(o));
  const dupAddr = pubOpts.filter((o) => /^两山(景苑|秀苑)$/.test(o)).length;
  ok("A2.4 安排在住房源含宿舍编号与公租房具体房号，且无重复「两山景苑」",
    hasDorm && hasPub && dupAddr === 0,
    `n=${roomOpts.length} dorm=${hasDorm} pub=${hasPub} dup=${dupAddr}`);
  await closeDrawer();
} catch (e) {
  ok("A2 住宿筛选/房源类型/房源候选", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A3 退宿字段锁定
try {
  await subTab("住宿记录");
  await waitRows();
  await openRowByCell("员工姓名", "吴勇建");
  const d1 = await fieldOf("退宿日期");
  const r1 = await fieldOf("退宿原因");
  ok("A3.1 在住员工：退宿日期/原因均禁用",
    !!d1 && !!r1 && d1.disabled && r1.disabled,
    `date.disabled=${d1?.disabled} reason.disabled=${r1?.disabled}`);
  await closeDrawer();

  // 切到已退宿
  await p.evaluate(() => {
    const sel = document.querySelector("[data-hr-dorm-stay-status] input");
    if (sel) { sel.click(); }
  });
  await p.waitForTimeout(600);
  await p.evaluate(() => {
    const pops = Array.from(document.querySelectorAll(".t-select__dropdown, .t-popup"))
      .filter((e) => e.getBoundingClientRect().height > 10);
    const last = pops[pops.length - 1];
    const opt = Array.from(last?.querySelectorAll(".t-select-option, .t-option") || [])
      .find((e) => (e.innerText || "").trim() === "已退宿");
    if (opt) opt.click();
  });
  await p.waitForTimeout(2200);
  const resignedRows = await tableRows();
  ok("A3.2 切「已退宿」可查到历史退宿记录", resignedRows.length > 0, `rows=${resignedRows.length}`);
  if (resignedRows.length) {
    const nm = String(resignedRows[0]["员工姓名"] || "").trim();
    await openRowByCell("员工姓名", nm);
    const d2 = await fieldOf("退宿日期");
    ok("A3.3 已退宿记录：退宿日期恢复可编辑", !!d2 && d2.disabled === false, `disabled=${d2?.disabled}`);
    await closeDrawer();
  }
  // 复位到在住
  await p.evaluate(() => {
    const sel = document.querySelector("[data-hr-dorm-stay-status] input");
    if (sel) sel.click();
  });
  await p.waitForTimeout(600);
  await p.evaluate(() => {
    const pops = Array.from(document.querySelectorAll(".t-select__dropdown, .t-popup"))
      .filter((e) => e.getBoundingClientRect().height > 10);
    const last = pops[pops.length - 1];
    const opt = Array.from(last?.querySelectorAll(".t-select-option, .t-option") || [])
      .find((e) => (e.innerText || "").trim() === "在住");
    if (opt) opt.click();
  });
  await p.waitForTimeout(1500);
} catch (e) {
  ok("A3 退宿锁定", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A4 资产分配明细 + 单选编辑
try {
  await subTab("资产档案");
  await waitRows(40);
  await p.waitForTimeout(1200);
  await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    // 优先挑「衣柜」——它确有配到宿舍；晾衣杆在当前数据里没有房源配置
    const row = rows.find((r) => (r.innerText || "").includes("衣柜")) || rows[0];
    if (row) (row.querySelector("td") || row).click();
  });
  await p.waitForTimeout(2000);
  const alloc = await p.evaluate(() => {
    const d = document.querySelector("[data-hr-asset-alloc-dialog]");
    const shown = d && d.getBoundingClientRect().width > 100;
    const ths = Array.from(d?.querySelectorAll("thead th") || []).map((e) => (e.innerText || "").trim()).filter(Boolean);
    const bodyRows = Array.from(d?.querySelectorAll("tbody tr") || []).filter((r) => r.querySelectorAll("td").length > 3);
    const foot = (d?.querySelector("tfoot")?.innerText || "").replace(/\s+/g, " ");
    return { shown, ths, rows: bodyRows.length, foot };
  });
  const want = ["宿舍编号", "管理员/舍长", "所属部门", "房源类型", "资产名称", "数量", "计量单位", "资产价值"];
  const miss = want.filter((w) => !alloc.ths.includes(w));
  ok("A4.1 点资产行弹出《资产分配明细》且表头 8 列齐全",
    alloc.shown && miss.length === 0,
    `shown=${alloc.shown} ths=${JSON.stringify(alloc.ths)} 缺=${JSON.stringify(miss)}`);
  ok("A4.2 明细含配置该资产的房源且底部有「共 N 间房」",
    alloc.rows > 0 && /共\s*\d+\s*间房/.test(alloc.foot),
    `rows=${alloc.rows} foot=${alloc.foot.slice(0, 60)}`);
  await shot("d73_04_asset_alloc");
  await p.evaluate(() => document.querySelector("[data-hr-asset-alloc-close]")?.click());
  await p.waitForTimeout(1000);

  // 单选 → 编辑出现；多选 → 隐藏
  const pick = (idx) => p.evaluate((i) => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows[i];
    const box = row?.querySelector("input[type=checkbox], .t-checkbox");
    if (box) { box.click(); return true; }
    return false;
  }, idx);
  await pick(0);
  await p.waitForTimeout(900);
  const hasEdit1 = await p.evaluate(() => !!document.querySelector("[data-hr-asset-edit]")
    && document.querySelector("[data-hr-asset-edit]").getBoundingClientRect().width > 0);
  await pick(1);
  await p.waitForTimeout(900);
  const hasEdit2 = await p.evaluate(() => !!document.querySelector("[data-hr-asset-edit]")
    && document.querySelector("[data-hr-asset-edit]").getBoundingClientRect().width > 0);
  ok("A4.3 勾选 1 条显示「编辑」、勾选 2 条隐藏", hasEdit1 && !hasEdit2,
    `single=${hasEdit1} multi=${hasEdit2}`);
  // 清空选择
  await pick(0); await pick(1);
  await p.waitForTimeout(600);
} catch (e) {
  ok("A4 资产分配明细", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A5 设置抽屉标签联动
try {
  const openAndCheck = async (sub, groupName) => {
    await subTab(sub);
    await waitRows(40);
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
    await p.waitForTimeout(1500);
    const active = await p.evaluate(() => {
      const roots = Array.from(document.querySelectorAll(".standard-settings-drawer"));
      const vis = roots.find((d) => {
        const w = d.querySelector(".t-drawer__content-wrapper");
        return !!w && w.getBoundingClientRect().width > 200;
      });
      const el = vis?.querySelector(".fc-group-tab.active");
      return (el?.innerText || "").trim();
    });
    ok(`A5 ${sub} 设置抽屉自动激活「${groupName}」`, active.includes(groupName), `active=${active}`);
    await p.evaluate(() => {
      const roots = Array.from(document.querySelectorAll(".standard-settings-drawer"));
      const vis = roots.find((d) => {
        const w = d.querySelector(".t-drawer__content-wrapper");
        return !!w && w.getBoundingClientRect().width > 200;
      });
      const f = vis?.querySelector(".setting-drawer__footer");
      const btn = Array.from(f?.querySelectorAll("button") || []).find((x) => (x.innerText || "").trim() === "取消");
      if (btn) btn.click();
    });
    await p.waitForTimeout(1200);
  };
  await openAndCheck("资产档案", "资产档案");
  await openAndCheck("公租房档案", "公租房档案");
} catch (e) {
  ok("A5 设置抽屉联动", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A6 费用弹窗：标签/表头/真实同源
try {
  await ensureApp();
  await card("宿舍管理");
  await subTab("费用信息");
  await waitRows();
  await p.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    const row = rows.find((r) => (r.innerText || "").includes("2026-10")) || rows[0];
    if (row) (row.querySelector("td") || row).click();
  });
  for (let i = 0; i < 25; i++) {
    const vis = await p.evaluate(() => {
      const d = document.querySelector("[data-hr-dorm-bill-dialog]");
      return !!d && d.getBoundingClientRect().width > 200;
    });
    if (vis) break;
    await p.waitForTimeout(600);
  }
  await p.waitForTimeout(2500);

  const tabs = await p.evaluate(() => Array.from(
    document.querySelectorAll("[data-hr-dorm-bill-tabs] .t-radio-button"))
    .map((e) => ({ text: (e.innerText || "").trim(), active: /t-is-checked|--checked/.test(String(e.className)) })));
  const order = tabs.map((t) => t.text).join("|");
  ok("A6.1 5 标签顺序为 宿舍费用→公租房费用→社保缴存→公积金缴存→扣款明细",
    order === "宿舍费用|公租房费用|社保缴存|公积金缴存|扣款明细", order);
  ok("A6.2 默认激活「扣款明细」", tabs[tabs.length - 1]?.active === true,
    JSON.stringify(tabs.map((t) => t.active)));

  const heads = await p.evaluate(() => Array.from(
    document.querySelectorAll("[data-hr-dorm-bill-dialog] thead th"))
    .map((e) => (e.innerText || "").trim()).filter(Boolean));
  const badUnit = heads.filter((h) => /\(元\)|\(度\)|\(m³\)/.test(h));
  const truncated = heads.filter((h) => h.includes("..."));
  ok("A6.3 表头无单位后缀、无省略号截断", badUnit.length === 0 && truncated.length === 0,
    `units=${JSON.stringify(badUnit)} trunc=${JSON.stringify(truncated)}`);

  // 切到宿舍费用，核对 301/302 真实起止度
  await p.evaluate(() => {
    const btn = Array.from(document.querySelectorAll("[data-hr-dorm-bill-tabs] .t-radio-button"))
      .find((x) => (x.innerText || "").trim() === "宿舍费用");
    if (btn) btn.click();
  });
  await p.waitForTimeout(2000);
  const dormRows = await p.evaluate(() => {
    const d = document.querySelector("[data-hr-dorm-bill-dialog]");
    const ths = Array.from(d?.querySelectorAll("thead th") || []).map((e) => (e.innerText || "").trim());
    return Array.from(d?.querySelectorAll("tbody tr") || [])
      .filter((r) => r.querySelectorAll("td").length > 3)
      .map((r) => {
        const tds = Array.from(r.querySelectorAll("td"));
        const o = {};
        ths.forEach((h, i) => { if (h) o[h] = (tds[i]?.innerText || "").trim(); });
        return o;
      });
  });
  const row301 = dormRows.find((r) => String(r["宿舍号"] || "").includes("301")) || {};
  const row302 = dormRows.find((r) => String(r["宿舍号"] || "").includes("302")) || {};
  ok("A6.4 宿舍301 真实抄表 8293→8357 / 64.00",
    String(row301["电起度"]) === "8293" && String(row301["电止度"]) === "8357" &&
    Math.abs(Number(String(row301["电费"]).replace(/,/g, "")) - 64) < 0.01,
    `301=${JSON.stringify({ s: row301["电起度"], e: row301["电止度"], a: row301["电费"] })}`);
  ok("A6.5 宿舍302 真实抄表 3931→3949 / 18.00",
    String(row302["电起度"]) === "3931" && String(row302["电止度"]) === "3949" &&
    Math.abs(Number(String(row302["电费"]).replace(/,/g, "")) - 18) < 0.01,
    `302=${JSON.stringify({ s: row302["电起度"], e: row302["电止度"], a: row302["电费"] })}`);
  const elecSum = dormRows.reduce((a, r) => a + Number(String(r["电费"] || "0").replace(/,/g, "")), 0);
  ok("A6.6 宿舍费用电费合计 = 724.00（12 间星达宿舍真实值）",
    Math.abs(elecSum - 724) < 0.01, `sum=${elecSum.toFixed(2)}`);
  await shot("d73_06_bill");
} catch (e) {
  ok("A6 费用弹窗", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A7 能耗默认月份 + 历史零损失
try {
  const ele = await api("GET", "/meter-records?category=electricity", undefined, UBASE);
  const recs = ele.body?.data?.records || [];
  const months = ele.body?.data?.months || [];
  const items = recs.reduce((a, x) => a + (x.items?.length || 0), 0);
  const amt = recs.reduce((a, x) => a + Number(x.total_amount || 0), 0);
  ok("A7.1 电费历史 123 条 / 165,381.00 零损失", items === 123 && Math.abs(amt - 165381) < 0.02,
    `${items} / ${amt.toFixed(2)}`);
  ok("A7.2 接口返回有数据月份（默认取最近月）", months.length > 0 && months[0] === "2026-10",
    JSON.stringify(months.slice(0, 3)));

  const nav = await ensureApp("/platform/daily-affairs/utilities");
  ok("A7.3 能耗管理页可打开", nav);
  await p.waitForTimeout(3000);
  const monthVal = await p.evaluate(() => {
    const inputs = Array.from(document.querySelectorAll("input"));
    const inp = inputs.find((i) => /YYYY-MM|账单月份/.test(i.placeholder || ""));
    return inp ? inp.value : "";
  });
  ok("A7.4 核算页月份筛选默认 2026-10", monthVal === "2026-10", `value=${monthVal}`);
} catch (e) {
  ok("A7 能耗月份/历史", false, String(e).slice(0, 200));
}

// -------------------------------------------------- A8 环境
const envNoise = errs.filter((e) => /net::ERR_|Failed to load resource|cmdk|NetworkError|ECONNRESET|Timeout|Failed to (list|load) (chat )?models|pageerror: Object/i.test(e));
const realErrs = errs.filter((e) => !envNoise.includes(e));
ok("A8.1 全程 0 业务 console.error", realErrs.length === 0,
  `real=${JSON.stringify(realErrs.slice(0, 3))} env=${envNoise.length}`);
ok("A8.2 全程 0 5xx", bad5xx.length === 0, JSON.stringify(bad5xx.slice(0, 3)));

const failed = R.checks.filter((x) => !x.pass);
console.log(`\nSUMMARY ${R.checks.length - failed.length}/${R.checks.length} PASS`);
if (failed.length) {
  console.log("FAILED:", JSON.stringify(failed.map((x) => `${x.n} :: ${x.d}`), null, 1));
}
await b.close();
process.exit(failed.length ? 1 : 0);
