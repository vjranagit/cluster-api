package drift

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/vjranagit/cluster-api/pkg/api"
	"github.com/vjranagit/cluster-api/pkg/engine"
)

func TestDriftDetector_DetectDrift(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	eng := engine.NewEngine(nil, nil)
	detector := NewDriftDetector(eng, logger)

	tests := []struct {
		name        string
		desired     engine.State
		actual      engine.State
		wantDrifts  int
		wantCritical int
	}{
		{
			name: "no drift",
			desired: engine.State{
				Clusters: map[string]*api.Cluster{
					"cluster-1": {
						ID: "cluster-1",
						Metadata: api.ResourceMetadata{
							Name: "test-cluster",
						},
						Spec: api.ClusterSpec{
							Provider: "aws",
							ControlPlane: api.ControlPlaneSpec{
								Version: "1.28",
							},
						},
					},
				},
			},
			actual: engine.State{
				Clusters: map[string]*api.Cluster{
					"cluster-1": {
						ID: "cluster-1",
						Metadata: api.ResourceMetadata{
							Name: "test-cluster",
						},
						Spec: api.ClusterSpec{
							Provider: "aws",
							ControlPlane: api.ControlPlaneSpec{
								Version: "1.28",
							},
						},
					},
				},
			},
			wantDrifts:   0,
			wantCritical: 0,
		},
		{
			name: "version drift",
			desired: engine.State{
				Clusters: map[string]*api.Cluster{
					"cluster-1": {
						ID: "cluster-1",
						Metadata: api.ResourceMetadata{
							Name: "test-cluster",
						},
						Spec: api.ClusterSpec{
							Provider: "aws",
							ControlPlane: api.ControlPlaneSpec{
								Version: "1.29",
							},
						},
					},
				},
			},
			actual: engine.State{
				Clusters: map[string]*api.Cluster{
					"cluster-1": {
						ID: "cluster-1",
						Metadata: api.ResourceMetadata{
							Name: "test-cluster",
						},
						Spec: api.ClusterSpec{
							Provider: "aws",
							ControlPlane: api.ControlPlaneSpec{
								Version: "1.28",
							},
						},
					},
				},
			},
			wantDrifts:   1,
			wantCritical: 0,
		},
		{
			name: "cluster deleted",
			desired: engine.State{
				Clusters: map[string]*api.Cluster{
					"cluster-1": {
						ID: "cluster-1",
						Metadata: api.ResourceMetadata{
							Name: "test-cluster",
						},
						Spec: api.ClusterSpec{
							Provider: "aws",
						},
					},
				},
			},
			actual: engine.State{
				Clusters: map[string]*api.Cluster{},
			},
			wantDrifts:   1,
			wantCritical: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			report, err := detector.DetectDrift(ctx, tt.desired)

			if err != nil {
				t.Errorf("DetectDrift() error = %v", err)
				return
			}

			if len(report.Drifts) != tt.wantDrifts {
				t.Errorf("DetectDrift() got %d drifts, want %d", len(report.Drifts), tt.wantDrifts)
			}

			if report.Summary.CriticalCount != tt.wantCritical {
				t.Errorf("DetectDrift() got %d critical, want %d", report.Summary.CriticalCount, tt.wantCritical)
			}
		})
	}
}

func TestFormatReport(t *testing.T) {
	report := &DriftReport{
		DetectedAt: time.Now(),
		HasDrift:   true,
		Drifts: []ResourceDrift{
			{
				Resource: api.ResourceID{
					Kind: "Cluster",
					Name: "test-cluster",
				},
				DriftType:    DriftVersionSkew,
				Field:        "version",
				Expected:     "1.29",
				Actual:       "1.28",
				Severity:     SeverityHigh,
				Remediatable: true,
			},
		},
		Summary: DriftSummary{
			TotalDrifts:     1,
			HighCount:       1,
			RemediableCount: 1,
		},
	}

	output := FormatReport(report)
	if output == "" {
		t.Error("FormatReport() returned empty string")
	}

	if !contains(output, "Drift Detected") {
		t.Error("FormatReport() output missing expected content")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0
}
