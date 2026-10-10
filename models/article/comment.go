package article

import (
	"fmt"
	"strings"
	"time"
)

// Comment is a push of article
type Comment struct {
	Tag      string
	UserID   string
	Content  string
	DateTime time.Time
}

func (c Comment) String() string {
	// 推 ChoDino: 推文推文
	return fmt.Sprintf("%s %s%s", strings.TrimSpace(c.Tag), c.UserID, c.Content)
}

// Comments is a list of comment
type Comments []Comment

// NewComments returns comments of online article added after saved article was checked,
// comments are compared by count because ptt comment time is only precise to minute
func NewComments(saved, online Article) Comments {
	if len(online.Comments) <= saved.CommentCount {
		return nil
	}
	return online.Comments[saved.CommentCount:]
}
