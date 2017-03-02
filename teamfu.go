package main

import (
	"fmt"
	"log"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/bradhe/stopwatch"

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

const maxCommits = 500

//const repositoryPath = "/Users/sovanesyan/Work/tensorflow-full"

const repositoryPath = "/Users/sovanesyan/Work/angular"

func main() {
	start := stopwatch.Start()
	// f, _ := os.Create("cpu.prof")
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()
	debug.SetGCPercent(-1)

	log.Print("Started")
	ids := findCommitIds()
	commits := calculateCommits(ids)
	for commit := range commits {
		log.Print(commit.id)

		runtime.GC()
	}

	log.Print("Finished")
	watch := stopwatch.Stop(start)
	fmt.Printf("Seconds elapsed: %v\n", watch.Milliseconds()/1000)
}

func createRepository() *git.Repository {
	repo, _ := git.OpenRepository(repositoryPath)
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
			if commitCount >= maxCommits {
				return false
			}

			channel <- *commit.Id()
			commitCount++

			return true
		})

		close(channel)
	}()
	return channel
}

func calculateCommits(ids chan git.Oid) chan Commit {
	channel := make(chan Commit)

	go func() {
		for oid := range ids {
			channel <- processCommit(oid)
			log.Print(oid.String())

			runtime.GC()
		}
		close(channel)
	}()

	return channel
}

func processCommit(oid git.Oid) Commit {
	log.Print(oid.String())
	repo := createRepository()
	commit, _ := repo.LookupCommit(&oid)
	cm := createCommitMetadata(commit)
	if commit.ParentCount() == 0 {
		diff := createDiff(commit, nil, repo)
		applyStats(&cm, diff)
		return cm
	}
	for i := uint(0); i < commit.ParentCount(); i++ {
		parent := commit.Parent(i)
		diff := createDiff(commit, parent, repo)
		applyStats(&cm, diff)

		diff.ForEach(func(delta git.DiffDelta, number float64) (git.DiffForEachHunkCallback, error) {

			return func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {

				blame := blameFile(commit, parent, &hunk, &delta, repo)
				cm.hunkChanges++
				cm.newWork += uint(hunk.NewLines - hunk.OldLines)

				return func(line git.DiffLine) error {
					if delta.NewFile.Mode == 57344 {
						log.Printf("Submodule change ignored: %v", commit.Id())
						return nil
					}
					if strings.HasPrefix(line.Content, "Subproject commit") {
						log.Printf("Submodule added ignored: %v", commit.Id())

						cm.newWork++
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
