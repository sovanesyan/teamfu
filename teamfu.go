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

var (
	threeWeeksAgo = time.Now().Add(-3 * 7 * 24 * time.Hour)
)

func main() {
	// f, _ := os.Create("cpu.prof")
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	processRepository()

}

func processRepository() {
	repo, err := git.OpenRepository("/Users/sovanesyan/Work/tensorflow")

	if err != nil {
		log.Fatal("Could not open repository: " + err.Error())
		return
	}

	walker, err := repo.Walk()
	if err != nil {
		log.Fatal("Could not create walker: " + err.Error())
		return
	}
	oldOid := findOldEnoughCommit(*repo)
	commits := []Commit{}
	count := 0
	walker.PushHead()
	walker.Iterate(func(commit *git.Commit) bool {
		count++
		if count > 20 {
			return false
		}
		parent := commit.Parent(0) //TODO: make it so that it is aggregated
		tree, _ := commit.Tree()
		parentTree, _ := parent.Tree()

		diffOptions, _ := git.DefaultDiffOptions()
		diff, _ := repo.DiffTreeToTree(parentTree, tree, &diffOptions)
		stats, _ := diff.Stats()

		commitMetadata := new(Commit)
		commitMetadata.id = commit.Id().String()
		commitMetadata.isMerge = commit.ParentCount() != 1
		commitMetadata.authorEmail = commit.Author().Email
		commitMetadata.authorName = commit.Author().Name
		commitMetadata.createdAt = commit.Author().When
		commitMetadata.summary = commit.Summary()
		commitMetadata.insertions = uint(stats.Insertions())
		commitMetadata.deletions = uint(stats.Deletions())
		commitMetadata.filesChanged = uint(stats.FilesChanged())

		diff.ForEach(func(delta git.DiffDelta, number float64) (git.DiffForEachHunkCallback, error) {

			return func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {
				blameOptions, _ := git.DefaultBlameOptions()
				blameOptions.NewestCommit = parent.Id()
				blameOptions.OldestCommit = &oldOid
				blameOptions.MinLine = uint32(hunk.OldStart)
				blameOptions.MaxLine = uint32(hunk.OldStart + hunk.OldLines)
				blame, _ := repo.BlameFile(delta.OldFile.Path, &blameOptions)

				commitMetadata.hunkChanges++
				commitMetadata.newWork += uint(hunk.NewLines - hunk.OldLines)

				return func(line git.DiffLine) error {
					if line.NewLineno != -1 {
						return nil
					}
					hunk, _ := blame.HunkByLine(line.OldLineno)

					if threeWeeksAgo.After(hunk.FinalSignature.When) {
						commitMetadata.refactoring++
					} else if hunk.FinalSignature.Email == commitMetadata.authorEmail {
						commitMetadata.churn++
					} else {
						commitMetadata.helpedOthers++
					}
					return nil
				}, nil
			}, nil
		}, git.DiffDetailLines)

		log.Printf("%+v\n", commitMetadata)
		commits = append(commits, *commitMetadata)
		return true
	})

	log.Print(len(commits))

	// marshaledData, _ := json.Marshal(commits)
	// ioutil.WriteFile("commits", marshaledData, 0644)
	// newOid, _ := git.NewOid("9d3eebb35ae339bfc8d58f56bb912336ca733e2d")
	// commit, _ := repo.LookupCommit(newOid)

}

func findOldEnoughCommit(repo git.Repository) git.Oid {

	oid := new(git.Oid)
	walker, _ := repo.Walk()
	walker.PushHead()
	walker.Iterate(func(commit *git.Commit) bool {
		if threeWeeksAgo.After(commit.Author().When) {
			oid = commit.Id()
			log.Printf("OldOid: %v", oid)
			return false
		}
		return true
	})
	return *oid
}
