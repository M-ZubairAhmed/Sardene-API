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
	PublisherID int64              `json:"publisher_id" bson:"publisher_id"`
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
	UserID int64  `json:"id"`
	Login  string `json:"login"`
	Name   string `json:"name"`
}

// GithubAuthUser : Strucutre of github user and its access tokens
type GithubAuthUser struct {
	UserID      int64  `json:"userID"`
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

// IdeaLikesStructure : Strucutre for like in like collections
type IdeaLikesStructure struct {
	UserID int64              `json:"userID" bson:"userID"`
	IdeaID primitive.ObjectID `json:"ideaID" bson:"ideaID"`
}

func getEnvValues(envKeyStrings [5]string) map[string]string {
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

func extractAuthHeader(ginContext *gin.Context) (string, error) {
	const emptyString string = ""
	invalidHeaderFormatError := fmt.Errorf("Invalid authentication header format")

	authHeader := ginContext.GetHeader("Authorization")

	if len(authHeader) == 0 {
		return emptyString, invalidHeaderFormatError
	}
	if strings.Contains(authHeader, "Bearer") == false {
		return emptyString, invalidHeaderFormatError
	}

	trimmedAuthFromHeader := strings.TrimPrefix(authHeader, "Bearer")
	trimmedAuthFromHeader = strings.TrimSpace(trimmedAuthFromHeader)
	if strings.Contains(trimmedAuthFromHeader, " ") == true {
		return emptyString, invalidHeaderFormatError
	}

	return trimmedAuthFromHeader, nil
}

func getUserGithubProfile(accessToken string) (GithubUserProfileStructure, error) {
	var emptyGithubProfile GithubUserProfileStructure
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
		return emptyGithubProfile, errInResponseFromGithub
	}
	defer responseReaderWithUser.Body.Close()

	responseBytesWithUser, errInResponseBody := ioutil.ReadAll(responseReaderWithUser.Body)
	if errInResponseBody != nil {
		return emptyGithubProfile, errInResponseBody
	}

	errInDecodingJSON := json.Unmarshal(responseBytesWithUser, &githubProfile)
	if errInDecodingJSON != nil {
		return emptyGithubProfile, errInDecodingJSON
	}

	if githubProfile.Login == "" {
		return githubProfile, fmt.Errorf("Invalid user")
	}

	return githubProfile, nil
}

func validateAndGetUser(ginContext *gin.Context) (GithubUserProfileStructure, error) {
	var emptyGithubUser GithubUserProfileStructure

	userAccessToken, errInAccessTokenFormat := extractAuthHeader(ginContext)
	if errInAccessTokenFormat != nil {
		return emptyGithubUser, errInAccessTokenFormat
	}

	githubUser, errInGithubAccess := getUserGithubProfile(userAccessToken)
	if errInGithubAccess != nil {
		return emptyGithubUser, errInGithubAccess
	}

	return githubUser, nil
}

func addUserToDatabase(githubUser GithubUserProfileStructure, databaseClient *mongo.Client) error {
	usersCollections := databaseClient.Database("sardene-db").Collection("users")
	databaseContext, cancelDBContext := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelDBContext()

	userFilter := bson.M{"userID": githubUser.UserID}
	userFoundResult := usersCollections.FindOne(databaseContext, userFilter, options.FindOne())

	var foundUserInDB GithubUserProfileStructure

	doesUserExistsInDB := true

	errInDecoding := userFoundResult.Decode(&foundUserInDB)
	if errInDecoding != nil {
		if errInDecoding.Error() == "mongo: no documents in result" {
			doesUserExistsInDB = false
		} else {
			return errInDecoding
		}
	}

	if doesUserExistsInDB == true {
		return nil
	}
	// Else user not found in db, new user
	userToAdd := bson.M{
		"userID": githubUser.UserID,
		"login":  githubUser.Login,
		"name":   githubUser.Name,
	}
	_, errInAddingUser := usersCollections.InsertOne(databaseContext, userToAdd, options.InsertOne())
	if errInAddingUser != nil {
		return errInAddingUser
	}

	return nil
}

func welcome(ginContext *gin.Context) {
	message := "Welcome to Sardene API, \nServer running successfully" +
		"\nVisit https://github.com/M-ZubairAhmed/Sardene-API for documentation."
	ginContext.String(http.StatusOK, message)
}

func getIdeas(ginContext *gin.Context, databaseClient *mongo.Client) {
	var ideas []*IdeaStructure

	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")
	databaseContext, cancelDBContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelDBContext()

	findOptions := options.Find()
	ideasCursor, errorInFinding := ideasCollection.Find(databaseContext, bson.D{{}}, findOptions)

	if errorInFinding != nil {
		_ = ideasCursor.Close(databaseContext)
		databaseContext.Done()
		ginContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable,
			"error": "Error in searching database"})
		return
	}

	for ideasCursor.Next(databaseContext) {
		var idea IdeaStructure

		errInDecoding := ideasCursor.Decode(&idea)
		if errInDecoding != nil {
			_ = ideasCursor.Close(databaseContext)
			databaseContext.Done()
			ginContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
				"error": "Error in decoding database", "errorDetails": errInDecoding.Error()})
			return
		}

		ideas = append(ideas, &idea)
	}

	errInCursor := ideasCursor.Err()
	if errInCursor != nil {
		databaseContext.Done()
		_ = ideasCursor.Close(databaseContext)
		ginContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
			"error": "Error while iterating database"})
	}

	errInClosingCursor := ideasCursor.Close(databaseContext)
	if errInClosingCursor != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable,
			"error": "Error while closing iterator of database"})
		return
	}

	lengthOfIdeas := len(ideas)

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": ideas, "count": lengthOfIdeas})
	databaseContext.Done()
	return
}

func authenticateUser(ginContext *gin.Context, databaseClient *mongo.Client, githubSecrets GithubSecretsEnvs) {
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
	githubAuthUser.UserID = userGithubProfile.UserID
	githubAuthUser.Login = userGithubProfile.Login
	githubAuthUser.Name = userGithubProfile.Name
	githubAuthUser.AccessToken = jsonRespFromGithub.AccessToken
	githubAuthUser.TokenType = jsonRespFromGithub.TokenType
	githubAuthUser.Scope = jsonRespFromGithub.Scope

	errInAddingUserInDB := addUserToDatabase(userGithubProfile, databaseClient)
	if errInAddingUserInDB != nil {
		ginContext.JSON(http.StatusForbidden, gin.H{"status": http.StatusForbidden,
			"error": "Cannot add user in database", "errorDetails": errInAddingUserInDB.Error()})
		return
	}

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK,
		"data": githubAuthUser})

	return
}

func addIdea(ginContext *gin.Context, databaseClient *mongo.Client) {

	user, errInValidatingUser := validateAndGetUser(ginContext)
	if errInValidatingUser != nil {
		ginContext.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized,
			"error": "Autherization failed", "errorDetails": errInValidatingUser.Error()})
		return
	}

	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")

	databaseContext, cancelContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelContext()

	var jsonInput IdeaStructure
	createdTime := time.Now().Unix()

	errInInputJSON := ginContext.ShouldBindJSON(&jsonInput)
	if errInInputJSON != nil {
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Wrong structure of posted data"})
		databaseContext.Done()
		return
	}

	lengthOfName := len(strings.TrimSpace(jsonInput.Name))
	lengthOfDescription := len(strings.TrimSpace(jsonInput.Description))

	if lengthOfName == 0 || lengthOfDescription == 0 {
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
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
	// User data
	jsonInput.Publisher = user.Login
	jsonInput.PublisherID = user.UserID

	ideaToAdd := bson.M{
		"name":         jsonInput.Name,
		"description":  jsonInput.Description,
		"publisher":    jsonInput.Publisher,
		"publisher_id": jsonInput.PublisherID,
		"makers":       jsonInput.Makers,
		"gazers":       jsonInput.Gazers,
		"created_at":   createdTime,
	}

	addedIdea, errInAdding := ideasCollection.InsertOne(databaseContext, ideaToAdd)
	if errInAdding != nil {
		ginContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
			"error": "Error while saving to database"})
		return
	}

	// Get the generated ID from DB
	jsonInput.ID = addedIdea.InsertedID.(primitive.ObjectID)

	ginContext.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "data": jsonInput})
	databaseContext.Done()
	return
}

func likeAnIdea(ginContext *gin.Context, databaseClient *mongo.Client, ideaID string) {

	// Check if Idea id is valid
	hexIdeaID, errInValidatingID := primitive.ObjectIDFromHex(ideaID)
	if errInValidatingID != nil {
		ginContext.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest,
			"error": "Error, Idea id is not valid"})
		return
	}

	// Getting user details from the header
	user, errInValidatingUser := validateAndGetUser(ginContext)
	if errInValidatingUser != nil {
		ginContext.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized,
			"error": "Autherization failed", "errorDetails": errInValidatingUser.Error()})
		return
	}

	databaseContext, cancelContext := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelContext()

	// Checking if idea exists
	var ideaFound IdeaStructure
	ideasCollection := databaseClient.Database("sardene-db").Collection("ideas")
	findIdeaFilter := bson.M{"_id": hexIdeaID}

	ideaFoundInDB := ideasCollection.FindOne(databaseContext, findIdeaFilter, options.FindOne())

	errInDecodingIdea := ideaFoundInDB.Decode(&ideaFound)
	if errInDecodingIdea != nil {
		databaseContext.Done()
		if errInDecodingIdea.Error() == "mongo: no documents in result" {
			ginContext.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound,
				"error": "Error, Idea does not exists", "errorDetails": errInDecodingIdea.Error()})
			return
		}
		ginContext.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound,
			"error": "Error, Couldnt decode idea from idea id", "errorDetails": errInDecodingIdea.Error()})
		return
	}

	// Checking if user already liked
	likesCollection := databaseClient.Database("sardene-db").Collection("likes")

	userlikedFilter := bson.M{"userID": user.UserID, "ideaID": hexIdeaID}
	userFoundResult := likesCollection.FindOne(databaseContext, userlikedFilter, options.FindOne())

	didUserLikedIdeaBefore := true

	var userLikedIdea IdeaLikesStructure
	errInDecoding := userFoundResult.Decode(&userLikedIdea)
	if errInDecoding != nil {
		if errInDecoding.Error() == "mongo: no documents in result" {
			didUserLikedIdeaBefore = false
		}
	}

	if didUserLikedIdeaBefore == true {
		databaseContext.Done()
		ginContext.JSON(http.StatusConflict, gin.H{"status": http.StatusConflict,
			"error": "Error, User already liked the idea"})
		return
	}

	// Find idea and Increasing count in idea DB
	updateGazeOfIdea := bson.M{"$inc": bson.M{"gazers": 1}}

	_, errInFindingIdea := ideasCollection.UpdateOne(databaseContext, findIdeaFilter, updateGazeOfIdea)
	if errInFindingIdea != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "error": "Error, Idea not found"})
		return
	}

	// Adding user to likes DB
	ideaLikedByUserToAdd := bson.M{
		"userID": user.UserID,
		"ideaID": hexIdeaID,
	}

	_, errInAdding := likesCollection.InsertOne(databaseContext, ideaLikedByUserToAdd)
	if errInAdding != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError,
			"error": "Error while saving to database"})
		return
	}

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": "",
		"message": "Increased gaze count of idea"})
	databaseContext.Done()
	return
}

func getUserLikedIdeas(ginContext *gin.Context, databaseClient *mongo.Client) {
	// Getting user details from the header
	user, errInValidatingUser := validateAndGetUser(ginContext)
	if errInValidatingUser != nil {
		ginContext.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized,
			"error": "Autherization failed", "errorDetails": errInValidatingUser.Error()})
		return
	}

	ideasCollection := databaseClient.Database("sardene-db").Collection("likes")
	databaseContext, cancelContext := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelContext()

	findingAllUserLikedIdeas := bson.M{"userID": user.UserID}
	foundIdeasUserLikedCursor, errInFindingUsersLikedIdeas := ideasCollection.Find(databaseContext, findingAllUserLikedIdeas, options.Find())

	// Cursor errors
	if errInFindingUsersLikedIdeas != nil {
		_ = foundIdeasUserLikedCursor.Close(databaseContext)
		databaseContext.Done()
		ginContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable,
			"error": "Error in searching database", "errorDetails": errInFindingUsersLikedIdeas.Error()})
		return
	}
	errInFoundIdeasCursor := foundIdeasUserLikedCursor.Err()
	if errInFoundIdeasCursor != nil {
		_ = foundIdeasUserLikedCursor.Close(databaseContext)
		databaseContext.Done()
		ginContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable,
			"error": "Error in searching database", "errorDetails": errInFoundIdeasCursor.Error()})
		return
	}

	// Will contains all the user liked ideas
	var userLikedIdeas []*IdeaLikesStructure

	// Looping throught all user ideas
	for foundIdeasUserLikedCursor.Next(databaseContext) {
		var userLikedIdea IdeaLikesStructure

		errInDecodedUserLikedIdea := foundIdeasUserLikedCursor.Decode(&userLikedIdea)

		if errInDecodedUserLikedIdea != nil {
			_ = foundIdeasUserLikedCursor.Close(databaseContext)
			databaseContext.Done()
			ginContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable,
				"error": "Error in searching database", "errorDetails": errInDecodedUserLikedIdea.Error()})
			return
		}

		// Appending to user liked ideas array if no error found above
		userLikedIdeas = append(userLikedIdeas, &userLikedIdea)
	}

	// Close the cursor after looping
	errInClosingCursor := foundIdeasUserLikedCursor.Close(databaseContext)
	if errInClosingCursor != nil {
		databaseContext.Done()
		ginContext.JSON(http.StatusServiceUnavailable, gin.H{"status": http.StatusServiceUnavailable,
			"error": "Error while closing iterator of database"})
		return
	}

	totalNumberOfIdeas := len(userLikedIdeas)

	ginContext.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": userLikedIdeas, "count": totalNumberOfIdeas})
	databaseContext.Done()
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
	envKeys := [5]string{"ENVIRONMENT", "DB_URL", "PORT", "GITHUB_CLIENT", "GITHUB_SECRET"}
	env := getEnvValues(envKeys)

	port := env["PORT"]

	router := gin.Default()

	allowedOrigin := "https://sardene.cf"
	if env["ENVIRONMENT"] == "dev" {
		allowedOrigin = "http://localhost:3000"
	}

	corsConfig := cors.Config{
		AllowOrigins:     []string{allowedOrigin},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowHeaders:     []string{"Origin", "Authorization", "Cache-Control", "Accept", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	router.Use(cors.New(corsConfig))

	databaseClient := connectToDatabase(env["DB_URL"])

	router.GET("/", welcome)

	// TODO convert to pagination endpoint
	router.GET("/ideas", func(ginContext *gin.Context) {
		getIdeas(ginContext, databaseClient)
	})

	router.POST("/auth", func(ginContext *gin.Context) {
		var githubSecrets GithubSecretsEnvs
		githubSecrets.Client = env["GITHUB_CLIENT"]
		githubSecrets.Secret = env["GITHUB_SECRET"]

		authenticateUser(ginContext, databaseClient, githubSecrets)
	})

	router.POST("/idea/add", func(ginContext *gin.Context) {
		addIdea(ginContext, databaseClient)
	})

	router.PATCH("/idea/gaze/:ideaID", func(ginContext *gin.Context) {
		ideaID := ginContext.Param("ideaID")
		likeAnIdea(ginContext, databaseClient, ideaID)
	})

	router.GET("/ideas/gazed", func(ginContext *gin.Context) {
		getUserLikedIdeas(ginContext, databaseClient)
	})

	// router.GET("/user" , func(ginContext *gin.Context)){
	// 	getUserProfile()
	// }

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
