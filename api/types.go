package api

import (
	"net/http"
	"time"

	git "github.com/libgit2/git2go"
)

type Oid git.Oid

type RequestContext struct {
	w          http.ResponseWriter
	path       string
	rootTree   *git.Tree
	rootCommit *git.Commit
}

type GitEntry struct {
	Name  string
	ID    *Oid
	IsDir bool

	Handle *git.TreeEntry `json:"-"`
}

type AuthorInfo struct {
	Name, Email string
}

type CommitInfo struct {
	ID        *Oid
	Date      time.Time
	CommitMsg string
	Author    AuthorInfo
}

type FileInfo struct {
	Path    string
	ID      *Oid
	History []CommitInfo
}

type TreeInfo struct {
	FileInfo
	Files []GitEntry
}

func (id Oid) MarshalJSON() ([]byte, error) {
	return []byte(`"` + id.String() + `"`), nil
}
func (id *Oid) String() string {
	if id == nil {
		return ""
	}
	return (*git.Oid)(id).String()
}
