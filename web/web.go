// Package web serves setup wizard, status page with admin controls, and health checks
package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/backup"
	"github.com/Ptt-Alertor/ptt-alertor/command"
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

const (
	// maxRestoreSize limits uploaded backup
	maxRestoreSize = 10 << 20
	// shownNotifications are shown, older ones are folded
	shownNotifications = 5
)

// Actions change members and keywords, telling users on telegram
type Actions interface {
	Approve(account string) error
	Reject(account string) error
	RemoveMember(account string) error
	// EditKeywords returns problem shown to admin when nothing changed
	EditKeywords(account, action, boards, keywords string) (problem string, err error)
}

// Server serves pages
type Server struct {
	Health  *health.Tracker
	Version string
	Actions Actions
	// Restore replaces all data by a backup, returns count of restored keys
	Restore func(r io.Reader) (int, error)
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
	mux.HandleFunc("/backup", s.download)
	mux.HandleFunc("/setup/token", post(s.saveToken))
	mux.HandleFunc("/members/approve", post(s.member("已允許", func(a string) error { return s.Actions.Approve(a) })))
	mux.HandleFunc("/members/reject", post(s.member("已拒絕", func(a string) error { return s.Actions.Reject(a) })))
	mux.HandleFunc("/members/remove", post(s.member("已移除", func(a string) error { return s.Actions.RemoveMember(a) })))
	mux.HandleFunc("/registration", post(s.registration))
	mux.HandleFunc("/keywords", post(s.keywords))
	mux.HandleFunc("/restore", post(s.restore))
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

// sameOrigin rejects cross site form posts, e.g. a web page changing a local server
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

// post accepts same origin POST only, others go back to index
func post(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

// back redirects to status page showing ok or failed message, anchor scrolls to a section
func back(w http.ResponseWriter, r *http.Request, ok, failed, anchor string) {
	q := url.Values{}
	if failed != "" {
		q.Set("err", failed)
	} else {
		q.Set("ok", ok)
	}
	if anchor != "" {
		anchor = "#" + anchor
	}
	http.Redirect(w, r, "/?"+q.Encode()+anchor, http.StatusSeeOther)
}

// failure describes err for admins and logs it
func failure(r *http.Request, what string, err error) string {
	log.WithError(err).WithField("path", r.URL.Path).Error("Web Action Failed")
	return what + "：" + err.Error()
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
	s.status(w, r)
}

func (s *Server) saveToken(w http.ResponseWriter, r *http.Request) {
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

// member runs action on posted account
func (s *Server) member(done string, action func(account string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account := r.FormValue("account")
		if _, err := strconv.ParseInt(account, 10, 64); err != nil {
			http.Error(w, "invalid account", http.StatusBadRequest)
			return
		}
		name := nameOf(account)
		if err := action(account); err != nil {
			back(w, r, "", failure(r, name+" 操作失敗", err), "members")
			return
		}
		back(w, r, done+" "+name, "", "members")
	}
}

func (s *Server) registration(w http.ResponseWriter, r *http.Request) {
	mode := r.FormValue("mode")
	name, ok := access.ModeNames[mode]
	if !ok {
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}
	if err := access.SetRegistration(mode); err != nil {
		back(w, r, "", failure(r, "切換註冊模式失敗", err), "")
		return
	}
	back(w, r, "註冊模式："+name, "", "")
}

func (s *Server) keywords(w http.ResponseWriter, r *http.Request) {
	account, action := r.FormValue("account"), r.FormValue("action")
	if ok, err := access.IsMember(account); err != nil || !ok || (action != "新增" && action != "刪除") {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	board, keywords := r.FormValue("board"), r.FormValue("keywords")
	problem, err := s.Actions.EditKeywords(account, action, board, keywords)
	if err != nil {
		problem = failure(r, action+"失敗", err)
	}
	back(w, r, nameOf(account)+"："+action+" "+board+" "+keywords, problem, "m"+account)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	// backup has the token, only links of this site may download it
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var buf bytes.Buffer
	if err := backup.Export(&buf); err != nil {
		http.Error(w, "備份失敗："+err.Error(), http.StatusInternalServerError)
		return
	}
	name := "ptt-alertor-" + time.Now().Format("20060102-150405") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRestoreSize)
	file, _, err := r.FormFile("file")
	if err != nil {
		back(w, r, "", "請選擇備份檔", "backup")
		return
	}
	defer file.Close()
	n, err := s.Restore(file)
	if err != nil {
		back(w, r, "", failure(r, "還原失敗，資料沒有變更", err), "backup")
		return
	}
	log.WithField("keys", n).Info("Data Restored")
	back(w, r, "已還原 "+strconv.Itoa(n)+" 筆資料。備份裡的 bot token 和目前不同時，請再執行一次 start", "", "")
}

func nameOf(account string) string {
	if u, err := user.Find(account); err == nil {
		return u.DisplayName()
	}
	return account
}

type sub struct {
	Board    string
	Keywords []string
}

type member struct {
	Account  string
	Name     string
	Admin    bool
	Enabled  bool
	Subs     []sub
	Articles int
}

type pending struct {
	Account string
	Name    string
}

type statusPage struct {
	health.Snapshot
	Version       string
	OK            string
	Error         string
	Members       []member
	Pending       []pending
	Mode          string
	Modes         []string
	ModeNames     map[string]string
	BoardOptions  []string
	Notifications []health.Notification
	// Shown is count of notifications shown, older ones are folded
	Shown int
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	page := statusPage{
		Snapshot:      s.Health.Snapshot(),
		Version:       s.Version,
		OK:            r.URL.Query().Get("ok"),
		Error:         r.URL.Query().Get("err"),
		Modes:         []string{access.ModeApproval, access.ModeOpen, access.ModeClosed},
		ModeNames:     access.ModeNames,
		Notifications: health.Recent(),
		Shown:         shownNotifications,
	}
	accounts, _ := access.Members()
	admins, _ := access.Admins()
	pendings, _ := access.Pending()
	page.Mode, _ = access.Registration()
	isAdmin := make(map[string]bool)
	for _, a := range admins {
		isAdmin[a] = true
	}
	// boards suggested by add keyword forms
	options := make(map[string]string)
	for _, b := range command.PopularBoards {
		options[strings.ToLower(b)] = b
	}
	for _, account := range accounts {
		m := member{Account: account, Name: account, Admin: isAdmin[account]}
		if u, err := user.Find(account); err == nil {
			m.Name = u.DisplayName()
			m.Enabled = u.Enable
			m.Articles = u.Subscribes.ArticleCount()
			for _, s := range u.Subscribes.Sorted() {
				if len(s.Keywords) > 0 {
					m.Subs = append(m.Subs, sub{Board: s.Board, Keywords: s.Keywords})
					if _, ok := options[strings.ToLower(s.Board)]; !ok {
						options[strings.ToLower(s.Board)] = s.Board
					}
				}
			}
		}
		page.Members = append(page.Members, m)
	}
	sort.Slice(page.Members, func(i, j int) bool {
		if page.Members[i].Admin != page.Members[j].Admin {
			return page.Members[i].Admin
		}
		return page.Members[i].Name < page.Members[j].Name
	})
	sort.Strings(pendings)
	for _, account := range pendings {
		page.Pending = append(page.Pending, pending{Account: account, Name: nameOf(account)})
	}
	for _, b := range options {
		page.BoardOptions = append(page.BoardOptions, b)
	}
	sort.Slice(page.BoardOptions, func(i, j int) bool {
		return strings.ToLower(page.BoardOptions[i]) < strings.ToLower(page.BoardOptions[j])
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
		Version  string   `json:"version"`
	}{
		Healthy:  snap.Healthy(),
		Problems: snap.Problems,
		Boards:   len(snap.Boards),
		Started:  snap.Started.Format(time.RFC3339),
		Version:  s.Version,
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
