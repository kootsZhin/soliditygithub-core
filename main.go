package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/coreos/pkg/flagutil"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/google/go-github/v48/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

func getTimeRange(duration string) (string, string) {
	timeDelta, _ := time.ParseDuration(duration)

	endTime := time.Now().UTC()
	startTime := endTime.Add(-timeDelta)

	endTimeString := endTime.Format("2006-01-02T15:04:05Z")
	startTimeString := startTime.Format("2006-01-02T15:04:05Z")

	return startTimeString, endTimeString
}

func fetchLatestRepos(client *github.Client, language string, duration string) *github.RepositoriesSearchResult {
	startTime, endTime := getTimeRange(duration)

	query := "pushed:" + startTime + ".." + endTime + " language:" + language
	searchOptions := github.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	res, _, _ := client.Search.Repositories(context.Background(), query, &searchOptions)

	return res
}

func isRepoFiltered(repo *github.Repository, commit *github.RepositoryCommit) bool {
	// custom filter logic, set to star > 10 for now for simplicity
	stargazerFilter := repo.GetStargazersCount() > 10

	author := commit.GetAuthor()
	nonUserFilter := author.GetLogin() != ""

	privateRepoFilter := !repo.GetPrivate()

	return stargazerFilter && nonUserFilter && privateRepoFilter
}

func getLastCommit(client *github.Client, owner string, name string, branch string) *github.RepositoryCommit {
	defaultBranch, _, err := client.Repositories.GetBranch(context.Background(), owner, name, branch, false)
	if err != nil {
		return nil
	}

	return defaultBranch.GetCommit()
}

func getCommitMessage(commit *github.RepositoryCommit) (string, string) {
	commitMessage := strings.Split(commit.GetCommit().GetMessage(), "\n")[0]

	commitAuthor := commit.GetAuthor()
	authorName := commitAuthor.GetLogin()

	return commitMessage, authorName
}

func trimString(str string, length int) string {
	if len(str) > length {
		return str[:length] + "..."
	}
	return str
}

func formatTweet(repo *github.Repository, commit *github.RepositoryCommit) string {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	stars := repo.GetStargazersCount()
	forks := repo.GetForksCount()
	url := repo.GetHTMLURL()
	description := trimString(repo.GetDescription(), 180)

	if description != "" {
		description += "\n"
	}

	commitMessage, commitAuthor := getCommitMessage(commit)

	// refernece: https://stackoverflow.com/questions/11123865/format-a-go-string-without-printing#:~:text=2.%20Complex%20strings%20(documents)
	headingTmpl := "%s/%s (★%d) (%d forks)\n"
	commitTmpl := "Last commit: %s by %s\n"

	heading := fmt.Sprintf(headingTmpl, owner, name, stars, forks) + description
	commitMsg := fmt.Sprintf(commitTmpl, commitMessage, commitAuthor)

	return heading + "\n" + commitMsg + "\n" + url + "\n"
}

func tweet(client *twitter.Client, message string) {
	client.Statuses.Update(message, nil)
}

func print(client *twitter.Client, message string) {
	fmt.Println(message)
	fmt.Println("====================================\n")
}

func getTwitterClient(_consumerKey string, _consumerSecret string, _accessToken string, _accessSecret string) *twitter.Client {
	flags := flag.NewFlagSet("user-auth", flag.ExitOnError)
	consumerKey := flags.String("consumer-key", _consumerKey, "Twitter Consumer Key")
	consumerSecret := flags.String("consumer-secret", _consumerKey, "Twitter Consumer Secret")
	accessToken := flags.String("access-token", _accessToken, "Twitter Access Token")
	accessSecret := flags.String("access-secret", _accessToken, "Twitter Access Secret")
	flags.Parse(os.Args[1:])
	flagutil.SetFlagsFromEnv(flags, "TWITTER")

	if *consumerKey == "" || *consumerSecret == "" || *accessToken == "" || *accessSecret == "" {
		log.Fatal("Consumer key/secret and Access token/secret required")
	}

	config := oauth1.NewConfig(*consumerKey, *consumerSecret)
	token := oauth1.NewToken(*accessToken, *accessSecret)
	// OAuth1 http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)

	return twitter.NewClient(httpClient)
}

func getGithubClient(_accessToken string) *github.Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: _accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}
func main() {
	godotenv.Load()

	githubAccessToken := os.Getenv("API_GITHUB_ACCESS_TOKEN")

	githubClient := getGithubClient(githubAccessToken)

	consumerKey := os.Getenv("TWITTER_CONSUMER_KEY")
	consumerSecret := os.Getenv("TWITTER_CONSUMER_SECRET")
	accessToken := os.Getenv("TWITTER_ACCESS_TOKEN")
	accessSecret := os.Getenv("TWITTER_ACCESS_SECRET")

	twitterClient := getTwitterClient(consumerKey, consumerSecret, accessToken, accessSecret)

	// 1. fetch latest updated repo within certain time
	repos := fetchLatestRepos(githubClient, "solidity", "8h")

	// 2. loop throught the list and filter out the repo that worth to be noticed (e.g. stars > x)
	//TODO: only the first 100 results is looped for now
	for _, repo := range repos.Repositories {
		commit := getLastCommit(githubClient, repo.GetOwner().GetLogin(), repo.GetName(), repo.GetDefaultBranch())
		if isRepoFiltered(repo, commit) {
			// 3. format the info and ping on twitter
			tweet(twitterClient, formatTweet(repo, commit))
		}
	}
}
