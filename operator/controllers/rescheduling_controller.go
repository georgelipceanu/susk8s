package controllers

import (
	"context"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"
)

type ReschedulingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups=sustainability.susk8s,resources=workloadpolicies,verbs=get;list;watch
func (r *ReschedulingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	var policy sustainabilityv1alpha1.WorkloadPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if policy.Spec.Reschedule == nil || !policy.Spec.Reschedule.Enabled {
		return ctrl.Result{}, nil
	}

	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace(policy.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	evictionLimit := int32(100)
	if policy.Spec.Reschedule.EvictionRateLimit > 0 {
		evictionLimit = policy.Spec.Reschedule.EvictionRateLimit
	}
	evictedCount := int32(0)

	// Find the best available node intensity in the cluster for Proactive evaluation
	bestIntensity, _ := r.findBestNodeIntensity(ctx)

	for _, pod := range podList.Items {

		// Skip pods that are already dying
		if pod.DeletionTimestamp != nil {
			continue
		}

		// only evaluate pods that have a max-carbon annotation injected by the Webhook!
		maxCarbonStr, podHasLimit := pod.Annotations["susk8s.io/max-carbon"]
		if !podHasLimit {
			continue // webhook didn't flag this pod, skip it
		}

		var node corev1.Node
		if err := r.Get(ctx, client.ObjectKey{Name: pod.Spec.NodeName}, &node); err != nil {
			continue
		}

		currentNodeIntensityStr := node.Annotations["susk8s.io/carbon-intensity"]
		currentNodeIntensity, _ := strconv.Atoi(currentNodeIntensityStr)

		// Reactive Bounding looking to see if the current node is dirtier than the pod's max limit
		isViolating := r.isViolatingSustainability(&node, maxCarbonStr)

		// Proactive Optimisation looing to see if there are any cleaner nodes
		isBetterAvailable := bestIntensity < currentNodeIntensity

		enforcementMode := pod.Annotations["susk8s.io/enforcement"]
		shouldEvict := false
		reason := ""

		if enforcementMode == "hard" {
			// HARD MODE: Evict if absolute limit is broken, OR if ANY better node is available (NOT WORKING)
			if isViolating {
				shouldEvict = true
				reason = "Hard Enforcement: Absolute limit violated"
			} else if isBetterAvailable {
				shouldEvict = true
				reason = "Hard Enforcement: Proactive optimisation available (chasing the sun)"
			}
		} else {
			// SOFT MODE: Evict ONLY if the absolute limit is broken (Reactive)
			if isViolating {
				shouldEvict = true
				reason = "Soft Enforcement: Absolute limit violated"
			}
		}

		if shouldEvict {
			if evictedCount >= evictionLimit {
				log.Info("Eviction rate limit reached. Pausing evictions.")
				break
			}

			log.Info("Evicting pod to force green rescheduling...",
				"pod", pod.Name,
				"reason", reason,
				"currentNodeIntensity", currentNodeIntensity,
				"bestAvailableIntensity", bestIntensity,
				"maxAllowed", maxCarbonStr)

			if err := r.evictPod(ctx, &pod); err != nil {
				log.Error(err, "Failed to evict pod", "pod", pod.Name)
				continue
			}
			evictedCount++
			PodEvictionsTotal.WithLabelValues(policy.Name, pod.Namespace).Inc()
		} else {
			// Log why it was skipped for debugging
			if isViolating && enforcementMode != "hard" && enforcementMode != "soft" {
				log.Info("Pod violates limits but enforcement mode is undefined. Defaulting to Soft Mode behavior.", "pod", pod.Name)
			}
		}
	}

	cooldown := time.Duration(30) * time.Second
	if policy.Spec.Reschedule.CooldownSeconds > 0 {
		cooldown = time.Duration(policy.Spec.Reschedule.CooldownSeconds) * time.Second
	}
	return ctrl.Result{RequeueAfter: cooldown}, nil
}

func (r *ReschedulingReconciler) findBestNodeIntensity(ctx context.Context) (int, error) {
	var nodeList corev1.NodeList
	if err := r.List(ctx, &nodeList); err != nil {
		return 0, err
	}

	minIntensity := 9999 // Start with an arbitrarily high number
	for _, node := range nodeList.Items {
		val, ok := node.Annotations["susk8s.io/carbon-intensity"]
		if !ok {
			continue
		}

		intensity, err := strconv.Atoi(val)
		if err == nil && intensity < minIntensity {
			minIntensity = intensity
		}
	}
	return minIntensity, nil
}

func (r *ReschedulingReconciler) evictPod(ctx context.Context, pod *corev1.Pod) error {
	eviction := &v1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	return r.SubResource("eviction").Create(ctx, pod, eviction)
}

func (r *ReschedulingReconciler) isViolatingSustainability(node *corev1.Node, podMaxCarbonStr string) bool {
	nodeCarbonStr, ok := node.Annotations["susk8s.io/carbon-intensity"]
	if !ok {
		return false
	}

	nodeCarbon, err1 := strconv.Atoi(nodeCarbonStr)
	podMaxCarbon, err2 := strconv.Atoi(podMaxCarbonStr)

	if err1 != nil || err2 != nil {
		return false
	}

	return nodeCarbon > podMaxCarbon
}

func (r *ReschedulingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sustainabilityv1alpha1.WorkloadPolicy{}).
		Named("rescheduling").
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.mapNodeToPolicies),
		).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.mapPodToPolicies),
		).
		Complete(r)
}

func (r *ReschedulingReconciler) mapNodeToPolicies(ctx context.Context, obj client.Object) []reconcile.Request {
	policyList := &sustainabilityv1alpha1.WorkloadPolicyList{}
	if err := r.List(ctx, policyList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, policy := range policyList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      policy.Name,
				Namespace: policy.Namespace,
			},
		})
	}
	return requests
}

func (r *ReschedulingReconciler) mapPodToPolicies(ctx context.Context, obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}

	if _, hasLimit := pod.Annotations["susk8s.io/max-carbon"]; !hasLimit {
		return nil
	}

	policyList := &sustainabilityv1alpha1.WorkloadPolicyList{}
	if err := r.List(ctx, policyList, client.InNamespace(pod.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, policy := range policyList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      policy.Name,
				Namespace: policy.Namespace,
			},
		})
	}
	return requests
}
