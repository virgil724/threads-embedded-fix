package models

import (
	"fmt"
	"net/url"
	"strings"
)

type Post struct {
	ID      int    `json:"id"`
	RawURL  string `json:"raw_url"`
	PostID  string `json:"post_id"`
	User    string `json:"user"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Media   string `json:"media"`
}

// Example Url https://www.threads.com/@kanpingart/post/DXVymbJEvi6

// url parser

func (p *Post) ParseURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	// u.Path = "/@zuck/post/ABC123"
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 {
		return fmt.Errorf("invalid threads url: %s", rawURL)
	}
	
	u.RawQuery = ""
	p.RawURL = u.String()
	p.User = strings.TrimPrefix(parts[0], "@")
	p.PostID = parts[2]
	return nil
}
