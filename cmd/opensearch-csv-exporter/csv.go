package main

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"

	"github.com/tidwall/gjson"
)

type CSV struct {
	writer *csv.Writer

	gzip    *gzip.Writer
	columns []string
}

func NewCSV(columns []string, writer io.Writer) (*CSV, error) {

	g := gzip.NewWriter(writer)

	c := &CSV{
		gzip:    g,
		writer:  csv.NewWriter(g),
		columns: append([]string{"@timestamp", "message"}, columns...),
	}
	c.writer.Comma = ';'
	err := c.writer.Write(c.columns)
	if err != nil {
		return c, fmt.Errorf("failed to write header to csv writer: %w", err)
	}
	return c, nil
}

func (csv *CSV) Close() error {
	csv.writer.Flush()
	err := csv.writer.Error()
	if err != nil {
		return fmt.Errorf("failed to close csv writer: %w", err)
	}

	err = csv.gzip.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush gzip writer: %w", err)
	}
	err = csv.gzip.Close()
	if err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return nil
}

func (csv *CSV) write(doc []byte) error {
	var record []string
	for _, column := range csv.columns {
		data := gjson.GetBytes(doc, column).Value()
		if data == nil {
			data = ""
		}
		record = append(record, fmt.Sprintf("%v", data))
	}
	err := csv.writer.Write(record)
	if err != nil {
		return fmt.Errorf("failed to write record to csv: %s", err)
	}
	return nil
}
