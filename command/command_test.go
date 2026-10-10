package command

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

var boardsOnPTT = map[string]bool{"stock": true, "movie": true, "gossiping": true}

func newHandler() *Handler {
	return &Handler{
		Accounts: []string{"1"},
		Health:   health.New(),
		CheckBoard: func(name string) error {
			if boardsOnPTT[name] {
				return nil
			}
			return board.ErrNotExist
		},
		FetchBoard: func(name string) (article.Articles, error) {
			if name != "stock" {
				return nil, errors.New("HTTP 500")
			}
			return article.Articles{
				{ID: 1, Title: "[標的] 台積電", Link: "l1"},
				{ID: 3, Title: "[新聞] 台積電法說", Link: "l3"},
				{ID: 2, Title: "[閒聊] 盤中", Link: "l2"},
			}, nil
		},
		FetchArticle: func(boardName, code string) (article.Article, error) {
			if code == "M.404.A.1" {
				return article.Article{}, web.URLNotFoundError{URL: code}
			}
			return article.Article{
				ID: 1, Code: code, Board: boardName, Title: "標題", Link: "https://www.ptt.cc/bbs/" + boardName + "/" + code + ".html",
				Comments: article.Comments{{Content: "a"}, {Content: "b"}},
			}, nil
		},
	}
}

func keywordsOf(t *testing.T, account, boardName string) []string {
	t.Helper()
	u, err := user.Find(account)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := u.Subscribes.Find(boardName)
	return s.Keywords
}

func TestHelp(t *testing.T) {
	redistest.Flush()
	h := newHandler()
	for _, text := range []string{"", " ", "指令", "/start", "/help"} {
		if got := h.Handle("1", 1, text); got != Help {
			t.Errorf("Handle(%q) = %q", text, got)
		}
	}
	if got := h.Handle("1", 1, "hello"); !strings.Contains(got, "無此指令") {
		t.Errorf("unknown command reply = %q", got)
	}
	u, err := user.Find("1")
	if err != nil || u.ChatID() != 1 {
		t.Errorf("user should be registered, %+v %v", u, err)
	}
}

func TestAddAndRemoveKeywords(t *testing.T) {
	redistest.Flush()
	h := newHandler()

	got := h.Handle("1", 1, "新增 Stock,movie 台積電，聯發科, 鴻海")
	if !strings.HasPrefix(got, "新增成功") {
		t.Fatalf("reply = %q", got)
	}
	if kws := keywordsOf(t, "1", "stock"); !reflect.DeepEqual(kws, []string{"台積電", "聯發科", "鴻海"}) {
		t.Errorf("stock keywords = %v", kws)
	}
	if subs, _ := keyword.Subscribers("movie"); !reflect.DeepEqual(subs, []string{"1"}) {
		t.Errorf("movie subscribers = %v", subs)
	}
	if !board.Exist("stock") || !board.Exist("movie") {
		t.Errorf("boards = %v", board.List())
	}

	h.Handle("1", 1, "新增 stock regexp:^\\[新聞\\].*,台積電")
	if kws := keywordsOf(t, "1", "stock"); kws[len(kws)-1] != "regexp:^\\[新聞\\].*,台積電" {
		t.Errorf("regexp keyword should not be split: %v", kws)
	}
	if got := h.Handle("1", 1, "新增 stock regexp:[a"); !strings.Contains(got, "正規表示式錯誤") {
		t.Errorf("reply = %q", got)
	}

	h.Handle("1", 1, "刪除 stock 聯發科")
	if kws := keywordsOf(t, "1", "stock"); len(kws) != 3 {
		t.Errorf("stock keywords = %v", kws)
	}
	h.Handle("1", 1, "刪除 movie *")
	if board.Exist("movie") {
		t.Error("board without subscribers should not be checked")
	}
	if subs, _ := keyword.Subscribers("movie"); len(subs) != 0 {
		t.Errorf("movie subscribers = %v", subs)
	}
}

func TestAddKeywordsWrongBoardSavesNothing(t *testing.T) {
	redistest.Flush()
	h := newHandler()
	h.Handle("1", 1, "新增 stock a")

	got := h.Handle("1", 1, "新增 stock,stokc,nope b")
	if !strings.Contains(got, "板名錯誤") || !strings.Contains(got, "stokc（是不是 stock？）") || !strings.Contains(got, "nope") {
		t.Errorf("reply = %q", got)
	}
	if kws := keywordsOf(t, "1", "stock"); !reflect.DeepEqual(kws, []string{"a"}) {
		t.Errorf("stock keywords = %v", kws)
	}
}

func TestKeywordFormatErrors(t *testing.T) {
	redistest.Flush()
	h := newHandler()
	for _, text := range []string{"新增", "新增 stock", "新增 st@ck a"} {
		if got := h.Handle("1", 1, text); !strings.Contains(got, "格式錯誤") {
			t.Errorf("Handle(%q) = %q", text, got)
		}
	}
}

func TestList(t *testing.T) {
	redistest.Flush()
	h := newHandler()
	if got := h.Handle("1", 1, "清單"); !strings.Contains(got, "（無）") {
		t.Errorf("empty list = %q", got)
	}
	h.Handle("1", 1, "新增 stock b,a")
	if got := h.Handle("1", 1, "清單"); !strings.Contains(got, "stock: b, a") || !strings.Contains(got, "推文追蹤 0 篇") {
		t.Errorf("list = %q", got)
	}
}

const articleURL = "https://www.ptt.cc/bbs/Stock/M.1497363598.A.74E.html"

func TestTrackComments(t *testing.T) {
	redistest.Flush()
	h := newHandler()

	if got := h.Handle("1", 1, "新增推文 "+articleURL); got != "新增推文成功：標題" {
		t.Fatalf("reply = %q", got)
	}
	saved, ok := article.Find("M.1497363598.A.74E")
	if !ok || saved.CommentCount != 2 || saved.Board != "Stock" {
		t.Errorf("saved = %+v", saved)
	}
	if subs, _ := article.Subscribers("M.1497363598.A.74E"); !reflect.DeepEqual(subs, []string{"1"}) {
		t.Errorf("subscribers = %v", subs)
	}
	if got := h.Handle("1", 1, "新增推文 "+articleURL); !strings.Contains(got, "已在追蹤") {
		t.Errorf("reply = %q", got)
	}
	if got := h.Handle("1", 1, "推文清單"); !strings.Contains(got, articleURL) || !strings.Contains(got, "（1/50）") {
		t.Errorf("list = %q", got)
	}

	if got := h.Handle("1", 1, "刪除推文 "+articleURL); got != "刪除推文成功。" {
		t.Errorf("reply = %q", got)
	}
	if _, ok := article.Find("M.1497363598.A.74E"); ok {
		t.Error("article nobody tracks should be destroyed")
	}
	if got := h.Handle("1", 1, "推文清單"); !strings.Contains(got, "尚未追蹤") {
		t.Errorf("list = %q", got)
	}
}

func TestTrackCommentsErrors(t *testing.T) {
	redistest.Flush()
	h := newHandler()
	if got := h.Handle("1", 1, "新增推文 https://www.ptt.cc/bbs/Stock/M.404.A.1.html"); got != "文章不存在。" {
		t.Errorf("reply = %q", got)
	}
	if got := h.Handle("1", 1, "新增推文 https://example.com"); !strings.Contains(got, "格式錯誤") {
		t.Errorf("reply = %q", got)
	}
}

func TestCleanArticles(t *testing.T) {
	redistest.Flush()
	h := newHandler()
	h.Handle("1", 1, "新增推文 "+articleURL)
	h.Handle("1", 1, "新增推文 https://www.ptt.cc/bbs/joke/M.1.A.1.html")
	article.Destroy("M.1.A.1")

	if got := h.Handle("1", 1, "清理推文"); got != "清理 1 則推文追蹤。" {
		t.Errorf("reply = %q", got)
	}
	u, _ := user.Find("1")
	if u.Subscribes.ArticleCount() != 1 || !u.Subscribes.HasArticle("M.1497363598.A.74E") {
		t.Errorf("subscriptions = %v", u.Subscribes)
	}
}

func TestTestBoard(t *testing.T) {
	redistest.Flush()
	h := newHandler()
	if got := h.Handle("1", 1, "測試 stock"); !strings.Contains(got, "共 3 篇") || !strings.Contains(got, "沒有設定關鍵字") {
		t.Errorf("reply = %q", got)
	}

	h.Handle("1", 1, "新增 stock 台積電,鴻海")
	if err := board.SaveArticles("stock", article.Articles{{ID: 1}}); err != nil {
		t.Fatal(err)
	}
	got := h.Handle("1", 1, "測試 Stock")
	for _, want := range []string{"最新：\n[新聞] 台積電法說", "「台積電」命中 2 篇", "「鴻海」命中 0 篇"} {
		if !strings.Contains(got, want) {
			t.Errorf("reply %q should contain %q", got, want)
		}
	}
	if saved, _ := board.SavedArticles("stock"); len(saved) != 1 {
		t.Errorf("測試 must not change saved articles: %v", saved)
	}

	if got := h.Handle("1", 1, "測試 movie"); !strings.Contains(got, "❌ movie 抓取失敗：HTTP 500") {
		t.Errorf("reply = %q", got)
	}
}

func TestStatus(t *testing.T) {
	redistest.Flush()
	h := newHandler()
	h.Health.SetBoards([]string{"stock"})
	h.Health.BoardChecked("stock", nil)
	got := h.Handle("1", 1, "狀態")
	for _, want := range []string{"✅ 全部正常", "看板（1）", "✅ stock"} {
		if !strings.Contains(got, want) {
			t.Errorf("status %q should contain %q", got, want)
		}
	}
}
