package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/scottmcmaster/twitter-pruner/pruner"
)

// results is the data format for the results output file.
type results struct {
	// DeletedTweets is the list of tweets deleted in the pruning operation.
	DeletedTweets []*twitter.Tweet `json:"deletedTweets"`
}

func isAgedOut(t *twitter.Tweet, env *pruner.Env) bool {
	createdTime, _ := t.CreatedAtTime()

	return env.MaxAge.After(createdTime)
}

func isBoring(t *twitter.Tweet, env *pruner.Env) bool {
	if env.AllRts && t.Retweeted {
		return true
	}
	if t.FavoriteCount >= env.Favs || t.RetweetCount >= env.Rts {
		rt := ""
		if t.Retweeted {
			rt += "re"
		}
		if env.Verbose {
			fmt.Printf("Ignoring %vtweet (%v fav/%v rt): %v\n", rt, t.FavoriteCount, t.RetweetCount, t.Text)
		}
		return false
	}
	return true
}

func calcTweetsToDelete(tweets []twitter.Tweet, env *pruner.Env) []*twitter.Tweet {
	var tweetsToDelete []*twitter.Tweet
	for _, tweet := range tweets {
		if isAgedOut(&tweet, env) && isBoring(&tweet, env) {
			if env.Verbose {
				fmt.Printf("%v --- %v %v --- %v\n", tweet.CreatedAt, tweet.FavoriteCount, tweet.RetweetCount, tweet.Text)
			}
			tweetsToDelete = append(tweetsToDelete, &tweet)
		}
	}
	return tweetsToDelete
}

func deleteTweets(c *pruner.Client, tweets []*twitter.Tweet) ([]*twitter.Tweet, int) {
	deletedTweets := []*twitter.Tweet{}
	errorCount := 0
	if c.Env.Commit {
		for _, tweet := range tweets {
			err := c.DestroyTweet(tweet.ID)
			if err != nil {
				if c.Env.Verbose {
					fmt.Printf("\n")
				}
				fmt.Printf("Error removing status: %v\n", err)
				errorCount++
				continue
			}
			if c.Env.Verbose {
				fmt.Printf(".")
			}
			deletedTweets = append(deletedTweets, tweet)
		}
	}
	return deletedTweets, errorCount
}

// writeResultsFile writes the results of pruning to a JSON file.
func writeResultsFile(deletedTweets []*twitter.Tweet) (string, error) {
	results := &results{
		DeletedTweets: deletedTweets,
	}
	file, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		return "", err
	}

	filename := fmt.Sprintf("twitter_pruner_%v.json", time.Now().Unix())
	err = ioutil.WriteFile(filename, file, 0644)
	if err != nil {
		return "", err
	}
	return filename, nil
}

// PruneTimeline does exactly what it says it does
func PruneTimeline(c *pruner.Client, user *twitter.User) error {
	var max int64
	count := 0
	markedForRemoval := 0
	removed := 0
	errorCount := 0
	shouldContinue := true
	allDeletedTweets := []*twitter.Tweet{}

	for shouldContinue {
		c.Env.MaxAPICalls--
		tweets, err := c.GetTimeline(max)
		if err != nil {
			fmt.Printf("Error in timeline retrieval: %+v", err)
			errorCount++
		}
		count += len(tweets)

		tweetsToDelete := calcTweetsToDelete(tweets, c.Env)
		markedForRemoval += len(tweetsToDelete)

		deletedTweets, errs := deleteTweets(c, tweetsToDelete)
		if c.Env.Commit && c.Env.Verbose {
			fmt.Printf("\n")
		}
		c.Env.MaxAPICalls -= len(deletedTweets)
		removed += len(deletedTweets)
		errorCount += errs
		for _, deletedTweet := range deletedTweets {
			allDeletedTweets = append(allDeletedTweets, deletedTweet)
		}

		if errorCount < 20 && len(tweets) > 0 && c.Env.MaxAPICalls > 0 {
			max = tweets[len(tweets)-1].ID - 1
			if c.Env.Verbose {
				fmt.Printf("%v errs -- %v tweets -- %v calls left -- %v current id\n", errorCount, len(tweets), c.Env.MaxAPICalls, max)
			}
		} else {
			shouldContinue = false
		}
	}

	if c.Env.SaveToFile {
		filename, err := writeResultsFile(allDeletedTweets)
		if err == nil {
			fmt.Printf("\nWrote results file to: %v\n", filename)
		} else {
			fmt.Printf("\nError writing results file: %v\n", err)
		}
	}

	fmt.Printf("\nTotal Scanned Tweets: %v; Removed: %v of %v; Max Age: %v\n", count, removed, markedForRemoval, c.Env.MaxAge.Format(time.RFC3339))

	return nil
}
