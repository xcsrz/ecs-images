# ecs-images

### _for those times when your ECS cluster gets segmented and you need to sort out which images:versions are used where_

This tool lists all unique container image URIs used by services in a given AWS ECS cluster, and shows which services use each image.

## Usage

```
go run main.go --cluster <ECS_CLUSTER_NAME> [--region <AWS_REGION>]
```
- `--cluster` (required): The name of the ECS cluster to inspect.
- `--region` (optional): The AWS region (default: `us-east-1`).

## Purpose

- Fetches all ECS services in the specified cluster.
- Lists all running tasks for each service.
- Retrieves the container images used by each task definition.
- Maps and displays which services use each unique image URI.
- Runs in parallel to speed up results

## Example Output

```
Fetching services in cluster 'my-ecs-cluster'...
Fetching task ARNs for each service...
Listing tasks  100% |██████████████████████████████████████████████████████████████████████████| (50/50, 42 it/s)
Describing tasks to get task definitions...
Describing tasks 100% |███████████████████████████████████████████████████████████████████████| (1/1, 42 it/min)
Describing task definitions to get container images...
Describing task defs 100% |███████████████████████████████████████████████████████████████████| (50/50, 42 it/s)

Unique container image URIs and services using them:
123456789012.dkr.ecr.us-east-1.amazonaws.com/my-app:latest
  Services:
    - my-app-service
    - another-service

123456789012.dkr.ecr.us-east-1.amazonaws.com/worker:1.2.3
  Services:
    - worker-service
```
