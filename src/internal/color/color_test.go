package color

import (
	"regexp"
	"testing"
	"time"
)

var hexPattern = regexp.MustCompile(`^#[0-9A-F]{6}$`)

func TestDailyIsStable(t *testing.T) {
	first, err := Daily("2026-11-25")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Daily("2026-11-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Swatches) != len(second.Swatches) {
		t.Fatalf("lengths differ: %d vs %d", len(first.Swatches), len(second.Swatches))
	}
	for index := range first.Swatches {
		if first.Swatches[index] != second.Swatches[index] {
			t.Fatalf("swatch %d is not stable: %#v vs %#v",
				index, first.Swatches[index], second.Swatches[index])
		}
	}
}

func TestDailyReportsTheDateAndWeekday(t *testing.T) {
	day, err := Daily("2026-11-25")
	if err != nil {
		t.Fatal(err)
	}
	if day.Date != "2026-11-25" {
		t.Fatalf("date = %q", day.Date)
	}
	if day.Weekday != "星期三" {
		t.Fatalf("weekday = %q", day.Weekday)
	}
}

func TestDailyRejectsMalformedDates(t *testing.T) {
	for _, value := range []string{"", "2026-13-01", "2026-02-30", "26-11-25", "2026-1-5"} {
		if _, err := Daily(value); err == nil {
			t.Fatalf("Daily(%q) accepted a malformed date", value)
		}
	}
}

func TestDailyOverAYear(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for offset := 0; offset < 400; offset++ {
		date := start.AddDate(0, 0, offset).Format("2006-01-02")
		day, err := Daily(date)
		if err != nil {
			t.Fatalf("%s: %v", date, err)
		}
		if len(day.Swatches) != 5 {
			t.Fatalf("%s: %d swatches", date, len(day.Swatches))
		}
		seen := make(map[string]bool, 5)
		for index, swatch := range day.Swatches {
			if !hexPattern.MatchString(swatch.Hex) {
				t.Fatalf("%s: swatch %d hex %q", date, index, swatch.Hex)
			}
			if seen[swatch.Hex] {
				t.Fatalf("%s: duplicate hex %s", date, swatch.Hex)
			}
			seen[swatch.Hex] = true
			if swatch.Ink != "#FFFFFF" && swatch.Ink != "#141416" {
				t.Fatalf("%s: ink %q", date, swatch.Ink)
			}
			if swatch.Name == "" || swatch.Role != roles[index] {
				t.Fatalf("%s: swatch %d name %q role %q",
					date, index, swatch.Name, swatch.Role)
			}
			if swatch.RGB == "" || swatch.HSL == "" {
				t.Fatalf("%s: swatch %d is missing a format", date, index)
			}
		}
	}
}

func TestDailyVariesAcrossDays(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seen := make(map[string]bool, 90)
	for offset := 0; offset < 90; offset++ {
		date := start.AddDate(0, 0, offset).Format("2006-01-02")
		day, err := Daily(date)
		if err != nil {
			t.Fatal(err)
		}
		seen[day.Swatches[0].Hex] = true
	}
	if len(seen) < 60 {
		t.Fatalf("only %d distinct main colours in 90 days", len(seen))
	}
}

func TestRampKeepsThePickedColourInTheMiddle(t *testing.T) {
	swatches, err := Ramp("#3a7bff", 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(swatches) != 9 {
		t.Fatalf("len = %d", len(swatches))
	}
	if swatches[4].Hex != "#3A7BFF" {
		t.Fatalf("middle = %q", swatches[4].Hex)
	}
	seen := make(map[string]bool, 9)
	previous := 101.0
	for index, swatch := range swatches {
		if seen[swatch.Hex] {
			t.Fatalf("duplicate %s", swatch.Hex)
		}
		seen[swatch.Hex] = true
		_, _, light := hexToHSL(swatch.Hex)
		if light > previous {
			t.Fatalf("step %d is lighter than the step before it", index)
		}
		previous = light
		if swatch.Ink == "" || swatch.Name == "" {
			t.Fatalf("step %d is incomplete: %#v", index, swatch)
		}
	}
}

func TestRampRejectsBadInput(t *testing.T) {
	if _, err := Ramp("blue", 9); err == nil {
		t.Fatal("accepted a word as a colour")
	}
	for _, count := range []int{1, 2, 4, 12, 13} {
		if _, err := Ramp("#3A7BFF", count); err == nil {
			t.Fatalf("accepted count %d", count)
		}
	}
	swatches, err := Ramp("#3A7BFF", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(swatches) != 9 {
		t.Fatalf("default count produced %d steps", len(swatches))
	}
}

func TestRampNormalisesShortHex(t *testing.T) {
	swatches, err := Ramp("abc", 3)
	if err != nil {
		t.Fatal(err)
	}
	if swatches[1].Hex != "#AABBCC" {
		t.Fatalf("middle = %q", swatches[1].Hex)
	}
}

func TestRampStaysDistinctAtTheEdges(t *testing.T) {
	// Near white or black, one arm of the scale has almost no room, so steps
	// can round to the same hex. Every step must still be a colour of its own,
	// and the middle step must stay exactly the picked colour.
	for _, value := range []string{"#FFFFFF", "#FEFEFE", "#F9F9F9", "#FAFAFA", "#000000", "#010101"} {
		for _, count := range []int{5, 7, 9, 11} {
			swatches, err := Ramp(value, count)
			if err != nil {
				t.Fatalf("%s/%d: %v", value, count, err)
			}
			seen := make(map[string]bool, count)
			for _, swatch := range swatches {
				if seen[swatch.Hex] {
					t.Fatalf("%s/%d repeats %s", value, count, swatch.Hex)
				}
				seen[swatch.Hex] = true
				if _, ok := NormalizeHex(swatch.Hex); !ok {
					t.Fatalf("%s/%d produced %q", value, count, swatch.Hex)
				}
			}
			if swatches[count/2].Hex != value {
				t.Fatalf("%s/%d middle = %q", value, count, swatches[count/2].Hex)
			}
		}
	}
}

func TestNormalizeHex(t *testing.T) {
	cases := map[string]string{
		"#aabbcc": "#AABBCC",
		"aabbcc":  "#AABBCC",
		"#abc":    "#AABBCC",
		" abc ":   "#AABBCC",
	}
	for input, want := range cases {
		got, ok := NormalizeHex(input)
		if !ok || got != want {
			t.Fatalf("NormalizeHex(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
	for _, input := range []string{"", "#12", "#12345", "#1234567", "#GGGGGG", "red"} {
		if _, ok := NormalizeHex(input); ok {
			t.Fatalf("NormalizeHex(%q) accepted it", input)
		}
	}
}

func TestInkReadsOnTheColour(t *testing.T) {
	if ink := inkFor("#FFFFFF"); ink != "#141416" {
		t.Fatalf("white ink = %q", ink)
	}
	if ink := inkFor("#000000"); ink != "#FFFFFF" {
		t.Fatalf("black ink = %q", ink)
	}
}

func TestColorNames(t *testing.T) {
	cases := []struct {
		hue, sat, light float64
		want            string
	}{
		{220, 60, 50, "蓝"},
		{220, 60, 80, "浅蓝"},
		{220, 60, 25, "深蓝"},
		{5, 4, 50, "灰"},
		{5, 4, 25, "深灰"},
		{120, 50, 50, "绿"},
		{350, 50, 50, "赤"},
	}
	for _, c := range cases {
		if got := colorName(c.hue, c.sat, c.light); got != c.want {
			t.Fatalf("colorName(%v, %v, %v) = %q; want %q",
				c.hue, c.sat, c.light, got, c.want)
		}
	}
}

func TestSwatchesRejectsInvalidStoredColours(t *testing.T) {
	if _, err := Swatches([]string{"#AABBCC", "nope"}); err == nil {
		t.Fatal("accepted an invalid stored colour")
	}
}
