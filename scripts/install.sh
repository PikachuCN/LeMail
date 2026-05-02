#!/usr/bin/env bash
set -euo pipefail

REPO="${LEMAIL_REPO:-PikachuCN/LeMail}"
VERSION="${LEMAIL_VERSION:-latest}"
INSTALL_DIR="${LEMAIL_INSTALL_DIR:-/opt/lemail}"
CONFIG_DIR="${LEMAIL_CONFIG_DIR:-/etc/lemail}"
SERVICE_NAME="${LEMAIL_SERVICE_NAME:-lemail}"
RUN_USER="${LEMAIL_USER:-lemail}"
HTTP_ADDR="${LEMAIL_HTTP_ADDR:-0.0.0.0:3000}"
SMTP_ADDR="${LEMAIL_SMTP_ADDR:-0.0.0.0:25}"
DOMAIN="${LEMAIL_DOMAIN:-}"
OPEN_FIREWALL="${LEMAIL_OPEN_FIREWALL:-1}"

info() { printf '\033[1;34m[LeMail]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[LeMail]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[LeMail]\033[0m %s\n' "$*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || fail "缺少命令：$1，请先安装后重试。"; }

if [ "$(id -u)" -ne 0 ]; then
  fail "请使用 root 执行，例如：curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh -o /tmp/lemail-install.sh && sudo bash /tmp/lemail-install.sh"
fi

need_cmd curl
need_cmd unzip
need_cmd systemctl
need_cmd sed
need_cmd uname

case "$(uname -s)" in
  Linux) os="linux" ;;
  *) fail "当前脚本只支持 Linux systemd 系统。" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) fail "不支持的 CPU 架构：$(uname -m)" ;;
esac

if [ "$VERSION" = "latest" ]; then
  info "查询最新 Release 版本..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  [ -n "$VERSION" ] || fail "无法获取最新 Release 版本。"
fi

asset="lemail_${VERSION}_${os}_${arch}.zip"
url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

info "下载 ${asset}..."
curl -fL --retry 3 --retry-delay 2 -o "$tmp/$asset" "$url"
unzip -q -o "$tmp/$asset" -d "$tmp"
pkg_dir="$(find "$tmp" -maxdepth 1 -type d -name "lemail_${VERSION}_${os}_${arch}" | head -n 1)"
[ -n "$pkg_dir" ] || fail "解压后未找到 LeMail 目录。"

info "创建运行目录..."
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR"
if ! id "$RUN_USER" >/dev/null 2>&1; then
  useradd --system --home "$INSTALL_DIR" --shell /usr/sbin/nologin "$RUN_USER"
fi

install -m 0755 "$pkg_dir/lemail" "$INSTALL_DIR/lemail"
if [ ! -f "$CONFIG_DIR/config.json" ]; then
  install -m 0640 "$pkg_dir/config/config.example.json" "$CONFIG_DIR/config.json"
  sed -i "s#\"httpAddr\": \"0.0.0.0:3000\"#\"httpAddr\": \"${HTTP_ADDR}\"#" "$CONFIG_DIR/config.json"
  sed -i "s#\"addr\": \"0.0.0.0:2525\"#\"addr\": \"${SMTP_ADDR}\"#" "$CONFIG_DIR/config.json"
  if [ -n "$DOMAIN" ]; then
    sed -i "s#\"domains\": \[\"localhost\"\]#\"domains\": [\"${DOMAIN}\"]#" "$CONFIG_DIR/config.json"
  fi
else
  warn "检测到已有 $CONFIG_DIR/config.json，保留原配置。"
fi

chown -R "$RUN_USER:$RUN_USER" "$INSTALL_DIR" "$CONFIG_DIR"
chmod 0750 "$INSTALL_DIR" "$CONFIG_DIR"
chmod 0640 "$CONFIG_DIR/config.json"

info "写入 systemd 服务..."
cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<SERVICE
[Unit]
Description=LeMail temporary mailbox service
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${RUN_USER}
Group=${RUN_USER}
WorkingDirectory=${INSTALL_DIR}
Environment=CONFIG_PATH=${CONFIG_DIR}/config.json
ExecStart=${INSTALL_DIR}/lemail
Restart=always
RestartSec=3
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable --now "$SERVICE_NAME"

if [ "$OPEN_FIREWALL" = "1" ]; then
  if command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld; then
    info "firewalld 放行 25/tcp 和 3000/tcp..."
    firewall-cmd --add-port=25/tcp --permanent >/dev/null || true
    firewall-cmd --add-port=3000/tcp --permanent >/dev/null || true
    firewall-cmd --reload >/dev/null || true
  fi
  if command -v ufw >/dev/null 2>&1 && ufw status | grep -qi active; then
    info "ufw 放行 25/tcp 和 3000/tcp..."
    ufw allow 25/tcp >/dev/null || true
    ufw allow 3000/tcp >/dev/null || true
  fi
fi

info "部署完成。"
info "服务状态：systemctl status ${SERVICE_NAME} --no-pager"
info "配置文件：${CONFIG_DIR}/config.json"
info "管理入口：http://服务器IP:3000"
info "如果使用宝塔/Nginx 反向代理，请代理到：http://127.0.0.1:3000"
info "公网收信请确保 DNS MX 指向本机，并且 25/tcp 未被云厂商屏蔽。"
