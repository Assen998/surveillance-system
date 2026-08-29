package webdav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// Client 轻量 WebDAV 客户端（PUT 上传 / MKCOL 建目录 / PROPFIND 检查）
type Client struct {
	baseURL  string // 如 http://192.168.1.100:5005/webdav（无尾斜杠）
	username string
	password string
	http     *http.Client
}

func NewClient(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username: username,
		password: password,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// buildURL 拼接远程路径（远程路径以 / 分隔，不含前导 /）
func (c *Client) buildURL(remotePath string) string {
	remotePath = strings.TrimPrefix(strings.TrimSpace(remotePath), "/")
	if remotePath == "" {
		return c.baseURL
	}
	return c.baseURL + "/" + remotePath
}

func (c *Client) newRequest(method, remotePath string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.buildURL(remotePath), body)
	if err != nil {
		return nil, err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return req, nil
}

// Check 检查 WebDAV 服务器连通性与凭据（PROPFIND 基础路径）
// 返回 error：401/403 凭据错误；网络错误；其他 HTTP 错误
func (c *Client) Check(basePath string) error {
	if c.baseURL == "" {
		return fmt.Errorf("WebDAV URL 未配置")
	}

	body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<propfind xmlns="DAV:">
  <prop>
    <resourcetype/>
    <getcontentlength/>
  </prop>
</propfind>`)

	req, err := c.newRequest("PROPFIND", basePath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接 WebDAV 服务器: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	switch {
	case resp.StatusCode == http.StatusMultiStatus || resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode == http.StatusNotFound:
		// 服务器可达、凭据有效，但基础目录不存在 —— 可接受（上传前会 MKCOL）
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("WebDAV 认证失败（401/403），请检查用户名/密码")
	case resp.StatusCode >= 500:
		return fmt.Errorf("WebDAV 服务器错误 (HTTP %d)", resp.StatusCode)
	default:
		return fmt.Errorf("WebDAV 响应异常 (HTTP %d)", resp.StatusCode)
	}
}

// Delete 删除远程文件（404 视为已不存在，不算错误）
func (c *Client) Delete(remotePath string) error {
	req, err := c.newRequest("DELETE", remotePath, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("删除远程文件失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	switch {
	case resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusMultiStatus:
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return nil
	default:
		return fmt.Errorf("删除远程文件失败 (HTTP %d)", resp.StatusCode)
	}
}

// EnsureDir 确保远程目录存在（MKCOL；已存在视为成功）
func (c *Client) EnsureDir(remotePath string) error {
	if strings.TrimSpace(remotePath) == "" {
		return nil
	}
	req, err := c.newRequest("MKCOL", remotePath, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("MKCOL 请求失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusMethodNotAllowed, http.StatusConflict:
		// 目录已存在
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("WebDAV 认证失败 (HTTP %d)", resp.StatusCode)
	default:
		return fmt.Errorf("创建远程目录失败 (HTTP %d): %s", resp.StatusCode, remotePath)
	}
}

// Upload 上传本地文件到远程路径（自动创建父目录）
func (c *Client) Upload(localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer f.Close()

	// 创建父目录
	if idx := strings.LastIndex(remotePath, "/"); idx > 0 {
		if err := c.EnsureDir(remotePath[:idx]); err != nil {
			return err
		}
	}

	req, err := c.newRequest("PUT", remotePath, f)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("PUT 上传失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("上传失败 (HTTP %d): %s", resp.StatusCode, remotePath)
}

// Entry WebDAV 目录内的一项（文件或子目录）
type Entry struct {
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

// propfindResponse PROPFIND 返回的 multistatus 结构（仅取所需字段）
type propfindResponse struct {
	XMLName   xml.Name `xml:"multistatus"`
	Responses []struct {
		Href     string `xml:"href"`
		Propstat struct {
			Prop struct {
				ResourceType struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
				GetContentLength string `xml:"getcontentlength"`
				GetLastModified  string `xml:"getlastmodified"`
			} `xml:"prop"`
			Status string `xml:"status"`
		} `xml:"propstat"`
	} `xml:"response"`
}

// List 列出远程目录的直接子项（PROPFIND Depth: 1）
func (c *Client) List(remotePath string) ([]Entry, error) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<propfind xmlns="DAV:">
  <prop>
    <resourcetype/>
    <getcontentlength/>
    <getlastmodified/>
  </prop>
</propfind>`)

	req, err := c.newRequest("PROPFIND", remotePath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PROPFIND 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("PROPFIND 失败 (HTTP %d)", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 PROPFIND 响应失败: %w", err)
	}

	var pr propfindResponse
	if err := xml.Unmarshal(raw, &pr); err != nil {
		return nil, fmt.Errorf("解析 PROPFIND 响应失败: %w", err)
	}

	// 规范化被列目录的路径，用于跳过"目录本身"这一项
	baseTrim := strings.Trim(strings.TrimSpace(remotePath), "/")

	var entries []Entry
	for _, r := range pr.Responses {
		href := r.Href
		if u, err := url.Parse(href); err == nil && u.Path != "" {
			href = u.Path
		}
		name := path.Base(strings.TrimRight(href, "/"))
		// 跳过目录本身（其 href 的 basename 与被列目录相同，且是 collection）
		isDir := r.Propstat.Prop.ResourceType.Collection != nil
		selfName := path.Base(baseTrim)
		if name == selfName && isDir {
			continue
		}
		if name == "" || name == "." {
			continue
		}

		entry := Entry{Name: name, IsDir: isDir}
		if r.Propstat.Prop.GetContentLength != "" {
			if n, err := strconv.ParseInt(r.Propstat.Prop.GetContentLength, 10, 64); err == nil {
				entry.Size = n
			}
		}
		if r.Propstat.Prop.GetLastModified != "" {
			for _, layout := range []string{time.RFC1123, time.RFC1123Z, time.RFC3339} {
				if t, err := time.Parse(layout, r.Propstat.Prop.GetLastModified); err == nil {
					entry.ModTime = t
					break
				}
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Get 流式读取远程文件（GET，支持 Range 请求头，用于浏览器分段播放/拖动进度）。
// 返回的 *http.Response 由调用方负责 Close。使用无总超时的独立客户端（大文件下载不受 60s 限制），
// 并通过 ctx 在客户端断开时中止上游请求。
func (c *Client) Get(ctx context.Context, remotePath, rangeHeader string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.buildURL(remotePath), nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	streamClient := &http.Client{Timeout: 0}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET 读取失败: %w", err)
	}
	return resp, nil
}

// TestAndUpload 测试连接并上传一个小文件验证可写性
func (c *Client) TestAndUpload(basePath string) error {
	if err := c.Check(basePath); err != nil {
		return err
	}
	// 上传 1 字节测试文件验证写权限
	testRemote := strings.TrimPrefix(basePath, "/")
	if testRemote != "" {
		testRemote += "/"
	}
	testRemote += ".ds_write_test"

	tmp := fmt.Sprintf("%s/ds_webdav_test_%d.bin", os.TempDir(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, []byte("ok"), 0644); err != nil {
		return err
	}
	defer os.Remove(tmp)

	if err := c.Upload(tmp, testRemote); err != nil {
		return fmt.Errorf("连接正常但写入失败: %w", err)
	}
	// 清理测试文件（DELETE，失败忽略）
	if req, err := c.newRequest("DELETE", testRemote, nil); err == nil {
		if resp, err := c.http.Do(req); err == nil {
			resp.Body.Close()
		}
	}
	return nil
}
