package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

type Client struct {
    client *github.Client
    owner  string
    repo   string
}

func NewClient(token, owner, repo string) *Client {
    ts := oauth2.StaticTokenSource(
        &oauth2.Token{AccessToken: token},
    )
    tc := oauth2.NewClient(context.Background(), ts)
    
    return &Client{
        client: github.NewClient(tc),
        owner:  owner,
        repo:   repo,
    }
}

func (c *Client) PostComment(ctx context.Context, prNumber int, body string) error {
    // Check if similar comment already exists
    if c.commentExists(ctx, prNumber, body) {
        fmt.Printf("⚠️ Similar comment already exists, skipping duplicate\n")
        return nil
    }

    comment := &github.IssueComment{
        Body: &body,
    }

    _, _, err := c.client.Issues.CreateComment(ctx, c.owner, c.repo, prNumber, comment)
    if err != nil {
        return fmt.Errorf("failed to post comment: %w", err)
    }

    return nil
}

func (c *Client) commentExists(ctx context.Context, prNumber int, newBody string) bool {
    // Get existing comments
    comments, _, err := c.client.Issues.ListComments(ctx, c.owner, c.repo, prNumber, nil)
    if err != nil {
        return false // If we can't check, allow posting
    }

    // Check for coffee message or Ollama review duplicates
    isCoffeeMessage := strings.Contains(newBody, "Grab a coffee")
    isOllamaReview := strings.Contains(newBody, "🦙 Ollama Code Review")

    for _, comment := range comments {
        if comment.Body == nil {
            continue
        }
        
        existingBody := *comment.Body
        
        // Check for duplicate coffee messages
        if isCoffeeMessage && strings.Contains(existingBody, "Grab a coffee") {
            return true
        }
        
        // Check for duplicate Ollama reviews
        if isOllamaReview && strings.Contains(existingBody, "🦙 Ollama Code Review") {
            return true
        }
    }

    return false
}