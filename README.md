# ResilenceOps

An application resilience console with guided chaos experiments, continuous tests,
recovery checks, saved reports, and a Grafana dashboard.

## Start

Go 1.22+ and Docker Compose are required. Build and open the menu from this directory:

```bash
go build -o resiletops ./cmd/gremlin
./resiletops
```

The banner is bold red in a terminal. Menu options:

1. Guided experiment
2. Continuous testing
3. Results & history
4. Application status
5. Advanced tools
6. View running services
0. Back or exit

Running services displays application service names, container names, and status.
Discovery uses Docker Compose ownership labels from `application_directory` in
`config.json`. Unrelated applications and monitoring containers are excluded.

## Test application

Sock Shop is the configured application under test. Clone once and start it:

```bash
git clone https://github.com/microservices-demo/microservices-demo.git aut/microservices-demo
cd aut/microservices-demo/deploy/docker-compose
docker compose -f docker-compose.yml up -d
```

Build the network-fault helper once before running latency, packet-loss, or
corruption experiments (from the project root):

```bash
docker build -t gremlin-helper:latest -f tools/netem.Dockerfile .
```

Open http://localhost/. The default recovery check uses this endpoint; it verifies
HTTP availability, so choose a more specific endpoint when testing backend behavior.
The upstream checkout is kept locally and ignored by this project's Git repository.
The old API/Postgres sample has been removed; CI now uses Sock Shop too.

## Continuous testing

```bash
./resiletops targets
./resiletops continuous --target docker-compose-front-end-1 --attacks container-pause,cpu --duration-s 5 --interval-s 10
# A bounded run:
./resiletops continuous --target docker-compose-front-end-1 --attacks container-pause --duration-s 1 --interval-s 1 --cycles 2
```

Tests execute sequentially, verify recovery, and wait between faults. A failed test
stops the run. Ctrl+C cancels the active test and waits for cleanup. Application
membership is checked before every fault. Supported attacks include latency,
packet loss, corruption, CPU throttling, container pause, and restart.

Use `./resiletops --help` for direct attacks, scheduling, load tests, threshold tests,
and reports. `schedule.json` supplies recurring jobs; run it explicitly with
`./resiletops schedule --file schedule.json`.

## Monitoring

```bash
docker compose -f monitoring/docker-compose.yml up -d --build
python3 monitoring/verify-data.py
```

Grafana: http://localhost:3001 (initial credentials admin/admin).
Prometheus: http://localhost:9091.

The persistent exporter reads `gremlin-reports.json` through a directory mount so
atomic report replacements remain visible. Grafana uses the internal Prometheus
service address. The dashboard includes gradient figures, subtle animation, and
reduced-motion support. Missing recovery measurements remain unmeasured rather
than becoming fabricated zeroes. Historical success and verified recovery are
reported separately. The audit compares live Grafana queries against saved reports.

Report history and `resilience-history.json` are retained across upgrades.
The old launchers are retired; the protected original binary remains archived in
`.retired-bin` pending administrator removal. The only current launcher is
`./resiletops`.

## Development

```bash
go test -race ./...
go vet ./...
go build -o resiletops ./cmd/gremlin
```

Source stays under `cmd/gremlin` and `internal` to preserve module imports.
GitHub Actions builds the launcher, starts Sock Shop, runs faults and threshold
checks, uploads reports, and tears down its CI application.
