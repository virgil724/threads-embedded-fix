package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"threads-scraper/models"
	"time"

	"github.com/chromedp/chromedp"
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

func scrapePost(post *models.Post) error {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var content, media string
	err := chromedp.Run(ctx,
		chromedp.Navigate(post.RawURL),
		chromedp.WaitVisible(`[data-pressable-container]`, chromedp.ByQuery),
		chromedp.Evaluate(`
			(function() {
				const spans = [...document.querySelectorAll('[data-pressable-container] span[dir="auto"]')]
					.filter(s => !s.closest('a') && !s.closest('time'));
				return spans.length > 1 ? spans[1].innerText : (spans[0] ? spans[0].innerText : '');
			})()
		`, &content),
		chromedp.AttributeValue(`[data-pressable-container] picture img`, "src", &media, nil),
	)
	if err != nil {
		log.Printf("failed to scrape post: %v", err)
		return err
	}

	post.Content = content
	post.Media = media
	return nil
}

// getPost 從 cache 或 scrape 取得 post，共用邏輯
func getPost(rawURL string) (*models.Post, error) {
	var post models.Post
	if err := post.ParseURL(rawURL); err != nil {
		return nil, err
	}

	// cache hit
	if err := db.Where("post_id = ?", post.PostID).First(&post).Error; err == nil {
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
