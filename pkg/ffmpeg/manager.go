package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

type Manager struct {
	streams map[uint]*Stream
	mu      sync.RWMutex
}

type StreamOptions struct {
	CameraID       uint
	SegmentDuration int
	OutputDir      string
	OnSegment      func(cameraID uint, segment *SegmentInfo)
	OnError        func(error)
	// NoRecord 纯预览模式：不输出 MP4 录像分段，仅输出 HLS 预览。
	// 用于 RecordType=motion 的事件型录像（平时不录，移动触发时另起临时录像进程）。
	// 注意：拆分后该模式已不再使用常驻预览流；保留字段以兼容旧调用，新代码走 PreviewStream。
	NoRecord bool
	// RecordOnly 纯录像模式：仅输出 MP4 分段录像（-c copy 主码流），
	// 不内嵌 HLS 预览输出。预览由独立的 PreviewStream（子码流、按需启动）承担。
	RecordOnly bool
}

type Stream struct {
	opts       StreamOptions
	rtspURL    string
	cmd        *exec.Cmd
	ctx        context.Context
	cancel     context.CancelFunc
	doneChan   chan struct{}
	running    bool
	mu         sync.Mutex
	startTime  time.Time
	segmentIdx int
	stderrTail bytes.Buffer
	stderrMu   sync.Mutex

	// 分段实时监控（CSV 索引文件追加即触发回调，不必等 ffmpeg 退出）
	runStartTime  time.Time
	runCounter    int // 每次 runSegment 递增，保证文件名前缀唯一（避免重连/重试覆盖旧文件）
	processedSegs map[string]bool
	processedMu   sync.Mutex
	csvWatcher    chan struct{}

	// restartRequested 标记本次 ffmpeg 退出是 Restart() 主动触发（健康检查重连），
	// runSegment 据此将其视为正常停止而非异常
	restartRequested bool
}

type SegmentInfo struct {
	Index      int
	StartTime  time.Time
	EndTime    time.Time
	FilePath   string
	FileSize   int64
	IndexPath  string
	Duration   float64
}

func NewManager() *Manager {
	return &Manager{
		streams: make(map[uint]*Stream),
	}
}

func (m *Manager) CreateStream(rtspURL string, opts StreamOptions) (*Stream, error) {
	// 确保输出目录存在
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Stream{
		opts:          opts,
		rtspURL:       rtspURL,
		ctx:           ctx,
		cancel:        cancel,
		doneChan:      make(chan struct{}),
		segmentIdx:    0,
		processedSegs: make(map[string]bool),
	}

	m.mu.Lock()
	m.streams[opts.CameraID] = s
	m.mu.Unlock()

	return s, nil
}

func (s *Stream) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("流已在运行")
	}

	s.startTime = time.Now()
	s.running = true

	go s.run()
	return nil
}

func (s *Stream) run() {
	defer func() {
		s.mu.Lock()
		s.running = false
		close(s.doneChan)
		s.mu.Unlock()
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			if err := s.runSegment(); err != nil {
				if s.ctx.Err() == context.Canceled {
					return
				}
				logrus.Errorf("摄像头 %d 录像分段错误: %v", s.opts.CameraID, err)
				if s.opts.OnError != nil {
					s.opts.OnError(err)
				}
				// 短暂等待后重试
				select {
				case <-s.ctx.Done():
					return
				case <-time.After(2 * time.Second):
					continue
				}
			}
		}
	}
}

func (s *Stream) runSegment() error {
	segmentStart := time.Now()
	s.runStartTime = segmentStart
	// 每次运行用唯一前缀（时间戳+序号），避免重连/重试后新流覆盖同名旧分段文件
	s.runCounter++
	runPrefix := "segment_" + segmentStart.Format("20060102_150405") + "_" + fmt.Sprintf("%02d", s.runCounter)
	// 分段文件名必须是模板（含 %d），由 ffmpeg 分段器自行编号
	segmentTemplate := filepath.Join(s.opts.OutputDir, runPrefix+"_%06d.mp4")
	indexFile := filepath.Join(s.opts.OutputDir, runPrefix+".idx")
	hlsSegmentFile := filepath.Join(s.opts.OutputDir, "hls_segment_%03d.ts")
	hlsPlaylist := filepath.Join(s.opts.OutputDir, "index.m3u8")

	// 单 RTSP 会话双输出：
	//   NoRecord=false：
	//     输出1：MP4 分段录像（流拷贝，不转码）
	//     输出2：HLS（libx264 低延迟转码）供 Web 预览
	//   NoRecord=true：仅输出 HLS 预览（纯预览模式，移动检测触发型录像时不写 MP4）
	// 摄像头通常限制并发 RTSP 会话数，必须合并为单会话
	args := []string{
		"-y",                                     // 覆盖输出
		"-rtsp_transport", "tcp",                 // TCP 传输更稳定
		"-timeout", "5000000",                    // RTSP 读超时 5 秒（微秒）
		"-i", "INPUT_URL_PLACEHOLDER",            // 占位符，稍后替换
	}

	if !s.opts.NoRecord {
		// ===== 输出 1：MP4 分段录像 =====
		args = append(args,
			"-map", "0:v:0",
			"-c", "copy",                             // 直接复制流，不转码（节省 CPU）
			"-movflags", "+faststart",                // 分段完成后将 moov 移到文件头：浏览器秒得时长、可直接拖动进度
			"-f", "segment",                          // 分段格式
			"-segment_time", strconv.Itoa(s.opts.SegmentDuration),
			"-segment_format", "mp4",
			"-reset_timestamps", "1",
			"-segment_list", indexFile,               // 生成索引文件
			"-segment_list_type", "csv",
			"-segment_list_entry_prefix", "",
			"-start_number", "0",                     // 每次运行前缀唯一，编号始终从 0 开始
			segmentTemplate,                          // 输出文件名模板
		)
	}

	// ===== 输出 2：HLS Web 预览 =====
	// 拆分后预览由独立的 PreviewStream（子码流、按需）承担；
	// RecordOnly 模式下录像流不再内嵌 HLS，省去常驻转码开销。
	if !s.opts.RecordOnly {
		args = append(args,
			"-map", "0:v:0",
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-g", "50",                               // 每 2 秒一个关键帧（25fps）
			"-keyint_min", "50",
			"-sc_threshold", "0",
			"-pix_fmt", "yuv420p",
			"-an",                                    // 预览不需要音频
			"-f", "hls",
			"-hls_time", "2",
			"-hls_list_size", "6",
			"-hls_flags", "delete_segments+append_list",
			"-hls_segment_filename", hlsSegmentFile,
			hlsPlaylist,
		)
	}

	// 替换 RTSP URL
	for i, arg := range args {
		if arg == "INPUT_URL_PLACEHOLDER" {
			args[i] = s.getRTSPURL()
			break
		}
	}

	s.mu.Lock()
	s.cmd = exec.CommandContext(s.ctx, "ffmpeg", args...)
	s.mu.Unlock()
	// 注意：不要设置 cmd.Dir——args 中的路径是相对于服务器工作目录的
	// （如 recordings/camera_N/...），ffmpeg 必须继承服务器 CWD 才能正确解析

	// 捕获 stderr 用于调试
	stderr, _ := s.cmd.StderrPipe()
	stdout, _ := s.cmd.StdoutPipe()

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("启动 ffmpeg 失败: %w", err)
	}

	// 异步读取输出
	go s.readOutput(stderr, "stderr")
	go s.readOutput(stdout, "stdout")

	// 启动分段实时监控：ffmpeg 分段器每完成一个分段就向 CSV 追加一行，
	// 轮询 CSV 即可实时触发 OnSegment（录像入库 / WebDAV 上传），无需等 ffmpeg 退出
	s.startCSVWatcher(indexFile)

	// 等待 ffmpeg 退出或取消
	err := s.cmd.Wait()
	s.stopCSVWatcher()

	// 取回"是否为 Restart 主动触发"标记（ffmpeg 收到 SIGINT 后可能以退出码退出而非信号）
	s.mu.Lock()
	wasRestart := s.restartRequested
	s.restartRequested = false
	s.mu.Unlock()

	if err != nil && s.ctx.Err() != context.Canceled {
		// Restart 主动触发或 SIGINT 终止：视为正常重启，不算异常
		if wasRestart || isInterrupted(err) {
			s.processCSVNewLines(indexFile)
			return nil
		}
		return fmt.Errorf("ffmpeg 退出异常: %w (stderr 尾部: %s)", err, s.stderrTailString())
	}

	// 收尾：处理退出前刚写入 CSV 的最后一个分段（幂等，按文件名去重）
	s.processCSVNewLines(indexFile)

	return nil
}

// startCSVWatcher 启动 CSV 索引文件监控（每 2 秒扫描一次新增行）
func (s *Stream) startCSVWatcher(indexFile string) {
	if s.csvWatcher != nil {
		return
	}
	s.csvWatcher = make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.csvWatcher:
				return
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.processCSVNewLines(indexFile)
			}
		}
	}()
}

// stopCSVWatcher 停止 CSV 监控
func (s *Stream) stopCSVWatcher() {
	if s.csvWatcher != nil {
		close(s.csvWatcher)
		s.csvWatcher = nil
	}
}

// processCSVNewLines 处理 CSV 中新增的分段记录（按文件名去重，可并发安全调用）
// CSV 格式: 文件名,开始秒,结束秒
func (s *Stream) processCSVNewLines(indexFile string) {
	data, err := os.ReadFile(indexFile)
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		f := parts[0]
		if !strings.HasSuffix(f, ".mp4") {
			continue
		}

		// 原子认领该分段（防止 watcher 与收尾处理重复注册）
		s.processedMu.Lock()
		if s.processedSegs[f] {
			s.processedMu.Unlock()
			continue
		}
		s.processedSegs[f] = true
		idx := s.segmentIdx
		s.segmentIdx++
		s.processedMu.Unlock()

		full := filepath.Join(s.opts.OutputDir, f)
		info, err := os.Stat(full)
		if err != nil || info.Size() == 0 {
			// 文件尚未落盘完成，撤销认领等待下轮
			s.processedMu.Lock()
			delete(s.processedSegs, f)
			s.segmentIdx--
			s.processedMu.Unlock()
			continue
		}

		segStart := s.runStartTime
		segEnd := time.Now()
		if len(parts) >= 3 {
			if so, err := strconv.ParseFloat(parts[1], 64); err == nil {
				segStart = s.runStartTime.Add(time.Duration(so * float64(time.Second)))
			}
			if eo, err := strconv.ParseFloat(parts[2], 64); err == nil {
				segEnd = s.runStartTime.Add(time.Duration(eo * float64(time.Second)))
			}
		}
		segInfo := &SegmentInfo{
			Index:     idx,
			StartTime: segStart,
			EndTime:   segEnd,
			FilePath:  full,
			FileSize:  info.Size(),
			IndexPath: indexFile,
			Duration:  segEnd.Sub(segStart).Seconds(),
		}
		if s.opts.OnSegment != nil {
			s.opts.OnSegment(s.opts.CameraID, segInfo)
		}
	}
}

// isInterrupted 判断进程是否被 SIGINT 终止（健康检查 Restart 触发）
func isInterrupted(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus); ok {
			return ws.Signaled() && ws.Signal() == syscall.SIGINT
		}
	}
	return false
}

func (s *Stream) getRTSPURL() string {
	return s.rtspURL
}

func (s *Stream) readOutput(pipe interface{}, prefix string) {
	rc, ok := pipe.(io.ReadCloser)
	if !ok {
		return
	}
	defer rc.Close()
	buf := make([]byte, 4096)
	for {
		n, err := rc.Read(buf)
		if n > 0 && prefix == "stderr" {
			// 保留 stderr 尾部 8KB，供失败诊断
			s.stderrMu.Lock()
			s.stderrTail.Write(buf[:n])
			if s.stderrTail.Len() > 16384 {
				b := make([]byte, s.stderrTail.Len())
				copy(b, s.stderrTail.Bytes())
				s.stderrTail.Reset()
				s.stderrTail.Write(b[len(b)-8192:])
			}
			s.stderrMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// stderrTailString 返回保留的 stderr 尾部
func (s *Stream) stderrTailString() string {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()
	return strings.TrimSpace(s.stderrTail.String())
}

// Stop 永久停止流（取消 ctx，等待 run 循环退出）
// 注意：绝不能在两个 goroutine 里并发调用 cmd.Wait()——
// process.Wait 的结果通过单值通道传递，第二次 Wait 会永久阻塞其中一个
func (s *Stream) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	var proc *os.Process
	if s.cmd != nil {
		proc = s.cmd.Process
	}
	s.mu.Unlock()

	// 先取消：run 循环看到 Done 后不会再新建 ffmpeg
	s.cancel()
	if proc != nil {
		// 优雅退出：让 ffmpeg 收尾当前分段
		proc.Signal(os.Interrupt)
	}

	// 等待 run 循环退出（runSegment 持有唯一的 cmd.Wait）
	select {
	case <-s.doneChan:
	case <-time.After(5 * time.Second):
		if proc != nil {
			proc.Kill()
		}
		// 有界等待（run 循环可能处于重连退避/新建 ffmpeg 间隙）
		select {
		case <-s.doneChan:
		case <-time.After(10 * time.Second):
			logrus.Warnf("流 %d 未能在超时内退出", s.opts.CameraID)
		}
	}
	return nil
}

// Restart 仅杀掉当前 ffmpeg 进程（不取消 ctx），run 循环检测到退出后会自动重连
// 供健康检查"重连"使用
func (s *Stream) Restart() {
	s.mu.Lock()
	var proc *os.Process
	if s.cmd != nil {
		proc = s.cmd.Process
	}
	running := s.running
	if running {
		s.restartRequested = true
	}
	s.mu.Unlock()

	if !running || proc == nil {
		return
	}
	proc.Signal(os.Interrupt)
}

func (s *Stream) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// IsHealthy 检查流是否健康
// 依据：
//   - HLS 模式：播放列表每 ~2 秒重写，若 60 秒无更新说明 RTSP 断流/编码卡死
//   - RecordOnly 模式：检查最新分段 .mp4 的写入时间，若超时说明拉流停滞
func (s *Stream) IsHealthy() bool {
	s.mu.Lock()
	running := s.running
	outDir := s.opts.OutputDir
	startTime := s.startTime
	recordOnly := s.opts.RecordOnly
	s.mu.Unlock()
	if !running {
		return false
	}

	// 刚启动的宽限期（流尚未产出首个分段/播放列表）
	gracePeriod := 30 * time.Second

	if recordOnly {
		// 纯录像：以最新分段文件 mtime 为心跳（-c copy 写盘较突发，窗口放宽到 90s）
		newest := s.newestSegmentModTime(outDir)
		if newest.IsZero() {
			return time.Since(startTime) < gracePeriod
		}
		return time.Since(newest) < 90*time.Second
	}

	info, err := os.Stat(filepath.Join(outDir, "index.m3u8"))
	if err != nil {
		// 播放列表尚未生成（流刚启动的几秒内）不算不健康
		return time.Since(startTime) < gracePeriod
	}
	return time.Since(info.ModTime()) < 60*time.Second
}

// newestSegmentModTime 返回输出目录下最新分段 .mp4 的修改时间（不含 motion_ 临时文件）
func (s *Stream) newestSegmentModTime(outDir string) time.Time {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return time.Time{}
	}
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "segment_") || !strings.HasSuffix(e.Name(), ".mp4") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

func (s *Stream) Done() <-chan struct{} {
	return s.doneChan
}

func (s *Stream) Snapshot() (string, error) {
	s.mu.Lock()
	running := s.running
	rtspURL := s.rtspURL
	outDir := s.opts.OutputDir
	cameraID := s.opts.CameraID
	recordOnly := s.opts.RecordOnly
	s.mu.Unlock()

	if !running {
		return "", fmt.Errorf("流未运行")
	}

	snapshotPath := filepath.Join(outDir, fmt.Sprintf("snapshot_%d_%d.jpg", cameraID, time.Now().Unix()))

	if recordOnly {
		// 纯录像流无 HLS 播放列表，直接拉主码流抓一帧（瞬时 RTSP 会话，抓完即断）
		args := []string{
			"-y",
			"-rtsp_transport", "tcp",
			"-i", rtspURL,
			"-vframes", "1",
			"-q:v", "2",
			snapshotPath,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("抓拍失败: %w", err)
		}
		return snapshotPath, nil
	}

	// 从本地 HLS 播放列表抓帧（不占用摄像头的 RTSP 会话）
	hlsPlaylist := filepath.Join(outDir, "index.m3u8")
	if _, err := os.Stat(hlsPlaylist); err != nil {
		return "", fmt.Errorf("HLS 播放列表不存在，预览流尚未就绪")
	}

	args := []string{
		"-y",
		"-i", hlsPlaylist,
		"-vframes", "1",
		"-q:v", "2",
		snapshotPath,
	}

	cmd := exec.CommandContext(context.Background(), "ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("抓拍失败: %w", err)
	}

	return snapshotPath, nil
}

// GetStream 获取流实例
func (m *Manager) GetStream(cameraID uint) (*Stream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.streams[cameraID]
	return s, ok
}

// StopAll 停止所有流
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.streams {
		s.Stop()
	}
	m.streams = make(map[uint]*Stream)
}

// ProbeStream 探测流信息
func ProbeStream(rtspURL string) (*StreamInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		"-rtsp_transport", "tcp",
		rtspURL,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe 失败: %w", err)
	}

	// 解析 JSON 输出（简化版）
	info := &StreamInfo{}
	// 实际应用中使用 json.Unmarshal 解析
	_ = output

	return info, nil
}

type StreamInfo struct {
	Width    int
	Height   int
	Codec    string
	FPS      float64
	Bitrate  int64
	Duration float64
}

// HLS 转码器（用于 Web 播放）
type HLSTranscoder struct {
	cameraID   uint
	rtspURL    string
	outputDir  string
	cmd        *exec.Cmd
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewHLSTranscoder(cameraID uint, rtspURL, outputDir string) *HLSTranscoder {
	ctx, cancel := context.WithCancel(context.Background())
	return &HLSTranscoder{
		cameraID:  cameraID,
		rtspURL:   rtspURL,
		outputDir: outputDir,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (h *HLSTranscoder) Start() error {
	if err := os.MkdirAll(h.outputDir, 0755); err != nil {
		return err
	}

	args := []string{
		"-y",
		"-rtsp_transport", "tcp",
		"-i", h.rtspURL,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-c:a", "aac",
		"-b:a", "128k",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "6",
		"-hls_flags", "delete_segments+append_list",
		"-hls_segment_filename", filepath.Join(h.outputDir, "segment_%03d.ts"),
		filepath.Join(h.outputDir, "index.m3u8"),
	}

	h.cmd = exec.CommandContext(h.ctx, "ffmpeg", args...)
	return h.cmd.Start()
}

func (h *HLSTranscoder) Stop() error {
	h.cancel()
	if h.cmd != nil && h.cmd.Process != nil {
		return h.cmd.Process.Kill()
	}
	return nil
}

// PreviewStream 按需启动的 HLS 预览流（子码流转码）。
// 与录像流 Stream 解耦：仅在 Web 端预览时启动，空闲超时后停止以回收内存。
// 输出 index.m3u8 + hls_segment_XXX.ts，与 /stream/camera/:id/hls 读盘逻辑对齐。
type PreviewStream struct {
	cameraID  uint
	rtspURL   string
	outputDir string
	cmd       *exec.Cmd
	ctx       context.Context
	cancel    context.CancelFunc
	doneChan  chan struct{}
	running   bool
	startTime time.Time
	mu        sync.Mutex
}

func NewPreviewStream(cameraID uint, rtspURL, outputDir string) *PreviewStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &PreviewStream{
		cameraID:  cameraID,
		rtspURL:   rtspURL,
		outputDir: outputDir,
		ctx:       ctx,
		cancel:    cancel,
		doneChan:  make(chan struct{}),
	}
}

// Start 异步启动预览 ffmpeg（子码流 HLS 转码），启动成功后返回。
func (p *PreviewStream) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		p.mu.Unlock()
		return err
	}

	args := []string{
		"-y",
		"-rtsp_transport", "tcp",
		"-timeout", "5000000",
		"-i", p.rtspURL,
		"-map", "0:v:0",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-g", "50",
		"-keyint_min", "50",
		"-sc_threshold", "0",
		"-pix_fmt", "yuv420p",
		"-an",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "6",
		"-hls_flags", "delete_segments+append_list",
		"-hls_segment_filename", filepath.Join(p.outputDir, "hls_segment_%03d.ts"),
		filepath.Join(p.outputDir, "index.m3u8"),
	}

	p.cmd = exec.CommandContext(p.ctx, "ffmpeg", args...)
	if err := p.cmd.Start(); err != nil {
		p.mu.Unlock()
		return err
	}
	p.running = true
	p.startTime = time.Now()
	p.mu.Unlock()

	// 后台等待进程退出
	go func() {
		p.cmd.Wait()
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		close(p.doneChan)
	}()
	return nil
}

// Stop 停止预览 ffmpeg 并回收内存。
func (p *PreviewStream) Stop() {
	p.mu.Lock()
	running := p.running
	var proc *os.Process
	if p.cmd != nil {
		proc = p.cmd.Process
	}
	p.mu.Unlock()

	if !running {
		return
	}
	p.cancel()
	if proc != nil {
		proc.Kill()
	}
	select {
	case <-p.doneChan:
	case <-time.After(5 * time.Second):
	}
}

func (p *PreviewStream) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *PreviewStream) Done() <-chan struct{} {
	return p.doneChan
}

// IsHealthy 基于 index.m3u8 更新频率判断健康（与录像流 HLS 逻辑一致）。
func (p *PreviewStream) IsHealthy() bool {
	p.mu.Lock()
	running := p.running
	outDir := p.outputDir
	startTime := p.startTime
	p.mu.Unlock()
	if !running {
		return false
	}
	info, err := os.Stat(filepath.Join(outDir, "index.m3u8"))
	if err != nil {
		return time.Since(startTime) < 15*time.Second
	}
	return time.Since(info.ModTime()) < 45*time.Second
}