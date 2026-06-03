package manifestrenderer

import (
	"sort"

	errs "github.com/pleme-io/errors-go"
)

// Source is the one typed declaration of a workload. Every dialect renderer is
// a pure projection of this value, so the dialects can never drift from one
// another.
//
// Construct a Source with [New] and functional options rather than building the
// struct literal directly; the constructor applies defaults and validates the
// invariants (notably a non-empty name and a non-empty image) in one place.
type Source struct {
	// Name is the workload name; it seeds resource names, labels, and the Helm
	// chart name. Required.
	Name string
	// Namespace is the target namespace. Empty leaves it unset (rendered into
	// the kustomize overlay / Helm release namespace by the deploy tool).
	Namespace string
	// Image is the container image repository (without the tag). Required.
	Image string
	// Tag is the container image tag. Defaults to "latest".
	Tag string
	// Replicas is the desired replica count. Defaults to 1.
	Replicas int
	// Ports are the named container ports, kept sorted by name for stable
	// output.
	Ports []Port
	// Env are the environment variables, kept sorted by name for stable output.
	Env []EnvVar
	// Labels are extra labels merged onto the standard label set, kept sorted by
	// key for stable output.
	Labels map[string]string
	// ServiceAccount is the workload's service account name. Empty uses the
	// namespace default.
	ServiceAccount string
}

// Port is a named container/service port.
type Port struct {
	Name string
	Port int
}

// EnvVar is a literal environment variable. Secret-valued env is intentionally
// out of scope here — secret materialization is a separate concern.
type EnvVar struct {
	Name  string
	Value string
}

// Option configures a [Source] during [New]. Options apply in order, so a later
// option of the same kind overrides an earlier one.
type Option func(*Source)

// WithNamespace sets the target namespace.
func WithNamespace(ns string) Option { return func(s *Source) { s.Namespace = ns } }

// WithImage sets the container image repository and tag.
func WithImage(image, tag string) Option {
	return func(s *Source) { s.Image, s.Tag = image, tag }
}

// WithReplicas sets the desired replica count.
func WithReplicas(n int) Option { return func(s *Source) { s.Replicas = n } }

// WithPort appends a named port.
func WithPort(name string, port int) Option {
	return func(s *Source) { s.Ports = append(s.Ports, Port{Name: name, Port: port}) }
}

// WithEnv appends a literal environment variable.
func WithEnv(name, value string) Option {
	return func(s *Source) { s.Env = append(s.Env, EnvVar{Name: name, Value: value}) }
}

// WithLabel sets one extra label.
func WithLabel(key, value string) Option {
	return func(s *Source) {
		if s.Labels == nil {
			s.Labels = map[string]string{}
		}
		s.Labels[key] = value
	}
}

// WithServiceAccount sets the workload service account.
func WithServiceAccount(name string) Option {
	return func(s *Source) { s.ServiceAccount = name }
}

// New builds and validates a [Source]. It returns a typed, code-carrying error
// (code "manifest_invalid") when a required field is missing.
func New(name string, opts ...Option) (*Source, error) {
	s := &Source{Name: name, Tag: "latest", Replicas: 1}
	for _, o := range opts {
		o(s)
	}
	if s.Name == "" {
		return nil, errs.New("manifestrenderer: name is required", errs.WithCode("manifest_invalid"))
	}
	if s.Image == "" {
		return nil, errs.New("manifestrenderer: image is required (WithImage)", errs.WithCode("manifest_invalid"))
	}
	if s.Replicas < 0 {
		return nil, errs.New("manifestrenderer: replicas must be >= 0", errs.WithCode("manifest_invalid"))
	}
	// Normalize for stable output: sort ports, env, and label keys.
	sort.Slice(s.Ports, func(i, j int) bool { return s.Ports[i].Name < s.Ports[j].Name })
	sort.Slice(s.Env, func(i, j int) bool { return s.Env[i].Name < s.Env[j].Name })
	return s, nil
}

// sortedLabelKeys returns the merged label keys in deterministic order.
func (s *Source) sortedLabelKeys() []string {
	keys := make([]string, 0, len(s.Labels))
	for k := range s.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stdLabels returns the standard recommended label set plus any extra labels,
// as a deterministically-ordered list of key/value pairs.
func (s *Source) stdLabels() []kv {
	out := []kv{
		{"app.kubernetes.io/name", s.Name},
		{"app.kubernetes.io/managed-by", "manifest-renderer-go"},
	}
	for _, k := range s.sortedLabelKeys() {
		out = append(out, kv{k, s.Labels[k]})
	}
	return out
}

// kv is one deterministically-ordered key/value pair.
type kv struct{ K, V string }
