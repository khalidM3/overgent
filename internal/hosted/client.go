package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	protocoltypes "github.com/stickguy/stickguy/protocol/generated/go"
)

const responseLimit = 1 << 20

type Client struct {
	base  *url.URL
	http  *http.Client
	token string
}

type Project struct{ ID, Label string }
type Invite struct {
	ID, Secret string
	ExpiresAt  time.Time
}
type Enrollment struct{ DeviceID, DeviceToken, DashboardTicket string }
type Bootstrap struct {
	DeviceID                     string
	SchemaMinimum, SchemaMaximum int
	Projects                     []Project
	Cursors                      map[string]string
}
type DashboardTicket struct {
	Ticket    string
	ExpiresAt time.Time
}
type BatchAck struct {
	AcceptedEventIDs []string
	Cursor           string
}
type ChangePage struct {
	Items  []map[string]any
	Cursor string
}
type APIError struct {
	Status    int
	Code      string
	RequestID string
	Retryable bool
}

func (e *APIError) Error() string { return fmt.Sprintf("hosted API %s (%d)", e.Code, e.Status) }

func New(rawBase, token string) (*Client, error) {
	base, err := url.Parse(strings.TrimRight(rawBase, "/"))
	if err != nil || base.Host == "" || base.Path != "" {
		return nil, errors.New("hosted API base must be an origin without a path")
	}
	host := base.Hostname()
	loopback := host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	if base.Scheme != "https" && !(base.Scheme == "http" && loopback) {
		return nil, errors.New("hosted API requires HTTPS except for loopback validation")
	}
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("hosted API origin contains unsupported userinfo, query, or fragment")
	}
	return &Client{base: base, token: token, http: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (c *Client) CreateProject(ctx context.Context, label, deviceLabel string) (Project, error) {
	var out struct{ ID, Label string }
	err := c.request(ctx, http.MethodPost, "/v1/projects", protocoltypes.CreateProjectJSONBody{Label: label, DeviceLabel: deviceLabel}, &out, http.StatusCreated)
	return Project(out), err
}

func (c *Client) CreateInvite(ctx context.Context, projectID string, expiresInSeconds, maxUses int) (Invite, error) {
	var out struct {
		ID, Secret string
		ExpiresAt  time.Time
	}
	err := c.request(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(projectID)+"/invites", protocoltypes.CreateInviteJSONBody{ExpiresInSeconds: expiresInSeconds, MaxUses: maxUses}, &out, http.StatusCreated)
	return Invite(out), err
}

func (c *Client) Enroll(ctx context.Context, inviteID, inviteSecret, deviceLabel, appVersion string) (Enrollment, error) {
	var out struct{ DeviceId, DeviceToken, DashboardTicket string }
	body := protocoltypes.CreateEnrollmentJSONBody{InviteId: inviteID, InviteSecret: inviteSecret, DeviceLabel: deviceLabel, AppVersion: appVersion, SchemaMinimum: 1, SchemaMaximum: 1}
	err := c.request(ctx, http.MethodPost, "/v1/enrollments", body, &out, http.StatusCreated)
	return Enrollment{DeviceID: out.DeviceId, DeviceToken: out.DeviceToken, DashboardTicket: out.DashboardTicket}, err
}

func (c *Client) Bootstrap(ctx context.Context) (Bootstrap, error) {
	var out struct {
		DeviceId                     string
		SchemaMinimum, SchemaMaximum int
		Projects                     []struct{ ID, Label string }
		Cursors                      map[string]string
	}
	err := c.request(ctx, http.MethodGet, "/v1/device/bootstrap", nil, &out, http.StatusOK)
	projects := make([]Project, len(out.Projects))
	for i, project := range out.Projects {
		projects[i] = Project(project)
	}
	return Bootstrap{DeviceID: out.DeviceId, SchemaMinimum: out.SchemaMinimum, SchemaMaximum: out.SchemaMaximum, Projects: projects, Cursors: out.Cursors}, err
}

func (c *Client) CreateDashboardTicket(ctx context.Context, projectID string) (DashboardTicket, error) {
	var out struct {
		Ticket    string
		ExpiresAt time.Time
	}
	err := c.request(ctx, http.MethodPost, "/v1/dashboard-tickets", protocoltypes.CreateDashboardTicketJSONBody{ProjectId: projectID}, &out, http.StatusCreated)
	return DashboardTicket(out), err
}

func (c *Client) PublishBatch(ctx context.Context, batch []byte) (BatchAck, error) {
	var out struct {
		AcceptedEventIds []string
		Cursor           string
	}
	err := c.requestBytes(ctx, http.MethodPost, "/v1/events/batch", batch, &out, http.StatusOK)
	return BatchAck{AcceptedEventIDs: out.AcceptedEventIds, Cursor: out.Cursor}, err
}

func (c *Client) Heartbeat(ctx context.Context, workspaceID, state string) error {
	body := protocoltypes.HeartbeatJSONBody{WorkspaceId: workspaceID, State: protocoltypes.HeartbeatJSONBodyState(state)}
	return c.request(ctx, http.MethodPost, "/v1/presence/heartbeat", body, nil, http.StatusNoContent)
}

func (c *Client) RevokeDevice(ctx context.Context, deviceID string) error {
	return c.request(ctx, http.MethodPost, "/v1/devices/"+url.PathEscape(deviceID)+"/revoke", nil, nil, http.StatusNoContent)
}

func (c *Client) ProjectChanges(ctx context.Context, projectID string) (ChangePage, error) {
	var out struct {
		Items  []map[string]any
		Cursor string
	}
	err := c.request(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(projectID)+"/changes", nil, &out, http.StatusOK)
	return ChangePage(out), err
}

func (c *Client) request(ctx context.Context, method, path string, body, out any, want int) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode hosted request: %w", err)
		}
	}
	return c.requestBytes(ctx, method, path, encoded, out, want)
}

func (c *Client) requestBytes(ctx context.Context, method, path string, body []byte, out any, want int) error {
	endpoint := *c.base
	endpoint.Path = path
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create hosted request: %w", err)
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call hosted API: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, responseLimit+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read hosted response: %w", err)
	}
	if len(payload) > responseLimit {
		return errors.New("hosted response exceeds limit")
	}
	if response.StatusCode != want {
		var envelope struct {
			Error struct {
				Code, RequestId string
				Retryable       bool
			}
		}
		_ = json.Unmarshal(payload, &envelope)
		code := envelope.Error.Code
		if code == "" {
			code = "unexpected_status"
		}
		return &APIError{Status: response.StatusCode, Code: code, RequestID: envelope.Error.RequestId, Retryable: envelope.Error.Retryable}
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode hosted response: %w", err)
	}
	return nil
}
