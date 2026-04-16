package utils

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

type Response struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func Success(c *fiber.Ctx, status int, data interface{}) error {
	return c.Status(status).JSON(Response{
		Code:    fmt.Sprintf("JOURNEY-%d000", status),
		Message: "Success",
		Data:    data,
	})
}

func Error(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(Response{
		Code:    fmt.Sprintf("JOURNEY-%d000", status),
		Message: message,
		Error:   nil,
	})
}
