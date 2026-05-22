package controllers

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

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=sustainability.susk8s,resources=workloadpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sustainability.susk8s,resources=workloadpolicies/status,verbs=get;update;patch
func (r *WorkloadPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the WorkloadPolicy
	var policy sustainabilityv1alpha1.WorkloadPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch all Deployments in the same namespace
	var depList appsv1.DeploymentList
	if err := r.List(ctx, &depList, client.InNamespace(policy.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	matchedCount := int32(0)
	for _, dep := range depList.Items {
		// Check if the Deployment matches the policy
		if r.matchesPolicy(&dep, &policy) {
			matchedCount++
		}
	}

	// Update the Policy Status
	policy.Status.Enforced = true
	policy.Status.MatchedWorkloads = matchedCount
	return ctrl.Result{}, r.Status().Update(ctx, &policy)
}

// matchesPolicy checks if the Deployment's labels match the Policy's target selector
func (r *WorkloadPolicyReconciler) matchesPolicy(dep *appsv1.Deployment, policy *sustainabilityv1alpha1.WorkloadPolicy) bool {
	// If the policy doesn't have MatchLabels specified don't match anything safely
	if len(policy.Spec.Target.MatchLabels) == 0 {
		return false
	}

	// Check if the deployment contains all the labels required by the policy
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
		Owns(&appsv1.Deployment{}). // Tells the manager to watch Deployments owned by this policy
		Complete(r)
}
