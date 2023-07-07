package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7/esapi"

	"github.com/sirupsen/logrus"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/gin-gonic/gin"
)

type Request struct {
	ToDate, FromDate string
	Query            string
	Columns          []string
}

func (r *Request) Valid() error {
	if r.Query == "" {
		return fmt.Errorf("missing query")
	}

	if r.FromDate == "" {
		return fmt.Errorf("missing fromdate")
	}

	if r.ToDate == "" {
		return fmt.Errorf("missing todate")
	}

	return nil
}

// Todo add session/cookie based auth
func extractBasicAuth(g *gin.Context) (http.Header, error) {
	authorization := g.GetHeader("authorization")
	if !strings.HasPrefix(authorization, "Basic ") {
		return nil, fmt.Errorf("wrong format on Authorization header")
	}

	header := http.Header{}
	header.Set("authorization", authorization)

	return header, nil
}

func export(config *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth, err := extractBasicAuth(c)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		client, err := elasticsearch.NewClient(config.Opensearch.Config(auth))
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		request := &Request{}
		err = c.BindJSON(request)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		err = request.Valid()
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		csv, err := NewCSV(request.Columns, c.Writer)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		c.Header("transfer-encoding", "chunked")

		defer func() {
			c.Header("content-type", "application/csv")
			c.Header("content-encoding", "gzip")

			err = csv.Close()
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
		}()

		body, err := createSearch(request)
		if err != nil {
			c.String(http.StatusBadGateway, err.Error())
			return
		}

		scrollID, totalHits, err := search(c.Request.Context(), config.Opensearch, client, csv, body)
		if err != nil {
			logrus.Error(err)
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		c.Header("total-hits", strconv.Itoa(totalHits))

		for scrollID != "" {
			body, err := createScroll(scrollID)
			if err != nil {
				c.String(http.StatusBadGateway, err.Error())
				return
			}
			scrollID, err = scroll(c.Request.Context(), client, csv, body)
			if err != nil {
				logrus.Error(err)
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

}

func createSearch(request *Request) (io.Reader, error) {
	v := struct {
		Size  int `json:"size"`
		Query struct {
			Bool struct {
				Must struct {
					QueryString struct {
						Query string `json:"query"`
					} `json:"query_string"`
				} `json:"must"`
				Filter struct {
					Range struct {
						Timestamp struct {
							Gte string `json:"gte"`
							Lte string `json:"lte"`
						} `json:"@timestamp"`
					} `json:"range"`
				} `json:"filter"`
			} `json:"bool"`
		} `json:"query"`
	}{}
	v.Size = 10000 //Needed since default size is 10
	v.Query.Bool.Must.QueryString.Query = request.Query
	v.Query.Bool.Filter.Range.Timestamp.Gte = request.FromDate
	v.Query.Bool.Filter.Range.Timestamp.Lte = request.ToDate

	b := &bytes.Buffer{}
	err := json.NewEncoder(b).Encode(v)
	if err != nil {
		return nil, fmt.Errorf("failed to encode: %w", err)
	}

	return b, nil
}

func createScroll(id string) (io.Reader, error) {
	v := struct {
		ScrollID string `json:"scroll_id"`
	}{
		ScrollID: id,
	}

	b := &bytes.Buffer{}
	err := json.NewEncoder(b).Encode(v)
	if err != nil {
		return nil, fmt.Errorf("failed to encode: %w", err)
	}
	return b, nil
}

func search(ctx context.Context, config OpensearchConfig, client *elasticsearch.Client, csv *CSV, body io.Reader) (string, int, error) {
	s := client.Search
	response, err := s(s.WithContext(ctx), s.WithIndex(config.Indices...), s.WithBody(body), s.WithScroll(time.Minute))
	if err != nil {
		return "", 0, fmt.Errorf("failed to search: %w", err)
	}
	defer response.Body.Close()
	return decode(response, csv)
}

func scroll(ctx context.Context, client *elasticsearch.Client, csv *CSV, body io.Reader) (string, error) {
	s := client.Scroll
	response, err := s(s.WithContext(ctx), s.WithBody(body), s.WithScroll(time.Minute))
	if err != nil {
		return "", fmt.Errorf("failed to scroll: %w", err)
	}
	defer response.Body.Close()
	scrollId, _, err := decode(response, csv)
	return scrollId, err
}

func decode(response *esapi.Response, csv *CSV) (string, int, error) {
	defer response.Body.Close()
	if response.IsError() {
		return "", 0, fmt.Errorf("failed to decode, response is error: %s", response)
	}

	v := &struct {
		ScrollID string `json:"_scroll_id"`
		Hits     struct {
			Total struct {
				Value int
			}
			Hits []struct {
				Source json.RawMessage `json:"_source"`
			}
		}
	}{}

	err := json.NewDecoder(response.Body).Decode(v)
	if err != nil {
		return "", 0, fmt.Errorf("failed to decode search response: %w", err)
	}

	for _, hit := range v.Hits.Hits {
		err := csv.write(hit.Source)
		if err != nil {
			return "", v.Hits.Total.Value, fmt.Errorf("failed to write csv: %w", err)
		}
	}

	if len(v.Hits.Hits) != 10000 {
		return "", v.Hits.Total.Value, nil
	}

	return v.ScrollID, v.Hits.Total.Value, nil
}
