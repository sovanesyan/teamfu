package main

import (
	"fmt"
	"log"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/bradhe/stopwatch"

	"time"

	"flag"

	"encoding/csv"

	"os"

	"strconv"

	git "gopkg.in/libgit2/git2go.v25"
)

type Commit struct {
	id           string
	authorEmail  string
	authorName   string
	isMerge      bool
	createdAt    time.Time
	insertions   uint64
	deletions    uint64
	filesChanged uint64
	hunkChanges  uint64

	newWork       uint64
	contribute    uint64
	legacy1m3m    uint64
	legacy3m6m    uint64
	legacy6m12m   uint64
	legacy12m24m  uint64
	legacy24m48m  uint64
	legacyOver48m uint64
	refactoring   uint64
	summary       string
}

func getHeaders() []string {
	return []string{
		"id",
		"authorEmail",
		"authorName",
		"isMerge",
		"created at",

		"insertions",
		"deletions",
		"filesChanged",
		"hunkChanges",

		"newWork",
		"contribute",
		"legacy3m6m",
		"legacy6m12m",
		"legacy12m24m",
		"legacy24m48m",
		"legacyOver48m",
		"refactoring",

		"summary",
	}
}
func (commit *Commit) toSlice() []string {
	return []string{
		commit.id,
		commit.authorEmail,
		commit.authorName,
		strconv.FormatBool(commit.isMerge),
		strconv.FormatInt(commit.createdAt.Unix(), 10),

		strconv.FormatUint(commit.insertions, 10),
		strconv.FormatUint(commit.deletions, 10),
		strconv.FormatUint(commit.filesChanged, 10),
		strconv.FormatUint(commit.hunkChanges, 10),

		strconv.FormatUint(commit.newWork, 10),
		strconv.FormatUint(commit.contribute, 10),
		strconv.FormatUint(commit.legacy3m6m, 10),
		strconv.FormatUint(commit.legacy6m12m, 10),
		strconv.FormatUint(commit.legacy12m24m, 10),
		strconv.FormatUint(commit.legacy24m48m, 10),
		strconv.FormatUint(commit.legacyOver48m, 10),
		strconv.FormatUint(commit.refactoring, 10),

		strings.Replace(commit.summary, ",", ".", 1),
	}
}

const maxCommits = 500

//const repositoryPath = "/Users/sovanesyan/Work/tensorflow-full"

const repositoryPath = "/Users/sovanesyan/Work/angular"

func main() {
	repositoryPtr := flag.String("repository", repositoryPath, "Repository to parse")
	flag.Parse()
	// Arrange
	createRepository := func() *git.Repository {
		repo, _ := git.OpenRepository(*repositoryPtr)
		return repo
	}
	debug.SetGCPercent(-1)

	// Act
	start := stopwatch.Start()

	// debug
	// oid, _ := git.NewOid("5ba55b0e043a3a421ce94a6f56dd97aa17529227")
	// log.Printf("%+v", processCommit(createRepository, *oid))

	log.Print("Started")
	ids := findCommitIds(createRepository)
	commits := calculateCommits(createRepository, ids)
	writeToFile(commits)

	log.Print("Finished")
	watch := stopwatch.Stop(start)
	fmt.Printf("Seconds elapsed: %v\n", watch.Milliseconds()/1000)
}

func findCommitIds(createRepository func() *git.Repository) chan git.Oid {
	repo := createRepository()
	walker, _ := repo.Walk()

	commitCount := 0
	walker.PushHead()
	//walker.Sorting(git.SortReverse)
	channel := make(chan git.Oid)

	go func() {
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

func calculateCommits(createRepository func() *git.Repository, ids chan git.Oid) chan Commit {
	channel := make(chan Commit)
	go func() {
		for oid := range ids {
			log.Print(oid.String())
			channel <- processCommit(createRepository, oid)

			runtime.GC()
		}
		close(channel)
	}()

	return channel
}

func writeToFile(commits chan Commit) {
	outputFile, err := os.OpenFile("output.csv", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return
	}

	defer outputFile.Close()
	writer := csv.NewWriter(outputFile)
	writer.Write(getHeaders())

	for commit := range commits {
		writer.Write(commit.toSlice())

		writer.Flush()
		runtime.GC()
	}
}

func processCommit(createRepository func() *git.Repository, oid git.Oid) Commit {
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
			cm.filesChanged++
			return func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {

				blame := blameFile(commit, parent, &hunk, &delta, repo)
				cm.hunkChanges++

				if hunk.NewLines > hunk.OldLines {
					cm.newWork += uint64(hunk.NewLines - hunk.OldLines)
				}

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
					if line.OldLineno == -1 || (line.NewLineno != -1 && line.OldLineno != -1) {
						return nil
					}

					hunk, _ := blame.HunkByLine(line.OldLineno)
					between := commit.Author().When.Sub(hunk.FinalSignature.When)
					months := between.Hours() / (30 * 24)
					switch {
					case months <= 1 && hunk.FinalSignature.Email == cm.authorEmail:
						cm.refactoring++
					case months <= 1 && hunk.FinalSignature.Email != cm.authorEmail:
						cm.contribute++
					case months > 1 && months <= 3:
						cm.legacy1m3m++
					case months > 3 && months <= 6:
						cm.legacy3m6m++
					case months > 6 && months <= 12:
						cm.legacy6m12m++
					case months > 12 && months <= 24:
						cm.legacy12m24m++
					case months > 24 && months <= 48:
						cm.legacy24m48m++
					case months > 48:
						cm.legacyOver48m++
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
	cm.insertions += uint64(stats.Insertions())
	cm.deletions += uint64(stats.Deletions())
	cm.filesChanged += uint64(stats.FilesChanged())
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
