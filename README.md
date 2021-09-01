# cluster-api-provider-kind

This is a provider which creates a KIND cluster on your local machine when a KINDCluster custom resource is created.

Please see the following document for detailed infromation about cluster-api: https://cluster-api.sigs.k8s.io/introduction.html

Please see the following document for detailed infromation how an infrastructure provider can be
implemented: https://cluster-api.sigs.k8s.io/developer/providers/implementers-guide/overview.html

Please see the following document for detailed information about the Kubernetes API Extension (with kubebuilder): https://book.kubebuilder.io/introduction.html

## High-Level Architecture

![](highlevelarch.png)

Here is how the system works from the high level. You can see three boxes in the figure. Here's what they are:

- Infrastructure Provider: This is actually the provider itself and the source code is in this repo. It is a kubernetes operator with a single controller. It works against Management Cluster and provisioned Workload Clusters on your local machine.

- Management Cluster: In this ideal case, it is the machine where the controller is running (in this architecture, the controller is run from the local machine for easier execution). The KINDCluster CRD is defined and its instances are reconciled in this cluster. In summary, the definitions of the clusters whose lifecycles will be managed are deployed to this cluster and the controller handles this process.

- Workload Cluster: These are the clusters whose definitions are specified in the management cluster. Lifecycles are managed by the provider.

## Provider Architecture

It is a kubernetes operator responsible for managing lifecycles of workload clusters. The type of clusters is `kind`. The desired states of the clusters are defined in CRs named KINDCluster. These CRs live in the management cluster. The features provided by the provider are:

- Creating the Workload Cluster: When a KINDCluster instance is created in the management cluster, the controller handles it and provisions a kind workload cluster appropriate to the specified desired state.

- Storing the Kubeconfig: When a KINDCluster instance is created in the management cluster, the controller handles it and in management cluster, creates a kubernetes secret that contains the kubeconfig data. Name convention is: `clusterName-config`

- Deletion of Cluster: When a KINDCluster instance is deleted in the management cluster, the controller handles it and deletes the workload kind cluster and kubeconfig secret. When you trigger a deletion (for example with kubectl delete), firstly the finalizer blocks the deletion until the external dependencies of the KINDCluster are deleted.

- Watching the Actual Status and History of Workload Cluster from the KINDCluster Instance: The status subresource of KINDCluster instances is quite informative. It is possible to observe whether the cluster is ready, its historical background and problematic situations by reviewing its status.

## How Can You Try?

First, create a management cluster using the kind tool. Then deploy the KINDCluster CRD to this cluster (make install). Then deploy some sample manifests in the config/samples/ directory to the cluster (kubectl apply -f filepath), and then run the provider (make run). If you wish, you can run the provider first and then deploy the manifests. 

Meanwhile, the KINDClusters you deploy are handled by the controller and kind workload clusters are created. It is possible to follow this with the `kind get clusters` command.
