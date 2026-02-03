// Package cost provides cost estimation capabilities
package cost

import (
	"context"
	"fmt"
	"time"

	"github.com/vjranagit/cluster-api/pkg/api"
)

// Estimator calculates estimated infrastructure costs
type Estimator struct {
	pricingData map[string]PricingData
}

// NewEstimator creates a new cost estimator
func NewEstimator() *Estimator {
	return &Estimator{
		pricingData: loadPricingData(),
	}
}

// CostEstimate contains cost estimation results
type CostEstimate struct {
	EstimatedAt       time.Time
	TotalMonthlyCost  float64
	TotalHourlyCost   float64
	Breakdown         []CostBreakdown
	Currency          string
	Assumptions       []string
	Warnings          []string
}

// CostBreakdown shows costs by resource
type CostBreakdown struct {
	Resource     api.ResourceID
	ResourceType ResourceType
	Quantity     int
	UnitCost     float64
	MonthlyCost  float64
	HourlyCost   float64
	Details      string
}

// ResourceType categorizes billable resources
type ResourceType string

const (
	ResourceCompute      ResourceType = "compute"       // EC2, VMs
	ResourceNetwork      ResourceType = "network"       // VPC, VNet, Load Balancers
	ResourceStorage      ResourceType = "storage"       // EBS, Disks
	ResourceManagedK8s   ResourceType = "managed_k8s"   // EKS, AKS control plane
	ResourceDataTransfer ResourceType = "data_transfer" // Bandwidth
)

// PricingData contains pricing information for resources
type PricingData struct {
	Provider      string
	Region        string
	InstanceTypes map[string]InstancePrice
	ManagedK8s    ManagedK8sPrice
	Network       NetworkPrice
	Storage       StoragePrice
}

// InstancePrice contains instance pricing
type InstancePrice struct {
	OnDemandHourly float64
	SpotHourly     float64
	VCPU           int
	MemoryGB       float64
}

// ManagedK8sPrice contains managed Kubernetes pricing
type ManagedK8sPrice struct {
	ControlPlaneHourly float64
	PerNodeHourly      float64
}

// NetworkPrice contains network resource pricing
type NetworkPrice struct {
	LoadBalancerHourly  float64
	NATGatewayHourly    float64
	DataTransferPerGB   float64
}

// StoragePrice contains storage pricing
type StoragePrice struct {
	GP3PerGBMonth float64
	IOPSPerMonth  float64
}

// EstimateCost calculates estimated costs for a cluster configuration
func (e *Estimator) EstimateCost(ctx context.Context, spec api.ClusterSpec) (*CostEstimate, error) {
	estimate := &CostEstimate{
		EstimatedAt: time.Now(),
		Breakdown:   []CostBreakdown{},
		Currency:    "USD",
		Assumptions: []string{
			"Assumes 730 hours per month (24/7 operation)",
			"Prices based on latest public pricing data",
			"Does not include data transfer or storage costs",
		},
	}

	pricing, err := e.getPricing(spec.Provider, spec.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get pricing data: %w", err)
	}

	// Estimate control plane costs
	cpCost := e.estimateControlPlane(spec, pricing)
	estimate.Breakdown = append(estimate.Breakdown, cpCost...)

	// Estimate worker pool costs
	for _, pool := range spec.WorkerPools {
		poolCosts := e.estimateWorkerPool(spec, pool, pricing)
		estimate.Breakdown = append(estimate.Breakdown, poolCosts...)
	}

	// Estimate network costs
	networkCost := e.estimateNetwork(spec, pricing)
	estimate.Breakdown = append(estimate.Breakdown, networkCost...)

	// Calculate totals
	for _, item := range estimate.Breakdown {
		estimate.TotalMonthlyCost += item.MonthlyCost
		estimate.TotalHourlyCost += item.HourlyCost
	}

	// Add warnings for high costs
	if estimate.TotalMonthlyCost > 5000 {
		estimate.Warnings = append(estimate.Warnings,
			fmt.Sprintf("⚠ High monthly cost: $%.2f - consider optimizations", estimate.TotalMonthlyCost))
	}

	// Check for cost optimization opportunities
	spotSavings := e.calculateSpotSavings(spec, pricing)
	if spotSavings > 0 {
		estimate.Warnings = append(estimate.Warnings,
			fmt.Sprintf("💡 Potential savings of $%.2f/month by using spot instances", spotSavings))
	}

	return estimate, nil
}

func (e *Estimator) estimateControlPlane(spec api.ClusterSpec, pricing PricingData) []CostBreakdown {
	var costs []CostBreakdown

	if spec.ControlPlane.Type == api.ControlPlaneManaged {
		// Managed Kubernetes (EKS/AKS)
		hourlyCost := pricing.ManagedK8s.ControlPlaneHourly
		costs = append(costs, CostBreakdown{
			Resource: api.ResourceID{
				Provider: spec.Provider,
				Kind:     "ControlPlane",
				Name:     "managed-control-plane",
			},
			ResourceType: ResourceManagedK8s,
			Quantity:     1,
			UnitCost:     hourlyCost,
			HourlyCost:   hourlyCost,
			MonthlyCost:  hourlyCost * 730,
			Details:      fmt.Sprintf("Managed K8s control plane (%s)", spec.ControlPlane.Version),
		})
	} else {
		// Self-managed control plane
		instancePrice, exists := pricing.InstanceTypes[spec.ControlPlane.InstanceType]
		if !exists {
			instancePrice = InstancePrice{OnDemandHourly: 0.10} // Default estimate
		}

		count := spec.ControlPlane.Count
		if count == 0 {
			count = 1
			if spec.ControlPlane.HA {
				count = 3
			}
		}

		hourlyCost := instancePrice.OnDemandHourly * float64(count)
		costs = append(costs, CostBreakdown{
			Resource: api.ResourceID{
				Provider: spec.Provider,
				Kind:     "ControlPlane",
				Name:     "self-managed-control-plane",
			},
			ResourceType: ResourceCompute,
			Quantity:     count,
			UnitCost:     instancePrice.OnDemandHourly,
			HourlyCost:   hourlyCost,
			MonthlyCost:  hourlyCost * 730,
			Details:      fmt.Sprintf("%d x %s instances", count, spec.ControlPlane.InstanceType),
		})
	}

	return costs
}

func (e *Estimator) estimateWorkerPool(spec api.ClusterSpec, pool api.WorkerPoolSpec, pricing PricingData) []CostBreakdown {
	var costs []CostBreakdown

	instancePrice, exists := pricing.InstanceTypes[pool.InstanceType]
	if !exists {
		instancePrice = InstancePrice{OnDemandHourly: 0.10} // Default estimate
	}

	// Use desired size, or average of min/max
	nodeCount := pool.DesiredSize
	if nodeCount == 0 {
		nodeCount = (pool.MinSize + pool.MaxSize) / 2
	}

	unitCost := instancePrice.OnDemandHourly
	if pool.Spot != nil && pool.Spot.Enabled {
		unitCost = instancePrice.SpotHourly
		if pool.Spot.MaxPrice > 0 && pool.Spot.MaxPrice < unitCost {
			unitCost = pool.Spot.MaxPrice
		}
	}

	hourlyCost := unitCost * float64(nodeCount)
	costType := "on-demand"
	if pool.Spot != nil && pool.Spot.Enabled {
		costType = "spot"
	}

	costs = append(costs, CostBreakdown{
		Resource: api.ResourceID{
			Provider: spec.Provider,
			Kind:     "NodePool",
			Name:     pool.Name,
		},
		ResourceType: ResourceCompute,
		Quantity:     nodeCount,
		UnitCost:     unitCost,
		HourlyCost:   hourlyCost,
		MonthlyCost:  hourlyCost * 730,
		Details:      fmt.Sprintf("%d x %s (%s)", nodeCount, pool.InstanceType, costType),
	})

	return costs
}

func (e *Estimator) estimateNetwork(spec api.ClusterSpec, pricing PricingData) []CostBreakdown {
	var costs []CostBreakdown

	// NAT Gateway cost
	if spec.Network.NATGateway {
		natCount := len(spec.Network.AvailabilityZones)
		if natCount == 0 {
			natCount = 1
		}

		hourlyCost := pricing.Network.NATGatewayHourly * float64(natCount)
		costs = append(costs, CostBreakdown{
			Resource: api.ResourceID{
				Provider: spec.Provider,
				Kind:     "Network",
				Name:     "nat-gateways",
			},
			ResourceType: ResourceNetwork,
			Quantity:     natCount,
			UnitCost:     pricing.Network.NATGatewayHourly,
			HourlyCost:   hourlyCost,
			MonthlyCost:  hourlyCost * 730,
			Details:      fmt.Sprintf("%d NAT Gateway(s)", natCount),
		})
	}

	// Load Balancer (typically needed for K8s)
	lbHourlyCost := pricing.Network.LoadBalancerHourly
	costs = append(costs, CostBreakdown{
		Resource: api.ResourceID{
			Provider: spec.Provider,
			Kind:     "Network",
			Name:     "load-balancer",
		},
		ResourceType: ResourceNetwork,
		Quantity:     1,
		UnitCost:     lbHourlyCost,
		HourlyCost:   lbHourlyCost,
		MonthlyCost:  lbHourlyCost * 730,
		Details:      "Network Load Balancer",
	})

	return costs
}

func (e *Estimator) calculateSpotSavings(spec api.ClusterSpec, pricing PricingData) float64 {
	savings := 0.0

	for _, pool := range spec.WorkerPools {
		if pool.Spot != nil && pool.Spot.Enabled {
			continue // Already using spot
		}

		instancePrice, exists := pricing.InstanceTypes[pool.InstanceType]
		if !exists {
			continue
		}

		nodeCount := pool.DesiredSize
		if nodeCount == 0 {
			nodeCount = (pool.MinSize + pool.MaxSize) / 2
		}

		onDemandMonthlyCost := instancePrice.OnDemandHourly * float64(nodeCount) * 730
		spotMonthlyCost := instancePrice.SpotHourly * float64(nodeCount) * 730
		savings += (onDemandMonthlyCost - spotMonthlyCost)
	}

	return savings
}

func (e *Estimator) getPricing(provider, region string) (PricingData, error) {
	key := provider + "-" + region
	if data, exists := e.pricingData[key]; exists {
		return data, nil
	}

	// Return default pricing if not found
	return PricingData{
		Provider: provider,
		Region:   region,
		InstanceTypes: map[string]InstancePrice{
			"t3.medium":       {OnDemandHourly: 0.0416, SpotHourly: 0.0125, VCPU: 2, MemoryGB: 4},
			"t3.large":        {OnDemandHourly: 0.0832, SpotHourly: 0.0250, VCPU: 2, MemoryGB: 8},
			"c5.xlarge":       {OnDemandHourly: 0.170, SpotHourly: 0.0510, VCPU: 4, MemoryGB: 8},
			"Standard_D2s_v3": {OnDemandHourly: 0.096, SpotHourly: 0.0288, VCPU: 2, MemoryGB: 8},
			"Standard_D4s_v3": {OnDemandHourly: 0.192, SpotHourly: 0.0576, VCPU: 4, MemoryGB: 16},
		},
		ManagedK8s: ManagedK8sPrice{
			ControlPlaneHourly: 0.10,
			PerNodeHourly:      0.00,
		},
		Network: NetworkPrice{
			LoadBalancerHourly: 0.025,
			NATGatewayHourly:   0.045,
			DataTransferPerGB:  0.09,
		},
		Storage: StoragePrice{
			GP3PerGBMonth: 0.08,
			IOPSPerMonth:  0.005,
		},
	}, nil
}

func loadPricingData() map[string]PricingData {
	// In production, this would load from a pricing database or API
	// For now, return hardcoded common pricing
	return map[string]PricingData{
		"aws-us-west-2": {
			Provider: "aws",
			Region:   "us-west-2",
			InstanceTypes: map[string]InstancePrice{
				"t3.medium": {OnDemandHourly: 0.0416, SpotHourly: 0.0125, VCPU: 2, MemoryGB: 4},
				"t3.large":  {OnDemandHourly: 0.0832, SpotHourly: 0.0250, VCPU: 2, MemoryGB: 8},
				"c5.xlarge": {OnDemandHourly: 0.170, SpotHourly: 0.0510, VCPU: 4, MemoryGB: 8},
			},
			ManagedK8s: ManagedK8sPrice{ControlPlaneHourly: 0.10},
			Network:    NetworkPrice{LoadBalancerHourly: 0.025, NATGatewayHourly: 0.045},
			Storage:    StoragePrice{GP3PerGBMonth: 0.08},
		},
		"azure-eastus": {
			Provider: "azure",
			Region:   "eastus",
			InstanceTypes: map[string]InstancePrice{
				"Standard_D2s_v3": {OnDemandHourly: 0.096, SpotHourly: 0.0288, VCPU: 2, MemoryGB: 8},
				"Standard_D4s_v3": {OnDemandHourly: 0.192, SpotHourly: 0.0576, VCPU: 4, MemoryGB: 16},
			},
			ManagedK8s: ManagedK8sPrice{ControlPlaneHourly: 0.00}, // AKS is free
			Network:    NetworkPrice{LoadBalancerHourly: 0.025, NATGatewayHourly: 0.045},
			Storage:    StoragePrice{GP3PerGBMonth: 0.08},
		},
	}
}

// FormatEstimate generates a human-readable cost estimate
func FormatEstimate(estimate *CostEstimate) string {
	output := fmt.Sprintf("💰 Cost Estimate (generated %s)\n\n", estimate.EstimatedAt.Format("2006-01-02 15:04:05"))
	
	output += fmt.Sprintf("Total Monthly Cost: $%.2f\n", estimate.TotalMonthlyCost)
	output += fmt.Sprintf("Total Hourly Cost:  $%.4f\n\n", estimate.TotalHourlyCost)

	output += "Breakdown by Resource:\n"
	
	// Group by resource type
	typeBreakdown := make(map[ResourceType]float64)
	for _, item := range estimate.Breakdown {
		typeBreakdown[item.ResourceType] += item.MonthlyCost
		
		output += fmt.Sprintf("  • %s/%s: $%.2f/month\n",
			item.Resource.Kind,
			item.Resource.Name,
			item.MonthlyCost,
		)
		output += fmt.Sprintf("    %s ($%.4f/hour x %d)\n\n",
			item.Details,
			item.UnitCost,
			item.Quantity,
		)
	}

	output += "Breakdown by Type:\n"
	for resType, cost := range typeBreakdown {
		percentage := (cost / estimate.TotalMonthlyCost) * 100
		output += fmt.Sprintf("  • %s: $%.2f/month (%.1f%%)\n", resType, cost, percentage)
	}

	if len(estimate.Assumptions) > 0 {
		output += "\nAssumptions:\n"
		for _, assumption := range estimate.Assumptions {
			output += fmt.Sprintf("  • %s\n", assumption)
		}
	}

	if len(estimate.Warnings) > 0 {
		output += "\nWarnings & Recommendations:\n"
		for _, warning := range estimate.Warnings {
			output += fmt.Sprintf("  %s\n", warning)
		}
	}

	return output
}
