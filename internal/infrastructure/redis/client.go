package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	maxAttempts  = 10
	initialDelay = 1 * time.Second
	maxDelay     = 30 * time.Second
)

func Connect(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis parse url: %w", err)
	}

	// Production-ready pool settings
	opts.PoolSize = 20
	opts.MinIdleConns = 5
	opts.MaxIdleConns = 10
	opts.ConnMaxLifetime = 30 * time.Minute
	opts.ConnMaxIdleTime = 5 * time.Minute
	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second
	opts.PoolTimeout = 4 * time.Second // how long to wait for a free connection

	client := redis.NewClient(opts)

	delay := initialDelay
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := client.Ping(context.Background()).Err(); err == nil {
			return client, nil
		}

		if attempt == maxAttempts {
			_ = client.Close()
			return nil, fmt.Errorf("redis: could not connect after %d attempts", maxAttempts)
		}

		fmt.Printf("redis: connection attempt %d/%d failed, retrying in %s...\n", attempt, maxAttempts, delay)
		time.Sleep(delay)

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}

	_ = client.Close()
	return nil, fmt.Errorf("redis: could not connect after %d attempts", maxAttempts)
}
