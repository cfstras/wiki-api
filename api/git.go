package api

import git "github.com/libgit2/git2go"

// GetRepoPath looks up an object a tree.
// You will need to call object.Free() after usage.
func GetRepoPath(tree *git.Tree, path string) (*git.Object, error) {
	if path == "/" || path == "" {
		return &tree.Object, nil
	}
	entry, err := tree.EntryByPath(path[1:])
	if err != nil {
		return nil, err
	}

	object, err := repo.Lookup(entry.Id)
	return object, err
}

// ListDirCurrent lists entries in a tree object and returns an array.
func ListDirCurrent(tree *git.Tree) []GitEntry {
	num := tree.EntryCount()
	list := make([]GitEntry, 0, num)

	for i := uint64(0); i < num; i++ {
		gitEntry := tree.EntryByIndex(i)
		entry := GitEntry{
			gitEntry.Name,
			(*Oid)(gitEntry.Id),
			gitEntry.Type == git.ObjectTree,
			gitEntry}
		list = append(list, entry)
	}
	return list
}

// GetRootTree returns a tree object for the root directory of HEAD.
// You will need to call tree.Free() after usage.
func GetRootTree() (*git.Tree, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}
	return GetTreeFromRef(head)
}

// GetRootCommit returns a commit object for HEAD.
// You will need to call commit.Free() after usage.
func GetRootCommit() (*git.Commit, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}
	return GetCommitFromRef(head)
}

// GetTreeFromRef returns the tree associated with a reference
func GetTreeFromRef(ref *git.Reference) (*git.Tree, error) {
	treeOb, err := ref.Peel(git.ObjectTree)
	if err != nil {
		return nil, err
	}
	tree, err := treeOb.AsTree()
	if err != nil {
		return nil, err
	}
	return tree, nil
}

// GetTreeFromRef returns the commit associated with a reference
func GetCommitFromRef(ref *git.Reference) (*git.Commit, error) {
	commitOb, err := ref.Peel(git.ObjectCommit)
	if err != nil {
		return nil, err
	}
	commit, err := commitOb.AsCommit()
	if err != nil {
		return nil, err
	}
	return commit, nil
}
