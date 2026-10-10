package rss

import (
	"errors"
	"net/http"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	pttHttp "github.com/Ptt-Alertor/ptt-alertor/ptt/http"
	"github.com/mmcdole/gofeed"
)

// ErrTooManyRequests means ptt.cc limits requests, fetching should pause for a while
var ErrTooManyRequests = errors.New("too many requests")

func feedURL(board string) string {
	return "https://www.ptt.cc/atom/" + board + ".xml"
}

// CheckBoardExist use for checking board exist or not
func CheckBoardExist(board string) bool {
	feed, err := parseURL(feedURL(board))
	// category didn't have any items but has rss link as well
	return err == nil && len(feed.Items) > 0
}

// BuildArticles fetches latest articles of board from its atom feed
func BuildArticles(board string) (articles article.Articles, err error) {
	feed, err := parseURL(feedURL(board))
	if err != nil {
		if herr, ok := err.(gofeed.HTTPError); ok && herr.StatusCode == http.StatusTooManyRequests {
			return nil, ErrTooManyRequests
		}
		return nil, err
	}
	for _, item := range feed.Items {
		a := article.Article{
			Title: item.Title,
			Link:  item.GUID,
			Date:  item.Published,
		}
		if item.Author != nil {
			a.Author = item.Author.Name
		}
		a.ID = a.ParseID(item.GUID)
		articles = append(articles, a)
	}
	return articles, nil
}

var fp = gofeed.NewParser()
var client = http.Client{
	Timeout: 30 * time.Second,
}

func parseURL(feedURL string) (feed *gofeed.Feed, err error) {
	req, err := pttHttp.HttpRequest(feedURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, gofeed.HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}

	return fp.Parse(resp.Body)
}
