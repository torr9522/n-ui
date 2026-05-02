#!/bin/bash
set -euo pipefail

DB_PATH="/etc/x-ui/x-ui.db"
TABLE_FAMILY="inet"
TABLE_NAME="xui_auto_portlimit"
INPUT_CHAIN_NAME="input"
OUTPUT_CHAIN_NAME="output"
DEFAULT_TIMEOUT="${XUI_PORTLIMIT_TIMEOUT:-5m}"
PORT_RELEASE_WAIT_SECONDS="${XUI_PORT_RELEASE_WAIT_SECONDS:-20}"
RESTART_COOLDOWN_SECONDS="${XUI_PORT_RELEASE_RESTART_COOLDOWN_SECONDS:-120}"
STATE_FILE="/var/lib/xui-portlimit/desired.conf"
LOCK_FILE="/run/xui-portlimit-sync.lock"
RESTART_STAMP_FILE="/var/lib/xui-portlimit/last-heal-restart.ts"

command -v nft >/dev/null 2>&1 || { echo "nft command not found"; exit 1; }
command -v sqlite3 >/dev/null 2>&1 || { echo "sqlite3 command not found"; exit 1; }
[ -f "$DB_PATH" ] || { echo "db not found: $DB_PATH"; exit 1; }
mkdir -p /var/lib/xui-portlimit

table_exists() {
  nft list table "$TABLE_FAMILY" "$TABLE_NAME" >/dev/null 2>&1
}

port_is_blocked() {
  local port="$1"
  nft list set "$TABLE_FAMILY" "$TABLE_NAME" blocked_ports 2>/dev/null | grep -Eq "(^|[^0-9])${port}([^0-9]|$)"
}

runtime_maintain() {
  [ -z "${DESIRED:-}" ] && return 0
  while IFS=',' read -r PORT LIMIT TIMEOUT _PRN _PRU _IRN _IRU; do
    [ -z "${PORT:-}" ] && continue
    SETNAME="p_${PORT}"
    if [ "${LIMIT:-0}" -le 0 ]; then
      continue
    fi
    if port_is_blocked "$PORT"; then
      nft flush set "$TABLE_FAMILY" "$TABLE_NAME" "$SETNAME" >/dev/null 2>&1 || true
    fi
  done <<< "$DESIRED"
}

get_enabled_ports() {
  sqlite3 -noheader -batch "$DB_PATH" "select port from inbounds where enable=1 and port between 1 and 65535 order by port;" 2>/dev/null || true
}

get_xray_listen_ports() {
  {
    ss -lntp 2>/dev/null || true
    ss -lnup 2>/dev/null || true
  } | awk '
    /xray-linux/ {
      local_addr=$5
      sub(/^.*\]:/, "", local_addr)
      sub(/^.*:/, "", local_addr)
      if (local_addr ~ /^[0-9]+$/) print local_addr
    }
  ' | sort -n -u
}

find_stale_ports() {
  local enabled xray stale
  enabled="$(get_enabled_ports)"
  xray="$(get_xray_listen_ports)"
  stale="$(comm -23 <(printf "%s\n" "$xray" | sed '/^$/d' | sort -n -u) <(printf "%s\n" "$enabled" | sed '/^$/d' | sort -n -u) || true)"
  printf "%s" "$stale"
}

restart_xui_with_cooldown() {
  local now last
  now="$(date +%s)"
  if [ -f "$RESTART_STAMP_FILE" ]; then
    last="$(cat "$RESTART_STAMP_FILE" 2>/dev/null || echo 0)"
  else
    last=0
  fi
  if [ $((now - last)) -lt "$RESTART_COOLDOWN_SECONDS" ]; then
    echo "heal-skip: cooldown active"
    return 0
  fi
  if command -v systemctl >/dev/null 2>&1; then
    echo "heal-action: restarting x-ui due to stale xray ports"
    systemctl restart x-ui || true
    echo "$now" > "$RESTART_STAMP_FILE"
  fi
}

heal_stale_ports_if_needed() {
  local stale i
  stale="$(find_stale_ports)"
  [ -z "$stale" ] && return 0

  echo "heal-detect: stale xray ports: $(echo "$stale" | tr '\n' ' ')"
  for ((i=0; i<PORT_RELEASE_WAIT_SECONDS; i++)); do
    sleep 1
    stale="$(find_stale_ports)"
    [ -z "$stale" ] && return 0
  done

  restart_xui_with_cooldown
}

if command -v flock >/dev/null 2>&1; then
  exec 200>"$LOCK_FILE"
  if ! flock -w 20 200; then
    echo "busy"
    exit 0
  fi
fi

DESIRED=$(
python3 - <<'PY'
import json
import re
import sqlite3

con = sqlite3.connect('/etc/x-ui/x-ui.db')
cur = con.cursor()

cols = {r[1] for r in cur.execute("pragma table_info(inbounds)").fetchall()}
parts = [
    "port",
    "enable",
    "listen",
    "ip_limit" if "ip_limit" in cols else "0 as ip_limit",
    "ip_timeout" if "ip_timeout" in cols else "5 as ip_timeout",
    "port_rate" if "port_rate" in cols else "'' as port_rate",
    "ip_rate" if "ip_rate" in cols else "'' as ip_rate",
    "settings",
]
rows = cur.execute(
    f"select {','.join(parts)} from inbounds where enable=1 and port between 1 and 65535"
).fetchall()

rate_re = re.compile(r'^\s*(\d+(?:\.\d+)?)\s*(kbit|mbit|gbit|kbps|mbps|gbps|kbyte(?:/s)?|mbyte(?:/s)?|gbyte(?:/s)?)?\s*$', re.I)
unit_factor = {
    'kbit': 1000 / 8,
    'mbit': 1000_000 / 8,
    'gbit': 1000_000_000 / 8,
    'kbps': 1000 / 8,
    'mbps': 1000_000 / 8,
    'gbps': 1000_000_000 / 8,
    'kbyte/s': 1000,
    'mbyte/s': 1000_000,
    'gbyte/s': 1000_000_000,
}

def parse_rate(v):
    if not v:
        return (0, '')
    s = str(v).strip().lower().replace(' ', '')
    m = rate_re.match(s)
    if not m:
        return (0, '')
    num = float(m.group(1))
    unit = (m.group(2) or 'mbit').lower()
    unit = {
        'kbyte': 'kbyte/s',
        'mbyte': 'mbyte/s',
        'gbyte': 'gbyte/s',
    }.get(unit, unit)
    bps = int(num * unit_factor[unit])
    if bps <= 0:
        return (0, '')
    if bps >= 1000:
        return (max(1, bps // 1000), 'kbytes/second')
    return (max(1, bps), 'bytes/second')

merged = {}
for port, enable, listen, ip_limit, ip_timeout, port_rate, ip_rate, settings in rows:
    l = (listen or '').strip().lower()
    if l in ('127.0.0.1', '::1', 'localhost'):
        continue

    try:
        limit = int(ip_limit or 0)
    except Exception:
        limit = 0
    try:
        timeout_m = int(ip_timeout or 5)
    except Exception:
        timeout_m = 5
    if timeout_m <= 0:
        timeout_m = 5

    if settings:
        try:
            s = json.loads(settings)
            for c in s.get('clients', []):
                if not isinstance(c, dict):
                    continue
                if c.get('enable', True) is False:
                    continue
                li = int(c.get('limitIp', 0) or 0)
                if li > limit:
                    limit = li
        except Exception:
            pass

    pr_num, pr_unit = parse_rate(port_rate)
    ir_num, ir_unit = parse_rate(ip_rate)

    if limit <= 0 and pr_num <= 0 and ir_num <= 0:
        continue

    p = int(port)
    old = merged.get(p)
    if old is None:
        merged[p] = [max(0, int(limit)), timeout_m, pr_num, pr_unit, ir_num, ir_unit]
    else:
        old[0] = max(old[0], int(limit))
        old[1] = max(old[1], timeout_m)
        if pr_num > 0 and (old[2] <= 0 or pr_num < old[2]):
            old[2], old[3] = pr_num, pr_unit
        if ir_num > 0 and (old[4] <= 0 or ir_num < old[4]):
            old[4], old[5] = ir_num, ir_unit

for p in sorted(merged):
    l, t, prn, pru, irn, iru = merged[p]
    print(f"{p},{l},{t}m,{prn},{pru},{irn},{iru}")
PY
)

OLD=""
[ -f "$STATE_FILE" ] && OLD="$(cat "$STATE_FILE")"
if [ "${XUI_PORTLIMIT_FORCE_REBUILD:-0}" != "1" ] && [ "$DESIRED" = "$OLD" ]; then
  if ! table_exists; then
    XUI_PORTLIMIT_FORCE_REBUILD=1
  else
    runtime_maintain
    echo "unchanged"
    exit 0
  fi
fi

nft delete table "$TABLE_FAMILY" "$TABLE_NAME" 2>/dev/null || true
nft add table "$TABLE_FAMILY" "$TABLE_NAME"
nft "add chain $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME { type filter hook input priority 0; policy accept; }"
nft "add chain $TABLE_FAMILY $TABLE_NAME $OUTPUT_CHAIN_NAME { type filter hook output priority 0; policy accept; }"
nft "add set $TABLE_FAMILY $TABLE_NAME blocked_ports { type inet_service; flags timeout,dynamic; timeout $DEFAULT_TIMEOUT; }"
nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME tcp dport @blocked_ports counter drop"
nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME udp dport @blocked_ports counter drop"
nft "add rule $TABLE_FAMILY $TABLE_NAME $OUTPUT_CHAIN_NAME tcp sport @blocked_ports counter drop"
nft "add rule $TABLE_FAMILY $TABLE_NAME $OUTPUT_CHAIN_NAME udp sport @blocked_ports counter drop"

if [ -n "$DESIRED" ]; then
  while IFS=',' read -r PORT LIMIT TIMEOUT PRNUM PRUNIT IRNUM IRUNIT; do
    [ -z "${PORT:-}" ] && continue

    if [ "${PRNUM:-0}" -gt 0 ] && [ -n "${PRUNIT:-}" ]; then
      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME tcp dport $PORT limit rate over $PRNUM $PRUNIT counter drop"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME udp dport $PORT limit rate over $PRNUM $PRUNIT counter drop"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $OUTPUT_CHAIN_NAME tcp sport $PORT limit rate over $PRNUM $PRUNIT counter drop"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $OUTPUT_CHAIN_NAME udp sport $PORT limit rate over $PRNUM $PRUNIT counter drop"
    fi

    if [ "${IRNUM:-0}" -gt 0 ] && [ -n "${IRUNIT:-}" ]; then
      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME tcp dport $PORT meter m_tcp_ipr_in_${PORT} { ip saddr limit rate over $IRNUM $IRUNIT } counter drop"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME udp dport $PORT meter m_udp_ipr_in_${PORT} { ip saddr limit rate over $IRNUM $IRUNIT } counter drop"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $OUTPUT_CHAIN_NAME tcp sport $PORT meter m_tcp_ipr_out_${PORT} { ip daddr limit rate over $IRNUM $IRUNIT } counter drop"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $OUTPUT_CHAIN_NAME udp sport $PORT meter m_udp_ipr_out_${PORT} { ip daddr limit rate over $IRNUM $IRUNIT } counter drop"
    fi

    if [ "${LIMIT:-0}" -gt 0 ]; then
      SETNAME="p_${PORT}"
      nft "add set $TABLE_FAMILY $TABLE_NAME $SETNAME { type ipv4_addr; flags timeout,dynamic; timeout $TIMEOUT; size $LIMIT; }"

      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME tcp dport $PORT ip saddr @$SETNAME accept"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME udp dport $PORT ip saddr @$SETNAME accept"

      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME tcp dport $PORT add @$SETNAME { ip saddr timeout $TIMEOUT } accept"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME udp dport $PORT add @$SETNAME { ip saddr timeout $TIMEOUT } accept"

      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME tcp dport $PORT add @blocked_ports { tcp dport timeout $TIMEOUT } counter drop"
      nft "add rule $TABLE_FAMILY $TABLE_NAME $INPUT_CHAIN_NAME udp dport $PORT add @blocked_ports { udp dport timeout $TIMEOUT } counter drop"
    fi
  done <<< "$DESIRED"
fi

printf '%s' "$DESIRED" > "$STATE_FILE"
runtime_maintain

heal_stale_ports_if_needed

echo "applied"
if [ -z "$DESIRED" ]; then
  echo "managed_ports=none"
else
  echo "managed_ports=$(echo "$DESIRED" | tr '\n' ',' | sed 's/,$//')"
fi
