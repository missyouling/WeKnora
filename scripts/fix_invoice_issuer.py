#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""从 chunks 原文修复开票人（issuer / drawer）。

背景（第 25 轮问题 2）：
    真机「重庆璧山天然气有限责任公司」的气费发票，票面开票人是 `璧山1`
    （自助开票机工位号）。旧提取正则 `[\\p{Han}·]{2,8}` 只吃汉字、丢掉末尾
    数字，落库成 `璧山`；另有若干张完全为空。同一批票在列表里出现
    `璧山` / `璧山1` / 空 三种值。

本脚本不跑模型、不重提，直接从 chunks 原文做确定性修复：
    1. 只处理「该文件恰好 1 张票」且「原文恰好 1 个开票人」的场景 —— 多票文件
       需要按页配对，交给重提链路，避免张冠李戴；
    2. 原文里的开票人必须与库里不同才更新，避免无意义写入；
    3. 同时写 invoices[i].issuer、invoices[i].drawer、custom_metadata.issuer/drawer
       四处，保证列表/抽屉/文档摘要同源。

用法：
    python3 fix_invoice_issuer.py --dry-run   # 只打印将要修改的内容
    python3 fix_invoice_issuer.py --apply     # 实际写库
"""
import argparse
import json
import os
import re
import sys

import psycopg2

# 与 Go 侧 issuerLooseRe 保持同构：允许字间空格、冒号后换行、末尾 0~4 位字母数字
ISSUER_RE = re.compile(
    r"开\s*票\s*人\s*(?:为)?\s*[:：]?\s*\n?\s*([\u4e00-\u9fff\u00b7]{2,8}[0-9A-Za-z]{0,4})"
)
# 字段名黑名单：票面里「开票人」后面常紧跟下一列的标签（信息/备注/日期…），
# 正则会把标签首词当成人名（真机：`开票人|信息|备注` → 捕获「信息」）。
NOT_A_NAME = {
    "信息", "备注", "说明", "日期", "号码", "金额", "名称", "地址", "电话",
    "账号", "开户", "银行", "税率", "税额", "合计", "价税", "项目", "规格",
    # 表头自身复现（`开票人` 后紧跟表格分隔符再出现一次列名）
    "开票人", "收款人", "复核人",
}
# 噪词（与 Go 侧 issuerNoise 一致）
NOISE = {
    "管理员", "系统", "电子发票", "备注", "开票", "复核",
    "收款人", "销售方", "购买方", "国家税务总局", "发票专用章",
}


def connect():
    return psycopg2.connect(
        host=os.environ["DB_HOST"],
        port=os.environ.get("DB_PORT", "5432"),
        user=os.environ["DB_USER"],
        password=os.environ["DB_PASSWORD"],
        dbname=os.environ["DB_NAME"],
    )


def clean_issuer(raw: str) -> str:
    s = re.sub(r"\s+", "", raw or "")
    if not s or s in NOISE or s in NOT_A_NAME:
        return ""
    han = re.sub(r"[0-9A-Za-z]", "", s).replace("·", "")
    suffix = re.sub(r"[\u4e00-\u9fff\u00b7]", "", s)
    if not (2 <= len(han) <= 8) or len(suffix) > 4:
        return ""
    # 名字里含字段标签 → 不是人名
    for bad in NOT_A_NAME:
        if bad in s:
            return ""
    return s


def fetch_targets(conn):
    """所有发票文件：id、标题、invoices、chunks 全文。"""
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT k.id::text,
                   k.title,
                   k.custom_metadata,
                   (SELECT string_agg(c.content, E'\n' ORDER BY c.chunk_index)
                      FROM chunks c WHERE c.knowledge_id = k.id)
              FROM knowledges k
             WHERE k.deleted_at IS NULL
               AND k.custom_metadata->>'kind' = 'invoice'
               AND jsonb_typeof(k.custom_metadata->'invoices') = 'array'
            """
        )
        return cur.fetchall()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()
    if not args.apply and not args.dry_run:
        ap.error("必须指定 --apply 或 --dry-run")
    apply = args.apply

    conn = connect()
    rows = fetch_targets(conn)
    print(f"扫描 {len(rows)} 个发票文件")

    fixed_docs = 0
    fixed_items = 0
    skipped = []
    updates = []

    for kid, title, meta, text in rows:
        invs = meta.get("invoices") or []
        if len(invs) != 1:
            continue  # 多票文件需按页配对，交给重提
        if not text:
            continue
        names = []
        for m in ISSUER_RE.finditer(text):
            name = clean_issuer(m.group(1))
            if name and name not in names:
                names.append(name)
        if len(names) != 1:
            if len(names) > 1:
                skipped.append((kid, title, "原文多开票人: " + ",".join(names)))
            continue
        name = names[0]
        inv = invs[0]
        cur_issuer = (inv.get("issuer") or "").strip()
        cur_drawer = (inv.get("drawer") or "").strip()
        if cur_issuer == name and cur_drawer == name:
            continue
        # 保守策略：只在「库里为空」或「原文值是库里值的前缀扩展」（即截断）
        # 时才修。两个毫不相关的名字说明配对不可靠（多页错位），跳过等重提。
        if cur_issuer and cur_issuer != name:
            if not name.startswith(cur_issuer):
                skipped.append((kid, title, f"名字不相关: 库内 {cur_issuer} vs 原文 {name}"))
                continue
        updates.append((kid, title, cur_issuer or "(空)", name, inv.get("invoice_no")))
        fixed_docs += 1
        fixed_items += 1
        if apply:
            new_meta = json.loads(json.dumps(meta))
            ni = new_meta["invoices"][0]
            ni["issuer"] = name
            ni["drawer"] = name
            new_meta["issuer"] = name
            new_meta["drawer"] = name
            with conn.cursor() as cur:
                cur.execute(
                    "UPDATE knowledges SET custom_metadata = %s::jsonb WHERE id = %s",
                    (json.dumps(new_meta, ensure_ascii=False), kid),
                )
            conn.commit()

    for kid, title, old, new, no in updates[:40]:
        print(f"  {kid[:8]} {title[:26]:<28} {old:>6} -> {new:<8} no={no}")
    if len(updates) > 40:
        print(f"  ... 另有 {len(updates) - 40} 条")
    for kid, title, why in skipped[:10]:
        print(f"  SKIP {kid[:8]} {title[:24]} {why}")

    print(f"合计：待修 {fixed_docs} 个文件 / {fixed_items} 张票；已跳过 {len(skipped)}")
    print("模式：" + ("已写库" if apply else "dry-run（未写库）"))
    conn.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
