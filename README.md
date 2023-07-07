# opensearch-csv-exporter

## Overview

The OpenSearch CSV Export API allows you to export data from an OpenSearch index to CSV format. By making a POST request to the API endpoint, you can specify the date range, query, and columns to export. The exported data will be compressed and saved to a file.


## Configuration Options

The `opensearch-csv-exporter` utility provides various configuration options that can be used to customize its behavior. The following command-line flags can be used to set these options:

| Flag                          | Description                                           |
|-------------------------------|-------------------------------------------------------|
| -log-format                   | Change the log format. (default: text)                |
| -log-formatter                | Change the log formatter. (default: <nil>)            |
| -log-level                    | Change the log level.                                 |
| -opensearch-addresses         | Change the OpenSearch addresses. (default: [])        |
| -opensearch-cacertfilepath    | Change the OpenSearch CA certificate file path.       |
| -opensearch-indices           | Change the OpenSearch indices. (default: [])          |
| -port                         | Change the port. (default: 8080)                      |

These options can also be set using environment variables. The generated environment variables for each option are:

| Environment Variable           | Description                                     |
|--------------------------------|-------------------------------------------------|
| CONFIG_LOG_FORMAT              | Change the log format.                          |
| CONFIG_LOG_FORMATTER           | Change the log formatter.                       |
| CONFIG_LOG_LEVEL               | Change the log level.                           |
| CONFIG_OPENSEARCH_ADDRESSES    | Change the OpenSearch addresses.                |
| CONFIG_OPENSEARCH_CACERTFILEPATH | Change the OpenSearch CA certificate file path. |
| CONFIG_OPENSEARCH_INDICES       | Change the OpenSearch indices.                  |
| CONFIG_PORT                     | Change the port.                                |

## Endpoint

```
POST /api/opensearch/csv-export-v1
```

## Request Parameters

The request should include the following parameters:

| Parameter   | Type    | Description                            |
|-------------|---------|----------------------------------------|
| fromDate    | string  | The start date for the export (format: "YYYY-MM-DD"). |
| toDate      | string  | The end date for the export (format: "YYYY-MM-DD").   |
| query       | string  | The query to filter the documents.                     |
| columns     | array   | The list of columns to include in the CSV.              |

## Request Headers

The request should include the following header for authentication:

```
Authorization: Basic base64(username:password)
```

Replace `username` and `password` with your actual credentials, encoded in Base64.

## Response

The response will be a compressed CSV file containing the exported data. The file will be downloaded with the filename `test.csv.gz` in this example.

## Example

### cURL Command

```bash
curl -XPOST localhost:8080/api/opensearch/csv-export-v1 -u username:password -d '{"fromDate":"2023-06-13","toDate":"2023-06-14","query":"MY_QUERY","columns":["MY_CUSTOM_CSV_COLUMN"]}' -o test.csv.gz
```

### Example Request

```http
POST /api/opensearch/csv-export-v1 HTTP/1.1
Host: localhost:8080
Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ=
Content-Type: application/json
Content-Length: 101

{
  "fromDate": "2023-06-13",
  "toDate": "2023-06-14",
  "query": "MY_QUERY",
  "columns": ["MY_CUSTOM_CSV_COLUMN"]
}
```

### Example Response

The response will be a file named `test.csv.gz`, containing the exported data.

## Error Handling

If an error occurs during the export process, the API will return an appropriate HTTP status code along with an error message in the response body.

## Authentication

The API uses Basic Authentication for authentication purposes. The `Authorization` header should contain the Base64 encoded username and password.

Please note that it is highly recommended to use secure connections (e.g., HTTPS) when using this API in a production environment to protect sensitive information.