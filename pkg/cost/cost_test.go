package cost

import (
	"context"
	"testing"

	"github.com/vjranagit/cluster-api/pkg/api"
)

func TestEstimator_EstimateCost(t *testing.T) {
	estimator := NewEstimator()

	tests := []struct {
		name          string
		spec          api.ClusterSpec
		wantMinCost   float64
		wantMaxCost   float64
		wantBreakdown int
	}{
		{
			name: "small AWS cluster",
			spec: api.ClusterSpec{
				Provider: "aws",
				Region:   "us-west-2",
				ControlPlane: api.ControlPlaneSpec{
					Type:    api.ControlPlaneManaged,
					Version: "1.28",
				},
				WorkerPools: []api.WorkerPoolSpec{
					{
						Name:         "general",
						InstanceType: "t3.medium",
						MinSize:      1,
						MaxSize:      3,
						DesiredSize:  2,
					},
				},
				Network: api.NetworkSpec{
					NATGateway: true,
					AvailabilityZones: []string{"us-west-2a"},
				},
			},
			wantMinCost:   100.0,  // At least $100/month
			wantMaxCost:   500.0,  // No more than $500/month
			wantBreakdown: 4,      // Control plane + workers + NAT + LB
		},
		{
			name: "large Azure cluster with spot",
			spec: api.ClusterSpec{
				Provider: "azure",
				Region:   "eastus",
				ControlPlane: api.ControlPlaneSpec{
					Type:    api.ControlPlaneManaged,
					Version: "1.28",
				},
				WorkerPools: []api.WorkerPoolSpec{
					{
						Name:         "spot-pool",
						InstanceType: "Standard_D4s_v3",
						MinSize:      3,
						MaxSize:      10,
						DesiredSize:  5,
						Spot: &api.SpotConfig{
							Enabled: true,
						},
					},
				},
				Network: api.NetworkSpec{
					NATGateway: false,
					AvailabilityZones: []string{"1"},
				},
			},
			wantMinCost:   200.0,
			wantMaxCost:   1000.0,
			wantBreakdown: 2, // Workers + LB (no NAT, AKS CP is free)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			estimate, err := estimator.EstimateCost(ctx, tt.spec)

			if err != nil {
				t.Errorf("EstimateCost() error = %v", err)
				return
			}

			if estimate.TotalMonthlyCost < tt.wantMinCost {
				t.Errorf("EstimateCost() cost too low: $%.2f, want >= $%.2f",
					estimate.TotalMonthlyCost, tt.wantMinCost)
			}

			if estimate.TotalMonthlyCost > tt.wantMaxCost {
				t.Errorf("EstimateCost() cost too high: $%.2f, want <= $%.2f",
					estimate.TotalMonthlyCost, tt.wantMaxCost)
			}

			if len(estimate.Breakdown) != tt.wantBreakdown {
				t.Errorf("EstimateCost() got %d breakdown items, want %d",
					len(estimate.Breakdown), tt.wantBreakdown)
			}

			if estimate.TotalHourlyCost <= 0 {
				t.Error("EstimateCost() hourly cost should be positive")
			}

			if estimate.Currency != "USD" {
				t.Errorf("EstimateCost() currency = %s, want USD", estimate.Currency)
			}
		})
	}
}

func TestEstimator_SpotSavings(t *testing.T) {
	estimator := NewEstimator()
	pricing := PricingData{
		InstanceTypes: map[string]InstancePrice{
			"t3.medium": {OnDemandHourly: 0.0416, SpotHourly: 0.0125},
		},
	}

	spec := api.ClusterSpec{
		WorkerPools: []api.WorkerPoolSpec{
			{
				Name:         "on-demand-pool",
				InstanceType: "t3.medium",
				MinSize:      2,
				MaxSize:      5,
				DesiredSize:  3,
			},
		},
	}

	savings := estimator.calculateSpotSavings(spec, pricing)
	if savings <= 0 {
		t.Error("calculateSpotSavings() should show savings for on-demand instances")
	}

	// With spot enabled, savings should be 0
	spec.WorkerPools[0].Spot = &api.SpotConfig{Enabled: true}
	savings = estimator.calculateSpotSavings(spec, pricing)
	if savings != 0 {
		t.Error("calculateSpotSavings() should be 0 when already using spot")
	}
}

func TestFormatEstimate(t *testing.T) {
	estimate := &CostEstimate{
		TotalMonthlyCost: 250.50,
		TotalHourlyCost:  0.343,
		Currency:         "USD",
		Breakdown: []CostBreakdown{
			{
				Resource: api.ResourceID{
					Kind: "ControlPlane",
					Name: "managed-cp",
				},
				ResourceType: ResourceManagedK8s,
				Quantity:     1,
				UnitCost:     0.10,
				HourlyCost:   0.10,
				MonthlyCost:  73.0,
				Details:      "EKS control plane",
			},
		},
	}

	output := FormatEstimate(estimate)
	if output == "" {
		t.Error("FormatEstimate() returned empty string")
	}

	if !contains(output, "250.50") {
		t.Error("FormatEstimate() missing total cost")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0
}
