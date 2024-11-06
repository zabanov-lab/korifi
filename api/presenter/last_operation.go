package presenter

import (
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/tools"
)

type LastOperationResponse struct {
	Type        string `json:"type"`
	State       string `json:"state"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

//counterfeiter:generate -o fake -fake-name RecordWithLastOperation . RecordWithLastOperation
type RecordWithLastOperation interface {
	GetCreatedAt() time.Time
	GetUpdatedAt() *time.Time
	GetDeletedAt() *time.Time
	GetState() repositories.RecordState
}

func ForLastOperation(record RecordWithLastOperation) LastOperationResponse {
	return LastOperationResponse{
		Type:        toLastResponseType(record),
		State:       toLastResponseState(record),
		Description: record.GetState().Description,
		CreatedAt:   formatTimestamp(tools.PtrTo(record.GetCreatedAt())),
		UpdatedAt:   formatTimestamp(record.GetUpdatedAt()),
	}
}

func toLastResponseType(record RecordWithLastOperation) string {
	if record.GetDeletedAt() != nil {
		return "delete"
	}

	if record.GetUpdatedAt() != nil && *record.GetUpdatedAt() != record.GetCreatedAt() {
		return "update"
	}

	return "create"
}

func toLastResponseState(record RecordWithLastOperation) string {
	if record.GetState().Value == model.CFResourceStateFailed {
		return "failed"
	}

	if record.GetState().Value == model.CFResourceStateReady {
		return "succeeded"
	}

	return "in progress"
}
