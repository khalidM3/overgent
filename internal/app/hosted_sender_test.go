package app

import (
	"context"
	"errors"
	"testing"

	"github.com/overgent/overgent/internal/hosted"
)

type fakePublisher struct {
	ack hosted.BatchAck
	err error
}

func (f fakePublisher) PublishBatch(context.Context, []byte) (hosted.BatchAck, error) {
	return f.ack, f.err
}
func (f fakePublisher) Heartbeat(context.Context, string, string) error { return f.err }
func (f fakePublisher) CreateBrief(context.Context, string, string, string, int) (hosted.CoordinationBrief, error) {
	return hosted.CoordinationBrief{}, f.err
}
func (f fakePublisher) Collaboration(context.Context, string) (hosted.CollaborationSnapshot, error) {
	return hosted.CollaborationSnapshot{}, f.err
}
func TestHostedSenderRequiresEveryAcknowledgement(t *testing.T) {
	batch := []byte(`{"events":[{"eventId":"evt_a"},{"eventId":"evt_b"}]}`)
	if err := (hostedSender{client: fakePublisher{ack: hosted.BatchAck{AcceptedEventIDs: []string{"evt_a", "evt_b"}, Cursor: "seq:2"}}}).Send(context.Background(), "wsp_fixture", batch); err != nil {
		t.Fatal(err)
	}
	if err := (hostedSender{client: fakePublisher{ack: hosted.BatchAck{AcceptedEventIDs: []string{"evt_a"}, Cursor: "seq:1"}}}).Send(context.Background(), "wsp_fixture", batch); err == nil {
		t.Fatal("partial acknowledgement accepted")
	}
	if err := (hostedSender{client: fakePublisher{err: errors.New("offline")}}).Send(context.Background(), "wsp_fixture", batch); err == nil {
		t.Fatal("transport error ignored")
	}
}
