package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
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
	Makers      int64              `json:"makers" bson:"makers"`
	Gazers      int64              `json:"gazers" bson:"gazers"`
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
		log.Fatal(errInConnection , "Failed to connect to DB")
	}

	errInPing := databaseClient.Ping(connectContext, nil)

	if errInPing != nil {
		log.Fatal(errInPing, "DB not found")
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

func getIdeas(gContext *gin.Context, databaseClient *mongo.Client) {
	var ideas []*IdeaStructure

	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	databaseContext, cancelDBContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelDBContext()

	findOptions := options.Find()
	ideasCursor, errorInFinding := ideasCollection.Find(databaseContext, bson.D{{}}, findOptions)

	if errorInFinding != nil {
		_ = ideasCursor.Close(databaseContext)
		databaseContext.Done()
		gContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable, "error": "Error in searching database"})
		return
	}

	for ideasCursor.Next(databaseContext) {
		var idea IdeaStructure

		errInDecoding := ideasCursor.Decode(&idea)
		if errInDecoding != nil {
			_ = ideasCursor.Close(databaseContext)
			databaseContext.Done()
			gContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "error": "Error in decoding database"})
			log.Fatal(errInDecoding)
		}

		ideas = append(ideas, &idea)
	}

	errInCursor := ideasCursor.Err()
	if errInCursor != nil {
		databaseContext.Done()
		_ = ideasCursor.Close(databaseContext)
		gContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "error": "Error while iterating database"})
	}

	errInClosingCursor := ideasCursor.Close(databaseContext)
	if errInClosingCursor != nil{
		databaseContext.Done()
		gContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable, "error": "Error while closing iterator of database"})
		return
	}

	lengthOfIdeas := len(ideas)
	gContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": ideas, "count": lengthOfIdeas})
	databaseContext.Done()

	return
}

func addIdea(gContext *gin.Context, databaseClient *mongo.Client) {
	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	databaseContext, cancelContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelContext()

	var jsonInput IdeaStructure
	createdTime := time.Now().Unix()

	errInInputJSON := gContext.ShouldBindJSON(&jsonInput)
	if errInInputJSON != nil {
		gContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Wrong structure of posted data"})
		databaseContext.Done()
		return
	}

	lengthOfName := len(strings.TrimSpace(jsonInput.Name))
	lengthOfDescription := len(strings.TrimSpace(jsonInput.Description))

	if lengthOfName == 0 || lengthOfDescription == 0 {
		gContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Name or description is not provided in the post"})
		databaseContext.Done()
		return

	}

	// Cleaning data
	jsonInput.Name = strings.TrimSpace(jsonInput.Name)
	jsonInput.Description = strings.TrimSpace(jsonInput.Description)
	// Defaulting data
	jsonInput.Makers = 0
	jsonInput.Gazers = 0
	jsonInput.CreatedAt = createdTime

	ideaToAdd := bson.M{
		"name":        jsonInput.Name,
		"description": jsonInput.Description,
		"makers":      jsonInput.Makers,
		"gazers":      jsonInput.Gazers,
		"created_at":  createdTime,
	}

	addedIdea, errInAdding := ideasCollection.InsertOne(databaseContext, ideaToAdd)
	if errInAdding != nil {
		gContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
			"error":     "Error while saving to database"})
		return
	}

	// Get the generated ID from DB
	jsonInput.ID = addedIdea.InsertedID.(primitive.ObjectID)

	gContext.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "data": jsonInput})
	databaseContext.Done()
	return
}

func main() {
	port := os.Getenv("PORT")
	if os.Getenv("PORT") == "" {
		port = "8000"
	}

	router := gin.Default()

	defaultCors := cors.DefaultConfig()

	defaultCors.AllowOrigins = []string{ "http://localhost:3000"}
	router.Use(cors.New(defaultCors))

	databaseClient := connectToDatabase()

	router.GET("/", welcome)

	router.GET("/ping", ping)

	// TODO convert to pagination endpoint
	// router.GET("/ideas/:page", getIdeas)
	router.GET("/ideas", func (gContext *gin.Context){
		getIdeas(gContext, databaseClient)
	})

	router.POST("/idea/add", func (gContext *gin.Context){
		addIdea(gContext, databaseClient)
	})

	// router.PUT("/updateidea/:ideaID", updateIdea)

	router.Run(":" + port)
}
