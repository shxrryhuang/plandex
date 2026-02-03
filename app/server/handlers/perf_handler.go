package handlers

import (
	"fmt"
	"net/http"
	"os"

	"plandex-server/perf"
)

// PerfMetricsHandler serves the live metrics report as plain text.
// It is disabled in production; in development / self-hosted it requires
// no authentication so that operators can curl it without tokens.
func PerfMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("GOENV") == "production" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, perf.Report())
}
