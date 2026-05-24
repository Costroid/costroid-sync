package providers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	awsTestAccessKey = "AKIATESTFAKE"
	awsTestSecret    = "test-secret-FAKE"
	awsTestSession   = "test-session-FAKE"
	awsTestAccount   = "123456789012"
)

type awsBedrockRecordedReq struct {
	Path            string
	Method          string
	Body            string
	Target          string
	Authorization   string
	SecurityToken   string
	ContentType     string
	ContentEncoding string
	AmzDate         string
}

type awsBedrockTestServer struct {
	server       *httptest.Server
	costHandler  http.HandlerFunc
	cloudHandler http.HandlerFunc
	captured     []awsBedrockRecordedReq
}

func newAWSBedrockTestServer(t *testing.T) *awsBedrockTestServer {
	t.Helper()
	ts := &awsBedrockTestServer{}
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ResultsByTime":[]}`))
	}
	ts.cloudHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Metrics":[]}`))
	}
	ts.server = httptest.NewServer(http.HandlerFunc(ts.dispatch))
	t.Cleanup(ts.server.Close)
	return ts
}

func (s *awsBedrockTestServer) dispatch(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.captured = append(s.captured, awsBedrockRecordedReq{
		Path:            r.URL.Path,
		Method:          r.Method,
		Body:            string(body),
		Target:          r.Header.Get("X-Amz-Target"),
		Authorization:   r.Header.Get("Authorization"),
		SecurityToken:   r.Header.Get("X-Amz-Security-Token"),
		ContentType:     r.Header.Get("Content-Type"),
		ContentEncoding: r.Header.Get("Content-Encoding"),
		AmzDate:         r.Header.Get("X-Amz-Date"),
	})
	if strings.Contains(r.Header.Get("X-Amz-Target"), "GetCostAndUsage") {
		s.costHandler(w, r)
		return
	}
	s.cloudHandler(w, r)
}

func newAWSBedrockTestProvider(ts *awsBedrockTestServer) *AWSBedrockProvider {
	p := NewAWSBedrockProvider(AWSBedrockConfig{
		AccessKeyID:        awsTestAccessKey,
		SecretAccessKey:    awsTestSecret,
		SessionToken:       awsTestSession,
		Region:             awsBedrockDefaultRegion,
		CostExplorerRegion: awsBedrockDefaultRegion,
		AccountID:          awsTestAccount,
		CostExplorerURL:    ts.server.URL,
		CloudWatchURL:      ts.server.URL,
	})
	p.HTTPClient = ts.server.Client()
	p.Now = func() time.Time { return time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC) }
	return p
}

func TestAWSBedrock_SigV4DeterministicNoSecretLeak(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	req1 := newSignedTestRequest(t, body)
	req2 := newSignedTestRequest(t, body)
	auth1 := req1.Header.Get("Authorization")
	auth2 := req2.Header.Get("Authorization")

	if auth1 == "" || auth1 != auth2 {
		t.Fatalf("authorization not deterministic: %q vs %q", auth1, auth2)
	}
	for _, bad := range []string{awsTestSecret, awsTestSession, "hello", "world"} {
		if strings.Contains(auth1, bad) {
			t.Fatalf("authorization leaked %q: %s", bad, auth1)
		}
	}
	for _, want := range []string{
		"AWS4-HMAC-SHA256",
		"Credential=" + awsTestAccessKey + "/20260524/us-east-1/ce/aws4_request",
		"SignedHeaders=content-type;host;x-amz-date;x-amz-security-token;x-amz-target",
		"Signature=",
	} {
		if !strings.Contains(auth1, want) {
			t.Errorf("authorization missing %q: %s", want, auth1)
		}
	}
}

func newSignedTestRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://ce.us-east-1.amazonaws.com/", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	setAWSJSONHeaders(req, awsCostExplorerTarget, "application/x-amz-json-1.1", "", awsBedrockUserAgentDev)
	signer := awsSigner{
		Region:  "us-east-1",
		Service: "ce",
		Now:     time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
		Creds:   awsCredentials{AccessKeyID: awsTestAccessKey, SecretAccessKey: awsTestSecret, SessionToken: awsTestSession},
	}
	if err := signer.sign(req, body); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return req
}
