package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

const proxyNetwork = "tidefly_proxy"

const TaskServiceHeal = "service:heal"

type ServiceHealPayload struct {
	ServiceName string `json:"service_name"`
	ContainerID string `json:"container_id"`
	Reason      string `json:"reason"`
}

func EnqueueServiceHeal(client *asynq.Client, serviceName, containerID, reason string) error {
	data, err := json.Marshal(ServiceHealPayload{
		ServiceName: serviceName,
		ContainerID: containerID,
		Reason:      reason,
	})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(
		asynq.NewTask(TaskServiceHeal, data,
			asynq.MaxRetry(2),
			asynq.Timeout(2*time.Minute),
			asynq.Queue("critical"),
			asynq.TaskID(fmt.Sprintf("heal:%s", serviceName)),
		),
	)
	if err != nil && strings.Contains(err.Error(), "task ID already exists") {
		return nil
	}
	return err
}
