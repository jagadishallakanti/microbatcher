package microbatcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockBatchProcessor is a mock implementation of the BatchProcessor interface
type MockBatchProcessor[T any] struct {
	ProcessFunc func(jobs []Job[T]) []JobResult[T]
}

func (m *MockBatchProcessor[T]) Process(jobs []Job[T]) []JobResult[T] {
	if m.ProcessFunc != nil {
		return m.ProcessFunc(jobs)
	}
	return nil
}

func TestNewMicroBatching(t *testing.T) {
	mockProcessor := &MockBatchProcessor[int]{}
	batchSize := 10
	batchInterval := time.Millisecond * 100
	resultTTL := time.Second * 10

	mb := NewMicroBatching[int](mockProcessor, batchSize, batchInterval, resultTTL)

	require.NotNil(t, mb)
	assert.Equal(t, mockProcessor, mb.processor)
	assert.Equal(t, batchSize, mb.batchSize)
	assert.Equal(t, batchInterval, mb.batchInterval)
	assert.Equal(t, resultTTL, mb.resultTTL)
	assert.NotNil(t, mb.jobQueue)
	assert.NotNil(t, mb.resultQueue)
	assert.NotNil(t, mb.shutdownFlag)
	assert.NotNil(t, mb.results)
	assert.NotNil(t, mb.cleanupStopper)
}

func TestSubmitJob_Success(t *testing.T) {
	mockProcessor := &MockBatchProcessor[int]{
		ProcessFunc: func(jobs []Job[int]) []JobResult[int] {
			results := make([]JobResult[int], len(jobs))
			for i, job := range jobs {
				results[i] = JobResult[int]{JobID: job.ID, Success: true, Result: job.Data, Time: time.Now(), Consumed: false}
			}
			return results
		},
	}
	mb := NewMicroBatching[int](mockProcessor, 2, time.Millisecond*100, time.Second*1)

	job := Job[int]{ID: "1", Data: 42}
	resultChan, err := mb.SubmitJob(job)

	require.NoError(t, err)
	require.NotNil(t, resultChan)
}

func TestSubmitJob_Shutdown(t *testing.T) {
	mockProcessor := &MockBatchProcessor[int]{}
	mb := NewMicroBatching[int](mockProcessor, 2, time.Millisecond*100, time.Second*1)

	// Shutdown the micro-batching system
	close(mb.shutdownFlag)

	job := Job[int]{ID: "1", Data: 42}
	resultChan, err := mb.SubmitJob(job)

	require.Error(t, err)
	assert.Nil(t, resultChan)
}

func TestProcessBatches_NormalOperation(t *testing.T) {
	// Initialize a mock processor with a custom process function
	var processedJobs []Job[int]
	mockProcessor := &MockBatchProcessor[int]{
		ProcessFunc: func(jobs []Job[int]) []JobResult[int] {
			// Record the processed jobs for verification
			processedJobs = append(processedJobs, jobs...)
			// Generate mock results for each job
			results := make([]JobResult[int], len(jobs))
			for i, job := range jobs {
				results[i] = JobResult[int]{JobID: job.ID, Success: true, Result: job.Data, Time: time.Now(), Consumed: false}
			}
			return results
		},
	}

	// Configuration for the MicroBatching system
	batchSize := 2
	batchInterval := time.Millisecond * 50
	resultTTL := time.Second * 1

	// Create a new MicroBatching instance with the mock processor
	mb := NewMicroBatching[int](mockProcessor, batchSize, batchInterval, resultTTL)

	// Define jobs to be submitted
	jobs := []Job[int]{
		{ID: "1", Data: 42},
		{ID: "2", Data: 43},
		{ID: "3", Data: 44},
	}

	// Submit jobs to the MicroBatching system
	for _, job := range jobs {
		_, err := mb.SubmitJob(job)
		require.NoError(t, err)
	}

	// Allow time for batch processing
	time.Sleep(time.Millisecond * 150)

	// Verify that all jobs were processed
	assert.ElementsMatch(t, jobs, processedJobs)
}

func TestProcessBatches_PartialBatch(t *testing.T) {
	// Initialize a mock processor with a custom process function
	var processedJobs []Job[int]
	mockProcessor := &MockBatchProcessor[int]{
		ProcessFunc: func(jobs []Job[int]) []JobResult[int] {
			// Record the processed jobs for verification
			processedJobs = append(processedJobs, jobs...)
			// Generate mock results for each job
			results := make([]JobResult[int], len(jobs))
			for i, job := range jobs {
				results[i] = JobResult[int]{JobID: job.ID, Success: true, Result: job.Data, Time: time.Now(), Consumed: false}
			}
			return results
		},
	}

	// Configuration for the MicroBatching system
	batchSize := 3 // Set batch size larger than the number of jobs to simulate a partial batch
	batchInterval := time.Millisecond * 50
	resultTTL := time.Second * 1

	// Create a new MicroBatching instance with the mock processor
	mb := NewMicroBatching[int](mockProcessor, batchSize, batchInterval, resultTTL)

	// Define jobs to be submitted, less than the batch size to create a partial batch
	jobs := []Job[int]{
		{ID: "1", Data: 42},
		{ID: "2", Data: 43},
	}

	// Submit jobs to the MicroBatching system
	for _, job := range jobs {
		_, err := mb.SubmitJob(job)
		require.NoError(t, err)
	}

	// Allow time for the batch interval to elapse and process the partial batch
	time.Sleep(time.Millisecond * 100)

	// Verify that all jobs in the partial batch were processed
	assert.ElementsMatch(t, jobs, processedJobs)
}

func TestResultAvailableInChannel(t *testing.T) {
	// Setup a mock processor that immediately returns a result for any job processed
	mockProcessor := &MockBatchProcessor[int]{
		ProcessFunc: func(jobs []Job[int]) []JobResult[int] {
			results := make([]JobResult[int], len(jobs))
			for i, job := range jobs {
				results[i] = JobResult[int]{JobID: job.ID, Success: true, Result: job.Data, Time: time.Now(), Consumed: false}
			}
			return results
		},
	}

	// Initialize the MicroBatching system with a short batch interval for quick processing
	mb := NewMicroBatching[int](mockProcessor, 1, 10*time.Millisecond, 1*time.Second)

	// Submit a job to the system
	job := Job[int]{ID: "test-job", Data: 100}
	resultChan, err := mb.SubmitJob(job)
	require.NoError(t, err, "Submitting job should not produce an error")

	// Wait for the job to be processed and the result to be available in the channel
	select {
	case result := <-resultChan:
		assert.Equal(t, job.ID, result.JobID, "The result should have the correct JobID")
		assert.True(t, result.Success, "The result should indicate success")
		assert.Equal(t, job.Data, result.Result, "The result should contain the correct job data")
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for job result")
	}
}

func TestCleanupExpiredResults(t *testing.T) {
	// Setup a mock processor that immediately returns a result for any job processed
	mockProcessor := &MockBatchProcessor[int]{
		ProcessFunc: func(jobs []Job[int]) []JobResult[int] {
			results := make([]JobResult[int], len(jobs))
			for i, job := range jobs {
				// Set the result time to the past to simulate expiration
				results[i] = JobResult[int]{JobID: job.ID, Success: true, Result: job.Data, Time: time.Now().Add(-2 * time.Hour), Consumed: false}
			}
			return results
		},
	}

	// Initialize the MicroBatching system with a short TTL to force quick cleanup
	mb := NewMicroBatching[int](mockProcessor, 1, 10*time.Millisecond, 50*time.Millisecond)

	// Submit a job to the system
	job := Job[int]{ID: "expired-job", Data: 100}
	_, err := mb.SubmitJob(job)
	require.NoError(t, err, "Submitting job should not produce an error")

	// Wait a bit longer than the TTL for the cleanup to run
	time.Sleep(100 * time.Millisecond)

	// Check if the result has been cleaned up
	mb.resultsMutex.Lock()
	defer mb.resultsMutex.Unlock()
	_, exists := mb.results[job.ID]
	assert.False(t, exists, "The result for the expired job should have been cleaned up")
}

func TestShutdownWithResultConsumption(t *testing.T) {
	mockProcessor := &MockBatchProcessor[string]{
		ProcessFunc: func(jobs []Job[string]) []JobResult[string] {
			results := make([]JobResult[string], len(jobs))
			for i, job := range jobs {
				results[i] = JobResult[string]{JobID: job.ID, Success: true, Result: job.Data, Time: time.Now(), Consumed: false}
			}
			return results
		},
	}
	mb := NewMicroBatching[string](mockProcessor, 3, 1*time.Second, 2*time.Second)

	// Submit jobs
	jobs := []Job[string]{
		{ID: "1", Data: "data1"},
		{ID: "2", Data: "data2"},
	}
	for _, job := range jobs {
		resultChan, err := mb.SubmitJob(job)
		require.NoError(t, err, "Submitting job should not produce an error")

		// Simulate result consumption
		go func(resultChan <-chan JobResult[string]) {
			for result := range resultChan {
				result.Consumed = true
			}
		}(resultChan)
	}

	// Allow some time for jobs to be processed and consumed
	time.Sleep(100 * time.Millisecond)

	// Shutdown the system
	mb.Shutdown()

	// Attempt to submit another job after shutdown
	_, err := mb.SubmitJob(Job[string]{ID: "3", Data: "data3"})
	assert.Error(t, err, "SubmitJob should return an error after shutdown")

	// Verify shutdownFlag is closed
	select {
	case <-mb.shutdownFlag:
		// Expected case: shutdownFlag should be closed
	default:
		t.Fatal("Expected shutdownFlag to be closed")
	}

	// Verify all job results are processed and channels closed
	mb.resultsMutex.Lock()
	defer mb.resultsMutex.Unlock()
	for id, ch := range mb.results {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("Channel for job ID %s not properly closed", id)
			}
		default:
			t.Errorf("Channel for job ID %s still open", id)
		}
	}

	// Optionally, verify that the results map is empty
	assert.Empty(t, mb.results, "All job results should be consumed and results map should be empty")
}
