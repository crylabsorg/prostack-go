package ps_metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Registry struct {
	*prometheus.Registry
}

func New() Registry {
	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewGoCollector())
	r.MustRegister(collectors.NewBuildInfoCollector())
	r.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return Registry{r}
}

func (r Registry) Register(mux *http.ServeMux) {
	handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})
	mux.Handle("GET /metrics", handler)
}
