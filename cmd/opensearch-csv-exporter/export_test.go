package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToken(t *testing.T) {

	f, err := os.Open("test.json")
	assert.NoError(t, err)

	r, w := io.Pipe()

	buf := &bytes.Buffer{}
	var ch = make(chan struct{})
	go func() {
		zr, err := gzip.NewReader(r)
		assert.NoError(t, err)
		if _, err := io.Copy(buf, zr); err != nil {
			t.Error(err)
		}
		close(ch)
	}()

	csv, err := NewCSV([]string{"type", "container.image.name"}, w)
	assert.NoError(t, err)

	size = 3
	scrollId, cnt, err := decode(f, csv)
	assert.NoError(t, err)

	assert.Equal(t, "cool scroll id", scrollId)
	assert.Equal(t, 3, cnt)

	// test again with size set to 10000 and expect empty scrollId since we only get 3 rows.
	f, err = os.Open("test.json")
	assert.NoError(t, err)
	size = 10000
	scrollId, cnt, err = decode(f, csv)
	assert.NoError(t, err)

	assert.Equal(t, "", scrollId)
	assert.Equal(t, 3, cnt)
	err = csv.Close()
	assert.NoError(t, err)
	w.Close()

	<-ch
	assert.Contains(t, buf.String(), "2024-04-03T06:11:55.105Z;cool log number 1;cool type;cool image name 1")
	assert.Contains(t, buf.String(), "2024-04-03T06:11:55.205Z;cool log number 2;cool type;cool image name 2")
	assert.Contains(t, buf.String(), "2024-04-03T06:11:55.305Z;cool log number 3;cool type;cool image name 3")
}
