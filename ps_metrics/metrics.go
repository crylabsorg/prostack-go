package ps_metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//nolint:gochecknoglobals // fine
var registry = func() *prometheus.Registry {
	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewGoCollector())
	r.MustRegister(collectors.NewBuildInfoCollector())
	r.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return r
}()

func GetRegistry() *prometheus.Registry {
	return registry
}

func Register(mux *http.ServeMux) {
	handler := promhttp.HandlerFor(GetRegistry(), promhttp.HandlerOpts{})
	mux.Handle("GET /metrics", handler)
}
