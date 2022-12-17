// Copyright 2017 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The simple command demonstrates a simple functionality which
// prompts the user for a GitHub username and lists all the public
// organization memberships of the specified username.
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

func isRepoFiltered(repo *github.Repository) bool {
	// custom filter logic, set to star > 5 for now for simplicity
	return *repo.StargazersCount > 5
}

func getCommitMessase(client *github.Client, owner string, name string, branch string) (string, string) {
	defaultBranch, _, err := client.Repositories.GetBranch(context.Background(), owner, name, branch, false)
	if err != nil {
		return "", ""
	}

	commit := defaultBranch.GetCommit()

	commitMessage := strings.Split(*commit.Commit.Message, "\n")[0]
	commitAuthor := *commit.Commit.Author.Name

	return commitMessage, commitAuthor
}

func trimString(str string, length int) string {
	if len(str) > length {
		return str[:length] + "..."
	}
	return str
}

func formatTweet(client *github.Client, repo *github.Repository) string {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	stars := repo.GetStargazersCount()
	forks := repo.GetForksCount()
	url := repo.GetHTMLURL()
	description := trimString(repo.GetDescription(), 180)

	if description != "" {
		description += "\n"
	}

	commitMessage, commitAuthor := getCommitMessase(client, owner, name, repo.GetDefaultBranch())

	// refernece: https://stackoverflow.com/questions/11123865/format-a-go-string-without-printing#:~:text=2.%20Complex%20strings%20(documents)
	headingTmpl := "%s/%s (★%d) (%d forks)\n"
	commitTmpl := "Last commit: %s by %s\n"

	heading := fmt.Sprintf(headingTmpl, owner, name, stars, forks) + description
	commit := fmt.Sprintf(commitTmpl, commitMessage, commitAuthor)

	return heading + "\n" + commit + "\n" + url + "\n"
}

func tweet(client *twitter.Client, message string) {
	client.Statuses.Update(message, nil)
}

func print(client *twitter.Client, message string) {
	// client.Statuses.Update(message, nil)
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

func main() {
	godotenv.Load()

	githubClient := github.NewClient(nil)

	consumerKey := os.Getenv("TWITTER_CONSUMER_KEY")
	consumerSecret := os.Getenv("TWITTER_CONSUMER_SECRET")
	accessToken := os.Getenv("TWITTER_ACCESS_TOKEN")
	accessSecret := os.Getenv("TWITTER_ACCESS_SECRET")

	twitterClient := getTwitterClient(consumerKey, consumerSecret, accessToken, accessSecret)

	// 1. fetch latest updated repo within certain time
	repos := fetchLatestRepos(githubClient, "solidity", "3h")

	// 2. loop throught the list and filter out the repo that worth to be noticed (e.g. stars > x)
	//TODO: only the first 100 results is looped for now
	for _, repo := range repos.Repositories {
		if isRepoFiltered(repo) {
			// 3. format the info and ping on twitter
			tweet(twitterClient, formatTweet(githubClient, repo))
		}
	}
}
