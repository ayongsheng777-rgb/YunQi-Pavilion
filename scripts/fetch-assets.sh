#!/bin/bash
# 拉取二进制静态资源（图标库等）。
#
# 说明：frontend/public/icons/ 等 1000+ 个图标文件体积大，且 GitHub 推送接口
# 只接受 UTF-8 文本，所以它们没有进 git。这些资源来自上游 FlatNas 开源项目，
# 从 GitHub 直接构建 Docker 镜像前，先运行本脚本把它们拉回来。
#
# 用法：./scripts/fetch-assets.sh

set -e
cd "$(dirname "$0")/.."

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo "==> 下载上游 FlatNas 资源包（约 40MB）..."
curl -sL --retry 3 https://github.com/Garry-QD/FlatNas/archive/refs/heads/main.tar.gz -o "$TMPDIR/flatnas.tar.gz"

echo "==> 解压..."
tar xzf "$TMPDIR/flatnas.tar.gz" -C "$TMPDIR"
SRC="$TMPDIR/FlatNas-main/frontend/public"

if [ ! -d "$SRC/icons" ]; then
  echo "解压后找不到图标目录，退出"; exit 1
fi

echo "==> 复制图标库到 frontend/public/（不覆盖已有文件）..."
mkdir -p frontend/public
cp -rn "$SRC/icons" frontend/public/
for f in favicon.ico ICON.PNG alipay.jpg wechat.jpg daoliyu.png; do
  [ -f "$SRC/$f" ] && cp -n "$SRC/$f" frontend/public/ 2>/dev/null || true
done

echo "==> 完成：$(find frontend/public/icons -type f | wc -l) 个图标文件就绪"
