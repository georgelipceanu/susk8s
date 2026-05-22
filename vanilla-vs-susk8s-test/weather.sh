#!/bin/bash
# weather.sh - Simulates an EXTREME 15-minute carbon grid fluctuation

# MAKE SURE THESE MATCH YOUR CLUSTER (vanilla-demo-worker OR susk8s-demo-worker)
NODE_1="susk8s-demo-worker"
NODE_2="susk8s-demo-worker2"
NODE_3="susk8s-demo-worker3"

echo "Starting EXTREME Carbon Weather Simulation..."

# Minute 0: Baseline - All nodes are average (200g)
echo "[00:00] Setting Baseline Grid (200g)..."
kubectl annotate node $NODE_1 $NODE_2 $NODE_3 susk8s.io/carbon-intensity="200" --overwrite
sleep 180

# Minute 3: Node 1 experiences a SEVERE coal spike (800g)
echo "[03:00] WEATHER EVENT: Massive Coal Spike on Node 1 (800g!)"
kubectl annotate node $NODE_1 susk8s.io/carbon-intensity="800" --overwrite
sleep 180

# Minute 6: Node 2 gets pure, surplus solar power (10g)
echo "[06:00] WEATHER EVENT: Node 2 achieves pure Solar (10g!)"
kubectl annotate node $NODE_2 susk8s.io/carbon-intensity="10" --overwrite
sleep 180

# Minute 9: Node 3 experiences a moderate gas spike (400g)
echo "[09:00] WEATHER EVENT: Node 3 gets dirtier (400g)"
kubectl annotate node $NODE_3 susk8s.io/carbon-intensity="400" --overwrite
sleep 180

# Minute 12: Return to baseline 
echo "[12:00] WEATHER EVENT: Grid normalizing (200g)..."
kubectl annotate node $NODE_1 $NODE_2 $NODE_3 susk8s.io/carbon-intensity="200" --overwrite
sleep 180

echo "[15:00] Experiment Complete!"
