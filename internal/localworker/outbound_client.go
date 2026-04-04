package localworker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type websocketDialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (*websocket.Conn, *http.Response, error)
}

type outboundClient struct {
	mu     sync.Mutex
	conn   *websocket.Conn
	closed bool
}

func newOutboundClient(ctx context.Context, dialer websocketDialer, endpoint *store.WorkerEndpointData) (*outboundClient, error) {
	if endpoint == nil {
		return nil, fmt.Errorf("worker endpoint is required")
	}
	url := strings.TrimSpace(endpoint.EndpointURL)
	if url == "" {
		return nil, fmt.Errorf("worker endpoint URL is required")
	}
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}

	headers := make(http.Header)
	if token := strings.TrimSpace(endpoint.AuthToken); token != "" {
		headers.Set(DefaultOutboundAuthHeader, token)
	}

	conn, resp, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("dial outbound worker %q: %s: %w", endpoint.Name, resp.Status, err)
		}
		return nil, fmt.Errorf("dial outbound worker %q: %w", endpoint.Name, err)
	}
	return &outboundClient{conn: conn}, nil
}

func (c *outboundClient) Healthy() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.conn != nil
}

func (c *outboundClient) Dispatch(ctx context.Context, env OutboundEnvelope) error {
	return c.dispatch(ctx, env)
}

func (c *outboundClient) dispatch(ctx context.Context, env OutboundEnvelope) error {
	if c == nil {
		return fmt.Errorf("outbound worker client not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.conn == nil {
		return fmt.Errorf("outbound worker connection is closed")
	}
	deadline, hasDeadline := writeDeadlineFromContext(ctx)
	if hasDeadline {
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		defer func() {
			_ = c.conn.SetWriteDeadline(time.Time{})
		}()
	}
	if err := c.conn.WriteJSON(env); err != nil {
		c.closed = true
		_ = c.conn.Close()
		c.conn = nil
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (c *outboundClient) Close() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *outboundClient) ReadReply() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("outbound worker client not configured")
	}

	c.mu.Lock()
	if c.closed || c.conn == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("outbound worker connection is closed")
	}
	conn := c.conn
	c.mu.Unlock()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		c.mu.Lock()
		c.closed = true
		if c.conn != nil {
			_ = c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
		return nil, err
	}
	return msg, nil
}

func writeDeadlineFromContext(ctx context.Context) (time.Time, bool) {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline, true
	}
	if err := ctx.Err(); errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return time.Now(), true
	}
	return time.Time{}, false
}
