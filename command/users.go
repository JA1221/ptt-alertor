package command

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/models/access"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

var modeDescriptions = map[string]string{
	access.ModeApproval: "陌生人傳訊息時，你會收到是否允許的按鈕。",
	access.ModeOpen:     "任何人傳訊息給 bot 都能直接使用。",
	access.ModeClosed:   "不接受新使用者，現有成員不受影響。",
}

// stranger handles input of non-member by registration mode
func (h *Handler) stranger(in Input) (notify.Message, error) {
	admins, err := access.Admins()
	if err != nil {
		return notify.Message{}, err
	}
	if len(admins) == 0 {
		return notify.Text("這個 bot 還沒有管理員。\n架設者請打開設定頁面，點「認領管理員」連結。"), nil
	}
	mode, err := access.Registration()
	if err != nil {
		return notify.Message{}, err
	}
	switch mode {
	case access.ModeClosed:
		return notify.Text("這個 bot 目前不開放新使用者。"), nil
	case access.ModeOpen:
		if err := access.Approve(in.Account); err != nil {
			return notify.Message{}, err
		}
		log.WithField("account", in.Account).Info("Member Joined")
		if err := h.register(in); err != nil {
			return notify.Message{}, err
		}
		return Welcome(), nil
	}

	// save name so admins know who asks
	if _, err := user.Register(in.Account, in.ChatID, in.Name); err != nil {
		return notify.Message{}, err
	}
	added, err := access.Request(in.Account)
	if err != nil {
		return notify.Message{}, err
	}
	if !added {
		return notify.Text("⏳ 申請審核中，管理員同意後會通知你。"), nil
	}
	log.WithField("account", in.Account).Info("Member Requested")
	request := notify.Message{
		Text: fmt.Sprintf("🙋 <b>%s</b>（%s）想使用這個 bot", notify.E(nameOf(in.Account)), notify.E(in.Account)),
		Buttons: buttonRow(
			button("✅ 允許", cbApprove+in.Account),
			button("❌ 拒絕", cbReject+in.Account),
		),
	}
	for _, admin := range admins {
		h.send(admin, request)
	}
	return notify.Text("📨 已送出使用申請，管理員同意後會通知你。"), nil
}

// send sends message to account, errors are logged
func (h *Handler) send(account string, msg notify.Message) {
	chatID, err := strconv.ParseInt(account, 10, 64)
	if err != nil {
		return
	}
	if u, err := user.Find(account); err == nil {
		chatID = u.ChatID()
	}
	if err := h.Notifier.Send(chatID, msg); err != nil {
		log.WithError(err).WithField("account", account).Warn("Send Message Failed")
	}
}

// manage handles approve, reject and remove buttons of admins
func (h *Handler) manage(data string) (notify.Message, error) {
	account := data[len(cbApprove):]
	name := account
	if u, err := user.Find(account); err == nil {
		name = u.DisplayName()
	}
	switch data[:len(cbApprove)] {
	case cbApprove:
		if err := access.Approve(account); err != nil {
			return notify.Message{}, err
		}
		if err := keyword.Update(account, true); err != nil {
			return notify.Message{}, err
		}
		welcome := Welcome()
		welcome.Text = "✅ 管理員已同意，可以開始使用了！\n\n" + welcome.Text
		h.send(account, welcome)
		log.WithField("account", account).Info("Member Approved")
		return notify.Message{Text: "✅ 已允許 " + notify.E(name), Edit: true}, nil
	case cbReject:
		if err := access.Reject(account); err != nil {
			return notify.Message{}, err
		}
		h.send(account, notify.Text("管理員沒有同意你的使用申請。"))
		return notify.Message{Text: "❌ 已拒絕 " + notify.E(name), Edit: true}, nil
	default:
		admin, err := access.IsAdmin(account)
		if err != nil {
			return notify.Message{}, err
		}
		if admin {
			return notify.Text("不能移除管理員。"), nil
		}
		if err := access.Remove(account); err != nil {
			return notify.Message{}, err
		}
		if err := keyword.Update(account, false); err != nil {
			return notify.Message{}, err
		}
		log.WithField("account", account).Info("Member Removed")
		return h.users(true)
	}
}

// users lists members and pending accounts for admins
func (h *Handler) users(edit bool) (notify.Message, error) {
	members, err := access.Members()
	if err != nil {
		return notify.Message{}, err
	}
	admins, err := access.Admins()
	if err != nil {
		return notify.Message{}, err
	}
	pending, err := access.Pending()
	if err != nil {
		return notify.Message{}, err
	}
	mode, err := access.Registration()
	if err != nil {
		return notify.Message{}, err
	}
	isAdmin := make(map[string]bool)
	for _, a := range admins {
		isAdmin[a] = true
	}
	sort.Strings(members)
	sort.Strings(pending)

	msg := notify.Message{Edit: edit}
	lines := []string{fmt.Sprintf("👥 <b>成員</b>（%d）", len(members))}
	for _, account := range members {
		name := account
		line := "• " + notify.E(account)
		if u, err := user.Find(account); err == nil {
			name = u.DisplayName()
			line = fmt.Sprintf("• %s（%s）看板 %d、推文 %d", notify.E(name), notify.E(account),
				len(u.Subscribes.KeywordBoards()), u.Subscribes.ArticleCount())
			if !u.Enable {
				line += "、已封鎖 bot"
			}
		}
		if isAdmin[account] {
			line += " 👑"
		} else if len(msg.Buttons) < maxListButtons {
			msg.Buttons = append(msg.Buttons, buttonRow(button("🚫 移除 "+truncate(name, 20), cbRemoveUser+account))...)
		}
		lines = append(lines, line)
	}
	if len(pending) > 0 {
		lines = append(lines, "", fmt.Sprintf("⏳ <b>待審核</b>（%d）", len(pending)))
		for _, account := range pending {
			name := nameOf(account)
			lines = append(lines, "• "+notify.E(name)+"（"+notify.E(account)+"）")
			if len(msg.Buttons) < maxListButtons {
				msg.Buttons = append(msg.Buttons, buttonRow(
					button("✅ 允許 "+truncate(name, 12), cbApprove+account),
					button("❌ 拒絕", cbReject+account),
				)...)
			}
		}
	}
	lines = append(lines, "", "註冊模式："+access.ModeNames[mode]+"（<code>註冊 審核</code>／<code>註冊 開放</code>／<code>註冊 關閉</code>）")
	msg.Text = strings.Join(lines, "\n")
	return msg, nil
}

func nameOf(account string) string {
	if u, err := user.Find(account); err == nil {
		return u.DisplayName()
	}
	return account
}

func (h *Handler) setRegistration(name string) (notify.Message, error) {
	for mode, n := range access.ModeNames {
		if n != name {
			continue
		}
		if err := access.SetRegistration(mode); err != nil {
			return notify.Message{}, err
		}
		return notify.Text("✅ 註冊模式：" + name + "\n" + modeDescriptions[mode]), nil
	}
	return notify.Text("格式錯誤。範例：註冊 審核、註冊 開放、註冊 關閉"), nil
}
