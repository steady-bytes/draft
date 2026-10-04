// Command verify-symbol-changed is a Phase 9 diagnostic: opens a real Catalyst Consume stream and
// prints every tooling.allele.v1.SymbolChanged event it receives until a timeout -- the same
// kept-diagnostic-tool precedent cmd/parse, cmd/diff, and cmd/merge set in earlier phases, not a
// one-shot scratch script. This is the standalone subscriber the plan's own Phase 9 deliverable asks
// for: "a standalone Consume subscriber receives a real tooling.allele.v1.SymbolChanged event during
// a live push."
//
// Usage: go run ./cmd/verify-symbol-changed [catalyst-addr] [timeout-seconds] [event-type]
// event-type defaults to tooling.allele.v1.SymbolChanged; pass tooling.allele.v1.ConflictDetected or
// tooling.allele.v1.VerificationCompleted to check Phase 9's other two wired event types with this
// same tool, since all three share one publish code path (events.go's catalystPublisher.publish) --
// no need for a second tool to exercise the same CloudEvent envelope/transport logic again.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	acConnect "github.com/steady-bytes/draft/api/core/message_broker/actors/v1/v1connect"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
)

const defaultEventType = "tooling.allele.v1.SymbolChanged"

func main() {
	addr := "http://localhost:2220"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	timeoutSec := 30
	if len(os.Args) > 2 {
		if t, err := strconv.Atoi(os.Args[2]); err == nil {
			timeoutSec = t
		}
	}
	eventType := defaultEventType
	if len(os.Args) > 3 {
		eventType = os.Args[3]
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	client := acConnect.NewConsumerClient(h2cClient(), addr, connect.WithGRPC())
	stream, err := client.Consume(ctx, connect.NewRequest(&acv1.ConsumeRequest{
		Message: &acv1.CloudEvent{
			Source: "/cmd/verify-symbol-changed",
			Type:   eventType,
		},
	}))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open consume stream: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("listening for %s (timeout %ds) ...\n", eventType, timeoutSec)
	for stream.Receive() {
		event := stream.Msg().GetMessage()
		if event == nil || event.GetType() != eventType {
			continue
		}
		fmt.Printf("RECEIVED: type=%s source=%s data=%s\n", event.GetType(), event.GetSource(), event.GetTextData())
	}
	if err := stream.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stream ended: %v\n", err)
	}
}

// h2cClient: see events.go's own identical helper -- required for gRPC streaming against Catalyst's
// plain-TCP server.
func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}
