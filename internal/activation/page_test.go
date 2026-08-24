package activation

import (
	"bytes"
	"strings"
	"testing"
)

func TestActivationPagePostsTicketOutsideURLAndEscapesValues(t *testing.T) {
	var page bytes.Buffer
	if err := writePage(&page, "https://api.stickguy.dev/v1/dashboard-activations", `synthetic-ticket-<unsafe>-value`); err != nil {
		t.Fatal(err)
	}
	html := page.String()
	if !strings.Contains(html, `method="post"`) || !strings.Contains(html, `type="hidden"`) || strings.Contains(html, `<unsafe>`) {
		t.Fatalf("unsafe activation page: %s", html)
	}
	if strings.Contains(html, `dashboard-activations?`) {
		t.Fatal("ticket moved into activation URL")
	}
}
