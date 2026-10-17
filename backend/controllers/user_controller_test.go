package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

const (
	strongPassword        = "CorrectHorseBattery42!"
	strongPasswordAlt     = "TrulySecurePassphrase99#"
	strongPasswordAnother = "UltraSafePassphrase88$"
)

func TestRegisterUser(t *testing.T) {
	_, router := setupRouter()
	cfg := &config.Config{}
	router.POST("/register", middleware.ValidateJSONMiddleware(&models.UserRegistrationInput{}), RegisterUser(cfg))

	// Create a new user using the registration DTO
	newUser := models.UserRegistrationInput{
		Username: "testuser",
		Email:    "testuser@example.com",
		Password: strongPassword,
	}

	jsonValue, _ := json.Marshal(newUser)
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var responseBody map[string]string
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, "User registered successfully", responseBody["message"])
}

func TestRegisterUser_InvalidInput(t *testing.T) {
	_, router := setupRouter()
	cfg := &config.Config{}
	router.POST("/register", middleware.ValidateJSONMiddleware(&models.UserRegistrationInput{}), RegisterUser(cfg))

	// Invalid input (no email)
	invalidUser := models.UserRegistrationInput{
		Username: "invaliduser",
		Password: strongPassword,
	}

	jsonValue, _ := json.Marshal(invalidUser)
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	errorDetail := response["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errorDetail["code"])
}

func TestLoginUser(t *testing.T) {
	config := config.Config{
		JWTSecretKey:   "mysecretkey",
		JWTExpiryHours: 24,
	}

	db, router := setupRouter()
	router.POST("/login", func(c *gin.Context) {
		LoginUser(c, &config)
	})

	// First, register a user to test login
	newUser := models.User{
		Username: "testuser",
		Email:    "testuser@example.com",
		Password: strongPassword,
	}
	hashedPassword, _ := services.HashPassword(newUser.Password)
	newUser.Password = hashedPassword
	db.Create(&newUser)

	// Now try to login with email
	loginData := map[string]string{
		"identifier": "testuser@example.com",
		"password":   strongPassword,
	}

	jsonValue, _ := json.Marshal(loginData)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var responseBody map[string]string
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Contains(t, responseBody, "language")
	assert.Contains(t, responseBody, "date_format")
	assert.NotEmpty(t, w.Result().Cookies())
	var authCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	assert.NotNil(t, authCookie, "auth_token cookie should be set")
	assert.NotEmpty(t, authCookie.Value)
}

func TestLoginUser_WithUsername(t *testing.T) {
	config := config.Config{
		JWTSecretKey:   "mysecretkey",
		JWTExpiryHours: 24,
	}

	db, router := setupRouter()
	router.POST("/login", func(c *gin.Context) {
		LoginUser(c, &config)
	})

	// First, register a user to test login
	newUser := models.User{
		Username: "testuser",
		Email:    "testuser@example.com",
		Password: strongPassword,
	}
	hashedPassword, _ := services.HashPassword(newUser.Password)
	newUser.Password = hashedPassword
	db.Create(&newUser)

	// Now try to login with username
	loginData := map[string]string{
		"identifier": "testuser",
		"password":   strongPassword,
	}

	jsonValue, _ := json.Marshal(loginData)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var responseBody map[string]string
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Contains(t, responseBody, "language")
	assert.Contains(t, responseBody, "date_format")
	var authCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	assert.NotNil(t, authCookie, "auth_token cookie should be set")
	assert.NotEmpty(t, authCookie.Value)
}

func TestLoginUser_LegacyEmailField(t *testing.T) {
	config := config.Config{
		JWTSecretKey:   "mysecretkey",
		JWTExpiryHours: 24,
	}

	db, router := setupRouter()
	router.POST("/login", func(c *gin.Context) {
		LoginUser(c, &config)
	})

	// First, register a user to test login
	newUser := models.User{
		Username: "testuser",
		Email:    "testuser@example.com",
		Password: strongPassword,
	}
	hashedPassword, _ := services.HashPassword(newUser.Password)
	newUser.Password = hashedPassword
	db.Create(&newUser)

	// Now try to login with legacy email field (backward compatibility)
	loginData := map[string]string{
		"email":    "testuser@example.com",
		"password": strongPassword,
	}

	jsonValue, _ := json.Marshal(loginData)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var responseBody map[string]string
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Contains(t, responseBody, "language")
	assert.Contains(t, responseBody, "date_format")
	var authCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	assert.NotNil(t, authCookie, "auth_token cookie should be set")
	assert.NotEmpty(t, authCookie.Value)
}

func TestLoginUser_InvalidCredentials(t *testing.T) {
	config := config.Config{
		JWTSecretKey:   "mysecretkey",
		JWTExpiryHours: 24,
	}
	_, router := setupRouter()
	router.POST("/login", func(c *gin.Context) {
		LoginUser(c, &config)
	})
	// Try to login with unregistered user
	loginData := map[string]string{
		"identifier": "wronguser@example.com",
		"password":   "wrongpassword",
	}

	jsonValue, _ := json.Marshal(loginData)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	errorDetail := response["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_CREDENTIALS", errorDetail["code"])
}

func TestLoginUser_InvalidInput(t *testing.T) {
	config := config.Config{
		JWTSecretKey: "mysecretkey",
	}
	_, router := setupRouter()
	router.POST("/login", func(c *gin.Context) {
		LoginUser(c, &config)
	})

	// Trying to login with invalid input (missing identifier)
	invalidData := map[string]string{
		"password": strongPassword,
	}

	jsonValue, _ := json.Marshal(invalidData)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	errorDetail := response["error"].(map[string]interface{})
	assert.Equal(t, "MISSING_FIELD", errorDetail["code"])
}
