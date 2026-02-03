# Example Azure AKS cluster configuration

cluster "production-aks" {
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

    config = {
      network_plugin = "azure"
      network_policy = "calico"
      load_balancer_sku = "standard"
    }
  }

  worker_pools {
    pool "system" {
      instance_type = "Standard_D2s_v3"
      min_size      = 3
      max_size      = 5
      desired_size  = 3

      labels = {
        "kubernetes.azure.com/mode" = "system"
      }
    }

    pool "application" {
      instance_type = "Standard_D4s_v3"
      min_size      = 5
      max_size      = 20
      desired_size  = 10

      labels = {
        workload = "application"
      }

      config = {
        enable_auto_scaling = true
        enable_node_public_ip = false
      }
    }

    pool "batch-spot" {
      instance_type = "Standard_F8s_v2"
      min_size      = 0
      max_size      = 30

      spot {
        enabled = true
      }

      labels = {
        workload = "batch-processing"
        spot     = "true"
      }

      taints {
        key    = "batch"
        value  = "true"
        effect = "NoSchedule"
      }
    }
  }

  tags = {
    Environment = "production"
    Team        = "platform"
    ManagedBy   = "provctl"
    CostCenter  = "engineering"
  }
}
