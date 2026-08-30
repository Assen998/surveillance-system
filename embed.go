// Package embedui 将前端构建产物（web/dist）嵌入到单二进制中，
// 使发布产物无需额外携带 web/dist 目录。
package embedui

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var dist embed.FS

// Dist 返回以 web/dist 为根的文件系统，用于静态资源与 SPA 回退。
func Dist() (fs.FS, error) {
	return fs.Sub(dist, "web/dist")
}