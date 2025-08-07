package utils

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alrifqi/multitenant-messaging/internal/auth"
	"github.com/alrifqi/multitenant-messaging/internal/models"
	"github.com/stretchr/testify/require"
)

// TestHelper provides common test utilities
type TestHelper struct {
	JWTManager *auth.JWTManager
}

// NewTestHelper creates a new test helper
func NewTestHelper(jwtManager *auth.JWTManager) *TestHelper {
	return &TestHelper{
		JWTManager: jwtManager,
	}
}

// GetTestToken generates a test JWT token
func (th *TestHelper) GetTestToken(t *testing.T) string {
	token, err := th.JWTManager.GenerateToken(1, "test-user", 1)
	require.NoError(t, err)
	return token
}

// CreateTestTenantRequest creates a test tenant creation request
func (th *TestHelper) CreateTestTenantRequest(t *testing.T, name string) (*http.Request, *httptest.ResponseRecorder) {
	reqBody := models.CreateTenantRequest{Name: name}
	reqJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+th.GetTestToken(t))

	rec := httptest.NewRecorder()
	return req, rec
}

// CreateTestMessageRequest creates a test message creation request
func (th *TestHelper) CreateTestMessageRequest(t *testing.T, tenantID int, queueName, messageBody string) (*http.Request, *httptest.ResponseRecorder) {
	reqBody := models.CreateMessageRequest{
		QueueName:   queueName,
		MessageBody: messageBody,
	}
	reqJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/tenants/"+string(rune(tenantID))+"/messages",
		bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+th.GetTestToken(t))

	rec := httptest.NewRecorder()
	return req, rec
}

// CreateTestConsumerConfigRequest creates a test consumer configuration request
func (th *TestHelper) CreateTestConsumerConfigRequest(t *testing.T, tenantID int, queueName string, workers int) (*http.Request, *httptest.ResponseRecorder) {
	reqBody := models.UpdateConsumerConfigRequest{Workers: workers}
	reqJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/tenants/"+string(rune(tenantID))+"/consumers/"+queueName+"/config",
		bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+th.GetTestToken(t))

	rec := httptest.NewRecorder()
	return req, rec
}

// ParseAPIResponse parses an API response
func (th *TestHelper) ParseAPIResponse(t *testing.T, rec *httptest.ResponseRecorder) models.APIResponse {
	var response models.APIResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	return response
}

// AssertSuccessResponse asserts that the response indicates success
func (th *TestHelper) AssertSuccessResponse(t *testing.T, rec *httptest.ResponseRecorder, expectedStatus int) {
	response := th.ParseAPIResponse(t, rec)
	require.Equal(t, expectedStatus, rec.Code)
	require.True(t, response.Success)
}

// AssertErrorResponse asserts that the response indicates an error
func (th *TestHelper) AssertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, expectedStatus int) {
	response := th.ParseAPIResponse(t, rec)
	require.Equal(t, expectedStatus, rec.Code)
	require.False(t, response.Success)
	require.NotEmpty(t, response.Error)
}
