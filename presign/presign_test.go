package presign

import (
	"fmt"
	"net/url"
	"testing"
	"time"
)

// https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html#query-string-auth-v4-signing-example
func TestPresign(t *testing.T) {
	//20060102T150405Z
	// Specified in AWS as Fri, 24 May 2013 00:00:00 GMT
	mockTime, err := time.ParseInLocation(timeFormatISO8061, "20130524T000000Z", time.UTC)
	fmt.Println(mockTime)
	// mockTime, err := time.Parse("", "Fri, 24 May 2013 00:00:00 GMT")
	if err != nil {
		t.Fatal("can't parse mock timestamp", err)
	}

	s := Signer{
		Endpoint:        "https://s3.amazonaws.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		AccessKeySecret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:          "us-east-1",
		t:               mockTime,
	}

	presignedUrlStr := s.Presign("GET", "examplebucket", "test.txt", "24h", nil)
	fmt.Println("Full url", presignedUrlStr)
	presignedUrl, err := url.Parse(presignedUrlStr)
	if err != nil {
		t.Error("presigned url is not a valid url", err)
	}

	computed := presignedUrl.Query().Get("X-Amz-Signature")
	expected := "aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
	if computed != expected {
		t.Error("signatures do not match, got", computed, "expected", expected)
	}
}
