package file

import (
	"encoding/json"
	"io/ioutil"
	"log"

	"github.com/liam-lai/ptt-alertor/models/ptt/article"
	"github.com/liam-lai/ptt-alertor/models/ptt/board"
	"github.com/liam-lai/ptt-alertor/myutil"
)

type Board struct {
	board.Board
}

var articlesDir string = myutil.StoragePath() + "/articles/"

func (bd Board) All() []*Board {
	files, _ := ioutil.ReadDir(articlesDir)
	bds := make([]*Board, 0)
	for _, file := range files {
		name, ok := myutil.JsonFile(file)
		if !ok {
			continue
		}
		bd := new(Board)
		bd.Name = name
		bds = append(bds, bd)
	}
	return bds
}

func (bd Board) GetArticles() []article.Article {
	articlesJSON, err := ioutil.ReadFile(articlesDir + bd.Name + ".json")
	if err != nil {
		log.Fatal(err)
	}
	articles := make([]article.Article, 0)
	json.Unmarshal(articlesJSON, &articles)
	return articles
}

func (bd *Board) WithArticles() {
	bd.Articles = bd.GetArticles()
}

func (bd *Board) WithNewArticles() {
	bd.NewArticles = board.NewArticles(bd)
}

func (bd Board) Create() error {
	err := ioutil.WriteFile(articlesDir+bd.Name+".json", []byte("[]"), 664)
	return err
}

func (bd Board) Save() error {
	articlesJSON, err := json.Marshal(bd.Articles)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile(articlesDir+bd.Name+".json", articlesJSON, 0644)
	if err != nil {
		log.Fatal(err)
	}
	return err
}
