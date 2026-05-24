package providers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	awsSigV4Algorithm  = "AWS4-HMAC-SHA256"
	awsSigV4Request    = "aws4_request"
	awsLongDateLayout  = "20060102T150405Z"
	awsShortDateLayout = "20060102"
)

type awsCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type awsSigner struct {
	Region  string
	Service string
	Now     time.Time
	Creds   awsCredentials
}

func (s awsSigner) sign(req *http.Request, body []byte) error {
	if s.Creds.AccessKeyID == "" || s.Creds.SecretAccessKey == "" {
		return fmt.Errorf("aws-bedrock signing: missing AWS credentials")
	}
	now := s.Now.UTC()
	req.Header.Set("X-Amz-Date", now.Format(awsLongDateLayout))
	if s.Creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", s.Creds.SessionToken)
	}

	canonical, signedHeaders := canonicalAWSRequest(req, body)
	scope := s.credentialScope(now)
	stringToSign := strings.Join([]string{
		awsSigV4Algorithm,
		now.Format(awsLongDateLayout),
		scope,
		hexSHA256([]byte(canonical)),
	}, "\n")
	signature := hmacHex(awsSigningKey(s.Creds.SecretAccessKey, now, s.Region, s.Service), stringToSign)
	req.Header.Set("Authorization", awsAuthorization(s.Creds.AccessKeyID, scope, signedHeaders, signature))
	return nil
}

func canonicalAWSRequest(req *http.Request, body []byte) (string, string) {
	headers := awsCanonicalHeaders(req)
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(headers[name])
		b.WriteByte('\n')
	}
	signedHeaders := strings.Join(names, ";")
	return strings.Join([]string{
		req.Method,
		awsCanonicalURI(req.URL.EscapedPath()),
		req.URL.RawQuery,
		b.String(),
		signedHeaders,
		hexSHA256(body),
	}, "\n"), signedHeaders
}

func awsCanonicalHeaders(req *http.Request) map[string]string {
	headers := map[string]string{"host": req.URL.Host}
	for _, name := range []string{
		"Content-Encoding", "Content-Type", "X-Amz-Date",
		"X-Amz-Security-Token", "X-Amz-Target",
	} {
		if value := req.Header.Get(name); value != "" {
			headers[strings.ToLower(name)] = normalizeAWSHeaderValue(value)
		}
	}
	return headers
}

func normalizeAWSHeaderValue(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func awsCanonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func (s awsSigner) credentialScope(now time.Time) string {
	return strings.Join([]string{
		now.Format(awsShortDateLayout),
		s.Region,
		s.Service,
		awsSigV4Request,
	}, "/")
}

func awsSigningKey(secret string, now time.Time, region, service string) []byte {
	kDate := hmacBytes([]byte("AWS4"+secret), now.Format(awsShortDateLayout))
	kRegion := hmacBytes(kDate, region)
	kService := hmacBytes(kRegion, service)
	return hmacBytes(kService, awsSigV4Request)
}

func awsAuthorization(accessKey, scope, signedHeaders, signature string) string {
	return fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		awsSigV4Algorithm, accessKey, scope, signedHeaders, signature)
}

func hmacBytes(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func hmacHex(key []byte, data string) string {
	return hex.EncodeToString(hmacBytes(key, data))
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
