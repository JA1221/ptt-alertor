// Package command parses and handles text commands from users
package command

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

const (
	articlesLimit = 50
	// testShowLimit limits articles shown per keyword by 測試
	testShowLimit = 5
	failedMsg     = "失敗，請稍後再試。"
)

// Help lists commands in fixed order
var Help = strings.Join([]string{
	"[一般]",
	"指令：可使用的指令清單",
	"清單：設定的看板與關鍵字",
	"狀態：各元件運作狀態",
	"測試 看板：立即抓取看板最新文章，以你的關鍵字比對",
	"",
	"[關鍵字]",
	"新增 看板 關鍵字：新增追蹤關鍵字",
	"刪除 看板 關鍵字：取消追蹤關鍵字，關鍵字為 * 時全部刪除",
	"範例：新增 gossiping,movie 金城武,結衣",
	"同時包含：新增 stock 台積電&法說",
	"排除：新增 stock 台積電&!公告",
	"正規表示式：新增 stock regexp:^\\[新聞\\]",
	"",
	"[推文]",
	"新增推文 網址：有新推文時通知",
	"刪除推文 網址：取消追蹤推文",
	"推文清單：追蹤中的文章",
	"清理推文：刪除已失效的追蹤",
	"範例：新增推文 https://www.ptt.cc/bbs/EZsoft/M.1497363598.A.74E.html",
}, "\n")

var (
	keywordRe   = regexp.MustCompile(`^(新增|刪除)\s+(\S+?):?\s+(\S.*)$`)
	boardNameRe = regexp.MustCompile(`^[\w.-]+$`)
	articleRe   = regexp.MustCompile(`^(新增推文|刪除推文)\s+https?://www\.ptt\.cc/bbs/([\w.-]+)/([MG]\.\d+\.A\.[0-9A-Za-z]+)\.html\s*$`)
	testRe      = regexp.MustCompile(`^測試\s+([\w.-]+)$`)
)

// Handler handles commands of users
type Handler struct {
	// Accounts are users allowed to use the bot, keyword index is built from them
	Accounts []string
	Health   *health.Tracker

	CheckBoard   func(name string) error
	FetchBoard   func(name string) (article.Articles, error)
	FetchArticle func(board, code string) (article.Article, error)
}

// NewHandler creates Handler fetching from ptt
func NewHandler(accounts []string, tracker *health.Tracker) *Handler {
	return &Handler{
		Accounts:     accounts,
		Health:       tracker,
		CheckBoard:   board.CheckExist,
		FetchBoard:   board.Fetch,
		FetchArticle: web.FetchArticle,
	}
}

// Handle handles text from account in chat, returns reply
func (h *Handler) Handle(account string, chatID int64, text string) string {
	text = strings.TrimSpace(text)
	if err := h.register(account, chatID); err != nil {
		log.WithError(err).WithField("account", account).Error("Register User Failed")
		return "讀取使用者資料" + failedMsg
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return Help
	}
	log.WithFields(log.Fields{"account": account, "command": fields[0]}).Info("Command")

	switch fields[0] {
	case "指令", "help", "/help", "/start":
		return Help
	case "清單", "list", "/list":
		return h.list(account)
	case "狀態", "/status":
		return StatusText(h.Health.Snapshot())
	case "測試":
		args := testRe.FindStringSubmatch(text)
		if args == nil {
			return "格式錯誤。範例：測試 stock"
		}
		return h.test(account, args[1])
	case "新增", "刪除":
		args := keywordRe.FindStringSubmatch(text)
		if args == nil {
			return "格式錯誤，需以空白分隔動作、看板、關鍵字。\n範例：" + fields[0] + " gossiping,movie 金城武,結衣"
		}
		return h.keywords(account, args[1], args[2], args[3])
	case "新增推文", "刪除推文":
		args := articleRe.FindStringSubmatch(text)
		if args == nil {
			return "格式錯誤，網址需為 PTT 文章網址。\n範例：" + fields[0] + " https://www.ptt.cc/bbs/EZsoft/M.1497363598.A.74E.html"
		}
		if args[1] == "新增推文" {
			return h.addArticle(account, args[2], args[3])
		}
		return h.removeArticle(account, args[3])
	case "推文清單":
		return h.articleList(account)
	case "清理推文":
		return h.cleanArticles(account)
	}
	return "無此指令，請打「指令」查看指令清單"
}

func (h *Handler) register(account string, chatID int64) error {
	u, err := user.Find(account)
	if err == nil && u.Profile.TelegramChat == chatID {
		return nil
	}
	if err != nil && err != user.ErrNotFound {
		return err
	}
	return user.Register(account, chatID)
}

func (h *Handler) list(account string) string {
	u, err := user.Find(account)
	if err != nil {
		return "讀取清單" + failedMsg
	}
	keywords := u.Subscribes.StringKeywords()
	if keywords == "" {
		keywords = "（無）"
	}
	return fmt.Sprintf("關鍵字\n%s\n----\n推文追蹤 %d 篇，輸入「推文清單」查看。", keywords, u.Subscribes.ArticleCount())
}

// splitParams splits comma separated params (half or full width), empty ones are dropped
func splitParams(s string) []string {
	var params []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' }) {
		if p = strings.TrimSpace(p); p != "" {
			params = append(params, p)
		}
	}
	return params
}

func (h *Handler) keywords(account, action, boardStr, keywordStr string) string {
	boards := splitParams(boardStr)
	for i, b := range boards {
		if !boardNameRe.MatchString(b) {
			return "板名格式錯誤：" + b
		}
		boards[i] = strings.ToLower(b)
	}
	if len(boards) == 0 {
		return "請輸入看板。"
	}

	var keywords []string
	if strings.HasPrefix(keywordStr, "regexp:") {
		if _, err := regexp.Compile(strings.TrimPrefix(keywordStr, "regexp:")); err != nil {
			return "正規表示式錯誤：" + err.Error()
		}
		keywords = []string{keywordStr}
	} else {
		keywords = splitParams(keywordStr)
	}
	if len(keywords) == 0 {
		return "請輸入關鍵字。"
	}

	// all boards are checked before saving, one wrong board saves nothing
	if action == "新增" {
		if msg := h.checkBoards(boards); msg != "" {
			return msg
		}
	}
	err := user.Update(account, func(u *user.User) error {
		for _, b := range boards {
			if action == "新增" {
				u.Subscribes.AddKeywords(b, keywords)
			} else {
				u.Subscribes.RemoveKeywords(b, keywords)
			}
		}
		return nil
	})
	if err == nil {
		err = keyword.Sync(h.Accounts)
	}
	if err != nil {
		log.WithError(err).WithField("account", account).Error("Update Keywords Failed")
		return action + failedMsg
	}
	return action + "成功。\n\n" + h.list(account)
}

func (h *Handler) checkBoards(boards []string) string {
	var wrong []string
	for _, b := range boards {
		err := h.CheckBoard(b)
		if err == nil {
			continue
		}
		msg := b
		if !errors.Is(err, board.ErrNotExist) {
			msg += "（" + err.Error() + "）"
		} else if s := board.Suggest(b); s != "" && !strings.EqualFold(s, b) {
			msg += "（是不是 " + s + "？）"
		}
		wrong = append(wrong, msg)
	}
	if len(wrong) == 0 {
		return ""
	}
	return "板名錯誤，未做任何變更：\n" + strings.Join(wrong, "\n")
}

func (h *Handler) addArticle(account, boardName, code string) string {
	u, err := user.Find(account)
	if err != nil {
		return "新增推文" + failedMsg
	}
	if u.Subscribes.HasArticle(code) {
		return "已在追蹤這篇文章。"
	}
	if u.Subscribes.ArticleCount() >= articlesLimit {
		return fmt.Sprintf("推文追蹤最多 %d 篇，輸入「推文清單」整理追蹤列表。", articlesLimit)
	}

	a, tracked := article.Find(code)
	if !tracked {
		online, err := h.FetchArticle(boardName, code)
		if _, ok := err.(web.URLNotFoundError); ok {
			return "文章不存在。"
		}
		if err != nil {
			return "抓取文章失敗：" + err.Error()
		}
		// comments so far are the base, only later ones are sent
		a = article.Article{
			ID:           online.ID,
			Code:         code,
			Title:        online.Title,
			Link:         online.Link,
			Board:        online.Board,
			CommentCount: len(online.Comments),
		}
		if err := a.Save(); err != nil {
			log.WithError(err).WithField("code", code).Error("Save Article Failed")
			return "新增推文" + failedMsg
		}
	}
	err = article.Track(code, account)
	if err == nil {
		err = user.Update(account, func(u *user.User) error {
			u.Subscribes.AddArticle(a.Board, code)
			return nil
		})
	}
	if err != nil {
		log.WithError(err).WithField("code", code).Error("Track Article Failed")
		return "新增推文" + failedMsg
	}
	return "新增推文成功：" + a.Title
}

func (h *Handler) removeArticle(account, code string) string {
	var removed bool
	err := user.Update(account, func(u *user.User) error {
		removed = u.Subscribes.RemoveArticle(code)
		return nil
	})
	if err == nil {
		err = article.Untrack(code, account)
	}
	if err != nil {
		log.WithError(err).WithField("code", code).Error("Untrack Article Failed")
		return "刪除推文" + failedMsg
	}
	if !removed {
		return "沒有追蹤這篇文章。"
	}
	return "刪除推文成功。"
}

func (h *Handler) articleList(account string) string {
	u, err := user.Find(account)
	if err != nil {
		return "讀取推文清單" + failedMsg
	}
	if u.Subscribes.ArticleCount() == 0 {
		return "尚未追蹤任何文章。請打「指令」查看新增方法。"
	}
	return fmt.Sprintf("推文追蹤清單（%d/%d）：\n%s\n\n輸入「清理推文」可刪除失效的追蹤。",
		u.Subscribes.ArticleCount(), articlesLimit, u.Subscribes.StringArticles())
}

// cleanArticles removes tracking of articles which are no longer checked (e.g. data lost)
func (h *Handler) cleanArticles(account string) string {
	var cleaned int
	err := user.Update(account, func(u *user.User) error {
		var codes []string
		for _, s := range u.Subscribes {
			codes = append(codes, s.Articles...)
		}
		for _, code := range codes {
			if _, ok := article.Find(code); ok {
				continue
			}
			if err := article.Untrack(code, account); err != nil {
				return err
			}
			u.Subscribes.RemoveArticle(code)
			cleaned++
		}
		return nil
	})
	if err != nil {
		log.WithError(err).WithField("account", account).Error("Clean Articles Failed")
		return "清理推文" + failedMsg
	}
	return fmt.Sprintf("清理 %d 則推文追蹤。", cleaned)
}

// test fetches latest articles of board now and matches keywords of account,
// saved articles are untouched so checking is not affected
func (h *Handler) test(account, boardName string) string {
	boardName = strings.ToLower(boardName)
	articles, err := h.FetchBoard(boardName)
	if err != nil {
		return fmt.Sprintf("❌ %s 抓取失敗：%s", boardName, err)
	}
	latest := make(article.Articles, len(articles))
	copy(latest, articles)
	sort.SliceStable(latest, func(i, j int) bool { return latest[i].ID > latest[j].ID })

	lines := []string{fmt.Sprintf("✅ %s 抓取成功，共 %d 篇。", boardName, len(articles))}
	if len(latest) > 0 {
		lines = append(lines, "最新：", latest[0].String())
	}

	u, err := user.Find(account)
	if err != nil {
		return strings.Join(append(lines, "", "讀取關鍵字"+failedMsg), "\n")
	}
	sub, _ := u.Subscribes.Find(boardName)
	if len(sub.Keywords) == 0 {
		return strings.Join(append(lines, "", "你在 "+boardName+" 沒有設定關鍵字。"), "\n")
	}
	for _, kw := range sub.Keywords {
		matched := latest.Match(kw)
		lines = append(lines, "", fmt.Sprintf("「%s」命中 %d 篇", kw, len(matched)))
		if len(matched) > testShowLimit {
			matched = matched[:testShowLimit]
		}
		for _, a := range matched {
			lines = append(lines, a.String())
		}
	}
	return strings.Join(lines, "\n")
}
