package corwhisk

import (
	"context"
	"fmt"
	"golang.org/x/time/rate"
	"time"
)

type Limiter interface {
	//Wait acquires a single token from the limiter, blocks until token is availibe or context is canceled.
	Wait(ctx context.Context) error

	//Allow releases a single token to the limiter
	Allow()
}

type ConcurrentRateLimiter struct {
	TokenPerMinute  int64
	ConcurrentToken int

	limiter *rate.Limiter
	tokens  chan struct{}
}

func NewConcurrentRateLimiter(tokenPerMinute int64, concurrentToken int) (*ConcurrentRateLimiter, error) {
	if tokenPerMinute <= 0 || concurrentToken <= 0 || tokenPerMinute < int64(concurrentToken) {
		return nil, fmt.Errorf("invalid values for tokenPerMinute: %d, concurrentToken: %d", tokenPerMinute, concurrentToken)
	}

	return &ConcurrentRateLimiter{
		limiter:         rate.NewLimiter(rate.Every(time.Minute/time.Duration(tokenPerMinute)), int(tokenPerMinute)),
		tokens:          make(chan struct{}, concurrentToken),
		TokenPerMinute:  tokenPerMinute,
		ConcurrentToken: concurrentToken,
	}, nil
}

func (c *ConcurrentRateLimiter) Wait(ctx context.Context) error {
	err := c.limiter.Wait(ctx)
	if err != nil {
		return err
	}
	c.tokens <- struct{}{}
	return nil
}

func (c *ConcurrentRateLimiter) Allow() {
	<-c.tokens
}
