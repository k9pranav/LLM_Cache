package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/k9pranav/LLM_Cache/internal/config"
	"github.com/k9pranav/LLM_Cache/internal/embedder"
)

func main() {
	fmt.Println("LLM Cache Gateway starting...")

	//Calling LoadConfig function to get
	cfg, err := config.LoadConfig()

	if err != nil {
		log.Fatal("failed to load config: ", err)
	}
	r := gin.Default()

	r.GET(cfg.Health.Path, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"app":    cfg.App.Name,
			"env":    cfg.App.Env,
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	log.Printf("server listening on %s\n", addr)

	new_embed_request := embedder.NewMxbaiEmbedder()
	vector, err := new_embed_request.Embed("What is the capital of France?")

	r.GET("/embed", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"my_vector": vector,
		})
	})

	if err := r.Run(addr); err != nil {
		log.Fatal("server failed: ", err)
	}

}
