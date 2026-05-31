package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/k9pranav/LLM_Cache/internal/cache"
)

func main() {
	fmt.Println("LLM Cache Gateway starting...")

	//Calling LoadConfig function to get
	// cfg, err := config.LoadConfig()

	// if err != nil {
	// 	log.Fatal("failed to load config: ", err)
	// }
	r := gin.Default()

	// r.GET(cfg.Health.Path, func(c *gin.Context) {
	// 	c.JSON(http.StatusOK, gin.H{
	// 		"status": "ok",
	// 		"app":    cfg.App.Name,
	// 		"env":    cfg.App.Env,
	// 	})
	// })

	// addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	// log.Printf("server listening on %s\n", addr)

	// new_embed_request := embedder.CreateEmbedder(cfg.Embedder.BaseURL, cfg.Embedder.Model)
	// vector, err := new_embed_request.Embed("What is the capital of France?")

	// r.GET("/embed", func(c *gin.Context) {
	// 	c.JSON(http.StatusOK, gin.H{
	// 		"status":    "ok",
	// 		"my_vector": vector,
	// 	})
	// })

	// if err := r.Run(addr); err != nil {
	// 	log.Fatal("server failed: ", err)
	// }

	ctx := context.Background()

	redisCache := cache.NewRedisCache("redis-cache:6379", "", 0)

	if err := redisCache.Ping(ctx); err != nil {
		log.Fatal("failed to connec to Redis: ", err)
	}

	fmt.Println("Connected to Redis")

	r.GET("/redis-test", func(c *gin.Context) {

		ctx := c.Request.Context()

		err := redisCache.Set(ctx, "hello", "world", 5*time.Minute)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})

			return
		}

		value, err := redisCache.Get(ctx, "hello")

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})

			return
		}

		c.JSON(http.StatusOK, gin.H{
			"value": value,
		})
	})

	if err := r.Run("0.0.0.0:8080"); err != nil {
		log.Fatal("server failed: ", err)
	}

}
