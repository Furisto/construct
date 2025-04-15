package conv

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	dpb "google.golang.org/genproto/googleapis/type/decimal"
)

type MissingRelatedEntityError struct {
	Entity string
}

func (e *MissingRelatedEntityError) Error() string {
	return fmt.Sprintf("missing related entity: %s", e.Entity)
}

func ConvertUUIDToString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func ConvertUUIDPtrToStringPtr(id uuid.UUID) *string {
	if id == uuid.Nil {
		return nil
	}
	return strPtr(id.String())
}

func ConvertUUIDPtrToString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func ConvertStringToUUID(id string) (uuid.UUID, error) {
	return uuid.Parse(id)
}

func ConvertTimeToTimestamp(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

func ConvertTimestampToTime(t *timestamppb.Timestamp) time.Time {
	return t.AsTime()
}

func strPtr(s string) *string {
	return &s
}

func Float64ToProtoDecimal(f float64) *dpb.Decimal {
	return &dpb.Decimal{
		Value: fmt.Sprintf("%f", f),
	}
}
