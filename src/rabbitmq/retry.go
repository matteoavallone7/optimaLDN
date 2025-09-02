package rabbitmq

import "time"

func ExponentialBackoff(attempt int) time.Duration {
	base := time.Millisecond * 500
	return base * (1 << attempt)
}
