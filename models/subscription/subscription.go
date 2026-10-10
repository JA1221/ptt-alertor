// Package subscription is user's keyword and comment subscriptions per board
package subscription

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/myutil"
)

// Subscription is keywords and tracked articles (codes) of a board
type Subscription struct {
	Board    string             `json:"board"`
	Keywords myutil.StringSlice `json:"keywords"`
	Articles myutil.StringSlice `json:"articles"`
}

func (s Subscription) empty() bool {
	return len(s.Keywords) == 0 && len(s.Articles) == 0
}

// ArticleURL is ptt link of article code
func ArticleURL(board, code string) string {
	return fmt.Sprintf("https://www.ptt.cc/bbs/%s/%s.html", board, code)
}

// Subscriptions are subscriptions of a user, boards are case insensitive
type Subscriptions []Subscription

func (ss Subscriptions) index(board string) int {
	for i, s := range ss {
		if strings.EqualFold(s.Board, board) {
			return i
		}
	}
	return -1
}

// Find gets subscription of board
func (ss Subscriptions) Find(board string) (Subscription, bool) {
	if i := ss.index(board); i >= 0 {
		return ss[i], true
	}
	return Subscription{}, false
}

func (ss *Subscriptions) edit(board string, fn func(s *Subscription)) {
	i := ss.index(board)
	if i < 0 {
		*ss = append(*ss, Subscription{Board: board})
		i = len(*ss) - 1
	}
	fn(&(*ss)[i])
	if (*ss)[i].empty() {
		*ss = append((*ss)[:i], (*ss)[i+1:]...)
	}
}

// AddKeywords adds keywords of board, repeated ones are ignored
func (ss *Subscriptions) AddKeywords(board string, keywords []string) {
	ss.edit(board, func(s *Subscription) {
		s.Keywords.AppendNonRepeat(keywords, false)
		s.Keywords.Clean()
	})
}

// RemoveKeywords removes keywords of board, "*" removes all
func (ss *Subscriptions) RemoveKeywords(board string, keywords []string) {
	ss.edit(board, func(s *Subscription) {
		if len(keywords) == 1 && keywords[0] == "*" {
			s.Keywords = nil
			return
		}
		s.Keywords.Delete(keywords, false)
	})
}

// AddArticle tracks comments of article
func (ss *Subscriptions) AddArticle(board, code string) {
	ss.edit(board, func(s *Subscription) {
		s.Articles.AppendNonRepeatElement(code, true)
	})
}

// RemoveArticle stops tracking article of any board, reports it was tracked
func (ss *Subscriptions) RemoveArticle(code string) bool {
	for _, s := range *ss {
		if s.Articles.Index(code, true) >= 0 {
			ss.edit(s.Board, func(s *Subscription) {
				s.Articles.DeleteElement(code, true)
			})
			return true
		}
	}
	return false
}

// HasArticle reports article is tracked
func (ss Subscriptions) HasArticle(code string) bool {
	for _, s := range ss {
		if s.Articles.Index(code, true) >= 0 {
			return true
		}
	}
	return false
}

// ArticleCount counts tracked articles
func (ss Subscriptions) ArticleCount() (n int) {
	for _, s := range ss {
		n += len(s.Articles)
	}
	return n
}

// KeywordBoards lists boards having keywords
func (ss Subscriptions) KeywordBoards() []string {
	var boards []string
	for _, s := range ss {
		if len(s.Keywords) > 0 {
			boards = append(boards, s.Board)
		}
	}
	return boards
}

func (ss Subscriptions) sorted() Subscriptions {
	sorted := make(Subscriptions, len(ss))
	copy(sorted, ss)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Board) < strings.ToLower(sorted[j].Board)
	})
	return sorted
}

// StringKeywords lists keywords per board, one board a line
func (ss Subscriptions) StringKeywords() string {
	var lines []string
	for _, s := range ss.sorted() {
		if len(s.Keywords) > 0 {
			lines = append(lines, s.Board+": "+strings.Join(s.Keywords, ", "))
		}
	}
	return strings.Join(lines, "\n")
}

// StringArticles lists links of tracked articles grouped by board
func (ss Subscriptions) StringArticles() string {
	var blocks []string
	for _, s := range ss.sorted() {
		if len(s.Articles) == 0 {
			continue
		}
		lines := []string{s.Board + ":"}
		for _, code := range s.Articles {
			lines = append(lines, ArticleURL(s.Board, code))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n")
}
