package report

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"time"

	"openlist/internal/duplicate"
)

// ReportData holds all data needed to render the HTML report.
type ReportData struct {
	GeneratedAt    string
	MovieGroups    []duplicate.DuplicateGroup
	TVGroups       []duplicate.DuplicateGroup
	Stats          duplicate.Stats
	StorageStats   []StorageStat
}

// StorageStat holds file statistics for a single storage provider.
type StorageStat struct {
	Name       string
	FileCount  int
	TotalSize  int64
	DupeSize   int64
	Percentage float64
}

// Generate creates the HTML report file at the given path.
func Generate(path string, data ReportData) error {
	if data.GeneratedAt == "" {
		data.GeneratedAt = time.Now().Format("2006-01-02 15:04:05")
	}

	// Separate movie and TV groups
	for _, g := range data.MovieGroups {
		if g.IsEpisode {
			data.TVGroups = append(data.TVGroups, g)
		}
	}
	// Filter — only keep non-episode groups as movie groups
	var movieOnly []duplicate.DuplicateGroup
	for _, g := range data.MovieGroups {
		if !g.IsEpisode {
			movieOnly = append(movieOnly, g)
		}
	}
	data.MovieGroups = movieOnly

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"formatSize": formatSize,
		"formatInt":  formatInt,
		"decisionLabel": func(d duplicate.Decision) string {
			if d == duplicate.Keep {
				return "keep"
			}
			if d == duplicate.Delete {
				return "delete"
			}
			return "unique"
		},
		"sub": func(a, b int) int { return a - b },
	}).Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return nil
}

// formatSize converts bytes to a human-readable string.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// formatInt formats an int64 with comma separators.
func formatInt(n int) string {
	if n < 1000 {
		return itoa(n)
	}
	s := ""
	for n > 0 {
		if len(s)%4 == 3 {
			s = "," + s
		}
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// CountDuplicateFiles returns the number of files marked as Delete.
func CountDuplicateFiles(data ReportData) int {
	return data.Stats.DeleteFiles
}

const reportTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>OpenList 媒体去重报告</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #f5f5f5; color: #333; padding: 20px; }
.container { max-width: 1200px; margin: 0 auto; }
h1 { font-size: 24px; margin-bottom: 8px; }
.header { margin-bottom: 24px; }
.header .meta { color: #666; font-size: 14px; }
.card { background: #fff; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); padding: 20px; margin-bottom: 20px; }
.card h2 { font-size: 18px; margin-bottom: 16px; padding-bottom: 8px; border-bottom: 2px solid #eee; }
.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; }
.stat-card { text-align: center; padding: 16px; background: #fafafa; border-radius: 6px; }
.stat-card .value { font-size: 28px; font-weight: 700; color: #2196F3; }
.stat-card .label { font-size: 13px; color: #666; margin-top: 4px; }
.stat-card.warn .value { color: #f44336; }
.stat-card.ok .value { color: #4CAF50; }
table { width: 100%; border-collapse: collapse; font-size: 14px; }
th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid #eee; }
th { background: #fafafa; font-weight: 600; color: #555; }
tr:hover { background: #f8f9fa; }
.badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }
.badge-keep { background: #e8f5e9; color: #2e7d32; }
.badge-delete { background: #ffebee; color: #c62828; }
.badge-unique { background: #e3f2fd; color: #1565c0; }
.empty { text-align: center; padding: 40px; color: #999; }
.bar-container { position: relative; height: 20px; background: #eee; border-radius: 4px; overflow: hidden; min-width: 100px; }
.bar-fill { height: 100%; background: linear-gradient(90deg, #42a5f5, #2196F3); border-radius: 4px; transition: width 0.3s; }
.bar-label { position: absolute; top: 50%; left: 8px; transform: translateY(-50%); font-size: 11px; font-weight: 600; color: #333; }
@media (max-width: 768px) {
  table { font-size: 12px; }
  th, td { padding: 6px 8px; }
  .stats-grid { grid-template-columns: repeat(2, 1fr); }
}
</style>
</head>
<body>
<div class="container">

<div class="header">
<h1>🎬 OpenList 媒体去重报告</h1>
<div class="meta">生成时间: {{.GeneratedAt}} | 总计: {{formatInt .Stats.TotalFiles}} 个文件</div>
</div>

<div class="card">
<h2>📊 统计概览</h2>
<div class="stats-grid">
  <div class="stat-card">
    <div class="value">{{formatInt .Stats.TotalFiles}}</div>
    <div class="label">扫描文件</div>
  </div>
  <div class="stat-card ok">
    <div class="value">{{formatInt .Stats.UniqueFiles}}</div>
    <div class="label">唯一文件</div>
  </div>
  <div class="stat-card warn">
    <div class="value">{{formatInt .Stats.DuplicateFiles}}</div>
    <div class="label">重复文件</div>
  </div>
  <div class="stat-card warn">
    <div class="value">{{formatInt .Stats.DuplicateSets}}</div>
    <div class="label">重复组数</div>
  </div>
  <div class="stat-card ok">
    <div class="value">{{formatSize .Stats.DuplicateSize}}</div>
    <div class="label">可节省空间</div>
  </div>
</div>
</div>

{{if .StorageStats}}
<div class="card">
<h2>💾 存储统计</h2>
<table>
<thead>
<tr><th>存储</th><th>文件数</th><th>总大小</th><th>重复大小</th><th>占比</th></tr>
</thead>
<tbody>
{{range $s := .StorageStats}}
<tr>
  <td><strong>{{$s.Name}}</strong></td>
  <td>{{formatInt $s.FileCount}}</td>
  <td>{{formatSize $s.TotalSize}}</td>
  <td>{{formatSize $s.DupeSize}}</td>
  <td>
    <div class="bar-container">
      <div class="bar-fill" style="width: {{printf "%.1f" $s.Percentage}}%"></div>
      <span class="bar-label">{{printf "%.1f" $s.Percentage}}%</span>
    </div>
  </td>
</tr>
{{end}}
</tbody>
</table>
</div>
{{end}}

{{if .MovieGroups}}
<div class="card">
<h2>🎬 重复电影 ({{len .MovieGroups}} 组)</h2>
{{range $g := .MovieGroups}}
<h3 style="margin: 16px 0 8px; font-size: 15px;">📁 {{$g.NormalizedName}}</h3>
<table>
<thead>
<tr><th>操作</th><th>存储</th><th>路径</th><th>大小</th></tr>
</thead>
<tbody>
{{range $g.Files}}
<tr>
  <td><span class="badge badge-{{decisionLabel .Decision}}">{{.Decision}}</span></td>
  <td>{{.Storage}}</td>
  <td><code>{{.Path}}</code></td>
  <td>{{formatSize .Size}}</td>
</tr>
{{end}}
</tbody>
</table>
{{end}}
</div>
{{end}}

{{if .TVGroups}}
<div class="card">
<h2>📺 重复剧集 ({{len .TVGroups}} 组)</h2>
{{range $g := .TVGroups}}
<h3 style="margin: 16px 0 8px; font-size: 15px;">📁 {{$g.NormalizedName}} {{$g.EpisodeTag}}</h3>
<table>
<thead>
<tr><th>操作</th><th>存储</th><th>路径</th><th>大小</th></tr>
</thead>
<tbody>
{{range $g.Files}}
<tr>
  <td><span class="badge badge-{{decisionLabel .Decision}}">{{.Decision}}</span></td>
  <td>{{.Storage}}</td>
  <td><code>{{.Path}}</code></td>
  <td>{{formatSize .Size}}</td>
</tr>
{{end}}
</tbody>
</table>
{{end}}
</div>
{{end}}

{{if not .MovieGroups}}{{if not .TVGroups}}
<div class="card"><div class="empty">✅ 未发现重复资源</div></div>
{{end}}{{end}}

</div>
</body>
</html>`
