FROM gcr.io/distroless/static-debian11:nonroot
COPY opensearch-csv-exporter /
USER nonroot
ENTRYPOINT ["/opensearch-csv-exporter"]
