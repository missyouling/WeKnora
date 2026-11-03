#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把页面里自写的批量浮条迁移到 BusinessFilterBar → BusinessListToolbar 的
batch-actions slot（第 26 轮 O-1）。

三页（发票/合同/奖惩）此前各写一份 `.doc-batch-bar` 外壳（fixed 定位 + 已选计数 +
清除按钮），并给 BusinessFilterBar 传 `hide-batch-bar` 把中台浮条关掉。
本脚本做等价迁移，**按钮内容与顺序 100% 保留**，只换外壳。

安全性：
- 用 `<div` / `</div>` 深度计数定位配对闭合，不用缩进猜测（模板里嵌套
  t-popconfirm，缩进层次不稳定）。
- actions 内容逐行搬运并补回换行，避免拼成一行。
- 顶部紧邻的注释行一并删除（那些注释描述的是「自写浮条」这件事本身）。

用法：python3 scripts/migrate_batch_bar.py <file.vue> [--dry-run]
"""
import argparse
import io
import re
import sys

STALE_NOTE = '底部摘要栏已下线'


def find_matching_div(src: str, start: int) -> int:
    """从 start 处的 `<div` 开始，返回配对 `</div>` 的结束偏移（含）。"""
    depth = 0
    i = start
    n = len(src)
    while i < n:
        if src.startswith('<div', i):
            depth += 1
            i += 4
            continue
        if src.startswith('</div>', i):
            depth -= 1
            i += 6
            if depth == 0:
                return i
            continue
        i += 1
    raise RuntimeError('unbalanced <div>')


def drop_leading_comment_lines(src: str, pos: int) -> int:
    """从 pos 往上删掉紧邻的注释行（含空行），返回新的块起点。"""
    head = src.rindex('\n', 0, pos) + 1
    lines = src[:head].rstrip('\n').split('\n')
    while lines and lines[-1].strip().startswith('<!--'):
        # 单行注释
        if lines[-1].strip().endswith('-->'):
            lines.pop()
            continue
        # 多行注释：往上找到 <!--
        k = len(lines) - 1
        while k >= 0 and '<!--' not in lines[k]:
            k -= 1
        if k < 0:
            break
        del lines[k:]
    return len('\n'.join(lines)) + (1 if lines else 0)


def migrate(path: str, dry: bool = False) -> None:
    src = io.open(path, encoding='utf-8').read()
    original = src

    # ---- ① 取 batch-bar-actions 的**内容**（不含外层 div）----
    m = re.search(r'\n([ \t]*)<div class="batch-bar-actions">\n', src)
    if not m:
        raise RuntimeError('未找到 batch-bar-actions')
    actions_open = src.rindex('<div', 0, m.end())
    actions_full_end = find_matching_div(src, actions_open)
    actions_close = src.rindex('</div>', 0, actions_full_end)
    actions_raw = src[m.end():actions_close]
    # 去掉一级缩进 + 去掉尾部空行
    actions_lines = actions_raw.rstrip('\n').split('\n')
    base = min(
        (len(l) - len(l.lstrip(' ')) for l in actions_lines if l.strip()),
        default=0,
    )
    actions_lines = [l[base:] if len(l) >= base else l.lstrip(' ') for l in actions_lines]
    actions = '\n'.join(actions_lines)

    # ---- ② 删除自写浮条整块 ----
    bar_open = src.rindex('<div', 0, src.index('class="doc-batch-bar"'))
    bar_end = find_matching_div(src, bar_open)
    block_start = drop_leading_comment_lines(src, bar_open)
    src = src[:block_start] + src[bar_end:].lstrip('\n')

    # ---- ③ BusinessFilterBar → 带 slot ----
    bf_start = src.index('<BusinessFilterBar')
    bf_end = src.index('/>', bf_start) + 2
    bf = src[bf_start:bf_end]
    if 'hide-batch-bar' not in bf:
        raise RuntimeError('BusinessFilterBar 上未见 hide-batch-bar（可能已迁移过）')
    bf = bf.replace(' hide-batch-bar', '')
    bf = bf.replace('@clear-selection="clearSelection"', '@clear-selection="clearSelectionSafe"')
    assert bf.endswith('/>'), 'BusinessFilterBar 不是自闭合标签'
    bf = bf[:-2].rstrip() + '>\n'

    # 缩进取自「<BusinessFilterBar 所在行的行首空白」——bf_start 指向 '<'，
    # 直接用 bf 的首行算会得到 0（因为首字符就是 '<'）。
    line_start = src.rindex('\n', 0, bf_start) + 1 if src.rindex('\n', 0, bf_start) >= 0 else 0
    indent = src[line_start:bf_start]
    if indent.strip():
        indent = ''
    inner = indent + '  '
    slot_lines = [
        f'{inner}<!-- 批量操作走中台 batch-actions slot：浮条外壳（fixed 定位 / 已选计数 /',
        f'{inner}     清除）由 BusinessListToolbar 提供，本页只声明按钮。 -->',
        f'{inner}<template #batch-actions>',
    ]
    for l in actions.split('\n'):
        slot_lines.append(f'{inner}  {l}' if l.strip() else '')
    slot_lines.append(f'{inner}</template>')
    slot_lines.append(f'{indent}</BusinessFilterBar>')
    slot = '\n'.join(slot_lines) + '\n'
    src = src[:bf_start] + bf + slot + src[bf_end:]

    # ---- ④ 清理「底部摘要栏已下线」历史注释（O-8）----
    src = re.sub(r'[ \t]*<!--[^\n]*' + STALE_NOTE + r'[^\n]*-->\n', '', src)

    if dry:
        print(f'{path}: 将 {len(original)} -> {len(src)}（未写盘）')
        return
    io.open(path, 'w', encoding='utf-8', newline='').write(src)
    print(f'{path}: {len(original)} -> {len(src)}')


if __name__ == '__main__':
    ap = argparse.ArgumentParser()
    ap.add_argument('path')
    ap.add_argument('--dry-run', action='store_true')
    a = ap.parse_args()
    try:
        migrate(a.path, a.dry_run)
    except Exception as e:  # noqa: BLE001
        print(f'FAILED: {e}', file=sys.stderr)
        sys.exit(1)
