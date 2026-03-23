package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	templatev1alpha1 "github.com/builderhub/build-operator/api/buildertemplate/v1alpha1"
	buildkitv1alpha1 "github.com/builderhub/build-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)


// BuildkitBuilderReconciler reconciles a BuildkitBuilder object
type BuildkitBuilderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=builder-hub.dev,resources=buildkitbuilders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=builder-hub.dev,resources=buildkitbuilders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=builder-hub.dev,resources=buildkitbuilders/finalizers,verbs=update
// +kubebuilder:rbac:groups=builder-template.builder-hub.dev,resources=buildkitbuildertemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// SetupWithManager sets up the controller with the Manager.
func (r *BuildkitBuilderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&buildkitv1alpha1.BuildkitBuilder{}).
		Owns(&corev1.Pod{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

// Reconcile is the main reconciliation loop.
func (r *BuildkitBuilderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var builder buildkitv1alpha1.BuildkitBuilder
	if err := r.Get(ctx, req.NamespacedName, &builder); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve template to get full spec
	spec, err := r.resolveSpec(ctx, &builder)
	if err != nil {
		logger.Error(err, "failed to resolve spec")
		r.Recorder.Event(&builder, corev1.EventTypeWarning, "SpecResolution", err.Error())
		return ctrl.Result{}, err
	}

	// Handle deletion: run finalizers
	if !builder.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &builder)
	}

	// Branch by mode
	switch builder.Spec.Mode {
	case buildkitv1alpha1.BuilderModeEphemeral:
		return r.reconcileEphemeral(ctx, &builder, spec)
	case buildkitv1alpha1.BuilderModePersistent:
		return r.reconcilePersistent(ctx, &builder, spec)
	case buildkitv1alpha1.BuilderModeSleepy:
		return r.reconcileSleepy(ctx, &builder, spec)
	default:
		return ctrl.Result{}, fmt.Errorf("unknown mode: %s", builder.Spec.Mode)
	}
}

// resolveSpec merges template (if ref) with inline overrides.
func (r *BuildkitBuilderReconciler) resolveSpec(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder) (*templatev1alpha1.BuildkitBuilderTemplateSpec, error) {
	var base *templatev1alpha1.BuildkitBuilderTemplateSpec
	if b.Spec.TemplateRef != nil {
		var tmpl templatev1alpha1.BuildkitBuilderTemplate
		if err := r.Get(ctx, client.ObjectKey{Name: *b.Spec.TemplateRef}, &tmpl); err != nil {
			return nil, fmt.Errorf("template %s not found: %w", *b.Spec.TemplateRef, err)
		}
		base = &tmpl.Spec
	}
	if b.Spec.Template != nil {
		merged := mergeSpec(base, b.Spec.Template)
		base = &merged
	}
	if base == nil {
		return nil, fmt.Errorf("either templateRef or template must be set")
	}
	return base, nil
}

func mergeSpec(base, override *templatev1alpha1.BuildkitBuilderTemplateSpec) templatev1alpha1.BuildkitBuilderTemplateSpec {
	var out templatev1alpha1.BuildkitBuilderTemplateSpec
	if base != nil {
		out = *base.DeepCopy()
	}
	if override == nil {
		return out
	}
	if override.BuildkitImage != "" {
		out.BuildkitImage = override.BuildkitImage
	}
	out.Rootless = override.Rootless
	if len(override.Resources.Limits) > 0 || len(override.Resources.Requests) > 0 {
		out.Resources = override.Resources
	}
	if override.NodeSelector != nil {
		out.NodeSelector = override.NodeSelector
	}
	if override.Tolerations != nil {
		out.Tolerations = override.Tolerations
	}
	if override.Affinity != nil {
		out.Affinity = override.Affinity
	}
	if override.BuildkitdToml != "" {
		out.BuildkitdToml = override.BuildkitdToml
	}
	if override.CacheConfig.Type != "" {
		out.CacheConfig = override.CacheConfig
	}
	if override.Arch != "" {
		out.Arch = override.Arch
	}
	return out
}

// ---------------------------------------------------------------------------
// EPHEMERAL MODE
// ---------------------------------------------------------------------------
// Creates a plain Pod (not Deployment) named builder-<name>-<rand>, ownerReference
// to the CR so it deletes automatically. Mount emptyDir or PVC if requested.
// Expose tcp://<podIP>:1234 in .status.endpoint. Add preStop hook for graceful shutdown.
func (r *BuildkitBuilderReconciler) reconcileEphemeral(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder, spec *templatev1alpha1.BuildkitBuilderTemplateSpec) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling ephemeral builder")

	// Ensure buildkitd ConfigMap exists
	if err := r.ensureBuildkitdConfigMap(ctx, b, spec); err != nil {
		return ctrl.Result{}, err
	}

	// For ephemeral we typically want one pod. If one already exists and is running, update status.
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(b.Namespace), client.MatchingLabels{
		LabelKeyBuilderName: b.Name,
		LabelKeyBuilderMode: "ephemeral",
	}); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure NodePort Service for external access (no port-forward)
	svc := nodePortServiceForBuilder(b)
	if err := r.createOrUpdateService(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}

	// If no pod exists, create one
	if len(pods.Items) == 0 {
		pod, err := r.buildEphemeralPod(b, spec)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := controllerutil.SetControllerReference(b, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, pod); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(b, corev1.EventTypeNormal, "Created", "Created ephemeral build pod")
		return ctrl.Result{Requeue: true}, nil
	}

	// Update status from existing pod
	pod := &pods.Items[0]
	endpoint := ""
	phase := "Pending"
	if pod.Status.PodIP != "" {
		endpoint = fmt.Sprintf("tcp://%s:1234", pod.Status.PodIP)
	}
	if pod.Status.Phase == corev1.PodRunning {
		phase = "Ready"
	}
	return r.updateStatus(ctx, b, phase, endpoint, 1, 1, r.resolveNodePort(ctx, b))
}

func (r *BuildkitBuilderReconciler) buildEphemeralPod(b *buildkitv1alpha1.BuildkitBuilder, spec *templatev1alpha1.BuildkitBuilderTemplateSpec) (*corev1.Pod, error) {
	name := fmt.Sprintf("builder-%s-%s", b.Name, randomSuffix())
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   b.Namespace,
			Name:        name,
			Labels:      labelsForBuilder(b, "ephemeral"),
			Annotations: b.Spec.Labels,
		},
		Spec: r.buildPodSpec(spec, b, nil),
	}
	// preStop hook for graceful shutdown
	pod.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-c", "sleep 5"},
			},
		},
	}
	return pod, nil
}

// ---------------------------------------------------------------------------
// PERSISTENT MODE
// ---------------------------------------------------------------------------
// StatefulSet (headless service) + PVC if cacheConfig.type == pvc.
// PVC name stable: builder-<name>-cache. Pod always ready.
func (r *BuildkitBuilderReconciler) reconcilePersistent(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder, spec *templatev1alpha1.BuildkitBuilderTemplateSpec) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling persistent builder")

	// Ensure buildkitd ConfigMap exists
	if err := r.ensureBuildkitdConfigMap(ctx, b, spec); err != nil {
		return ctrl.Result{}, err
	}

	replicas := int32(1)
	if b.Spec.Replicas != nil {
		replicas = *b.Spec.Replicas
	}

	// Ensure headless Service (required by StatefulSet)
	svc := headlessServiceForBuilder(b)
	if err := r.createOrUpdateService(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}
	// Ensure NodePort Service for external access
	clientSvc := nodePortServiceForBuilder(b)
	if err := r.createOrUpdateService(ctx, clientSvc); err != nil {
		return ctrl.Result{}, err
	}

	// Create PVC if cache type is pvc
	if spec.CacheConfig.Type == templatev1alpha1.CacheTypePVC && spec.CacheConfig.PVC != nil {
		pvc := pvcForBuilder(b, spec)
		if err := controllerutil.SetControllerReference(b, pvc, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.createOrUpdatePVC(ctx, pvc); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Create or update StatefulSet
	sts := r.statefulSetForBuilder(b, spec, replicas)
	if err := controllerutil.SetControllerReference(b, sts, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdateStatefulSet(ctx, sts); err != nil {
		return ctrl.Result{}, err
	}

	// Resolve endpoint from StatefulSet pod
	endpoint, readyReplicas, err := r.resolveStatefulSetEndpoint(ctx, b)
	if err != nil {
		return ctrl.Result{}, err
	}
	if endpoint == "" {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return r.updateStatus(ctx, b, "Ready", endpoint, readyReplicas, replicas, r.resolveNodePort(ctx, b))
}

// ---------------------------------------------------------------------------
// SLEEPY MODE
// ---------------------------------------------------------------------------
// Same StatefulSet + PVC as persistent, but controller actively scales replicas between 0 and 1.
// Uses builder.builder-hub.dev/last-used annotation (RFC3339) patched by BuilderHub API when build starts.
// If (now - lastUsed > idleTimeout) && no active builds, scale StatefulSet to 0.
// When new build comes in, scale to 1 (PVC re-attaches → cache instantly available).
func (r *BuildkitBuilderReconciler) reconcileSleepy(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder, spec *templatev1alpha1.BuildkitBuilderTemplateSpec) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling sleepy builder")

	// Ensure buildkitd ConfigMap exists
	if err := r.ensureBuildkitdConfigMap(ctx, b, spec); err != nil {
		return ctrl.Result{}, err
	}

	idleTimeout := int32(300)
	if b.Spec.IdleTimeoutSeconds != nil {
		idleTimeout = *b.Spec.IdleTimeoutSeconds
	}

	// Ensure headless Service (required by StatefulSet)
	svc := headlessServiceForBuilder(b)
	if err := r.createOrUpdateService(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}
	// Ensure NodePort Service for external access
	clientSvc := nodePortServiceForBuilder(b)
	if err := r.createOrUpdateService(ctx, clientSvc); err != nil {
		return ctrl.Result{}, err
	}

	// PVC for cache
	if spec.CacheConfig.Type == templatev1alpha1.CacheTypePVC && spec.CacheConfig.PVC != nil {
		pvc := pvcForBuilder(b, spec)
		if err := controllerutil.SetControllerReference(b, pvc, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.createOrUpdatePVC(ctx, pvc); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Determine desired replicas
	lastUsedStr := b.Annotations[AnnotationLastUsed]
	desiredReplicas := int32(0)
	if lastUsedStr != "" {
		lastUsed, err := time.Parse(time.RFC3339, lastUsedStr)
		if err == nil {
			if time.Since(lastUsed) < time.Duration(idleTimeout)*time.Second {
				desiredReplicas = 1
			}
		}
	}
	// If there's no last-used annotation, scale up to 1 so the builder is available
	if lastUsedStr == "" {
		desiredReplicas = 1
	}

	sts := r.statefulSetForBuilder(b, spec, desiredReplicas)
	if err := controllerutil.SetControllerReference(b, sts, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdateStatefulSet(ctx, sts); err != nil {
		return ctrl.Result{}, err
	}

	phase := "ScalingDown"
	if desiredReplicas == 1 {
		phase = "Ready"
	}

	endpoint := ""
	readyReplicas := int32(0)
	if desiredReplicas == 1 {
		ep, ready, err := r.resolveStatefulSetEndpoint(ctx, b)
		if err != nil {
			return ctrl.Result{}, err
		}
		endpoint = ep
		readyReplicas = ready
	}

	now := metav1.Now()
	_, err := r.updateStatusWithLastScaled(ctx, b, phase, endpoint, readyReplicas, desiredReplicas, r.resolveNodePort(ctx, b), &now)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Requeue to re-check idle timeout
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// ---------------------------------------------------------------------------
// HELPERS
// ---------------------------------------------------------------------------

// validLabelValue matches Kubernetes label value: alphanumeric, '-', '_', '.', empty or single segment.
var validLabelValueRegex = regexp.MustCompile(`^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$`)

func labelsForBuilder(b *buildkitv1alpha1.BuildkitBuilder, mode string) map[string]string {
	m := map[string]string{
		LabelKeyBuilderName: b.Name,
		LabelKeyBuilderMode: mode,
	}
	for k, v := range b.Spec.Labels {
		if validLabelValueRegex.MatchString(v) {
			m[k] = v
		}
		// Skip labels with invalid values (e.g. platform: "linux/amd64") so Service selector stays valid
	}
	return m
}

func (r *BuildkitBuilderReconciler) buildPodSpec(spec *templatev1alpha1.BuildkitBuilderTemplateSpec, b *buildkitv1alpha1.BuildkitBuilder, pvcName *string) corev1.PodSpec {
	securityContext := &corev1.PodSecurityContext{}
	if spec.Rootless {
		securityContext.RunAsNonRoot = ptr(true)
		securityContext.RunAsUser = ptr(int64(1000))
	}

	nodeSelector := map[string]string{}
	if spec.Arch != "" {
		nodeSelector["kubernetes.io/arch"] = spec.Arch
	}
	for k, v := range spec.NodeSelector {
		nodeSelector[k] = v
	}

	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}

	// buildkitd config - ConfigMap created by ensureBuildkitdConfigMap in reconcile
	volumes = append(volumes, corev1.Volume{
		Name: "buildkitd-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("builder-%s-buildkitd", b.Name),
				},
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: "buildkitd-config", MountPath: "/etc/buildkit"})

	// Cache volume (PVC or emptyDir)
	if spec.CacheConfig.Type == templatev1alpha1.CacheTypePVC && pvcName != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "cache",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: *pvcName,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: "cache", MountPath: "/var/lib/buildkit"})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name:         "cache",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: "cache", MountPath: "/var/lib/buildkit"})
	}

	// S3 credentials (projected)
	if spec.CacheConfig.Type == templatev1alpha1.CacheTypeS3 && spec.CacheConfig.S3 != nil && spec.CacheConfig.S3.SecretRef != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "s3-creds",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: *spec.CacheConfig.S3.SecretRef,
								Items: []corev1.KeyToPath{
									{Key: "credentials", Path: "credentials"},
								},
							},
						},
					},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: "s3-creds", MountPath: "/etc/buildkit/s3-creds", ReadOnly: true})
	}

	return corev1.PodSpec{
		SecurityContext:  securityContext,
		NodeSelector:     nodeSelector,
		Tolerations:      spec.Tolerations,
		Affinity:         spec.Affinity,
		Volumes:          volumes,
		RestartPolicy:    corev1.RestartPolicyAlways,
		ImagePullSecrets: []corev1.LocalObjectReference{},
		Containers: []corev1.Container{
			{
				Name:  "buildkitd",
				Image: spec.BuildkitImage,
				Args:  []string{"--config", "/etc/buildkit/buildkitd.toml"},
				Ports: []corev1.ContainerPort{
					{Name: "buildkit", ContainerPort: 1234},
					{Name: "metrics", ContainerPort: 1235},
				},
				Resources:    spec.Resources,
				VolumeMounts: volumeMounts,
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(1234)},
					},
					InitialDelaySeconds: 10,
					PeriodSeconds:       10,
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(1234)},
					},
					InitialDelaySeconds: 5,
					PeriodSeconds:       5,
				},
			},
		},
	}
}

func defaultBuildkitdToml() string {
	// Operator injects tcp listener on 1234 + optional TLS via buildkitdToml in spec
	return `[grpc]
  address = [ "tcp://0.0.0.0:1234" ]

[worker.containerd]
  enabled = false

[worker.oci]
  enabled = true

[registry]
  [registry."docker.io"]
    mirrors = ["docker.io"]
`
}

// ensureBuildkitdConfigMap creates/updates the ConfigMap with buildkitd.toml (tcp listener on 1234 + TLS if configured).
func (r *BuildkitBuilderReconciler) ensureBuildkitdConfigMap(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder, spec *templatev1alpha1.BuildkitBuilderTemplateSpec) error {
	buildkitdToml := spec.BuildkitdToml
	if buildkitdToml == "" {
		buildkitdToml = defaultBuildkitdToml()
	}
	if !strings.Contains(buildkitdToml, "tcp://") {
		buildkitdToml = "[grpc]\n  address = [ \"tcp://0.0.0.0:1234\" ]\n\n" + buildkitdToml
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.Namespace,
			Name:      fmt.Sprintf("builder-%s-buildkitd", b.Name),
		},
		Data: map[string]string{
			"buildkitd.toml": buildkitdToml,
		},
	}
	if err := controllerutil.SetControllerReference(b, cm, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKeyFromObject(cm), existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, cm)
		}
		return err
	}
	existing.Data = cm.Data
	return r.Update(ctx, existing)
}

func ptr[T any](v T) *T { return &v }

func randomSuffix() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:8]
}
