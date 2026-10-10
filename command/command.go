// Package command parses and handles messages and button presses from users
package command

import (
	"regexp"
	"strings"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/access"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

// limits per user
const (
	MaxBoards   = 20
	MaxKeywords = 20
	MaxArticles = 50
)

// callbackLimit is max bytes of telegram callback data
const callbackLimit = 64

const failedMsg = "失敗，請稍後再試。"

// callback data prefixes of buttons
const (
	cbDeleteKeyword = "dk|"
	cbAddKeyword    = "ak|"
	cbDeleteArticle = "da|"
	cbTrackArticle  = "ta|"
	cbApprove       = "ap|"
	cbReject        = "rj|"
	cbRemoveUser    = "rm|"
	cbPickBoard     = "pb|"
	cbOtherBoard    = "ob"
	cbBoardView     = "bv|"
	cbDeleteBoard   = "db|"
	// buttons of notifications, they must not replace the notification
	cbTrackToast   = "tt|"
	cbUntrackToast = "ut|"
	cbBoardMessage = "bm|"
	cbCancel       = "cancel"
)

// Input is a message or a button press from user
type Input struct {
	Account string
	ChatID  int64
	// Name is display name of user
	Name string
	Text string
	// Callback means Text is data of a pressed button
	Callback bool
}

// Handler handles inputs of users
type Handler struct {
	Health *health.Tracker
	// Notifier sends messages to users other than the sender, e.g. admins
	Notifier notify.Notifier

	CheckBoard   func(name string) error
	FetchBoard   func(name string) (article.Articles, error)
	FetchArticle func(board, code string) (article.Article, error)

	prompts prompts
}

// NewHandler creates Handler fetching from ptt
func NewHandler(tracker *health.Tracker, n notify.Notifier) *Handler {
	return &Handler{
		Health:       tracker,
		Notifier:     n,
		CheckBoard:   board.CheckExist,
		FetchBoard:   board.Fetch,
		FetchArticle: web.FetchArticle,
	}
}

// keyboard buttons send these texts
var keyboardAlias = map[string]string{
	"➕ 新增":   "新增",
	"📋 清單":   "清單",
	"💬 推文清單": "推文清單",
	"📊 狀態":   "狀態",
	"❓ 指令":   "指令",
}

// slash commands of telegram menu
var slashAlias = map[string]string{
	"/add":      "新增",
	"/list":     "清單",
	"/articles": "推文清單",
	"/status":   "狀態",
	"/help":     "指令",
	"/users":    "使用者",
}

var (
	keywordRe   = regexp.MustCompile(`^(新增|刪除)\s+(\S+?):?\s+(\S.*)$`)
	boardNameRe = regexp.MustCompile(`^[\w.-]+$`)
	// links of ptt.cc and pttweb.cc
	articleURLRe   = regexp.MustCompile(`https?://(?:www\.)?ptt(?:web)?\.cc/bbs/([\w.-]+)/([MG]\.\d+\.A\.[0-9A-Za-z]+)(?:\.html)?`)
	articleCmdRe   = regexp.MustCompile(`^(新增推文|刪除推文)\s+` + articleURLRe.String() + `\s*$`)
	testRe         = regexp.MustCompile(`^測試\s+([\w.-]+)$`)
	registrationRe = regexp.MustCompile(`^註冊\s+(審核|開放|關閉)$`)
	guessKeywordRe = regexp.MustCompile(`^([\w.-]+)\s+(\S.*)$`)
)

// Handle handles input and returns reply
func (h *Handler) Handle(in Input) notify.Message {
	in.Text = strings.TrimSpace(in.Text)
	msg, err := h.handle(in)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{"account": in.Account, "text": in.Text}).Error("Handle Command Failed")
		return notify.Text("處理" + failedMsg)
	}
	return msg
}

func (h *Handler) handle(in Input) (notify.Message, error) {
	if code, ok := strings.CutPrefix(in.Text, "/start "); ok && !in.Callback {
		claimed, err := access.Claim(in.Account, strings.TrimSpace(code))
		if err != nil {
			return notify.Message{}, err
		}
		if claimed {
			log.WithField("account", in.Account).Info("Admin Claimed")
			if err := h.register(in); err != nil {
				return notify.Message{}, err
			}
			welcome := Welcome()
			welcome.Text = "👑 你已成為管理員。\n朋友傳訊息給 bot 時，你會收到是否允許的按鈕。\n\n" + welcome.Text
			return welcome, nil
		}
	}

	member, err := access.IsMember(in.Account)
	if err != nil {
		return notify.Message{}, err
	}
	if !member {
		return h.stranger(in)
	}
	if err := h.register(in); err != nil {
		return notify.Message{}, err
	}
	if in.Callback {
		return h.callback(in)
	}
	return h.command(in)
}

// register saves name and chat of member and resumes notifications of a disabled user
func (h *Handler) register(in Input) error {
	changed, err := user.Register(in.Account, in.ChatID, in.Name)
	if err != nil || !changed {
		return err
	}
	return keyword.Update(in.Account, true)
}

func (h *Handler) command(in Input) (notify.Message, error) {
	// input asked by a menu is dropped when user runs another command
	asked, prompted := h.prompts.take(in.Account)
	text := in.Text
	if alias, ok := keyboardAlias[text]; ok {
		text = alias
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return Help(), nil
	}
	if alias, ok := slashAlias[fields[0]]; ok {
		fields[0] = alias
		text = strings.Join(fields, " ")
	}
	log.WithFields(log.Fields{"account": in.Account, "command": fields[0]}).Info("Command")

	switch fields[0] {
	case "/start":
		return Welcome(), nil
	case "指令", "help":
		return Help(), nil
	case "清單", "list":
		return h.list(in.Account, false)
	case "狀態":
		return h.status(in.Account)
	case "測試":
		args := testRe.FindStringSubmatch(text)
		if args == nil {
			return notify.Text("格式錯誤。範例：測試 stock"), nil
		}
		return h.test(in.Account, args[1])
	case "新增", "刪除":
		if text == "新增" {
			return h.addMenu(in.Account)
		}
		args := keywordRe.FindStringSubmatch(text)
		if args == nil {
			return notify.Text("格式錯誤，需以空白分隔動作、看板、關鍵字。\n範例：" + fields[0] + " gossiping,movie 金城武,結衣"), nil
		}
		if args[1] == "刪除" && args[3] == "*" && !in.Callback && len(text) <= callbackLimit {
			return confirm("確定刪除 "+args[2]+" 的全部關鍵字？", text, cbCancel), nil
		}
		return h.keywords(in.Account, args[1], args[2], args[3])
	case "新增推文", "刪除推文":
		args := articleCmdRe.FindStringSubmatch(text)
		if args == nil {
			return notify.Text("格式錯誤，請貼上 PTT 文章網址。\n範例：" + fields[0] + " https://www.ptt.cc/bbs/EZsoft/M.1497363598.A.74E.html"), nil
		}
		if args[1] == "新增推文" {
			return h.addArticle(in.Account, args[2], args[3])
		}
		return h.removeArticle(in.Account, args[3])
	case "推文清單":
		return h.articleList(in.Account, false)
	case "清理推文":
		return h.cleanArticles(in.Account)
	case "使用者", "註冊":
		admin, err := access.IsAdmin(in.Account)
		if err != nil || !admin {
			return notify.Text("只有管理員可以使用這個指令。"), err
		}
		if fields[0] == "使用者" {
			return h.users(false)
		}
		args := registrationRe.FindStringSubmatch(text)
		if args == nil {
			return notify.Text("格式錯誤。範例：註冊 審核、註冊 開放、註冊 關閉"), nil
		}
		return h.setRegistration(args[1])
	}
	if prompted {
		return h.answerPrompt(in.Account, asked, text)
	}
	return h.guess(text), nil
}

// guess suggests command for text which is not a command
func (h *Handler) guess(text string) notify.Message {
	if args := articleURLRe.FindStringSubmatch(text); args != nil {
		msg := notify.Text("要追蹤這篇文章的推文嗎？")
		msg.Buttons = buttonRow(button("💬 追蹤推文", cbTrackArticle+args[1]+"|"+args[2]))
		return msg
	}
	if args := guessKeywordRe.FindStringSubmatch(text); args != nil {
		cmd := "新增 " + args[1] + " " + args[2]
		msg := notify.Message{Text: "要新增關鍵字嗎？\n<code>" + notify.E(cmd) + "</code>"}
		if data := cbAddKeyword + args[1] + "|" + args[2]; len(data) <= callbackLimit {
			msg.Buttons = buttonRow(button("➕ 新增", data), button("取消", cbCancel))
		}
		return msg
	}
	msg := notify.Text("看不懂這個指令 🤔\n新增關鍵字範例：新增 stock 台積電\n輸入「指令」查看全部用法。")
	msg.Keyboard = true
	return msg
}

func (h *Handler) callback(in Input) (notify.Message, error) {
	data := in.Text
	// pressing any button cancels input asked before
	h.prompts.take(in.Account)
	split := func(prefix string) []string {
		return strings.SplitN(strings.TrimPrefix(data, prefix), "|", 2)
	}
	switch {
	case data == cbCancel:
		return notify.Message{Text: "已取消。", Edit: true}, nil
	case strings.HasPrefix(data, cbDeleteKeyword):
		args := split(cbDeleteKeyword)
		if len(args) != 2 {
			return notify.Text("按鈕已失效。"), nil
		}
		if _, err := h.keywords(in.Account, "刪除", args[0], args[1]); err != nil {
			return notify.Message{}, err
		}
		return h.boardView(in.Account, args[0], true)
	case strings.HasPrefix(data, cbBoardView):
		return h.boardView(in.Account, strings.TrimPrefix(data, cbBoardView), true)
	case strings.HasPrefix(data, cbDeleteBoard):
		name := strings.TrimPrefix(data, cbDeleteBoard)
		msg := confirm("確定刪除 "+name+" 的全部關鍵字？", "刪除 "+name+" *", cbBoardView+name)
		msg.Edit = true
		return msg, nil
	case strings.HasPrefix(data, cbTrackToast):
		args := split(cbTrackToast)
		if len(args) != 2 {
			return notify.Message{Toast: "按鈕已失效。"}, nil
		}
		return toast(h.addArticle(in.Account, args[0], args[1]))
	case strings.HasPrefix(data, cbUntrackToast):
		return toast(h.removeArticle(in.Account, strings.TrimPrefix(data, cbUntrackToast)))
	case strings.HasPrefix(data, cbBoardMessage):
		return h.boardView(in.Account, strings.TrimPrefix(data, cbBoardMessage), false)
	case strings.HasPrefix(data, cbPickBoard):
		return h.askKeywords(in.Account, strings.TrimPrefix(data, cbPickBoard)), nil
	case data == cbOtherBoard:
		return h.askBoard(in.Account), nil
	case strings.HasPrefix(data, cbAddKeyword):
		args := split(cbAddKeyword)
		if len(args) != 2 {
			return notify.Text("按鈕已失效。"), nil
		}
		msg, err := h.keywords(in.Account, "新增", args[0], args[1])
		msg.Edit = true
		return msg, err
	case strings.HasPrefix(data, cbDeleteArticle):
		if _, err := h.removeArticle(in.Account, strings.TrimPrefix(data, cbDeleteArticle)); err != nil {
			return notify.Message{}, err
		}
		return h.articleList(in.Account, true)
	case strings.HasPrefix(data, cbTrackArticle):
		args := split(cbTrackArticle)
		if len(args) != 2 {
			return notify.Text("按鈕已失效。"), nil
		}
		msg, err := h.addArticle(in.Account, args[0], args[1])
		msg.Edit = true
		return msg, err
	case strings.HasPrefix(data, cbApprove), strings.HasPrefix(data, cbReject), strings.HasPrefix(data, cbRemoveUser):
		admin, err := access.IsAdmin(in.Account)
		if err != nil || !admin {
			return notify.Text("只有管理員可以使用這個按鈕。"), err
		}
		return h.manage(data)
	}
	// other buttons run their data as command, e.g. confirmations
	msg, err := h.command(in)
	msg.Edit = true
	return msg, err
}

func button(text, data string) notify.Button {
	return notify.Button{Text: text, Data: data}
}

func buttonRow(buttons ...notify.Button) [][]notify.Button {
	return [][]notify.Button{buttons}
}

// confirm asks to run cmd, otherwise runs no, both must fit callback limit
func confirm(question, cmd, no string) notify.Message {
	msg := notify.Text(question)
	msg.Buttons = buttonRow(button("是", cmd), button("否", no))
	return msg
}
