package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// A small background job queue stored in MongoDB (no Redis needed).
//
//   Producer : jobQueue.Enqueue(...)   puts a job in the "jobs" collection
//   Worker   : jobQueue.Run(...)       claims jobs, runs a handler, retries on failure
//   DLQ      : status "dead"           jobs that failed every attempt stay here for review

const (
	jobPending = "pending"
	jobRunning = "running"
	jobDone    = "done"
	jobDead    = "dead" // Dead Letter Queue

	jobMaxAttempts = 3
	jobLease       = 30 * time.Second // if a worker crashes, its job is re-claimed after this
	jobPollEvery   = time.Second
	jobBaseBackoff = 5 * time.Second
)

type job struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	Type        string             `bson:"type"`
	Key         string             `bson:"key"` // unique: the same key can never be queued twice
	Payload     map[string]string  `bson:"payload"`
	Status      string             `bson:"status"`
	Attempts    int                `bson:"attempts"`
	MaxAttempts int                `bson:"maxAttempts"`
	RunAt       time.Time          `bson:"runAt"`
	LockedUntil time.Time          `bson:"lockedUntil"`
	LastError   string             `bson:"lastError,omitempty"`
	CreatedAt   time.Time          `bson:"createdAt"`
}

type jobHandler func(ctx context.Context, j job) error

type jobQueue struct {
	jobs     *mongo.Collection
	handlers map[string]jobHandler
}

func newJobQueue(db *mongo.Database) (*jobQueue, error) {
	q := &jobQueue{jobs: db.Collection("jobs"), handlers: map[string]jobHandler{}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := q.jobs.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "runAt", Value: 1}}},
	})
	return q, err
}

func (q *jobQueue) Handle(jobType string, h jobHandler) { q.handlers[jobType] = h }

// Enqueue is the producer side. It returns quickly and never runs the slow work itself.
// Queuing the same key twice is silently ignored (first layer of idempotency).
func (q *jobQueue) Enqueue(ctx context.Context, jobType, key string, payload map[string]string) error {
	now := time.Now().UTC()
	_, err := q.jobs.InsertOne(ctx, job{
		Type: jobType, Key: key, Payload: payload, Status: jobPending,
		MaxAttempts: jobMaxAttempts, RunAt: now, CreatedAt: now,
	})
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

// claim atomically takes one due job, so two workers never get the same job.
// A "running" job whose lease expired belongs to a crashed worker and is taken again.
func (q *jobQueue) claim(ctx context.Context) (*job, error) {
	now := time.Now().UTC()
	filter := bson.M{"$or": bson.A{
		bson.M{"status": jobPending, "runAt": bson.M{"$lte": now}},
		bson.M{"status": jobRunning, "lockedUntil": bson.M{"$lt": now}},
	}}
	update := bson.M{
		"$set": bson.M{"status": jobRunning, "lockedUntil": now.Add(jobLease)},
		"$inc": bson.M{"attempts": 1},
	}
	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "runAt", Value: 1}}).
		SetReturnDocument(options.After)
	var j job
	err := q.jobs.FindOneAndUpdate(ctx, filter, update, opts).Decode(&j)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func backoffFor(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return jobBaseBackoff * time.Duration(1<<(attempt-1)) // 5s, 10s, 20s ...
}

func (q *jobQueue) finish(j *job, runErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	set := bson.M{}
	switch {
	case runErr == nil:
		set = bson.M{"status": jobDone}
	case j.Attempts >= j.MaxAttempts:
		log.Printf("job %s (%s) moved to dead letter queue: %v", j.ID.Hex(), j.Type, runErr)
		set = bson.M{"status": jobDead, "lastError": runErr.Error()}
	default:
		log.Printf("job %s (%s) failed attempt %d/%d, will retry: %v", j.ID.Hex(), j.Type, j.Attempts, j.MaxAttempts, runErr)
		set = bson.M{"status": jobPending, "lastError": runErr.Error(), "runAt": time.Now().UTC().Add(backoffFor(j.Attempts))}
	}
	if _, err := q.jobs.UpdateByID(ctx, j.ID, bson.M{"$set": set}); err != nil {
		log.Println("update job status:", err)
	}
}

func (q *jobQueue) process(ctx context.Context, j *job) {
	h, ok := q.handlers[j.Type]
	if !ok {
		q.finish(j, errors.New("no handler for job type "+j.Type))
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, jobLease-5*time.Second)
	defer cancel()
	q.finish(j, h(runCtx, *j))
}

// Run starts `workers` goroutines and blocks until ctx is cancelled.
// Graceful shutdown: a job that is already running is finished before Run returns.
func (q *jobQueue) Run(ctx context.Context, workers int) {
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				j, err := q.claim(ctx)
				if err != nil {
					log.Println("claim job:", err)
				}
				if j == nil {
					select {
					case <-ctx.Done():
					case <-time.After(jobPollEvery):
					}
					continue
				}
				q.process(context.Background(), j)
			}
		}()
	}
	wg.Wait()
}
