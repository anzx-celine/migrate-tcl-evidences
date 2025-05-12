package main

import (
	"github.com/lib/pq"
	"time"
)

type Evidence struct {
	Title              string         `json:"title"`
	EvidenceTypeId     string         `json:"evidenceTypeId" db:"evidence_type_id"`
	EvidenceTypeTitle  string         `json:"evidenceTypeTitle" db:"evidence_type_title"`
	Content            string         `json:"content"`
	ProvidedAt         time.Time      `json:"providedAt" db:"provided_at"`
	ProvidedBy         string         `json:"providedBy" db:"provided_by"`
	Status             string         `json:"status"`
	ExpiresAt          time.Time      `json:"expiresAt" db:"expires_at"`
	ControlId          string         `json:"controlId" db:"control_id"`
	ControlComponentId string         `json:"controlComponentId" db:"control_component_id"`
	AssetId            string         `json:"assetId" db:"asset_id"`
	ComponentId        *string        `json:"componentId,omitempty" db:"component_id"`
	AttachmentNames    pq.StringArray `json:"attachmentNames,omitempty" db:"attachment_names"`
	ControlType        string         `json:"controlType" db:"control_type"`
}
