package html

import (
	"embed"
	"html/template"
	"io"
	"net/http"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
)

//go:embed *.html
var htmlTemplates embed.FS

//go:embed assets
var assets embed.FS

var (
	monitorList = parse("monitorlist.html")
)

type MonitorListParams struct {
	Monitors *monitor.Monitors
}

func MonitorList(w io.Writer, p MonitorListParams) error {
	return monitorList.Execute(w, p)
}

func parse(file string) *template.Template {
	return template.Must(
		template.New("layout.html").ParseFS(htmlTemplates, "layout.html", file))
}

func GetAssetFS() http.FileSystem {
	return http.FS(assets)
}
