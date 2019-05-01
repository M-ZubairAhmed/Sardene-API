package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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
	CreatedAt   int64              `json:"created_at" bson:"created_at"`
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

	var jsonInput IdeaStructure
	createdTime := time.Now().Unix()

	errInInputJSON := gContext.ShouldBindJSON(&jsonInput)
	if errInInputJSON != nil {
		gContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "errorCode": "bad_json_structure",
			"error": "Wrong structure of posted data"})
		return
	}

	lengthOfName := len(strings.TrimSpace(jsonInput.Name))
	lenghtOfDescription := len(strings.TrimSpace(jsonInput.Description))

	if lengthOfName == 0 || lenghtOfDescription == 0 {
		gContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "errorCode": "empty_fields",
			"error": "Name or description is not provided in the post"})
		return

	}

	// Cleaning data
	jsonInput.Name = strings.TrimSpace(jsonInput.Name)
	jsonInput.Description = strings.TrimSpace(jsonInput.Description)

	ideaToAdd := bson.M{
		"name":        jsonInput.Name,
		"description": jsonInput.Description,
		"makers":      "0",
		"gazers":      "0",
		"created_at":  createdTime,
	}

	addedIdea, errInAdding := ideasCollection.InsertOne(connectContext, ideaToAdd)
	if errInAdding != nil {
		gContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
			"errorCode": "cannot_add_to_db",
			"error":     "Error while saving to database"})
		log.Fatal(errInAdding)
		return
	}

	// Get the generated ID from DB
	jsonInput.ID = addedIdea.InsertedID.(primitive.ObjectID)

	gContext.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "data": jsonInput})
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

	router.POST("/idea/add", addIdea)

	// router.PUT("/updateidea/:ideaID", updateIdea)

	router.Run(":" + port)
}
