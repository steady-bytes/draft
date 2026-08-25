package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	stepexecutorv1connect "github.com/steady-bytes/draft/api/tooling/step_executor/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
)

// This file implements StepExecutor.Execute (see
// api/tooling/step_executor/v1/service.proto) — the fixed, minimal contract
// every Garage plugin implements, called directly by whatever resolved this
// plugin (Bench, once its own Phase 6 garage:// resolution exists).
//
// Execute expects req.Msg.Config to already be fully resolved/templated —
// the step's `with:` block after any `{{ steps.X.result }}`-style
// substitution has already happened upstream, not a template string this
// handler would need to resolve itself. See channelAndMessage below.

type (
	Handler interface {
		chassis.RPCRegistrar
		stepexecutorv1connect.StepExecutorHandler
	}
	handler struct {
		logger     chassis.Logger
		httpClient *http.Client
		// webhookURL is read lazily (rather than captured once at
		// construction) so a config reload or a test double can vary it
		// per call without rebuilding the handler.
		webhookURL func() string
	}
)

func NewHandler(logger chassis.Logger, httpClient *http.Client, webhookURL func() string) Handler {
	return &handler{
		logger:     logger,
		httpClient: httpClient,
		webhookURL: webhookURL,
	}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := stepexecutorv1connect.NewStepExecutorHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

// Execute posts a message to a Slack channel via the configured incoming
// webhook. A returned Go error is reserved for the RPC call itself being
// malformed (a nil Config); an ordinary "the step's work failed" case (a
// missing required config field, a non-2xx from Slack) is reported through
// StepResponse.Success/Error instead, per the design doc's contract.
func (h *handler) Execute(ctx context.Context, req *connect.Request[stepexecutorv1.StepRequest]) (*connect.Response[stepexecutorv1.StepResponse], error) {
	msg := req.Msg
	if msg.GetConfig() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config is required"))
	}

	channel, message, err := channelAndMessage(msg.GetConfig())
	if err != nil {
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	webhookURL := h.webhookURL()
	if webhookURL == "" {
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   "slack.webhook_url is not configured",
		}), nil
	}

	respBody, err := postToSlack(ctx, h.httpClient, webhookURL, slackWebhookPayload{
		Channel: channel,
		Text:    message,
	})
	if err != nil {
		h.logger.WithError(err).WithField("channel", channel).Error("failed to post to slack")
		return connect.NewResponse(&stepexecutorv1.StepResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	result, err := resultStruct(respBody)
	if err != nil {
		// A malformed result is not a failed step — the message was posted
		// successfully; there's just nothing structured to hand back to
		// later steps. Log and fall back to an empty (non-nil) result.
		h.logger.WithError(err).Warn("failed to build result struct from slack response; returning empty result")
		result = &structpb.Struct{}
	}

	return connect.NewResponse(&stepexecutorv1.StepResponse{
		Success: true,
		Result:  result,
	}), nil
}

// channelAndMessage extracts the required `channel`/`message` string fields
// from config, matching config_schema in manifest.go (required: [channel,
// message], both type: string). config is the step's `with:` block passed
// through opaquely by StepRequest — see StepRequest.config's doc comment in
// api/tooling/step_executor/v1/service.proto.
func channelAndMessage(config *structpb.Struct) (channel, message string, err error) {
	fields := config.GetFields()

	channelVal, ok := fields["channel"]
	if !ok || channelVal.GetStringValue() == "" {
		return "", "", errors.New("config.channel is required")
	}
	messageVal, ok := fields["message"]
	if !ok || messageVal.GetStringValue() == "" {
		return "", "", errors.New("config.message is required")
	}

	return channelVal.GetStringValue(), messageVal.GetStringValue(), nil
}

// resultStruct builds StepResponse.Result from Slack's webhook response
// body, matching result_schema in manifest.go (message_ts: string).
//
// A real Slack incoming webhook responds with the plain text "ok", not
// JSON — message_ts is a chat.postMessage-API concept, not something
// incoming webhooks actually return. That's a property of Slack's API, not
// a bug here: a non-JSON body isn't an error, it just means there's no
// message_ts to surface, and Result comes back as an empty (non-nil)
// Struct. If the body is JSON with a "ts" or "message_ts" field (as this
// package's own httptest-based tests, and any Slack-API-compatible test
// double, return), it's surfaced as message_ts.
func resultStruct(respBody []byte) (*structpb.Struct, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return &structpb.Struct{}, nil
	}

	result := map[string]interface{}{}
	if ts, ok := parsed["ts"]; ok {
		result["message_ts"] = ts
	} else if ts, ok := parsed["message_ts"]; ok {
		result["message_ts"] = ts
	}

	return structpb.NewStruct(result)
}
