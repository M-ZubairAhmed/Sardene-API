package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// IdeaStructure : Structure of Idea in database
type IdeaStructure struct {
	ID          primitive.ObjectID `json:"id" bson:"_id"`
	Name        string             `json:"name" bson:"name"`
	Description string             `json:"description" bson:"description"`
	Makers      string             `json:"makers" bson:"makers"`
	Gazers      string             `json:"gazers" bson:"gazers"`
	// Dated       primitive.DateTime `json:"dated" bson:"dated"`
}

func connectToDatabase() *mongo.Client {
	mlabsDbURL := os.Getenv("MONGO_DB_URL")
	if len(mlabsDbURL) == 0 {
		log.Fatal("No Database URL provided")
	}
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

	return databaseClient
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

func getIdeas(gContext *gin.Context) {
	var ideas []*IdeaStructure

	databaseClient := connectToDatabase()
	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	connectContext, errorInContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer errorInContext()

	findOptions := options.Find()
	ideasCursor, errorInFinding := ideasCollection.Find(connectContext, bson.D{{}}, findOptions)

	if errorInFinding != nil {
		log.Fatal(errorInFinding, " // Error in running find on DB")
		panic("Error in running find on DB")
	}

	defer ideasCursor.Close(connectContext)

	for ideasCursor.Next(connectContext) {
		var idea IdeaStructure

		errInDecoding := ideasCursor.Decode(&idea)
		if errInDecoding != nil {
			log.Fatal(errInDecoding)
		}

		ideas = append(ideas, &idea)
	}

	errInCursor := ideasCursor.Err()
	if errInCursor != nil {
		log.Fatal(errInCursor)
	}

	ideasCursor.Close(connectContext)

	lenghtOfIdeas := len(ideas)
	gContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": ideas, "count": lenghtOfIdeas})
}

func addIdea(gContext *gin.Context) {
	databaseClient := connectToDatabase()
	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	connectContext, errorInContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer errorInContext()

	docu := bson.M{
		"name":        "My first title",
		"description": "description goes here and here",
		"makers":      "0",
		"gazers":      "0",
	}

	resultOfAdding, errInAdding := ideasCollection.InsertOne(connectContext, docu)
	if errInAdding != nil {
		log.Fatal(errInAdding)
	}
	fmt.Printf(
		"new post created with id: %s",
		resultOfAdding.InsertedID.(primitive.ObjectID).Hex(),
	)

}

func updateIdea(gContext *gin.Context) {}

func main() {
	port := os.Getenv("PORT")
	if os.Getenv("PORT") == "" {
		port = "8000"
	}

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/", welcome)

	router.GET("/ping", ping)

	// TODO convert to pagination endpoint
	// router.GET("/ideas/:page", getIdeas)
	router.GET("/ideas", getIdeas)

	router.GET("/idea/add", addIdea)

	// router.PUT("/updateidea/:ideaID", updateIdea)

	router.Run(":" + port)
}
