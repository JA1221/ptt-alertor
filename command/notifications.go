package command

import (
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

// maxTrackButtons limits articles of a notification offering tracking comments
const maxTrackButtons = 3

// ArticlesButtons are buttons of a keyword notification: tracking comments of the articles,
// and managing keywords of the board
func ArticlesButtons(boardName string, articles article.Articles) [][]notify.Button {
	var rows [][]notify.Button
	for _, a := range articles {
		if len(rows) == maxTrackButtons {
			break
		}
		args := articleURLRe.FindStringSubmatch(a.Link)
		if args == nil {
			continue
		}
		if data := cbTrackToast + args[1] + "|" + args[2]; len(data) <= callbackLimit {
			rows = append(rows, []notify.Button{button("💬 追蹤推文："+truncate(a.Title, 24), data)})
		}
	}
	if data := cbBoardMessage + boardName; len(data) <= callbackLimit {
		rows = append(rows, []notify.Button{button("⚙️ 管理 "+boardName+" 關鍵字", data)})
	}
	return rows
}

// CommentsButtons are buttons of a comment notification
func CommentsButtons(code string) [][]notify.Button {
	return buttonRow(button("🛑 停止追蹤推文", cbUntrackToast+code))
}

// toast pops up result of msg instead of sending it
func toast(msg notify.Message, err error) (notify.Message, error) {
	return notify.Message{Toast: notify.Plain(msg.Text)}, err
}
