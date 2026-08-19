// Package boundary contains transport-facing helpers shared by contract
// adapters. It does not define a second event or enum catalog.
package boundary

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
)

const (
	EventIDHeader        = "X-Curriculum-Studio-Event-Id"
	EventTypeHeader      = "X-Curriculum-Studio-Event-Type"
	EventSignatureHeader = "X-Curriculum-Studio-Signature"
)

// EventTypeWireName derives the public dotted event name from the generated
// protobuf enum descriptor. The generated enum remains the only closed set;
// this helper contains only the transport spelling rule.
func EventTypeWireName(eventType v1.EventType) (string, error) {
	name := eventType.Descriptor().Values().ByNumber(eventType.Number())
	if name == nil || eventType == v1.EventType_EVENT_TYPE_UNSPECIFIED {
		return "", fmt.Errorf("unknown event type %s", eventType)
	}
	suffix := strings.TrimPrefix(string(name.Name()), "EVENT_TYPE_")
	parts := strings.Split(strings.ToLower(suffix), "_")
	if len(parts) < 2 {
		return "", fmt.Errorf("event type %s has no verb", name.Name())
	}
	return strings.Join(parts[:len(parts)-1], "_") + "." + parts[len(parts)-1], nil
}

// EventDeliveryHeaders returns the documented webhook headers. The signature
// is HMAC-SHA256 over the exact payload bytes and is intentionally omitted when
// secret is empty; platform configuration decides whether delivery is allowed.
func EventDeliveryHeaders(event *v1.DomainEvent, payload, secret []byte) (http.Header, error) {
	if event == nil || event.GetId() == "" {
		return nil, fmt.Errorf("event id is required")
	}
	typeName, err := EventTypeWireName(event.GetType())
	if err != nil {
		return nil, err
	}
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	h.Set(EventIDHeader, event.GetId())
	h.Set(EventTypeHeader, typeName)
	if len(secret) > 0 {
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write(payload)
		h.Set(EventSignatureHeader, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	return h, nil
}
