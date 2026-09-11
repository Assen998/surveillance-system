#!/bin/bash
#
# Surveillance System 一键安装脚本（Linux，systemd + 单文件二进制）
#
# 用法:
#   sudo bash deployments/deploy.sh [本地二进制路径]
#     - 不带参数：自动从 GitHub Releases 下载当前平台最新资产
#     - 带参数  ：安装指定本地二进制
#   sudo bash deployments/deploy.sh --uninstall
#
# 安装位置:
#   /opt/surveillance/surveillance-server   程序
#   /opt/surveillance/configs/config.yaml   配置（首次安装自动生成）
#   /opt/surveillance/data|recordings|logs  数据目录
#   /etc/systemd/system/surveillance.service
#
# 前置依赖: ffmpeg (拉流/录像/预览必需)
#   Debian/Armbian: sudo apt update && sudo apt install -y ffmpeg
#   RHEL/Fedora:    sudo dnf install -y ffmpeg
#
set -e

REPO="Assen998/surveillance-system"
INSTALL_DIR="/opt/surveillance"
SERVICE_NAME="surveillance"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
log_info()    { echo -e "${YELLOW}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1"; }

[[ $EUID -ne 0 ]] && { log_error "请用 root 运行: sudo bash $0"; exit 1; }

# ---------- 卸载 ----------
if [[ "${1:-}" == "--uninstall" ]]; then
    systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    systemctl disable "$SERVICE_NAME" 2>/dev/null || true
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    log_info "已停止并移除 systemd 服务。数据保留在 $INSTALL_DIR（如需彻底删除请手动 rm -rf $INSTALL_DIR）"
    exit 0
fi

# ---------- 依赖检查 ----------
command -v ffmpeg >/dev/null 2>&1 || {
    log_error "未找到 ffmpeg（拉流/录像/预览必需）"
    log_error "Debian/Armbian: sudo apt update && sudo apt install -y ffmpeg"
    log_error "RHEL/Fedora:    sudo dnf install -y ffmpeg"
    exit 1
}
command -v curl >/dev/null 2>&1 || { log_error "未找到 curl"; exit 1; }

# ---------- 获取二进制 ----------
BINARY="${1:-}"
if [[ -n "$BINARY" ]]; then
    [[ -f "$BINARY" ]] || { log_error "文件不存在: $BINARY"; exit 1; }
    log_info "使用本地二进制: $BINARY"
else
    UNAME_S=$(uname -s | tr '[:upper:]' '[:lower:]')
    UNAME_M=$(uname -m)
    case "$UNAME_M" in
        x86_64|amd64)  GOOS_ARCH="amd64" ;;
        aarch64|arm64) GOOS_ARCH="arm64" ;;
        armv7l)        GOOS_ARCH="arm-armv7" ;;
        *) log_error "不支持的架构: $UNAME_M"; exit 1 ;;
    esac
    if [[ "$UNAME_S" != "linux" ]]; then
        log_error "本脚本仅支持 Linux（其他平台请从 Releases 手动下载对应资产）"
        exit 1
    fi
    # 取最新 release 的资产下载地址（支持 http_proxy 代理）
    log_info "查询最新版本: https://github.com/$REPO/releases/latest"
    VER=$(curl -sL --max-time 30 "https://api.github.com/repos/$REPO/releases/latest" \
        | python3 -c "import sys,json; print(json.load(sys.stdin).get('tag_name',''))" 2>/dev/null || true)
    if [[ -z "$VER" ]]; then
        log_error "获取最新版本失败（如需代理请设置 http_proxy=https://192.168.1.5:7890 后重试）"
        exit 1
    fi
    VER_NUM="${VER#v}"
    ASSET="surveillance-system-${VER_NUM}-linux-${GOOS_ARCH}.tar.gz"
    URL="https://github.com/$REPO/releases/download/$VER/$ASSET"
    TMP_DIR="/tmp/surveillance-install.$$"
    mkdir -p "$TMP_DIR"
    log_info "下载 $VER 的 $ASSET ..."
    curl -L --fail --progress-bar --max-time 600 -o "$TMP_DIR/pkg.tar.gz" "$URL" || {
        rm -rf "$TMP_DIR"
        log_error "下载失败: $URL"
        exit 1
    }
    tar xzf "$TMP_DIR/pkg.tar.gz" -C "$TMP_DIR" || { rm -rf "$TMP_DIR"; log_error "解压失败"; exit 1; }
    PKG_DIR=$(find "$TMP_DIR" -maxdepth 1 -type d -name "surveillance-system-linux-*" | head -1)
    if [[ ! -f "$PKG_DIR/surveillance-server" ]]; then
        rm -rf "$TMP_DIR"
        log_error "资产包结构异常，未找到 surveillance-server 可执行文件"
        exit 1
    fi
    # 资产包内附带 config.yaml：无源码时优先使用
    ASSET_CONFIG="$PKG_DIR/config.yaml"
    BINARY="$PKG_DIR/surveillance-server"
fi

# ---------- 安装 ----------
log_info "安装到 $INSTALL_DIR ..."
mkdir -p "$INSTALL_DIR/configs" "$INSTALL_DIR/data" "$INSTALL_DIR/recordings" "$INSTALL_DIR/logs"

if [[ -f "$INSTALL_DIR/configs/config.yaml" ]]; then
    log_info "检测到已有配置，保留原 config.yaml（不覆盖）"
else
    # 配置来源优先级：源码 configs/ > 资产包内 config.yaml > 内置默认值
    REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
    if [[ -f "$REPO_ROOT/configs/config.yaml" ]]; then
        cp "$REPO_ROOT/configs/config.yaml" "$INSTALL_DIR/configs/config.yaml"
    elif [[ -n "${ASSET_CONFIG:-}" && -f "$ASSET_CONFIG" ]]; then
        cp "$ASSET_CONFIG" "$INSTALL_DIR/configs/config.yaml"
    else
        cat > "$INSTALL_DIR/configs/config.yaml" <<'EOF'
server:
    host: 0.0.0.0
    http_port: 8080
    ws_port: 8081
    mode: release
database:
    type: sqlite
    sqlite:
        path: /opt/surveillance/data/surveillance.db
storage:
    local:
        enabled: true
        root_path: /opt/surveillance/recordings
        segment_duration: 180
        max_days: 7
        max_storage_gb: 0
        cleanup_interval: 3600
    minio:
        enabled: false
    webdav:
        enabled: false
camera:
    discovery_timeout: 10
    stream_timeout: 30
    reconnect_interval: 5
    max_reconnect: 10
    preview_stream: main
alert:
    enabled: true
    channels:
        webhook:
            enabled: false
logging:
    level: info
    format: json
    output: /opt/surveillance/logs/surveillance.log
    max_size: 100
    max_backups: 30
    max_age: 30
    compress: true
update:
    proxy: ""
EOF
    fi
    log_info "已生成默认配置 $INSTALL_DIR/configs/config.yaml"
fi

install -m 755 "$BINARY" "$INSTALL_DIR/surveillance-server"
[[ -n "${TMP_DIR:-}" ]] && rm -rf "$TMP_DIR"

# 修正旧安装的绝对路径（sqlite/录像/日志）
sed -i -e 's|path: ./data/surveillance.db|path: /opt/surveillance/data/surveillance.db|' \
       -e 's|root_path: ./recordings|root_path: /opt/surveillance/recordings|' \
       -e 's|output: ./logs/surveillance.log|output: /opt/surveillance/logs/surveillance.log|' \
       "$INSTALL_DIR/configs/config.yaml" || true

# ---------- systemd ----------
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Surveillance Video System
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/surveillance-server
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null 2>&1
systemctl restart "$SERVICE_NAME"

sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
    log_success "安装完成，服务已启动"
    log_success "Web 控制台: http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo 127.0.0.1):8080"
    log_info "常用命令:"
    log_info "  查看状态   systemctl status $SERVICE_NAME"
    log_info "  查看日志   journalctl -u $SERVICE_NAME -f   或 tail -f $INSTALL_DIR/logs/surveillance.log"
    log_info "  卸载       sudo bash $0 --uninstall"
else
    log_error "服务启动失败，请查看日志: journalctl -u $SERVICE_NAME -n 50"
    exit 1
fi
