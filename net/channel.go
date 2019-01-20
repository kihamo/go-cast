package net

import (
	"sync"
	"sync/atomic"

	"golang.org/x/net/context"

	"github.com/barnybug/go-cast/api"
	"github.com/barnybug/go-cast/log"
)

type Channel struct {
	conn          *Connection
	sourceId      string
	DestinationId string
	namespace     string
	_             int32
	requestId     int64
	mutex         sync.Mutex
	inFlight      map[int]chan *api.CastMessage
	listeners     []channelListener
}

type channelListener struct {
	responseType string
	callback     func(*api.CastMessage)
}

type Payload interface {
	setRequestId(id int)
	getRequestId() int
}

func NewChannel(conn *Connection, sourceId, destinationId, namespace string) *Channel {
	return &Channel{
		conn:          conn,
		sourceId:      sourceId,
		DestinationId: destinationId,
		namespace:     namespace,
		listeners:     make([]channelListener, 0),
		inFlight:      make(map[int]chan *api.CastMessage),
	}
}

func (c *Channel) Message(message *api.CastMessage, headers *PayloadHeaders) {
	if *message.DestinationId != "*" && (*message.SourceId != c.DestinationId || *message.DestinationId != c.sourceId || *message.Namespace != c.namespace) {
		return
	}

	if headers.Type == "" && headers.ResponseType == "" {
		log.Errorf("Warning: No message type. Don't know what to do. headers: %v message:%v", headers, message)
		return
	}

	if headers.RequestId != nil && *headers.RequestId != 0 {
		c.mutex.Lock()
		listener, ok := c.inFlight[*headers.RequestId]
		c.mutex.Unlock()

		if ok {
			listener <- message

			c.mutex.Lock()
			delete(c.inFlight, *headers.RequestId)
			c.mutex.Unlock()
		}
	}

	for _, listener := range c.listeners {
		if listener.responseType == headers.Type || (headers.ResponseType != "" && listener.responseType == headers.ResponseType) {
			listener.callback(message)
		}
	}
}

func (c *Channel) OnMessage(responseType string, cb func(*api.CastMessage)) {
	c.listeners = append(c.listeners, channelListener{responseType, cb})
}

func (c *Channel) Send(payload interface{}) error {
	return c.conn.Send(payload, c.sourceId, c.DestinationId, c.namespace)
}

func (c *Channel) Request(ctx context.Context, payload Payload) (*api.CastMessage, error) {
	requestId := int(atomic.AddInt64(&c.requestId, 1))

	payload.setRequestId(requestId)
	response := make(chan *api.CastMessage)

	c.mutex.Lock()
	c.inFlight[requestId] = response
	c.mutex.Unlock()

	err := c.Send(payload)
	if err != nil {
		c.mutex.Lock()
		delete(c.inFlight, requestId)
		c.mutex.Unlock()

		return nil, err
	}

	select {
	case reply := <-response:
		return reply, nil
	case <-ctx.Done():
		c.mutex.Lock()
		delete(c.inFlight, requestId)
		c.mutex.Unlock()

		return nil, ctx.Err()
	}
}
