package jobs

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

type message struct {
	chatID int64
	text   string
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []message
	err  error
}

func (n *fakeNotifier) Send(chatID int64, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, message{chatID, text})
	return nil
}

func register(t *testing.T, account string, chatID int64, fn func(u *user.User)) {
	t.Helper()
	if err := user.Register(account, chatID); err != nil {
		t.Fatal(err)
	}
	err := user.Update(account, func(u *user.User) error {
		fn(u)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func a(id int, title string) article.Article {
	return article.Article{ID: id, Title: title, Link: "https://www.ptt.cc/bbs/stock/M." + title + ".html"}
}

func TestBoardCheckerNotifiesMatchedKeywords(t *testing.T) {
	redistest.Flush()
	register(t, "1", 11, func(u *user.User) { u.Subscribes.AddKeywords("stock", []string{"台積電", "regexp:^\\[新聞\\]"}) })
	register(t, "2", 22, func(u *user.User) { u.Subscribes.AddKeywords("stock", []string{"聯發科"}) })
	if err := keyword.Sync([]string{"1", "2"}); err != nil {
		t.Fatal(err)
	}

	online := article.Articles{a(100, "[標的] 台積電"), a(101, "[閒聊] 盤中")}
	n := &fakeNotifier{}
	c := &BoardChecker{
		Fetch:    func(string) (article.Articles, error) { return online, nil },
		Notifier: n,
		Health:   health.New(),
	}
	if err := c.Check("stock"); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 0 {
		t.Fatalf("first check only saves base, sent %v", n.sent)
	}

	online = article.Articles{a(0, "(本文已被刪除)"), a(101, "[閒聊] 盤中"), a(102, "[新聞] 台積電法說"), a(103, "[閒聊] 台積電")}
	if err := c.Check("stock"); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 2 {
		t.Fatalf("sent %d messages, want 2: %v", len(n.sent), n.sent)
	}
	for _, m := range n.sent {
		if m.chatID != 11 {
			t.Errorf("message to %d, want 11", m.chatID)
		}
	}
	if !strings.Contains(n.sent[0].text, "[新聞] 台積電法說") || !strings.Contains(n.sent[0].text, "[閒聊] 台積電") {
		t.Errorf("台積電 message = %q", n.sent[0].text)
	}
	if strings.Contains(n.sent[1].text, "[閒聊] 台積電") || !strings.Contains(n.sent[1].text, "[新聞] 台積電法說") {
		t.Errorf("regexp message = %q", n.sent[1].text)
	}
	if recent := health.Recent(); len(recent) != 2 || recent[0].Kind != health.KindKeyword {
		t.Errorf("recent notifications = %v", recent)
	}

	// nothing new
	n.sent = nil
	if err := c.Check("stock"); err != nil || len(n.sent) != 0 {
		t.Errorf("Check() = %v, sent %v", err, n.sent)
	}
}

func TestBoardCheckerFetchError(t *testing.T) {
	redistest.Flush()
	fetchErr := errors.New("timeout")
	c := &BoardChecker{
		Fetch:    func(string) (article.Articles, error) { return nil, fetchErr },
		Notifier: &fakeNotifier{},
		Health:   health.New(),
	}
	if err := c.Check("stock"); err != fetchErr {
		t.Errorf("Check() = %v, want fetch error", err)
	}
}

func trackArticle(t *testing.T, saved article.Article, accounts ...string) {
	t.Helper()
	if err := saved.Save(); err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		register(t, account, 0, func(u *user.User) { u.Subscribes.AddArticle(saved.Board, saved.Code) })
		if err := article.Track(saved.Code, account); err != nil {
			t.Fatal(err)
		}
	}
}

func comments(contents ...string) article.Comments {
	var cs article.Comments
	for _, c := range contents {
		cs = append(cs, article.Comment{Tag: "推 ", UserID: "u", Content: ": " + c})
	}
	return cs
}

func TestCommentCheckerSendsNewCommentsOnce(t *testing.T) {
	redistest.Flush()
	trackArticle(t, article.Article{Code: "M.1.A.1", Board: "stock", Title: "t", Link: "l", CommentCount: 1}, "1")

	online := article.Article{Comments: comments("a", "b", "c")}
	n := &fakeNotifier{}
	c := &CommentChecker{
		FetchArticle: func(board, code string) (article.Article, error) { return online, nil },
		Notifier:     n,
		Health:       health.New(),
	}
	if err := c.Check("M.1.A.1"); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 1 || n.sent[0].chatID != 1 {
		t.Fatalf("sent %v", n.sent)
	}
	if text := n.sent[0].text; strings.Contains(text, ": a") || !strings.Contains(text, ": b") || !strings.Contains(text, ": c") {
		t.Errorf("text = %q", text)
	}

	// same comments in the same minute are not new
	n.sent = nil
	if err := c.Check("M.1.A.1"); err != nil || len(n.sent) != 0 {
		t.Errorf("Check() = %v, sent %v", err, n.sent)
	}

	// broken page with fewer comments must not lower the base
	online = article.Article{}
	if err := c.Check("M.1.A.1"); err != nil {
		t.Fatal(err)
	}
	online = article.Article{Comments: comments("a", "b", "c")}
	if err := c.Check("M.1.A.1"); err != nil || len(n.sent) != 0 {
		t.Errorf("Check() = %v, sent %v", err, n.sent)
	}
}

func TestCommentCheckerLegacyArticleSetsBase(t *testing.T) {
	redistest.Flush()
	legacy := article.Article{Code: "M.2.A.2", Board: "stock", Comments: comments("old")}
	trackArticle(t, legacy, "1")

	n := &fakeNotifier{}
	c := &CommentChecker{
		FetchArticle: func(board, code string) (article.Article, error) {
			return article.Article{Title: "t", Comments: comments("1", "2", "old")}, nil
		},
		Notifier: n,
		Health:   health.New(),
	}
	if err := c.Check("M.2.A.2"); err != nil || len(n.sent) != 0 {
		t.Fatalf("Check() = %v, sent %v", err, n.sent)
	}
	saved, _ := article.Find("M.2.A.2")
	if saved.CommentCount != 3 || len(saved.Comments) != 0 || saved.Title != "t" {
		t.Errorf("saved = %+v", saved)
	}
}

func TestCommentCheckerStopsDeletedArticle(t *testing.T) {
	redistest.Flush()
	trackArticle(t, article.Article{Code: "M.3.A.3", Board: "stock", Title: "t"}, "1", "2")

	n := &fakeNotifier{}
	c := &CommentChecker{
		FetchArticle: func(board, code string) (article.Article, error) {
			return article.Article{}, web.URLNotFoundError{URL: "x"}
		},
		Notifier: n,
		Health:   health.New(),
	}
	if err := c.Check("M.3.A.3"); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 2 || !strings.Contains(n.sent[0].text, "文章已刪除") {
		t.Errorf("sent %v", n.sent)
	}
	if _, ok := article.Find("M.3.A.3"); ok {
		t.Error("article should be destroyed")
	}
	for _, account := range []string{"1", "2"} {
		u, _ := user.Find(account)
		if u.Subscribes.HasArticle("M.3.A.3") {
			t.Errorf("user %s still tracks article", account)
		}
	}
}
