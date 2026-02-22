package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CacheType represents the type of BuildKit cache backend.
// +kubebuilder:validation:Enum=pvc;none;s3
type CacheType string

const (
	CacheTypePVC  CacheType = "pvc"
	CacheTypeNone CacheType = "none"
	CacheTypeS3   CacheType = "s3"
)

// PVCConfig defines PersistentVolumeClaim settings for cache storage.
type PVCConfig struct {
	StorageClassName string   `json:"storageClassName,omitempty"`
	Size             string   `json:"size"`
	AccessModes      []string `json:"accessModes,omitempty"` // default: [ReadWriteOnce]
}

// S3Config defines S3/MinIO settings for cache storage.
// Credentials are provided via secretRef; operator mounts them as a projected volume.
type S3Config struct {
	Bucket    string                       `json:"bucket"`
	Region    string                       `json:"region,omitempty"`
	Endpoint  string                       `json:"endpoint,omitempty"`
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// CacheConfig defines the BuildKit cache backend configuration.
type CacheConfig struct {
	Type CacheType  `json:"type"`
	PVC  *PVCConfig `json:"pvc,omitempty"`
	S3   *S3Config  `json:"s3,omitempty"`
}

// BuildkitBuilderTemplateSpec defines the desired state of BuildkitBuilderTemplate
type BuildkitBuilderTemplateSpec struct {
	// BuildkitImage is the BuildKit daemon image (default: moby/buildkit:v0.18.0-rootless)
	// +optional
	// +kubebuilder:default="moby/buildkit:v0.18.0-rootless"
	BuildkitImage string `json:"buildkitImage,omitempty"`

	// Rootless runs BuildKit in rootless mode for security (default: true)
	// +optional
	// +kubebuilder:default=true
	Rootless bool `json:"rootless,omitempty"`

	// Resources defines CPU/memory/ephemeral-storage requests and limits
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// NodeSelector for bare-metal arch pinning (e.g. kubernetes.io/arch: amd64)
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for scheduling
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity for scheduling (e.g. prefer nodes with specific arch)
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// BuildkitdToml is the buildkitd.toml config as a multiline string.
	// Operator injects tcp listener on port 1234 + TLS config.
	// +optional
	BuildkitdToml string `json:"buildkitdToml,omitempty"`

	// CacheConfig defines the cache backend (pvc, none, or s3)
	CacheConfig CacheConfig `json:"cacheConfig"`

	// Arch is the target architecture for multi-arch builds (amd64, arm64).
	// Used to set nodeSelector kubernetes.io/arch.
	// +optional
	// +kubebuilder:validation:Enum=amd64;arm64
	// +kubebuilder:default="amd64"
	Arch string `json:"arch,omitempty"`
}

// BuildkitBuilderTemplate is the Schema for the BuildkitBuilderTemplate API (cluster-scoped blueprint)
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=buildkitbuildertemplates,scope=Cluster,singular=buildkitbuildertemplate
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.buildkitImage`
// +kubebuilder:printcolumn:name="Arch",type=string,JSONPath=`.spec.arch`
// +kubebuilder:printcolumn:name="Cache",type=string,JSONPath=`.spec.cacheConfig.type`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type BuildkitBuilderTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BuildkitBuilderTemplateSpec `json:"spec,omitempty"`
}

// BuildkitBuilderTemplateList contains a list of BuildkitBuilderTemplate
// +kubebuilder:object:root=true
type BuildkitBuilderTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuildkitBuilderTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BuildkitBuilderTemplate{}, &BuildkitBuilderTemplateList{})
}
