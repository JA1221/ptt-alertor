package health

import (
	"fmt"
	"time"
)

// Ago describes how long ago t was from now, e.g. "3 分鐘前"
func Ago(now, t time.Time) string {
	if t.IsZero() {
		return "從未"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d 秒前", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d 分鐘前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小時前", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
}

// Duration describes length of d, e.g. "2 天 3 小時"
func Duration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d 分鐘", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小時 %d 分鐘", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%d 天 %d 小時", int(d.Hours()/24), int(d.Hours())%24)
	}
}
