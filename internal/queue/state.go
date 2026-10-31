package queue

type QueueState string

const (
	QueueStateActive      QueueState = "active"
	QueueStatePaused      QueueState = "paused"
	QueueStateDraining    QueueState = "draining"
	QueueStateMaintenance QueueState = "maintenance"
)

func IsValidQueueState(s string) bool {
	switch QueueState(s) {
	case QueueStateActive, QueueStatePaused, QueueStateDraining, QueueStateMaintenance:
		return true
	default:
		return false
	}
}

func (s QueueState) String() string {
	return string(s)
}

func (s QueueState) CanClaimJobs() bool {
	return s == QueueStateActive
}

