package main

import (
	"flag"
	"os"
	"strings"

	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/cfstras/wiki-api/data"
	"github.com/julienschmidt/httprouter"
	"github.com/libgit2/git2go"
	"github.com/pkg/errors"

	"github.com/cbroglie/mustache"
	"github.com/jteeuwen/go-bindata"
)

var ErrorNotFound error = errors.New("Not Found")

type GitEntry struct {
	Name  string
	ID    string
	IsDir bool

	Handle *git.TreeEntry
}

var (
	TemplateIndexOf string

	repoPath string
	repo     *git.Repository
)

func init() {
	_ = bindata.Config{}

	TemplateIndexOf = string(data.MustAsset("indexOf.mustache"))
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s <repository>\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	if len(flag.Args()) != 1 {
		flag.Usage()
		return
	}
	repoPath = flag.Args()[0]

	var err error
	repo, err = git.OpenRepository(repoPath)
	if err != nil {
		fmt.Println("Opening repo: ", err)
		return
	}

	router := httprouter.New()
	router.GET("/*path", Index)

	listenOn := ":3000"
	fmt.Println("Listening on", listenOn)
	fmt.Println(http.ListenAndServe(listenOn, router))
}
func Index(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	path := p.ByName("path")

	entry, err := GetRepoPath(path)

	if err != nil {
		if err.Code == git.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}

	switch entry.Type() {
	case git.ObjectTree:
		if !strings.HasSuffix(path, "/") {
			http.Redirect(w, r, path+"/", 301)
			return
		}

		tree, err := entry.AsTree()
		if err != nil {
			http.Error(w, "Getting tree "+path+": "+err.Error(), 500)
			return
		}
		files := ListDirCurrent(tree)

		context := map[string]interface{}{"Files": files, "Path": path}
		html, err := mustache.Render(TemplateIndexOf, context)
		if err != nil {
			http.Error(w, "Rendering template: "+err.Error(), 500)
			return
		}
		w.Write([]byte(html))

	case git.ObjectBlob:

		blob, err := entry.AsBlob()
		if err != nil {
			http.Error(w, "Getting blob "+path+": "+err.Error(), 500)
			return
		}
		w.Write(blob.Contents())
	default:
		http.Error(w, "Unknown entry: "+entry.Type().String(), 500)
	}
}

func GetRepoPath(path string) (*git.Object, *git.GitError) {
	tree, errG := GetRootTree()
	if errG != nil {
		return nil, errG
	}
	if path == "/" || path == "" {
		return &tree.Object, nil
	}
	entry, err := tree.EntryByPath(path[1:])

	if err != nil {
		if err.(*git.GitError).Code == git.ErrNotFound {
			return nil, err.(*git.GitError)
		}
		return nil, err.(*git.GitError)
	}
	object, err := repo.Lookup(entry.Id)
	if err != nil {
		errG = err.(*git.GitError)
	}
	return object, errG
}

func ListDirCurrent(tree *git.Tree) []GitEntry {
	num := tree.EntryCount()
	list := make([]GitEntry, 0, num)

	for i := uint64(0); i < num; i++ {
		gitEntry := tree.EntryByIndex(i)
		entry := GitEntry{
			gitEntry.Name,
			hex.EncodeToString(gitEntry.Id[:]),
			gitEntry.Type == git.ObjectTree,
			gitEntry}
		list = append(list, entry)
	}
	return list
}
func GetRootTree() (*git.Tree, *git.GitError) {
	head, err := repo.Head()
	if err != nil {
		return nil, err.(*git.GitError)
	}
	commit, err := head.Peel(git.ObjectTree)
	if err != nil {
		return nil, err.(*git.GitError)
	}
	tree, err := commit.AsTree()
	if err != nil {
		return nil, err.(*git.GitError)
	}
	return tree, nil
}
