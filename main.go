package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v26/github"
	"github.com/kelseyhightower/envconfig"
	"github.com/thoas/go-funk"
	"golang.org/x/oauth2"
)

type commit struct {
	date time.Time
	sha  string
}

type env struct {
	Token          string `envconfig:"GITHUB_ACCESS_TOKEN" required:"true"`
	Key            string `envconfig:"GITHUB_KEY" required:"true"`
	IntegrationId  int    `envconfig:"GITHUB_INTEGRATION_ID" required:"true"`
	InstallationId int    `envconfig:"GITHUB_INSTALLATION_ID" required:"true"`
	ConfigURL      string `envconfig:"CONFIG_URL" required:"false"`
	ConfigURLToken string `envconfig:"CONFIG_URL_TOKEN" required:"false"`
}

type options struct {
	DryRun bool `arg:"--dry-run" help:"dry run mode"`
}

type config struct {
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Head      string   `json:"head"`
	Base      string   `json:"base"`
	Reviewers []string `json:"reviewers"`
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

func parseJsonConfigFromURL(url string, token string, config *[]config) error {
	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	if token == "" {
		authzHeader := fmt.Sprintf("Bearer %s", token)
		request.Header.Set("Authorization", authzHeader)
	}

	res, err := client.Do(request)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf("HTTP Error: %d", res.StatusCode))
	}

	if err := json.NewDecoder(res.Body).Decode(config); err != nil {
		return err
	}

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

	pull, _, err := client.PullRequests.Create(context.Background(), config.Owner, config.Repo, &pr)
	if err != nil {
		return err
	}

	reviewers := github.ReviewersRequest{Reviewers: config.Reviewers}
	_, _, err = client.PullRequests.RequestReviewers(context.Background(), config.Owner, config.Repo, *pull.Number, reviewers)
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
	var goenv env
	err := envconfig.Process("env", &goenv)
	if err != nil {
		log.Fatal(err.Error())
	}

	// Parse arg
	var opt options
	arg.MustParse(&opt)

	// Parse config
	configFile := "app.config.json"
	var myConfig []config

	if goenv.ConfigURL == "" {
		err = parseJsonConfigFromURL(goenv.ConfigURL, goenv.ConfigURLToken, &myConfig)
		if err != nil {
			log.Fatal("Failed to parse the config file: ", err)
		}
	} else {
		err = parseJsonConfig(configFile, &myConfig)
		if err != nil {
			log.Fatal("Failed to parse the config file: ", err)
		}
	}

	// Create new client
	ctx := context.Background()
	var client *github.Client
	if goenv.Token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: goenv.Token})
		tc := oauth2.NewClient(ctx, ts)
		client = github.NewClient(tc)
		log.Println("Create client with token")
	} else if goenv.Key != "" {
		// Validate environment variables
		if goenv.IntegrationId == 0 || goenv.InstallationId == 0 {
			log.Fatal("`GITHUB_INTEGRATION_ID` and `GITHUB_INSTALLATION_ID` is required.")
		}
		if goenv.IntegrationId < 0 || goenv.InstallationId < 0 {
			log.Fatal("`GITHUB_INTEGRATION_ID` or `GITHUB_INSTALLATION_ID` is invalid.")
		}

		// Decode base64 env
		dec, err := base64.StdEncoding.DecodeString(goenv.Key)
		if err != nil {
			log.Fatal("Failed to decode key", err.Error())
		}

		// Create new github client
		itr, err := ghinstallation.New(http.DefaultTransport, goenv.IntegrationId, goenv.InstallationId, dec)
		if err != nil {
			log.Fatal("Failed to get key file: ", err)
		}
		client = github.NewClient(&http.Client{Transport: itr})
		log.Println("Create client with key")
	} else {
		client = github.NewClient(nil)
		log.Println("Create client with no config")
	}

	// Run create new PR func
	var wg sync.WaitGroup
	for _, v := range myConfig {
		wg.Add(1)
		go newPR(client, v, &opt, &wg)
	}
	wg.Wait()
}
