package controller

// Keys for labels, annotations, and finalizers on builder-owned Kubernetes objects (pods,
// services, statefulsets, etc.). Prefix builder.builder-hub.dev is distinct from the API
// group domain builder-hub.dev.
const (
	LabelKeyBuilderName = "builder.builder-hub.dev/name"
	LabelKeyBuilderMode   = "builder.builder-hub.dev/mode"

	// AnnotationLastUsed is patched by BuilderHub API when a build starts (RFC3339).
	AnnotationLastUsed = "builder.builder-hub.dev/last-used"

	// FinalizerPVC ensures PVC cleanup on CR delete when applicable.
	FinalizerPVC = "builder.builder-hub.dev/pvc-finalizer"
)
