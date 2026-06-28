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
	defaultExposure(obj)
	return nil
}

func defaultExposure(obj *BuildkitBuilder) {
	if obj.Spec.Exposure == nil || !obj.Spec.Exposure.Enabled {
		return
	}
	if obj.Spec.Exposure.IngressController == "" {
		obj.Spec.Exposure.IngressController = IngressControllerTraefik
	}
	if obj.Spec.Exposure.EntryPoint == "" {
		// mTLS passthrough always rides the TLS entrypoint.
		obj.Spec.Exposure.EntryPoint = "websecure"
	}
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
	if err := r.validateExposure(); err != nil {
		return err
	}
	return nil
}

func (r *BuildkitBuilder) validateExposure() error {
	if r.Spec.Exposure == nil || !r.Spec.Exposure.Enabled {
		return nil
	}
	if r.Spec.Exposure.Host == "" {
		return fmt.Errorf("exposure.host is required when exposure is enabled")
	}
	switch r.Spec.Exposure.IngressController {
	case "", IngressControllerTraefik:
		// ok
	default:
		return fmt.Errorf("unsupported ingressController %q (only traefik is supported)", r.Spec.Exposure.IngressController)
	}
	return nil
}
