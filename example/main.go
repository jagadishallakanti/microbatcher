package main

import (
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"microbatcher" // Adjust the import path as necessary
	"time"
)

// SimpleProcessor is a concrete implementation of BatchProcessor.
type SimpleProcessor struct{}

// ProcessedData represents the structure of the data after processing, to be encoded in JSON.
type ProcessedData struct {
	Message string `json:"message"`
}

func (sp *SimpleProcessor) Process(jobs []microbatcher.Job[string]) []microbatcher.JobResult[string] {
	var results []microbatcher.JobResult[string]
	for _, job := range jobs {
		// Create an instance of ProcessedData with the processed message.
		processedData := ProcessedData{
			Message: fmt.Sprintf("Processed data: %s", job.Data),
		}

		// Marshal the processedData into JSON format.
		jsonResult, err := json.Marshal(processedData)
		if err != nil {
			// Handle the error appropriately; for simplicity, log and continue here.
			fmt.Printf("Error marshalling JSON: %v\n", err)
			continue
		}

		// Append the JSON result to the results slice.
		results = append(results, microbatcher.JobResult[string]{
			JobID:   job.ID,
			Success: true,
			Result:  string(jsonResult), // Convert the JSON bytes to string
			Time:    time.Now(),
		})
	}
	return results
}

func main() {
	// Initialize the logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Create a SimpleProcessor
	processor := &SimpleProcessor{}

	// Initialize the MicroBatching system
	batchSize := 3
	batchInterval := 2 * time.Second
	resultTTL := 10 * time.Second
	mb := microbatcher.NewMicroBatching[string](processor, batchSize, batchInterval, resultTTL)

	// Submit jobs
	for i := 0; i < 10; i++ {
		job := microbatcher.Job[string]{ID: fmt.Sprintf("job-%d", i), Data: fmt.Sprintf("data-%d", i)}
		resultChan, err := mb.SubmitJob(job)
		if err != nil {
			logger.Errorf("Failed to submit job %s: %v", job.ID, err)
			continue
		}

		go func(jobID string) {
			// Wait for the job result to consume
			for res := range resultChan {
				if res.Consumed {
					logger.Infof("Result for job ID %s: Success=%v, Result=%s", jobID, res.Success, res.Result)
					return
				}
			}
		}(job.ID)
	}

	// Allow some time for jobs to be processed
	time.Sleep(5 * time.Second)

	// Shutdown the system
	mb.Shutdown()

	logger.Info("MicroBatching demo complete")
}
