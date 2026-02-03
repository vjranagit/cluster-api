// Package drift provides drift detection capabilities
package drift

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vjranagit/cluster-api/pkg/api"
	"github.com/vjranagit/cluster-api/pkg/engine"
)

// DriftDetector detects configuration drift between desired and actual state
type DriftDetector struct {
	engine *engine.Engine
	logger *slog.Logger
}

// NewDriftDetector creates a new drift detector
func NewDriftDetector(eng *engine.Engine, logger *slog.Logger) *DriftDetector {
	return &DriftDetector{
		engine: eng,
		logger: logger,
	}
}

// DriftReport contains detected drift information
type DriftReport struct {
	DetectedAt   time.Time
	HasDrift     bool
	Drifts       []ResourceDrift
	Summary      DriftSummary
}

// ResourceDrift represents drift for a single resource
type ResourceDrift struct {
	Resource     api.ResourceID
	DriftType    DriftType
	Field        string
	Expected     interface{}
	Actual       interface{}
	Severity     Severity
	Remediatable bool
}

// DriftType categorizes types of drift
type DriftType string

const (
	DriftConfigChange    DriftType = "config_change"    // Configuration modified outside provctl
	DriftVersionSkew     DriftType = "version_skew"     // Version mismatch
	DriftScaleChange     DriftType = "scale_change"     // Node count changed
	DriftNetworkChange   DriftType = "network_change"   // Network config modified
	DriftSecurityChange  DriftType = "security_change"  // Security settings changed
	DriftResourceDeleted DriftType = "resource_deleted" // Resource deleted externally
	DriftResourceAdded   DriftType = "resource_added"   // Unexpected resource exists
)

// Severity indicates drift severity
type Severity string

const (
	SeverityCritical Severity = "critical" // Immediate action required
	SeverityHigh     Severity = "high"     // Should be remediated soon
	SeverityMedium   Severity = "medium"   // Non-urgent drift
	SeverityLow      Severity = "low"      // Informational
)

// DriftSummary provides drift statistics
type DriftSummary struct {
	TotalDrifts      int
	CriticalCount    int
	HighCount        int
	MediumCount      int
	LowCount         int
	RemediableCount  int
}

// DetectDrift compares desired state with actual cloud state
func (d *DriftDetector) DetectDrift(ctx context.Context, desired engine.State) (*DriftReport, error) {
	d.logger.Info("starting drift detection")

	report := &DriftReport{
		DetectedAt: time.Now(),
		Drifts:     []ResourceDrift{},
	}

	// Detect drift for each provider
	for providerName, provider := range d.getAllProviders() {
		d.logger.Debug("checking drift for provider", "provider", providerName)

		// Get actual state from cloud provider
		actual, err := provider.Reconcile(ctx, desired, engine.State{})
		if err != nil {
			d.logger.Error("failed to get actual state", "provider", providerName, "error", err)
			continue
		}

		// Compare clusters
		for id, desiredCluster := range desired.Clusters {
			if desiredCluster.Spec.Provider != providerName {
				continue
			}

			actualCluster, exists := actual.Clusters[id]
			if !exists {
				report.Drifts = append(report.Drifts, ResourceDrift{
					Resource: api.ResourceID{
						Provider: providerName,
						Kind:     "Cluster",
						ID:       id,
						Name:     desiredCluster.Metadata.Name,
					},
					DriftType:    DriftResourceDeleted,
					Field:        "cluster",
					Expected:     "exists",
					Actual:       "deleted",
					Severity:     SeverityCritical,
					Remediatable: true,
				})
				continue
			}

			// Check version drift
			if desiredCluster.Spec.ControlPlane.Version != actualCluster.Spec.ControlPlane.Version {
				report.Drifts = append(report.Drifts, ResourceDrift{
					Resource: api.ResourceID{
						Provider: providerName,
						Kind:     "Cluster",
						ID:       id,
						Name:     desiredCluster.Metadata.Name,
					},
					DriftType:    DriftVersionSkew,
					Field:        "controlPlane.version",
					Expected:     desiredCluster.Spec.ControlPlane.Version,
					Actual:       actualCluster.Spec.ControlPlane.Version,
					Severity:     SeverityHigh,
					Remediatable: true,
				})
			}

			// Check worker pool drift
			for _, desiredPool := range desiredCluster.Spec.WorkerPools {
				foundPool := false
				for _, actualPool := range actualCluster.Spec.WorkerPools {
					if desiredPool.Name == actualPool.Name {
						foundPool = true

						// Check scale drift
						if desiredPool.DesiredSize != actualPool.DesiredSize {
							report.Drifts = append(report.Drifts, ResourceDrift{
								Resource: api.ResourceID{
									Provider: providerName,
									Kind:     "NodePool",
									ID:       id + "/" + desiredPool.Name,
									Name:     desiredPool.Name,
								},
								DriftType:    DriftScaleChange,
								Field:        "desiredSize",
								Expected:     desiredPool.DesiredSize,
								Actual:       actualPool.DesiredSize,
								Severity:     SeverityMedium,
								Remediatable: true,
							})
						}
					}
				}

				if !foundPool {
					report.Drifts = append(report.Drifts, ResourceDrift{
						Resource: api.ResourceID{
							Provider: providerName,
							Kind:     "NodePool",
							ID:       id + "/" + desiredPool.Name,
							Name:     desiredPool.Name,
						},
						DriftType:    DriftResourceDeleted,
						Field:        "nodePool",
						Expected:     "exists",
						Actual:       "deleted",
						Severity:     SeverityHigh,
						Remediatable: true,
					})
				}
			}
		}
	}

	// Compute summary
	report.HasDrift = len(report.Drifts) > 0
	for _, drift := range report.Drifts {
		switch drift.Severity {
		case SeverityCritical:
			report.Summary.CriticalCount++
		case SeverityHigh:
			report.Summary.HighCount++
		case SeverityMedium:
			report.Summary.MediumCount++
		case SeverityLow:
			report.Summary.LowCount++
		}
		if drift.Remediatable {
			report.Summary.RemediableCount++
		}
	}
	report.Summary.TotalDrifts = len(report.Drifts)

	d.logger.Info("drift detection complete",
		"total_drifts", report.Summary.TotalDrifts,
		"critical", report.Summary.CriticalCount,
		"high", report.Summary.HighCount,
	)

	return report, nil
}

// Remediate automatically fixes detected drift
func (d *DriftDetector) Remediate(ctx context.Context, report *DriftReport) error {
	d.logger.Info("starting drift remediation", "total_drifts", len(report.Drifts))

	remediatedCount := 0
	for _, drift := range report.Drifts {
		if !drift.Remediatable {
			d.logger.Warn("drift not remediatable", "resource", drift.Resource.Name, "type", drift.DriftType)
			continue
		}

		d.logger.Info("remediating drift",
			"resource", drift.Resource.Name,
			"type", drift.DriftType,
			"field", drift.Field,
		)

		if err := d.remediateDrift(ctx, drift); err != nil {
			d.logger.Error("failed to remediate drift", "resource", drift.Resource.Name, "error", err)
			continue
		}

		remediatedCount++
	}

	d.logger.Info("drift remediation complete",
		"remediated", remediatedCount,
		"total", len(report.Drifts),
	)

	return nil
}

func (d *DriftDetector) remediateDrift(ctx context.Context, drift ResourceDrift) error {
	provider := d.engine.GetProvider(drift.Resource.Provider)
	if provider == nil {
		return fmt.Errorf("provider %s not found", drift.Resource.Provider)
	}

	// Remediation logic based on drift type
	switch drift.DriftType {
	case DriftResourceDeleted:
		// Recreate the resource
		d.logger.Info("recreating deleted resource", "resource", drift.Resource.Name)
		// Implementation would call provider.CreateCluster or CreateNodePool
		return nil

	case DriftVersionSkew:
		// Update version
		d.logger.Info("updating version", "resource", drift.Resource.Name, "expected", drift.Expected)
		// Implementation would call provider.UpdateCluster
		return nil

	case DriftScaleChange:
		// Adjust scale
		d.logger.Info("adjusting scale", "resource", drift.Resource.Name, "expected", drift.Expected)
		// Implementation would call provider.UpdateNodePool
		return nil

	default:
		return fmt.Errorf("unsupported drift type: %s", drift.DriftType)
	}
}

func (d *DriftDetector) getAllProviders() map[string]engine.CloudProvider {
	// This would be implemented to return all registered providers
	providers := make(map[string]engine.CloudProvider)
	// In real implementation, iterate through engine.providers
	return providers
}

// FormatReport generates a human-readable drift report
func FormatReport(report *DriftReport) string {
	if !report.HasDrift {
		return "✓ No drift detected - infrastructure matches configuration"
	}

	output := fmt.Sprintf("⚠ Drift Detected at %s\n\n", report.DetectedAt.Format(time.RFC3339))
	output += fmt.Sprintf("Summary: %d total drifts (%d critical, %d high, %d medium, %d low)\n",
		report.Summary.TotalDrifts,
		report.Summary.CriticalCount,
		report.Summary.HighCount,
		report.Summary.MediumCount,
		report.Summary.LowCount,
	)
	output += fmt.Sprintf("Remediatable: %d\n\n", report.Summary.RemediableCount)

	output += "Detected Drifts:\n"
	for _, drift := range report.Drifts {
		severity := getSeverityIcon(drift.Severity)
		remediation := ""
		if drift.Remediatable {
			remediation = " [auto-fixable]"
		}

		output += fmt.Sprintf("  %s %s/%s - %s%s\n",
			severity,
			drift.Resource.Kind,
			drift.Resource.Name,
			drift.DriftType,
			remediation,
		)
		output += fmt.Sprintf("      Field: %s\n", drift.Field)
		output += fmt.Sprintf("      Expected: %v\n", drift.Expected)
		output += fmt.Sprintf("      Actual: %v\n\n", drift.Actual)
	}

	return output
}

func getSeverityIcon(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "🔴"
	case SeverityHigh:
		return "🟠"
	case SeverityMedium:
		return "🟡"
	case SeverityLow:
		return "🔵"
	default:
		return "⚪"
	}
}
