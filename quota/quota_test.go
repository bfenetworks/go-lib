// Copyright (c) 2026 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package quota

import (
	"math"
	"testing"
)

func TestIsRMB(t *testing.T) {
	cases := []struct {
		unit string
		want bool
	}{
		{"RMB", true},
		{"total_token", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsRMB(c.unit); got != c.want {
			t.Errorf("IsRMB(%q) = %v, want %v", c.unit, got, c.want)
		}
	}
}

func TestToRedisValue(t *testing.T) {
	cases := []struct {
		quota float64
		unit  string
		want  int64
	}{
		{1.0, UnitRMB, 1e8},
		{0.000001, UnitRMB, 100},
		{0.00000001, UnitRMB, 1},
		{100.0, UnitTotalToken, 100},
		{1.5, UnitTotalToken, 1},
	}
	for _, c := range cases {
		if got := ToRedisValue(c.quota, c.unit); got != c.want {
			t.Errorf("ToRedisValue(%v, %q) = %d, want %d", c.quota, c.unit, got, c.want)
		}
	}
}

func TestFromRedisValue(t *testing.T) {
	cases := []struct {
		value int64
		unit  string
		want  float64
	}{
		{1e8, UnitRMB, 1.0},
		{100, UnitRMB, 0.000001},
		{1, UnitRMB, 0.00000001},
		{100, UnitTotalToken, 100.0},
	}
	for _, c := range cases {
		got := FromRedisValue(c.value, c.unit)
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("FromRedisValue(%d, %q) = %v, want %v", c.value, c.unit, got, c.want)
		}
	}
}

func TestPtrToRedisValue(t *testing.T) {
	q := 1.23
	u := UnitRMB
	got := PtrToRedisValue(&q, &u)
	want := int64(1.23 * RmbPrecision)
	if got != want {
		t.Errorf("PtrToRedisValue(%v, %q) = %d, want %d", q, u, got, want)
	}

	if got := PtrToRedisValue(nil, &u); got != 0 {
		t.Errorf("PtrToRedisValue(nil, ...) = %d, want 0", got)
	}
}

func TestMaxRMBQuota(t *testing.T) {
	// MaxRMBQuota = 90,000,000.00 yuan
	// Its fixed-point representation must not exceed Lua number's exact
	// integer limit (2^53 - 1 ≈ 9.007e15).
	fixed := RmbToFixedPoint(MaxRMBQuota)
	if fixed > (1<<53)-1 {
		t.Errorf("MaxRMBQuota fixed-point value %d exceeds 2^53-1", fixed)
	}
	if MaxRMBQuota != 90000000.0 {
		t.Errorf("MaxRMBQuota = %v, want 90000000.0", MaxRMBQuota)
	}
}

func TestRmbToFixedPoint(t *testing.T) {
	cases := []struct {
		yuan float64
		want int64
	}{
		{1.0, 1e8},
		{0.000001, 100},
		{0.00000001, 1},
	}
	for _, c := range cases {
		if got := RmbToFixedPoint(c.yuan); got != c.want {
			t.Errorf("RmbToFixedPoint(%v) = %d, want %d", c.yuan, got, c.want)
		}
	}
}

func TestFixedPointToRmb(t *testing.T) {
	cases := []struct {
		value int64
		want  float64
	}{
		{1e8, 1.0},
		{100, 0.000001},
		{1, 0.00000001},
	}
	for _, c := range cases {
		got := FixedPointToRmb(c.value)
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("FixedPointToRmb(%d) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestRmbToFixedPointRounding(t *testing.T) {
	// Values beyond 8 decimal places must be rounded, not truncated.
	if got := RmbToFixedPoint(7.6234102728e-08); got != 8 {
		t.Errorf("RmbToFixedPoint(7.6234102728e-08) = %d, want 8 (rounded)", got)
	}
	// Exactly representable values keep their fixed-point value.
	if got := RmbToFixedPoint(0.00000001); got != 1 {
		t.Errorf("RmbToFixedPoint(0.00000001) = %d, want 1", got)
	}
}

func TestCalcCostUnits(t *testing.T) {
	cases := []struct {
		usage int64
		price float64
		want  int64
	}{
		// zero usage or zero price costs nothing
		{0, 0.000002, 0},
		{-1, 0.000002, 0},
		{1000, 0, 0},
		// prices with <= 8 decimals are lossless
		{1000, 0.000002, 200000},
		{1000, 0.00000001, 1000},
		// rounding, not truncation: 10 * 0.15 = 1.5 -> 2
		{10, 1.5e-9, 2},
		// boyue catalog prices with 10-12 decimal places
		// 1e6 * 7.6234102728e-08 * 1e8 = 7623410.2728
		{1000000, 7.6234102728e-08, 7623410},
		// 1000 * 4.141631732e-06 * 1e8 = 414163.1732
		{1000, 4.141631732e-06, 414163},
	}
	for _, c := range cases {
		if got := CalcCostUnits(c.usage, c.price); got != c.want {
			t.Errorf("CalcCostUnits(%d, %v) = %d, want %d", c.usage, c.price, got, c.want)
		}
	}
}

func TestCalcCostUnitsConsistentWithFixedPoint(t *testing.T) {
	// For prices with at most 8 decimal places, per-item conversion must
	// agree exactly with converting the price first and multiplying as
	// integers (the pre-v0.6 behavior).
	prices := []float64{0.000002, 0.000008, 0.0000005, 0.03, 0.5}
	usages := []int64{1, 7, 1000, 12345, 1000000}
	for _, p := range prices {
		fp := RmbToFixedPoint(p)
		for _, u := range usages {
			if got, want := CalcCostUnits(u, p), u*fp; got != want {
				t.Errorf("CalcCostUnits(%d, %v) = %d, want %d (= usage * RmbToFixedPoint)",
					u, p, got, want)
			}
		}
	}
}
