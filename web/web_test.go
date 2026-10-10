package web

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
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

// fakeActions records calls
type fakeActions struct {
	calls []string
}

func (f *fakeActions) Approve(account string) error {
	f.calls = append(f.calls, "approve "+account)
	return access.Approve(account)
}

func (f *fakeActions) Reject(account string) error {
	f.calls = append(f.calls, "reject "+account)
	return nil
}

func (f *fakeActions) RemoveMember(account string) error {
	f.calls = append(f.calls, "remove "+account)
	return errors.New("不能移除管理員")
}

func (f *fakeActions) EditKeywords(account, action, boards, keywords string) (string, error) {
	f.calls = append(f.calls, action+" "+account+" "+boards+" "+keywords)
	if boards == "nope" {
		return "❌ 板名錯誤", nil
	}
	return "", nil
}

func newServer() (*Server, *string) {
	token := new(string)
	tracker := health.New()
	return &Server{
		Health:  tracker,
		Version: "1.2.3",
		Actions: &fakeActions{},
		Restore: func(r io.Reader) (int, error) {
			b, _ := io.ReadAll(r)
			if string(b) != "backup" {
				return 0, errors.New("invalid backup")
			}
			return 3, nil
		},
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
	for _, want := range []string{"全部正常", "<b>小明</b> 👑", `value="台積電"`, "&lt;b&gt;台積電&lt;/b&gt;", "看板（1）", "待審核（1）", `value="approval" class="on"`, "版本 1.2.3", `<option value="Stock">`} {
		if !strings.Contains(body, want) {
			t.Errorf("status page should contain %q", want)
		}
	}
	if w := get(s, "/nope"); w.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d", w.Code)
	}
}

func postForm(s *Server, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(s, req)
}

func flash(t *testing.T, w *httptest.ResponseRecorder) url.Values {
	t.Helper()
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	u, _ := url.Parse(w.Header().Get("Location"))
	return u.Query()
}

func TestActions(t *testing.T) {
	redistest.Flush()
	access.AddAdmin("1")
	access.Request("2")
	user.Register("2", 2, "小華")
	s, _ := newServer()
	actions := s.Actions.(*fakeActions)

	if q := flash(t, postForm(s, "/members/approve", url.Values{"account": {"2"}})); q.Get("ok") != "已允許 小華" {
		t.Errorf("approve flash = %v", q)
	}
	if q := flash(t, postForm(s, "/members/remove", url.Values{"account": {"1"}})); !strings.Contains(q.Get("err"), "不能移除管理員") {
		t.Errorf("remove flash = %v", q)
	}
	if w := postForm(s, "/members/reject", url.Values{"account": {"x"}}); w.Code != http.StatusBadRequest {
		t.Errorf("invalid account = %d", w.Code)
	}

	if q := flash(t, postForm(s, "/keywords", url.Values{"account": {"2"}, "action": {"新增"}, "board": {"stock"}, "keywords": {"a,b"}})); q.Get("ok") == "" {
		t.Errorf("keywords flash = %v", q)
	}
	if q := flash(t, postForm(s, "/keywords", url.Values{"account": {"2"}, "action": {"新增"}, "board": {"nope"}, "keywords": {"a"}})); q.Get("err") != "❌ 板名錯誤" {
		t.Errorf("keywords problem flash = %v", q)
	}
	if w := postForm(s, "/keywords", url.Values{"account": {"9"}, "action": {"新增"}, "board": {"stock"}, "keywords": {"a"}}); w.Code != http.StatusBadRequest {
		t.Errorf("keywords of non-member = %d", w.Code)
	}
	want := []string{"approve 2", "remove 1", "新增 2 stock a,b", "新增 2 nope a"}
	if strings.Join(actions.calls, ";") != strings.Join(want, ";") {
		t.Errorf("calls = %v", actions.calls)
	}

	flash(t, postForm(s, "/registration", url.Values{"mode": {"open"}}))
	if mode, _ := access.Registration(); mode != access.ModeOpen {
		t.Errorf("mode = %s", mode)
	}
	if w := postForm(s, "/registration", url.Values{"mode": {"x"}}); w.Code != http.StatusBadRequest {
		t.Errorf("invalid mode = %d", w.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/members/approve", strings.NewReader("account=2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	if w := do(s, req); w.Code != http.StatusForbidden {
		t.Errorf("cross site action = %d", w.Code)
	}
}

func upload(s *Server, content string) *httptest.ResponseRecorder {
	var body strings.Builder
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "backup.json")
	fw.Write([]byte(content))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/restore", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return do(s, req)
}

func TestBackupRestore(t *testing.T) {
	redistest.Flush()
	access.AddAdmin("1")
	s, _ := newServer()

	w := get(s, "/backup")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(w.Body.String(), `"access:admins"`) {
		t.Errorf("GET /backup = %d %s", w.Code, w.Body)
	}
	req := httptest.NewRequest(http.MethodGet, "/backup", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if w := do(s, req); w.Code != http.StatusForbidden {
		t.Errorf("cross site backup = %d", w.Code)
	}

	if q := flash(t, upload(s, "backup")); !strings.Contains(q.Get("ok"), "已還原 3 筆") {
		t.Errorf("restore flash = %v", q)
	}
	if q := flash(t, upload(s, "broken")); !strings.Contains(q.Get("err"), "還原失敗") {
		t.Errorf("broken restore flash = %v", q)
	}
}
