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

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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

		defer func() {
			c.Header("content-type", "application/csv")
			c.Header("content-encoding", "gzip")

			err = csv.Close()
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
		}()

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
	v.Size = size //Needed since default size is 10
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
	if response.IsError() {
		return "", 0, fmt.Errorf("failed to decode, response is error: %s", response)
	}
	return decode(response.Body, csv)
}

var size = 10000

func scroll(ctx context.Context, client *elasticsearch.Client, csv *CSV, body io.Reader) (string, error) {
	s := client.Scroll
	response, err := s(s.WithContext(ctx), s.WithBody(body), s.WithScroll(time.Minute))
	if err != nil {
		return "", fmt.Errorf("failed to scroll: %w", err)
	}
	defer response.Body.Close()
	if response.IsError() {
		return "", fmt.Errorf("failed to decode, response is error: %s", response)
	}
	scrollId, _, err := decode(response.Body, csv)
	return scrollId, err
}

func decode(body io.ReadCloser, csv *CSV) (string, int, error) {
	d := json.NewDecoder(body)
	_, err := d.Token()
	if err != nil {
		return "", 0, err
	}
	var totalCount int
	var hintsCount int
	var scrollId string

	for d.More() {
		s, err := d.Token()
		if err != nil {
			return "", 0, err
		}

		if s == "_scroll_id" {
			a, err := d.Token()
			if err != nil {
				return "", 0, err
			}
			if id, ok := a.(string); ok {
				scrollId = id
			}

			_, err = d.Token()
			if err != nil {
				return "", 0, err
			}
		}

		if s != "hits" {
			if err = skip(d); err != nil {
				return "", 0, err
			}
			continue
		}

		_, err = d.Token()
		if err != nil {
			return "", 0, err
		}
		for d.More() {
			a, _ := d.Token()

			switch a {

			case "hits":
				_, err := d.Token()
				if err != nil {
					return "", 0, err
				}

				for d.More() {
					hintsCount++
					v := &struct {
						Source json.RawMessage `json:"_source"`
					}{}
					err = d.Decode(&v)
					if err != nil {
						return "", 0, err
					}
					err = csv.write(v.Source)
					if err != nil {
						return "", totalCount, fmt.Errorf("failed to write csv: %w", err)
					}
				}
				_, err = d.Token()
				if err != nil {
					return "", 0, err
				}
				continue
			case "total":
				_, err := d.Token()
				if err != nil {
					return "", 0, err
				}
				for d.More() {
					a, err = d.Token()
					if err != nil {
						return "", 0, err
					}
					if a != "value" {
						continue
					}
					err = d.Decode(&totalCount)
					if err != nil {
						return "", 0, err
					}
				}
				_, err = d.Token()
				if err != nil {
					return "", 0, err
				}
				continue
			default:
				if err := skip(d); err != nil {
					return "", 0, err
				}
				continue

			}
		}
	}

	if hintsCount != size { // returned documents fewer than requested page size means we are on the last page.
		return "", totalCount, nil
	}

	return scrollId, totalCount, nil
}
func skip(d *json.Decoder) error {
	n := 0
	for {
		t, err := d.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('['), json.Delim('{'):
			n++
		case json.Delim(']'), json.Delim('}'):
			n--
		}
		if n == 0 {
			return nil
		}
	}
}
