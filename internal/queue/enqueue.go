// Package queue provides asynq enqueue helpers shared across packages.
// It deliberately has no imports from internal feature packages to avoid cycles.
package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TaskServiceDeploy   = "service:deploy"
	TaskServiceRedeploy = "service:redeploy"
	TaskServiceUpdate   = "service:update"
	TaskServiceDelete   = "service:delete"
	TaskServiceCleanup  = "service:cleanup"
	TaskServiceHeal     = "service:heal"
	TaskWebhookDeploy   = "webhooks:services"
)

// ── Service deploy ────────────────────────────────────────────────────────────
// APIInput is a self-contained copy of converter.APIInput.
// Duplicated here to avoid manifest/converter → queue → manifest cycle.

type APIInput struct {
	ManifestJSON     string     `json:"manifest_json,omitempty"`
	Image            string     `json:"image,omitempty"`
	ComposeYAML      string     `json:"compose,omitempty"`
	Dockerfile       string     `json:"dockerfile,omitempty"`
	GitURL           string     `json:"git_url,omitempty"`
	Name             string     `json:"name,omitempty"`
	StackName        string     `json:"stack_name,omitempty"`
	ProjectID        string     `json:"project_id,omitempty"`
	Domain           string     `json:"domain,omitempty"`
	Port             int        `json:"port,omitempty"`
	Expose           bool       `json:"expose,omitempty"`
	Branch           string     `json:"branch,omitempty"`
	GitIntegrationID string     `json:"git_integration_id,omitempty"`
	Env              []EnvEntry `json:"env,omitempty"`
	Replicas         int        `json:"replicas,omitempty"`
	Strategy         string     `json:"strategy,omitempty"`
}

type EnvEntry struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	Secret string `json:"secret,omitempty"`
}

func (a *APIInput) ServiceName() string {
	if a.Name != "" {
		return a.Name
	}
	return ""
}

type ServiceDeployPayload struct {
	ServiceID string   `json:"service_id"`
	Input     APIInput `json:"input"`
	GitToken  string   `json:"git_token"`
}

func EnqueueServiceDeploy(client *asynq.Client, serviceID string, input APIInput, gitToken string) error {
	data, err := json.Marshal(ServiceDeployPayload{ServiceID: serviceID, Input: input, GitToken: gitToken})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceDeploy, data,
		asynq.MaxRetry(1), asynq.Timeout(15*time.Minute), asynq.Queue("critical")))
	return err
}

// ── Service redeploy ──────────────────────────────────────────────────────────

type ServiceRedeployPayload struct {
	ServiceID     string `json:"service_id"`
	ImageOverride string `json:"image_override,omitempty"`
	GitToken      string `json:"git_token,omitempty"`
}

func EnqueueServiceRedeploy(client *asynq.Client, serviceID, imageOverride string) error {
	data, err := json.Marshal(ServiceRedeployPayload{ServiceID: serviceID, ImageOverride: imageOverride})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceRedeploy, data,
		asynq.MaxRetry(1), asynq.Timeout(15*time.Minute), asynq.Queue("critical")))
	return err
}

// ── Service update ────────────────────────────────────────────────────────────

type ServiceUpdatePayload struct {
	ServiceID string `json:"service_id"`
	Image     string `json:"image,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Replicas  int    `json:"replicas,omitempty"`
}

func EnqueueServiceUpdate(client *asynq.Client, serviceID, image, domain string, replicas int) error {
	data, err := json.Marshal(ServiceUpdatePayload{ServiceID: serviceID, Image: image, Domain: domain, Replicas: replicas})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceUpdate, data,
		asynq.MaxRetry(1), asynq.Timeout(15*time.Minute), asynq.Queue("critical")))
	return err
}

// ── Service delete ────────────────────────────────────────────────────────────

type ServiceDeletePayload struct {
	ServiceID string `json:"service_id"`
}

func EnqueueServiceDelete(client *asynq.Client, serviceID string) error {
	data, err := json.Marshal(ServiceDeletePayload{ServiceID: serviceID})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceDelete, data,
		asynq.MaxRetry(0), asynq.Timeout(5*time.Minute), asynq.Queue("critical")))
	return err
}

// ── Service cleanup ───────────────────────────────────────────────────────────

type ServiceCleanupPayload struct {
	ServiceName string   `json:"service_name"`
	Images      []string `json:"images,omitempty"`
	Volumes     []string `json:"volumes,omitempty"`
}

func EnqueueServiceCleanup(client interface {
	Enqueue(*asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error)
}, serviceName string, images, volumes []string) error {
	data, err := json.Marshal(ServiceCleanupPayload{
		ServiceName: serviceName, Images: images, Volumes: volumes,
	})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceCleanup, data,
		asynq.MaxRetry(1), asynq.Timeout(2*time.Minute), asynq.Queue("low")))
	return err
}

// ── Service heal ──────────────────────────────────────────────────────────────

type ServiceHealPayload struct {
	ServiceName string `json:"service_name"`
	ContainerID string `json:"container_id"`
	Reason      string `json:"reason"`
}

func EnqueueServiceHeal(client *asynq.Client, serviceName, containerID, reason string) error {
	data, err := json.Marshal(ServiceHealPayload{
		ServiceName: serviceName, ContainerID: containerID, Reason: reason,
	})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskServiceHeal, data,
		asynq.MaxRetry(2), asynq.Timeout(2*time.Minute), asynq.Queue("critical"),
		asynq.TaskID(fmt.Sprintf("heal:%s", serviceName)),
	))
	if err != nil && strings.Contains(err.Error(), "task ID already exists") {
		return nil
	}
	return err
}

// ── Webhook deploy ────────────────────────────────────────────────────────────
// WebhookPayload is a self-contained copy of webhook.Payload fields.
// Duplicated here to avoid webhook → queue → webhook cycle.

type WebhookPayload struct {
	Provider  string `json:"provider"`
	EventType string `json:"event_type"`
	Branch    string `json:"branch"`
	Tag       string `json:"tag"`
	Commit    string `json:"commit"`
	CommitMsg string `json:"commit_msg"`
	PushedBy  string `json:"pushed_by"`
	RepoURL   string `json:"repo_url"`
	RepoName  string `json:"repo_name"`
}

type WebhookDeployPayload struct {
	WebhookID  string         `json:"webhook_id"`
	DeliveryID string         `json:"delivery_id"`
	Payload    WebhookPayload `json:"payload"`
}

func EnqueueWebhookDeploy(client *asynq.Client, webhookID, deliveryID string, p WebhookPayload) error {
	data, err := json.Marshal(WebhookDeployPayload{
		WebhookID: webhookID, DeliveryID: deliveryID, Payload: p,
	})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(asynq.NewTask(TaskWebhookDeploy, data,
		asynq.MaxRetry(2), asynq.Timeout(10*time.Minute), asynq.Queue("webhooks")))
	return err
}
