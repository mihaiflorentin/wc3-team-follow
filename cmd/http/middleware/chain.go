package middleware

import "net/http"

// Chain collects middleware and wraps a handler in it. Execution order
// equals registration order: the first registered middleware is the
// outermost one and sees the request first.
type Chain struct {
	middleware []func(http.Handler) http.Handler
}

func NewChain() *Chain {
	return &Chain{}
}

// Register appends mw to the chain; it runs inside every middleware
// registered before it.
func (c *Chain) Register(mw func(http.Handler) http.Handler) {
	c.middleware = append(c.middleware, mw)
}

// Wrap returns handler wrapped so that the registered middleware runs in
// registration order.
func (c *Chain) Wrap(handler http.Handler) http.Handler {
	for i := len(c.middleware) - 1; i >= 0; i-- {
		handler = c.middleware[i](handler)
	}
	return handler
}
