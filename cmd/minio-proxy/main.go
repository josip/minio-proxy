package main

import (
	"encoding/hex"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/josip/minioproxy"
)

func main() {
	godotenv.Load()

	cfg := minioproxy.Config{
		ServerAddr: os.Getenv("SERVER_ADDR"),
		Endpoint:   os.Getenv("MINIO_ENDPOINT"),
		AccessKey:  os.Getenv("MINIO_ACCESS_KEY"),
		SecretKey:  os.Getenv("MINIO_SECRET_KEY"),
		BucketName: os.Getenv("MINIO_BUCKET_NAME"),
	}

	chunkSizeStr := os.Getenv("UPLOAD_CHUNK_SIZE_MB")
	chunkSize, err := strconv.ParseInt(chunkSizeStr, 10, 0)
	if err != nil {
		chunkSize = -1
	}
	cfg.UploadChunkSizeMb = int(chunkSize)

	encKey, _ := hex.DecodeString(os.Getenv("ENC_KEY"))
	cfg.EncKey = encKey
	hmacKey, _ := hex.DecodeString(os.Getenv("HMAC_KEY"))
	cfg.HmacKey = hmacKey

	app, err := minioproxy.New(cfg)
	if err != nil {
		panic(err)
	}

	if err := app.ListenAndServe(); err != nil {
		panic(err)
	}
}
