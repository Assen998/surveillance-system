#!/bin/bash
#
# Surveillance System one-click installer (Linux, systemd + single binary)
# 监控录像系统一键安装脚本（Linux，systemd + 单文件二进制）
#
# Usage / 用法:
#   sudo bash deployments/deploy.sh [local binary path]
#   sudo bash deployments/deploy.sh [本地二进制路径]
#     - no arg: auto-download latest asset from GitHub Releases
#     - with arg: install the given local binary
#   sudo bash deployments/deploy.sh --uninstall
#
# Install locations / 安装位置:
#   /opt/surveillance/surveillance-server
#   /opt/surveillance/configs/config.yaml
#   /opt/surveillance/data|recordings|logs
#   /etc/systemd/system/surveillance.service
#
# Dependency / 前置依赖: ffmpeg (required for streaming/recording/preview 拉流/录像/预览必需)
#   Debian/Armbian: sudo apt update && sudo apt install -y ffmpeg
#   RHEL/Fedora:    sudo dnf install -y ffmpeg
#
# Language / 语言:
#   DEPLOY_LANG=zh|en overrides; interactive runs prompt before starting
#
set -e

REPO="Assen998/surveillance-system"
INSTALL_DIR="/opt/surveillance"
SERVICE_NAME="surveillance"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# ---------- i18n ----------
T() {
    local key="$1"; shift
    local text
    case "$LANG_CODE" in
        en)
            case "$key" in
                need_root)      text="Please run as root: sudo bash $0" ;;
                uninstall_done) text="Service stopped and removed. Data kept in $INSTALL_DIR (rm -rf it manually to delete completely)" ;;
                no_ffmpeg)      text="ffmpeg not found (required for streaming/recording/preview)" ;;
                ffmpeg_deb)     text="Debian/Armbian: sudo apt update && sudo apt install -y ffmpeg" ;;
                ffmpeg_rhel)    text="RHEL/Fedora:    sudo dnf install -y ffmpeg" ;;
                no_curl)        text="curl not found" ;;
                file_not_found) text="File not found: %s" ;;
                use_local)      text="Using local binary: %s" ;;
                unsupported_arch) text="Unsupported architecture: %s" ;;
                linux_only)     text="This script only supports Linux (download the matching release asset manually on other platforms)" ;;
                query_latest)   text="Querying latest release: https://github.com/%s/releases/latest" ;;
                get_ver_fail)   text="Failed to get latest version (if a proxy is needed, set http_proxy=... and retry)" ;;
                download)       text="Downloading %s asset %s ..." ;;
                download_fail)  text="Download failed: %s" ;;
                extract_fail)   text="Extraction failed" ;;
                pkg_bad)        text="Asset package structure invalid, surveillance-server binary not found" ;;
                install_to)     text="Installing to %s ..." ;;
                keep_cfg)       text="Existing config found, keeping current config.yaml (not overwritten)" ;;
                gen_cfg)        text="Generated default config %s" ;;
                port_http)      text="HTTP port [8080]: " ;;
                port_ws)        text="WebSocket port [8081]: " ;;
                port_invalid)   text="Invalid port: %s (must be a number between 1 and 65535)" ;;
                port_in_use)    text="Port %s is already in use on this machine, please choose another" ;;
                port_same)      text="HTTP and WebSocket ports must be different" ;;
                ports_chosen)   text="Using HTTP port %s, WebSocket port %s" ;;
                start_fail)     text="Service failed to start, check logs: journalctl -u $SERVICE_NAME -n 50" ;;
                done)           text="Installation complete, service started" ;;
                web_url)        text="Web console: http://%s:%s" ;;
                hints)          text="Common commands:" ;;
                hint_status)    text="  status:     systemctl status $SERVICE_NAME" ;;
                hint_logs)      text="  logs:       journalctl -u $SERVICE_NAME -f   (or tail -f $INSTALL_DIR/logs/surveillance.log)" ;;
                hint_uninstall) text="  uninstall:  sudo bash <this-script> --uninstall" ;;
            esac ;;
        *)
            case "$key" in
                need_root)      text="请用 root 运行: sudo bash $0" ;;
                uninstall_done) text="已停止并移除 systemd 服务。数据保留在 $INSTALL_DIR（如需彻底删除请手动 rm -rf $INSTALL_DIR）" ;;
                no_ffmpeg)      text="未找到 ffmpeg（拉流/录像/预览必需）" ;;
                ffmpeg_deb)     text="Debian/Armbian: sudo apt update && sudo apt install -y ffmpeg" ;;
                ffmpeg_rhel)    text="RHEL/Fedora:    sudo dnf install -y ffmpeg" ;;
                no_curl)        text="未找到 curl" ;;
                file_not_found) text="文件不存在: %s" ;;
                use_local)      text="使用本地二进制: %s" ;;
                unsupported_arch) text="不支持的架构: %s" ;;
                linux_only)     text="本脚本仅支持 Linux（其他平台请从 Releases 手动下载对应资产）" ;;
                query_latest)   text="查询最新版本: https://github.com/%s/releases/latest" ;;
                get_ver_fail)   text="获取最新版本失败（如需代理请设置 http_proxy=... 后重试）" ;;
                download)       text="下载 %s 的 %s ..." ;;
                download_fail)  text="下载失败: %s" ;;
                extract_fail)   text="解压失败" ;;
                pkg_bad)        text="资产包结构异常，未找到 surveillance-server 可执行文件" ;;
                install_to)     text="安装到 %s ..." ;;
                keep_cfg)       text="检测到已有配置，保留原 config.yaml（不覆盖）" ;;
                gen_cfg)        text="已生成默认配置 %s" ;;
                port_http)      text="HTTP 端口号 [8080]: " ;;
                port_ws)        text="WebSocket 端口号 [8081]: " ;;
                port_invalid)   text="无效端口: %s（应为 1-65535 的数字）" ;;
                port_in_use)    text="端口 %s 已被本机占用，请更换" ;;
                port_same)      text="HTTP 与 WebSocket 端口不能相同" ;;
                ports_chosen)   text="使用 HTTP 端口 %s，WebSocket 端口 %s" ;;
                start_fail)     text="服务启动失败，请查看日志: journalctl -u $SERVICE_NAME -n 50" ;;
                done)           text="安装完成，服务已启动" ;;
                web_url)        text="Web 控制台: http://%s:%s" ;;
                hints)          text="常用命令:" ;;
                hint_status)    text="  查看状态   systemctl status $SERVICE_NAME" ;;
                hint_logs)      text="  查看日志   journalctl -u $SERVICE_NAME -f   或 tail -f $INSTALL_DIR/logs/surveillance.log" ;;
                hint_uninstall) text="  卸载       sudo bash <本脚本> --uninstall" ;;
            esac ;;
    esac
    if (( $# > 0 )); then printf "$text" "$@"; else printf "%s" "$text"; fi
}

detect_lang() {
    local loc="${LC_ALL:-${LC_MESSAGES:-${LANG:-}}}"
    case "$loc" in
        *zh*) echo "zh" ;;
        *)    echo "en" ;;
    esac
}

# ---------- 启动前选择语言 / Language selection ----------
if [[ -n "${DEPLOY_LANG:-}" ]]; then
    LANG_CODE="$DEPLOY_LANG"
elif [[ ! -t 0 ]]; then
    LANG_CODE=$(detect_lang)
else
    echo ""
    echo "  Surveillance System Installer / 监控录像系统安装程序"
    echo "  --------------------------------------------------"
    echo "    1) 简体中文"
    echo "    2) English"
    echo -n "  选择语言 / Select language [1]: "
    read -r choice || choice="1"
    case "$choice" in
        2|en|EN|English|english) LANG_CODE="en" ;;
        *) LANG_CODE="zh" ;;
    esac
fi

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
log_info()    { echo -e "${YELLOW}[INFO]${NC} $(T "$@")"; }
log_success() { echo -e "${GREEN}[OK]${NC} $(T "$@")"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $(T "$@")"; }

[[ $EUID -ne 0 ]] && { log_error need_root; exit 1; }

# ---------- 卸载 / uninstall ----------
if [[ "${1:-}" == "--uninstall" ]]; then
    systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    systemctl disable "$SERVICE_NAME" 2>/dev/null || true
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    log_info uninstall_done
    exit 0
fi

# ---------- 依赖检查 / dependency check ----------
command -v ffmpeg >/dev/null 2>&1 || {
    log_error no_ffmpeg
    log_error ffmpeg_deb
    log_error ffmpeg_rhel
    exit 1
}
command -v curl >/dev/null 2>&1 || { log_error no_curl; exit 1; }

# ---------- 获取二进制 / obtain binary ----------
BINARY="${1:-}"
if [[ -n "$BINARY" ]]; then
    [[ -f "$BINARY" ]] || { log_error file_not_found "$BINARY"; exit 1; }
    log_info use_local "$BINARY"
else
    UNAME_S=$(uname -s | tr '[:upper:]' '[:lower:]')
    UNAME_M=$(uname -m)
    case "$UNAME_M" in
        x86_64|amd64)  GOOS_ARCH="amd64" ;;
        aarch64|arm64) GOOS_ARCH="arm64" ;;
        armv7l)        GOOS_ARCH="arm-armv7" ;;
        *) log_error unsupported_arch "$UNAME_M"; exit 1 ;;
    esac
    if [[ "$UNAME_S" != "linux" ]]; then
        log_error linux_only
        exit 1
    fi
    log_info query_latest "$REPO"
    VER=$(curl -sL --max-time 30 "https://api.github.com/repos/$REPO/releases/latest" \
        | python3 -c "import sys,json; print(json.load(sys.stdin).get('tag_name',''))" 2>/dev/null || true)
    if [[ -z "$VER" ]]; then
        log_error get_ver_fail
        exit 1
    fi
    VER_NUM="${VER#v}"
    ASSET="surveillance-system-${VER_NUM}-linux-${GOOS_ARCH}.tar.gz"
    URL="https://github.com/$REPO/releases/download/$VER/$ASSET"
    TMP_DIR="/tmp/surveillance-install.$$"
    mkdir -p "$TMP_DIR"
    log_info download "$VER" "$ASSET"
    curl -L --fail --progress-bar --max-time 600 -o "$TMP_DIR/pkg.tar.gz" "$URL" || {
        rm -rf "$TMP_DIR"
        log_error download_fail "$URL"
        exit 1
    }
    tar xzf "$TMP_DIR/pkg.tar.gz" -C "$TMP_DIR" || { rm -rf "$TMP_DIR"; log_error extract_fail; exit 1; }
    PKG_DIR=$(find "$TMP_DIR" -maxdepth 1 -type d -name "surveillance-system-linux-*" | head -1)
    if [[ ! -f "$PKG_DIR/surveillance-server" ]]; then
        rm -rf "$TMP_DIR"
        log_error pkg_bad
        exit 1
    fi
    ASSET_CONFIG="$PKG_DIR/config.yaml"
    BINARY="$PKG_DIR/surveillance-server"
fi

# ---------- 端口选择（仅首次安装生成配置时）----------
# 默认 8080/8081；交互运行会询问，非交互可用 SURVEILLANCE_HTTP_PORT / SURVEILLANCE_WS_PORT 覆盖
port_format_ok() { [[ "$1" =~ ^[0-9]+$ ]] && (( 10#$1 >= 1 && 10#$1 <= 65535 )); }
port_in_use()    { command -v ss >/dev/null 2>&1 && ss -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${1}$"; }

if [[ ! -f "$INSTALL_DIR/configs/config.yaml" ]]; then
    HTTP_PORT="${SURVEILLANCE_HTTP_PORT:-8080}"
    WS_PORT="${SURVEILLANCE_WS_PORT:-8081}"
    if [[ -t 0 ]]; then
        for _attempt in 1 2 3; do
            echo -n "  $(T port_http)"
            read -r INPUT || INPUT=""
            [[ -n "$INPUT" ]] && HTTP_PORT="$INPUT"
            echo -n "  $(T port_ws)"
            read -r INPUT || INPUT=""
            [[ -n "$INPUT" ]] && WS_PORT="$INPUT"
            ERRS=()
            port_format_ok "$HTTP_PORT" || ERRS+=("$(T port_invalid "$HTTP_PORT")")
            port_format_ok "$WS_PORT"   || ERRS+=("$(T port_invalid "$WS_PORT")")
            port_format_ok "$HTTP_PORT" && port_in_use "$HTTP_PORT" && ERRS+=("$(T port_in_use "$HTTP_PORT")")
            port_format_ok "$WS_PORT"   && port_in_use "$WS_PORT"   && ERRS+=("$(T port_in_use "$WS_PORT")")
            [[ "$HTTP_PORT" == "$WS_PORT" ]] && ERRS+=("$(T port_same)")
            if (( ${#ERRS[@]} == 0 )); then break; fi
            for e in "${ERRS[@]}"; do log_error "$e"; done
            if [[ "$_attempt" -eq 3 ]]; then exit 1; fi
        done
    else
        port_format_ok "$HTTP_PORT" || { log_error port_invalid "$HTTP_PORT"; exit 1; }
        port_format_ok "$WS_PORT"   || { log_error port_invalid "$WS_PORT"; exit 1; }
        [[ "$HTTP_PORT" == "$WS_PORT" ]] && { log_error port_same; exit 1; }
    fi
    log_info ports_chosen "$HTTP_PORT" "$WS_PORT"
fi

# ---------- 安装 / install ----------
log_info install_to "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR/configs" "$INSTALL_DIR/data" "$INSTALL_DIR/recordings" "$INSTALL_DIR/logs"

if [[ -f "$INSTALL_DIR/configs/config.yaml" ]]; then
    log_info keep_cfg
else
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
    # 写入用户选择的端口
    sed -i -E -e "s|^([[:space:]]*http_port:)[[:space:]]*[0-9]+|\1 ${HTTP_PORT}|" \
              -e "s|^([[:space:]]*ws_port:)[[:space:]]*[0-9]+|\1 ${WS_PORT}|" \
              "$INSTALL_DIR/configs/config.yaml"
    log_info gen_cfg "$INSTALL_DIR/configs/config.yaml"
fi

install -m 755 "$BINARY" "$INSTALL_DIR/surveillance-server"
[[ -n "${TMP_DIR:-}" ]] && rm -rf "$TMP_DIR"

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
    # 已有配置时从 config.yaml 读取实际端口
    if [[ -z "${HTTP_PORT:-}" && -f "$INSTALL_DIR/configs/config.yaml" ]]; then
        HTTP_PORT=$(grep -E '^[[:space:]]*http_port:' "$INSTALL_DIR/configs/config.yaml" | grep -oE '[0-9]+' | head -1)
    fi
    log_success done
    log_success web_url "$(hostname -I 2>/dev/null | awk '{print $1}' || echo 127.0.0.1)" "${HTTP_PORT:-8080}"
    log_info hints
    log_info hint_status
    log_info hint_logs
    log_info hint_uninstall
else
    log_error start_fail
    exit 1
fi
