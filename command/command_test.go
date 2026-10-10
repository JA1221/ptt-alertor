package command

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
	"github.com/Ptt-Alertor/ptt-alertor/models/access"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

type sent struct {
	chatID int64
	msg    notify.Message
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []sent
}

func (n *fakeNotifier) Send(chatID int64, msg notify.Message) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, sent{chatID, msg})
	return nil
}

var boardsOnPTT = map[string]bool{"stock": true, "movie": true, "gossiping": true}

// setup resets data, "1" is admin
func setup(t *testing.T) (*Handler, *fakeNotifier) {
	t.Helper()
	redistest.Flush()
	if err := access.AddAdmin("1"); err != nil {
		t.Fatal(err)
	}
	n := &fakeNotifier{}
	return &Handler{
		Health:   health.New(),
		Notifier: n,
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
				ID: 1, Code: code, Board: boardName, Title: "標題<b>", Link: "https://www.ptt.cc/bbs/" + boardName + "/" + code + ".html",
				Comments: article.Comments{{Content: "a"}, {Content: "b"}},
			}, nil
		},
	}, n
}

func say(h *Handler, account, text string) notify.Message {
	chatID := int64(len(account))
	return h.Handle(Input{Account: account, ChatID: chatID, Name: "name" + account, Text: text})
}

func press(h *Handler, account, data string) notify.Message {
	return h.Handle(Input{Account: account, ChatID: int64(len(account)), Text: data, Callback: true})
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

func hasButton(msg notify.Message, data string) bool {
	for _, row := range msg.Buttons {
		for _, b := range row {
			if b.Data == data {
				return true
			}
		}
	}
	return false
}

func TestHelp(t *testing.T) {
	h, _ := setup(t)
	for _, text := range []string{"", "指令", "/help", "❓ 指令"} {
		if got := say(h, "1", text); got.Text != Help().Text || !got.Keyboard {
			t.Errorf("Handle(%q) = %q", text, got.Text)
		}
	}
	if got := say(h, "1", "/start"); !strings.Contains(got.Text, "歡迎") {
		t.Errorf("/start = %q", got.Text)
	}
	if got := say(h, "1", "hello"); !strings.Contains(got.Text, "看不懂") {
		t.Errorf("unknown command reply = %q", got.Text)
	}
	if u, err := user.Find("1"); err != nil || u.ChatID() != 1 || u.DisplayName() != "name1" {
		t.Errorf("user should be registered, %+v %v", u, err)
	}
}

func TestStrangerApproval(t *testing.T) {
	h, n := setup(t)
	got := say(h, "22", "hi")
	if !strings.Contains(got.Text, "已送出使用申請") {
		t.Fatalf("reply = %q", got.Text)
	}
	if len(n.sent) != 1 || n.sent[0].chatID != 1 || !hasButton(n.sent[0].msg, "ap|22") || !strings.Contains(n.sent[0].msg.Text, "name22") {
		t.Fatalf("admin request = %+v", n.sent)
	}
	if got := say(h, "22", "清單"); !strings.Contains(got.Text, "審核中") {
		t.Errorf("reply = %q", got.Text)
	}
	if len(n.sent) != 1 {
		t.Error("admins are asked once")
	}

	if got := press(h, "22", "ap|22"); !strings.Contains(got.Text, "審核中") {
		t.Errorf("non-member pressing approve = %q", got.Text)
	}
	got = press(h, "1", "ap|22")
	if !got.Edit || !strings.Contains(got.Text, "已允許 name22") {
		t.Errorf("approve reply = %+v", got)
	}
	if ok, _ := access.IsMember("22"); !ok {
		t.Error("22 should be member")
	}
	if last := n.sent[len(n.sent)-1]; last.chatID != 2 || !strings.Contains(last.msg.Text, "已同意") {
		t.Errorf("approved user message = %+v", last)
	}
	if got := say(h, "22", "使用者"); !strings.Contains(got.Text, "只有管理員") {
		t.Errorf("member using admin command = %q", got.Text)
	}
}

func TestStrangerReject(t *testing.T) {
	h, n := setup(t)
	say(h, "22", "hi")
	if got := press(h, "1", "rj|22"); !strings.Contains(got.Text, "已拒絕") {
		t.Errorf("reject reply = %q", got.Text)
	}
	if ok, _ := access.IsMember("22"); ok {
		t.Error("22 should not be member")
	}
	if last := n.sent[len(n.sent)-1]; !strings.Contains(last.msg.Text, "沒有同意") {
		t.Errorf("rejected user message = %+v", last)
	}
}

func TestRegistrationModes(t *testing.T) {
	h, _ := setup(t)
	if got := say(h, "1", "註冊 關閉"); !strings.Contains(got.Text, "註冊模式：關閉") {
		t.Fatalf("reply = %q", got.Text)
	}
	if got := say(h, "33", "hi"); !strings.Contains(got.Text, "不開放") {
		t.Errorf("closed reply = %q", got.Text)
	}
	say(h, "1", "註冊 開放")
	if got := say(h, "33", "hi"); !strings.Contains(got.Text, "歡迎") {
		t.Errorf("open reply = %q", got.Text)
	}
	if ok, _ := access.IsMember("33"); !ok {
		t.Error("33 should be member")
	}
}

func TestNoAdminAndClaim(t *testing.T) {
	h, _ := setup(t)
	access.Remove("1")
	if got := say(h, "1", "hi"); !strings.Contains(got.Text, "還沒有管理員") {
		t.Errorf("reply = %q", got.Text)
	}
	code, _ := access.ClaimCode()
	if got := say(h, "1", "/start wrong"); !strings.Contains(got.Text, "還沒有管理員") {
		t.Errorf("wrong code reply = %q", got.Text)
	}
	if got := say(h, "1", "/start "+code); !strings.Contains(got.Text, "管理員") || !got.Keyboard {
		t.Errorf("claim reply = %q", got.Text)
	}
	if ok, _ := access.IsAdmin("1"); !ok {
		t.Error("1 should be admin")
	}
}

func TestUsersAndRemove(t *testing.T) {
	h, _ := setup(t)
	access.Approve("22")
	say(h, "22", "新增 stock a")
	say(h, "44", "hi")
	got := say(h, "1", "使用者")
	for _, want := range []string{"成員</b>（2）", "name22（22）看板 1", "待審核</b>（1）", "註冊模式：審核"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("users %q should contain %q", got.Text, want)
		}
	}
	if !hasButton(got, "rm|22") || hasButton(got, "rm|1") || !hasButton(got, "ap|44") {
		t.Errorf("buttons = %+v", got.Buttons)
	}

	got = press(h, "1", "rm|22")
	if !got.Edit || strings.Contains(got.Text, "name22") {
		t.Errorf("after remove = %q", got.Text)
	}
	if ok, _ := access.IsMember("22"); ok {
		t.Error("22 should be removed")
	}
	if board.Exist("stock") {
		t.Error("boards of removed member should not be checked")
	}
	if got := press(h, "1", "rm|1"); !strings.Contains(got.Text, "不能移除管理員") {
		t.Errorf("remove admin = %q", got.Text)
	}
}

func TestAddAndRemoveKeywords(t *testing.T) {
	h, _ := setup(t)

	got := say(h, "1", "新增 Stock,movie 台積電，聯發科, 鴻海")
	if !strings.HasPrefix(got.Text, "✅ 新增成功") || !strings.Contains(got.Text, "<b>stock</b>：台積電、聯發科、鴻海") {
		t.Fatalf("reply = %q", got.Text)
	}
	if !hasButton(got, "bv|stock") || !hasButton(got, "bv|movie") || !hasButton(got, "新增") {
		t.Errorf("buttons = %+v", got.Buttons)
	}
	if subs, _ := keyword.Subscribers("movie"); !reflect.DeepEqual(subs, []string{"1"}) {
		t.Errorf("movie subscribers = %v", subs)
	}
	if !board.Exist("stock") || !board.Exist("movie") {
		t.Errorf("boards = %v", board.List())
	}

	say(h, "1", "新增 stock regexp:^\\[新聞\\].*,台積電")
	if kws := keywordsOf(t, "1", "stock"); kws[len(kws)-1] != "regexp:^\\[新聞\\].*,台積電" {
		t.Errorf("regexp keyword should not be split: %v", kws)
	}
	if got := say(h, "1", "新增 stock regexp:[a"); !strings.Contains(got.Text, "正規表示式錯誤") {
		t.Errorf("reply = %q", got.Text)
	}

	// board view lists keywords with delete buttons, deleting refreshes it
	got = press(h, "1", "bv|stock")
	if !got.Edit || !strings.Contains(got.Text, "<b>stock</b> 的關鍵字（4/20）") || !hasButton(got, "dk|stock|聯發科") {
		t.Fatalf("board view = %+v", got)
	}
	got = press(h, "1", "dk|stock|聯發科")
	if !got.Edit || !strings.Contains(got.Text, "台積電、鴻海") || hasButton(got, "dk|stock|聯發科") {
		t.Errorf("after delete = %+v", got)
	}

	// delete whole board from board view asks first, "no" goes back
	got = press(h, "1", "db|movie")
	if !got.Edit || !hasButton(got, "刪除 movie *") || !hasButton(got, "bv|movie") {
		t.Fatalf("delete board = %+v", got)
	}

	got = say(h, "1", "刪除 movie *")
	if !hasButton(got, "刪除 movie *") || board.Exist("movie") == false {
		t.Fatalf("delete all should ask first: %+v", got)
	}
	press(h, "1", "刪除 movie *")
	if board.Exist("movie") {
		t.Error("board without subscribers should not be checked")
	}
	if got := press(h, "1", "cancel"); got.Text != "已取消。" || !got.Edit {
		t.Errorf("cancel = %+v", got)
	}
}

func TestKeywordLimits(t *testing.T) {
	h, _ := setup(t)
	var kws []string
	for i := 0; i <= MaxKeywords; i++ {
		kws = append(kws, string(rune('a'+i)))
	}
	if got := say(h, "1", "新增 stock "+strings.Join(kws, ",")); !strings.Contains(got.Text, "最多") {
		t.Errorf("reply = %q", got.Text)
	}
	if kws := keywordsOf(t, "1", "stock"); len(kws) != 0 {
		t.Errorf("nothing should be saved: %v", kws)
	}
}

func TestAddKeywordsWrongBoardSavesNothing(t *testing.T) {
	h, _ := setup(t)
	say(h, "1", "新增 stock a")

	got := say(h, "1", "新增 stock,stokc b")
	if !strings.Contains(got.Text, "板名錯誤") || !strings.Contains(got.Text, "stokc（是不是 stock？）") {
		t.Errorf("reply = %q", got.Text)
	}
	if !hasButton(got, "ak|stock,stock|b") {
		t.Errorf("buttons = %+v", got.Buttons)
	}
	if kws := keywordsOf(t, "1", "stock"); !reflect.DeepEqual(kws, []string{"a"}) {
		t.Errorf("stock keywords = %v", kws)
	}
	if got := say(h, "1", "新增 nope b"); hasButton(got, "ak|nope|b") || len(got.Buttons) != 0 {
		t.Errorf("no suggestion, no button: %+v", got.Buttons)
	}

	got = press(h, "1", "ak|stock,stock|b")
	if !got.Edit || !strings.Contains(got.Text, "新增成功") {
		t.Errorf("retry = %+v", got)
	}
}

func TestGuess(t *testing.T) {
	h, _ := setup(t)
	got := say(h, "1", "stock 台積電")
	if !hasButton(got, "ak|stock|台積電") {
		t.Fatalf("guess = %+v", got)
	}
	press(h, "1", "ak|stock|台積電")
	if kws := keywordsOf(t, "1", "stock"); !reflect.DeepEqual(kws, []string{"台積電"}) {
		t.Errorf("keywords = %v", kws)
	}

	got = say(h, "1", "看這篇 https://www.ptt.cc/bbs/Stock/M.1497363598.A.74E.html")
	if !hasButton(got, "ta|Stock|M.1497363598.A.74E") {
		t.Fatalf("guess = %+v", got)
	}
	if got := press(h, "1", "ta|Stock|M.1497363598.A.74E"); !strings.Contains(got.Text, "開始追蹤") || !strings.Contains(got.Text, "標題&lt;b&gt;") {
		t.Errorf("track = %q", got.Text)
	}
}

func TestKeywordFormatErrors(t *testing.T) {
	h, _ := setup(t)
	for _, text := range []string{"刪除", "新增 stock", "新增 st@ck a"} {
		if got := say(h, "1", text); !strings.Contains(got.Text, "格式錯誤") {
			t.Errorf("Handle(%q) = %q", text, got.Text)
		}
	}
}

func TestList(t *testing.T) {
	h, _ := setup(t)
	if got := say(h, "1", "📋 清單"); !strings.Contains(got.Text, "還沒有關鍵字") {
		t.Errorf("empty list = %q", got.Text)
	}
	say(h, "1", "新增 stock b,a<i>")
	got := say(h, "1", "/list")
	if !strings.Contains(got.Text, "<b>stock</b>：b、a&lt;i&gt;") || !strings.Contains(got.Text, "推文追蹤 0 篇") {
		t.Errorf("list = %q", got.Text)
	}
}

const articleURL = "https://www.ptt.cc/bbs/Stock/M.1497363598.A.74E.html"

func TestTrackComments(t *testing.T) {
	h, _ := setup(t)

	if got := say(h, "1", "新增推文 "+articleURL); !strings.Contains(got.Text, "開始追蹤") {
		t.Fatalf("reply = %q", got.Text)
	}
	saved, ok := article.Find("M.1497363598.A.74E")
	if !ok || saved.CommentCount != 2 || saved.Board != "Stock" {
		t.Errorf("saved = %+v", saved)
	}
	if subs, _ := article.Subscribers("M.1497363598.A.74E"); !reflect.DeepEqual(subs, []string{"1"}) {
		t.Errorf("subscribers = %v", subs)
	}
	if got := say(h, "1", "新增推文 "+articleURL); !strings.Contains(got.Text, "已在追蹤") {
		t.Errorf("reply = %q", got.Text)
	}
	got := say(h, "1", "推文清單")
	if !strings.Contains(got.Text, "（1/50）") || !strings.Contains(got.Text, `href="`+articleURL) || !hasButton(got, "da|M.1497363598.A.74E") {
		t.Errorf("list = %+v", got)
	}

	got = press(h, "1", "da|M.1497363598.A.74E")
	if !got.Edit || !strings.Contains(got.Text, "還沒有追蹤") {
		t.Errorf("after delete = %+v", got)
	}
	if _, ok := article.Find("M.1497363598.A.74E"); ok {
		t.Error("article nobody tracks should be destroyed")
	}
	if got := say(h, "1", "刪除推文 "+articleURL); !strings.Contains(got.Text, "沒有追蹤") {
		t.Errorf("reply = %q", got.Text)
	}
}

func TestTrackCommentsErrors(t *testing.T) {
	h, _ := setup(t)
	if got := say(h, "1", "新增推文 https://www.ptt.cc/bbs/Stock/M.404.A.1.html"); got.Text != "文章不存在。" {
		t.Errorf("reply = %q", got.Text)
	}
	if got := say(h, "1", "新增推文 https://example.com"); !strings.Contains(got.Text, "格式錯誤") {
		t.Errorf("reply = %q", got.Text)
	}
}

func TestCleanArticles(t *testing.T) {
	h, _ := setup(t)
	say(h, "1", "新增推文 "+articleURL)
	say(h, "1", "新增推文 https://www.ptt.cc/bbs/joke/M.1.A.1.html")
	article.Destroy("M.1.A.1")

	if got := say(h, "1", "清理推文"); got.Text != "清理 1 則推文追蹤。" {
		t.Errorf("reply = %q", got.Text)
	}
	u, _ := user.Find("1")
	if u.Subscribes.ArticleCount() != 1 || !u.Subscribes.HasArticle("M.1497363598.A.74E") {
		t.Errorf("subscriptions = %v", u.Subscribes)
	}
}

func TestTestBoard(t *testing.T) {
	h, _ := setup(t)
	if got := say(h, "1", "測試 stock"); !strings.Contains(got.Text, "共 3 篇") || !strings.Contains(got.Text, "沒有設定關鍵字") {
		t.Errorf("reply = %q", got.Text)
	}

	say(h, "1", "新增 stock 台積電,鴻海")
	if err := board.SaveArticles("stock", article.Articles{{ID: 1}}); err != nil {
		t.Fatal(err)
	}
	got := say(h, "1", "測試 Stock")
	for _, want := range []string{`最新：<a href="l3">[新聞] 台積電法說</a>`, "「台積電」命中 2 篇", "「鴻海」命中 0 篇"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("reply %q should contain %q", got.Text, want)
		}
	}
	if saved, _ := board.SavedArticles("stock"); len(saved) != 1 {
		t.Errorf("測試 must not change saved articles: %v", saved)
	}
	if got := say(h, "1", "測試 movie"); !strings.Contains(got.Text, "❌ movie 抓取失敗：HTTP 500") {
		t.Errorf("reply = %q", got.Text)
	}
}

func TestStatus(t *testing.T) {
	h, _ := setup(t)
	h.Health.SetBot("bot")
	h.Health.SetBoards([]string{"stock", "joke"})
	h.Health.BoardChecked("stock", nil)
	h.Health.BoardChecked("joke", nil)
	got := say(h, "1", "狀態")
	for _, want := range []string{"全部正常", "看板</b>（2）", "✅ stock", "Telegram"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("admin status %q should contain %q", got.Text, want)
		}
	}

	access.Approve("22")
	say(h, "22", "新增 stock a")
	got = say(h, "22", "📊 狀態")
	if !strings.Contains(got.Text, "看板</b>（1）") || strings.Contains(got.Text, "joke") || strings.Contains(got.Text, "Telegram") {
		t.Errorf("member status = %q", got.Text)
	}
}

func TestDisabledUserResumes(t *testing.T) {
	h, _ := setup(t)
	say(h, "1", "新增 stock a")
	user.Disable("1")
	keyword.Update("1", true)
	if board.Exist("stock") {
		t.Fatal("disabled user should not be checked")
	}
	say(h, "1", "清單")
	if !board.Exist("stock") {
		t.Error("talking to the bot again resumes notifications")
	}
}

func TestAddMenu(t *testing.T) {
	h, _ := setup(t)
	say(h, "1", "新增 movie a")

	for _, text := range []string{"➕ 新增", "新增", "/add"} {
		got := say(h, "1", text)
		if !hasButton(got, "pb|movie") || !hasButton(got, "pb|Stock") || !hasButton(got, "ob") {
			t.Fatalf("menu %q = %+v", text, got)
		}
		if got.Buttons[0][0].Data != "pb|movie" {
			t.Errorf("boards of user come first: %+v", got.Buttons[0])
		}
	}

	got := press(h, "1", "pb|Stock")
	if !got.Edit || !strings.Contains(got.Text, "<b>Stock</b> 通知的關鍵字") {
		t.Fatalf("ask keywords = %+v", got)
	}
	got = say(h, "1", "台積電, 聯發科")
	if !strings.HasPrefix(got.Text, "✅ 新增成功") {
		t.Fatalf("answer = %q", got.Text)
	}
	if kws := keywordsOf(t, "1", "stock"); !reflect.DeepEqual(kws, []string{"台積電", "聯發科"}) {
		t.Errorf("keywords = %v", kws)
	}
	if got := say(h, "1", "鴻海"); !strings.Contains(got.Text, "看不懂") {
		t.Errorf("prompt is answered once: %q", got.Text)
	}
}

func TestAddMenuOtherBoard(t *testing.T) {
	h, _ := setup(t)
	say(h, "1", "新增 stock a")

	press(h, "1", "ob")
	got := say(h, "1", "stokc")
	if !strings.Contains(got.Text, "是不是 stock") || !hasButton(got, "pb|stock") {
		t.Fatalf("wrong board = %+v", got)
	}
	if got := say(h, "1", "台積電"); !strings.Contains(got.Text, "看不懂") {
		t.Errorf("wrong board ends prompt: %q", got.Text)
	}

	press(h, "1", "ob")
	if got := say(h, "1", "Gossiping"); !strings.Contains(got.Text, "<b>gossiping</b> 通知的關鍵字") || got.Edit {
		t.Fatalf("board answer = %+v", got)
	}
	say(h, "1", "柯")
	if kws := keywordsOf(t, "1", "gossiping"); !reflect.DeepEqual(kws, []string{"柯"}) {
		t.Errorf("keywords = %v", kws)
	}
}

func TestAddMenuCancelledByCommand(t *testing.T) {
	h, _ := setup(t)
	press(h, "1", "pb|stock")
	if got := say(h, "1", "清單"); !strings.Contains(got.Text, "你的關鍵字") {
		t.Fatalf("command while asked = %q", got.Text)
	}
	say(h, "1", "台積電")
	if kws := keywordsOf(t, "1", "stock"); len(kws) != 0 {
		t.Errorf("prompt should be dropped: %v", kws)
	}

	press(h, "1", "pb|stock")
	press(h, "1", "cancel")
	say(h, "1", "台積電")
	if kws := keywordsOf(t, "1", "stock"); len(kws) != 0 {
		t.Errorf("cancel should drop prompt: %v", kws)
	}

	press(h, "1", "pb|stock")
	got := say(h, "1", "regexp:[")
	if !strings.Contains(got.Text, "正規表示式錯誤") || !hasButton(got, "pb|stock") {
		t.Errorf("invalid answer offers retry: %+v", got)
	}
}

func TestEditKeywordsForWeb(t *testing.T) {
	h, _ := setup(t)
	if problem, err := h.EditKeywords("1", "新增", "stock", "a&b"); problem != "" || err != nil {
		t.Fatalf("EditKeywords() = %q, %v", problem, err)
	}
	if kws := keywordsOf(t, "1", "stock"); !reflect.DeepEqual(kws, []string{"a&b"}) {
		t.Errorf("keywords = %v", kws)
	}
	if problem, _ := h.EditKeywords("1", "新增", "stokc", "x"); !strings.Contains(problem, "是不是 stock") {
		t.Errorf("problem = %q", problem)
	}
	if problem, _ := h.EditKeywords("1", "刪除", "stock", "a&b"); problem != "" || board.Exist("stock") {
		t.Errorf("delete problem = %q", problem)
	}
	if err := h.RemoveMember("1"); err != ErrRemoveAdmin {
		t.Errorf("RemoveMember(admin) = %v", err)
	}
}

func TestKeyboardButtonsAreCommands(t *testing.T) {
	for _, row := range KeyboardRows {
		for _, text := range row {
			if _, ok := keyboardAlias[text]; !ok {
				t.Errorf("keyboard button %q has no alias", text)
			}
		}
	}
}
