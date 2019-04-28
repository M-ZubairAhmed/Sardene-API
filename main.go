package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mongodb/mongo-go-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var databaseClient *mongo.Client

type IdeaStructure struct {
	name        string
	description string
	makers      int
	gazers      int
}

func connectToDatabase() {
	mlabsDbURL := os.Getenv("MONGO_DB_URL")
	databaseURL := fmt.Sprint(mlabsDbURL)

	connectOptions := options.Client()
	connectOptions.ApplyURI(databaseURL)

	connectContext, errorInContext := context.WithTimeout(context.Background(), 10*time.Second)

	defer errorInContext()

	databaseClient, errInConnection := mongo.Connect(connectContext, connectOptions)

	if errInConnection != nil {
		log.Fatal(errInConnection)
		panic("Failed to connect to DB")
	}

	errInPing := databaseClient.Ping(connectContext, nil)

	if errInPing != nil {
		log.Fatal(errInPing)
		panic("DB not found")
	}

	fmt.Println("Connected to Database")
}

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

func getIdeas(gContext *gin.Context) {}

func addIdea(gContext *gin.Context) {}

func updateIdea(gContext *gin.Context) {}

func main() {
	port := os.Getenv("PORT")
	if os.Getenv("PORT") == "" {
		port = "8000"
	}

	connectToDatabase()

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/", welcome)

	router.GET("/ping", ping)

	// router.GET("/ideas/:page", getIdeas)

	// router.POST("/idea", addIdea)

	// router.PUT("/idea/:ideaID", updateIdea)

	router.Run(":" + port)
}
