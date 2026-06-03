package manifestrenderer

import (
	"sort"
	"strings"
)

// Files is a rendered artifact set: a path → bytes map. Use [Files.Paths] for a
// deterministically-ordered iteration so callers writing the set to disk (and
// diffing it in CI) get stable ordering.
type Files map[string][]byte

// Paths returns the file paths sorted lexicographically — deterministic order
// for writing and diffing.
func (f Files) Paths() []string {
	out := make([]string, 0, len(f))
	for p := range f {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// RenderHelm renders the source as a complete, byte-stable Helm chart: a
// Chart.yaml, a values.yaml, and templated manifests under templates/. The
// templates render concrete manifests (no Go-template expansion) so the chart's
// rendered output equals the kustomize/OpenShift output object-for-object —
// that equality is the drift-killer.
func (s *Source) RenderHelm() (Files, error) {
	chartYAML := mapping().
		set("apiVersion", scalar("v2")).
		set("name", scalar(s.Name)).
		set("description", scalar("Chart for "+s.Name+", rendered by manifest-renderer-go.")).
		set("type", scalar("application")).
		set("version", scalar("0.1.0")).
		set("appVersion", scalar(s.Tag))

	valuesYAML := mapping().
		set("image", mapping().
			set("repository", scalar(s.Image)).
			set("tag", scalar(s.Tag))).
		set("replicas", intScalar(s.Replicas))

	files := Files{
		s.Name + "/Chart.yaml":                toYAML(chartYAML),
		s.Name + "/values.yaml":               toYAML(valuesYAML),
		s.Name + "/templates/deployment.yaml": toYAML(s.deployment(false)),
	}
	if len(s.Ports) > 0 {
		files[s.Name+"/templates/service.yaml"] = toYAML(s.service())
	}
	return files, nil
}

// RenderKustomize renders the source as a kustomize base plus an overlay. The
// base holds the raw objects; the overlay holds a kustomization.yaml that
// references the base and pins the namespace/image tag — the canonical
// base+overlay split. Output is byte-stable.
func (s *Source) RenderKustomize() (Files, error) {
	files := Files{
		"base/deployment.yaml": toYAML(s.deployment(false)),
	}
	resources := sequence(scalar("deployment.yaml"))
	if len(s.Ports) > 0 {
		files["base/service.yaml"] = toYAML(s.service())
		resources.items = append(resources.items, scalar("service.yaml"))
	}
	files["base/kustomization.yaml"] = toYAML(mapping().
		set("apiVersion", scalar("kustomize.config.k8s.io/v1beta1")).
		set("kind", scalar("Kustomization")).
		set("resources", resources))

	overlay := mapping().
		set("apiVersion", scalar("kustomize.config.k8s.io/v1beta1")).
		set("kind", scalar("Kustomization")).
		set("resources", sequence(scalar("../../base")))
	if s.Namespace != "" {
		overlay.set("namespace", scalar(s.Namespace))
	}
	overlay.set("images", sequence(mapping().
		set("name", scalar(s.Image)).
		set("newTag", scalar(s.Tag))))
	files["overlays/default/kustomization.yaml"] = toYAML(overlay)
	return files, nil
}

// RenderOpenShift renders the source as OpenShift-flavoured manifests: the same
// objects as the K8s base, minus the runAsNonRoot securityContext (the
// OpenShift SCC assigns the UID), plus a Route for the first port when one
// exists. Output is byte-stable.
func (s *Source) RenderOpenShift() (Files, error) {
	files := Files{
		"openshift/deployment.yaml": toYAML(s.deployment(true)),
	}
	if len(s.Ports) > 0 {
		files["openshift/service.yaml"] = toYAML(s.service())
		// Route to the first (sorted) port — the OpenShift ingress idiom.
		first := s.Ports[0]
		route := mapping().
			set("apiVersion", scalar("route.openshift.io/v1")).
			set("kind", scalar("Route")).
			set("metadata", mapping().
				set("name", scalar(s.Name)).
				set("labels", s.labelMap())).
			set("spec", mapping().
				set("to", mapping().
					set("kind", scalar("Service")).
					set("name", scalar(s.Name))).
				set("port", mapping().set("targetPort", scalar(first.Name))))
		files["openshift/route.yaml"] = toYAML(route)
	}
	return files, nil
}

// concatDocs joins rendered documents with the YAML document separator — a
// helper for callers that want a single multi-document stream rather than a
// file set. Documents are concatenated in lexicographic path order for stable
// output.
func concatDocs(f Files) []byte {
	var b strings.Builder
	for i, p := range f.Paths() {
		if i > 0 {
			b.WriteString("---\n")
		}
		b.Write(f[p])
	}
	return []byte(b.String())
}
