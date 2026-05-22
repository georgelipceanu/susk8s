package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"
)

// +kubebuilder:webhook:path=/mutate-core-v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups=core,resources=pods,verbs=create;update,versions=v1,name=mpod.susk8s.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=sustainability.susk8s,resources=workloadpolicies,verbs=get;list;watch

// PodMutator intercepts Pod creation and injects the custom scheduler
type PodMutator struct {
	Client  client.Client
	Decoder admission.Decoder
}

func (m *PodMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	// Decode the incoming Pod from the API Server
	if err := m.Decoder.Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	mutated := false

	// Find the WorkloadPolicy that governs this Pod
	var policyList sustainabilityv1alpha1.WorkloadPolicyList
	if err := m.Client.List(ctx, &policyList, client.InNamespace(req.Namespace)); err == nil {

		for _, policy := range policyList.Items {
			// Check if the policy's matchLabels apply to this pod
			matches := true
			for key, val := range policy.Spec.Target.MatchLabels {
				if pod.Labels[key] != val {
					matches = false
					break
				}
			}

			// 3. If the policy targets this pod, inject the rules!
			if matches && len(policy.Spec.Target.MatchLabels) > 0 {
				pod.Spec.SchedulerName = "susk8s-scheduler"
				mutated = true

				if policy.Spec.MaxCarbonIntensity != nil {
					if pod.Annotations == nil {
						pod.Annotations = make(map[string]string)
					}
					pod.Annotations["susk8s.io/max-carbon"] = fmt.Sprintf("%d", *policy.Spec.MaxCarbonIntensity)

					if policy.Spec.SchedulerHints != nil {
						if policy.Spec.SchedulerHints.UtilisationWeight != nil {
							pod.Annotations["susk8s.io/utilisationWeight"] = fmt.Sprintf("%d", *policy.Spec.SchedulerHints.UtilisationWeight)
						}
						if policy.Spec.SchedulerHints.CarbonWeight != nil {
							pod.Annotations["susk8s.io/carbonWeight"] = fmt.Sprintf("%d", *policy.Spec.SchedulerHints.CarbonWeight)
						}
					}

					if policy.Spec.Enforcement != "" {
						pod.Annotations["susk8s.io/enforcement"] = policy.Spec.Enforcement
					} else {
						// Safe fallback just in case the YAML didn't specify it
						pod.Annotations["susk8s.io/enforcement"] = "soft"
					}
				}
				break
			}
		}
	}

	// If no policy applied, let it pass untouched to the default scheduler
	if !mutated {
		return admission.Allowed("Pod untouched")
	}

	// Create a JSON patch showing the API Server exactly what we changed
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}
