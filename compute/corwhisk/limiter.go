package corwhisk

import (
	"context"
	"fmt"
	"golang.org/x/time/rate"
	"sync"
	"time"
)

type Limiter interface {
	//Wait acquires a single token from the limiter, blocks until token is availibe or context is canceled.
	Wait(ctx context.Context) error
	//Wait acquires n token from the limiter, blocks until token is availibe or context is canceled.
	WaitN(ctx context.Context, n int) error
	//Allow releases a single token to the limiter
	Allow()
}

type ConcurrentRateLimiter struct {
	TokenPerMinute  int64
	ConcurrentToken int

	limiter    *rate.Limiter
	waitLocker sync.Mutex
	tokens     chan struct{}
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

func (c *ConcurrentRateLimiter) WaitN(ctx context.Context, n int) error {
	if n > c.ConcurrentToken {
		return fmt.Errorf("can't reserve more token than concurrent limit. limit:%d requested:%d", c.ConcurrentToken, n)
	}
	err := c.limiter.WaitN(ctx, int(n))
	if err != nil {
		return err
	}
	c.waitLocker.Lock()
	for i := 0; i < n; i++ {
		c.tokens <- struct{}{}
	}
	c.waitLocker.Unlock()
	return nil
}

func (c *ConcurrentRateLimiter) Wait(ctx context.Context) error {
	return c.WaitN(ctx, 1)
}

func (c *ConcurrentRateLimiter) Allow() {
	<-c.tokens
}

//TryAllow tries to release a single token to the limiter times out after 1 second. returns false on timeout.
func (c *ConcurrentRateLimiter) TryAllow() bool {
	select {
	case <-c.tokens:
		return true
	case <-time.After(time.Second):
		return false
	}
}
