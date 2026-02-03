// Package planner provides infrastructure planning capabilities
package planner

import (
	"context"
	"fmt"

	"github.com/vjranagit/cluster-api/pkg/api"
	"github.com/vjranagit/cluster-api/pkg/engine"
)

// Planner generates execution plans for infrastructure changes
type Planner struct {
	provider engine.CloudProvider
}

// NewPlanner creates a new planner
func NewPlanner(provider engine.CloudProvider) *Planner {
	return &Planner{
		provider: provider,
	}
}

// GeneratePlan creates a plan by comparing desired and actual state
func (p *Planner) GeneratePlan(ctx context.Context, desired, actual engine.State) (engine.Plan, error) {
	plan := engine.Plan{
		Actions: []engine.Action{},
	}

	// Determine clusters to create
	for id, cluster := range desired.Clusters {
		if _, exists := actual.Clusters[id]; !exists {
			plan.Actions = append(plan.Actions, engine.Action{
				Type: engine.ActionCreate,
				Resource: api.ResourceID{
					Provider: cluster.Spec.Provider,
					Kind:     "Cluster",
					ID:       id,
					Name:     cluster.Metadata.Name,
				},
				Parameters: map[string]interface{}{
					"spec": cluster.Spec,
				},
			})
		}
	}

	// Determine clusters to update
	for id, desiredCluster := range desired.Clusters {
		if actualCluster, exists := actual.Clusters[id]; exists {
			if needsUpdate(desiredCluster, actualCluster) {
				plan.Actions = append(plan.Actions, engine.Action{
					Type: engine.ActionUpdate,
					Resource: api.ResourceID{
						Provider: desiredCluster.Spec.Provider,
						Kind:     "Cluster",
						ID:       id,
						Name:     desiredCluster.Metadata.Name,
					},
					Parameters: map[string]interface{}{
						"spec": desiredCluster.Spec,
					},
				})
			}
		}
	}

	// Determine clusters to delete
	for id, cluster := range actual.Clusters {
		if _, exists := desired.Clusters[id]; !exists {
			plan.Actions = append(plan.Actions, engine.Action{
				Type: engine.ActionDelete,
				Resource: api.ResourceID{
					Provider: cluster.Spec.Provider,
					Kind:     "Cluster",
					ID:       id,
					Name:     cluster.Metadata.Name,
				},
			})
		}
	}

	// Similar logic for node pools
	for id, pool := range desired.NodePools {
		if _, exists := actual.NodePools[id]; !exists {
			plan.Actions = append(plan.Actions, engine.Action{
				Type: engine.ActionCreate,
				Resource: api.ResourceID{
					Kind: "NodePool",
					ID:   id,
					Name: pool.Metadata.Name,
				},
				Parameters: map[string]interface{}{
					"spec": pool.Spec,
				},
			})
		}
	}

	return plan, nil
}

// PrintPlan formats and displays a plan
func (p *Planner) PrintPlan(plan engine.Plan) string {
	output := "Infrastructure Plan:\n\n"

	creates := 0
	updates := 0
	deletes := 0

	for _, action := range plan.Actions {
		switch action.Type {
		case engine.ActionCreate:
			creates++
			output += fmt.Sprintf("  + %s %s (%s)\n", action.Resource.Kind, action.Resource.Name, action.Resource.ID)
		case engine.ActionUpdate:
			updates++
			output += fmt.Sprintf("  ~ %s %s (%s)\n", action.Resource.Kind, action.Resource.Name, action.Resource.ID)
		case engine.ActionDelete:
			deletes++
			output += fmt.Sprintf("  - %s %s (%s)\n", action.Resource.Kind, action.Resource.Name, action.Resource.ID)
		}
	}

	output += fmt.Sprintf("\nPlan: %d to create, %d to update, %d to delete\n", creates, updates, deletes)
	return output
}

func needsUpdate(desired, actual *api.Cluster) bool {
	// Compare specs to determine if update is needed
	// Simplified - real implementation would deep compare
	return desired.Spec.ControlPlane.Version != actual.Spec.ControlPlane.Version
}
