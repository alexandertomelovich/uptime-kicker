// Тесты для функций-конвертеров между domain-типами и pgtype-типами.
//
// Все функции чистые (без побочных эффектов и внешних зависимостей), поэтому
// проверяются таблично. Особое внимание — паре NumericToFloat64/Float64ToNumeric:
// там реальная логика (Scan/Sscan) и обработка невалидных значений.
package converters

import (
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ptr — маленький хелпер для получения указателя на значение.
func ptr[T any](v T) *T { return &v }

// --- SafeString ---

func TestSafeString(t *testing.T) {
	if got := SafeString(nil); got != "" {
		t.Errorf("SafeString(nil) = %q, want empty", got)
	}
	if got := SafeString(ptr("")); got != "" {
		t.Errorf("SafeString(пустой) = %q, want empty", got)
	}
	if got := SafeString(ptr("hello")); got != "hello" {
		t.Errorf("SafeString(hello) = %q, want hello", got)
	}
}

// --- SafeBool ---

func TestSafeBool(t *testing.T) {
	if got := SafeBool(nil); got != false {
		t.Errorf("SafeBool(nil) = %v, want false", got)
	}
	if got := SafeBool(ptr(false)); got != false {
		t.Errorf("SafeBool(false) = %v, want false", got)
	}
	if got := SafeBool(ptr(true)); got != true {
		t.Errorf("SafeBool(true) = %v, want true", got)
	}
}

// --- TimeToPgTimestamp ---

func TestTimeToPgTimestamp(t *testing.T) {
	// nil -> невалидная метка времени.
	if got := TimeToPgTimestamp(nil); got.Valid {
		t.Errorf("TimeToPgTimestamp(nil).Valid = true, want false")
	}

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := TimeToPgTimestamp(&now)
	if !got.Valid {
		t.Fatal("TimeToPgTimestamp(&t).Valid = false, want true")
	}
	if !got.Time.Equal(now) {
		t.Errorf("Time = %v, want %v", got.Time, now)
	}
}

// --- PgTimestampToPtr ---

func TestPgTimestampToPtr(t *testing.T) {
	// Невалидная метка -> nil.
	if got := PgTimestampToPtr(pgtype.Timestamptz{Valid: false}); got != nil {
		t.Errorf("PgTimestampToPtr(invalid) = %v, want nil", got)
	}

	now := time.Date(2026, 6, 7, 8, 9, 10, 0, time.UTC)
	got := PgTimestampToPtr(pgtype.Timestamptz{Time: now, Valid: true})
	if got == nil {
		t.Fatal("PgTimestampToPtr(valid) = nil, want non-nil")
	}
	if !got.Equal(now) {
		t.Errorf("time = %v, want %v", *got, now)
	}
}

// Round-trip: time -> pgtype -> *time сохраняет значение.
func TestTimestamp_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond) // избегаем потерь точности
	got := PgTimestampToPtr(TimeToPgTimestamp(&now))
	if got == nil {
		t.Fatal("round-trip вернул nil")
	}
	if !got.Equal(now) {
		t.Errorf("round-trip = %v, want %v", *got, now)
	}
}

// --- IntPtrToInt32Ptr ---

func TestIntPtrToInt32Ptr(t *testing.T) {
	if got := IntPtrToInt32Ptr(nil); got != nil {
		t.Errorf("IntPtrToInt32Ptr(nil) = %v, want nil", got)
	}

	cases := []int{0, 1, -5, math.MaxInt32, math.MinInt32}
	for _, v := range cases {
		got := IntPtrToInt32Ptr(ptr(v))
		if got == nil {
			t.Fatalf("IntPtrToInt32Ptr(%d) = nil, want non-nil", v)
		}
		if *got != int32(v) {
			t.Errorf("IntPtrToInt32Ptr(%d) = %d, want %d", v, *got, int32(v))
		}
	}
}

// --- Float64ToNumeric / NumericToFloat64 ---

// round-trip float64 -> Numeric -> float64 сохраняет значение.
func TestFloat64ToNumeric_RoundTrip(t *testing.T) {
	cases := []float64{0, 1, -1, 3.14159, 100.5, 0.0001, 1234567.89}
	for _, want := range cases {
		n := Float64ToNumeric(want)
		if !n.Valid {
			t.Errorf("Float64ToNumeric(%v).Valid = false, want true", want)
			continue
		}
		got := NumericToFloat64(n)
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("round-trip %v -> %v, want %v", want, got, want)
		}
	}
}

// Невалидный Numeric -> 0.
func TestNumericToFloat64_Invalid(t *testing.T) {
	if got := NumericToFloat64(pgtype.Numeric{Valid: false}); got != 0 {
		t.Errorf("NumericToFloat64(invalid) = %v, want 0", got)
	}
}

// Валидный Numeric, читаемый как строка, корректно парсится.
func TestNumericToFloat64_Valid(t *testing.T) {
	var n pgtype.Numeric
	if err := n.Scan("42.5"); err != nil {
		t.Fatalf("не удалось подготовить Numeric: %v", err)
	}

	if got := NumericToFloat64(n); math.Abs(got-42.5) > 1e-9 {
		t.Errorf("NumericToFloat64 = %v, want 42.5", got)
	}
}

// Нулевое значение float64 даёт валидный Numeric, равный нулю.
func TestFloat64ToNumeric_Zero(t *testing.T) {
	n := Float64ToNumeric(0)
	if !n.Valid {
		t.Fatal("Float64ToNumeric(0).Valid = false, want true")
	}
	if got := NumericToFloat64(n); got != 0 {
		t.Errorf("NumericToFloat64 = %v, want 0", got)
	}
}
