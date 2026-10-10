package command

import (
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

// KeyboardRows are texts of main reply keyboard
var KeyboardRows = [][]string{
	{"📋 清單", "💬 推文清單"},
	{"📊 狀態", "❓ 指令"},
}

// Welcome greets new members and shows keyboard
func Welcome() notify.Message {
	return notify.Message{
		Text: strings.Join([]string{
			"👋 <b>歡迎使用 PTT 關鍵字通知</b>",
			"",
			"看板有新文章的標題符合你的關鍵字，就會傳訊息給你。",
			"",
			"➕ 新增關鍵字：<code>新增 stock 台積電</code>",
			"💬 追蹤推文：直接貼上 PTT 文章網址",
			"📋 下方按鈕可查看清單與狀態",
			"",
			"輸入「指令」查看完整說明。",
		}, "\n"),
		Keyboard: true,
	}
}

// Help lists commands in fixed order
func Help() notify.Message {
	return notify.Message{
		Text: strings.Join([]string{
			"<b>🔔 關鍵字</b>",
			"<code>新增 看板 關鍵字</code>",
			"<code>刪除 看板 關鍵字</code>（關鍵字為 * 全部刪除）",
			"多個看板、關鍵字用逗號分隔：",
			"<code>新增 gossiping,movie 金城武,結衣</code>",
			"同時包含：<code>新增 stock 台積電&amp;法說</code>",
			"排除：<code>新增 stock 台積電&amp;!公告</code>",
			"正規表示式：<code>新增 stock regexp:^\\[新聞\\]</code>",
			"",
			"<b>💬 推文追蹤</b>",
			"直接貼上文章網址，或 <code>新增推文 網址</code>",
			"<code>刪除推文 網址</code>、<code>推文清單</code>、<code>清理推文</code>",
			"",
			"<b>📋 其他</b>",
			"<code>清單</code>：你的關鍵字（可按鈕刪除）",
			"<code>狀態</code>：運作狀態",
			"<code>測試 看板</code>：立即抓取最新文章並比對你的關鍵字",
			"",
			"<b>👑 管理員</b>",
			"<code>使用者</code>：成員與待審核清單",
			"<code>註冊 審核</code>／<code>註冊 開放</code>／<code>註冊 關閉</code>",
			"",
			"上限：每人 20 個看板、每個看板 20 個關鍵字、50 篇推文追蹤。",
		}, "\n"),
		Keyboard: true,
	}
}
