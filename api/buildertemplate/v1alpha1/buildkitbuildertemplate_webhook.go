package v1alpha1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var buildkitbuildertemplatelog = logf.Log.WithName("buildkitbuildertemplate-resource")

func (r *BuildkitBuilderTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-builder-template-builder-hub-dev-v1alpha1-buildkitbuildertemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=builder-template.builder-hub.dev,resources=buildkitbuildertemplates,verbs=create;update,versions=v1alpha1,name=mbuildkitbuildertemplate.kb.io,admissionReviewVersions=v1

var _ admission.Defaulter[*BuildkitBuilderTemplate] = (*BuildkitBuilderTemplate)(nil)

func (r *BuildkitBuilderTemplate) Default(ctx context.Context, obj *BuildkitBuilderTemplate) error {
	buildkitbuildertemplatelog.Info("default", "name", obj.Name)
	if obj.Spec.BuildkitImage == "" {
		if obj.Spec.Rootless {
			obj.Spec.BuildkitImage = "moby/buildkit:master-rootless"
		} else {
			obj.Spec.BuildkitImage = "moby/buildkit:master"
		}
	}
	if obj.Spec.CacheConfig.Type == CacheTypePVC && obj.Spec.CacheConfig.PVC != nil {
		if len(obj.Spec.CacheConfig.PVC.AccessModes) == 0 {
			obj.Spec.CacheConfig.PVC.AccessModes = []string{string(corev1.ReadWriteOnce)}
		}
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-builder-template-builder-hub-dev-v1alpha1-buildkitbuildertemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=builder-template.builder-hub.dev,resources=buildkitbuildertemplates,verbs=create;update,versions=v1alpha1,name=vbuildkitbuildertemplate.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*BuildkitBuilderTemplate] = (*BuildkitBuilderTemplate)(nil)

func (r *BuildkitBuilderTemplate) ValidateCreate(ctx context.Context, obj *BuildkitBuilderTemplate) (admission.Warnings, error) {
	buildkitbuildertemplatelog.Info("validate create", "name", obj.Name)
	return nil, obj.validate()
}

func (r *BuildkitBuilderTemplate) ValidateUpdate(ctx context.Context, old, new *BuildkitBuilderTemplate) (admission.Warnings, error) {
	buildkitbuildertemplatelog.Info("validate update", "name", new.Name)
	return nil, new.validate()
}

func (r *BuildkitBuilderTemplate) ValidateDelete(ctx context.Context, obj *BuildkitBuilderTemplate) (admission.Warnings, error) {
	buildkitbuildertemplatelog.Info("validate delete", "name", obj.Name)
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
