# 监控录像系统

一个基于 Go + Vue 3 的视频监控与录像管理系统，支持多路摄像头接入、实时预览、历史回放、录像存储、报警推送与用户权限管理。

> 界面与文档均为中文。

---

## ✨ 功能特性

### 🎥 摄像头接入
- 以 **ONVIF** 方式接入（自动发现 → 获取 RTSP 流地址 → 拉流预览 / 录像）
- ONVIF 设备自动发现与局域网探测
- 实时预览（HLS 低延迟）、PTZ 云台控制、手动抓拍
- 摄像头启停 / 重启、在线状态监控

### 🎬 录像与回放
- 录像策略：连续录像、移动侦测录像、定时录像、手动录像
- 分段存储（默认 3 分钟 / 段，可配置），自动索引、快速定位
- 历史回放（HLS）、片段下载
- 抓拍图片管理（全局浏览、筛选、一键清空）

### 🔔 报警推送
- **ONVIF 事件驱动的移动侦测**：摄像头硬件上报移动事件 → 生成报警 → 自动触发录像
  - 录像时长 60 秒、冷却 60 秒（`camera.motion_record` 可配置）
- 分级报警（低 / 中 / 高 / 严重），去重与冷却
- 多渠道推送：Webhook（钉钉 / 飞书 / 企业微信）、Email（SMTP）、SMS

### 💾 存储管理
- **本地存储**：磁盘空间监控、按保留天数 / 容量上限自动清理
- **WebDAV 存储**：远程归档、按保留天数 / 容量上限清理；支持「仅存 WebDAV」模式（上传即删本地，本地仅作临时缓冲，旧录像回放自动走 WebDAV 流式播放）
- **MinIO / S3**：可选对象存储（配置已预留）
- 存储占用统计

### 👥 权限管理
- RBAC 三级角色：管理员 / 操作员 / 只读用户
- JWT 认证、用户管理、修改密码

---

## 🏗 技术架构

```
┌──────────────────────────────────────────────────────────────┐
│                        前端 (Vue 3)                          │
│   Vue 3 + Element Plus + Pinia + Vue Router + HLS.js + ECharts │
└──────────────────────────┬───────────────────────────────────┘
                           │ HTTP (REST) / WebSocket
┌──────────────────────────▼───────────────────────────────────┐
│                      API 服务 (Gin)                          │
│        JWT 认证 · 静态资源服务 · SPA 回退 · WebSocket        │
└──────────────────────────┬───────────────────────────────────┘
                            │
       ┌────────────────────┼────────────────────┐
       ▼                    ▼                    ▼
┌──────────────┐   ┌────────────────┐   ┌────────────────┐
│  摄像头管理   │   │    存储管理     │   │  报警 / 事件   │
│  RTSP/FFmpeg │   │ 本地 · WebDAV  │   │ ONVIF 事件订阅 │
│  ONVIF 发现  │   │ MinIO (可选)   │   │ Webhook/Email/ │
│  PTZ 控制    │   │ 自动清理        │   │ SMS 推送       │
└──────────────┘   └────────────────┘   └────────────────┘
       │                    │                    │
       └────────────────────┼────────────────────┘
                            ▼
                  ┌──────────────────────┐
                  │  基础设施             │
                  │ SQLite + FFmpeg      │
                  └──────────────────────┘
```

### 技术栈

| 端 | 技术 |
|----|------|
| 后端 | Go 1.22 · Gin · GORM · SQLite（纯 Go 驱动，零 CGO）· FFmpeg · Viper · logrus |
| 前端 | Vue 3 · TypeScript · Element Plus · Pinia · Vue Router · HLS.js · ECharts |
| 协议 | ONVIF（底层 RTSP 拉流） |

---

## 🚀 快速开始

### 环境要求

- Go 1.22+
- Node.js 18+（仅需构建前端时）
- FFmpeg（已安装 `ffmpeg` 命令，用于拉流、转码、分段）
- SQLite（GORM 内置，无需单独安装）

### 构建

前端构建产物通过 Go 的 `go:embed` 直接嵌入后端二进制，最终产出**单个可执行文件**，分发无需携带 `web/dist` 目录。

```bash
cd surveillance-system

# 1) 先构建前端（生成 web/dist，供 go:embed 嵌入）
cd web
npm install
npm run build
cd ..

# 2) 再构建后端（自动嵌入前端 → 单二进制）
go build -o surveillance-server ./cmd/server
```

> 交叉编译（零 CGO，纯静态，支持 linux / windows / darwin × amd64 / 386 / arm / arm64）：
> ```bash
> CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -o surveillance-server ./cmd/server
> ```

### 运行

直接把 `config.yaml` 放在二进制同目录，无需参数直接启动：

```bash
./surveillance-server
```

程序会自动按顺序查找配置：同目录 `config.yaml` → `configs/config.yaml`，也可显式指定：

```bash
./surveillance-server /path/to/config.yaml
```

启动后：

- Web 界面：`http://localhost:8080`
- 健康检查：`GET http://localhost:8080/health`
- 默认账号：**admin / admin123**（首次登录后请尽快修改密码）

### 直接下载发行版（无需编译）

到 [GitHub Releases](https://github.com/Assen998/surveillance-system/releases) 下载对应平台的压缩包（包名带版本号，如 `surveillance-system-1.0.7-linux-arm64.tar.gz`）。解压后得到一个**不带版本号的稳定目录**（如 `surveillance-system-linux-arm64/`），目录内已含：

```
surveillance-server         # 单二进制（前端已内嵌）
config.yaml                 # 默认配置
README.md
```

进入该目录直接运行即可：

```bash
./surveillance-server        # 自动读取同目录 config.yaml
```

### 升级

目录名不带版本号，升级时**直接在新版压缩包上解压覆盖同名目录**即可：

```bash
cd /root    # 假设部署在 /root/surveillance-system-linux-arm64
cp surveillance-system-linux-arm64/config.yaml /tmp/config.yaml.bak   # ① 备份你的配置
tar xzf ~/surveillance-system-1.0.7-linux-arm64.tar.gz                # ② 覆盖解压
cp /tmp/config.yaml.bak surveillance-system-linux-arm64/config.yaml   # ③ 恢复配置
systemctl restart surveillance-server   # ④ 重启（前台运行则重新 ./surveillance-server）
```

- `data/`（摄像头/账号等数据库）、`recordings/`（录像）**不在包内，覆盖解压自动保留**，无需手动拷贝
- `config.yaml` 在包内，覆盖解压会被重置为默认值 → 按上面 ①③ 备份恢复（或升级后在网页「系统设置」里重新保存各项配置）
- systemd 服务、开机自启等对目录的引用**无需任何改动**

---

## 📖 配置说明

配置文件位于 `configs/config.yaml`（源码）/ 产物目录内 `config.yaml`（发行版），关键配置项如下：

```yaml
server:
  host: 0.0.0.0
  http_port: 8080
  mode: release

database:
  type: sqlite
  sqlite:
    path: ./data/surveillance.db

storage:
  local:
    enabled: true
    root_path: ./recordings
    segment_duration: 180       # 分段时长（秒）
    max_days: 7                 # 保留天数
    max_storage_gb: 10          # 容量上限（GB）
    cleanup_interval: 3600      # 清理检查间隔（秒）
  webdav:
    enabled: true
    url: http://127.0.0.1:5244/dav
    username: ""
    password: ""
    base_path: surveillance
    max_days: 30
    max_storage_gb: 150
    only: false                 # 仅存 WebDAV：true 时录像上传成功即删本地副本，本地仅作临时缓冲
  minio:
    enabled: false              # 可选

camera:
  onvif_event:
    enabled: true
    poll_interval: 10
    subscription_timeout: 60
  motion_record:                # 移动侦测录像（ONVIF 事件触发）
    duration: 60                # 录像时长（秒）
    pre_record: 0
    cooldown: 60                # 冷却（秒）

alert:
  enabled: true
  channels:
    webhook:                    # 钉钉/飞书/企业微信
      enabled: true
      url: ""
    email:                      # SMTP
      enabled: false
      # ...
    sms:                        # 短信
      enabled: false
      # ...

logging:
  level: info
  format: json
  output: ./logs/surveillance.log
```

> 说明：配置中的 `redis`、`postgres`、`minio`、`gb28181` 为预留项，当前版本默认使用 SQLite + 本地存储，可按需启用。

---

## 📡 API 接口

统一前缀 `/api/v1`，除登录外均需 `Authorization: Bearer <token>`。

### 认证
| 方法 | 路径 | 说明 |
|-----|------|------|
| POST | `/auth/login` | 登录 |
| POST | `/auth/logout` | 登出 |
| GET | `/auth/me` | 当前用户 |
| PUT | `/auth/password` | 修改密码 |

### 摄像头管理
| 方法 | 路径 | 说明 |
|-----|------|------|
| GET | `/cameras` | 列表 |
| POST | `/cameras` | 创建 |
| GET | `/cameras/:id` | 详情 |
| PUT | `/cameras/:id` | 更新 |
| DELETE | `/cameras/:id` | 删除 |
| GET | `/cameras/:id/status` | 运行状态 |
| POST | `/cameras/:id/start` | 启动 |
| POST | `/cameras/:id/stop` | 停止 |
| POST | `/cameras/:id/restart` | 重启 |
| POST | `/cameras/:id/snapshot` | 手动抓拍 |
| POST | `/cameras/:id/ptz` | PTZ 控制 |
| GET | `/cameras/:id/snapshots` | 摄像头抓拍列表 |
| GET | `/cameras/discover` | ONVIF 自动发现 |
| POST | `/cameras/discover/lan` | 局域网探测 |
| GET | `/cameras/probe` | ONVIF 探测 |

### 录像管理
| 方法 | 路径 | 说明 |
|-----|------|------|
| GET | `/recordings` | 列表（分页 / 筛选） |
| GET | `/recordings/:id` | 详情 |
| DELETE | `/recordings/:id` | 删除 |
| GET | `/recordings/camera/:cameraId` | 指定摄像头录像 |
| GET | `/recordings/camera/:cameraId/segments` | 时间段片段 |

### 抓拍图片
| 方法 | 路径 | 说明 |
|-----|------|------|
| GET | `/snapshots` | 全局抓拍列表 |
| DELETE | `/snapshots` | 一键清空 |
| DELETE | `/snapshots/:id` | 删除单张 |

### 报警
| 方法 | 路径 | 说明 |
|-----|------|------|
| GET | `/analytics/alerts` | 报警列表 |
| DELETE | `/analytics/alerts` | 清空报警 |
| GET | `/analytics/alerts/:id` | 报警详情 |
| PUT | `/analytics/alerts/:id/ack` | 确认 |
| PUT | `/analytics/alerts/:id/resolve` | 解决 |
| DELETE | `/analytics/alerts/:id` | 删除 |
| GET | `/alerts/config` | 报警配置 |
| PUT | `/alerts/config` | 更新报警配置 |
| POST | `/alerts/test` | 发送测试报警 |

### 存储
| 方法 | 路径 | 说明 |
|-----|------|------|
| GET | `/storage/stats` | 存储统计 |
| POST | `/storage/cleanup` | 触发清理 |
| GET | `/settings/storage` | 存储设置 |
| PUT | `/settings/storage` | 更新存储设置 |
| POST | `/settings/webdav/test` | WebDAV 连接测试 |

### 流媒体 / 回放
| 方法 | 路径 | 说明 |
|-----|------|------|
| GET | `/stream/camera/:id/hls` | 实时 HLS 播放列表 |
| GET | `/stream/camera/:id/hls/*file` | HLS 切片 |
| GET | `/stream/camera/:id/mp4` | 最新 MP4 片段 |
| GET | `/stream/camera/:id/snapshot` | 最新抓拍 |
| GET | `/stream/camera/:id/snapshots/*file` | 抓拍文件 |
| GET | `/stream/camera/:id/recordings/:recordingId/hls` | 历史录像 HLS |
| GET | `/recordings/:id/file` | 录像文件 |
| GET | `/recordings/:id/download` | 下载录像 |
| GET | `/webdav/list` | WebDAV 文件列表 |
| GET | `/webdav/file` | WebDAV 文件流 |

### 系统
| 方法 | 路径 | 说明 |
|-----|------|------|
| GET | `/system/config` | 系统配置 |
| PUT | `/system/config` | 更新系统配置 |
| GET | `/system/info` | 系统信息 |
| POST | `/system/restart` | 重启系统 |

### WebSocket
| 路径 | 说明 |
|------|------|
| `/ws` | 全局实时事件（报警 / 状态 / 录像） |
| `/api/v1/ws/camera/:cameraId` | 摄像头实时事件 |

---

## 🐳 Docker 部署

部署脚本与编排文件位于 `deployments/`，包含主应用、Redis、MinIO、PostgreSQL、Nginx、Prometheus、Grafana 等可选服务。

```bash
cd deployments
./deploy.sh start      # 启动所有服务
./deploy.sh stop       # 停止
./deploy.sh restart    # 重启
./deploy.sh logs       # 查看日志
./deploy.sh status     # 状态
./deploy.sh backup     # 备份
./deploy.sh restore    # 恢复
```

> 当前版本默认无需 Redis / MinIO / PostgreSQL 即可运行，相关服务为可选扩展。

---

## 🔧 开发指南

### 目录结构

```
surveillance-system/
├── cmd/server/             # 主程序入口
├── internal/
│   ├── api/                # HTTP API 处理、路由注册
│   ├── camera/             # 摄像头管理、录像触发
│   ├── storage/            # 存储管理（本地 / WebDAV / MinIO）
│   ├── alert/              # 报警推送（Webhook/Email/SMS）
│   ├── onvifevent/         # ONVIF 事件订阅（移动侦测上报）
│   ├── config/             # 配置加载与持久化
│   ├── database/           # 数据库初始化与默认数据
│   └── models/             # 数据模型
├── pkg/
│   ├── ffmpeg/             # FFmpeg 封装（拉流 / HLS / 分段）
│   ├── onvif/              # ONVIF 客户端
│   ├── webdav/             # WebDAV 客户端
│   └── gb28181/            # GB28181 协议（预留）
├── web/                    # Vue 3 前端
│   └── src/
│       ├── api/            # API 请求封装
│       ├── views/          # 页面视图
│       ├── stores/         # Pinia 状态
│       ├── router/         # 路由配置
│       └── components/     # 通用组件
├── configs/config.yaml     # 配置文件
└── deployments/            # 部署相关（Docker）
```

### 关键流程

- **移动侦测录像**：摄像头通过 ONVIF 事件上报移动 → `onvifevent` 生成报警 → `TriggerMotionRecording` 按 `camera.motion_record` 配置录制 60 秒并进入 60 秒冷却。

### 添加新功能模块

1. 在 `internal/` 下创建模块目录
2. 实现 `Start()` / `Stop()` 生命周期方法
3. 在 `cmd/server/main.go` 中注册并启动
4. 在 `internal/api/server.go` 中添加路由
5. 前端添加对应页面和 API 调用

---

## 🔍 常见问题

### 摄像头显示离线 / 连接失败
1. 检查网络连通性：`ping <camera_ip>`，确认 RTSP 端口（通常 554）可达
2. 用 VLC 验证 RTSP 地址：`rtsp://user:pass@ip:554/stream1`
3. 确认摄像头编码为 H.264 / H.265
4. 查看服务日志定位错误信息

### 视频无法播放 / 卡顿
1. 确保浏览器支持 HLS（Chrome / Firefox / Edge 均可）
2. 确认 FFmpeg 已安装：`ffmpeg -version`
3. 调整 `storage.local.segment_duration` 与 HLS 切片时长
4. 检查磁盘写入速度是否成为瓶颈

### 移动侦测录像不触发
- 移动侦测依赖摄像头 ONVIF 事件上报，请确认：
  1. 摄像头已启用并支持 ONVIF 事件（Analytics / MotionDetection）
  2. `camera.onvif_event.enabled` 为 `true`
  3. ONVIF 事件订阅参数正确（`poll_interval` / `subscription_timeout`）
  4. 录像类型设置为「移动侦测」或移动侦测触发录像已开启

### 磁盘空间不足
1. **开启「仅存 WebDAV」模式**（系统设置 → 存储 → WebDAV → 仅存 WebDAV，或配置 `webdav.only: true`）：录像上传成功即删本地副本，本地磁盘只保留上传前的临时缓冲（一段录像大小），适合本地磁盘小、以 WebDAV 为主存储的场景；旧录像回放自动走 WebDAV 流式播放
2. 减小 `max_days` 或增大分段清理频率
3. 设置 `max_storage_gb` 容量上限
4. 启用 WebDAV 远程归档分流
5. 手动触发清理：`POST /api/v1/storage/cleanup`

---

## 🤝 贡献指南

1. Fork 项目
2. 创建特性分支：`git checkout -b feat/amazing-feature`
3. 提交变更：`git commit -m 'feat: add amazing feature'`
4. 推送分支：`git push origin feat/amazing-feature`
5. 发起 Pull Request

---

## 🙏 致谢

- [FFmpeg](https://ffmpeg.org/) — 多媒体处理核心
- [ONVIF](https://www.onvif.org/) — 网络视频接口标准
- [Gin](https://github.com/gin-gonic/gin) — Go Web 框架
- [Vue.js](https://vuejs.org/) — 渐进式前端框架
- [Element Plus](https://element-plus.org/) — Vue 3 组件库
- [HLS.js](https://github.com/video-dev/hls.js/) — HTTP Live Streaming 播放器
- [GORM](https://gorm.io/) — Go ORM 框架