package api

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"

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
		http.Error(w, "Could not get tree: "+err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "Could not get path: "+err.Error(), http.StatusInternalServerError)
		return
	}

	switch entry.Type() {
	case git.ObjectTree:
		if !strings.HasSuffix(path, "/") {
			http.Redirect(w, r, path+"/", http.StatusMovedPermanently)
			return
		}

		tree, err := entry.AsTree()
		if err != nil {
			http.Error(w, "Getting tree "+path+": "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		files := ListDirCurrent(tree)

		context := map[string]interface{}{"Files": files, "Path": path}
		html, err := mustache.Render(TemplateIndexOf, context)
		if err != nil {
			http.Error(w, "Rendering template: "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		w.Write([]byte(html))

	case git.ObjectBlob:
		blob, err := entry.AsBlob()
		if err != nil {
			http.Error(w, "Getting blob "+path+": "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		w.Write(blob.Contents())
	default:
		http.Error(w, "Unknown entry: "+entry.Type().String(),
			http.StatusInternalServerError)
	}
}

func httpErrorOnPanic(w http.ResponseWriter, errorCode *int) {
	if err := recover(); err != nil {
		if _, ok := err.(runtime.Error); ok {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("in request: %+v", errors.WithStack(err.(error)))
		} else {
			http.Error(w, err.(error).Error(), *errorCode)
		}
	}
}

func PutFile(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	var currentErrorCode int
	var err error
	defer httpErrorOnPanic(w, &currentErrorCode)
	check := func(status string, errorCode int) {
		if err != nil {
			currentErrorCode = errorCode
			panic(errors.WithMessage(err, "Error "+status))
		}
	}

	path := p.ByName("path")
	lastId := r.Header.Get("Last-Id")

	if strings.HasSuffix(path, ".json") {
		http.Error(w, "Files cannot end in \".json\".", http.StatusConflict)
		return
	}

	head, err := repo.Head()
	check("getting HEAD", http.StatusInternalServerError)
	headCommitObject, err := head.Peel(git.ObjectCommit)
	check("getting HEAD", http.StatusInternalServerError)
	headCommit, err := headCommitObject.AsCommit()
	check("getting HEAD", http.StatusInternalServerError)

	oldRootTree, errG := GetTreeFromRef(head)
	check("getting HEAD tree", http.StatusInternalServerError)
	defer oldRootTree.Free()

	oldEntry, errG := GetRepoPath(oldRootTree, path)
	if errG != nil && errG.Code == git.ErrNotFound {
		oldEntry = nil
	} else if errG != nil {
		http.Error(w, "Could not get path: "+errG.Error(), http.StatusInternalServerError)
		return
	}

	if oldEntry != nil {
		switch oldEntry.Type() {
		case git.ObjectTree:
			http.Error(w, "Specified path exists and is a directory.",
				http.StatusConflict)
			return
		case git.ObjectBlob:
			switch lastId {
			case "":
				// no checks to perform
			case "null":
				http.Error(w, "lastId was null but specified path exists.",
					http.StatusConflict)
				return
			default:
				if lastId != oldEntry.Id().String() {
					http.Error(w, "lastId did not match existing entry.",
						http.StatusConflict)
					return
				}
			}
		default:
			http.Error(w, "Unknown old entry: "+oldEntry.Type().String(),
				http.StatusInternalServerError)
			return
		}
	} else {
		if lastId != "" && lastId != "null" {
			http.Error(w, "lastId specified but specified path does not exist.",
				http.StatusGone)
			return
		}
	}
	// all checks okay, add, lock, and commit!

	//TODO lock
	path = path[1:]

	content, err := ioutil.ReadAll(r.Body)
	check("receiving request", http.StatusInternalServerError)
	blobId, err := repo.CreateBlobFromBuffer(content)
	check("writing request blob", http.StatusInternalServerError)
	treeBuilder, err := repo.TreeBuilderFromTree(oldRootTree)
	check("creating new tree object", http.StatusInternalServerError)
	defer treeBuilder.Free()
	err = treeBuilder.Insert(path, blobId, git.FilemodeBlob)
	check("adding file to tree", http.StatusInternalServerError)
	treeId, err := treeBuilder.Write()
	check("writing new tree", http.StatusInternalServerError)
	tree, err := repo.LookupTree(treeId)
	check("getting new tree", http.StatusInternalServerError)

	author := &git.Signature{ //TODO add user info
		Email: "root@localhost",
		Name:  "root",
		When:  time.Now()}
	committer := author
	message := "Auto commit" //TODO add header

	commitId, err := repo.CreateCommit("HEAD", author, committer, message, tree,
		headCommit)
	check("creating commit", http.StatusInternalServerError)
	fmt.Fprintln(w, commitId)
	return
}
