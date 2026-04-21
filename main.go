package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"threads-scraper/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open("threads.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect database:", err)
	}
	// auto migrate the Post model
	if err := db.AutoMigrate(&models.Post{}); err != nil {
		log.Fatal("failed to migrate database:", err)
	}
}

var metaPropertyRe = regexp.MustCompile(`(?i)<meta[^>]+property=["']([^"']+)["'][^>]+content=["']([^"']*)["']`)
var metaContentRe = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']*)["'][^>]+property=["']([^"']+)["']`)

func extractOGMeta(body, property string) string {
	for _, re := range []*regexp.Regexp{metaPropertyRe, metaContentRe} {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			prop, content := m[1], m[2]
			if re == metaContentRe {
				prop, content = m[2], m[1]
			}
			if strings.EqualFold(prop, property) {
				return html.UnescapeString(content)
			}
		}
	}
	return ""
}

func scrapePost(post *models.Post) error {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", post.RawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "facebookexternalhit/1.1")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	body := string(bodyBytes)
	log.Printf("scrape %s → HTTP %d, body[:200]: %s", post.RawURL, resp.StatusCode, body[:min(200, len(body))])

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	post.Content = extractOGMeta(body, "og:description")
	post.Media = extractOGMeta(body, "og:image")
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getPost 從 cache 或 scrape 取得 post，共用邏輯
func getPost(rawURL string) (*models.Post, error) {
	var post models.Post
	if err := post.ParseURL(rawURL); err != nil {
		return nil, err
	}

	// cache hit
	if err := db.Where("user = ? AND post_id = ?", post.User, post.PostID).First(&post).Error; err == nil {
		return &post, nil
	}

	// cache miss → scrape
	if err := scrapePost(&post); err != nil {
		return nil, err
	}
	if err := db.Create(&post).Error; err != nil {
		return nil, err
	}
	return &post, nil
}

func main() {
	initDB()

	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	// JSON API
	r.GET("/posts", func(c *gin.Context) {
		post, err := getPost(c.Query("url"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Type", "application/json")
		enc := json.NewEncoder(c.Writer)
		enc.SetEscapeHTML(false)
		enc.Encode(post)
	})

	// Discord embed
	r.GET("/embed", func(c *gin.Context) {
		post, err := getPost(c.Query("url"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <meta property="og:title" content="%s" />
  <meta property="og:description" content="%s" />
  <meta property="og:image" content="%s" />
  <meta property="og:url" content="%s" />
  <meta property="og:type" content="article" />
  <meta name="twitter:card" content="summary_large_image" />
</head>
<body><a href="%s">View on Threads</a></body>
</html>`,
			html.EscapeString(post.User),
			html.EscapeString(post.Content),
			html.EscapeString(post.Media),
			html.EscapeString(post.RawURL),
			html.EscapeString(post.RawURL),
		))
	})

	if err := r.Run(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
