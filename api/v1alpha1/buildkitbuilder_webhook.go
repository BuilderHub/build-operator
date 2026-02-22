package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var buildkitbuilderlog = logf.Log.WithName("buildkitbuilder-resource")

// SetupWebhookWithManager registers the webhook
func (r *BuildkitBuilder) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-builder-hub-dev-v1alpha1-buildkitbuilder,mutating=true,failurePolicy=fail,sideEffects=None,groups=builder-hub.dev,resources=buildkitbuilders,verbs=create;update,versions=v1alpha1,name=mbuildkitbuilder.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &BuildkitBuilder{}

// Default implements webhook.Defaulter
func (r *BuildkitBuilder) Default() {
	buildkitbuilderlog.Info("default", "name", r.Name)
	if r.Spec.Replicas == nil {
		one := int32(1)
		r.Spec.Replicas = &one
	}
	if r.Spec.Mode == BuilderModeSleepy && r.Spec.IdleTimeoutSeconds == nil {
		threeHundred := int32(300)
		r.Spec.IdleTimeoutSeconds = &threeHundred
	}
}

// +kubebuilder:webhook:path=/validate-builder-hub-dev-v1alpha1-buildkitbuilder,mutating=false,failurePolicy=fail,sideEffects=None,groups=builder-hub.dev,resources=buildkitbuilders,verbs=create;update,versions=v1alpha1,name=vbuildkitbuilder.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &BuildkitBuilder{}

// ValidateCreate implements webhook.Validator
func (r *BuildkitBuilder) ValidateCreate() (admission.Warnings, error) {
	buildkitbuilderlog.Info("validate create", "name", r.Name)
	return nil, r.validate()
}

// ValidateUpdate implements webhook.Validator
func (r *BuildkitBuilder) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	buildkitbuilderlog.Info("validate update", "name", r.Name)
	return nil, r.validate()
}

// ValidateDelete implements webhook.Validator
func (r *BuildkitBuilder) ValidateDelete() (admission.Warnings, error) {
	buildkitbuilderlog.Info("validate delete", "name", r.Name)
	return nil, nil
}

func (r *BuildkitBuilder) validate() error {
	if r.Spec.TemplateRef == nil && r.Spec.Template == nil {
		return fmt.Errorf("either templateRef or template must be set")
	}
	if r.Spec.TemplateRef != nil && *r.Spec.TemplateRef == "" {
		return fmt.Errorf("templateRef cannot be empty")
	}
	if r.Spec.Mode == BuilderModeEphemeral && r.Spec.Replicas != nil && *r.Spec.Replicas != 1 {
		return fmt.Errorf("replicas is ignored for ephemeral mode")
	}
	return nil
}
