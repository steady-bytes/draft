package broker

import (
	"encoding/base64"
	"fmt"
	"sync"

	"connectrpc.com/connect"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

type (
	atomicMap struct {
		mu sync.RWMutex
		// Store the routine client connection
		m map[string][]*connect.ServerStream[acv1.ConsumeResponse]
		// yield the client connection to a thread, and then send events to it
		n map[string]chan *acv1.Message
	}
)

func newAtomicMap() *atomicMap {
	return &atomicMap{
		mu: sync.RWMutex{},
		m:  make(map[string][]*connect.ServerStream[acv1.ConsumeResponse]),
		n:  make(map[string]chan *acv1.Message),
	}
}

// hash to calculate the same key for two strings
func (am *atomicMap) hash(domain, msgKindName string) string {
	key := fmt.Sprintf("%s%s", domain, msgKindName)
	bs := []byte(key)
	return base64.StdEncoding.EncodeToString(bs)
}

func (am *atomicMap) Insert(key string, resStream *connect.ServerStream[acv1.ConsumeResponse]) {
	// TODO: Figure out how to start with a read lock?
	am.mu.RLock()
	defer am.mu.RUnlock()
	list, ok := am.m[key]
	if !ok {
		am.mu.RUnlock()
		am.mu.Lock()
		defer am.mu.Unlock()
		var list []*connect.ServerStream[acv1.ConsumeResponse]
		list = append(list, resStream)
		am.m[key] = list
	} else {
		list = append(list, resStream)
		am.m[key] = list
	}
}

func (am *atomicMap) Broker(key string, resStream *connect.ServerStream[acv1.ConsumeResponse]) {
	am.mu.RLock()
	ch, found := am.n[key]
	if !found {
		// create the channel to add to map
		ch := make(chan *acv1.Message)
		// store channel in map for future connections
		am.mu.RUnlock()
		am.mu.Lock()
		am.n[key] = ch
		am.mu.Unlock()
		// now start a new routine and keep it open as long as the `ch` channel has connected clients
		go am.send(ch, resStream)

		return
	} else {
		// the channel is already made and shared with other consumers, and producers so we can just use `ch`
		go am.send(ch, resStream)
		am.mu.RUnlock()
	}
}

func (am *atomicMap) send(ch chan *acv1.Message, stream *connect.ServerStream[acv1.ConsumeResponse]) {
	// when the channel receives a message send to the stream the client is holding onto
	for {
		m := <-ch
		msg := &acv1.ConsumeResponse{
			Message: m,
		}
		stream.Send(msg)
	}
}

func (am *atomicMap) Broadcast(key string, msg *acv1.Message) {
	ch, ok := am.n[key]
	if ok {
		ch <- msg
	} else {
		// we don't have any consumers that will listen to the message so as of right now
		// the message is not worth sending

		// TODO: We might consider a dead letter queue
		return
	}
}
