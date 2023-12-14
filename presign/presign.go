// Implements logic presign URLS for AWS S3/minio as documented at
// https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html
package presign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const timeFormatISO8061 = "20060102T150405Z"
const timeFormatYMD = "20060102"
const defaultRegion = "us-east-1"
const serviceName = "s3"
const serviceRequestType = "aws4_request"

type Signer struct {
	AccessKeyID     string
	AccessKeySecret string
	Region          string
	Endpoint        string

	// for tests
	t time.Time
}

func (s *Signer) Presign(method, bucket, filename, duration string, extraQueryOptions url.Values) string {
	region := s.Region
	if len(region) == 0 {
		region = defaultRegion
	}

	qp := url.Values{}

	now := time.Now().UTC()
	if !s.t.IsZero() {
		now = s.t
	}

	endpoint := s.Endpoint
	u, _ := url.Parse(endpoint)
	// for real S3/minio instances buckets are part of the host url, not path
	if !isIp(u.Hostname()) {
		endpoint = u.Scheme + "://" + bucket + "." + u.Host
		u, _ = u.Parse(endpoint)
		bucket = ""
	}

	qp.Add("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	qp.Add("X-Amz-Date", now.Format(timeFormatISO8061))
	t, _ := time.ParseDuration(duration)
	qp.Add("X-Amz-Expires", strconv.FormatInt(int64(math.Round(t.Abs().Seconds())), 10))
	qp.Add("X-Amz-SignedHeaders", "host")
	scope := strings.Join([]string{
		now.Format(timeFormatYMD),
		region,
		serviceName,
		serviceRequestType,
	}, "/")
	qp.Add("X-Amz-Credential", s.AccessKeyID+"/"+scope)

	for k, v := range extraQueryOptions {
		qp.Add(k, v[0])
	}

	var canonReq strings.Builder
	canonReq.WriteString(method + "\n")
	//
	if len(bucket) != 0 {
		canonReq.WriteString("/" + bucket)
	}
	canonReq.WriteString("/" + filename + "\n")
	canonReq.WriteString(qp.Encode() + "\n")
	canonReq.WriteString("host:" + u.Host + "\n\n") // canon headers
	canonReq.WriteString("host\n")                  // signed headers
	canonReq.WriteString("UNSIGNED-PAYLOAD")

	var toSign strings.Builder
	toSign.WriteString("AWS4-HMAC-SHA256\n")
	toSign.WriteString(now.Format(timeFormatISO8061))
	toSign.WriteRune('\n')
	toSign.WriteString(scope)
	toSign.WriteRune('\n')
	toSign.WriteString(sha256Hash(canonReq.String()))

	dateKey := hmacSha256([]byte("AWS4"+s.AccessKeySecret), []byte(now.Format(timeFormatYMD)))
	dateRegionKey := hmacSha256(dateKey, []byte(region))
	dateRegionServiceKey := hmacSha256(dateRegionKey, []byte("s3"))
	signingKey := hmacSha256(dateRegionServiceKey, []byte("aws4_request"))

	// tada
	signature := hex.EncodeToString(hmacSha256(signingKey, []byte(toSign.String())))

	urlWithPath := strings.Join([]string{
		endpoint,
		bucket,
		filename,
	}, "/")

	// sprintf to ensure signature is last
	return fmt.Sprintf("%s?%s&X-Amz-Signature=%s", urlWithPath, qp.Encode(), signature)
}

func sha256Hash(data string) string {
	h := sha256.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func hmacSha256(key []byte, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func isIp(str string) bool {
	return net.ParseIP(str) != nil
}
