# ktop

<h1 align="center">
    <img src="./docs/ktop.png" alt="ktop">
</h1>

A `top`-like tool for your Kubernetes cluster.

Following the tradition of Unix/Linux `top` tools, `ktop` is a tool that displays useful metrics information about nodes, pods, and other workload resources running in a Kubernetes cluster.

## Features

* Insightful summary of cluster resource metrics
* Ability to work with or without a metrics-server deployed
* Displays nodes and pods usage metrics when a Metrics Server is found
* Uses your existing cluster configuration to connect to a cluster's API server

## Installing ktop

### Download binary

One easy way to get started with ktop is to download the pre-built binary (for your system):

> https://github.com/vladimirvivien/ktop/releases/latest

Then, extract the ktop binary and copy it to your system's execution path.

### Using `go install`

If you have a recent version of Go installed (1.14 or later) you can build and install ktop as follows:

```
go install github.com/vladimirvivien/ktop@latest
```

This should place the ktop binary in your configured `$GOBIN` path or place it in its default location, `$HOME/go/bin`.

### Build from source

Download or clone the source (from GitHub). From the project's root directory, do the following:

```
go build .
```

The project also comes with a Go program that you can use for cross-platform builds.
```
go run ./ci/build.go
```

## Running ktop

With a locally accessible kubeconfig file on your machine, ktop can be executed simply:

```
ktop
```

The previous command will use either environment variable `$KUBECONFIG` or the default path for the kubeconfig file.

The program currently accepts the following arguments:

```
Usage of ktop:
  -context : Name of the cluster context, if empty, uses current context (default empty)
  -kubeconfig : The path for the kubeconfig file, if empty, env $KUBECONFIG or $HOME/.kube/config will be used (default empty)
  -namespace : The namespace to use, if set to * or leave empty, uses all namespaces (defaults to empty)
```

For instance, the following will show cluster information for workload resources associated with namespace `my-app` in context `web-cluster` using the default kubconfig file path:

```
ktop --namespace my-app --context web-cluster
```

## ktop metrics

The ktop UI provides several metrics including a high-level summary of workload components installed on your cluster:

<h1 align="center">
    <img src="./docs/ktop-cluster-summary.png" alt="ktop">
</h1>

### Usage metrics from `metrics-server`

ktop can display metrics with or without Metrics Server present.  When a cluster has an instance of a [kubernetes-sigs/metrics-server](https://github.com/kubernetes-sigs/metrics-server) installed (and properly configured), ktop will automatically discover the server as shown:

<h1 align="center">
    <img src="./docs/ktop-metrics-connected.png" alt="ktop">
</h1>

With the metrics server installed, ktop will display resource utilization metrics as reported by the Metrics Server.

### Request/limit metrics

When there is no Metrics Server present in the cluster, ktop will still work:

<h1 align="center">
    <img src="./docs/ktop-metrics-not-connected.png" alt="ktop">
</h1>

Instead of resource utilization, ktop will display resource requests and limits for nodes and pods.

## Roadmap

* A multi-page UI to display metrics for additional components
* Display OOM processes
* Additional installation methods (Homebrew, linux packages, etc)
* kubectl plugin
* Etc
