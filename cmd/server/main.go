package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/k9pranav/LLM_Cache/internal/config"
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

	if err := r.Run(addr); err != nil {
		log.Fatal("server failed: ", err)
	}
}
