package ingest

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// Rational is a frame rate or a ratio.
type Rational struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

// Float is the value as a float64; zero for a zero denominator.
func (r Rational) Float() float64 {
	if r.Den == 0 {
		return 0
	}
	return float64(r.Num) / float64(r.Den)
}

func (r Rational) String() string {
	return strconv.FormatInt(r.Num, 10) + "/" + strconv.FormatInt(r.Den, 10)
}

// ParseRational reads "30000/1001" as ffprobe prints it. A zero numerator or denominator
// ("0/0") is not a rate.
func ParseRational(s string) (Rational, bool) {
	n, d, ok := strings.Cut(s, "/")
	if !ok {
		return Rational{}, false
	}
	num, err1 := strconv.ParseInt(n, 10, 64)
	den, err2 := strconv.ParseInt(d, 10, 64)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return Rational{}, false
	}
	return Rational{num, den}, true
}

func within(r Rational, lo, hi float64) bool {
	f := r.Float()
	return r.Den > 0 && f >= lo && f <= hi
}

// MeasureRate is step 2 of SDD-011 3.3: the frame rate the packet timestamps say, before
// any halving or snapping. With at least 10 positive deltas it is 1 over the mean of the
// regular deltas (those within 0.6 to 1.4 times the median); with fewer it is r_frame_rate
// when that lies in [1, 120], else avg_frame_rate, else 30 with the warning frame_rate_unknown.
// The second result is the fraction of deltas outside the regular set (0 when too few).
func MeasureRate(pts []float64, r, a Rational) (v float64, outliers float64, warnings []string) {
	s := slices.Clone(pts)
	slices.Sort(s)
	var deltas []float64
	for i := 0; i+1 < len(s); i++ {
		if d := s[i+1] - s[i]; d > 0 {
			deltas = append(deltas, d)
		}
	}
	if len(deltas) >= 10 {
		sorted := slices.Clone(deltas)
		slices.Sort(sorted)
		var med float64
		if n := len(sorted); n%2 == 1 {
			med = sorted[n/2]
		} else {
			med = (sorted[n/2-1] + sorted[n/2]) / 2
		}
		var sum float64
		cnt := 0
		for _, d := range deltas {
			if d >= 0.6*med && d <= 1.4*med {
				sum += d
				cnt++
			}
		}
		if cnt > 0 {
			return 1 / (sum / float64(cnt)), float64(len(deltas)-cnt) / float64(len(deltas)), nil
		}
	}
	switch {
	case within(r, 1, 120):
		return r.Float(), 0, nil
	case within(a, 1, 120):
		return a.Float(), 0, nil
	}
	return 30, 0, []string{"frame_rate_unknown"}
}

var snapFamilies = []int64{12, 15, 24, 30, 48, 60, 120}

// SnapRate is steps 3 and 4 of SDD-011 3.3: halve while above MaxFPS (whole frames are
// dropped, never invented), then snap to i/1 or (i*1000)/1001. The warning
// irregular_frame_rate is added when the result is more than 2% from the measurement.
func SnapRate(v float64) (Rational, []string) {
	// A 0.5% tolerance: a 60 fps source whose timestamps were rounded to milliseconds measures
	// 60.0003, and halving that to 30 would drop half of its frames.
	for v > MaxFPS*1.005 {
		v /= 2
	}
	i := max(1, int64(math.Round(v)))
	res := Rational{i, 1}
	if slices.Contains(snapFamilies, i) && math.Abs(v-float64(i)*1000/1001) < math.Abs(v-float64(i)) {
		res = Rational{i * 1000, 1001}
	}
	var warn []string
	if math.Abs(v-res.Float())/res.Float() > 0.02 {
		warn = []string{"irregular_frame_rate"}
	}
	return res, warn
}

// NominalRate is the frame rate of what the capture device was doing, not what a container
// field claims (SDD-011 3.3): the measurement of MeasureRate, halved and snapped.
func NominalRate(pts []float64, r, a Rational) (Rational, []string) {
	v, _, w := MeasureRate(pts, r, a)
	res, w2 := SnapRate(v)
	return res, append(w, w2...)
}

// IsVFR is step 5 of 3.3: informational, true when the average and the r_frame_rate differ by
// more than 2%, or when more than 1% of the deltas lie outside the regular set.
func IsVFR(r, a Rational, outliers float64) bool {
	if outliers > 0.01 {
		return true
	}
	if r.Float() == 0 || a.Float() == 0 {
		return false
	}
	return math.Abs(r.Float()-a.Float())/r.Float() > 0.02
}
