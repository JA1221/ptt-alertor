// Package web serves status page and health check
package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

//go:embed templates/*.html
var templates embed.FS

var funcs = template.FuncMap{
	"ago":      health.Ago,
	"duration": health.Duration,
	"since":    func(now, t time.Time) time.Duration { return now.Sub(t) },
	"clock": func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.Local().Format("01/02 15:04:05")
	},
}

var statusTpl = template.Must(template.New("status.html").Funcs(funcs).ParseFS(templates, "templates/status.html"))

// Server serves status of tracker
type Server struct {
	Health *health.Tracker
	// Accounts are allowed users shown in subscriptions
	Accounts []string
}

// Handler routes requests
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthCheck)
	mux.HandleFunc("/", s.status)
	return mux
}

type subscriber struct {
	Account  string
	Keywords string
	Articles int
}

type statusPage struct {
	health.Snapshot
	Subscribers   []subscriber
	Notifications []health.Notification
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page := statusPage{
		Snapshot:      s.Health.Snapshot(),
		Notifications: health.Recent(),
	}
	for _, account := range s.Accounts {
		u, err := user.Find(account)
		if err != nil {
			continue
		}
		page.Subscribers = append(page.Subscribers, subscriber{
			Account:  account,
			Keywords: u.Subscribes.StringKeywords(),
			Articles: u.Subscribes.ArticleCount(),
		})
	}
	sort.Slice(page.Subscribers, func(i, j int) bool { return page.Subscribers[i].Account < page.Subscribers[j].Account })
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := statusTpl.Execute(w, page); err != nil {
		log.WithError(err).Error("Render Status Page Failed")
	}
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	snap := s.Health.Snapshot()
	body := struct {
		Healthy  bool     `json:"healthy"`
		Problems []string `json:"problems"`
		Boards   int      `json:"boards"`
		Started  string   `json:"started"`
	}{
		Healthy:  snap.Healthy(),
		Problems: snap.Problems,
		Boards:   len(snap.Boards),
		Started:  snap.Started.Format(time.RFC3339),
	}
	if body.Problems == nil {
		body.Problems = []string{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !body.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(body)
}
