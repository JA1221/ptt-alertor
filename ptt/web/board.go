package web

import "golang.org/x/net/html"

func findArticleBlocks(node *html.Node) *html.Node {
	return findDivByClassName(node, "r-ent")
}

func findPagingBlock(node *html.Node) *html.Node {
	return findDivByClassName(node, "btn-group btn-group-paging")
}
