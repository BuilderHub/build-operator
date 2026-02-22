package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var buildkitbuildertemplatelog = logf.Log.WithName("buildkitbuildertemplate-resource")

// SetupWebhookWithManager registers the webhook
func (r *BuildkitBuilderTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to disable mutation.
// +kubebuilder:webhook:path=/mutate-builder-hub-dev-v1alpha1-buildkitbuildertemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=builder-hub.dev,resources=buildkitbuildertemplates,verbs=create;update,versions=v1alpha1,name=mbuildkitbuildertemplate.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &BuildkitBuilderTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *BuildkitBuilderTemplate) Default() {
	buildkitbuildertemplatelog.Info("default", "name", r.Name)
	if r.Spec.BuildkitImage == "" {
		if r.Spec.Rootless {
			r.Spec.BuildkitImage = "moby/buildkit:v0.18.0-rootless"
		} else {
			r.Spec.BuildkitImage = "moby/buildkit:v0.18.0"
		}
	}
	if r.Spec.Arch == "" {
		r.Spec.Arch = "amd64"
	}
	if r.Spec.CacheConfig.Type == CacheTypePVC && r.Spec.CacheConfig.PVC != nil {
		if len(r.Spec.CacheConfig.PVC.AccessModes) == 0 {
			r.Spec.CacheConfig.PVC.AccessModes = []string{string(corev1.ReadWriteOnce)}
		}
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to disable validation.
// +kubebuilder:webhook:path=/validate-builder-hub-dev-v1alpha1-buildkitbuildertemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=builder-hub.dev,resources=buildkitbuildertemplates,verbs=create;update,versions=v1alpha1,name=vbuildkitbuildertemplate.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &BuildkitBuilderTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *BuildkitBuilderTemplate) ValidateCreate() (admission.Warnings, error) {
	buildkitbuildertemplatelog.Info("validate create", "name", r.Name)
	return nil, r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *BuildkitBuilderTemplate) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	buildkitbuildertemplatelog.Info("validate update", "name", r.Name)
	return nil, r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *BuildkitBuilderTemplate) ValidateDelete() (admission.Warnings, error) {
	buildkitbuildertemplatelog.Info("validate delete", "name", r.Name)
	return nil, nil
}

func (r *BuildkitBuilderTemplate) validate() error {
	switch r.Spec.CacheConfig.Type {
	case CacheTypePVC:
		if r.Spec.CacheConfig.PVC == nil {
			return fmt.Errorf("cacheConfig.pvc: required when type is pvc")
		}
		if r.Spec.CacheConfig.PVC.Size == "" {
			return fmt.Errorf("cacheConfig.pvc.size: required")
		}
	case CacheTypeS3:
		if r.Spec.CacheConfig.S3 == nil {
			return fmt.Errorf("cacheConfig.s3: required when type is s3")
		}
		if r.Spec.CacheConfig.S3.Bucket == "" {
			return fmt.Errorf("cacheConfig.s3.bucket: required")
		}
	}
	return nil
}
