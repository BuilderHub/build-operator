package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	buildkitv1alpha1 "github.com/builderhub/build-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// nodePortServiceForBuilder returns a NodePort Service for external access (no port-forward needed).
func nodePortServiceForBuilder(b *buildkitv1alpha1.BuildkitBuilder) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.Namespace,
			Name:      fmt.Sprintf("builder-%s-client", b.Name),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labelsForBuilder(b, string(b.Spec.Mode)),
			Ports: []corev1.ServicePort{
				{Name: "buildkit", Port: 1234, TargetPort: intstr.FromInt(1234), NodePort: 0},
				{Name: "metrics", Port: 1235, TargetPort: intstr.FromInt(1235), NodePort: 0},
			},
		},
	}
}

// headlessServiceForBuilder returns a headless Service for the StatefulSet.
func headlessServiceForBuilder(b *buildkitv1alpha1.BuildkitBuilder) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.Namespace,
			Name:      fmt.Sprintf("builder-%s", b.Name),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labelsForBuilder(b, string(b.Spec.Mode)),
			Ports: []corev1.ServicePort{
				{Name: "buildkit", Port: 1234, TargetPort: intstr.FromInt(1234)},
				{Name: "metrics", Port: 1235, TargetPort: intstr.FromInt(1235)},
			},
		},
	}
}

// pvcForBuilder returns a PVC for the builder cache (stable name: builder-<name>-cache).
func pvcForBuilder(b *buildkitv1alpha1.BuildkitBuilder, spec *buildkitv1alpha1.BuildkitBuilderTemplateSpec) *corev1.PersistentVolumeClaim {
	pvc := spec.CacheConfig.PVC
	size := pvc.Size
	if size == "" {
		size = "100Gi"
	}
	accessModes := pvc.AccessModes
	if len(accessModes) == 0 {
		accessModes = []string{string(corev1.ReadWriteOnce)}
	}
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.Namespace,
			Name:      fmt.Sprintf("builder-%s-cache", b.Name),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      toAccessModes(accessModes),
			StorageClassName: strPtr(pvc.StorageClassName),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}
}

func toAccessModes(ss []string) []corev1.PersistentVolumeAccessMode {
	out := make([]corev1.PersistentVolumeAccessMode, len(ss))
	for i, s := range ss {
		out[i] = corev1.PersistentVolumeAccessMode(s)
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// statefulSetForBuilder returns a StatefulSet for persistent/sleepy mode.
// Uses a pre-created PVC (builder-<name>-cache) for cache; StatefulSet pod mounts it.
func (r *BuildkitBuilderReconciler) statefulSetForBuilder(b *buildkitv1alpha1.BuildkitBuilder, spec *buildkitv1alpha1.BuildkitBuilderTemplateSpec, replicas int32) *appsv1.StatefulSet {
	var pvcName *string
	if spec.CacheConfig.Type == buildkitv1alpha1.CacheTypePVC && spec.CacheConfig.PVC != nil {
		n := fmt.Sprintf("builder-%s-cache", b.Name)
		pvcName = &n
	}
	podSpec := r.buildPodSpec(spec, b, pvcName)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.Namespace,
			Name:      fmt.Sprintf("builder-%s", b.Name),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: fmt.Sprintf("builder-%s", b.Name),
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForBuilder(b, string(b.Spec.Mode)),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsForBuilder(b, string(b.Spec.Mode)),
				},
				Spec: podSpec,
			},
		},
	}
	_ = controllerutil.SetControllerReference(b, sts, r.Scheme)
	return sts
}

// createOrUpdateService creates or updates a Service.
func (r *BuildkitBuilderReconciler) createOrUpdateService(ctx context.Context, svc *corev1.Service) error {
	existing := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKeyFromObject(svc), existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, svc)
		}
		return err
	}
	svc.ResourceVersion = existing.ResourceVersion
	svc.Spec.ClusterIP = existing.Spec.ClusterIP
	return r.Update(ctx, svc)
}

// createOrUpdatePVC creates or updates a PVC (only if it doesn't exist - PVCs are mostly immutable).
func (r *BuildkitBuilderReconciler) createOrUpdatePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	existing := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKeyFromObject(pvc), existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, pvc)
		}
		return err
	}
	// PVC spec is immutable after creation; only update metadata if needed
	return nil
}

// createOrUpdateStatefulSet creates or updates a StatefulSet.
func (r *BuildkitBuilderReconciler) createOrUpdateStatefulSet(ctx context.Context, sts *appsv1.StatefulSet) error {
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, client.ObjectKeyFromObject(sts), existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, sts)
		}
		return err
	}
	sts.ResourceVersion = existing.ResourceVersion
	return r.Update(ctx, sts)
}

// resolveNodePort returns the allocated NodePort from the builder-<name>-client Service.
func (r *BuildkitBuilderReconciler) resolveNodePort(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder) int32 {
	var svc corev1.Service
	err := r.Get(ctx, client.ObjectKey{Namespace: b.Namespace, Name: fmt.Sprintf("builder-%s-client", b.Name)}, &svc)
	if err != nil {
		return 0
	}
	for _, p := range svc.Spec.Ports {
		if p.Name == "buildkit" && p.NodePort > 0 {
			return p.NodePort
		}
	}
	return 0
}

// resolveStatefulSetEndpoint returns the TCP endpoint and ready replica count from the StatefulSet pods.
func (r *BuildkitBuilderReconciler) resolveStatefulSetEndpoint(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder) (endpoint string, readyReplicas int32, err error) {
	var pods corev1.PodList
	if err = r.List(ctx, &pods, client.InNamespace(b.Namespace), client.MatchingLabels{
		"builder-hub.dev/builder": b.Name,
	}); err != nil {
		return "", 0, err
	}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning && p.Status.PodIP != "" {
			readyReplicas++
			if endpoint == "" {
				endpoint = fmt.Sprintf("tcp://%s:1234", p.Status.PodIP)
			}
		}
	}
	return endpoint, readyReplicas, nil
}

// updateStatus updates the BuildkitBuilder status.
func (r *BuildkitBuilderReconciler) updateStatus(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder, phase, endpoint string, readyReplicas, desiredReplicas int32, nodePort int32) (ctrl.Result, error) {
	b.Status.Phase = phase
	b.Status.Endpoint = endpoint
	b.Status.NodePort = nodePort
	b.Status.ReadyReplicas = readyReplicas
	b.Status.DesiredReplicas = desiredReplicas
	setCondition(&b.Status, buildkitv1alpha1.ConditionReady, phase == "Ready", "Builder ready")
	setCondition(&b.Status, buildkitv1alpha1.ConditionEndpoint, endpoint != "", "Endpoint set")
	if err := r.Status().Update(ctx, b); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// updateStatusWithLastScaled updates status including LastScaledAt.
func (r *BuildkitBuilderReconciler) updateStatusWithLastScaled(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder, phase, endpoint string, readyReplicas, desiredReplicas int32, nodePort int32, lastScaled *metav1.Time) (ctrl.Result, error) {
	b.Status.Phase = phase
	b.Status.Endpoint = endpoint
	b.Status.NodePort = nodePort
	b.Status.ReadyReplicas = readyReplicas
	b.Status.DesiredReplicas = desiredReplicas
	b.Status.LastScaledAt = lastScaled
	setCondition(&b.Status, buildkitv1alpha1.ConditionReady, phase == "Ready", "Builder ready")
	setCondition(&b.Status, buildkitv1alpha1.ConditionLastScaled, true, "Last scaled")
	if err := r.Status().Update(ctx, b); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func setCondition(status *buildkitv1alpha1.BuildkitBuilderStatus, condType string, statusVal bool, message string) {
	statusStr := "False"
	if statusVal {
		statusStr = "True"
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == condType {
			status.Conditions[i].Status = metav1.ConditionStatus(statusStr)
			status.Conditions[i].Message = message
			status.Conditions[i].LastTransitionTime = metav1.Now()
			return
		}
	}
	status.Conditions = append(status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             metav1.ConditionStatus(statusStr),
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// reconcileDelete runs finalizers on CR delete (ephemeral: clean up PVCs).
func (r *BuildkitBuilderReconciler) reconcileDelete(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(b, FinalizerPVC) {
		// Delete any ephemeral PVCs owned by this builder
		// (in our design, ephemeral rarely uses PVC; mostly emptyDir)
		controllerutil.RemoveFinalizer(b, FinalizerPVC)
		if err := r.Update(ctx, b); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
