package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/overgent/overgent/internal/config"
	"github.com/overgent/overgent/internal/credential"
	"github.com/overgent/overgent/internal/hosted"
)

type batchPublisher interface {
	PublishBatch(context.Context, []byte) (hosted.BatchAck, error)
	Heartbeat(context.Context, string, string) error
	CreateBrief(context.Context, string, string, string, int) (hosted.CoordinationBrief, error)
	Collaboration(context.Context, string) (hosted.CollaborationSnapshot, error)
}

type hostedSender struct{ client batchPublisher }

func NewHostedSender(ctx context.Context, root string) (Sender, error) {
	paths, err := config.Resolve(root)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return nil, err
	}
	if cfg.DeviceID == "" || cfg.APIBaseURL == "" {
		return nil, errors.New("service is not enrolled; run overgent create or join")
	}
	token, err := credential.Get(ctx, cfg.DeviceID)
	if err != nil {
		return nil, err
	}
	client, err := hosted.New(cfg.APIBaseURL, token)
	if err != nil {
		return nil, err
	}
	return hostedSender{client: client}, nil
}

func (s hostedSender) Send(ctx context.Context, _ string, batch []byte) error {
	var request struct {
		Events []struct {
			EventID string `json:"eventId"`
		} `json:"events"`
	}
	decoder := json.NewDecoder(bytes.NewReader(batch))
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("decode queued event batch: %w", err)
	}
	if len(request.Events) == 0 {
		return errors.New("queued event batch is empty")
	}
	ack, err := s.client.PublishBatch(ctx, batch)
	if err != nil {
		return err
	}
	accepted := make(map[string]struct{}, len(ack.AcceptedEventIDs))
	for _, id := range ack.AcceptedEventIDs {
		accepted[id] = struct{}{}
	}
	for _, event := range request.Events {
		if _, ok := accepted[event.EventID]; !ok {
			return fmt.Errorf("hosted acknowledgement omitted queued event %s", event.EventID)
		}
	}
	if ack.Cursor == "" {
		return errors.New("hosted acknowledgement omitted workspace cursor")
	}
	return nil
}

func (s hostedSender) Heartbeat(ctx context.Context, workspaceID, state string) error {
	return s.client.Heartbeat(ctx, workspaceID, state)
}

func (s hostedSender) CreateBrief(ctx context.Context, workstreamID, trigger, sinceCursor string, budget int) (hosted.CoordinationBrief, error) {
	return s.client.CreateBrief(ctx, workstreamID, trigger, sinceCursor, budget)
}
func (s hostedSender) Collaboration(ctx context.Context, projectID string) (hosted.CollaborationSnapshot, error) {
	return s.client.Collaboration(ctx, projectID)
}
