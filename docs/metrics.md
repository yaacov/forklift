# Forklift Metrics

This document covers the architecture behind metric collection, labeling
conventions, and how to add new metrics as a developer. For the complete list
of every metric (types, labels, descriptions, and example PromQL queries) see
the [Metrics Reference](./metrics-reference.md).

## Architecture overview

The forklift-controller exposes all `mtv_*` metrics on a single HTTPS
endpoint that is scraped by the in-cluster Prometheus stack.

```
┌─────────────────────────────────────────────────────┐
│  forklift-controller                                │
│                                                     │
│  RecordMigrationMetrics ──┐                         │
│  RecordPlanMetrics ───────┤  promauto (default reg) │
│                           ▼                         │
│              promhttp  :8443/metrics  ◄── ServiceMonitor ◄── Prometheus
└─────────────────────────────────────────────────────┘
```

### Metrics endpoint

Metrics are registered with `promauto` into the default Prometheus registry in
[pkg/monitoring/metrics/forklift-controller/metrics.go](../pkg/monitoring/metrics/forklift-controller/metrics.go).
The HTTP endpoint is started by `StartPrometheusEndpoint` in
[pkg/metrics/promethousutil.go](../pkg/metrics/promethousutil.go), which
generates a self-signed TLS certificate and serves on `:8443/metrics`.

The OpenShift Prometheus stack discovers this endpoint through the
`ServiceMonitor` resource defined in
[operator/config/prometheus/monitor.yaml](../operator/config/prometheus/monitor.yaml).

Two background goroutines drive the controller metrics:

- **`RecordMigrationMetrics`** -- started by the Migration controller
  (`pkg/controller/migration/controller.go`). Polls every 10 seconds, lists all
  `Migration` objects, and increments counters for terminal states (Succeeded,
  Failed, Canceled). Uses in-memory maps keyed by migration UID to guarantee
  each migration is counted exactly once.

- **`RecordPlanMetrics`** -- started by the Plan controller
  (`pkg/controller/plan/controller.go`). Polls every 10 seconds, lists all
  `Plan` objects, and recalculates gauge values from scratch. Stale label
  combinations (plans that no longer exist or changed state) are reset to zero
  or deleted.

---

## Metric catalog

See the [Metrics Reference](./metrics-reference.md) for the complete catalog of
every metric, including type, labels, description, and example queries.

At a glance:

| Area | Key metrics | Source |
|---|---|---|
| Migrations | `mtv_migrations_status_total`, `mtv_workload_migrations_status_total`, `mtv_migration_duration_seconds`, `mtv_migrations_duration_seconds`, `mtv_migration_data_transferred_bytes` | `pkg/monitoring/metrics/forklift-controller/` |
| Plans | `mtv_plans_status`, `mtv_plan_alert_status` | `pkg/monitoring/metrics/forklift-controller/` |

---

## Labeling conventions

### Prometheus / OpenShift conventions

Forklift follows standard Prometheus naming conventions adopted by the
OpenShift ecosystem:

| Convention | Example |
|---|---|
| Project prefix | `mtv_` for controller-level metrics |
| Counter suffix | `_total` (e.g. `mtv_migrations_status_total`) |
| Duration unit suffix | `_seconds` (e.g. `mtv_migration_duration_seconds`) |
| Size unit suffix | `_bytes` (e.g. `mtv_migration_data_transferred_bytes`) |
| Histogram buckets | named `_bucket`, `_sum`, `_count` (auto-generated) |
| Snake_case labels | `storage_vendor`, `clone_method`, `owner_uid` |

### Forklift-specific label values

These labels are shared across most controller metrics and have a fixed set of
allowed values:

| Label | Values | Meaning |
|---|---|---|
| `status` | `Succeeded`, `Failed`, `Canceled`, `Executing`, `Running`, `Pending`, `Blocked`, `Deleted` | Lifecycle state of a plan or migration. Not all metrics use all values. |
| `provider` | `oVirt`, `VSphere`, `Openstack`, `OVA`, `Openshift` | Source virtualization provider type (derived from `sourceProvider.Type().String()`). |
| `mode` | `Cold`, `Warm` | Migration mode. Cold copies disks once; warm performs incremental snapshots. |
| `target` | `Local`, `Remote` | Whether the destination provider is the host cluster (`Local`, URL is empty) or a remote cluster (`Remote`). |
| `plan` | UID string | Kubernetes UID of the `Plan` resource, used to correlate metrics to a specific plan. |
| `plan_name` | string | Human-readable name of the plan (used only in `mtv_plan_alert_status`). |
| `phase` | string | Pipeline phase where an error occurred, or `Completed`/`Executing` (used only in `mtv_plan_alert_status`). |

### Deduplication strategy

- **Migration counters** (`mtv_migrations_status_total`,
  `mtv_workload_migrations_status_total`): use three in-memory maps
  (`processedSucceededMigrations`, `processedFailedMigrations`,
  `processedCanceledMigrations`) keyed by migration UID. Each migration is
  counted exactly once per terminal state.

- **Plan gauges** (`mtv_plans_status`, `mtv_plan_alert_status`): recalculated
  from scratch every 10-second cycle. Label combinations that existed in the
  previous cycle but are absent in the current one are explicitly set to zero
  (for `mtv_plans_status`) or deleted (for `mtv_plan_alert_status`).

---

## Developer guide

### Where to define new metrics

All controller-level Prometheus metrics are defined as package-level `var`
blocks in
[pkg/monitoring/metrics/forklift-controller/metrics.go](../pkg/monitoring/metrics/forklift-controller/metrics.go)
using `promauto.NewCounterVec`, `promauto.NewGaugeVec`, or
`promauto.NewHistogramVec`. The `promauto` package automatically registers
metrics with the default Prometheus registry -- no manual `Register()` call is
needed.

### Where to record new metrics

Recording logic lives in the `*_metrics.go` files in the same package:

- [migration_metrics.go](../pkg/monitoring/metrics/forklift-controller/migration_metrics.go) -- migration lifecycle metrics
- [plan_metrics.go](../pkg/monitoring/metrics/forklift-controller/plan_metrics.go) -- plan status metrics

Both follow the same pattern: a goroutine that polls every 10 seconds, lists
the relevant CRs, and updates metrics.

### Adding a new controller metric

1. **Define** the metric in `metrics.go`:

   ```go
   myNewGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
       Name: "mtv_my_new_metric",
       Help: "Description of what this metric measures",
   },
       []string{"label_a", "label_b"},
   )
   ```

2. **Record** it in the appropriate `*_metrics.go` file inside the polling
   loop:

   ```go
   myNewGauge.With(prometheus.Labels{"label_a": valA, "label_b": valB}).Set(value)
   ```

3. **Document** the metric in the
   [Metrics Reference](./metrics-reference.md).

### Key source files

| File | Purpose |
|---|---|
| `pkg/monitoring/metrics/forklift-controller/metrics.go` | Metric definitions |
| `pkg/monitoring/metrics/forklift-controller/migration_metrics.go` | Migration metric recording loop |
| `pkg/monitoring/metrics/forklift-controller/plan_metrics.go` | Plan metric recording loop |
| `pkg/metrics/promethousutil.go` | TLS `/metrics` HTTP server setup |
| `operator/config/prometheus/monitor.yaml` | ServiceMonitor for Prometheus scraping |
