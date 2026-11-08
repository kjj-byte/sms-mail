package identity

import (
	"context"
	"sync"
)

type catalogRead[T any] struct {
	Code    string
	Value   T
	Err     error
	Started bool
}

// Bound upstream concurrency and merge results on the caller's goroutine.
func readItems[T any](ctx context.Context, codes []string, read func(string) (T, error)) []catalogRead[T] {
	results := make([]catalogRead[T], len(codes))
	jobs := make(chan int, len(codes))
	for i := range codes {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for worker := 0; worker < 3; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i].Code = codes[i]
				if ctx.Err() != nil {
					results[i].Err = ctx.Err()
					continue
				}
				results[i].Started = true
				results[i].Value, results[i].Err = read(codes[i])
			}
		}()
	}
	wg.Wait()
	return results
}
