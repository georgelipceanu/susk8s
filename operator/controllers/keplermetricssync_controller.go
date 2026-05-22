package controllers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	promapi "github.com/prometheus/client_golang/api"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type KeplerMetricsSyncReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	PrometheusClient promapi.Client
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch
func (r *KeplerMetricsSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes); err != nil {
		return ctrl.Result{}, err
	}
	totalClusterEmissions := 0.0
	for _, node := range nodes.Items {
		energyValue, err := r.queryPrometheusForNode(ctx, &node) // query Prometheus for metrics this specific node
		if err != nil {
			l.Info("Prometheus telemetry unavailable, skipping node", "node", node.Name, "msg", err.Error())
			continue
		}
		NodeEnergyUsage.WithLabelValues(node.Name).Set(energyValue)
		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}
		carbonStr, exists := node.Annotations["susk8s.io/carbon-intensity"]
		if exists {
			// Convert the annotation string to a float
			if carbonIntensity, err := strconv.ParseFloat(carbonStr, 64); err == nil {
				// (Watts * gCO2/kWh) / 1000 = gCO2/h
				nodeEmissions := (energyValue * carbonIntensity) / 1000.0
				totalClusterEmissions += nodeEmissions
			} else {
				l.Error(err, "Failed to parse carbon intensity annotation", "node", node.Name)
			}
		}

		node.Annotations["susk8s.io/energy-usage"] = fmt.Sprintf("%f", energyValue) // project the value onto Node annotations
		if err := r.Update(ctx, &node); err != nil {
			l.Error(err, "Failed to update Node sustainability labels", "node", node.Name)
			continue
		}
	}

	ClusterRealtimeEmissions.Set(totalClusterEmissions)
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *KeplerMetricsSyncReconciler) queryPrometheusForNode(ctx context.Context, node *corev1.Node) (float64, error) {
	v1api := promv1.NewAPI(r.PrometheusClient)
	nodeIP := ""
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			nodeIP = addr.Address
			break
		}
	}
	query := fmt.Sprintf("sum(rate(kepler_node_platform_joules_total{instance=~\"%s.*\"}[1m]))", nodeIP) // query for power consumption
	result, _, err := v1api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if vector, ok := result.(model.Vector); ok && len(vector) > 0 {
		return float64(vector[0].Value), nil
	}

	return 0, fmt.Errorf("no metrics found for node %s (IP: %s)", node.Name, nodeIP)
}

func (r *KeplerMetricsSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Named("keplermetricssync").
		Complete(r)
}
