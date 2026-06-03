package manifestrenderer

// This file builds the dialect-neutral Kubernetes object nodes. Every renderer
// projects from these builders, so a Deployment rendered into a Helm template,
// a kustomize base, or an OpenShift manifest is structurally identical — the
// dialects differ only in packaging, never in workload shape.

// labelMap renders the standard label set as a YAML mapping node.
func (s *Source) labelMap() *node {
	m := mapping()
	for _, p := range s.stdLabels() {
		m.set(p.K, scalar(p.V))
	}
	return m
}

// selectorLabels renders the minimal selector label set (name only) so the
// selector is immutable across replica/label edits.
func (s *Source) selectorLabels() *node {
	return mapping().set("app.kubernetes.io/name", scalar(s.Name))
}

// containerNode renders the single container spec shared by every dialect.
func (s *Source) containerNode() *node {
	c := mapping().
		set("name", scalar(s.Name)).
		set("image", scalar(s.Image+":"+s.Tag))
	if len(s.Ports) > 0 {
		ports := sequence()
		for _, p := range s.Ports {
			ports.items = append(ports.items, mapping().
				set("name", scalar(p.Name)).
				set("containerPort", intScalar(p.Port)))
		}
		c.set("ports", ports)
	}
	if len(s.Env) > 0 {
		env := sequence()
		for _, e := range s.Env {
			env.items = append(env.items, mapping().
				set("name", scalar(e.Name)).
				set("value", scalar(e.Value)))
		}
		c.set("env", env)
	}
	return c
}

// deployment renders a Deployment object node. When openshift is true the
// container's securityContext is omitted so the OpenShift SCC assigns a UID
// from the namespace range (the canonical OpenShift difference).
func (s *Source) deployment(openshift bool) *node {
	podSpec := mapping()
	if s.ServiceAccount != "" {
		podSpec.set("serviceAccountName", scalar(s.ServiceAccount))
	}
	if !openshift {
		// On vanilla K8s we pin a non-root securityContext; on OpenShift the SCC
		// owns this, and pinning a UID conflicts with the assigned range.
		podSpec.set("securityContext", mapping().
			set("runAsNonRoot", boolScalar(true)).
			set("seccompProfile", mapping().set("type", scalar("RuntimeDefault"))))
	}
	podSpec.set("containers", sequence(s.containerNode()))

	tmpl := mapping().
		set("metadata", mapping().set("labels", s.selectorLabels())).
		set("spec", podSpec)

	spec := mapping().
		set("replicas", intScalar(s.Replicas)).
		set("selector", mapping().set("matchLabels", s.selectorLabels())).
		set("template", tmpl)

	return s.object("apps/v1", "Deployment", spec)
}

// service renders a Service object node, omitted by the caller when there are
// no ports.
func (s *Source) service() *node {
	ports := sequence()
	for _, p := range s.Ports {
		ports.items = append(ports.items, mapping().
			set("name", scalar(p.Name)).
			set("port", intScalar(p.Port)).
			set("targetPort", scalar(p.Name)))
	}
	spec := mapping().
		set("selector", s.selectorLabels()).
		set("ports", ports)
	return s.object("v1", "Service", spec)
}

// object wraps a spec in the apiVersion/kind/metadata envelope shared by every
// dialect.
func (s *Source) object(apiVersion, kind string, spec *node) *node {
	meta := mapping().set("name", scalar(s.Name))
	if s.Namespace != "" {
		meta.set("namespace", scalar(s.Namespace))
	}
	meta.set("labels", s.labelMap())
	return mapping().
		set("apiVersion", scalar(apiVersion)).
		set("kind", scalar(kind)).
		set("metadata", meta).
		set("spec", spec)
}
