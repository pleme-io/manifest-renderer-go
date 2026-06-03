// Package manifestrenderer is the fleet's one typed source → manifest-dialect
// renderer — so a workload is declared ONCE as a typed [Source] and rendered,
// byte-stable, to a Helm chart, a kustomize overlay, and an OpenShift variant.
// This kills chart-vs-manifest drift: the three dialects can never disagree
// because they are projections of the same typed value.
//
// # Why
//
// Teams routinely maintain a Helm chart AND a hand-written kustomize tree AND
// an OpenShift manifest set for the same service. They drift — a replica bump
// lands in the chart but not the overlay, a security-context tweak lands in
// kustomize but not OpenShift. The cure is a single typed source of truth and
// pure, deterministic renderers.
//
// # Shape (Law 1, §3.5)
//
//	src := manifestrenderer.New("api",
//	    manifestrenderer.WithImage("ghcr.io/acme/api", "1.4.0"),
//	    manifestrenderer.WithReplicas(3),
//	    manifestrenderer.WithPort("http", 8080))
//	chart, err := src.RenderHelm()       // map[path]bytes — a complete chart
//	overlay, err := src.RenderKustomize() // map[path]bytes — base + overlay
//	ocp, err := src.RenderOpenShift()     // OpenShift-flavoured manifests
//
// Every renderer is a pure function of the [Source]: same input → identical
// bytes, every time (deterministic key ordering, no maps iterated in render
// order, no timestamps). That byte-stability is the property that makes the
// rendered artifacts safe to commit and diff in CI.
//
// # Weight (Law 6)
//
// Pure standard library plus errors-go for typed, code-carrying errors. No YAML
// dependency: the deterministic emitter is internal so the rendered bytes are
// owned by this package, not by an upstream marshaller's formatting choices.
package manifestrenderer
