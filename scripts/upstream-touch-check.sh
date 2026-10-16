#!/usr/bin/env bash
# 上游同步适配助手（二开分支专用）
#
# 背景：main（origin/main）是纯上游镜像，不与 v4 合并；
#       每次同步原项目后，需在二开分支 v4 上单独做一次适配。
#       凡是我们改动过的「上游已有文件」，都可能成为适配时的冲突点。
#
# 功能：算出「v4 改动过的上游文件」与「上游新改动的文件」的交集，
#       交集即本次适配必须人工确认的位置。
#
# 用法：
#   git fetch origin main
#   bash scripts/upstream-touch-check.sh            # 与当前分支比对
#   bash scripts/upstream-touch-check.sh daily-affairs-v4
#
# 退出码：0 无冲突点 / 0 且有输出为待确认项（始终 0，便于在 CI 或钩子中调用）

set -euo pipefail

BRANCH=${1:-$(git rev-parse --abbrev-ref HEAD)}
UPSTREAM_REF=${UPSTREAM_REF:-origin/main}

if ! git rev-parse --verify "$UPSTREAM_REF" >/dev/null 2>&1; then
  echo "ERROR: 找不到 $UPSTREAM_REF，请先 git fetch origin main" >&2
  exit 1
fi

BASE=$(git merge-base "$UPSTREAM_REF" "$BRANCH")
UPSTREAM_NEW=$(git rev-list --count "$BASE".."$UPSTREAM_REF")
LOCAL_NEW=$(git rev-list --count "$BASE".."$BRANCH")

echo "=== 上游同步适配检查 ==="
echo "分支        : $BRANCH"
echo "上游镜像    : $UPSTREAM_REF"
echo "同步点(BASE): $(git log --oneline -1 "$BASE")"
echo "上游待合入  : $UPSTREAM_NEW 个提交"
echo "本分支新增  : $LOCAL_NEW 个提交"
echo ""

# 本分支改动过的「上游已有文件」（排除纯新增，新增文件不会冲突）
tmp_out=$(mktemp)
tmp_up=$(mktemp)
trap 'rm -f "$tmp_out" "$tmp_up"' EXIT

git diff --diff-filter=M --name-only "$BASE".."$BRANCH" | sort -u >"$tmp_out"
git diff --name-only "$BASE".."$UPSTREAM_REF" | sort -u >"$tmp_up"

echo "本分支修改的上游文件: $(wc -l <"$tmp_out") 个"
echo "上游新改动的文件    : $(wc -l <"$tmp_up") 个"
echo ""

if [ "$UPSTREAM_NEW" -eq 0 ]; then
  echo "✅ 上游没有新提交，当前已是同步状态，无需适配。"
  exit 0
fi

# 交集 = 冲突点
mapfile -t conflicts < <(comm -12 "$tmp_out" "$tmp_up")

if [ "${#conflicts[@]}" -eq 0 ]; then
  echo "✅ 无冲突点：上游改动的文件，本分支都没碰过。可直接合入。"
  exit 0
fi

echo "⚠️  需人工确认的冲突点 ${#conflicts[@]} 处（两边都改了同一文件）："
echo ""
# 按目录聚合展示，便于定位
printf '%s\n' "${conflicts[@]}" | sed 's|/[^/]*$||' | sort | uniq -c | sort -rn |
  awk '{ printf "  %4d  %s\n", $1, $2 }'
echo ""
echo "详细清单："
printf '  %s\n' "${conflicts[@]}"
echo ""
echo "提示：优先确认带 [upstream-touch] 标记的提交，以及 "
echo "      internal/router/、internal/application/service/ 下的业务改动。"
