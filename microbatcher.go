package microbatcher

import (
	"errors"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

// Job represents a task to be processed.
type Job[T any] struct {
	ID   string
	Data T
}

// JobResult represents the result of a processed job.
type JobResult[T any] struct {
	JobID    string
	Success  bool
	Result   T
	Error    error
	Time     time.Time
	Consumed bool
}

// BatchProcessor is an interface that should be implemented by the user of the library.
type BatchProcessor[T any] interface {
	Process(jobs []Job[T]) []JobResult[T]
}

// MicroBatching represents the micro-batching system.
type MicroBatching[T any] struct {
	processor      BatchProcessor[T]
	batchSize      int
	batchInterval  time.Duration
	jobQueue       chan Job[T]
	resultQueue    chan JobResult[T]
	shutdownFlag   chan struct{}
	shutdownWG     sync.WaitGroup
	results        map[string]chan JobResult[T]
	resultsMutex   sync.Mutex
	resultTTL      time.Duration
	cleanupTicker  *time.Ticker
	cleanupStopper chan struct{}
	logger         *logrus.Logger
}

// NewMicroBatching creates a new instance of the MicroBatching system.
func NewMicroBatching[T any](processor BatchProcessor[T], batchSize int, batchInterval, resultTTL time.Duration) *MicroBatching[T] {
	mb := &MicroBatching[T]{
		processor:      processor,
		batchSize:      batchSize,
		batchInterval:  batchInterval,
		jobQueue:       make(chan Job[T], batchSize*10),
		resultQueue:    make(chan JobResult[T], batchSize*10),
		shutdownFlag:   make(chan struct{}),
		results:        make(map[string]chan JobResult[T]),
		resultTTL:      resultTTL,
		cleanupStopper: make(chan struct{}),
		logger:         logrus.New(),
	}
	mb.logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	mb.logger.SetLevel(logrus.DebugLevel) // Set log level to Debug
	mb.shutdownWG.Add(1)
	go mb.processBatches()
	go mb.cleanupResults()
	return mb
}

// cleanupResults periodically cleans up job results based on the configured TTL.
func (mb *MicroBatching[T]) cleanupResults() {
	mb.cleanupTicker = time.NewTicker(mb.resultTTL)
	defer mb.cleanupTicker.Stop()

	for {
		select {
		case <-mb.cleanupTicker.C:
			mb.logger.Debug("Checking for any old consumed results for cleanup")
			mb.cleanupExpiredResults()
		case <-mb.cleanupStopper:
			return
		}
	}
}

// cleanupExpiredResults handles the logic of cleaning up expired results.
func (mb *MicroBatching[T]) cleanupExpiredResults() {
	now := time.Now()
	mb.resultsMutex.Lock()
	defer mb.resultsMutex.Unlock()
	for id, resultChan := range mb.results {
		select {
		case res := <-resultChan:
			if now.Sub(res.Time) > mb.resultTTL && res.Consumed {
				mb.logger.Debugf("Result for JobID %s is expired and cleaned up", id)
				close(resultChan)
				delete(mb.results, id)
			} else {
				// If result is not expired or not consumed, put it back in the channel
				resultChan <- res
			}
		default:
			continue
		}
	}
}

// SubmitJob submits a job to the micro-batching system.
func (mb *MicroBatching[T]) SubmitJob(job Job[T]) (<-chan JobResult[T], error) {
	mb.logger.Debugf("Attempting to submit job: %+v", job)

	// Check if the system is shutting down before attempting to enqueue the job
	select {
	case <-mb.shutdownFlag:
		mb.logger.Warn("System is shutting down, cannot accept new jobs")
		return nil, errors.New("system is shutting down, cannot accept new jobs")
	default:
		// Proceed to attempt to enqueue the job
		select {
		case mb.jobQueue <- job:
			mb.shutdownWG.Add(1) // Increment the WaitGroup counter
			// Only create the channel and map entry after successfully enqueuing the job
			resultChan := make(chan JobResult[T], 1)
			mb.resultsMutex.Lock()
			mb.results[job.ID] = resultChan
			mb.resultsMutex.Unlock()
			mb.logger.Debugf("Job %s added to job queue", job.ID)
			go func() {
				// Wait for the result to be processed and sent
				<-resultChan
				mb.shutdownWG.Done() // Decrement the WaitGroup counter
			}()
			return resultChan, nil
		case <-mb.shutdownFlag:
			// In case the system starts shutting down right after the initial check
			mb.logger.Warn("System started shutting down, cannot accept new jobs")
			return nil, errors.New("system started shutting down, cannot accept new jobs")
		}
	}
}

// processBatches processes jobs in batches and sends results to the resultQueue.
func (mb *MicroBatching[T]) processBatches() {
	defer mb.shutdownWG.Done()
	ticker := time.NewTicker(mb.batchInterval)
	defer ticker.Stop()

	var jobs []Job[T]
	for {
		select {
		case job := <-mb.jobQueue:
			mb.logger.Debugf("Received job: %+v", job)
			jobs = append(jobs, job)
			if len(jobs) >= mb.batchSize {
				mb.logger.Debugf("Processing batch of %d jobs", len(jobs))
				mb.processAndSendResults(jobs)
				jobs = nil
			}
		case <-ticker.C:
			if len(jobs) > 0 {
				mb.logger.Debugf("Processing batch of %d jobs due to ticker", len(jobs))
				mb.processAndSendResults(jobs)
				jobs = nil
			}
		case <-mb.shutdownFlag:
			if len(jobs) > 0 {
				mb.logger.Debugf("Shutting down after %d remaining jobs processed", len(jobs))
				mb.processAndSendResults(jobs)
			}
			return
		}
	}
}

// processAndSendResults processes a batch of jobs and sends results to the resultQueue.
func (mb *MicroBatching[T]) processAndSendResults(jobs []Job[T]) {
	results := mb.processor.Process(jobs)

	mb.resultsMutex.Lock()
	defer mb.resultsMutex.Unlock()

	for i, result := range results {
		result.Time = time.Now()
		result.Consumed = false
		if resultChan, exists := mb.results[jobs[i].ID]; exists {
			mb.logger.Debugf("Sending result for JobID %s: %+v", jobs[i].ID, result)
			resultChan <- result
			close(resultChan)
			delete(mb.results, jobs[i].ID)
		}
	}
}

// Shutdown gracefully shuts down the micro-batching system.
func (mb *MicroBatching[T]) Shutdown() {
	mb.logger.Info("Shutting down MicroBatching system")
	close(mb.shutdownFlag)
	mb.shutdownWG.Wait()

	close(mb.resultQueue)
	close(mb.jobQueue)
	mb.cleanupStopper <- struct{}{}
}
