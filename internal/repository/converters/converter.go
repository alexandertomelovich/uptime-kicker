package converters

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func SafeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func SafeBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func TimeToPgTimestamp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func PgTimestampToPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func IntPtrToInt32Ptr(i *int) *int32 {
    if i == nil {
        return nil
    }
    v := int32(*i)
    return &v
}

func NumericToFloat64(n pgtype.Numeric) float64 {
    if !n.Valid {
        return 0
    }

    var s string
    err := n.Scan(&s)
    if err != nil {
        return 0
    }

    var f float64
    _, err = fmt.Sscan(s, &f)
    return f
}

func Float64ToNumeric(f float64) pgtype.Numeric {
    var n pgtype.Numeric
    
    err := n.Scan(fmt.Sprintf("%f", f))
    if err != nil {
        return pgtype.Numeric{}
    }
    
    return n
}