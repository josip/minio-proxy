package minioproxy

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

type App struct {
	ctx    context.Context
	router *mux.Router
	client *minioClient

	addr       string
	chunkSize  int64
	bucketName string

	encKey  []byte
	hmacKey []byte
}

func New(cfg Config) (*App, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	app := &App{
		ctx:       context.Background(),
		router:    mux.NewRouter(),
		addr:      cfg.ServerAddr,
		chunkSize: cfg.uploadChunkSizeInBytes(),
		encKey:    cfg.EncKey,
		hmacKey:   cfg.HmacKey,
		client:    newMinioClient(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey),
	}
	app.bucketName = cfg.BucketName

	bindUploadApi(app)
	bindReadApi(app)

	return app, nil
}

func (app *App) ListenAndServe() error {
	log.Println("file server started at", app.addr)

	app.router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		m, _ := route.GetMethods()
		p, _ := route.GetPathTemplate()
		log.Println("-", m, p)
		return nil
	})

	return http.ListenAndServe(app.addr, app.router)
}
