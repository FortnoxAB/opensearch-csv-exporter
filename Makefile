.PHONY:	build push run

IMAGE = quay.io/fortnox/opensearch-csv-exporter
# supply when running make: make all VERSION=1.0.0
#VERSION = 0.0.1 

build:
	CGO_ENABLED=0 GOOS=linux go build ./cmd/opensearch-csv-exporter/

docker: build
	docker build --pull --rm -t $(IMAGE):$(VERSION) .
	rm opensearch-csv-exporter

push:
	docker push $(IMAGE):$(VERSION)

all: build docker push

run:
	docker run -i --env-file=.env --rm -p 8080:8080 -t $(IMAGE):$(VERSION)

test: imports
	go test ./...

cover:
	@echo Running coverage
	go install github.com/wadey/gocovmerge
	$(eval PKGS := $(shell go list ./... | grep -v /vendor/))
	$(eval PKGS_DELIM := $(shell echo $(PKGS) | sed -e 's/ /,/g'))
	go list -f '{{if or (len .TestGoFiles) (len .XTestGoFiles)}}go test -test.v -test.timeout=120s -covermode=count -coverprofile={{.Name}}_{{len .Imports}}_{{len .Deps}}.coverprofile -coverpkg $(PKGS_DELIM) {{.ImportPath}}{{end}}' $(PKGS) | xargs -I {} bash -c {}
	gocovmerge `ls *.coverprofile` > cover.out
	rm *.coverprofile

cover-html: cover
	go tool cover -html cover.out
cover-test: cover
	go install github.com/jonaz/gototcov
	gototcov -f cover.out -limit 80 -ignore-zero

localrun:
	bash -c "env `grep -Ev '^#' .env | xargs` go run cmd/opensearch-csv-exporter/*.go"

# To format your files according to goimports you can run: `goimports -w .`
# or setup your ide to do it for you
imports: SHELL:=/bin/bash
imports:
	go install golang.org/x/tools/cmd/goimports@latest
	ASD=$$(goimports -l . 2>&1); test -z "$$ASD" || (echo "Code is not formatted correctly according to goimports!  $$ASD" && exit 1)
