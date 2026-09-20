package dockerclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mintoolkit/mint/pkg/app/master/config"
)

// TestNewClientVersionsRequestsWithoutExplicitAPIVersion covers #95.
//
// When the caller leaves DOCKER_API_VERSION / config.APIVersion empty - the
// default for a plain "mint build" - go-dockerclient never parses an API
// version into the client's requestedAPIVersion, and getURL() consults that
// field alone when it builds a request path. Every request the client sends
// therefore omits the "/vX.Y/" segment. A daemon reached through a proxy,
// which is what dind gives a CI pipeline, reads an unversioned request as
// coming from the oldest client it supports and rejects it with
// "client version ... is too old".
//
// The client's own /version negotiation does not help: its result is stored
// in expectedAPIVersion and never copied into requestedAPIVersion. So the
// request worth asserting on is the next one - here Info()'s GET /info -
// which is the one a proxying daemon actually rejects.
func TestNewClientVersionsRequestsWithoutExplicitAPIVersion(t *testing.T) {
	var infoPath string
	sawInfoRequest := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.HasSuffix(r.URL.Path, "/version") {
			// Answer the version probe truthfully so the client proceeds.
			_, _ = w.Write([]byte(`{"ApiVersion":"1.44"}`))
			return
		}
		infoPath = r.URL.Path
		sawInfoRequest = true
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := &config.DockerClient{
		Host:   srv.URL,
		UseTLS: false,
		// Left empty on purpose: this is the path every report hit.
		APIVersion: "",
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with an empty APIVersion must succeed: %v", err)
	}

	if _, err := client.Info(); err != nil {
		t.Fatalf("Info() must succeed against a daemon that answers /version: %v", err)
	}

	if !sawInfoRequest {
		t.Fatal("expected the fake daemon to receive the /info request, got none")
	}

	if !strings.Contains(infoPath, "/v") {
		t.Fatalf("the /info request path %q carries no API version segment although the daemon "+
			"answered the version probe; a daemon behind a proxy rejects such a request as coming "+
			"from a client that is too old (#95)", infoPath)
	}
}
