#!/bin/bash
# 监控录像系统部署脚本
# 用法: ./deploy.sh [start|stop|restart|logs|status|update|backup]

set -e

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="$PROJECT_DIR/deployments/docker-compose.yml"
ENV_FILE="$PROJECT_DIR/deployments/.env"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# 检查依赖
check_deps() {
    command -v docker >/dev/null 2>&1 || { log_error "Docker 未安装"; exit 1; }
    command -v docker-compose >/dev/null 2>&1 || { log_error "Docker Compose 未安装"; exit 1; }
}

# 生成 .env 文件
gen_env() {
    if [[ ! -f "$ENV_FILE" ]]; then
        log_info "生成环境配置文件..."
        cat > "$ENV_FILE" <<EOF
# 监控录像系统环境配置
# 生成时间: $(date)

# 数据库
POSTGRES_USER=surveillance
POSTGRES_PASSWORD=surveillance123
POSTGRES_DB=surveillance

# Redis
REDIS_PASSWORD=

# MinIO
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin123

# 应用
SURVEILLANCE_VERSION=1.0.0
TZ=Asia/Shanghai

# SSL (生产环境请替换为真实证书)
SSL_CERT_PATH=./ssl/cert.pem
SSL_KEY_PATH=./ssl/key.pem
EOF
        log_success "环境配置文件已生成: $ENV_FILE"
        log_warn "请根据实际情况修改密码等敏感配置"
    fi
}

# 创建必要目录
create_dirs() {
    log_info "创建数据目录..."
    mkdir -p "$PROJECT_DIR/data"/{recordings,logs,models,ssl}
    mkdir -p "$PROJECT_DIR/deployments/ssl"
    
    # 设置权限
    chmod 755 "$PROJECT_DIR/data"/{recordings,logs,models}
    
    # 生成自签名证书 (开发用)
    if [[ ! -f "$PROJECT_DIR/deployments/ssl/cert.pem" ]]; then
        log_info "生成自签名 SSL 证书..."
        openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
            -keyout "$PROJECT_DIR/deployments/ssl/key.pem" \
            -out "$PROJECT_DIR/deployments/ssl/cert.pem" \
            -subj "/C=CN/ST=State/L=City/O=Organization/CN=localhost" \
            2>/dev/null
        log_success "SSL 证书已生成"
    fi
}

# 启动服务
cmd_start() {
    check_deps
    gen_env
    create_dirs
    
    log_info "启动监控录像系统..."
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d
    
    log_success "服务启动完成"
    log_info "访问地址:"
    echo "  - Web 界面: https://localhost (或 http://localhost:8080)"
    echo "  - API 文档: http://localhost:8080/api/v1"
    echo "  - MinIO 控制台: http://localhost:9001"
    echo "  - Grafana: http://localhost:3000 (admin/admin123)"
    echo "  - Prometheus: http://localhost:9090"
}

# 停止服务
cmd_stop() {
    log_info "停止监控录像系统..."
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" down
    log_success "服务已停止"
}

# 重启服务
cmd_restart() {
    cmd_stop
    sleep 2
    cmd_start
}

# 查看日志
cmd_logs() {
    local service=${2:-}
    if [[ -n "$service" ]]; then
        docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs -f "$service"
    else
        docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs -f
    fi
}

# 查看状态
cmd_status() {
    log_info "服务状态:"
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" ps
    
    echo ""
    log_info "资源使用:"
    docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}"
}

# 更新服务
cmd_update() {
    log_info "更新监控录像系统..."
    cd "$PROJECT_DIR"
    git pull
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" build --no-cache
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d
    log_success "更新完成"
}

# 备份数据
cmd_backup() {
    local backup_dir="$PROJECT_DIR/backups/$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    
    log_info "备份数据到 $backup_dir ..."
    
    # 备份数据库
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T postgres \
        pg_dump -U surveillance surveillance > "$backup_dir/database.sql"
    
    # 备份配置
    cp -r "$PROJECT_DIR/configs" "$backup_dir/"
    cp "$ENV_FILE" "$backup_dir/.env"
    
    # 备份录像索引 (不包含大视频文件)
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T surveillance \
        find /app/recordings -name "*.idx" -o -name "*.json" | tar -czf "$backup_dir/recordings_index.tar.gz" -T -
    
    log_success "备份完成: $backup_dir"
}

# 恢复数据
cmd_restore() {
    local backup_path=${2:-}
    if [[ -z "$backup_path" || ! -d "$backup_path" ]]; then
        log_error "请指定有效的备份目录: $0 restore <backup_dir>"
        exit 1
    fi
    
    log_warn "即将恢复备份: $backup_path"
    read -p "确定继续? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "已取消"
        exit 0
    fi
    
    log_info "恢复数据..."
    
    # 恢复数据库
    if [[ -f "$backup_path/database.sql" ]]; then
        docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" exec -T postgres \
            psql -U surveillance -d surveillance < "$backup_path/database.sql"
    fi
    
    # 恢复配置
    cp -r "$backup_path/configs/"* "$PROJECT_DIR/configs/"
    cp "$backup_path/.env" "$ENV_FILE"
    
    # 重启服务
    cmd_restart
    
    log_success "恢复完成"
}

# 清理资源
cmd_clean() {
    log_warn "这将删除所有容器、网络和数据卷！"
    read -p "确定继续? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "已取消"
        exit 0
    fi
    
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" down -v --remove-orphans
    docker system prune -f
    log_success "清理完成"
}

# 显示帮助
show_help() {
    cat <<EOF
监控录像系统部署管理脚本

用法: $0 <command> [options]

命令:
  start       启动所有服务
  stop        停止所有服务
  restart     重启所有服务
  logs [svc]  查看日志 (可指定服务名)
  status      查看服务状态和资源使用
  update      更新代码并重新构建
  backup      备份数据库和配置
  restore <dir>  恢复指定目录的备份
  clean       清理所有 Docker 资源 (危险!)
  help        显示此帮助

示例:
  $0 start
  $0 logs surveillance
  $0 backup
  $0 restore ./backups/20240101_120000

服务端口:
  - 80/443: Nginx 反向代理 (Web 界面)
  - 8080: API 服务
  - 8081: WebSocket
  - 9000/9001: MinIO API/控制台
  - 3000: Grafana
  - 9090: Prometheus
  - 5432: PostgreSQL
  - 6379: Redis

配置文件:
  - $PROJECT_DIR/configs/config.yaml
  - $PROJECT_DIR/deployments/.env

EOF
}

# 主入口
case "${1:-help}" in
    start) cmd_start ;;
    stop) cmd_stop ;;
    restart) cmd_restart ;;
    logs) cmd_logs "$@" ;;
    status) cmd_status ;;
    update) cmd_update ;;
    backup) cmd_backup ;;
    restore) cmd_restore "$@" ;;
    clean) cmd_clean ;;
    help|*) show_help ;;
esac