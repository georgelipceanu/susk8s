package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"
)

type CarbonInfoReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ElectricityMapsMockResponse matches our mock server
type ElectricityMapsMockResponse struct {
	Zone            string `json:"zone"`
	CarbonIntensity int    `json:"carbonIntensity"` // JSON unmarshals to standard int
}

func (r *CarbonInfoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var carbonInfo sustainabilityv1alpha1.CarbonInfo
	if err := r.Get(ctx, req.NamespacedName, &carbonInfo); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var newIntensity int32

	if carbonInfo.Spec.OverrideIntensity > 0 {
		newIntensity = carbonInfo.Spec.OverrideIntensity
		logger.Info("Manual override active, skipping API", "Intensity", newIntensity)
	} else if carbonInfo.Spec.Provider == "mock" {
		// Query our Mock API Server
		url := fmt.Sprintf("http://host.docker.internal:8080/carbon?zone=%s", carbonInfo.Spec.Region)

		resp, err := http.Get(url)
		if err != nil {
			logger.Error(err, "Failed to fetch from Mock Carbon API")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		defer resp.Body.Close()

		var apiData ElectricityMapsMockResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiData); err != nil {
			logger.Error(err, "Failed to decode JSON from Mock API")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		// Explicitly cast the int to int32
		newIntensity = int32(apiData.CarbonIntensity)

	} else if carbonInfo.Spec.Provider == "electricitymaps" {
		url := fmt.Sprintf("https://api-access.electricitymaps.com/free-tier/carbon-intensity/latest?zone=%s", carbonInfo.Spec.Region)

		reqAPI, err := http.NewRequest("GET", url, nil)
		if err != nil {
			logger.Error(err, "Failed to create request")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}

		reqAPI.Header.Add("auth-token", "j7EXFHw2SSRQamh2KsrF")

		clientHTTP := &http.Client{}
		resp, err := clientHTTP.Do(reqAPI)
		apiFailed := false
		if err != nil {
			logger.Error(err, "Network error fetching from ElectricityMaps")
			apiFailed = true
		} else if resp.StatusCode != 200 {
			logger.Error(fmt.Errorf("API returned HTTP %d", resp.StatusCode), "API rejected the request")
			apiFailed = true
		}

		if apiFailed {
			if carbonInfo.Spec.StaticIntensity > 0 {
				logger.Info("Using Static Intensity due to API failure", "Intensity", carbonInfo.Spec.StaticIntensity)
				newIntensity = carbonInfo.Spec.StaticIntensity
			} else {
				meta.SetStatusCondition(&carbonInfo.Status.Conditions, metav1.Condition{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "APIRequestFailed",
					Message: "API request failed and no fallback was provided",
				})
				r.Status().Update(ctx, &carbonInfo)
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
		} else {
			defer resp.Body.Close()
			var apiData struct {
				CarbonIntensity int `json:"carbonIntensity"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&apiData); err != nil {
				logger.Error(err, "Failed to decode ElectricityMaps JSON")
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			newIntensity = int32(apiData.CarbonIntensity)
		}

	}

	if carbonInfo.Status.CurrentIntensity != newIntensity {
		carbonInfo.Status.CurrentIntensity = newIntensity

		now := metav1.Now()
		carbonInfo.Status.LastUpdated = &now

		meta.SetStatusCondition(&carbonInfo.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "Synced",
			Message: fmt.Sprintf("Successfully fetched intensity from %s", carbonInfo.Spec.Provider),
		})

		if err := r.Status().Update(ctx, &carbonInfo); err != nil {
			logger.Error(err, "Failed to update CarbonInfo status")
			return ctrl.Result{}, err
		}
		logger.Info("CarbonInfo updated!", "Region", carbonInfo.Spec.Region, "NewIntensity", newIntensity)

		if err := r.updateSchedulerConfigMap(ctx, &carbonInfo, newIntensity); err != nil {
			logger.Error(err, "Failed to update Scheduler ConfigMap")
		}
	}

	// Automating Node Annotations for the Scheduler
	var nodeList corev1.NodeList
	if err := r.List(ctx, &nodeList, client.MatchingLabels{"topology.kubernetes.io/region": carbonInfo.Spec.Region}); err != nil {
		logger.Error(err, "Failed to list nodes for region", "Region", carbonInfo.Spec.Region)
	} else {
		for _, node := range nodeList.Items {
			if node.Annotations == nil {
				node.Annotations = make(map[string]string)
			}

			node.Annotations["susk8s.io/carbon-intensity"] = fmt.Sprintf("%d", newIntensity)

			if err := r.Update(ctx, &node); err != nil {
				logger.Error(err, "Failed to update node annotation", "Node", node.Name)
			} else {
				logger.Info("Successfully synced live carbon data to Node", "Node", node.Name, "Intensity", newIntensity)
			}
		}
	}

	pollSeconds := carbonInfo.Spec.PollSeconds
	if pollSeconds <= 0 {
		pollSeconds = 60 // Default to 60 seconds if not set
	}

	return ctrl.Result{RequeueAfter: time.Duration(pollSeconds) * time.Second}, nil
}

func (r *CarbonInfoReconciler) updateSchedulerConfigMap(ctx context.Context, ci *sustainabilityv1alpha1.CarbonInfo, intensity int32) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "carbon-intensity-cache",
			Namespace: "kube-system",
		},
	}

	if err := r.Get(ctx, client.ObjectKey{Name: cm.Name, Namespace: cm.Namespace}, cm); err != nil {
		// Create if it doesn't exist
		cm.Data = map[string]string{ci.Spec.Region: fmt.Sprintf("%d", intensity)}
		return r.Create(ctx, cm)
	}

	// Update if it exists
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[ci.Spec.Region] = fmt.Sprintf("%d", intensity)
	return r.Update(ctx, cm)
}

func (r *CarbonInfoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sustainabilityv1alpha1.CarbonInfo{}).
		Complete(r)
}
