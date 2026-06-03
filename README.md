# manifest-renderer-go

One typed source → a Helm chart **and** a kustomize overlay **and** an OpenShift
variant — byte-stable — so chart-vs-manifest drift becomes structurally
impossible.

## What

A single typed [`Source`](./source.go) (name, image, replicas, ports, env,
labels) is rendered by three pure functions to three manifest dialects:

| Verb | Output |
|---|---|
| `Source.RenderHelm()` | `Chart.yaml` + `values.yaml` + `templates/` manifests |
| `Source.RenderKustomize()` | `base/` objects + `overlays/default/kustomization.yaml` |
| `Source.RenderOpenShift()` | OpenShift manifests (SCC-aware Deployment + `Route`) |

Each returns a `Files` (path → bytes) map whose `Paths()` iterate in
lexicographic order. The renderers are **pure functions of the `Source`**: the
same input yields identical bytes on every call (deterministic key ordering, no
map-iteration nondeterminism, no timestamps).

## Why

Teams maintain a Helm chart, a hand-written kustomize tree, and an OpenShift
manifest set for the *same* service — and they drift. A replica bump lands in
the chart but not the overlay; a security-context tweak lands in kustomize but
not OpenShift. The cure is one typed source of truth and pure renderers: the
three dialects are projections of the same value, so they **cannot** disagree.
Byte-stability makes the rendered artifacts safe to commit and diff in CI.

## Install

```
go get github.com/pleme-io/manifest-renderer-go
```

## Usage

```go
src, err := manifestrenderer.New("api",
    manifestrenderer.WithImage("ghcr.io/acme/api", "1.4.0"),
    manifestrenderer.WithReplicas(3),
    manifestrenderer.WithPort("http", 8080),
    manifestrenderer.WithEnv("LOG_LEVEL", "info"),
    manifestrenderer.WithNamespace("prod"))
if err != nil {
    return errs.Exit(err) // typed code-carrying error (manifest_invalid)
}

helm, _ := src.RenderHelm()
for _, path := range helm.Paths() {
    os.WriteFile(path, helm[path], 0o644)
}
```

## Configuration

None — a pure library. Callers that read the workload shape from config use
`shikumi-go` and pass the fields via the functional options. A `FromConfig`
bridge over a `shikumi`-loaded `Source`-shaped struct is the natural extension
when a consumer wants config-driven rendering.

## Release

Pull-model (Go modules): an annotated `vX.Y.Z` tag is the release; pkg.go.dev
indexes it. See the GSDS module delivery FSM.
