# Open Port Exporter

Open Port Exporter is a tool designed to scan specified hosts and ports to determine if they are open. The results are exported as Prometheus metrics, allowing for easy monitoring and alerting based on the port status.

## Features

- Scans specified hosts and ports for open connections.
- Caches results to optimize performance and reduce scanning frequency.
- Exposes the results as Prometheus metrics.
- Configurable through command-line flags.
- Handles a large number of concurrent connections with a configurable limit.

## Installation

You can install Open Port Exporter using the go install command:

```shell
go install github.com/FindMyKids/open-port-exporter@latest
```

## Usage

Run the Open Port Exporter with the following command:

```shell
open-port-exporter [flags]
```

### Flags

- `-web.listen-address`: The address to listen on for HTTP requests. Default is `:9116`.
- `-hosts`: Comma-separated list of hosts to scan ports for. Example: `localhost,example.com`.
- `-ports`: Comma-separated list of ports or port ranges to scan. Example: `80,443,1000-2000`.
- `-list`: Path to a file containing a list of hosts to scan ports for.
- `-max-connections`: Maximum number of concurrent connections. Default is `100`.
- `-timeout`: Timeout for each connection attempt. Default is `10s`.
- `-cache-expires`: Cache expiration time for closed ports. Default is `72h`.
- `-open-port-cache-expires`: Cache expiration time for open ports. Default is `15m`.
- `-cache-path`: Path to the cache database. Default is `.cache`.

## Example

```shell
open-port-exporter -web.listen-address ":9116" -hosts "localhost,example.com" -ports "80,443,1000-2000" -max-connections 200 -timeout 5s -cache-expires 48h -open-port-cache-expires 10m -cache-path "/path/to/cache"
```

## Metrics

The following Prometheus metric is exposed by the Open Port Exporter:

`open_port{host="host", port="port"}`: Indicates the status of the port (1 - open, 0 - closed).

## Development

### Dependencies

- Badger: A fast key-value database in Go.
- Prometheus Go client: Prometheus instrumentation library for Go applications.

### Building from Source

Ensure you have Go installed on your machine.

Clone the repository:

```shell
git clone https://github.com/FindMyKids/open-port-exporter.git
```

Navigate to the project directory:

```shell
cd open-port-exporter
```

Build the project:

```shell
go build -o open-port-exporter
```

### Contributing
Contributions are welcome! Please open an issue or submit a pull request with your changes.

## License
This project is licensed under the MIT License. See the LICENSE file for details.