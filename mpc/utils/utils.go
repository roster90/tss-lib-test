package utils

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// LogMessage logs messages with a consistent format
func LogMessage(msg string, args ...interface{}) {
	log.Printf(msg, args...)
}

// Retry function for retrying operations that might fail
func Retry(attempts int, sleep time.Duration, operation func() error) error {
	for i := 0; i < attempts; i++ {
		err := operation()
		if err == nil {
			return nil
		}
		log.Printf("Attempt %d failed: %v", i+1, err)
		time.Sleep(sleep)
	}
	return fmt.Errorf("operation failed after %d attempts", attempts)
}

// FormatErrorMessage generates a formatted error message
func FormatErrorMessage(nodeID string, err error) string {
	return fmt.Sprintf("Error in node %s: %v", nodeID, err)
}

// WaitGroupWait safely waits for a group of nodes to be ready
func WaitGroupWait(wg *sync.WaitGroup) {
	wg.Wait()
}

// SleepWithLog pauses execution with a log message
func SleepWithLog(duration time.Duration, msg string) {
	log.Println(msg)
	time.Sleep(duration)
}
