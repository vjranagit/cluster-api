// Package api defines core resource types for cluster provisioning
package api

import (
	"time"

	"github.com/google/uuid"
)

// Resource represents a generic cloud resource with metadata
type Resource[T any] struct {
	ID       string           `json:"id"`
	Metadata ResourceMetadata `json:"metadata"`
	Spec     T                `json:"spec"`
	Status   ResourceStatus   `json:"status"`
}

// ResourceMetadata contains common metadata for all resources
type ResourceMetadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// ResourceStatus represents the current state of a resource
type ResourceStatus struct {
	Phase      Phase             `json:"phase"`
	Conditions []Condition       `json:"conditions,omitempty"`
	Message    string            `json:"message,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// Phase represents the lifecycle phase of a resource
type Phase string

const (
	PhasePending     Phase = "Pending"
	PhaseProvisioning Phase = "Provisioning"
	PhaseRunning     Phase = "Running"
	PhaseUpdating    Phase = "Updating"
	PhaseDeleting    Phase = "Deleting"
	PhaseFailed      Phase = "Failed"
)

// Condition represents a condition of a resource
type Condition struct {
	Type               ConditionType `json:"type"`
	Status             bool          `json:"status"`
	LastTransitionTime time.Time     `json:"lastTransitionTime"`
	Reason             string        `json:"reason,omitempty"`
	Message            string        `json:"message,omitempty"`
}

// ConditionType represents different condition types
type ConditionType string

const (
	ConditionReady            ConditionType = "Ready"
	ConditionNetworkReady     ConditionType = "NetworkReady"
	ConditionControlPlaneReady ConditionType = "ControlPlaneReady"
	ConditionNodesReady       ConditionType = "NodesReady"
)

// ClusterSpec defines the desired state of a cluster
type ClusterSpec struct {
	Provider     string                 `json:"provider" hcl:"provider"`
	Region       string                 `json:"region" hcl:"region"`
	Network      NetworkSpec            `json:"network" hcl:"network,block"`
	ControlPlane ControlPlaneSpec       `json:"controlPlane" hcl:"control_plane,block"`
	WorkerPools  []WorkerPoolSpec       `json:"workerPools" hcl:"worker_pools,block"`
	Tags         map[string]string      `json:"tags,omitempty" hcl:"tags,optional"`
	Config       map[string]interface{} `json:"config,omitempty" hcl:"config,optional"`
}

// NetworkSpec defines network configuration
type NetworkSpec struct {
	VPCCIDR           string   `json:"vpcCidr" hcl:"vpc_cidr"`
	AvailabilityZones []string `json:"availabilityZones" hcl:"availability_zones"`
	Subnets           []Subnet `json:"subnets,omitempty" hcl:"subnets,block"`
	NATGateway        bool     `json:"natGateway" hcl:"nat_gateway,optional"`
	PrivateCluster    bool     `json:"privateCluster" hcl:"private_cluster,optional"`
}

// Subnet defines a subnet configuration
type Subnet struct {
	Name             string `json:"name" hcl:"name,label"`
	CIDR             string `json:"cidr" hcl:"cidr"`
	AvailabilityZone string `json:"availabilityZone" hcl:"availability_zone"`
	Public           bool   `json:"public" hcl:"public,optional"`
}

// ControlPlaneSpec defines control plane configuration
type ControlPlaneSpec struct {
	Type         ControlPlaneType       `json:"type" hcl:"type"`
	Version      string                 `json:"version" hcl:"version"`
	InstanceType string                 `json:"instanceType,omitempty" hcl:"instance_type,optional"`
	Count        int                    `json:"count,omitempty" hcl:"count,optional"`
	HA           bool                   `json:"ha" hcl:"ha,optional"`
	Identity     *IdentitySpec          `json:"identity,omitempty" hcl:"identity,block"`
	Config       map[string]interface{} `json:"config,omitempty" hcl:"config,optional"`
}

// ControlPlaneType defines the type of control plane
type ControlPlaneType string

const (
	ControlPlaneManaged      ControlPlaneType = "managed"      // EKS, AKS
	ControlPlaneSelfManaged  ControlPlaneType = "self-managed" // EC2, VM based
)

// IdentitySpec defines identity/RBAC configuration
type IdentitySpec struct {
	Type            string   `json:"type" hcl:"type"`
	ServiceAccounts []string `json:"serviceAccounts,omitempty" hcl:"service_accounts,optional"`
	RoleARN         string   `json:"roleArn,omitempty" hcl:"role_arn,optional"`
}

// WorkerPoolSpec defines a worker node pool
type WorkerPoolSpec struct {
	Name         string                 `json:"name" hcl:"name,label"`
	InstanceType string                 `json:"instanceType" hcl:"instance_type"`
	MinSize      int                    `json:"minSize" hcl:"min_size"`
	MaxSize      int                    `json:"maxSize" hcl:"max_size"`
	DesiredSize  int                    `json:"desiredSize,omitempty" hcl:"desired_size,optional"`
	Spot         *SpotConfig            `json:"spot,omitempty" hcl:"spot,block"`
	Labels       map[string]string      `json:"labels,omitempty" hcl:"labels,optional"`
	Taints       []Taint                `json:"taints,omitempty" hcl:"taints,block"`
	Config       map[string]interface{} `json:"config,omitempty" hcl:"config,optional"`
}

// SpotConfig defines spot/preemptible instance configuration
type SpotConfig struct {
	Enabled  bool    `json:"enabled" hcl:"enabled"`
	MaxPrice float64 `json:"maxPrice,omitempty" hcl:"max_price,optional"`
}

// Taint represents a Kubernetes taint
type Taint struct {
	Key    string `json:"key" hcl:"key"`
	Value  string `json:"value" hcl:"value"`
	Effect string `json:"effect" hcl:"effect"`
}

// Cluster is a complete cluster resource
type Cluster = Resource[ClusterSpec]

// NodePool is a worker node pool resource
type NodePool = Resource[WorkerPoolSpec]

// Event represents a state change event
type Event struct {
	ID        uuid.UUID   `json:"id"`
	Timestamp time.Time   `json:"timestamp"`
	Type      EventType   `json:"type"`
	Resource  ResourceID  `json:"resource"`
	Actor     string      `json:"actor"`
	Payload   interface{} `json:"payload"`
}

// EventType defines types of events
type EventType string

const (
	EventCreated EventType = "Created"
	EventUpdated EventType = "Updated"
	EventDeleted EventType = "Deleted"
	EventFailed  EventType = "Failed"
)

// ResourceID uniquely identifies a resource
type ResourceID struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name"`
}
