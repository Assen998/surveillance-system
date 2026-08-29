-- PostgreSQL 初始化脚本
-- 此脚本在 PostgreSQL 容器首次启动时自动执行

-- 创建扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 设置时区
SET timezone = 'Asia/Shanghai';

-- 创建用户表
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    username VARCHAR(50) UNIQUE NOT NULL,
    password VARCHAR(100) NOT NULL,
    email VARCHAR(100) UNIQUE,
    phone VARCHAR(20),
    role VARCHAR(20) NOT NULL DEFAULT 'viewer',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    last_login TIMESTAMPTZ
);

-- 创建摄像头表
CREATE TABLE IF NOT EXISTS cameras (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    protocol VARCHAR(20) NOT NULL DEFAULT 'rtsp',
    ip VARCHAR(45) NOT NULL,
    port INTEGER NOT NULL DEFAULT 554,
    username VARCHAR(50),
    password VARCHAR(100),
    path VARCHAR(200),
    onvif_address VARCHAR(200),
    device_id VARCHAR(50) UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'offline',
    last_online TIMESTAMPTZ,
    error_msg VARCHAR(500),
    record_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    record_schedule VARCHAR(100) DEFAULT '0-23',
    record_type VARCHAR(20) DEFAULT 'continuous',
    width INTEGER DEFAULT 1920,
    height INTEGER DEFAULT 1080,
    fps INTEGER DEFAULT 25,
    bitrate INTEGER DEFAULT 4096,
    codec VARCHAR(20) DEFAULT 'h264',
    ptz_enabled BOOLEAN DEFAULT FALSE,
    manufacturer VARCHAR(100),
    model VARCHAR(100),
    firmware VARCHAR(50),
    serial_number VARCHAR(100)
);

-- 创建录像表
CREATE TABLE IF NOT EXISTS recordings (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    camera_id BIGINT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    duration INTEGER NOT NULL,
    file_path VARCHAR(500) NOT NULL,
    file_size BIGINT NOT NULL DEFAULT 0,
    segment_index INTEGER DEFAULT 0,
    record_type VARCHAR(20) DEFAULT 'continuous',
    status VARCHAR(20) DEFAULT 'completed',
    index_path VARCHAR(500),
    storage_type VARCHAR(20) DEFAULT 'local',
    storage_path VARCHAR(500)
);

-- 创建抓拍表
CREATE TABLE IF NOT EXISTS snapshots (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    camera_id BIGINT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL,
    file_path VARCHAR(500) NOT NULL,
    file_size BIGINT NOT NULL DEFAULT 0,
    type VARCHAR(20) DEFAULT 'schedule',
    storage_type VARCHAR(20) DEFAULT 'local'
);

-- 创建报警表
CREATE TABLE IF NOT EXISTS alerts (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    camera_id BIGINT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    type VARCHAR(30) NOT NULL,
    level VARCHAR(10) NOT NULL DEFAULT 'medium',
    message VARCHAR(500),
    details TEXT,
    snapshot_path VARCHAR(500),
    video_path VARCHAR(500),
    status VARCHAR(20) NOT NULL DEFAULT 'new',
    acked_by BIGINT REFERENCES users(id),
    acked_at TIMESTAMPTZ,
    resolved_by BIGINT REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    notified BOOLEAN DEFAULT FALSE,
    notify_at TIMESTAMPTZ
);

-- 创建用户-摄像头权限表
CREATE TABLE IF NOT EXISTS camera_permissions (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    camera_id BIGINT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    permission VARCHAR(20) NOT NULL,
    UNIQUE(user_id, camera_id)
);

-- 创建系统配置表
CREATE TABLE IF NOT EXISTS system_configs (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    key VARCHAR(100) UNIQUE NOT NULL,
    value TEXT,
    description VARCHAR(200)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_cameras_status ON cameras(status);
CREATE INDEX IF NOT EXISTS idx_cameras_device_id ON cameras(device_id);
CREATE INDEX IF NOT EXISTS idx_recordings_camera_time ON recordings(camera_id, start_time DESC, end_time DESC);
CREATE INDEX IF NOT EXISTS idx_recordings_time_range ON recordings(start_time, end_time);
CREATE INDEX IF NOT EXISTS idx_snapshots_camera_time ON snapshots(camera_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_camera_time ON alerts(camera_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_type_level ON alerts(type, level);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- 插入默认管理员用户 (密码: admin123, bcrypt hash)
INSERT INTO users (username, password, email, role, status) 
VALUES ('admin', '$2a$10$MY62Xh/mqv2QCIb.NI19WOS0nSLxStwPtC/NrmhraUn3zPBBPxmOq', 'admin@localhost', 'admin', 'active')
ON CONFLICT (username) DO NOTHING;

-- 插入默认系统配置
INSERT INTO system_configs (key, value, description) VALUES
('system.name', '监控录像系统', '系统名称'),
('system.version', '1.0.0', '系统版本'),
('storage.cleanup.enabled', 'true', '是否启用自动清理'),
('storage.cleanup.max_days', '7', '录像保留天数'),
('camera.discovery.enabled', 'true', '是否启用自动发现'),
('analytics.motion.enabled', 'true', '是否启用运动检测'),
('analytics.object.enabled', 'true', '是否启用目标检测')
ON CONFLICT (key) DO NOTHING;

-- 更新 updated_at 触发器函数
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- 为所有表添加 updated_at 触发器
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_cameras_updated_at BEFORE UPDATE ON cameras FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_recordings_updated_at BEFORE UPDATE ON recordings FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_snapshots_updated_at BEFORE UPDATE ON snapshots FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_alerts_updated_at BEFORE UPDATE ON alerts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_camera_permissions_updated_at BEFORE UPDATE ON camera_permissions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_system_configs_updated_at BEFORE UPDATE ON system_configs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- 授权 surveillance 用户
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO surveillance;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO surveillance;