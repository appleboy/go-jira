package util

import (
	"os"
	"strings"
)

// GetGlobalValue returns the value of an environment variable.
// It first checks if there is an environment variable with the format "INPUT_<KEY>",
// where "<KEY>" is the input key converted to uppercase.
// If the "INPUT_<KEY>" environment variable doesn't exist or is empty,
// it returns the value of the "<KEY>" environment variable.
//
// Parameters:
//
//	key - the key of the environment variable to retrieve.
//
// Returns:
//
//	string - the value of the environment variable.
func GetGlobalValue(key string) string {
	key = strings.ToUpper(key) // Convert key to uppercase

	// Check if there is an environment variable with the format "INPUT_<KEY>"
	if value := os.Getenv("INPUT_" + key); value != "" {
		return value // Return the value of the "INPUT_<KEY>" environment variable
	}

	// If the "INPUT_<KEY>" environment variable doesn't exist or is empty,
	// return the value of the "<KEY>" environment variable
	return os.Getenv(key)
}

// ToBool converts a string to a boolean value.
// It returns true if the input string is "true" (case insensitive) or "1",
// and false otherwise.
//
// Parameters:
//
//	s - the input string to be converted to a boolean.
//
// Returns:
//
//	bool - the boolean representation of the input string.
func ToBool(s string) bool {
	return strings.ToLower(s) == "true" || s == "1"
}
