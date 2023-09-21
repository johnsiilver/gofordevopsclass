# Kubernetes Petstore Operator
This is a Kubernetes operator that extends the Kubernetes API using Custom Resource Definitions (CRDs) to enable the
creation and mutation of pets in a petstore. That means that you can use `kubectl` to create, update, and delete pets!
The pets are stored in the Petstore service, which is a gRPC service that is also included in this repository.

## Prerequisites
* [Tilt](https://tilt.dev/)
* [Kind](https://kind.sigs.k8s.io/)
* [Docker](https://www.docker.com/)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)

## Running this example
```shell
kind create cluster
tilt up
kubectl apply -f examples/thor.yaml
# see that Thor was created in the petstore and has been given an ID from the petstore service
kubectl get thor -o yaml
kubectl delete thor
```

## Tearing down this example
```shell
tilt down
kind delete cluster
```