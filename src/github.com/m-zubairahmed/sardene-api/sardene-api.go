package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func welcome(gContext *gin.Context) {
	message := "Welcome to Sardene API, \nplease visit https://github.com/M-ZubairAhmed/Sardene-API for complete documentation."
	gContext.String(http.StatusOK, message)
}

func ping(gContext *gin.Context) {
	gContext.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "pinged success",
	})
}

func main() {
	port := "8000"

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/", welcome)

	router.GET("/ping", ping)

	router.Run(":" + port)
}
