package article

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Article is a ptt article
type Article struct {
	ID               int    `json:"ID,omitempty"`
	Code             string `json:"code,omitempty"`
	Title            string
	Link             string
	Date             string    `json:"Date,omitempty"`
	Author           string    `json:"Author,omitempty"`
	Comments         Comments  `json:"comments,omitempty"`
	CommentCount     int       `json:"commentCount,omitempty"`
	LastPushDateTime time.Time `json:"lastPushDateTime,omitempty"`
	Board            string    `json:"board,omitempty"`
}

var idRegexp = regexp.MustCompile(`https?://www.ptt.cc/bbs/.*/[GM]\.(\d+)\..*`)

// ParseID gets article id (post timestamp) from link, 0 if not found
func (a Article) ParseID(link string) int {
	strs := idRegexp.FindStringSubmatch(link)
	if len(strs) < 2 {
		return 0
	}
	id, err := strconv.Atoi(strs[1])
	if err != nil {
		return 0
	}
	return id
}

// MatchKeyword checks title matches keyword,
// "a&b" matches all, "!a" excludes, "regexp:pattern" matches regular expression
func (a Article) MatchKeyword(keyword string) bool {
	if strings.HasPrefix(keyword, "regexp:") {
		return matchRegex(a.Title, keyword)
	}
	if strings.Contains(keyword, "&") {
		for _, keyword := range strings.Split(keyword, "&") {
			if !matchKeyword(a.Title, keyword) {
				return false
			}
		}
		return true
	}
	return matchKeyword(a.Title, keyword)
}

func matchRegex(title string, regex string) bool {
	b, err := regexp.MatchString(strings.TrimPrefix(regex, "regexp:"), title)
	return err == nil && b
}

func matchKeyword(title string, keyword string) bool {
	if strings.HasPrefix(keyword, "!") {
		return !containKeyword(title, strings.TrimPrefix(keyword, "!"))
	}
	return containKeyword(title, keyword)
}

func containKeyword(title string, keyword string) bool {
	return strings.Contains(strings.ToLower(title), strings.ToLower(keyword))
}

// Articles is a list of article
type Articles []Article

// Match returns articles matching keyword
func (as Articles) Match(keyword string) Articles {
	var matched Articles
	for _, a := range as {
		if a.MatchKeyword(keyword) {
			matched = append(matched, a)
		}
	}
	return matched
}
