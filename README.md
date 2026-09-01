# Gremlin-in-a-Box

A chaos engineering platform built from scratch in Go, with zero third-party dependencies. It injects controlled failures (container kills, network latency, CPU exhaustion) into a live application, measures how long the system takes to recover, and exposes the results as metrics, dashboards, and reports.

## Why zero dependencies

Every piece here - the Docker Engine API client, the Prometheus metrics exporter, the CLI - is hand-written against the Go standard library instead of using the Docker SDK, Cobra, or the Prometheus client library. This was a deliberate choice: it means the whole project builds anywhere with just a Go toolchain, there is no dependency-version drift to break a grading run, and every line of behavior can be explained without pointing at a third-party library doing the real work underneath.

## Architecture

Two systems, kept deliberately separate:

- Application Under Test (AUT) - the thing that gets attacked. A minimal Go HTTP API with a /health endpoint, backed by PostgreSQL, running in Docker Compose.
- Gremlin-in-a-Box - the chaos platform itself. A CLI that drives a controller, which loads an attack plugin, runs it against the AUT, measures recovery time, and records everything.

This separation means the AUT can be swapped for any other containerized application later without touching a single line of Gremlin.
## What is actually implemented

| Component | Status | Notes |
|---|---|---|
| Application Under Test | Done | Go API + PostgreSQL, Docker Compose |
| CLI | Done | version, status, attack, report, serve-metrics, schedule |
| Controller | Done | Orchestrates attack -> recovery measurement -> report |
| Docker manager | Done | Raw Docker Engine API client (unix socket), no SDK |
| Network chaos | Done | Latency via tc/netem, run through Docker exec |
| CPU chaos | Done | stress-ng, run through Docker exec |
| Plugin architecture | Done | Plugin interface + registry, adding an attack is one new file |
| Scheduler | Done | Recurring attacks on a configurable interval, schedule.json |
| Metrics | Done | Hand-rolled Prometheus text exposition format, labeled by attack type |
| Dashboard | Done | Grafana, read-only, 8 panels |
| Reports | Done | JSON file store, queryable via gremlin report |
| CI/CD | Done | GitHub Actions: build, run AUT, run 3 attacks, upload report artifact |
| Kubernetes manager | Not built | Would follow the same pattern as the Docker manager, against the kube-apiserver REST API |

## Project layout

    gremlin-in-a-box/
      cmd/gremlin/main.go         CLI entrypoint
      internal/
        config/                   JSON config + schedule loading
        logger/                   structured logging (log/slog)
        dockerapi/                Docker Engine API client (unix socket)
        network/                  tc/netem wrapper
        plugins/                  Plugin interface, registry, and the 3 built-in attacks
        controller/               orchestration: run attack, measure recovery, save report
        metrics/                  Prometheus-format /metrics endpoint
        reports/                  JSON-backed report store
        scheduler/                recurring attack scheduling
      aut/                        Application Under Test (Go API + Postgres)
      monitoring/                 Prometheus + Grafana compose stack
      .github/workflows/chaos.yml CI pipeline
      schedule.json               recurring attack configuration
      config.json                 Gremlin runtime configuration
## Running it

Bring up the application under test:

    cd aut
    docker compose up -d --build
    curl http://localhost:8080/health

Install dependencies:

    go mod tidy
    go mod download

Build Gremlin:

    go build -o bin/gremlin ./cmd/gremlin

See available attacks:

    ./bin/gremlin status

Run one manually:

    ./bin/gremlin attack --name container-kill --target aut-api
    ./bin/gremlin attack --name latency --target aut-api --delay-ms 300 --duration-s 15
    ./bin/gremlin attack --name cpu --target aut-api --workers 2 --duration-s 15

Check what happened:

    ./bin/gremlin report

Run attacks automatically on a schedule (edit schedule.json to change intervals):

    ./bin/gremlin schedule --file schedule.json

Expose metrics for Prometheus:

    ./bin/gremlin serve-metrics

Bring up Prometheus + Grafana:

    cd monitoring
    docker compose up -d

Grafana: http://localhost:3001 (default admin/admin, or whatever you set). Prometheus: http://localhost:9091.
## CI/CD

Every push to main triggers .github/workflows/chaos.yml, which builds Gremlin from a clean checkout, brings up the AUT in a container, runs all three attack types against it, and uploads the resulting gremlin-reports.json as a downloadable build artifact. This proves the whole pipeline works unattended, not just on one development machine.

## Design notes worth calling out

- Plugin architecture: adding a new attack type means writing one file that implements the Plugin interface and registering it in internal/plugins/setup.go. The controller and CLI never need to change.
- Read-only dashboard: Grafana only ever reads from Prometheus; it has no ability to trigger or control attacks. Observability and control are kept as separate concerns, matching how production chaos engineering tools are designed.
- Recovery measurement is real: after every attack, the controller polls the AUT actual /health endpoint until it responds, rather than assuming recovery based on a fixed delay.
- Metrics are computed from the reports file on every scrape, not from in-memory state - this means metrics stay correct even though each gremlin attack invocation is a separate, short-lived process.

## Known limitations

- No Kubernetes manager yet (Docker-only for container/network/CPU attacks).
- Recovery time granularity is limited by the health-check poll interval (500ms), so very fast recoveries are all reported in the low single-digit milliseconds and do not meaningfully differentiate between attack types against this particular AUT, which is intentionally lightweight.
- The network chaos plugin (tc/netem) has been verified working against a local Docker host; behavior inside more restricted CI environments that may not grant full NET_ADMIN capability to nested containers has not been separately confirmed.
