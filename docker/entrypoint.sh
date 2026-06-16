#!/bin/sh
set -eu

DATA_DIR="${OPENPT_DATA_DIR:-/data}"
APP_DIR="${OPENPT_APP_DIR:-/app}"

mkdir -p "${DATA_DIR}/torrents" "${DATA_DIR}/clients"

if [ ! -f "${DATA_DIR}/config.toml" ]; then
  cp "${APP_DIR}/examples/config.docker.toml" "${DATA_DIR}/config.toml"
  echo "created default config: ${DATA_DIR}/config.toml"
fi

for client in "${APP_DIR}"/clients/*.client; do
  [ -e "${client}" ] || continue
  target="${DATA_DIR}/clients/$(basename "${client}")"
  if [ ! -f "${target}" ]; then
    cp "${client}" "${target}"
  fi
done

if [ "$#" -eq 0 ]; then
  set -- openpt --config "${DATA_DIR}/config.toml"
elif [ "${1#-}" != "$1" ]; then
  set -- openpt "$@"
fi

if [ "$(id -u)" = "0" ]; then
  chown -R openpt:openpt "${DATA_DIR}"
  exec su-exec openpt "$@"
fi

exec "$@"
