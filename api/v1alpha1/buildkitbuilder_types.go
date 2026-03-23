package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	templatev1alpha1 "github.com/builderhub/build-operator/api/buildertemplate/v1alpha1"
)

// BuilderMode defines the lifecycle of a BuildkitBuilder.
// +kubebuilder:validation:Enum=ephemeral;persistent;sleepy
type BuilderMode string

const (
	BuilderModeEphemeral  BuilderMode = "ephemeral"
	BuilderModePersistent BuilderMode = "persistent"
	BuilderModeSleepy     BuilderMode = "sleepy"
)

// BuildkitBuilderSpec defines the desired state of BuildkitBuilder
//
// +kubebuilder:object:generate=true
type BuildkitBuilderSpec struct {
	// TemplateRef references a BuildkitBuilderTemplate (cluster-scoped) by name.
	// If set, spec is derived from the template. Inline spec overrides take precedence.
	// +optional
	TemplateRef *string `json:"templateRef,omitempty"`

	// Inline spec when not using a template (or to override template fields).
	// +optional
	Template *templatev1alpha1.BuildkitBuilderTemplateSpec `json:"template,omitempty"`

	// Mode: ephemeral (one pod per build, auto-cleanup), persistent (always-on),
	// sleepy (scale-to-zero, cache preserved).
	// +kubebuilder:validation:Required
	Mode BuilderMode `json:"mode"`

	// Replicas for persistent/sleepy mode (default: 1). Ignored for ephemeral.
	// +optional
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// IdleTimeoutSeconds for sleepy mode: scale to 0 after this idle period (default: 300).
	// BuilderHub API patches builder.builder-hub.dev/last-used annotation when a build starts.
	// +optional
	// +kubebuilder:default=300
	IdleTimeoutSeconds *int32 `json:"idleTimeoutSeconds,omitempty"`

	// Labels for service discovery and routing in BuilderHub frontend
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// OwnerReferences are preserved so CI jobs can own ephemeral builders
	// (set by the creating controller, not in spec)
}

// BuildkitBuilderStatus defines the observed state of BuildkitBuilder
//
// +kubebuilder:object:generate=true
type BuildkitBuilderStatus struct {
	// Endpoint is the BuildKit TCP address for cluster-internal use (e.g. tcp://10.0.0.1:1234)
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// NodePort is the allocated NodePort for external access (connect via <node-ip>:NodePort)
	// +optional
	NodePort int32 `json:"nodePort,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase: Pending, Ready, ScalingDown, Error
	// +optional
	Phase string `json:"phase,omitempty"`

	// ReadyReplicas for persistent/sleepy mode
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// DesiredReplicas for sleepy mode (0 or 1)
	// +optional
	DesiredReplicas int32 `json:"desiredReplicas,omitempty"`

	// LastScaledAt is the last time the controller scaled (sleepy)
	// +optional
	LastScaledAt *metav1.Time `json:"lastScaledAt,omitempty"`
}

// Condition types for BuildkitBuilder
const (
	ConditionReady         = "Ready"
	ConditionEndpoint      = "Endpoint"
	ConditionCacheAttached = "CacheAttached"
	ConditionLastScaled    = "LastScaled"
)

// BuildkitBuilder is the Schema for the BuildkitBuilder API (namespaced instance)
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=buildkitbuilders,scope=Namespaced,singular=buildkitbuilder
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`
// +kubebuilder:printcolumn:name="NodePort",type=integer,JSONPath=`.status.nodePort`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BuildkitBuilder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildkitBuilderSpec   `json:"spec,omitempty"`
	Status BuildkitBuilderStatus `json:"status,omitempty"`
}

// BuildkitBuilderList contains a list of BuildkitBuilder
// +kubebuilder:object:root=true
type BuildkitBuilderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuildkitBuilder `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BuildkitBuilder{}, &BuildkitBuilderList{})
}
