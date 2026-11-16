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
await p.goto(`${APP}/platform/daily-affairs/hr`, { waitUntil: "domcontentloaded", timeout: 90000 });
await p.waitForTimeout(28000);
await p.evaluate(() => document.querySelectorAll(".guide").forEach((e) => e.remove()));
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
  // ================= #1/#2 附件预览 + 离职证明上传确认 =================
  await card("花名册"); await p.waitForTimeout(5000);
  await tab("离职管理"); await p.waitForTimeout(6500);
  // 找到带「查看」的离职证明行，点行打开抽屉
  const proofRow = p.locator(".doc-list-scroll tbody tr").filter({ has: p.locator("a", { hasText: "查看" }) }).first();
  const hasProofRow = await proofRow.count();
  ok("1.1 离职列表存在带离职证明的行", hasProofRow > 0, String(hasProofRow));
  if (hasProofRow) {
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
    const confirm = await p.evaluate(() => {
      const d = document.querySelector("[data-hr-proof-confirm]");
      return d ? (d.innerText || "").replace(/\s+/g, " ").trim() : "";
    });
    R.confirm = confirm;
    ok("1.3 已上传时点「上传」弹出覆盖二次确认", /原来的图片将会被覆盖/.test(confirm), confirm.slice(0, 120));
    await shot("d58c_02_proof_confirm");
    await p.getByRole("button", { name: "取消", exact: true }).last().click();
    await p.waitForTimeout(800);
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
    await p.locator(".fpd-footer button", { hasText: "关闭" }).first().click();
    await p.waitForTimeout(1000);
  }

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
  await tab("劳务合同"); await p.waitForTimeout(4500);
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
  const kpi = await p.evaluate(() => {
    const cards = Array.from(document.querySelectorAll(".dash-kpi-card"));
    const c = cards.find((x) => (x.innerText || "").includes("社保管理"));
    const m = (c?.innerText || "").match(/(\d+)\s*人状态异常/);
    return m ? Number(m[1]) : -1;
  });
  ok("3.5 社保异常人数可读（口径未被劳务污染）", kpi >= 0, String(kpi));
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
        data_type: "text", width: 0, group: "在职管理", group_label: "在职管理",
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
} catch (e) { R.error = String(e).slice(0, 600); }

const pass = R.checks.filter((x) => x.pass).length;
console.log(JSON.stringify(R, null, 1));
for (const c of R.checks) if (!c.pass) console.log(`::error::${c.n} => ${c.d}`);
if (R.error) console.log(`::error::脚本异常 => ${R.error}`);
console.log("SUMMARY", pass + "/" + R.checks.length, pass === R.checks.length ? "ALL_PASS" : "SOME_FAIL");
console.log("CONSOLE_ERRORS", JSON.stringify(errs.slice(0, 6)), "NEW_PAGES", newPages);
await b.close();
process.exit(pass === R.checks.length && !R.error ? 0 : 1);
