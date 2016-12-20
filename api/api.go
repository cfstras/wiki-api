package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"github.com/pkg/errors"

	"github.com/cfstras/wiki-api/data"
	"github.com/julienschmidt/httprouter"
	git "github.com/libgit2/git2go"
)

var ErrorNotFound error = errors.New("Not Found")

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
	log.Printf("repo:%+v\n", repo)

	router := httprouter.New()

	router.GET("/*path", Index)
	router.PUT("/*path", putFileHandler)

	fmt.Println("Listening on", address)
	return http.ListenAndServe(address, router)
}

func Index(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	ctx := &RequestContext{w: w}
	defer HttpErrorOnPanic(w, http.StatusInternalServerError)
	ctx.path = p.ByName("path")

	var err error
	ctx.path, err = checkPath(ctx.path)
	Check(err, "invalid path", http.StatusBadRequest)

	if debug && strings.HasPrefix(ctx.path, "/debug/pprof/") {
		pprof.Index(w, r)
		return
	}

	jsonInfo := strings.HasSuffix(ctx.path, ".json")
	if jsonInfo {
		ctx.path = strings.TrimSuffix(ctx.path, ".json")
	}

	ctx.rootCommit, err = GetRootCommit()
	Check(err, "getting commit", 0)
	defer ctx.rootCommit.Free()
	rootTreeO, err := ctx.rootCommit.Peel(git.ObjectTree)
	Check(err, "getting tree", 0)
	ctx.rootTree, err = rootTreeO.AsTree()
	Check(err, "getting tree", 0)
	defer ctx.rootTree.Free()

	entry, err := GetRepoPath(ctx.rootTree, ctx.path)
	if err != nil && err.(*git.GitError).Code == git.ErrNotFound {
		http.NotFound(w, r)
		return
	}
	Check(err, "getting path", 0)

	switch entry.Type() {
	case git.ObjectTree:
		if !strings.HasSuffix(ctx.path, "/") {
			http.Redirect(w, r, ctx.path+"/", http.StatusMovedPermanently)
			return
		}

		tree, err := entry.AsTree()
		Check(err, "getting tree", 0)
		files := ListDirCurrent(tree)

		if jsonInfo {
			renderTreeJson(ctx, entry, files)
		} else {
			renderDirListing(ctx, files)
		}
	case git.ObjectBlob:
		if jsonInfo {
			renderJsonInfo(ctx, entry)
		} else {
			blob, err := entry.AsBlob()
			Check(err, "getting blob", 0)
			w.Write(blob.Contents())
		}
	default:
		http.Error(w, "Unknown entry: "+entry.Type().String(),
			http.StatusInternalServerError)
	}
}

func renderDirListing(ctx *RequestContext, files []GitEntry) {
	// Add top-level link, but only for the dir listing.
	if ctx.path != "/" {
		files = append([]GitEntry{{IsDir: true, Name: ".."}}, files...)
	}
	context := map[string]interface{}{"Files": files, "Path": ctx.path}
	html, err := mustache.Render(TemplateIndexOf, context)
	Check(err, "rendering template", 0)
	ctx.w.Write([]byte(html))
}

func renderTreeJson(ctx *RequestContext, object *git.Object, files []GitEntry) {
	commitInfos, err := getCommitInfos(ctx.rootCommit, object, ctx.path)
	Check(err, "getting history", http.StatusInternalServerError)
	info := TreeInfo{
		FileInfo: FileInfo{
			ID:   (*Oid)(object.Id()),
			Path: ctx.path, History: commitInfos},
		Files: files}

	b, err := json.MarshalIndent(&info, "", "  ")
	Check(err, "rendering JSON", http.StatusInternalServerError)
	ctx.w.Write(b)
}

func renderJsonInfo(ctx *RequestContext, object *git.Object) {
	commitInfos, err := getCommitInfos(ctx.rootCommit, object, ctx.path)
	Check(err, "getting history", http.StatusInternalServerError)
	info := FileInfo{
		ID:   (*Oid)(object.Id()),
		Path: ctx.path, History: commitInfos}

	b, err := json.MarshalIndent(&info, "", "  ")
	Check(err, "rendering JSON", http.StatusInternalServerError)
	ctx.w.Write(b)
}

func getCommitInfos(parentCommit *git.Commit, object *git.Object, path string) ([]CommitInfo, error) {
	// get commit info
	walk, err := repo.Walk()
	if err != nil {
		return nil, err
	}
	defer walk.Free()
	if err = walk.Push(parentCommit.Id()); err != nil {
		return nil, err
	}
	walk.Sorting(git.SortTime | git.SortTopological)
	walk.SimplifyFirstParent()
	walk.Hide(object.Id())
	walk.HideGlob("tags/*")

	res := []CommitInfo{}
	var currentCommitId git.Oid
	var currentFileId *git.Oid
	// walk backwards in history
	for {
		err = walk.Next(&currentCommitId)
		if err != nil {
			break
		}

		// get path at at that commit
		currentCommit, err := repo.LookupCommit(&currentCommitId)
		if err != nil {
			return nil, err
		}
		currentTreeO, err := currentCommit.Peel(git.ObjectTree)
		if err != nil {
			return nil, err
		}
		currentTree, err := currentTreeO.AsTree()
		if err != nil {
			return nil, err
		}

		objectAtCommit, err := GetRepoPath(currentTree, path)
		if err != nil && err.(*git.GitError).Code == git.ErrNotFound {
			// file appeared the commit before
			break
		}
		if err != nil {
			return nil, err
		}
		if objectAtCommit == nil {
			break
		}
		if currentFileId != nil && objectAtCommit.Id().Equal(currentFileId) {
			// file did not change at that revision
			continue
		}
		currentFileId = objectAtCommit.Id()
		// if we arrive here, the file changed at this revision! mark it!

		commit, err := repo.LookupCommit(&currentCommitId)
		if err != nil {
			return nil, err
		}
		info := CommitInfo{
			(*Oid)(commit.Id()),
			commit.Author().When,
			strings.TrimSpace(commit.Message()),
			AuthorInfo{commit.Author().Name, commit.Author().Email},
		}
		res = append(res, info)
	}
	if err != nil && err.(*git.GitError).Code != git.ErrIterOver {
		return nil, err
	}
	return res, nil
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

func putFileHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer HttpErrorOnPanic(w, http.StatusInternalServerError)
	var err error

	path := p.ByName("path")
	path, err = checkPath(path)
	Check(err, "in supplied path", http.StatusBadRequest)

	lastId := r.Header.Get("Wiki-Last-Id")
	commitMsg := r.Header.Get("Wiki-Commit-Msg")

	err, code := PutFile(path, lastId, commitMsg, r.Body)
	if err != nil {
		http.Error(w, err.Error(), code)
	}
}

func PutFile(path, lastId, commitMsg string, body io.Reader) (error, int) {
	if strings.HasSuffix(path, ".json") {
		return errors.New("Files cannot end in \".json\"."), http.StatusConflict
	}

	var headCommits []*git.Commit
	var oldRootTree *git.Tree
	if head, err := repo.Head(); err == nil {
		headCommitObject, err := head.Peel(git.ObjectCommit)
		Check(err, "getting HEAD", 0)
		headCommit, err := headCommitObject.AsCommit()
		Check(err, "getting HEAD", 0)
		headCommits = append(headCommits, headCommit)

		oldRootTree, err = GetTreeFromRef(head)
		Check(err, "getting HEAD tree", 0)
		defer oldRootTree.Free()

		oldEntry, err := GetRepoPath(oldRootTree, path)
		if err != nil && err.(*git.GitError).Code == git.ErrNotFound {
			oldEntry = nil
		} else if err != nil {
			return errors.New("Could not get path: " + err.Error()), 0
		}

		if oldEntry != nil {
			switch oldEntry.Type() {
			case git.ObjectTree:
				return errors.New("Specified path exists and is a directory."),
					http.StatusConflict

			case git.ObjectBlob:
				switch lastId {
				case "":
					// no checks to perform
				case "null":
					return errors.New("lastId was null but specified path exists."),
						http.StatusConflict
				default:
					if lastId != oldEntry.Id().String() {
						return errors.New("lastId did not match existing entry."),
							http.StatusConflict
					}
				}
			default:
				return errors.New("Unknown old entry: " + oldEntry.Type().String()), 0
			}
		} else {
			if lastId != "" && lastId != "null" {
				return errors.New("lastId specified but specified path does not exist."),
					http.StatusGone
			}
		}
	} else {
		if lastId != "" && lastId != "null" {
			return errors.New("lastId specified but no commit exists."),
				http.StatusGone
		}
	}
	// all checks okay, add, lock, and commit!

	//TODO lock

	content, err := ioutil.ReadAll(body)
	Check(err, "receiving request", 0)
	blobId, err := repo.CreateBlobFromBuffer(content)
	Check(err, "writing request blob", 0)
	content = nil

	index, err := git.NewIndex()
	Check(err, "creating index", 0)
	if oldRootTree != nil {
		Check(index.ReadTree(oldRootTree), "Adding old files to index", 0)
	}

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
		headCommits...)
	Check(err, "creating commit", 0)
	return errors.New(commitId.String()), http.StatusOK
}
