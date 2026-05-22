package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"
)

type WorkloadPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sustainability.susk8s,resources=workloadpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sustainability.susk8s,resources=workloadpolicies/status,verbs=get;update;patch
func (r *WorkloadPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var policy sustainabilityv1alpha1.WorkloadPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var depList appsv1.DeploymentList
	if err := r.List(ctx, &depList, client.InNamespace(policy.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	matchedCount := int32(0)
	for _, dep := range depList.Items {

		if r.matchesPolicy(&dep, &policy) {
			matchedCount++
			needsUpdate := false

			// if dep.Spec.Template.Spec.SchedulerName != "susk8s-scheduler" {
			// 	dep.Spec.Template.Spec.SchedulerName = "susk8s-scheduler"
			// 	needsUpdate = true
			// }

			if dep.Spec.Template.Annotations == nil {
				dep.Spec.Template.Annotations = make(map[string]string)
			}

			if needsUpdate {
				if err := r.Update(ctx, &dep); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	policy.Status.Enforced = true
	policy.Status.MatchedWorkloads = matchedCount
	return ctrl.Result{}, r.Status().Update(ctx, &policy)
}

func (r *WorkloadPolicyReconciler) matchesPolicy(dep *appsv1.Deployment, policy *sustainabilityv1alpha1.WorkloadPolicy) bool {
	if len(policy.Spec.Target.MatchLabels) == 0 {
		return false
	}

	for key, val := range policy.Spec.Target.MatchLabels {
		if depVal, exists := dep.Labels[key]; !exists || depVal != val {
			return false
		}
	}
	return true
}

func (r *WorkloadPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sustainabilityv1alpha1.WorkloadPolicy{}).
		Named("workloadpolicy").
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
