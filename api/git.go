package api

import git "github.com/libgit2/git2go"

// GetRepoPath looks up an object a tree.
// You will need to call object.Free() after usage.
func GetRepoPath(tree *git.Tree, path string) (*git.Object, *git.GitError) {
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
	var errG *git.GitError
	if err != nil {
		errG = err.(*git.GitError)
	}
	return object, errG
}

// ListDirCurrent lists entries in a tree object and returns an array.
func ListDirCurrent(tree *git.Tree) []GitEntry {
	num := tree.EntryCount()
	list := make([]GitEntry, 0, num)

	for i := uint64(0); i < num; i++ {
		gitEntry := tree.EntryByIndex(i)
		entry := GitEntry{
			gitEntry.Name,
			gitEntry.Id.String(),
			gitEntry.Type == git.ObjectTree,
			gitEntry}
		list = append(list, entry)
	}
	return list
}

// GetRootTree returns a tree object for the root directory of HEAD.
// You will need to call tree.Free() after usage.
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
