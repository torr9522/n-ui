#!/usr/bin/env bash

export LANG=en_US.UTF-8
SCRIPT_DIR="$(cd -P "$(dirname "${BASH_SOURCE[0]:-$0}")" >/dev/null 2>&1 && pwd)"
XUI_RAW_BASE="${XUI_RAW_BASE:-https://raw.githubusercontent.com/torr9522/n-ui/n-ui}"
if [[ -f "${SCRIPT_DIR}/install.sh" ]]; then
    exec bash "${SCRIPT_DIR}/install.sh"
fi
exec bash <(curl -Ls "${XUI_RAW_BASE}/install.sh")
