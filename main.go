package main

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/libgit2/git2go"
)

func main() {
	router := httprouter.New()
	router.GET("/", Index)

	fmt.Println(http.ListenAndServe(":3000", router))
}
func Index(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	repo, err := git.OpenRepository("../wiki-data")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	head, err := repo.Head()
	if err != nil {
		http.Error(w, "getting HEAD: "+err.Error(), 500)
		return
	}
	commit, err := head.Peel(git.ObjectTree)
	if err != nil {
		http.Error(w, "Peeling commit: "+err.Error(), 500)
		return
	}
	tree, err := commit.AsTree()
	if err != nil {
		http.Error(w, "getting tree:"+err.Error(), 500)
		return
	}
	num := tree.EntryCount()
	fmt.Fprintln(w, num, "entries.\n---")
	for i := uint64(0); i < num; i++ {
		entry := tree.EntryByIndex(i)
		fmt.Fprintln(w, entry.Name)
	}
}
