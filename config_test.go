package minioproxy

import "testing"

func TestConfigValidation(t *testing.T) {
	cfg := &Config{}
	if err := cfg.validate(); err == nil {
		t.Error("expected config validation to fail")
	}

	cfg = &Config{
		Endpoint:   "http://localhost:1234",
		AccessKey:  "abcd",
		SecretKey:  "defg",
		ServerAddr: ":1234",
		BucketName: "test",
		EncKey:     genRandBytes(32),
		HmacKey:    genRandBytes(32),
	}
	if err := cfg.validate(); err != nil {
		t.Error("expected config to be valid, instead got:", err)
	}
}

func TestConfigInvalidEndpoint(t *testing.T) {
	cfg := &Config{
		Endpoint: "not-a-url/hello.aspx",
	}
	if err := cfg.validate(); err == nil {
		t.Error("expected config validation to fail: invalid endpoint")
	}
}

func TestConfigInvalidChunkSize(t *testing.T) {
	cfg := &Config{
		UploadChunkSizeMb: MIN_CHUNK_SIZE_MB - 1,
	}
	if err := cfg.validate(); err == nil {
		t.Error("expected config validation to fail: too small chunk size")
	}
	cfg.UploadChunkSizeMb = maxChunkedFileSizeMB + 1024
	if err := cfg.validate(); err == nil {
		t.Error("expected config validation to fail: too large chunk size")
	}
}
