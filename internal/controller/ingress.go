package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	buildkitv1alpha1 "github.com/builderhub/build-operator/api/v1alpha1"
)

// ingressRouteTCPGVK is the Traefik IngressRouteTCP used for SNI passthrough so
// buildkitd terminates TLS (mTLS) itself.
var ingressRouteTCPGVK = schema.GroupVersionKind{
	Group:   "traefik.io",
	Version: "v1alpha1",
	Kind:    "IngressRouteTCP",
}

// externalDNSHostnameAnnotation lets ExternalDNS (traefik-proxy source) create a record for the route.
const externalDNSHostnameAnnotation = "external-dns.alpha.kubernetes.io/hostname"

func exposureEnabled(b *buildkitv1alpha1.BuildkitBuilder) bool {
	return b.Spec.Exposure != nil && b.Spec.Exposure.Enabled
}

func resolvedExposureEntryPoint(exp *buildkitv1alpha1.ExposureConfig) string {
	if exp.EntryPoint != "" {
		return exp.EntryPoint
	}
	return "websecure"
}

func externalEndpointForBuilder(b *buildkitv1alpha1.BuildkitBuilder) string {
	if !exposureEnabled(b) || b.Spec.Exposure.Host == "" {
		return ""
	}
	// mTLS passthrough always rides the TLS entrypoint (:443 externally).
	return fmt.Sprintf("tcp://%s:443", b.Spec.Exposure.Host)
}

func ingressRouteName(b *buildkitv1alpha1.BuildkitBuilder) string {
	return fmt.Sprintf("builder-%s", b.Name)
}

func ingressRouteForBuilder(b *buildkitv1alpha1.BuildkitBuilder) *unstructured.Unstructured {
	exp := b.Spec.Exposure
	entryPoint := resolvedExposureEntryPoint(exp)

	route := map[string]interface{}{
		"match": fmt.Sprintf("HostSNI(`%s`)", exp.Host),
		"services": []interface{}{
			map[string]interface{}{
				"name": fmt.Sprintf("builder-%s", b.Name),
				"port": int64(1234),
			},
		},
	}

	spec := map[string]interface{}{
		"entryPoints": []interface{}{entryPoint},
		"routes":      []interface{}{route},
		// passthrough: Traefik routes on the TLS SNI and forwards the raw TLS
		// stream to buildkitd, which terminates mTLS itself.
		"tls": map[string]interface{}{
			"passthrough": true,
		},
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": ingressRouteTCPGVK.GroupVersion().String(),
			"kind":       ingressRouteTCPGVK.Kind,
			"metadata": map[string]interface{}{
				"name":      ingressRouteName(b),
				"namespace": b.Namespace,
			},
			"spec": spec,
		},
	}
	// Always advertise the hostname to ExternalDNS (traefik-proxy source). Caller-supplied
	// annotations take precedence so operators can override target, TTL, etc.
	annotations := map[string]string{
		externalDNSHostnameAnnotation: exp.Host,
	}
	for k, v := range exp.Annotations {
		annotations[k] = v
	}
	obj.SetAnnotations(annotations)
	return obj
}

func (r *BuildkitBuilderReconciler) reconcileExposure(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder) error {
	if !exposureEnabled(b) {
		if err := r.deleteIngressRouteIfExists(ctx, b); err != nil {
			return err
		}
		return r.deleteBuilderServerCert(ctx, b)
	}
	if b.Spec.Exposure.IngressController != "" && b.Spec.Exposure.IngressController != buildkitv1alpha1.IngressControllerTraefik {
		return fmt.Errorf("unsupported ingressController %q", b.Spec.Exposure.IngressController)
	}
	// Issue the mTLS server certificate before the pod (which mounts it) is created.
	if _, err := r.ensureBuilderServerCert(ctx, b); err != nil {
		return err
	}
	desired := ingressRouteForBuilder(b)
	if err := controllerutil.SetControllerReference(b, desired, r.Scheme); err != nil {
		return err
	}
	return r.createOrUpdateIngressRoute(ctx, desired)
}

func (r *BuildkitBuilderReconciler) createOrUpdateIngressRoute(ctx context.Context, desired *unstructured.Unstructured) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(ingressRouteTCPGVK)
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

func (r *BuildkitBuilderReconciler) deleteIngressRouteIfExists(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ingressRouteTCPGVK)
	obj.SetName(ingressRouteName(b))
	obj.SetNamespace(b.Namespace)
	err := r.Delete(ctx, obj)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}
