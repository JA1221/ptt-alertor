package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"golang.org/x/net/html"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	pttHttp "github.com/Ptt-Alertor/ptt-alertor/ptt/http"
)

const pttHostURL = "https://www.ptt.cc"

// FetchArticles makes board's index articles to a article slice
func FetchArticles(board string, page int) (articles article.Articles, err error) {
	reqURL := makeBoardURL(board, page)
	htmlNodes, err := fetchHTML(reqURL)
	if err != nil {
		return nil, err
	}
	articleBlocks := findNodes(htmlNodes, findArticleBlocks)
	for _, articleBlock := range articleBlocks {
		article := article.Article{}
		for _, titleDiv := range findNodes(articleBlock, findTitleDiv) {

			anchors := findNodes(titleDiv, findAnchor)

			if len(anchors) == 0 {
				article.Title = firstText(titleDiv)
				article.Link = ""
				continue
			}

			for _, anchor := range anchors {
				article.Title = firstText(anchor)
				link := pttHostURL + getAnchorLink(anchor)
				article.Link = link
				article.ID = article.ParseID(link)
			}
		}
		for _, metaDiv := range findNodes(articleBlock, findMetaDiv) {
			for _, date := range findNodes(metaDiv, findDateDiv) {
				article.Date = strings.TrimSpace(firstText(date))
			}
			for _, author := range findNodes(metaDiv, findAuthorDiv) {
				article.Author = firstText(author)
			}
		}
		articles = append(articles, article)
		if isLastArticleBlock(articleBlock) {
			break
		}
	}
	return articles, nil
}

// isLastArticleBlock reports articleBlock is followed by the divider of pinned articles
func isLastArticleBlock(articleBlock *html.Node) bool {
	for next := articleBlock.NextSibling; next != nil; next = next.NextSibling {
		if next.Type == html.ElementNode && len(next.Attr) > 0 {
			return next.Attr[0].Val == "r-list-sep"
		}
	}
	return false
}

// FetchArticle build article object from html
func FetchArticle(board, articleCode string) (article.Article, error) {
	reqURL := makeArticleURL(board, articleCode)
	htmlNodes, err := fetchHTML(reqURL)
	if err != nil {
		return article.Article{}, err
	}
	atcl := article.Article{
		Link:  reqURL,
		Code:  articleCode,
		Board: board,
	}
	nodes := findNodes(htmlNodes, findOgTitleMeta)
	if len(nodes) > 0 {
		atcl.Title = getMetaContent(nodes[0])
	} else {
		atcl.Title = "[內文標題已被刪除]"
	}
	atcl.ID = atcl.ParseID(reqURL)
	pushBlocks := findNodes(htmlNodes, findPushBlocks)
	pushes := []article.Comment{}

PushBlocks:
	for _, pushBlock := range pushBlocks {
		push := article.Comment{}
		for _, pushIPDateTime := range findNodes(pushBlock, findPushIPDateTime) {
			ipdatetime := strings.TrimSpace(firstText(pushIPDateTime))
			if ipdatetime == "" {
				continue PushBlocks
			}
			dateTime, err := parseDateTime(ipdatetime)
			if err != nil {
				log.WithFields(log.Fields{
					"ipdatetime": ipdatetime,
					"board":      board,
					"code":       articleCode,
				}).WithError(err).Warn("Parse DateTime Error")
				continue PushBlocks
			}
			push.DateTime = dateTime
		}
		for _, pushTag := range findNodes(pushBlock, findPushTag) {
			push.Tag = firstText(pushTag)
		}
		for _, pushUserID := range findNodes(pushBlock, findPushUserID) {
			push.UserID = firstText(pushUserID)
		}
		for _, pushContent := range findNodes(pushBlock, findPushContent) {
			if pushContent.FirstChild == nil {
				continue
			}
			content := pushContent.FirstChild.Data
			for n := pushContent.FirstChild.NextSibling; n != nil; n = n.NextSibling {
				if findEmailProtected(n) != nil {
					break
				}
				if n.FirstChild != nil {
					content += n.FirstChild.Data
				}
				if n.NextSibling != nil {
					content += n.NextSibling.Data
				}
			}
			push.Content = content
		}
		pushes = append(pushes, push)
	}
	atcl.Comments = pushes
	return atcl, nil
}

var ipDateTimeRe = regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)?\s*(.*)`)

func parseDateTime(ipdatetime string) (time.Time, error) {
	subMatches := ipDateTimeRe.FindStringSubmatch(ipdatetime)
	dateTime := strings.TrimSpace(subMatches[len(subMatches)-1])
	loc := time.FixedZone("CST", 8*60*60)
	t, err := time.ParseInLocation("01/02 15:04", dateTime, loc)
	if err != nil {
		return time.Time{}, err
	}
	t = t.AddDate(getYear(t), 0, 0)
	return t, nil
}

func getYear(commentTime time.Time) int {
	t := time.Now()
	commentTime = commentTime.AddDate(t.Year(), 0, 0)
	if commentTime.After(t) {
		return t.Year() - 1
	}
	return t.Year()
}

// CheckBoardExist use for checking board exist or not
func CheckBoardExist(board string) bool {
	return checkURLExist(makeBoardURL(board, -1))
}

// existClient follows redirects, e.g. over 18 confirmation of some boards
var existClient = &http.Client{Timeout: 15 * time.Second}

func checkURLExist(url string) bool {
	req, err := pttHttp.HttpRequest(url)
	if err != nil {
		return false
	}
	resp, err := existClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// makeBoardURL makes url of board index page, the latest page when page < 0
func makeBoardURL(board string, page int) string {
	var pageStr string
	if page >= 0 {
		pageStr = strconv.Itoa(page)
	}
	return pttHostURL + "/bbs/" + board + "/index" + pageStr + ".html"
}

func makeArticleURL(board, articleCode string) string {
	return pttHostURL + "/bbs/" + board + "/" + articleCode + ".html"
}

// URLNotFoundError is an error type present 404 Not Found
type URLNotFoundError struct {
	URL string
}

func (u URLNotFoundError) Error() string {
	return "Fetched URL Not Found"
}

var errRedirect = errors.New("redirect")

var client = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return errRedirect
	},
	Timeout: 30 * time.Second,
}

func fetchHTML(reqURL string) (doc *html.Node, err error) {
	req, err := pttHttp.HttpRequest(reqURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil && resp == nil {
		log.WithField("url", reqURL).WithError(err).Error("Fetch URL Failed")
		return nil, err
	}
	defer resp.Body.Close()

	if err == nil && resp.StatusCode == http.StatusNotFound {
		err = URLNotFoundError{reqURL}
		return nil, err
	}

	if uerr, ok := err.(*url.Error); ok && uerr.Err == errRedirect {
		resp, err = client.Do(passR18(reqURL))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
	}

	// error pages (e.g. 429, 5xx) must not be parsed as boards or articles without comments
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: %s", reqURL, resp.Status)
	}

	doc, err = html.Parse(resp.Body)
	if err != nil {
		log.WithError(err).Error("Crawler Fetch HTML Failed")
		return nil, err
	}
	return doc, nil
}

// passR18 requests url of an over 18 board with the confirmation cookie
func passR18(reqURL string) *http.Request {
	req, _ := pttHttp.HttpRequest(reqURL)
	req.AddCookie(&http.Cookie{Name: "over18", Value: "1"})
	return req
}
