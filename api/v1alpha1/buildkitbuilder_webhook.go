package v1alpha1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var buildkitbuilderlog = logf.Log.WithName("buildkitbuilder-resource")

// SetupWebhookWithManager registers the webhook
func (r *BuildkitBuilder) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-builder-hub-dev-v1alpha1-buildkitbuilder,mutating=true,failurePolicy=fail,sideEffects=None,groups=builder-hub.dev,resources=buildkitbuilders,verbs=create;update,versions=v1alpha1,name=mbuildkitbuilder.kb.io,admissionReviewVersions=v1

var _ admission.Defaulter[*BuildkitBuilder] = (*BuildkitBuilder)(nil)

// Default implements admission.Defaulter
func (r *BuildkitBuilder) Default(ctx context.Context, obj *BuildkitBuilder) error {
	buildkitbuilderlog.Info("default", "name", obj.Name)
	if obj.Spec.Replicas == nil {
		one := int32(1)
		obj.Spec.Replicas = &one
	}
	if obj.Spec.Mode == BuilderModeSleepy && obj.Spec.IdleTimeoutSeconds == nil {
		threeHundred := int32(300)
		obj.Spec.IdleTimeoutSeconds = &threeHundred
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-builder-hub-dev-v1alpha1-buildkitbuilder,mutating=false,failurePolicy=fail,sideEffects=None,groups=builder-hub.dev,resources=buildkitbuilders,verbs=create;update,versions=v1alpha1,name=vbuildkitbuilder.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*BuildkitBuilder] = (*BuildkitBuilder)(nil)

// ValidateCreate implements admission.Validator
func (r *BuildkitBuilder) ValidateCreate(ctx context.Context, obj *BuildkitBuilder) (admission.Warnings, error) {
	buildkitbuilderlog.Info("validate create", "name", obj.Name)
	return nil, obj.validate()
}

// ValidateUpdate implements admission.Validator
func (r *BuildkitBuilder) ValidateUpdate(ctx context.Context, old, new *BuildkitBuilder) (admission.Warnings, error) {
	buildkitbuilderlog.Info("validate update", "name", new.Name)
	return nil, new.validate()
}

// ValidateDelete implements admission.Validator
func (r *BuildkitBuilder) ValidateDelete(ctx context.Context, obj *BuildkitBuilder) (admission.Warnings, error) {
	buildkitbuilderlog.Info("validate delete", "name", obj.Name)
	return nil, nil
}

func (r *BuildkitBuilder) validate() error {
	if r.Spec.TemplateRef == nil && r.Spec.Template == nil {
		return fmt.Errorf("either templateRef or template must be set")
	}
	if r.Spec.TemplateRef != nil && *r.Spec.TemplateRef == "" {
		return fmt.Errorf("templateRef cannot be empty")
	}
	return nil
}
