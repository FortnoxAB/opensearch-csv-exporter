package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/jonaz/ginlogrus"
	"github.com/sirupsen/logrus"
	"github.com/xonvanetta/shutdown/pkg/shutdown"

	"github.com/gin-gonic/gin"

	"github.com/fortnoxab/fnxlogrus"
	"github.com/fortnoxab/ginprometheus"
	"github.com/koding/multiconfig"
)

type Config struct {
	Log  fnxlogrus.Config
	Port int `default:"8080"`

	Opensearch OpensearchConfig
}

func (c *Config) Valid() error {
	if len(c.Opensearch.Addresses) == 0 {
		return fmt.Errorf("missing opensearch addresses")
	}

	return nil
}

type OpensearchConfig struct {
	Addresses []string
	Indices   []string

	CACertFilePath string
	ca             []byte
}

func (c *OpensearchConfig) Config(header http.Header) elasticsearch.Config {
	esConfig := elasticsearch.Config{
		Addresses: c.Addresses,
		Header:    header,
		CACert:    c.ca,
	}

	if c.CACertFilePath != "" && len(c.ca) == 0 {
		var err error
		c.ca, err = os.ReadFile(c.CACertFilePath)
		if err != nil {
			panic(fmt.Errorf("failed to load ca cert: %w", err))
		}
		esConfig.CACert = c.ca
	}

	return esConfig
}

func main() {
	config := &Config{}
	multiconfig.MustLoad(config)

	err := config.Valid()
	if err != nil {
		logrus.Fatalf("failed to validate config: %s", err)
	}

	ctx := shutdown.Context()

	fnxlogrus.Init(config.Log, logrus.StandardLogger())

	router := gin.New()
	ginprometheus.New("http").Use(router)
	router.Use(ginlogrus.New(logrus.StandardLogger(), "/health", "/metrics"), gin.Recovery())
	router.GET("/health")

	router.POST("/api/opensearch/csv-export-v1", export(config))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logrus.Error(err)
		}
	}()

	<-ctx.Done()

	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		logrus.Debug("sleeping 5 sec before shutdown") // to give k8s ingresses time to sync
		time.Sleep(5 * time.Second)
	}
	ctxShutDown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctxShutDown); !errors.Is(err, http.ErrServerClosed) && err != nil {
		logrus.Error(err)
	}
}
