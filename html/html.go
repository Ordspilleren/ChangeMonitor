package html

import (
	"embed"
	"html/template"
	"io"
	"net/http"
	"strings"

	"github.com/Ordspilleren/ChangeMonitor/monitor"
)

//go:embed *.html
var htmlTemplates embed.FS

//go:embed assets
var assets embed.FS

var (
	monitorList = parse("monitorlist.html")
	monitorNew  = parse("monitornew.html")
)

var funcs = template.FuncMap{
	"StringsJoin": strings.Join,
}

type MonitorListParams struct {
	MonitorService *monitor.MonitorService
}

type MonitorNewParams struct {
	Monitor monitor.Monitor
	Success bool
}

func MonitorList(w io.Writer, p MonitorListParams) error {
	return monitorList.Execute(w, p)
}

func MonitorNew(w io.Writer, p MonitorNewParams) error {
	return monitorNew.Execute(w, p)
}

func parse(file string) *template.Template {
	return template.Must(
		template.New("layout.html").Funcs(funcs).ParseFS(htmlTemplates, "layout.html", file))
}

func GetAssetFS() http.FileSystem {
	return http.FS(assets)
}
