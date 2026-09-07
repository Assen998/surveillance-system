// Package minio 提供轻量 MinIO（S3 兼容）对象存储客户端，
// 用于录像分段上传、远程清理与回放回退流式播放。
package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client MinIO 客户端（绑定一个 bucket）
type Client struct {
	client *minio.Client
	bucket string
}

// Entry 对象条目（List 结果，扁平对象列表）
type Entry struct {
	Key     string    `json:"key"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// NewClient 创建客户端。endpoint 形如 host:port（不含 scheme）。
func NewClient(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 MinIO 客户端失败: %w", err)
	}
	return &Client{client: mc, bucket: bucket}, nil
}

// EnsureBucket 确保 bucket 存在（不存在则创建）
func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.client.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("检查 bucket 失败: %w", err)
	}
	if !exists {
		if err := c.client.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("创建 bucket 失败: %w", err)
		}
	}
	return nil
}

// Upload 上传本地文件为对象
func (c *Client) Upload(localPath, objectKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_, err := c.client.FPutObject(ctx, c.bucket, objectKey, localPath, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}
	return nil
}

// Stat 查询对象元信息
func (c *Client) Stat(objectKey string) (size int64, modTime time.Time, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := c.client.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, time.Time{}, err
	}
	return info.Size, info.LastModified, nil
}

// Delete 删除对象
func (c *Client) Delete(objectKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.client.RemoveObject(ctx, c.bucket, objectKey, minio.RemoveObjectOptions{})
}

// List 列出前缀下的所有对象（扁平，MinIO 无目录概念）。
// 返回的 Key 为完整对象键（含 base 前缀）。
func (c *Client) List(prefix string) ([]Entry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	prefix = strings.Trim(prefix, "/")
	var entries []Entry
	objCh := c.client.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})
	for obj := range objCh {
		if obj.Err != nil {
			return nil, fmt.Errorf("列对象失败: %w", obj.Err)
		}
		if strings.HasSuffix(obj.Key, "/") {
			continue // 目录占位对象
		}
		entries = append(entries, Entry{Key: obj.Key, Size: obj.Size, ModTime: obj.LastModified})
	}
	return entries, nil
}

// Get 获取对象内容（支持 Range 请求头，用于回放拖动进度）。
// rangeHeader 形如 "bytes=100-199"，为空则取全量。
// 返回读取器与响应元信息（总长、实际范围），调用方负责 Close。
type GetResult struct {
	Body       io.ReadCloser
	TotalSize  int64 // 对象总大小
	Start, End int64 // 本次返回的字节范围（闭区间）
}

func (c *Client) Get(ctx context.Context, objectKey, rangeHeader string) (*GetResult, error) {
	size, _, err := c.Stat(objectKey)
	if err != nil {
		return nil, err
	}

	opts := minio.GetObjectOptions{}
	start, end := int64(0), size-1
	if rangeHeader != "" {
		s, e, ok := parseRange(rangeHeader, size)
		if ok {
			if err := opts.SetRange(s, e); err != nil {
				return nil, fmt.Errorf("解析 Range 失败: %w", err)
			}
			start, end = s, e
		}
	}

	rc, err := c.client.GetObject(ctx, c.bucket, objectKey, opts)
	if err != nil {
		return nil, err
	}
	return &GetResult{Body: rc, TotalSize: size, Start: start, End: end}, nil
}

// parseRange 解析 HTTP Range 头（bytes=a-b / a- / -n），返回闭区间 [start, end]
func parseRange(rangeHeader string, total int64) (start, end int64, ok bool) {
	v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rangeHeader), "bytes="))
	if v == "" {
		return 0, 0, false
	}
	i := strings.Index(v, "-")
	if i < 0 {
		return 0, 0, false
	}
	a, b := v[:i], strings.TrimSpace(v[i+1:])
	switch {
	case a == "" && b == "":
		return 0, 0, false
	case a == "": // 后缀范围：-n（最后 n 字节）
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > total {
			n = total
		}
		return total - n, total - 1, true
	default:
		s, err := strconv.ParseInt(a, 10, 64)
		if err != nil || s < 0 || s >= total {
			return 0, 0, false
		}
		if b == "" {
			return s, total - 1, true
		}
		e, err := strconv.ParseInt(b, 10, 64)
		if err != nil || e < s || e >= total {
			return 0, 0, false
		}
		return s, e, true
	}
}

// TestAndUpload 连接测试：检查/创建 bucket → 写入测试对象 → 读取校验 → 删除
func (c *Client) TestAndUpload(basePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := c.EnsureBucket(ctx); err != nil {
		return err
	}

	key := strings.Trim(strings.TrimSpace(basePath), "/")
	if key != "" {
		key += "/"
	}
	key += ".connect_test"

	payload := []byte(fmt.Sprintf("surveillance-system minio connect test %d", time.Now().Unix()))
	if _, err := c.client.PutObject(ctx, c.bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{}); err != nil {
		return fmt.Errorf("写入测试对象失败: %w", err)
	}
	defer c.client.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})

	rc, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("读取测试对象失败: %w", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil || string(got) != string(payload) {
		return fmt.Errorf("读取测试对象校验失败: %v", err)
	}
	return nil
}
