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

// Package quota provides unit conversion between quota values and
// Redis fixed-point integers.
//
// For RMB quotas, values are stored in Redis as integers with a fixed
// precision of 1e-8 yuan per unit, so that Lua scripts can operate on
// integers only and avoid floating point errors.
//
// Model prices are kept as float64 (yuan per token) until a request
// cost is computed; CalcCostUnits converts usage * price into a
// fixed-point integer for Redis deduction.
package quota

import "math"

const (
	// UnitTotalToken is the unit for token-based quotas.
	UnitTotalToken = "total_token"

	// UnitRMB is the unit for RMB (yuan) based quotas.
	UnitRMB = "RMB"
)

// RmbPrecision is the fixed-point precision used when storing RMB
// quotas in Redis. One Redis unit equals 1e-8 yuan.
const RmbPrecision = 1e8

// MaxRMBQuota is the maximum allowed RMB quota value in yuan.
//
// RMB quotas are stored in Redis as fixed-point integers with precision
// RmbPrecision (1e-8 yuan per unit). Lua numbers are IEEE 754 doubles,
// which can exactly represent integers up to 2^53 (~9.007e15). With
// RmbPrecision = 1e8, the theoretical limit is about 90.07 million yuan.
// The business limit is standardized to 90 million yuan to guarantee
// lossless arithmetic in all scenarios.
const MaxRMBQuota = 90000000.0

// IsRMB returns true if the given unit is RMB.
// An empty unit is treated as total_token for backward compatibility.
func IsRMB(unit string) bool {
	return unit == UnitRMB
}

// PtrIsRMB returns true if the given unit pointer is RMB.
// A nil pointer is treated as total_token for backward compatibility.
func PtrIsRMB(unit *string) bool {
	if unit == nil {
		return false
	}
	return *unit == UnitRMB
}

// RmbToFixedPoint converts yuan to a fixed-point integer.
// One returned unit equals 1e-8 yuan.
func RmbToFixedPoint(yuan float64) int64 {
	return int64(math.Round(yuan * RmbPrecision))
}

// FixedPointToRmb converts a fixed-point integer back to yuan.
func FixedPointToRmb(value int64) float64 {
	return float64(value) / RmbPrecision
}

// CalcCostUnits computes the cost of a usage amount at the given price
// (yuan per unit) and returns it as a fixed-point integer.
//
// One returned unit equals 1e-8 yuan, suitable for Redis deduction.
// The price is multiplied by RmbPrecision first so that prices with
// more than 8 decimal places keep their precision until the final
// rounding step. The result is rounded (not truncated) so that errors
// stay unbiased; each billing item should be converted separately and
// the integers summed, instead of summing float costs first.
func CalcCostUnits(usage int64, priceYuan float64) int64 {
	if usage <= 0 || priceYuan <= 0 {
		return 0
	}
	return int64(math.Round(float64(usage) * (priceYuan * RmbPrecision)))
}

// ToRedisValue converts a quota value to a Redis fixed-point integer.
// For RMB quotas, the value is multiplied by RmbPrecision.
// For token quotas, the value is truncated to int64 directly.
func ToRedisValue(quota float64, unit string) int64 {
	if IsRMB(unit) {
		return RmbToFixedPoint(quota)
	}
	return int64(quota)
}

// FromRedisValue converts a Redis fixed-point integer back to a quota value.
// For RMB quotas, the value is divided by RmbPrecision.
// For token quotas, the value is converted to float64 directly.
func FromRedisValue(value int64, unit string) float64 {
	if IsRMB(unit) {
		return FixedPointToRmb(value)
	}
	return float64(value)
}

// PtrToRedisValue converts quota pointers to a Redis fixed-point integer.
// It accepts nil pointers for backward compatibility.
func PtrToRedisValue(quota *float64, unit *string) int64 {
	if quota == nil {
		return 0
	}
	return ToRedisValue(*quota, ptrString(unit))
}

// PtrFromRedisValue converts a Redis fixed-point integer to a float64 value.
// It accepts a nil unit pointer for backward compatibility.
func PtrFromRedisValue(value int64, unit *string) float64 {
	return FromRedisValue(value, ptrString(unit))
}

func ptrString(unit *string) string {
	if unit == nil {
		return ""
	}
	return *unit
}
