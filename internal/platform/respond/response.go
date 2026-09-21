package respond

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Send sends a standardized JSON response.
func Send(c *gin.Context, httpStatus int, status string, message string, data any) {
	c.JSON(httpStatus, Response{Status: status, Message: message, Data: data})
}

// Success sends a success response with data.
func Success(c *gin.Context, data any) {
	Send(c, http.StatusOK, "success", "", data)
}

// SuccessMessage sends a success response with message and data.
func SuccessMessage(c *gin.Context, message string, data any) {
	Send(c, http.StatusOK, "success", message, data)
}

// Error sends an error response with message.
func Error(c *gin.Context, httpStatus int, message string) {
	Send(c, httpStatus, "error", message, nil)
}

// DecodeJSONBody caps the request body at maxBytes and strictly decodes it
// into target, rejecting unknown fields.
func DecodeJSONBody(c *gin.Context, target any, maxBytes int64) error {
	body := http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	c.Request.Body = body
	defer body.Close()

	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}
