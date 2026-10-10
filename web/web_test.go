package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
	"github.com/Ptt-Alertor/ptt-alertor/models/access"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

func newServer() (*Server, *string) {
	token := new(string)
	tracker := health.New()
	return &Server{
		Health:   tracker,
		HasToken: func() bool { return *token != "" },
		SaveToken: func(t string) (string, error) {
			if t != "good" {
				return "", errors.New("Unauthorized")
			}
			*token = t
			tracker.SetBot("my_bot")
			return "my_bot", nil
		},
	}, token
}

func do(s *Server, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func get(s *Server, path string) *httptest.ResponseRecorder {
	return do(s, httptest.NewRequest(http.MethodGet, path, nil))
}

func postToken(s *Server, token, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/setup/token", strings.NewReader(url.Values{"token": {token}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return do(s, req)
}

func TestSetupWizard(t *testing.T) {
	redistest.Flush()
	s, token := newServer()

	if body := get(s, "/").Body.String(); !strings.Contains(body, "BotFather") {
		t.Fatal("step 1 should ask token")
	}
	if w := postToken(s, "good", "https://evil.example"); w.Code != http.StatusForbidden || *token != "" {
		t.Fatalf("cross site post = %d", w.Code)
	}
	if body := postToken(s, "bad", "").Body.String(); !strings.Contains(body, "token 無法使用") {
		t.Fatal("bad token should show error")
	}
	if w := postToken(s, "good", "http://example.com"); w.Code != http.StatusSeeOther || *token != "good" {
		t.Fatalf("post token = %d", w.Code)
	}

	body := get(s, "/").Body.String()
	code, _ := access.ClaimCode()
	if !strings.Contains(body, "https://t.me/my_bot?start="+code) {
		t.Fatalf("step 2 should show claim link: %s", body)
	}

	access.AddAdmin("1")
	if body := get(s, "/").Body.String(); !strings.Contains(body, "https://t.me/my_bot") || !strings.Contains(body, "成員（1）") {
		t.Fatal("status page should follow setup")
	}
}

func TestHealth(t *testing.T) {
	s, _ := newServer()
	s.Health.SetBot("bot")
	if w := get(s, "/health"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"healthy":true`) {
		t.Errorf("GET /health = %d %s", w.Code, w.Body)
	}

	s.Health.PTTChecked(errors.New("timeout"))
	w := get(s, "/health")
	var body struct {
		Healthy  bool
		Problems []string
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != http.StatusServiceUnavailable || body.Healthy || len(body.Problems) != 1 {
		t.Errorf("GET /health = %d %s", w.Code, w.Body)
	}
	if w := get(s, "/healthz"); w.Code != http.StatusOK {
		t.Errorf("GET /healthz = %d", w.Code)
	}
}

func TestStatusPage(t *testing.T) {
	redistest.Flush()
	access.AddAdmin("1")
	user.Register("1", 1, "小明")
	user.Update("1", func(u *user.User) error {
		u.Subscribes.AddKeywords("stock", []string{"台積電"})
		return nil
	})
	access.Request("2")
	health.Record(health.Notification{Time: time.Now(), Account: "1", Kind: health.KindKeyword, Board: "stock", Summary: "台積電：[新聞] <b>台積電</b>"})
	s, token := newServer()
	*token = "good"
	s.Health.SetBot("my_bot")
	s.Health.SetBoards([]string{"stock"})
	s.Health.BoardChecked("stock", nil)

	w := get(s, "/")
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("GET / = %d", w.Code)
	}
	for _, want := range []string{"全部正常", "小明 👑", "stock: 台積電", "&lt;b&gt;台積電&lt;/b&gt;", "看板（1）", "1 人待審核"} {
		if !strings.Contains(body, want) {
			t.Errorf("status page should contain %q", want)
		}
	}
	if w := get(s, "/nope"); w.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d", w.Code)
	}
}
