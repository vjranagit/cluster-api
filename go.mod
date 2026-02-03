module github.com/vjranagit/cluster-api

go 1.21

require (
	github.com/aws/aws-sdk-go-v2 v1.24.0
	github.com/aws/aws-sdk-go-v2/config v1.26.0
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.141.0
	github.com/aws/aws-sdk-go-v2/service/eks v1.35.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.28.0
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing v1.22.0
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.9.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.3.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4 v4.7.0
	github.com/google/uuid v1.5.0
	github.com/hashicorp/hcl/v2 v2.19.1
	github.com/spf13/cobra v1.8.0
	modernc.org/sqlite v1.28.0
	go.etcd.io/etcd/client/v3 v3.5.11
	go.opentelemetry.io/otel v1.21.0
	go.opentelemetry.io/otel/trace v1.21.0
)
