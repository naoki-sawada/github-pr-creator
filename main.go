package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/google/go-github/v26/github"
	"github.com/thoas/go-funk"
	"golang.org/x/oauth2"
)

type commit struct {
	date time.Time
	sha  string
}

type options struct {
	DryRun bool `arg:"--dry-run" help:"dry run mode"`
}

type config struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

func parseJsonConfig(filename string, config *[]config) error {
	jsonFile, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	json.Unmarshal(byteValue, config)

	return nil
}

func latestCommit(client *github.Client, config *config, branch string) (*commit, error) {
	b, _, err := client.Repositories.GetBranch(context.Background(), config.Owner, config.Repo, branch)
	if err != nil {
		return &commit{}, err
	}

	c, _, err := client.Git.GetCommit(context.Background(), config.Owner, config.Repo, *b.Commit.SHA)
	if err != nil {
		return &commit{}, err
	}

	return &commit{date: *c.Author.Date, sha: *c.SHA}, nil
}

func needPR(client *github.Client, config *config, headSHA string, baseSHA string) (bool, error) {
	pulls, _, err := client.PullRequests.List(context.Background(), config.Owner, config.Repo, nil)
	if err != nil {
		return false, err
	}

	releasePulls := funk.Filter(pulls, func(pull *github.PullRequest) bool {
		return *pull.Base.SHA == baseSHA && strings.Contains(*pull.Title, "[NEW RELEASE]")
	})

	if len(releasePulls.([]*github.PullRequest)) > 0 {
		return false, nil
	}

	return true, nil
}

func createPR(client *github.Client, config *config) error {
	title := fmt.Sprintf("[NEW RELEASE] %s <- %s (%s)", config.Base, config.Head, time.Now().Format("2006/01/02"))
	pr := github.NewPullRequest{Title: &title, Head: &config.Head, Base: &config.Base}

	_, _, err := client.PullRequests.Create(context.Background(), config.Owner, config.Repo, &pr)
	if err != nil {
		return err
	}

	return nil
}

func newPR(client *github.Client, config config, options *options, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("Config: %+v", config)

	headCommit, err := latestCommit(client, &config, config.Head)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Head branch found: %+v", headCommit)

	baseCommit, err := latestCommit(client, &config, config.Base)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Base branch found: %+v", baseCommit)

	if baseCommit.date.Before(headCommit.date) {
		needNewPR, err := needPR(client, &config, headCommit.sha, baseCommit.sha)
		if err != nil {
			log.Fatal(err)
		}

		if needNewPR {
			if !options.DryRun {
				err := createPR(client, &config)
				if err != nil {
					log.Fatal(err)
				}
			}
			log.Printf("New PR (%s <- %s) created.", config.Base, config.Head)
		}
	}
}

func main() {
	// Parse environ variables
	token := os.Getenv("GITHUB_ACCESS_TOKEN")
	if token == "" {
		log.Fatal("Unauthorized: No token present")
	}

	// Parse arg
	var opt options
	arg.MustParse(&opt)

	// Parse config
	configFile := "app.config.json"
	var myConfig []config
	err := parseJsonConfig(configFile, &myConfig)
	if err != nil {
		log.Fatal("Failed to parse the config file: ", err)
	}

	// Create new client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Run create new PR func
	var wg sync.WaitGroup
	for _, v := range myConfig {
		wg.Add(1)
		go newPR(client, v, &opt, &wg)
	}
	wg.Wait()
}
