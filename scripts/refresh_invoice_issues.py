#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""按「已落库字段」重算发票的 extract_issues（第 26 轮 T13b）。

问题：extract_issues 只在**重新提取**时才重算。历史文件的告警可能是早期算法
留下的（例如把备注里的 20 位银行账号当第二张发票，报「原文共 2 张锚点，
仅提取到 1 张」），而该文件的发票字段其实早已齐全 —— 于是详情页永久挂着
一条误导性的橙色警告。

本脚本不调模型、不读 PDF，只对 `custom_metadata.invoices` 里**已有的字段值**
做确定性判定：

**逐条判定，不清整包**（首次实现按「核心字段齐全就整包清空」，dry-run 发现
会误清「用户配置为必填但未提取到该字段」这类真实提示 —— 它对补录仍有价值）：

  1. 锚点类（detail 含「张发票锚点」）→ **总是清**。
     它是早期锚点算法的产物（把备注里的 20 位银行账号当第二张票），
     而 `CountInvoiceAnchors` 已修正为「优先认『发票号码：』标签并按票号去重」，
     这条结论在当前算法下不可能再成立。
  2. 其他 issue：按 `field` + `index` 定位到具体那张票 ——
     若该字段**现在非空**，说明用户/重提已补齐，这条已解决 → 清；
     若仍为空，说明是真问题 → 保留。
  3. 定位不到（index=-1 且非锚点类）→ 保留（宁可留着让人看）。

与后端 RevalidateInvoiceIssues 的分工：那个函数在**用户保存**时按完整校验器
重算（能识别日期格式、金额自洽等更细的问题）；本脚本只处理「存量里字段早
已齐全、却还挂着告警」这一种情况，判定口径刻意保守（只比核心字段），
避免误清真实问题。

用法：
    python3 scripts/refresh_invoice_issues.py --dry-run
    python3 scripts/refresh_invoice_issues.py --apply
"""
import argparse
import json
import os
import sys

import psycopg2


# 后端校验器用的字段名（snake_case）→ 票对象上的键
FIELD_ALIASES = {
    "invoice_no": ["invoice_no", "invoice_code"],
    "invoice_date": ["invoice_date"],
    "amount": ["amount"],
    "tax": ["tax"],
    "total_amount": ["total_amount"],
    "tax_rate": ["tax_rate"],
    "buyer_name": ["buyer_name"],
    "seller_name": ["seller_name"],
    "issuer": ["issuer", "drawer"],
    "remark": ["remark"],
}


def field_filled(inv: dict, field: str) -> bool:
    """该票的指定字段现在是否有值（数组/浮点都按「非空」处理）。"""
    keys = FIELD_ALIASES.get(field, [field])
    for k in keys:
        v = inv.get(k)
        if isinstance(v, str) and v.strip():
            return True
        if isinstance(v, (int, float)) and not isinstance(v, bool):
            return True
        if isinstance(v, (list, dict)) and len(v) > 0:
            return True
    return False


def is_anchor_issue(issue: dict) -> bool:
    return "张发票锚点" in str(issue.get("detail") or "")


def issue_resolved(issue: dict, invs: list) -> bool:
    """判断某条 issue 是否已因字段补齐而失效。"""
    if is_anchor_issue(issue):
        return True  # 算法已修正，旧结论必然过期
    idx = issue.get("index")
    field = str(issue.get("field") or "")
    if not isinstance(idx, int) or idx < 0 or idx >= len(invs):
        return False
    target = invs[idx]
    if not isinstance(target, dict):
        return False
    return field_filled(target, field)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--dry-run", action="store_true")
    a = ap.parse_args()
    if not (a.apply or a.dry_run):
        ap.error("必须指定 --apply 或 --dry-run")

    conn = psycopg2.connect(
        host=os.environ["DB_HOST"],
        port=os.environ.get("DB_PORT", "5432"),
        user=os.environ["DB_USER"],
        password=os.environ["DB_PASSWORD"],
        dbname=os.environ["DB_NAME"],
    )
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT id::text, title, custom_metadata
              FROM knowledges
             WHERE deleted_at IS NULL
               AND custom_metadata->>'kind' = 'invoice'
               AND jsonb_typeof(custom_metadata->'extract_issues') = 'array'
               AND custom_metadata->'extract_issues' <> '[]'::jsonb
            """
        )
        rows = cur.fetchall()

    print(f"带 extract_issues 的发票文件：{len(rows)}")
    cleared, kept, skipped = 0, 0, 0
    samples = []

    for kid, title, meta in rows:
        invs = meta.get("invoices")
        if not isinstance(invs, list) or not invs:
            skipped += 1
            continue
        issues = meta.get("extract_issues") or []
        if not isinstance(issues, list) or not issues:
            continue
        remain = [it for it in issues if not (isinstance(it, dict) and issue_resolved(it, invs))]
        removed = len(issues) - len(remain)
        if removed == 0:
            kept += 1
            continue
        cleared += 1
        if len(samples) < 20:
            samples.append((kid, title, removed, len(remain)))
        if a.apply:
            new_meta = dict(meta)
            new_meta["extract_issues"] = remain
            with conn.cursor() as cur:
                cur.execute(
                    "UPDATE knowledges SET custom_metadata = %s::jsonb WHERE id = %s",
                    (json.dumps(new_meta, ensure_ascii=False), kid),
                )
            conn.commit()

    for kid, title, n, left_n in samples:
        print(f"  {kid[:8]} {title[:34]:<36} 清 {n} 条，留 {left_n} 条")
    if cleared > len(samples):
        print(f"  ... 另有 {cleared - len(samples)} 个")
    print(f"合计：有可清项 {cleared} / 无需变动 {kept} / 跳过 {skipped}")
    print("模式：" + ("已写库" if a.apply else "dry-run（未写库）"))
    conn.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
