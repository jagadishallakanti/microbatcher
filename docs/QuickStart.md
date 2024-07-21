## Installation

To install MicroBatcher, use the `go get` command:

```sh
go get github.com/jagadishallakanti/microbatcher
```

This will download the library and add it to your project's dependencies.
## Quick Start

Here's a quick example to get you started with MicroBatcher:

Step 1: Implement the process based on your requirement. Below is the simple example for the same. where we are processing the data and returning the processed data in JSON format.
```
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
```
Step 2: You need to instantiate the custom processor and pass it to the MicroBatcher as follows

```
	// Create a SimpleProcessor
	processor := &SimpleProcessor{}

	// Initialize the MicroBatching system
	batchSize := 3
	batchInterval := 2 * time.Second
	resultTTL := 10 * time.Second
	mb := microbatcher.NewMicroBatching[string](processor, batchSize, batchInterval, resultTTL)
```

Step 3: Submit the jobs to the MicroBatcher as follows

```
    // Submit some jobs to the MicroBatcher
    job1 := microbatcher.Job[string]{ID: "1", Data: "Job 1"}

    mb.SubmitJob(job1)
```

Step 4: Listen to the results from the MicroBatcher as follows

```
    // Listen for results from the MicroBatcher
    for result := range mb.Results() {
        fmt.Printf("Received result: %v\n", result)
    }
```
