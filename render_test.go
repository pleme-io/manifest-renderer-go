package manifestrenderer

import (
	"bytes"
	"strings"
	"testing"

	errs "github.com/pleme-io/errors-go"
)

// mustSource builds a representative source for the render tests.
func mustSource(t *testing.T, opts ...Option) *Source {
	t.Helper()
	base := []Option{
		WithImage("ghcr.io/acme/api", "1.4.0"),
		WithReplicas(3),
		WithPort("http", 8080),
		WithEnv("LOG_LEVEL", "info"),
		WithNamespace("prod"),
	}
	s, err := New("api", append(base, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestNew_Validation(t *testing.T) {
	cases := []struct {
		name     string
		ctorName string
		opts     []Option
		wantCode string
	}{
		{"missing name", "", []Option{WithImage("x", "1")}, "manifest_invalid"},
		{"missing image", "api", nil, "manifest_invalid"},
		{"negative replicas", "api", []Option{WithImage("x", "1"), WithReplicas(-1)}, "manifest_invalid"},
		{"valid", "api", []Option{WithImage("x", "1")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.ctorName, tc.opts...)
			gotCode := errs.CodeOf(err)
			if gotCode != tc.wantCode {
				t.Fatalf("code = %q, want %q (err=%v)", gotCode, tc.wantCode, err)
			}
		})
	}
}

func TestRenderers_ProduceExpectedFiles(t *testing.T) {
	s := mustSource(t)
	cases := []struct {
		name      string
		render    func() (Files, error)
		wantPaths []string
	}{
		{
			"helm",
			s.RenderHelm,
			[]string{"api/Chart.yaml", "api/templates/deployment.yaml", "api/templates/service.yaml", "api/values.yaml"},
		},
		{
			"kustomize",
			s.RenderKustomize,
			[]string{"base/deployment.yaml", "base/kustomization.yaml", "base/service.yaml", "overlays/default/kustomization.yaml"},
		},
		{
			"openshift",
			s.RenderOpenShift,
			[]string{"openshift/deployment.yaml", "openshift/route.yaml", "openshift/service.yaml"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.render()
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			got := strings.Join(f.Paths(), ",")
			want := strings.Join(tc.wantPaths, ",")
			if got != want {
				t.Fatalf("paths = %q, want %q", got, want)
			}
		})
	}
}

// TestByteStable is the load-bearing invariant: the same source renders
// identical bytes every time (no map-iteration nondeterminism, no timestamps).
func TestByteStable(t *testing.T) {
	s := mustSource(t, WithLabel("team", "platform"), WithLabel("tier", "backend"))
	renderers := map[string]func() (Files, error){
		"helm":      s.RenderHelm,
		"kustomize": s.RenderKustomize,
		"openshift": s.RenderOpenShift,
	}
	for name, r := range renderers {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 16; i++ {
				a, _ := r()
				b, _ := r()
				for _, p := range a.Paths() {
					if !bytes.Equal(a[p], b[p]) {
						t.Fatalf("%s/%s not byte-stable across renders", name, p)
					}
				}
			}
		})
	}
}

// TestDialectsAgree proves the drift-kill property: the Deployment object
// rendered by every dialect is structurally identical except for the
// OpenShift securityContext difference.
func TestDialectsAgree(t *testing.T) {
	s := mustSource(t)
	helm, _ := s.RenderHelm()
	kust, _ := s.RenderKustomize()
	ocp, _ := s.RenderOpenShift()

	helmDeploy := string(helm["api/templates/deployment.yaml"])
	kustDeploy := string(kust["base/deployment.yaml"])
	ocpDeploy := string(ocp["openshift/deployment.yaml"])

	if helmDeploy != kustDeploy {
		t.Fatalf("helm and kustomize Deployments differ:\nHELM:\n%s\nKUST:\n%s", helmDeploy, kustDeploy)
	}
	// Every dialect must carry the same image:tag and replica count.
	for name, doc := range map[string]string{"helm": helmDeploy, "kustomize": kustDeploy, "openshift": ocpDeploy} {
		if !strings.Contains(doc, "image: ghcr.io/acme/api:1.4.0") {
			t.Errorf("%s deployment missing pinned image", name)
		}
		if !strings.Contains(doc, "replicas: 3") {
			t.Errorf("%s deployment missing replicas", name)
		}
	}
	// OpenShift drops the runAsNonRoot securityContext; vanilla K8s keeps it.
	if strings.Contains(ocpDeploy, "runAsNonRoot") {
		t.Error("openshift deployment must not pin runAsNonRoot (SCC owns it)")
	}
	if !strings.Contains(kustDeploy, "runAsNonRoot") {
		t.Error("kustomize deployment must pin runAsNonRoot")
	}
}

func TestConcatDocs_Deterministic(t *testing.T) {
	s := mustSource(t)
	f, _ := s.RenderOpenShift()
	a := concatDocs(f)
	b := concatDocs(f)
	if !bytes.Equal(a, b) {
		t.Fatal("concatDocs not deterministic")
	}
	if !bytes.Contains(a, []byte("---\n")) {
		t.Error("multi-doc stream missing separator")
	}
}

func TestYAML_QuotingRules(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"info", "info"},
		{"true", `"true"`},
		{"8080", `"8080"`},
		{"", `""`},
		{"a: b", `"a: b"`},
		{"plain-value", "plain-value"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := quoteScalar(tc.in); got != tc.want {
				t.Errorf("quoteScalar(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
