package main

import (
	"log"

	"time"

	git "gopkg.in/libgit2/git2go.v25"
)

type Commit struct {
	id           string
	authorEmail  string
	authorName   string
	summary      string
	isMerge      bool
	createdAt    time.Time
	insertions   uint
	deletions    uint
	filesChanged uint
	hunkChanges  uint
	newWork      uint
	helpedOthers uint
	refactoring  uint
	churn        uint
}

func main() {
	// f, _ := os.Create("cpu.prof")
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	log.Print("Started")
	processRepository()
	log.Print("Finished")

	// oid, _ := git.NewOid("ea2aac69da10fed4acac18dc291790433d9af0ac")
	// commit, _ := repo.LookupCommit(oid)
	// cm := processCommit(commit, repo)
	// log.Print(cm)
}

func processRepository() {

	ids := findCommitIds()

	commits := make(map[string]Commit)
	for oid := range ids {
		commits[oid.String()] = processCommit(oid)
		log.Print(oid.String())
	}
}

func createRepository() *git.Repository {
	repo, _ := git.OpenRepository("/Users/sovanesyan/Work/tensorflow-full")
	return repo
}

func findCommitIds() chan git.Oid {
	repo := createRepository()
	walker, _ := repo.Walk()

	commitCount := 0

	walker.PushHead()
	walker.Sorting(git.SortReverse)
	channel := make(chan git.Oid)

	go func() {
		log.Print("something")
		walker.Iterate(func(commit *git.Commit) bool {

			if commitCount >= 500 {
				close(channel)
				return false
			}

			channel <- *commit.Id()
			commitCount++

			return true
		})
	}()
	log.Print("bohos")
	return channel
}

func processCommit(oid git.Oid) Commit {
	repo := createRepository()
	defer repo.Free()
	commit, _ := repo.LookupCommit(&oid)
	defer commit.Free()
	cm := createCommitMetadata(commit)
	if commit.ParentCount() == 0 {
		diff := createDiff(commit, nil, repo)
		applyStats(&cm, diff)
		return cm
	}
	for i := uint(0); i < commit.ParentCount(); i++ {
		parent := commit.Parent(i)
		diff := createDiff(commit, commit.Parent(i), repo)
		applyStats(&cm, diff)

		diff.ForEach(func(delta git.DiffDelta, number float64) (git.DiffForEachHunkCallback, error) {
			return func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {

				blame := blameFile(commit, parent, &hunk, &delta, repo)

				cm.hunkChanges++
				cm.newWork += uint(hunk.NewLines - hunk.OldLines)

				return func(line git.DiffLine) error {
					if delta.NewFile.Mode == 57344 {
						log.Printf("Panic averted: %v", commit.Id())
						return nil
					}
					if line.NewLineno != -1 {
						return nil
					}
					hunk, _ := blame.HunkByLine(line.OldLineno)

					if commit.Author().When.Add(-3 * 7 * 24 * time.Hour).After(hunk.FinalSignature.When) {
						cm.refactoring++
					} else if hunk.FinalSignature.Email == cm.authorEmail {
						cm.churn++
					} else {
						cm.helpedOthers++
					}
					return nil
				}, nil
			}, nil
		}, git.DiffDetailLines)
	}
	return cm
}

func findOldEnoughCommit(commit git.Commit) *git.Commit {
	current := commit
	for current.Author().When.After(commit.Author().When.Add(-3 * 7 * 24 * time.Hour)) {
		if current.ParentCount() == 0 {
			break
		}
		current = *current.Parent(0)
	}

	return &current
}

func createCommitMetadata(commit *git.Commit) Commit {
	commitMetadata := new(Commit)
	commitMetadata.id = commit.Id().String()
	commitMetadata.isMerge = commit.ParentCount() != 1
	commitMetadata.authorEmail = commit.Author().Email
	commitMetadata.authorName = commit.Author().Name
	commitMetadata.createdAt = commit.Author().When
	commitMetadata.summary = commit.Summary()

	return *commitMetadata
}

func createDiff(commit, parent *git.Commit, repo *git.Repository) git.Diff {
	tree, _ := commit.Tree()
	var parentTree *git.Tree
	if parent != nil {
		parentTree, _ = parent.Tree()
	}

	diffOptions, _ := git.DefaultDiffOptions()
	diffOptions.IgnoreSubmodules = git.SubmoduleIgnoreAll
	diff, _ := repo.DiffTreeToTree(parentTree, tree, &diffOptions)

	return *diff
}

func applyStats(cm *Commit, diff git.Diff) {
	stats, _ := diff.Stats()
	cm.insertions += uint(stats.Insertions())
	cm.deletions += uint(stats.Deletions())
	cm.filesChanged += uint(stats.FilesChanged())
}

func blameFile(commit, parent *git.Commit, hunk *git.DiffHunk, delta *git.DiffDelta, repo *git.Repository) *git.Blame {
	oldCommit := findOldEnoughCommit(*commit)

	blameOptions, _ := git.DefaultBlameOptions()
	blameOptions.NewestCommit = parent.Id()
	blameOptions.OldestCommit = oldCommit.Id()
	blameOptions.MinLine = uint32(hunk.OldStart)
	blameOptions.MaxLine = uint32(hunk.OldStart + hunk.OldLines)
	blame, _ := repo.BlameFile(delta.OldFile.Path, &blameOptions)

	return blame
}
