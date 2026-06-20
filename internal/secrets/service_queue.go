package secrets

import (
	"github.com/moby/moby/api/types/swarm"
)

type TaskQueue struct {
	services    map[string]*ServiceTask
	serviceList []*ServiceTask

	secrets []UpdatedSecret
}

type ServiceTask struct {
	Service swarm.Service
	Secrets map[string]UpdatingServiceSecret
}

type UpdatedSecret struct {
	Name  string
	ID    string
	Path  string
	Value []byte

	ExternalID string
}

type UpdatingServiceSecret struct {
	Name string
	ID   string
	Path string
}

func newServiceQueue() *TaskQueue {
	return &TaskQueue{
		services: make(map[string]*ServiceTask),
		secrets:  make([]UpdatedSecret, 0),
	}
}

func (q *TaskQueue) PushService(service swarm.Service, secret UpdatingServiceSecret) {
	if _, ok := q.services[service.ID]; !ok {
		task := &ServiceTask{
			Service: service,
			Secrets: make(map[string]UpdatingServiceSecret),
		}

		q.services[service.ID] = task
		q.serviceList = append(q.serviceList, task)
	}

	q.services[service.ID].Secrets[secret.Path] = secret
}

func (q *TaskQueue) PushSecret(secret UpdatedSecret) {
	q.secrets = append(q.secrets, secret)
}
