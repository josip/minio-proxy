# minio proxy

> **Warning**
> Do not use this in production. This is a demo project.

## Running the proxy

Proxy can be configured with following environment variables:

```
SERVER_ADDR=:4040
ENC_KEY=(xxx key used for encryption the data, must be 32b xxx)
HMAC_KEY=(xxx key for hmac signature, must be 32b xxx)
UPLOAD_CHUNK_SIZE_MB=5 or -1 to disable multipart uploads
MINIO_ENDPOINT=http://127.0.0.1:9000
MINIO_ACCESS_KEY=(xxx minio access key id xxx)
MINIO_SECRET_KEY=(xxx minio secret key xxx)
MINIO_BUCKET_NAME=bucket_to_upload_files_to
```

Those can be also read from a `.env` file placed in the working directory.

Finally start the proxy with:

```
$ go install github.com/josip/minio-proxy/cmd/minio-proxy
$ minio-proxy
```

## Generating ENC_KEY and HMAC_KEY

You can generate valid keys for encryption using `gensecrets` provided in the repo. Both keys are 32bit bit passwords derived with `scrypt` using [recommended parameters](https://pkg.go.dev/golang.org/x/crypto/scrypt#Key).

To use and install the tool:
```
$ go install github.com/josip/minio-proxy/cmd/gensecrets
$ gensecrets
salt: sss...
pass: ppp...
key:  kkk...copy this value...kkk
```

## Encryption

The proxy performs a transparent encrypt/decrypt operation before uploading/downloading files to/from Minio.

Files are encrypted by:

1. Generating a random IV and writing it in cleartext to the output and HMAC
2. Each 4kb block of the file is:
    1. Encrypted with AES-256 in CTR mode and written to the output.
    2. Encrypted data is used for HMAC signature.
3. HMAC signature is written to the output.

File format:

|      | Random IV | Encrypted content | HMAC sum |
|------|-----------|-------------------|----------|
| size | 16B       | variable          | 32B      |
