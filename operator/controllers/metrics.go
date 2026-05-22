package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	NodeCarbonIntensity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "susk8s_node_carbon_intensity_gco2_kwh",
			Help: "Current carbon intensity annotated on the worker nodes",
		},
		[]string{"node_name"},
	)

	RegionCarbonIntensity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "susk8s_region_carbon_intensity_gco2_kwh",
			Help: "Current carbon intensity for a specific grid region from CarbonInfo CRDs",
		},
		[]string{"region_name"},
	)

	NodeEnergyUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "susk8s_node_energy_usage_joules",
			Help: "Current energy usage queried from Kepler for the node",
		},
		[]string{"node_name"},
	)

	PodEvictionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "susk8s_pod_evictions_total",
			Help: "Total number of pods evicted due to high carbon intensity",
		},
		[]string{"policy_name", "namespace"},
	)

	ClusterRealtimeEmissions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "susk8s_cluster_realtime_emissions_gco2_h",
			Help: "True real-time carbon emissions of the cluster in gCO2/h calculated by the operator",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(NodeCarbonIntensity, RegionCarbonIntensity, NodeEnergyUsage, PodEvictionsTotal, ClusterRealtimeEmissions)
}
