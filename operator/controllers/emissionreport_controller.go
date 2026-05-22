package controllers

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"
)

type EmissionReportReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	PromAPI promv1.API
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
func (r *EmissionReportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var report sustainabilityv1alpha1.EmissionReport
	if err := r.Get(ctx, req.NamespacedName, &report); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if meta.IsStatusConditionTrue(report.Status.Conditions, "Finished") {
		logger.Info("Report already finished, skipping", "Report", report.Name)
		return ctrl.Result{}, nil
	}

	// Fetch current Carbon Intensity
	carbonIntensity, err := r.getCarbonIntensity(ctx, report.Spec.Region)
	if err != nil {
		logger.Error(err, "Failed to get carbon intensity cache. Is CarbonInfoController running?")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Calculate the duration of the report in hours
	duration := report.Spec.To.Time.Sub(report.Spec.From.Time)
	hours := duration.Hours()
	if hours <= 0 {
		hours = 1 // prevent divide by zero or negative time
	}

	// convert the duration to a PromQL-friendly string
	window := fmt.Sprintf("%.0fh", hours)
	promQL := fmt.Sprintf(`sum(increase(kepler_node_platform_joules_total{}[%s]))`, window)

	totalJoules, err := r.getPrometheusMetric(ctx, promQL, report.Spec.To.Time)
	if err != nil {
		logger.Error(err, "Failed to fetch Kepler metrics, using fallback")
		totalJoules = 0
	}
	logger.Info("Fetched live hardware energy telemetry", "Query", promQL, "Joules", totalJoules)

	// // NOW DROPPED: Linear Power Model
	// // En = Pidle + Ucpu(Pmax - Pidle)
	// idlePowerW := 50.0 // Watts (Assumed baseline node power)
	// maxPowerW := 200.0 // Watts (Assumed peak node power)

	// Direct eBPF Hardware Measurement
	// 1 Kilowatt-hour (kWh) = 3,600,000 Joules
	energyKWh := totalJoules / 3600000.0

	// Calculate Emissions (gCO2)
	emissionsGCO2 := energyKWh * float64(carbonIntensity)
	report.Status.Totals = sustainabilityv1alpha1.ReportTotals{
		EnergyKWh:     energyKWh,
		EmissionsGCO2: emissionsGCO2,
	}

	breakdownMap := make(map[string]sustainabilityv1alpha1.ReportTotals)
	var breakdownQuery string
	var labelToExtract string

	// Build the correct PromQL query based on the CRD's Scope
	switch report.Spec.Scope {
	case "namespace":
		// Group container joules by namespace
		breakdownQuery = fmt.Sprintf(`sum by (pod_namespace) (increase(kepler_container_joules_total{}[%s]))`, window)
		labelToExtract = "pod_namespace"
	case "deployment":
		// label_replace to strip the ReplicaSet and Pod hashes from the pod_name to add up all identical pods under their parent deployment name.
		breakdownQuery = fmt.Sprintf(`sum by (deployment) (label_replace(increase(kepler_container_joules_total{}[%s]), "deployment", "$1", "pod_name", "(.*)-[^-]+-[^-]+"))`, window)
		labelToExtract = "deployment"
	case "workload":
		// Group container joules by pod name
		breakdownQuery = fmt.Sprintf(`sum by (pod_name) (increase(kepler_container_joules_total{}[%s]))`, window)
		labelToExtract = "pod_name"
	case "node":
		// Group node hardware joules by node instance
		breakdownQuery = fmt.Sprintf(`sum by (instance) (increase(kepler_node_platform_joules_total{}[%s]))`, window)
		labelToExtract = "instance"
	default:
		logger.Info("Unknown scope, defaulting to namespace", "Scope", report.Spec.Scope)
		breakdownQuery = fmt.Sprintf(`sum by (pod_namespace) (increase(kepler_container_joules_total{}[%s]))`, window)
		labelToExtract = "pod_namespace"
	}

	// Fetch the parsed vector array from Prometheus
	breakdownJoules, err := r.getPrometheusBreakdown(ctx, breakdownQuery, report.Spec.To.Time, labelToExtract)
	if err != nil {
		logger.Error(err, "Failed to fetch dynamic breakdown. Leaving map empty.", "Query", breakdownQuery)
	} else {
		// Convert Joules to KWh and gCO2 for each individual entity
		for entityName, joules := range breakdownJoules {
			entityKWh := joules / 3600000.0
			entityGCO2 := entityKWh * float64(carbonIntensity)

			breakdownMap[entityName] = sustainabilityv1alpha1.ReportTotals{
				EnergyKWh:     entityKWh,
				EmissionsGCO2: entityGCO2,
			}
		}
	}

	// Assign the real calculated map to the Status
	report.Status.Breakdown = breakdownMap
	meta.SetStatusCondition(&report.Status.Conditions, metav1.Condition{
		Type:    "Finished",
		Status:  metav1.ConditionTrue,
		Reason:  "CalculationComplete",
		Message: fmt.Sprintf("Report generated using intensity %d gCO2/kWh", carbonIntensity),
	})

	if err := r.Status().Update(ctx, &report); err != nil {
		logger.Error(err, "Failed to update EmissionReport status")
		return ctrl.Result{}, err
	}

	logger.Info("EmissionReport successfully generated!", "EnergyKWh", energyKWh, "EmissionsGCO2", emissionsGCO2)

	return ctrl.Result{}, nil
}

// Helper to read the ConfigMap created by the CarbonInfoController
func (r *EmissionReportReconciler) getCarbonIntensity(ctx context.Context, region string) (int, error) {
	var carbonInfoList sustainabilityv1alpha1.CarbonInfoList

	// List all CarbonInfo objects in the cluster
	if err := r.List(ctx, &carbonInfoList); err != nil {
		return 0, fmt.Errorf("failed to list CarbonInfo resources: %v", err)
	}

	// Loop through them to find the one matching the requested region
	for _, ci := range carbonInfoList.Items {
		if ci.Spec.Region == region {
			// Ensure it has actually synced at least once
			if ci.Status.CurrentIntensity == 0 && ci.Status.LastUpdated == nil {
				return 0, fmt.Errorf("CarbonInfo for region %s exists but hasn't fetched data yet", region)
			}
			return int(ci.Status.CurrentIntensity), nil
		}
	}

	return 0, fmt.Errorf("no CarbonInfo CRD found for region %s", region)
}

func (r *EmissionReportReconciler) getPrometheusMetric(ctx context.Context, query string, evaluateAt time.Time) (float64, error) {
	// Execute the PromQL query
	result, warnings, err := r.PromAPI.Query(ctx, query, evaluateAt)
	if err != nil {
		return 0, fmt.Errorf("error querying Prometheus: %v", err)
	}
	if len(warnings) > 0 {
		log.FromContext(ctx).Info("Prometheus warnings", "warnings", warnings)
	}

	// Parse the result
	vec, ok := result.(model.Vector)
	if !ok || len(vec) == 0 {
		return 0, fmt.Errorf("no data returned from Prometheus for query")
	}

	// Extract the numeric value
	return float64(vec[0].Value), nil
}

// Helper to execute PromQL queries that return multiple grouped results
func (r *EmissionReportReconciler) getPrometheusBreakdown(ctx context.Context, query string, evaluateAt time.Time, labelKey string) (map[string]float64, error) {
	result, warnings, err := r.PromAPI.Query(ctx, query, evaluateAt)
	if err != nil {
		return nil, fmt.Errorf("error querying Prometheus: %v", err)
	}
	if len(warnings) > 0 {
		log.FromContext(ctx).Info("Prometheus warnings", "warnings", warnings)
	}
	vec, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("expected vector result from query")
	}

	breakdown := make(map[string]float64)
	for _, sample := range vec {
		// Extract the label value
		entityName := string(sample.Metric[model.LabelName(labelKey)])
		if entityName == "" {
			// Fallback if Kepler tracked unlabelled background processes
			entityName = "unallocated_system_processes"
		}
		// Map the entity name to its total Joules
		breakdown[entityName] = float64(sample.Value)
	}

	return breakdown, nil
}

func (r *EmissionReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sustainabilityv1alpha1.EmissionReport{}).
		Complete(r)
}
