package command

import (
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

// ArticlesButtons are buttons of a keyword notification, one row to keep it compact
func ArticlesButtons(boardName string) [][]notify.Button {
	if data := cbBoardMessage + boardName; len(data) <= callbackLimit {
		return buttonRow(button("⚙️ 管理 "+boardName+" 關鍵字", data))
	}
	return nil
}

// CommentsButtons are buttons of a comment notification
func CommentsButtons(code string) [][]notify.Button {
	return buttonRow(button("🛑 停止追蹤推文", cbUntrackToast+code))
}

// toast pops up result of msg instead of sending it
func toast(msg notify.Message, err error) (notify.Message, error) {
	return notify.Message{Toast: notify.Plain(msg.Text)}, err
}
