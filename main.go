package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	Publisher   string             `json:"publisher" bson:"publisher"`
	Makers      int64              `json:"makers" bson:"makers"`
	Gazers      int64              `json:"gazers" bson:"gazers"`
	CreatedAt   int64              `json:"created_at" bson:"created_at"`
}

// GithubAccessTokenResponse : Structure of response from github after code is posted to them
type GithubAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// GithubUserProfileStructure : Strucutre of github profile json
type GithubUserProfileStructure struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

// GithubAuthUser : Strucutre of github user and its access tokens
type GithubAuthUser struct {
	ID          int64  `json:"id"`
	Login       string `json:"login"`
	Name        string `json:"name"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// GithubAuthCode : Structure for incoming code of github
type GithubAuthCode struct {
	Code string `json:"code"`
}

// GithubSecretsEnvs : Strucuture for passing secrets to func
type GithubSecretsEnvs struct {
	Client string
	Secret string
}

func getEnvValues(envKeyStrings [9]string) map[string]string {
	envValues := make(map[string]string)

	for _, keyString := range envKeyStrings {
		if os.Getenv(keyString) == "" {
			log.Fatal("No env value provided for " + keyString)
		}
		envValues[keyString] = os.Getenv(keyString)
	}
	return envValues
}

func connectToDatabase(databaseURL string) *mongo.Client {
	connectOptions := options.Client()
	connectOptions.ApplyURI(databaseURL)

	connectContext, errorInContext := context.WithTimeout(context.Background(), 10*time.Second)

	defer errorInContext()

	databaseClient, errInConnection := mongo.Connect(connectContext, connectOptions)

	if errInConnection != nil {
		log.Fatal(errInConnection, "Failed to connect to DB")
	}

	errInPing := databaseClient.Ping(connectContext, nil)

	if errInPing != nil {
		log.Fatal(errInPing, "DB not found")
	}

	return databaseClient
}

func getUserGithubProfile(accessToken string) (GithubUserProfileStructure, error) {
	var githubProfile GithubUserProfileStructure
	getGithubUserURL := "https://api.github.com/user"

	requestUser, errInRequestingUser := http.NewRequest("GET", getGithubUserURL, nil)

	if errInRequestingUser != nil {
		return githubProfile, errInRequestingUser
	}

	authHeader := "token " + accessToken
	requestUser.Header.Set("Accept", "application/vnd.github.v3+json")
	requestUser.Header.Set("Authorization", authHeader)
	httpClientForGithubProfile := http.Client{}
	httpClientForGithubProfile.Timeout = time.Minute * 10

	responseReaderWithUser, errInResponseFromGithub := httpClientForGithubProfile.Do(requestUser)
	if errInResponseFromGithub != nil {
		return githubProfile, errInResponseFromGithub
	}
	defer responseReaderWithUser.Body.Close()

	responseBytesWithUser, errInResponseBody := ioutil.ReadAll(responseReaderWithUser.Body)
	if errInResponseBody != nil {
		return githubProfile, errInResponseBody
	}

	errInDecodingJSON := json.Unmarshal(responseBytesWithUser, &githubProfile)
	if errInDecodingJSON != nil {
		return githubProfile, errInDecodingJSON
	}
	fmt.Print(githubProfile.Name)

	return githubProfile, nil
}

func welcome(gContext *gin.Context) {
	message := "Welcome to Sardene API, \nServer running successfully" +
		"\nVisit https://github.com/M-ZubairAhmed/Sardene-API for documentation."
	gContext.String(http.StatusOK, message)
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
		gContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable,
			"error": "Error in searching database"})
		return
	}

	for ideasCursor.Next(databaseContext) {
		var idea IdeaStructure

		errInDecoding := ideasCursor.Decode(&idea)
		if errInDecoding != nil {
			_ = ideasCursor.Close(databaseContext)
			databaseContext.Done()
			gContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
				"error": "Error in decoding database"})
			log.Fatal(errInDecoding)
		}

		ideas = append(ideas, &idea)
	}

	errInCursor := ideasCursor.Err()
	if errInCursor != nil {
		databaseContext.Done()
		_ = ideasCursor.Close(databaseContext)
		gContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
			"error": "Error while iterating database"})
	}

	errInClosingCursor := ideasCursor.Close(databaseContext)
	if errInClosingCursor != nil {
		databaseContext.Done()
		gContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable,
			"error": "Error while closing iterator of database"})
		return
	}

	lengthOfIdeas := len(ideas)

	gContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": ideas, "count": lengthOfIdeas})
	databaseContext.Done()
	return
}

func authUser(ginContext *gin.Context, databaseClient *mongo.Client, githubSecrets GithubSecretsEnvs) {
	var githubCodeInput GithubAuthCode

	errInInput := ginContext.ShouldBindJSON(&githubCodeInput)
	if errInInput != nil {
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Wrong structure of posted data"})
		return
	}

	githubAuthCode := githubCodeInput.Code
	githubAccessTokenURL := fmt.Sprint("https://github.com/login/oauth/access_token", "?client_id=", githubSecrets.Client, "&client_secret=", githubSecrets.Secret, "&code=", githubAuthCode)

	var jsonEmptyInput = []byte(`{}`)
	postReqToGithub, errInPostToGithub := http.NewRequest("POST", githubAccessTokenURL, bytes.NewBuffer(jsonEmptyInput))
	if errInPostToGithub != nil {
		ginContext.JSON(http.StatusForbidden, gin.H{"status": http.StatusForbidden,
			"error": "Cannot be authenciated", "errorDetails": errInInput.Error()})
		return
	}

	postReqToGithub.Header.Set("Accept", "application/json")
	httpClientForGithub := http.Client{}
	httpClientForGithub.Timeout = time.Minute * 10

	postResFromGithub, errInRespFromGithub := httpClientForGithub.Do(postReqToGithub)
	if errInRespFromGithub != nil {
		ginContext.JSON(http.StatusForbidden, gin.H{"status": http.StatusForbidden,
			"error": "Cannot be authenciated", "errorDetails": errInInput.Error()})
		return
	}
	defer postResFromGithub.Body.Close()

	githubRespInBytes, errInReader := ioutil.ReadAll(postResFromGithub.Body)
	if errInReader != nil {
		ginContext.JSON(http.StatusForbidden, gin.H{"status": http.StatusForbidden,
			"error": "Cannot be authenciated", "errorDetails": errInInput.Error()})
		return
	}

	var jsonRespFromGithub GithubAccessTokenResponse
	errInReadingToken := json.Unmarshal(githubRespInBytes, &jsonRespFromGithub)
	if errInReadingToken != nil {
		ginContext.JSON(http.StatusForbidden, gin.H{"status": http.StatusForbidden,
			"error": "Cannot be authenciated", "errorDetails": errInInput.Error()})
		return
	}

	userGithubProfile, errInGettingProfile := getUserGithubProfile(jsonRespFromGithub.AccessToken)
	if errInGettingProfile != nil {
		ginContext.JSON(http.StatusForbidden, gin.H{"status": http.StatusForbidden,
			"error": "Cannot get user", "errorDetails": errInGettingProfile.Error()})
		return
	}

	var githubAuthUser GithubAuthUser
	githubAuthUser.ID = userGithubProfile.ID
	githubAuthUser.Login = userGithubProfile.Login
	githubAuthUser.Name = userGithubProfile.Name
	githubAuthUser.AccessToken = jsonRespFromGithub.AccessToken
	githubAuthUser.TokenType = jsonRespFromGithub.TokenType
	githubAuthUser.Scope = jsonRespFromGithub.Scope

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK,
		"data": githubAuthUser})
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
	jsonInput.Publisher = "Unnamed contact"
	jsonInput.CreatedAt = createdTime

	ideaToAdd := bson.M{
		"name":        jsonInput.Name,
		"description": jsonInput.Description,
		"publisher":   jsonInput.Publisher,
		"makers":      jsonInput.Makers,
		"gazers":      jsonInput.Gazers,
		"created_at":  createdTime,
	}

	addedIdea, errInAdding := ideasCollection.InsertOne(databaseContext, ideaToAdd)
	if errInAdding != nil {
		gContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
			"error": "Error while saving to database"})
		return
	}

	// Get the generated ID from DB
	jsonInput.ID = addedIdea.InsertedID.(primitive.ObjectID)

	gContext.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "data": jsonInput})
	databaseContext.Done()
	return
}

func gazeIdea(ginContext *gin.Context, databaseClient *mongo.Client, ideaID string) {
	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	databaseContext, cancelContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelContext()

	hexIdeaID, errInValidatingID := primitive.ObjectIDFromHex(ideaID)
	if errInValidatingID != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Error, Idea id is not valid"})
		return
	}

	findIdeaFilter := bson.M{"_id": hexIdeaID}
	updateGazeOfIdea := bson.M{"$inc": bson.M{"gazers": 1}}

	updatedIdea, errInFindingIdea := ideasCollection.UpdateOne(databaseContext, findIdeaFilter, updateGazeOfIdea)
	if errInFindingIdea != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "error": "Error, Idea not found"})
		return
	}

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": "",
		"message": "Increased gaze count of " + string(updatedIdea.ModifiedCount) + "idea"})
	databaseContext.Done()
	return
}

func makeIdea(ginContext *gin.Context, databaseClient *mongo.Client, ideaID string) {
	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	databaseContext, cancelContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelContext()

	hexIdeaID, errInValidatingID := primitive.ObjectIDFromHex(ideaID)
	if errInValidatingID != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Error, Idea id is not valid"})
		return
	}

	findIdeaFilter := bson.M{"_id": hexIdeaID}
	updateGazeOfIdea := bson.M{"$inc": bson.M{"makers": 1}}

	updatedIdea, errInFindingIdea := ideasCollection.UpdateOne(databaseContext, findIdeaFilter, updateGazeOfIdea)
	if errInFindingIdea != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "error": "Error, Idea not found"})
		return
	}

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": "",
		"message": "Increased make count of " + string(updatedIdea.ModifiedCount) + "idea"})
	databaseContext.Done()
	return

}

func updateIdea(ginContext *gin.Context, databaseClient *mongo.Client, ideaID string) {
	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	databaseContext, cancelContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelContext()

	hexIdeaID, errInValidatingID := primitive.ObjectIDFromHex(ideaID)
	if errInValidatingID != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Error, Idea id is not valid"})
		return
	}

	var jsonInput IdeaStructure

	errInInputJSON := ginContext.ShouldBindJSON(&jsonInput)
	if errInInputJSON != nil {
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Wrong structure of posted data", "errorDetails": errInInputJSON})
		databaseContext.Done()
		return
	}

	lengthOfName := len(strings.TrimSpace(jsonInput.Name))
	lengthOfDescription := len(strings.TrimSpace(jsonInput.Description))

	if lengthOfName == 0 && lengthOfDescription == 0 {
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Both name and description are empty"})
		databaseContext.Done()
		return
	}

	filterOfUpdatingIdea := bson.M{"_id": hexIdeaID}
	var updateIdea bson.M

	if lengthOfName == 0 && lengthOfDescription != 0 {
		// Updating only description
		updateIdea = bson.M{"$set": bson.M{
			"description": jsonInput.Description,
		}}
	} else if lengthOfName != 0 && lengthOfDescription == 0 {
		// Updating only name
		updateIdea = bson.M{"$set": bson.M{
			"name": jsonInput.Name,
		}}
	} else {
		// updating both
		updateIdea = bson.M{"$set": bson.M{
			"name":        jsonInput.Name,
			"description": jsonInput.Description,
		}}
	}

	_, errInFindingIdea := ideasCollection.UpdateOne(databaseContext, filterOfUpdatingIdea, updateIdea)
	if errInFindingIdea != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "error": "Error, Idea not found"})
		return
	}

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "Updated idea successfully"})
	databaseContext.Done()
	return
}

func deleteIdea(ginContext *gin.Context, databaseClient *mongo.Client, ideaID string) {
	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	databaseContext, cancelContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelContext()

	hexIdeaID, errInValidatingID := primitive.ObjectIDFromHex(ideaID)
	if errInValidatingID != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Error, Idea id is not valid"})
		return
	}

	findIdeaFilter := bson.M{"_id": hexIdeaID}

	_, errInDeletingIdea := ideasCollection.DeleteOne(databaseContext, findIdeaFilter)
	if errInDeletingIdea != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "error": "Error, Idea not found"})
		return
	}

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "Idea deleted successfully"})
	databaseContext.Done()
	return

}

func main() {
	envKeys := [9]string{"ENVIRONMENT", "DB_HOST", "DB_USER", "DB_PASSWORD", "DB_URL", "DB_NAME", "PORT", "GITHUB_CLIENT", "GITHUB_SECRET"}
	env := getEnvValues(envKeys)

	port := env["PORT"]

	allowedOrigin := "https://sardene.cf"
	if env["ENVIRONMENT"] == "dev" {
		allowedOrigin = "http://localhost:3000"
	}

	router := gin.Default()

	defaultCors := cors.DefaultConfig()

	defaultCors.AllowOrigins = []string{allowedOrigin}
	router.Use(cors.New(defaultCors))

	databaseURL := fmt.Sprint(env["DB_HOST"], "://", env["DB_USER"], ":", env["DB_PASSWORD"], "@", env["DB_URL"], "/", env["DB_NAME"])
	databaseClient := connectToDatabase(databaseURL)

	router.GET("/", welcome)

	// TODO convert to pagination endpoint
	router.GET("/ideas", func(gContext *gin.Context) {
		getIdeas(gContext, databaseClient)
	})

	router.POST("/auth", func(gContext *gin.Context) {
		var githubSecrets GithubSecretsEnvs
		githubSecrets.Client = env["GITHUB_CLIENT"]
		githubSecrets.Secret = env["GITHUB_SECRET"]

		authUser(gContext, databaseClient, githubSecrets)
	})

	router.POST("/idea/add", func(gContext *gin.Context) {
		addIdea(gContext, databaseClient)
	})

	router.PATCH("/idea/gaze/:ideaID", func(ginContext *gin.Context) {
		ideaID := ginContext.Param("ideaID")
		gazeIdea(ginContext, databaseClient, ideaID)
	})

	router.PATCH("/idea/make/:ideaID", func(ginContext *gin.Context) {
		ideaID := ginContext.Param("ideaID")
		makeIdea(ginContext, databaseClient, ideaID)
	})

	router.PUT("/idea/update/:ideaID", func(ginContext *gin.Context) {
		ideaID := ginContext.Param("ideaID")
		updateIdea(ginContext, databaseClient, ideaID)
	})

	router.DELETE("/idea/delete/:ideaID", func(ginContext *gin.Context) {
		ideaID := ginContext.Param("ideaID")
		deleteIdea(ginContext, databaseClient, ideaID)
	})

	errInStartingServer := router.Run(":" + port)
	if errInStartingServer != nil {
		log.Fatal(errInStartingServer, "// Cannot start server")
	}
}
