package GatewayFlowController

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/RSS3-Network/GatewayFlowController/holster/collections"
	"golang.org/x/time/rate"
)

// Basically just https://github.com/traefik/traefik/blob/master/pkg/middlewares/ratelimiter/rate_limiter.go , with some custom modifications

type RateLimiter struct {
	rate  rate.Limit // reqs/s
	burst int64
	// maxDelay is the maximum duration we're willing to wait for a bucket reservation to become effective, in nanoseconds.
	// For now it is somewhat arbitrarily set to 1/(2*rate).
	maxDelay time.Duration
	// each rate limiter for a given source is stored in the buckets ttlmap.
	// To keep this ttlmap constrained in size,
	// each ratelimiter is "garbage collected" when it is considered expired.
	// It is considered expired after it hasn't been used for ttl seconds.
	ttl int

	buckets *collections.TTLMap
}

// Configurations explain:

// Average is the maximum rate, by default in requests/s, allowed for the given source.
// 0 means no rate limiting.
// The rate is actually defined by dividing Average by Period. So for a rate below 1req/s,
// one needs to define a Period larger than a second.

// Period, in combination with Average, defines the actual maximum rate, such as:
// r = Average / Period.

// Burst is the maximum number of requests allowed to arrive in the same arbitrarily small period of time.

func NewRateLimiter(maxSources int, average int64, period time.Duration, burst int64) (*RateLimiter, error) {
	// Tweak configurations
	if maxSources <= 0 {
		return nil, fmt.Errorf("invalid max sources")
	}

	if period <= 0 {
		period = time.Second
	}

	if burst < 1 {
		burst = 1
	}

	// Create TTLMap buckets
	buckets := collections.NewTTLMap(maxSources)

	// Initialized at rate.Inf to enforce no rate limiting when config.Average == 0
	rtl := float64(rate.Inf)
	// No need to set any particular value for maxDelay as the reservation's delay
	// will be <= 0 in the Inf case (i.e. the average == 0 case).
	var maxDelay time.Duration

	if average > 0 {
		rtl = float64(average*int64(time.Second)) / float64(period)
		// maxDelay does not scale well for rates below 1,
		// so we just cap it to the corresponding value, i.e. 0.5s, in order to keep the effective rate predictable.
		// One alternative would be to switch to a no-reservation mode (Allow() method) whenever we are in such a low rate regime.
		if rtl < 1 {
			maxDelay = 500 * time.Millisecond
		} else {
			maxDelay = time.Second / (time.Duration(rtl) * 2)
		}
	}

	// Make the ttl inversely proportional to how often a rate limiter is supposed to see any activity (when maxed out),
	// for low rate limiters.
	// Otherwise just make it a second for all the high rate limiters.
	// Add an extra second in both cases for continuity between the two cases.
	ttl := 1
	if rtl >= 1 {
		ttl++
	} else if rtl > 0 {
		ttl += int(1 / rtl)
	}

	return &RateLimiter{
		rate:     rate.Limit(rtl),
		burst:    burst,
		maxDelay: maxDelay,
		ttl:      ttl,
		buckets:  buckets,
	}, nil
}

func (rl *RateLimiter) RateLimit(rw http.ResponseWriter, req *http.Request, account *string) bool {
	var (
		source  string
		errCode int
	)

	if account != nil {
		source = *account
		errCode = http.StatusPaymentRequired
	} else {
		source = GetIP(req)
		errCode = http.StatusTooManyRequests
	}

	// Check rate limiter bucket
	var bucket *rate.Limiter
	if rlSource, exists := rl.buckets.Get(source); exists {
		bucket = rlSource.(*rate.Limiter)
	} else {
		bucket = rate.NewLimiter(rl.rate, int(rl.burst))
	}

	// We Set even in the case where the source already exists,
	// because we want to update the expiryTime everytime we get the source,
	// as the expiryTime is supposed to reflect the activity (or lack thereof) on that source.
	if err := rl.buckets.Set(source, bucket, rl.ttl); err != nil {
		_, _ = os.Stderr.WriteString("could not insert/update bucket")
		errCode = http.StatusInternalServerError
		http.Error(rw, http.StatusText(errCode), errCode)

		return false
	}

	res := bucket.Reserve()
	if !res.OK() {
		//_, _ = os.Stdout.WriteString("Reject request: no bursty traffic allowed\n")
		http.Error(rw, http.StatusText(errCode), errCode)

		return false
	}

	delay := res.Delay()
	if delay > rl.maxDelay {
		res.Cancel()
		rw.Header().Set("Retry-After", fmt.Sprintf("%.0f", math.Ceil(delay.Seconds())))
		rw.Header().Set("X-Retry-In", delay.String())

		//_, _ = os.Stdout.WriteString("Reject request: delay requests\n")
		http.Error(rw, http.StatusText(errCode), errCode)

		return false
	}

	time.Sleep(delay)

	return true // Able to proceed
}
