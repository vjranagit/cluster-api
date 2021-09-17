# cluster-api

> Multi-cloud Kubernetes cluster provisioner with declarative HCL configuration

[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Overview

A re-imagined implementation of Kubernetes cluster lifecycle management inspired by [Cluster API](https://github.com/kubernetes-sigs/cluster-api), featuring:

- **Multi-cloud support**: AWS (EKS, EC2) and Azure (AKS, VMs)
- **Declarative HCL configuration**: Infrastructure-as-code with HashiCorp Configuration Language
- **Plugin architecture**: Extensible provider system
- **Event sourcing**: Complete audit trail of all infrastructure changes
- **State management**: Built-in SQLite storage with optional etcd backend

## What's Different?

This project reimplements the functionality of cluster-api-provider-aws and cluster-api-provider-azure using a different architectural approach:

### Original Cluster API Providers
- Kubernetes-native with Custom Resource Definitions (CRDs)
- Controller-based reconciliation loops
- Separate providers for each cloud
- Uses kubebuilder framework

### Our Implementation
- **HCL-based configuration** (Terraform-like)
- **Planning engine** with explicit apply workflow
- **Unified multi-cloud interface** with provider plugins
- **Event-sourced state** for auditability
- **Modern Go patterns**: generics, slog, context propagation

## Features

### AWS Provider
- ✅ VPC creation with multi-AZ support
- ✅ EC2 instance provisioning
- ✅ EKS managed clusters
- ✅ Auto Scaling Groups for node pools
- ✅ Spot instance support
- ✅ IAM role management with IRSA
- ✅ Network Load Balancers
- ✅ Security group automation

### Azure Provider
- ✅ VNet creation with subnet management
- ✅ Virtual Machine provisioning
- ✅ AKS managed clusters
- ✅ VM Scale Sets for node pools
- ✅ Spot VM support
- ✅ Managed identity integration
- ✅ Load balancer configuration
- ✅ Network Security Groups

### Core Capabilities
- 🎯 Declarative cluster definitions
- 📊 Infrastructure planning (plan/apply workflow)
- 🔄 State management with SQLite
- 📝 Event sourcing for audit logs
- 🔍 Comprehensive logging with structured slog
- 🧩 Extensible provider plugin system

## Installation

```bash
go install github.com/vjranagit/cluster-api/cmd/provctl@latest
```

Or build from source:

```bash
git clone https://github.com/vjranagit/cluster-api
cd cluster-api
go build -o provctl ./cmd/provctl
sudo mv provctl /usr/local/bin/
```

## Quick Start

### Create an EKS Cluster on AWS

```bash
provctl create my-cluster \
  --provider aws \
  --region us-west-2
```

### Using HCL Configuration

Create `cluster.hcl`:

```hcl
cluster "production" {
  provider = "aws"
  region   = "us-west-2"

  network {
    vpc_cidr = "10.0.0.0/16"
    availability_zones = ["us-west-2a", "us-west-2b", "us-west-2c"]
    nat_gateway = true
    private_cluster = false
  }

  control_plane {
    type    = "managed"  # EKS
    version = "1.28"
    ha      = true

    identity {
      type = "oidc"
      service_accounts = [
        "kube-system/aws-load-balancer-controller",
        "kube-system/cluster-autoscaler"
      ]
    }
  }

  worker_pools {
    pool "general" {
      instance_type = "t3.medium"
      min_size      = 3
      max_size      = 10
      desired_size  = 5

      labels = {
        workload = "general"
      }
    }

    pool "compute" {
      instance_type = "c5.xlarge"
      min_size      = 0
      max_size      = 20

      spot {
        enabled   = true
        max_price = 0.08
      }

      labels = {
        workload = "compute-intensive"
      }

      taints {
        key    = "compute"
        value  = "true"
        effect = "NoSchedule"
      }
    }
  }

  tags = {
    Environment = "production"
    Team        = "platform"
    ManagedBy   = "provctl"
  }
}
```

Apply the configuration:

```bash
provctl apply cluster.hcl
```

### Create an AKS Cluster on Azure

```hcl
cluster "staging" {
  provider = "azure"
  region   = "eastus"

  network {
    vpc_cidr = "10.1.0.0/16"
    availability_zones = ["1", "2", "3"]
  }

  control_plane {
    type    = "managed"  # AKS
    version = "1.28"

    identity {
      type = "managed"
    }
  }

  worker_pools {
    pool "system" {
      instance_type = "Standard_D2s_v3"
      min_size      = 3
      max_size      = 5

      labels = {
        "kubernetes.azure.com/mode" = "system"
      }
    }

    pool "user" {
      instance_type = "Standard_D4s_v3"
      min_size      = 2
      max_size      = 10

      spot {
        enabled = true
      }
    }
  }
}
```

## Usage

### List Clusters

```bash
provctl list
```

Output:
```
Clusters:
  - production (cluster-abc123) - aws - Running
  - staging (cluster-xyz789) - azure - Running
```

### Delete a Cluster

```bash
provctl delete production
```

### Version Information

```bash
provctl version
```

## Architecture

### Component Overview

```
┌─────────────────────────────────────────┐
│            provctl CLI                   │
└───────────────┬─────────────────────────┘
                │
┌───────────────┴─────────────────────────┐
│         Provisioning Engine             │
│  - HCL Parser                           │
│  - Planning Engine                      │
│  - State Manager (SQLite)               │
│  - Event Store                          │
└───────────────┬─────────────────────────┘
                │
        ┌───────┴───────┐
        │               │
┌───────┴──────┐ ┌─────┴────────┐
│ AWS Provider │ │Azure Provider│
│  - EC2       │ │  - VMs       │
│  - EKS       │ │  - AKS       │
│  - VPC       │ │  - VNet      │
│  - IAM       │ │  - Identity  │
└──────────────┘ └──────────────┘
```

### Key Design Patterns

1. **Provider Interface**: Generic `CloudProvider` interface for all clouds
2. **Resource Generics**: Type-safe resources using Go 1.21+ generics
3. **Event Sourcing**: All state changes recorded as immutable events
4. **Planning Phase**: Generate execution plan before applying changes
5. **Structured Logging**: JSON logs with slog for observability

## Development

### Prerequisites

- Go 1.21 or later
- AWS credentials configured (for AWS provider)
- Azure credentials configured (for Azure provider)

### Build

```bash
make build
```

### Test

```bash
make test
```

### Run locally

```bash
go run ./cmd/provctl create test-cluster --provider aws
```

## Configuration Reference

### Cluster Spec

```hcl
cluster "name" {
  provider = "aws" | "azure"
  region   = "<region>"

  network {
    vpc_cidr           = "<cidr>"
    availability_zones = ["zone1", "zone2"]
    nat_gateway        = true | false
    private_cluster    = true | false

    subnets {
      subnet "name" {
        cidr              = "<cidr>"
        availability_zone = "<zone>"
        public            = true | false
      }
    }
  }

  control_plane {
    type         = "managed" | "self-managed"
    version      = "<k8s-version>"
    instance_type = "<instance-type>"  # for self-managed
    count        = <number>            # for self-managed
    ha           = true | false

    identity {
      type            = "oidc" | "managed"
      service_accounts = ["<sa1>", "<sa2>"]
      role_arn        = "<arn>"  # AWS only
    }
  }

  worker_pools {
    pool "name" {
      instance_type = "<instance-type>"
      min_size      = <number>
      max_size      = <number>
      desired_size  = <number>

      spot {
        enabled   = true | false
        max_price = <price>
      }

