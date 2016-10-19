package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
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
	defer HttpErrorOnPanic(w, http.StatusInternalServerError)
	path := p.ByName("path")
	var err error
	path, err = checkPath(path)
	Check(err, "invalid path", http.StatusBadRequest)

	if debug && strings.HasPrefix(path, "/debug/pprof/") {
		pprof.Index(w, r)
		return
	}

	tree, err := GetRootTree()
	Check(err, "getting tree", 0)
	defer tree.Free()

	entry, err := GetRepoPath(tree, path)
	if err != nil && err.(*git.GitError).Code == git.ErrNotFound {
		http.NotFound(w, r)
		return
	}
	Check(err, "getting path", 0)

	switch entry.Type() {
	case git.ObjectTree:
		if !strings.HasSuffix(path, "/") {
			http.Redirect(w, r, path+"/", http.StatusMovedPermanently)
			return
		}

		tree, err := entry.AsTree()
		Check(err, "getting tree", 0)
		files := ListDirCurrent(tree)

		// we only want to do that for the output, not the json and stuff
		if path != "/" {
			files = append([]GitEntry{{IsDir: true, Name: ".."}}, files...)
		}
		context := map[string]interface{}{"Files": files, "Path": path}
		html, err := mustache.Render(TemplateIndexOf, context)
		Check(err, "rendering template", 0)
		w.Write([]byte(html))

	case git.ObjectBlob:
		blob, err := entry.AsBlob()
		Check(err, "getting blob", 0)
		w.Write(blob.Contents())
	default:
		http.Error(w, "Unknown entry: "+entry.Type().String(),
			http.StatusInternalServerError)
	}
}

// checkPath checks that a path makes sense.
// expects
func checkPath(path string) (string, error) {
	if len(path) < 1 || path[0] != '/' {
		return "", errors.New("invalid path: has to start with '/'")
	}
	pathElements := strings.Split(path[1:], "/")
	for _, el := range pathElements {
		if el == "." || el == ".." {
			return "", errors.New("invalid path: cannot contain '.' or '..' elements")
		}
	}
	return path, nil
}

func PutFile(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer HttpErrorOnPanic(w, http.StatusInternalServerError)
	var err error

	path := p.ByName("path")
	path, err = checkPath(path)
	Check(err, "in supplied path", http.StatusBadRequest)

	lastId := r.Header.Get("Wiki-Last-Id")
	commitMsg := r.Header.Get("Wiki-Commit-Msg")

	if strings.HasSuffix(path, ".json") {
		http.Error(w, "Files cannot end in \".json\".", http.StatusConflict)
		return
	}

	head, err := repo.Head()
	Check(err, "getting HEAD", 0)
	headCommitObject, err := head.Peel(git.ObjectCommit)
	Check(err, "getting HEAD", 0)
	headCommit, err := headCommitObject.AsCommit()
	Check(err, "getting HEAD", 0)

	oldRootTree, err := GetTreeFromRef(head)
	Check(err, "getting HEAD tree", 0)
	defer oldRootTree.Free()

	oldEntry, err := GetRepoPath(oldRootTree, path)
	if err != nil && err.(*git.GitError).Code == git.ErrNotFound {
		oldEntry = nil
	} else if err != nil {
		http.Error(w, "Could not get path: "+err.Error(), 0)
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
			http.Error(w, "Unknown old entry: "+oldEntry.Type().String(), 0)
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

	content, err := ioutil.ReadAll(r.Body)
	Check(err, "receiving request", 0)
	blobId, err := repo.CreateBlobFromBuffer(content)
	Check(err, "writing request blob", 0)
	content = nil

	index, err := git.NewIndex()
	Check(err, "creating index", 0)
	Check(index.ReadTree(oldRootTree), "Adding old files to index", 0)

	entry := git.IndexEntry{
		Mode: git.FilemodeBlob,
		Size: uint32(len(content)),
		Id:   blobId,
		Path: path[1:], // without / at the beginning
	}
	Check(index.Add(&entry), "adding file to index", 0)
	treeId, err := index.WriteTreeTo(repo)
	Check(err, "writing new tree", 0)
	tree, err := repo.LookupTree(treeId)
	Check(err, "getting new tree", 0)

	author := &git.Signature{ //TODO add user info
		Email: "root@localhost",
		Name:  "root",
		When:  time.Now()}
	committer := author

	commitId, err := repo.CreateCommit("HEAD", author, committer, commitMsg, tree,
		headCommit)
	Check(err, "creating commit", 0)
	fmt.Fprintln(w, commitId)
	return
}
