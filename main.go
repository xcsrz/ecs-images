package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/schollz/progressbar/v3"
)

type ServiceTask struct {
	ServiceName string
	ServiceArn  string
}

type TaskInfo struct {
	TaskArn           string
	TaskDefinitionArn string
	ServiceName       string
}

func main() {
	cluster := flag.String("cluster", "", "ECS cluster name (required)")
	region := flag.String("region", "us-east-1", "AWS region (default: us-east-1)")
	flag.Parse()

	if *cluster == "" {
		fmt.Println("--cluster is required")
		os.Exit(1)
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	client := ecs.NewFromConfig(cfg)

	fmt.Printf("Fetching services in cluster '%s'...\n", *cluster)
	serviceArns := []string{}
	paginator := ecs.NewListServicesPaginator(client, &ecs.ListServicesInput{Cluster: cluster})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("failed to list services: %v", err)
		}
		serviceArns = append(serviceArns, page.ServiceArns...)
	}
	if len(serviceArns) == 0 {
		fmt.Println("No services found.")
		return
	}

	serviceNames := make([]string, len(serviceArns))
	serviceNameToArn := make(map[string]string)
	for i, arn := range serviceArns {
		parts := splitArn(arn)
		serviceNames[i] = parts[len(parts)-1]
		serviceNameToArn[serviceNames[i]] = arn
	}

	fmt.Println("Fetching task ARNs for each service...")
	taskArns := []string{}
	taskToService := make(map[string]string)

	// Semaphore worker pool for listing tasks
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	var mu sync.Mutex
	bar := progressbar.Default(int64(len(serviceNames)), "Listing tasks")

	for _, serviceName := range serviceNames {
		wg.Add(1)
		sem <- struct{}{}
		go func(svcName string) {
			defer wg.Done()
			defer func() { <-sem }()
			resp, err := client.ListTasks(ctx, &ecs.ListTasksInput{
				Cluster:     cluster,
				ServiceName: aws.String(svcName),
			})
			if err == nil {
				mu.Lock()
				taskArns = append(taskArns, resp.TaskArns...)
				for _, t := range resp.TaskArns {
					taskToService[t] = svcName
				}
				mu.Unlock()
			}
			bar.Add(1)
		}(serviceName)
	}
	wg.Wait()
	bar.Finish()

	if len(taskArns) == 0 {
		fmt.Println("No tasks found.")
		return
	}

	fmt.Println("Describing tasks to get task definitions...")
	taskDefArns := make(map[string]struct{})
	taskDefToService := make(map[string]map[string]struct{})
	bar = progressbar.Default(int64((len(taskArns)+99)/100), "Describing tasks")
	for i := 0; i < len(taskArns); i += 100 {
		end := i + 100
		if end > len(taskArns) {
			end = len(taskArns)
		}
		resp, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: cluster,
			Tasks:   taskArns[i:end],
		})
		if err != nil {
			continue
		}
		for _, task := range resp.Tasks {
			taskDefArns[*task.TaskDefinitionArn] = struct{}{}
			serviceName := taskToService[*task.TaskArn]
			if _, ok := taskDefToService[*task.TaskDefinitionArn]; !ok {
				taskDefToService[*task.TaskDefinitionArn] = make(map[string]struct{})
			}
			taskDefToService[*task.TaskDefinitionArn][serviceName] = struct{}{}
		}
		bar.Add(1)
	}
	bar.Finish()

	fmt.Println("Describing task definitions to get container images...")
	imageToServices := make(map[string]map[string]struct{})

	taskDefList := make([]string, 0, len(taskDefArns))
	for arn := range taskDefArns {
		taskDefList = append(taskDefList, arn)
	}
	bar = progressbar.Default(int64(len(taskDefList)), "Describing task defs")
	sem = make(chan struct{}, 5)
	wg = sync.WaitGroup{}
	mu = sync.Mutex{}
	for _, taskDefArn := range taskDefList {
		wg.Add(1)
		sem <- struct{}{}
		go func(tdArn string) {
			defer wg.Done()
			defer func() { <-sem }()
			resp, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
				TaskDefinition: aws.String(tdArn),
			})
			if err == nil {
				services := taskDefToService[tdArn]
				for _, container := range resp.TaskDefinition.ContainerDefinitions {
					image := *container.Image
					mu.Lock()
					if _, ok := imageToServices[image]; !ok {
						imageToServices[image] = make(map[string]struct{})
					}
					for svc := range services {
						imageToServices[image][svc] = struct{}{}
					}
					mu.Unlock()
				}
			}
			bar.Add(1)
		}(taskDefArn)
	}
	wg.Wait()
	bar.Finish()

	fmt.Println("\nUnique container image URIs and services using them:")
	for image, services := range imageToServices {
		fmt.Println(image)
		if len(services) > 0 {
			fmt.Println("  Services:")
			for svc := range services {
				fmt.Printf("    - %s\n", svc)
			}
		} else {
			fmt.Println("  No active services using this image")
		}
		fmt.Println()
	}
}

func splitArn(arn string) []string {
	return func() []string {
		var out []string
		start := 0
		for i := 0; i < len(arn); i++ {
			if arn[i] == '/' {
				out = append(out, arn[start:i])
				start = i + 1
			}
		}
		if start < len(arn) {
			out = append(out, arn[start:])
		}
		return out
	}()
}
