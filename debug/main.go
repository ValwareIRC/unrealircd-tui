package main

import (
	"fmt"

	"github.com/tim-st/go-zim"
)

func main() {
	zimPath := "/home/valerie/ubeValware/docs/unrealircd-wiki.zim"
	f, err := zim.Open(zimPath)
	if err != nil {
		fmt.Printf("Error opening ZIM: %v\n", err)
		return
	}
	defer f.Close()

	fmt.Printf("Total articles: %d\n", f.ArticleCount())

	// Check first 20 entries
	for i := 0; i < 20 && i < int(f.ArticleCount()); i++ {
		e, err := f.EntryAtTitlePosition(uint32(i))
		if err != nil {
			fmt.Printf("Entry %d: Error: %v\n", i, err)
			continue
		}
		fmt.Printf("Entry %d: Title=%q URL=%q IsArticle=%v IsRedirect=%v Namespace=%s\n",
			i, e.Title(), e.URL(), e.IsArticle(), e.IsRedirect(), e.Namespace())
	}
}
