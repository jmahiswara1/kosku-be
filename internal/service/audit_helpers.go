package service

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kosku/backend/internal/repository"
	"github.com/sqlc-dev/pqtype"
)

// auditLogParams builds a CreateAuditLogParams with proper nullable types.
// entityID may be uuid.Nil to indicate no entity.
func auditLogParams(actorID uuid.UUID, action, entityType string, entityID uuid.UUID, metadata map[string]string) repository.CreateAuditLogParams {
	meta, _ := json.Marshal(metadata)
	params := repository.CreateAuditLogParams{
		ActorID:    actorID,
		Action:     action,
		EntityType: entityType,
		Metadata:   pqtype.NullRawMessage{RawMessage: meta, Valid: len(meta) > 0},
	}
	if entityID != uuid.Nil {
		params.EntityID = uuid.NullUUID{UUID: entityID, Valid: true}
	}
	return params
}
