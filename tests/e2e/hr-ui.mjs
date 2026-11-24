// HR 人事管理 UI 回归（第 58 轮）：附件预览/上传确认、统一导入弹窗、劳务豁免、动态模板。
//
// 运行环境：需要可访问的 WeKnora 前端与后端（默认 127.0.0.1:5173 / :8080），
// 以及一个已登录 token 文件（默认 /tmp/pw/token.txt，可用 WEKNORA_TOKEN_FILE 覆盖）。
// 断言全部只读；唯一写操作是「动态模板」用例对字段配置的临时增删，脚本内 finally 还原。
//
// 本地/CI 运行：
//   cp tests/e2e/hr-ui.mjs /opt/pw-run/ && cd /opt/pw-run && node hr-ui.mjs
//   （脚本以裸包名 import "playwright"，须在装有 playwright 的目录执行）
import { chromium } from "playwright";
import fs from "node:fs";
import { execSync } from "node:child_process";

// 第 58 轮补测：附件预览 Footer / 上传覆盖确认 / 统一导入弹窗 / 劳务豁免 / 动态模板
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
const p = await c.newPage(); p.setDefaultTimeout(30000);
const errs = []; p.on("console", (m) => { if (m.type() === "error") errs.push(m.text().slice(0, 160)) });
let newPages = 0; c.on("page", () => { newPages++ });
/**
 * 第 64 轮：首屏就绪守卫。
 *
 * Vite 开发服务器偶发模块图失效（`Failed to fetch dynamically imported module`），
 * 表现为整页白屏、KPI 卡片 0 张 —— 后续所有 locator 都会超时。这里自动重载
 * 最多 4 轮，直到 5 张卡片渲染出来，再继续断言。
 */
const ensureApp = async () => {
  for (let round = 0; round < 4; round++) {
    await p.goto(`${APP}/platform/daily-affairs/hr`, { waitUntil: "domcontentloaded", timeout: 90000 });
    await p.waitForTimeout(round === 0 ? 28000 : 15000);
    for (let i = 0; i < 24; i++) {
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
const R = { checks: [] };
const ok = (n, cond, d) => R.checks.push({ n, pass: !!cond, d: String(d ?? "").slice(0, 300) });
const shot = (n) => p.screenshot({ path: `${SHOTS}/${n}.png` });
const card = (l) => p.evaluate((x) => {
  const e = Array.from(document.querySelectorAll(".dash-kpi-card")).find((q) => (q.innerText || "").includes(x));
  if (e) e.click();
}, l);
const tab = (l) => p.evaluate((x) => {
  const e = Array.from(document.querySelectorAll(".hr-subview-bar .t-radio-button")).find((q) => (q.innerText || "").trim() === x);
  if (e) e.click();
}, l);

try {
  const ready = await ensureApp();
  if (!ready) throw new Error("首屏 bootstrap 未就绪（KPI 卡片未渲染）");
  // ================= #1/#2 附件预览 + 离职证明上传确认 =================
  await card("花名册"); await p.waitForTimeout(5000);
  await tab("离职人员");
  // 第 63 轮：跨区 pooler 慢响应时列表可能还没回来，先等行（最多 20s）
  for (let i = 0; i < 13; i++) {
    const n = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3).length);
    if (n > 0) break;
    await p.waitForTimeout(1500);
  }
  await p.waitForTimeout(1500);
  // 找到带「查看」的离职证明行，点行打开抽屉
  const proofRow = p.locator(".doc-list-scroll tbody tr").filter({ has: p.locator("a", { hasText: "查看" }) }).first();
  const hasProofRow = await proofRow.count();
  ok("1.1 离职列表存在带离职证明的行", hasProofRow > 0, String(hasProofRow));
  if (hasProofRow) {
    const nameIdx = await p.evaluate(() => {
      const ths = Array.from(document.querySelectorAll(".doc-list-scroll thead th")).map((e) => (e.innerText || "").trim());
      const i = ths.indexOf("姓名");
      return i >= 0 ? i : 1;
    });
    const rowName = ((await proofRow.locator("td").nth(nameIdx).innerText()) || "").trim();
    await proofRow.locator("td").nth(1).click();
    await p.waitForTimeout(2600);
    const proofField = await p.evaluate(() => {
      const box = document.querySelector(".hr-proof-field");
      if (!box) return null;
      return {
        btns: Array.from(box.querySelectorAll("button")).map((b) => (b.innerText || "").trim()).filter(Boolean),
        tag: (box.querySelector(".hr-proof-tag")?.innerText || "").trim(),
      };
    });
    R.proofField = proofField;
    ok("1.2 抽屉离职证明无「查看」、只有「上传」",
      !!proofField && !proofField.btns.includes("查看") && proofField.btns.includes("上传"), JSON.stringify(proofField));
    await shot("d58c_01_proof_field");

    await p.locator(".hr-proof-field button", { hasText: "上传" }).first().click();
    await p.waitForTimeout(1200);
    // 第 60 轮：覆盖确认从模态框改为 t-popconfirm 气泡（模态框确认会丢 user activation，
    // 点「确定」后无法唤起系统文件选择框）。
    const confirm = await p.evaluate(() => {
      const els = Array.from(document.querySelectorAll(".t-popconfirm"))
        .filter((el) => el.offsetParent !== null);
      const d = els.find((el) => (el.innerText || "").includes("原来的图片将会被覆盖"));
      return d ? (d.innerText || "").replace(/\s+/g, " ").trim() : "";
    });
    R.confirm = confirm;
    ok("1.3 已上传时点「上传」弹出气泡覆盖确认（非模态框）", /原来的图片将会被覆盖/.test(confirm), confirm.slice(0, 120));
    ok("1.3b 覆盖确认不是 t-dialog 模态框",
      await p.evaluate(() => !document.querySelector("[data-hr-proof-confirm]")));
    await shot("d58c_02_proof_confirm");
    // 点气泡「确定」必须 100% 唤起文件选择框（同步 click，不丢 user activation）
    const chooser = p.waitForEvent("filechooser", { timeout: 8000 }).catch(() => null);
    await p.locator(".t-popconfirm button", { hasText: "确定" }).last().click();
    const fc = await chooser;
    ok("1.3c 气泡确定后成功唤起文件选择框", !!fc);
    if (fc) {
      // 用该员工**原有的离职证明**做替换上传：内容不变（只是存储路径换新），
      // 避免把演示库里的真实附件替换成占位图。
      let proofFile = `${SHOTS}/proof-upload.png`;
      try {
        const lr = await c.request.get(`${API}/api/v1/fleet/hr/roster/employees?status=${encodeURIComponent("离职")}&page_size=200`, { headers: HDR });
        const lj = await lr.json();
        const items = lj?.data?.items || lj?.items || [];
        const me = items.find((it) => String(it.name || it["姓名"] || "").trim() === rowName)
          || items.find((it) => String(it.resign_proof_path || "").trim());
        const pid = String(me?.resign_proof_path || "").split("/").filter(Boolean).pop();
        if (pid) {
          const fr = await c.request.get(`${API}/api/v1/fleet/hr/roster/resign-proof/${pid}`, { headers: HDR });
          const buf = await fr.body();
          const ext = /\.pdf/i.test(String(me.resign_proof_path)) ? "pdf" : "png";
          proofFile = `${SHOTS}/proof-upload.${ext}`;
          fs.writeFileSync(proofFile, buf);
        } else {
          fs.writeFileSync(proofFile, Buffer.from(
            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
            "base64",
          ));
        }
      } catch { /* 下载失败时退回占位图，仍能验证上传链路 */ }
      await fc.setFiles(proofFile);
      // Toast 只存活 3s：上传 + 局部 PUT 的耗时在本地与 CI 差异很大，
      // 固定 sleep 必然偶发漏读。改为「轮询到出现或超时」。
      let toast = "";
      const deadline = Date.now() + 12000;
      while (Date.now() < deadline) {
        toast = await p.evaluate(() => Array.from(document.querySelectorAll(".t-message"))
          .map((e) => (e.innerText || "").trim()).filter(Boolean).join(" | "));
        if (/离职证明已更新|同步失败|附件上传失败/.test(toast)) break;
        await p.waitForTimeout(250);
      }
      R.proofToast = toast;
      ok("1.3d 上传完成并提示离职证明已更新", /离职证明已更新/.test(toast), toast.slice(0, 120));
    } else {
      await p.keyboard.press("Escape");
    }
    await p.waitForTimeout(600);
    await p.keyboard.press("Escape"); await p.waitForTimeout(1000);

    // 列表「查看」→ FilePreviewDialog
    await p.locator(".doc-list-scroll tbody tr a", { hasText: "查看" }).first().click();
    await p.waitForTimeout(3000);
    const fpd = await p.evaluate(() => {
      const d = document.querySelector(".file-preview-dialog");
      if (!d) return null;
      return {
        foot: Array.from(d.querySelectorAll(".fpd-footer button")).map((x) => (x.innerText || "").trim()),
        toolbar: Array.from(d.querySelectorAll(".fpd-toolbar button")).map((x) => (x.innerText || "").trim()).filter(Boolean),
      };
    });
    R.fpd = fpd;
    ok("1.4 预览底部 Footer = 下载/打印/关闭",
      !!fpd && JSON.stringify(fpd.foot) === JSON.stringify(["下载", "打印", "关闭"]), JSON.stringify(fpd));
    ok("1.5 顶部工具栏不再有 打印/下载",
      !!fpd && !fpd.toolbar.includes("打印") && !fpd.toolbar.includes("下载"), JSON.stringify(fpd.toolbar));
    await shot("d58c_03_file_preview");
    // 第 63 轮：预览弹窗关闭后 DOM 仍保留，`.first()` 会点到隐藏实例 → 只点可见的那个
    await p.locator(".fpd-footer button:visible", { hasText: "关闭" }).last().click();
    await p.waitForTimeout(1000);
  }

  // ================= 第 60 轮：字段设置胶囊隔离（DOM 实渲染行） =================
  const openSettings = async () => {
    await p.keyboard.press("Escape"); await p.waitForTimeout(500);
    await p.getByRole("button", { name: "设置", exact: true }).first().click();
    await p.waitForTimeout(2400);
    return p.evaluate(() => !!document.querySelector(".field-config-row"));
  };
  const fieldRowNames = () => p.evaluate(() => Array.from(document.querySelectorAll(".field-config-row")).map((r) => ({
    name: String(r.querySelector(".fc-name-wrap input")?.value || "").trim(),
    locked: r.classList.contains("is-locked"),
  })));
  const clickCapsule = async (label) => {
    const hit = await p.evaluate((x) => {
      const el = Array.from(document.querySelectorAll(".fc-group-tab"))
        .find((e) => ((e.innerText || "").replace(/\s+/g, " ").trim()).startsWith(x));
      if (el) { el.click(); return true; }
      return false;
    }, label);
    await p.waitForTimeout(1300);
    return hit;
  };
  const closeSettings = async () => {
    await p.keyboard.press("Escape"); await p.waitForTimeout(700);
    const btn = p.locator(".t-drawer__close-btn:visible").first();
    if (await btn.count()) await btn.click().catch(() => {});
    await p.waitForTimeout(1300);
  };

  await card("劳动合同"); await p.waitForTimeout(6000);
  const cOpened = await openSettings();
  const cLabor = await fieldRowNames();
  ok("10.1 劳动合同胶囊恰有 1 个合同状态",
    cOpened && cLabor.filter((x) => x.name === "合同状态").length === 1,
    JSON.stringify({ n: cLabor.length, status: cLabor.filter((x) => x.name === "合同状态").length }));
  const cSwitched = await clickCapsule("劳务合同");
  const cService = await fieldRowNames();
  ok("10.2 劳务合同胶囊恰有 1 个合同状态",
    cSwitched && cService.filter((x) => x.name === "合同状态").length === 1,
    JSON.stringify({ n: cService.length, status: cService.filter((x) => x.name === "合同状态").length }));
  await shot("d60_11_contract_capsules");
  await closeSettings();

  await card("社保管理"); await p.waitForTimeout(6000);
  const sOpened = await openSettings();
  const SI_STATUS = ["养老参保状态", "失业参保状态", "医疗参保状态", "工伤参保状态", "生育参保状态"];
  const capsule60 = {};
  for (const [label, want] of [["正常记录", 0], ["补退记录", 0], ["月度账单", 1]]) {
    const hit = await clickCapsule(label);
    const rows = await fieldRowNames();
    capsule60[label] = { hit, n: rows.length, locked: rows.filter((x) => x.locked).map((x) => x.name) };
    ok(`10.3 社保「${label}」16 行且无串组加锁字段`,
      sOpened && hit && rows.length === 16
      && rows.filter((x) => SI_STATUS.includes(x.name)).length === 0
      && rows.filter((x) => x.name === "账单状态").length === want,
      JSON.stringify(capsule60[label]));
  }
  await shot("d60_12_social_capsules");
  await closeSettings();

  // 第 60 轮：生成账单前置校验（只读接口，不生成任何账单）
  const preEmpty = await (await c.request.get(`${API}/api/v1/fleet/hr/social/bills/precheck?month=2026-12`, { headers: HDR })).json();
  const preExists = await (await c.request.get(`${API}/api/v1/fleet/hr/social/bills/precheck?month=2026-11`, { headers: HDR })).json();
  const e6 = preEmpty?.data || preEmpty || {};
  const x6 = preExists?.data || preExists || {};
  ok("10.4 空月份 precheck 返回 0 条正常记录",
    Number(e6.normal_record_count) === 0 && e6.bill_exists === false, JSON.stringify(e6));
  ok("10.5 已有月份 precheck 返回正常记录数与账单存在",
    Number(x6.normal_record_count) > 0 && x6.bill_exists === true, JSON.stringify({ n: x6.normal_record_count, bill: x6.bill_exists }));

  // ================= #9 统一导入弹窗 =================
  const openImport = async () => {
    await p.locator("[data-hr-import]").first().click();
    await p.waitForTimeout(1600);
    return p.evaluate(() => {
      const d = Array.from(document.querySelectorAll("[data-std-import]"))
        .find((el) => el.offsetParent !== null);
      if (!d) return null;
      return {
        title: (d.querySelector(".t-dialog__header")?.innerText || "").trim(),
        template: !!d.querySelector(".si-template-row"),
        modes: !!d.querySelector(".si-mode-row"),
        drop: !!d.querySelector(".si-dropzone"),
        start: !!d.querySelector("[data-std-import-start]"),
        text: (d.innerText || "").replace(/\s+/g, " ").slice(0, 120),
      };
    });
  };
  const closeImport = async () => {
    const x = p.locator("[data-std-import]:visible .t-dialog__close").first();
    if (await x.count()) { await x.click(); }
    else { await p.keyboard.press("Escape"); }
    await p.waitForTimeout(1000);
  };

  await card("社保管理"); await p.waitForTimeout(6000);
  await tab("回盘表"); await p.waitForTimeout(6000);
  const siImp = await openImport();
  R.siImp = siImp;
  ok("9.1 回盘表导入 = 统一弹窗（模板条 + 拖拽区 + 开始导入）",
    !!siImp && siImp.template && siImp.drop && siImp.start, JSON.stringify(siImp));
  ok("9.2 回盘表固定覆盖更新（无模式切换行）", !!siImp && siImp.modes === false, JSON.stringify(siImp && siImp.modes));
  await shot("d58c_04_import_receipt");
  await closeImport();

  await tab("正常记录"); await p.waitForTimeout(6000);
  const siNormal = await openImport();
  ok("9.3 正常记录导入走同一弹窗（无模板条，多文件）",
    !!siNormal && siNormal.drop && siNormal.start && siNormal.template === false, JSON.stringify(siNormal));
  await closeImport();

  await card("花名册"); await p.waitForTimeout(6000);
  const rosterImp = await openImport();
  R.rosterImp = rosterImp;
  ok("9.4 花名册导入 = 统一弹窗（模板条 + 三模式 + 拖拽区）",
    !!rosterImp && rosterImp.template && rosterImp.modes && rosterImp.drop, JSON.stringify(rosterImp));
  await shot("d58c_05_import_roster");
  await closeImport();

  // ================= #4 劳务合同分流 + 社保/公积金豁免（只读，不改业务数据） =================
  // 口径：劳务人员（返聘/实习/劳务/借调）不得出现在公积金待补录与社保异常统计里。
  await card("劳动合同"); await p.waitForTimeout(6000);
  await tab("劳务合同");
  // 第 63 轮：跨区 pooler 偶发慢响应，列表空了就再等（最多 18s），避免误判“无独立人员”
  for (let i = 0; i < 12; i++) {
    const n = await p.evaluate(() => Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3).length);
    if (n > 0) break;
    await p.waitForTimeout(1500);
  }
  await p.waitForTimeout(1200);
  const svc = await p.evaluate(() => {
    const ths = Array.from(document.querySelectorAll("th")).map((e) => (e.innerText || "").trim());
    const iName = ths.indexOf("姓名");
    const rows = Array.from(document.querySelectorAll(".doc-list-scroll tbody tr"))
      .filter((r) => r.querySelectorAll("td").length > 3);
    return { count: rows.length, names: rows.map((r) => String(r.querySelectorAll("td")[iName]?.innerText || "").trim()).filter(Boolean) };
  });
  R.service = svc;
  ok("3.2 劳务合同标签有独立人员列表", svc.count > 0 && svc.names.length > 0, JSON.stringify(svc.names.slice(0, 8)));

  // 公积金待补录（API 口径：pending_count + status=pending 的姓名）
  const fundRes = await c.request.get(`${API}/api/v1/fleet/hr/fund/records`, { headers: HDR });
  const fundJson = await fundRes.json();
  const fundData = fundJson?.data || fundJson || {};
  const pendingNames = (fundData.items || [])
    .filter((it) => String(it.status || "") === "pending")
    .map((it) => String(it.name || it["姓名"] || "")).filter(Boolean);
  R.fundApi = { pending_count: fundData.pending_count, pendingNames, total: fundData.total };
  const overlap = svc.names.filter((n) => pendingNames.includes(n));
  ok("3.3 公积金待补录与劳务人员零交集", overlap.length === 0, JSON.stringify({ overlap, pendingNames }));
  ok("3.4 公积金 pending_count 可读且已排除劳务", Number(fundData.pending_count) >= 0, String(fundData.pending_count));

  // 社保异常人数（KPI 卡片副文案「X 人状态异常」）
  // 第 59 轮：卡片已弃用异常副文案，改为断言「只有条数、无异常字样」
  const kpiCard = await p.evaluate(() => {
    const c = Array.from(document.querySelectorAll(".dash-kpi-card"))
      .find((x) => (x.innerText || "").includes("社保管理"));
    return (c?.innerText || "").replace(/\s+/g, " ").trim();
  });
  ok("3.5 社保卡片只有条数、无异常副文案", /\d+\s*条/.test(kpiCard) && !/状态异常/.test(kpiCard), kpiCard);
  await shot("d58c_06_contract_service");

  // ================= #8 动态导入模板 =================
  const catRes = await c.request.get(`${API}/api/v1/fleet/categories?scope=hr-roster`, { headers: HDR });
  const catJson = await catRes.json();
  const cats = catJson?.data || catJson || [];
  const cat = Array.isArray(cats) ? cats[0] : null;
  ok("8.0 读到花名册字段分类", !!cat?.id, cat ? `${cat.id} ${cat.name}` : "none");
  if (cat?.id) {
    const origSubs = JSON.parse(JSON.stringify(cat.subs || []));
    const TEST_FIELD = "测试扩展列";
    const put = async (subs) => {
      const res = await c.request.put(`${API}/api/v1/fleet/categories/${cat.id}`, {
        headers: HDR,
        data: { name: cat.name, subs, enabled: cat.enabled !== false },
      });
      return res.status();
    };
    const dl = async () => {
      const r = await c.request.get(`${API}/api/v1/fleet/hr/roster/template?module=roster`, { headers: HDR });
      const buf = await r.body();
      const tplPath = `${SHOTS}/hr-template.xlsx`;
      fs.writeFileSync(tplPath, buf);
      const xml = execSync(`unzip -p ${tplPath} xl/sharedStrings.xml 2>/dev/null || true`, { encoding: "utf8" });
      return { status: r.status(), size: buf.length, has: xml.includes(TEST_FIELD) };
    };
    try {
      const before = await dl();
      R.tplBefore = before;
      ok("8.1 模板默认不含测试列", !before.has, JSON.stringify({ size: before.size }));

      const withField = [...origSubs, {
        name: TEST_FIELD, label: TEST_FIELD, enabled: true, is_default: true,
        data_type: "text", width: 0, group: "在职人员", group_label: "在职人员",
      }];
      const st1 = await put(withField);
      await p.waitForTimeout(800);
      const added = await dl();
      R.tplAdded = added;
      ok("8.2 新增自定义字段后模板同步出现该列", st1 === 200 && added.has, JSON.stringify({ put: st1, has: added.has }));

      const removedAgain = origSubs;
      const st2 = await put(removedAgain);
      await p.waitForTimeout(800);
      const afterDel = await dl();
      ok("8.3 删除字段后模板同步移除该列", st2 === 200 && !afterDel.has, JSON.stringify({ put: st2, has: afterDel.has }));
    } finally {
      const restored = await put(origSubs);
      R.restore = restored;
      ok("8.4 字段配置已还原", restored === 200, String(restored));
    }
  }

  // ================= 第 61 轮：明细三段表 / 离职证明无后缀 / 宿舍五标签 =================
  // 61.1 离职证明无后缀路由（历史种子文件动态生成 PNG，永不 404）
  const legacyName = "daaf5f1e-0a4c-497f-a103-e15f8d20ff6d.png";
  const proofRes = await c.request.get(`${API}/api/v1/fleet/hr/roster/resign-proof-view?id=${legacyName}`, { headers: HDR });
  const proofBytes = await proofRes.body();
  ok("61.1 无后缀离职证明接口 200 image/png",
    proofRes.status() === 200 && /image\/png/.test(proofRes.headers()["content-type"] || "") && proofBytes.length > 1000,
    JSON.stringify({ status: proofRes.status(), ct: proofRes.headers()["content-type"], size: proofBytes.length }));

  // 61.2 社保月度账单明细：三段分离表，只有数据区可滚动
  await card("社保管理"); await p.waitForTimeout(6000);
  await tab("月度账单"); await p.waitForTimeout(6000);
  const billRow2 = p.locator(".doc-list-scroll tbody tr").filter({ hasText: "2026-11" }).first();
  if (await billRow2.count()) {
    await billRow2.locator("td").nth(1).click();
    await p.waitForTimeout(4200);
    /**
     * 第 68 轮：明细弹窗改为**定高**（min(82vh,780px)），数据区吃掉剩余高度
     * （max-height:4000px），不再固定 380px。这里显式切到「个人明细」并等 75 行
     * 渲染完，再断言「只有数据区可滚动 + 列宽对齐」。
     */
    await p.evaluate(() => {
      const t = Array.from(document.querySelectorAll("[data-hr-bill-dialog] .hr-bill-tabs .t-radio-button"))
        .find((x) => (x.innerText || "").trim() === "个人明细");
      if (t) t.click();
    });
    await p.waitForTimeout(1500);
    for (let i = 0; i < 20; i++) {
      const n = await p.evaluate(() => {
        const dlg = Array.from(document.querySelectorAll(".t-dialog")).filter((d) => d.offsetParent !== null)
          .find((d) => d.querySelector(".detail-table"));
        return dlg ? Array.from(dlg.querySelectorAll(".detail-table-body tbody tr"))
          .filter((r) => r.querySelectorAll("td").length > 1).length : 0;
      });
      if (n > 20) break;
      await p.waitForTimeout(600);
    }
    const table61 = await p.evaluate(() => {
      const dlg = Array.from(document.querySelectorAll(".t-dialog")).filter((d) => d.offsetParent !== null)
        .find((d) => d.querySelector(".detail-table"));
      if (!dlg) return null;
      const head = dlg.querySelector(".detail-table-head");
      const body = dlg.querySelector(".detail-table-body");
      const foot = dlg.querySelector(".detail-table-foot");
      const cs = (el) => (el ? getComputedStyle(el) : null);
      const ths = Array.from(dlg.querySelectorAll(".detail-table-head th")).map((x) => Math.round(x.getBoundingClientRect().width));
      const tds = Array.from(dlg.querySelectorAll(".detail-table-body tbody tr:first-child td")).map((x) => Math.round(x.getBoundingClientRect().width));
      return {
        has: !!head && !!body && !!foot,
        head: cs(head)?.overflowY, body: cs(body)?.overflowY, foot: cs(foot)?.overflowY,
        max: cs(body)?.maxHeight, scroll: body.scrollHeight > body.clientHeight + 2,
        rows: dlg.querySelectorAll(".detail-table-body tbody tr").length,
        bodyH: Math.round(body.getBoundingClientRect().height),
        bodySH: body.scrollHeight,
        dlgH: Math.round(dlg.getBoundingClientRect().height),
        aligned: ths.length === tds.length && ths.every((w, i) => Math.abs(w - tds[i]) <= 1),
      };
    });
    R.table61 = table61;
    ok("61.2 明细弹窗仅数据区滚动且列宽对齐",
      !!table61 && table61.has && table61.head === "hidden" && table61.foot === "hidden"
      && table61.body === "auto" && table61.max === "4000px" && table61.aligned
      // 第 68 轮：定高弹窗里数据区吃满剩余高度；行数少（如部门汇总 7 行）时
      // 不发生内部滚动是正确表现，只有长表（>20 行）才必须出现滚动条。
      && (table61.rows <= 20 || table61.scroll),
      JSON.stringify(table61));
    await p.locator(".hr-bill-footer button", { hasText: "关闭" }).first().click();
    await p.waitForTimeout(1200);
  } else {
    ok("61.2 明细弹窗仅数据区滚动且列宽对齐", false, "未找到 2026-11 账单行");
  }

  // 61.3 宿舍管理五标签 + 公租房合计
  await card("宿舍管理"); await p.waitForTimeout(6500);
  const dormTabs = await p.evaluate(() => Array.from(document.querySelectorAll(".hr-subview-bar .t-radio-button"))
    .map((e) => (e.innerText || "").trim()));
  ok("61.3 宿舍管理五标签", dormTabs.join("/") === "宿舍档案/公租房档案/住宿记录/资产档案/费用信息", dormTabs.join("/"));
  await tab("公租房档案"); await p.waitForTimeout(5500);
  const pub61 = await p.evaluate(() => ({
    rows: Array.from(document.querySelectorAll(".doc-list-scroll tbody tr")).filter((r) => r.querySelectorAll("td").length > 3).length,
    foot: (document.querySelector(".doc-list-scroll tfoot")?.innerText || "").replace(/\s+/g, " ").trim(),
  }));
  R.pub61 = pub61;
  // 第 65 轮：公租房档案合计行改为「仅首列显示 合计（共 N 条）」，其余列一律留空
  // （建筑面积等数值汇总按需求移除），故不再断言 485.55。
  ok("61.4 公租房 19 套且合计行仅首列有文案",
    pub61.rows === 19 && /合计（共 19 条）/.test(pub61.foot) && !/485\.55/.test(pub61.foot),
    JSON.stringify(pub61));
  await shot("d61_ci_dorm_public");
} catch (e) { R.error = String(e).slice(0, 600); }

const pass = R.checks.filter((x) => x.pass).length;
console.log(JSON.stringify(R, null, 1));
for (const c of R.checks) if (!c.pass) console.log(`::error::${c.n} => ${c.d}`);
if (R.error) console.log(`::error::脚本异常 => ${R.error}`);
console.log("SUMMARY", pass + "/" + R.checks.length, pass === R.checks.length ? "ALL_PASS" : "SOME_FAIL");
console.log("CONSOLE_ERRORS", JSON.stringify(errs.slice(0, 6)), "NEW_PAGES", newPages);
await b.close();
process.exit(pass === R.checks.length && !R.error ? 0 : 1);
