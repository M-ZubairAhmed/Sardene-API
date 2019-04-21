package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/", func(ginContext *gin.Context) {
		ginContext.JSON(http.StatusOK, gin.H{
			"status":  http.StatusOK,
			"message": "pinged success",
		})
	})

	router.Run(":" + port)
}
