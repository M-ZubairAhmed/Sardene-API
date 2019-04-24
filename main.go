package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func welcome(gContext *gin.Context) {
	message := "Welcome to Sardene API, \nVisit https://github.com/M-ZubairAhmed/Sardene-API for documentation."
	gContext.String(http.StatusOK, message)
}

func ping(gContext *gin.Context) {
	gContext.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "pinged success",
	})
}

func main() {
	port := os.Getenv("PORT")

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/", welcome)

	router.GET("/ping", ping)

	router.Run(":" + port)
}
