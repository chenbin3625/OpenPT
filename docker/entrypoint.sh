#!/bin/sh
set -eu

DATA_DIR="${OPENPT_DATA_DIR:-/data}"
APP_DIR="${OPENPT_APP_DIR:-/app}"

# 创建必要的目录结构（不覆盖已有内容）
mkdir -p "${DATA_DIR}/torrents" "${DATA_DIR}/clients" "${DATA_DIR}/torrents_archive"

# 仅复制默认 config.toml（如果不存在）
if [ ! -f "${DATA_DIR}/config.toml" ]; then
  cp "${APP_DIR}/examples/config.docker.toml" "${DATA_DIR}/config.toml"
  echo "created default config: ${DATA_DIR}/config.toml"
fi

# 仅复制不存在的 client 文件
for client in "${APP_DIR}"/clients/*.client; do
  [ -e "${client}" ] || continue
  target="${DATA_DIR}/clients/$(basename "${client}")"
  if [ ! -f "${target}" ]; then
    cp "${client}" "${target}"
  fi
done

# 设置默认启动参数
if [ "$#" -eq 0 ]; then
  set -- openpt --config "${DATA_DIR}/config.toml"
elif [ "${1#-}" != "$1" ]; then
  set -- openpt "$@"
fi

# 修正权限：只 chown 程序需要读写的文件/目录，不递归触碰 torrents 中的用户种子
if [ "$(id -u)" = "0" ]; then
  chown openpt:openpt "${DATA_DIR}"
  chown -R openpt:openpt "${DATA_DIR}/clients"
  chown openpt:openpt "${DATA_DIR}/torrents" 2>/dev/null || true
  chown openpt:openpt "${DATA_DIR}/torrents_archive" 2>/dev/null || true
  chown openpt:openpt "${DATA_DIR}/config.toml" 2>/dev/null || true
  exec su-exec openpt "$@"
fi

exec "$@"
