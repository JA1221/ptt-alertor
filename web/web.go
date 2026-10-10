// Package web serves setup wizard, status page and health checks
package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/access"
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

var pages = template.Must(template.New("").Funcs(funcs).ParseFS(templates, "templates/*.html"))

// Server serves pages
type Server struct {
	Health *health.Tracker
	// HasToken reports telegram token is configured
	HasToken func() bool
	// SaveToken verifies and saves telegram token, returns bot user name
	SaveToken func(token string) (string, error)
}

// Handler routes requests
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthCheck)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/setup/token", s.saveToken)
	mux.HandleFunc("/", s.index)
	return mux
}

func render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		log.WithError(err).WithField("page", name).Error("Render Page Failed")
	}
}

type setupPage struct {
	Step      int
	Error     string
	Bot       string
	ClaimLink string
	ClaimCode string
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !s.HasToken() {
		render(w, "setup.html", setupPage{Step: 1})
		return
	}
	admins, err := access.Admins()
	if err != nil {
		http.Error(w, "Redis 無法連線："+err.Error(), http.StatusServiceUnavailable)
		return
	}
	bot := s.Health.Snapshot().Bot
	if len(admins) == 0 {
		page := setupPage{Step: 2, Bot: bot}
		if bot != "" {
			code, err := access.ClaimCode()
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			page.ClaimCode = code
			page.ClaimLink = "https://t.me/" + bot + "?start=" + code
		}
		render(w, "setup.html", page)
		return
	}
	s.status(w, bot)
}

// sameOrigin rejects cross site form posts, e.g. a web page setting token of a local server
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func (s *Server) saveToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if s.HasToken() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	if token == "" {
		render(w, "setup.html", setupPage{Step: 1, Error: "請貼上 token。"})
		return
	}
	bot, err := s.SaveToken(token)
	if err != nil {
		render(w, "setup.html", setupPage{Step: 1, Error: "token 無法使用：" + err.Error()})
		return
	}
	log.WithField("bot", bot).Info("Telegram Token Saved")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type member struct {
	Account  string
	Name     string
	Admin    bool
	Enabled  bool
	Keywords string
	Articles int
}

type statusPage struct {
	health.Snapshot
	Members       []member
	Pending       int
	Mode          string
	Notifications []health.Notification
}

var modeNames = map[string]string{access.ModeApproval: "審核", access.ModeOpen: "開放", access.ModeClosed: "關閉"}

func (s *Server) status(w http.ResponseWriter, bot string) {
	page := statusPage{
		Snapshot:      s.Health.Snapshot(),
		Notifications: health.Recent(),
	}
	accounts, _ := access.Members()
	admins, _ := access.Admins()
	pending, _ := access.Pending()
	mode, _ := access.Registration()
	page.Pending = len(pending)
	page.Mode = modeNames[mode]
	isAdmin := make(map[string]bool)
	for _, a := range admins {
		isAdmin[a] = true
	}
	for _, account := range accounts {
		m := member{Account: account, Name: account, Admin: isAdmin[account]}
		if u, err := user.Find(account); err == nil {
			m.Name = u.DisplayName()
			m.Enabled = u.Enable
			m.Keywords = u.Subscribes.StringKeywords()
			m.Articles = u.Subscribes.ArticleCount()
		}
		page.Members = append(page.Members, m)
	}
	sort.Slice(page.Members, func(i, j int) bool {
		if page.Members[i].Admin != page.Members[j].Admin {
			return page.Members[i].Admin
		}
		return page.Members[i].Name < page.Members[j].Name
	})
	render(w, "status.html", page)
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
