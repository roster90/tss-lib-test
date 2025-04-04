package handler

import (
	"log"
)

// ErrorHandler defines the interface for handling errors
type ErrorHandler interface {
	HandleError(err error)
}

// HandleErrorMessages handles error messages from the TSS protocol
func HandleErrorMessages(errCh chan error) {
	for err := range errCh {
		log.Printf("Error in TSS protocol: %v", err)
	}
}
