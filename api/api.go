package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/cfstras/wiki-api/data"
	"github.com/julienschmidt/httprouter"
	git "github.com/libgit2/git2go"
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
	TemplateIndexOf = string(data.MustAsset("indexOf.mustache"))
}

var debug bool

func Run(address, repoPath string, doDebug bool) error {
	debug = doDebug
	var err error
	repo, err = git.OpenRepository(repoPath)
	if err != nil {
		return err
	}

	router := httprouter.New()

	router.GET("/*path", Index)
	router.PUT("/*path", PutFile)

	fmt.Println("Listening on", address)
	return http.ListenAndServe(address, router)
}

func Index(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	path := p.ByName("path")

	if debug && strings.HasPrefix(path, "/debug/pprof/") {
		//pprof.Handler(r.RequestURI).ServeHTTP(w, r)
		pprof.Index(w, r)
		return
	}

	tree, err := GetRootTree()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tree.Free()
	//defer fmt.Println("free tree %p", tree)

	entry, err := GetRepoPath(tree, path)
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

func PutFile(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	//TODO
}
