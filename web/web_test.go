package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestHealth(t *testing.T) {
	tracker := health.New()
	s := &Server{Health: tracker}
	if w := get(t, s, "/health"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"healthy":true`) {
		t.Errorf("GET /health = %d %s", w.Code, w.Body)
	}

	tracker.PTTChecked(errors.New("timeout"))
	w := get(t, s, "/health")
	var body struct {
		Healthy  bool
		Problems []string
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != http.StatusServiceUnavailable || body.Healthy || len(body.Problems) != 1 {
		t.Errorf("GET /health = %d %s", w.Code, w.Body)
	}
}

func TestStatusPage(t *testing.T) {
	redistest.Flush()
	user.Register("1", 1)
	user.Update("1", func(u *user.User) error {
		u.Subscribes.AddKeywords("stock", []string{"台積電"})
		return nil
	})
	health.Record(health.Notification{Time: time.Now(), Account: "1", Kind: health.KindKeyword, Board: "stock", Summary: "台積電：[新聞] <b>台積電</b>"})
	tracker := health.New()
	tracker.SetBoards([]string{"stock"})
	tracker.BoardChecked("stock", nil)

	w := get(t, &Server{Health: tracker, Accounts: []string{"1"}}, "/")
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("GET / = %d", w.Code)
	}
	for _, want := range []string{"全部正常", "stock: 台積電", "&lt;b&gt;台積電&lt;/b&gt;", "看板（1）"} {
		if !strings.Contains(body, want) {
			t.Errorf("status page should contain %q", want)
		}
	}
	if w := get(t, &Server{Health: tracker}, "/nope"); w.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d", w.Code)
	}
}
