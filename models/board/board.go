// Package board keeps checking boards and their latest articles in redis:
// "boards" set of board names, board:<name> json of articles fetched last time
package board

import (
	"encoding/json"
	"errors"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/rss"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

const (
	listKey = "boards"
	prefix  = "board:"
)

// ErrNotExist means board does not exist on ptt
var ErrNotExist = errors.New("board is not exist")

// List lists checking boards
func List() []string {
	conn := connections.Redis()
	defer conn.Close()
	boards, err := redis.Strings(conn.Do("SMEMBERS", listKey))
	if err != nil {
		log.WithError(err).Error("List Boards Failed")
	}
	return boards
}

// Exist reports board is in checking list
func Exist(name string) bool {
	conn := connections.Redis()
	defer conn.Close()
	bl, err := redis.Bool(conn.Do("SISMEMBER", listKey, name))
	if err != nil {
		log.WithError(err).Error("Check Board Failed")
	}
	return bl
}

// Add adds board to checking list
func Add(name string) error {
	conn := connections.Redis()
	defer conn.Close()
	_, err := conn.Do("SADD", listKey, name)
	return err
}

// Remove removes board from checking list and its saved articles
func Remove(name string) error {
	conn := connections.Redis()
	defer conn.Close()
	conn.Send("MULTI")
	conn.Send("SREM", listKey, name)
	conn.Send("DEL", prefix+name)
	_, err := conn.Do("EXEC")
	return err
}

// Sync makes checking list equal to names, removes saved articles of dropped boards
func Sync(names []string) error {
	keep := make(map[string]bool, len(names))
	for _, name := range names {
		keep[name] = true
		if err := Add(name); err != nil {
			return err
		}
	}
	for _, name := range List() {
		if !keep[name] {
			log.WithField("board", name).Info("Remove Unsubscribed Board")
			if err := Remove(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// SavedArticles gets articles fetched last time, nil when never saved
func SavedArticles(name string) (articles article.Articles, err error) {
	conn := connections.Redis()
	defer conn.Close()
	b, err := redis.Bytes(conn.Do("GET", prefix+name))
	if err == redis.ErrNil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &articles)
	return articles, err
}

// SaveArticles saves fetched articles
func SaveArticles(name string, articles article.Articles) error {
	b, err := json.Marshal(articles)
	if err != nil {
		return err
	}
	conn := connections.Redis()
	defer conn.Close()
	_, err = conn.Do("SET", prefix+name, b)
	return err
}

// NewArticles returns online articles newer than all saved articles,
// articles without id (deleted) are ignored, nothing is new when saved has no id to compare
func NewArticles(saved, online article.Articles) article.Articles {
	var latest int
	for _, a := range saved {
		if a.ID > latest {
			latest = a.ID
		}
	}
	if latest == 0 {
		return nil
	}
	var news article.Articles
	for _, a := range online {
		if a.ID > latest {
			news = append(news, a)
		}
	}
	return news
}

// Fetch fetches latest articles of board, by rss first and html when rss fails
func Fetch(name string) (article.Articles, error) {
	articles, err := rss.BuildArticles(name)
	if err == rss.ErrTooManyRequests {
		return nil, err
	}
	if err != nil {
		log.WithField("board", name).WithError(err).Warn("RSS Fetch Failed, Switch to HTML Crawler")
		articles, err = web.FetchArticles(name, -1)
		if err != nil {
			return nil, err
		}
	}
	if strings.EqualFold(name, "allpost") {
		fixLink(articles)
	}
	return articles, nil
}

// fixLink makes ALLPOST links point to real boards, titles are like "title (board)"
func fixLink(articles article.Articles) {
	for i, a := range articles {
		pre, post := strings.LastIndex(a.Title, "("), strings.LastIndex(a.Title, ")")
		if pre < 0 || post <= pre {
			continue
		}
		articles[i].Link = strings.Replace(a.Link, "ALLPOST", a.Title[pre+1:post], -1)
	}
}

// CheckExist checks board exists on ptt
func CheckExist(name string) error {
	if Exist(strings.ToLower(name)) {
		return nil
	}
	if rss.CheckBoardExist(name) || web.CheckBoardExist(name) {
		return nil
	}
	return ErrNotExist
}

// Suggest finds the most similar checking board name, empty when nothing alike
func Suggest(name string) string {
	name = strings.ToLower(name)
	var best string
	var bestScore float64
	for _, b := range List() {
		if score := similarity(name, strings.ToLower(b)); score > bestScore {
			best, bestScore = b, score
		}
	}
	if bestScore < 0.5 {
		return ""
	}
	return best
}

// similarity is ratio of common characters to the longer name
func similarity(a, b string) float64 {
	counts := make(map[rune]int)
	for _, r := range b {
		counts[r]++
	}
	var common int
	for _, r := range a {
		if counts[r] > 0 {
			counts[r]--
			common++
		}
	}
	longer := len([]rune(a))
	if n := len([]rune(b)); n > longer {
		longer = n
	}
	if longer == 0 {
		return 0
	}
	return float64(common) / float64(longer)
}
