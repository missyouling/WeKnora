# -*- coding: utf-8 -*-
"""统一修复：水电/电费/通行费发票提取规则 items 八列口径 + 提示词强化（幂等 UPDATE by id）"""
import psycopg2, json

env = {}
with open(r"D:\Downloads\WeKnora-main\.env", encoding="utf-8", errors="ignore") as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        env[k.strip()] = v.strip()

conn = psycopg2.connect(host=env["DB_HOST"], port=env.get("DB_PORT", "5432"), user=env["DB_USER"],
                        password=env["DB_PASSWORD"], dbname=env.get("DB_NAME", "postgres"), connect_timeout=20)
cur = conn.cursor()

ITEMS_8COL = ("提取票面货物/服务明细，每行与票面明细表格行一一对应、逐列对齐「项目名称、规格型号、单位、数量、单价、金额、税率/征收率、税额」八列："
              "name 取项目名称、spec 取规格型号（无则空字符串）、unit 取单位（如千瓦时/立方米）、qty 取数量、price 取单价、"
              "amount 取票面「金额」列（不含税）、tax_rate 取该行税率（小数，如 13% 记为 0.13；0 税率填 0）、tax_amount 取「税额」列。"
              "严禁把税额填到金额、把行错位或把票面合计行当作明细行；可用勾稽关系自检（单价×数量≈金额、金额×税率≈税额），"
              "不一致时以票面各列原值为准。多行明细（如电费及政府性基金及附加）须逐行输出，行数与票面一致。")

WATER_PROMPT = ("你是一个水电发票信息提取助手。请从以下文档中提取结构化信息：{{fields_schema}}。\n\n"
                "水电发票的购买方通常为用电/用水的企业名称（户号对应单位），销售方通常为国家电网、南方电网或当地自来水/燃气公司。\n"
                "明细行提取要求：\n"
                "1. 每行明细与票面明细表格行一一对应、逐列对齐「项目名称、规格型号、单位、数量、单价、金额、税率/征收率、税额」："
                "金额取票面「金额」列（不含税），税额取「税额」列，税率取该行税率（小数，如 13% 记为 0.13、0 税率填 0），"
                "规格型号/单位无则输出空字符串，严禁把税额填到金额、把行错位或把票面合计行当作明细行。\n"
                "2. 勾稽自检：单价×数量≈金额、金额×税率≈税额；不一致时以票面各列原值为准。\n"
                "3. 多行明细税率不同时各行分别保留各自税率；发票级 tax_rate 取金额最大明细行的税率。\n"
                "4. 从明细行或备注中尝试提取总用电量/用水量、单价及计费起止日期，格式化汇总至备注字段。\n"
                "需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON；未找到的字段输出 null，不臆造内容。")

ELECTRIC_PROMPT = WATER_PROMPT.replace("水电发票信息提取助手", "电费发票信息提取助手").replace(
    "水电发票的购买方", "电费发票的购买方").replace("当地自来水/燃气公司", "当地供电公司/售电公司")

TOLL_ITEMS_RULE = ("提取票面货物/服务明细：name 取项目名称、amount 取票面「金额」列（不含税）、"
                   "tax_rate 取该行税率（小数，如 3% 记为 0.03）、tax_amount 取「税额」列；多行明细逐行输出。"
                   "通行费发票票面无规格/单位/数量/单价列时对应字段输出 null。"
                   "车牌号、车辆类型、通行日期起止等通行信息按各自独立字段提取。")

def patch_rule(rule_id, items_rule, prompt, ver):
    cur.execute("SELECT fields, prompt_template, version FROM knowledge_extract_configs WHERE id=%s AND deleted_at IS NULL", (rule_id,))
    row = cur.fetchone()
    if not row:
        print(f"!! rule {rule_id} NOT FOUND")
        return
    fields, old_prompt, old_ver = row
    # 找到 items 字段并替换 rule（规则表中 items 的 type 可能是 string/items/array，按 name 匹配）
    changed = False
    for f in fields:
        if f.get("name") == "items":
            if f.get("rule") != items_rule:
                f["rule"] = items_rule
                changed = True
    new_ver = max(old_ver or 0, ver)
    cur.execute("""
        UPDATE knowledge_extract_configs
        SET fields=%s, prompt_template=%s, version=%s, updated_at=now()
        WHERE id=%s AND deleted_at IS NULL
    """, (json.dumps(fields, ensure_ascii=False), prompt, new_ver, rule_id))
    print(f"OK rule {rule_id}: items_rule_changed={changed} ver {old_ver}->{new_ver}")

# 水电发票 c625b827
patch_rule("c625b827-59aa-4d6d-8965-c24cec354af7", ITEMS_8COL, WATER_PROMPT, 4)
# 电费发票 1978f2f4（items rule 原为空 + prompt 原为空）
patch_rule("1978f2f4-84db-4fcc-ba92-16d7e1ea7771", ITEMS_8COL, ELECTRIC_PROMPT, 2)
# 通行费发票 e788bece（items rule 强化 amount/tax_amount；prompt 在原内容基础上追加明细行要求）
cur.execute("SELECT prompt_template FROM knowledge_extract_configs WHERE id=%s AND deleted_at IS NULL", ("e788bece-6a07-4837-8825-bc25207a0ab4",))
toll_row = cur.fetchone()
toll_prompt = (toll_row[0] or "") if toll_row else ""
if "明细行提取" not in toll_prompt:
    toll_prompt = toll_prompt.replace("需要提取的字段定义见 {{fields_schema}}",
        "明细行提取：items 每行输出 name（项目名称）、amount（金额，不含税）、tax_rate（税率，小数）、tax_amount（税额）；多行逐行输出。\n\n需要提取的字段定义见 {{fields_schema}}")
patch_rule("e788bece-6a07-4837-8825-bc25207a0ab4", TOLL_ITEMS_RULE, toll_prompt, 20)

conn.commit()
print("COMMIT OK")

# 回读验证
print("\n==== 回读验证 ====")
cur.execute("""
    SELECT id, cert_type, version,
           (SELECT f->>'rule' FROM jsonb_array_elements(fields) f WHERE f->>'name'='items') AS items_rule,
           prompt_template
    FROM knowledge_extract_configs
    WHERE id IN ('c625b827-59aa-4d6d-8965-c24cec354af7','1978f2f4-84db-4fcc-ba92-16d7e1ea7771','e788bece-6a07-4837-8825-bc25207a0ab4')
    AND deleted_at IS NULL
""")
for r in cur.fetchall():
    print(f"\n=== {r[1]} ver={r[2]}")
    print("items_rule:", (r[3] or "")[:120])
    print("prompt_head:", (r[4] or "")[:200])
conn.close()
