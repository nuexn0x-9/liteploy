// Package deployment implements the LITEPLOY deployment state machine.
//
// A Deployment is a record of one execution of the build+run pipeline for
// an Application. The state machine progresses through:
//
//	QUEUED → PREPARING → BUILDING → STARTING → HEALTH_CHECK → ROUTING → SUCCESS
//
// Any state can transition to FAILED. Active deployments can be CANCELLED.
package deployment

import (
	"time"
)

// Status represents the current state of a Deployment.
type Status string

const (
	StatusQueued      Status = "queued"
	StatusPreparing   Status = "preparing"
	StatusBuilding    Status = "building"
	StatusStarting    Status = "starting"
	StatusHealthCheck Status = "health_check"
	StatusRouting     Status = "routing"
	StatusSuccess     Status = "success"
	StatusFailed      Status = "failed"
	StatusCancelling  Status = "cancelling"
	StatusCancelled   Status = "cancelled"
)

// IsTerminal reports whether the status is a final state (no further transitions).
func (s Status) IsTerminal() bool {
	return s == StatusSuccess || s == StatusFailed || s == StatusCancelled
}

// IsActive reports whether the deployment is currently executing.
func (s Status) IsActive() bool {
	return s == StatusQueued || s == StatusPreparing || s == StatusBuilding ||
		s == StatusStarting || s == StatusHealthCheck || s == StatusRouting ||
		s == StatusCancelling
}

// Deployment is the persisted record of a single deployment run.
type Deployment struct {
	// ID is a sequential identifier (e.g., "0001") within the application.
	ID string `json:"id"`

	// AppID links the deployment to its application.
	AppID string `json:"app_id"`

	// Status is the current state machine position.
	Status Status `json:"status"`

	// CreatedAt is when the deployment was enqueued.
	CreatedAt time.Time `json:"created_at"`

	// StartedAt is when the worker began executing (nil if still queued).
	StartedAt *time.Time `json:"started_at,omitempty"`

	// FinishedAt is when the deployment entered a terminal state.
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Duration is the total execution duration in seconds.
	Duration float64 `json:"duration_seconds,omitempty"`

	// LogPath is the path to the stored deployment logs.
	LogPath string `json:"log_path,omitempty"`

	// RollbackTo, if set, instructs the pipeline to skip build/pull
	// and deploy this specific ImageID instead.
	RollbackTo string `json:"rollback_to,omitempty"`

	// CommitSHA is the Git commit hash (empty for image deployments).
	CommitSHA string `json:"commit_sha,omitempty"`

	// ImageID is the Docker image used or built.
	ImageID string `json:"image_id,omitempty"`

	// ContainerID is the container created by this deployment.
	ContainerID string `json:"container_id,omitempty"`

	// OldContainerID is the previous container (kept until new passes health check).
	OldContainerID string `json:"old_container_id,omitempty"`

	// Error is a human-readable error message (set when Status = FAILED).
	// Must never contain secrets.
	Error string `json:"error,omitempty"`

	// Stage is the current step name within the active state (for UI display).
	Stage string `json:"stage,omitempty"`

	// TriggeredBy describes what caused this deployment (e.g., "manual", "webhook").
	TriggeredBy string `json:"triggered_by,omitempty"`
}

// Transition attempts to move the deployment to the next valid state.
// Invalid transitions return an error and leave the state unchanged.
func (d *Deployment) Transition(next Status) error {
	if !validTransition(d.Status, next) {
		return newTransitionError(d.Status, next)
	}
	d.Status = next
	return nil
}

// Fail marks the deployment as failed with an error message.
// Error messages must not contain secrets — the caller is responsible.
func (d *Deployment) Fail(stage, errMsg string) {
	d.Status = StatusFailed
	d.Stage = stage
	d.Error = errMsg
	now := time.Now().UTC()
	d.FinishedAt = &now
	if d.StartedAt != nil {
		d.Duration = now.Sub(*d.StartedAt).Seconds()
	}
}

// Succeed marks the deployment as successful.
func (d *Deployment) Succeed() {
	d.Status = StatusSuccess
	d.Stage = ""
	now := time.Now().UTC()
	d.FinishedAt = &now
	if d.StartedAt != nil {
		d.Duration = now.Sub(*d.StartedAt).Seconds()
	}
}

// Start marks the deployment as begun.
func (d *Deployment) Start() {
	now := time.Now().UTC()
	d.StartedAt = &now
}

// validTransition enforces the state machine graph.
func validTransition(from, to Status) bool {
	// Any state can go to FAILED or CANCELLING (from non-terminal).
	if to == StatusFailed && !from.IsTerminal() {
		return true
	}
	if to == StatusCancelling && from.IsActive() {
		return true
	}

	allowed := map[Status][]Status{
		StatusQueued:      {StatusPreparing, StatusFailed, StatusCancelled},
		StatusPreparing:   {StatusBuilding, StatusFailed},
		StatusBuilding:    {StatusStarting, StatusFailed},
		StatusStarting:    {StatusHealthCheck, StatusFailed},
		StatusHealthCheck: {StatusRouting, StatusFailed},
		StatusRouting:     {StatusSuccess, StatusFailed},
		StatusCancelling:  {StatusCancelled},
	}

	for _, valid := range allowed[from] {
		if to == valid {
			return true
		}
	}
	return false
}

// TransitionError is returned for invalid state transitions.
type TransitionError struct {
	From Status
	To   Status
}

func (e *TransitionError) Error() string {
	return "deployment: invalid transition " + string(e.From) + " → " + string(e.To)
}

func newTransitionError(from, to Status) *TransitionError {
	return &TransitionError{From: from, To: to}
}
