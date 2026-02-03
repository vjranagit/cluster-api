# Example AWS EKS cluster configuration

cluster "production-eks" {
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
        "kube-system/cluster-autoscaler",
        "kube-system/ebs-csi-driver"
      ]
    }
  }

  worker_pools {
    pool "system" {
      instance_type = "t3.medium"
      min_size      = 3
      max_size      = 6
      desired_size  = 3

      labels = {
        workload = "system"
        tier     = "control"
      }
    }

    pool "general" {
      instance_type = "t3.large"
      min_size      = 5
      max_size      = 20
      desired_size  = 10

      labels = {
        workload = "general"
      }
    }

    pool "compute-spot" {
      instance_type = "c5.2xlarge"
      min_size      = 0
      max_size      = 50

      spot {
        enabled   = true
        max_price = 0.15
      }

      labels = {
        workload = "compute-intensive"
        spot     = "true"
      }

      taints {
        key    = "spot"
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
