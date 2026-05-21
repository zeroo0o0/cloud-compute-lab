package cluster

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

func LifecycleHandler(next http.Handler, draining *atomic.Bool, activePlayers func() int, beginDrain func(string), preStopDelay time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = fmt.Fprintf(w, "ok active=%d draining=%t\n", safeActivePlayers(activePlayers), draining.Load())
			return
		case "/readyz":
			if draining.Load() {
				http.Error(w, "draining", http.StatusServiceUnavailable)
				return
			}
			_, _ = fmt.Fprintf(w, "ready active=%d\n", safeActivePlayers(activePlayers))
			return
		case "/drain":
			if beginDrain != nil {
				beginDrain("preStop")
			}
			if preStopDelay > 0 {
				time.Sleep(preStopDelay)
			}
			_, _ = fmt.Fprintf(w, "draining active=%d\n", safeActivePlayers(activePlayers))
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}
