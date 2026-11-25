// Package color owns every colour decision this extension makes: the palette
// a date grows, the tint/shade scale around one picked colour, and the words a
// reader sees beside them. The page only lays these records out, so the same
// day always yields the same palette and a saved palette never depends on the
// page that drew it.
package color

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Swatch is one colour, already formatted for the page. Ink is the text
// colour that reads on it (WCAG contrast, white or near-black).
type Swatch struct {
	Hex  string `json:"hex"`
	RGB  string `json:"rgb"`
	HSL  string `json:"hsl"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
	Ink  string `json:"ink"`
}

// Day is one date's palette: five colours with fixed roles.
type Day struct {
	Date     string   `json:"date"`
	Weekday  string   `json:"weekday"`
	Swatches []Swatch `json:"swatches"`
}

// Month is one calendar month's palettes in day order. The page draws its
// overview from it, so a day reached on the grid has the same colours as one
// reached with the arrows.
type Month struct {
	Month string `json:"month"`
	Days  []Day  `json:"days"`
}

var weekdayNames = [...]string{
	"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六",
}

var roles = [5]string{"主色", "浅调", "深调", "强调", "中性"}

// accentOffsets are the harmonies a day can land on: the accent sits this many
// degrees from the main hue. The light, deep and paper tones stay in the
// main's family, so a day reads as one palette rather than five colours.
var accentOffsets = [...]int{32, 180, 150, 120, 90, 20}

// an rng is a small deterministic generator. It is written out rather than
// borrowed so a given date's palette cannot change when a standard library
// updates its generator.
type rng uint64

func (r *rng) next() uint64 {
	*r = *r*6364136223846793005 + 1442695040888963407
	return uint64(*r >> 33)
}

func (r *rng) intn(n int) int {
	return int(r.next() % uint64(n))
}

// Daily returns the palette for one local date, formatted YYYY-MM-DD. The
// same date always produces the same five colours.
func Daily(date string) (Day, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		return Day{}, errors.New("日期格式应为 YYYY-MM-DD")
	}
	year, month, day := parsed.Date()
	seed := uint64(year)<<16 | uint64(month)<<8 | uint64(day)
	random := rng(seed ^ 0x9e3779b97f4a7c15)

	baseHue := float64(random.intn(360))
	baseSat := float64(46 + random.intn(30))
	baseLight := float64(38 + random.intn(18))
	accentOffset := accentOffsets[random.intn(len(accentOffsets))]

	swatches := make([]Swatch, 0, 5)
	seen := make(map[string]bool, 5)
	for index := 0; index < 5; index++ {
		// One hue family for main, light and deep; one harmony for the
		// accent; a quiet paper tint for the neutral.
		hue := baseHue + float64(random.intn(7)-3)
		sat := baseSat + float64(random.intn(4))
		light := baseLight + float64(random.intn(4))
		switch index {
		case 1:
			sat -= 6
			light += 20
		case 2:
			sat += 4
			light -= 18
		case 3:
			hue += float64(accentOffset)
			sat += 6
			light += 4
		case 4:
			sat = float64(10 + random.intn(7))
			light = float64(86 + random.intn(6))
		}
		hue = wrap(hue)
		sat = clamp(sat, 10, 92)
		light = clamp(light, 14, 92)
		hex := hslToHex(hue, sat, light)
		// Five colours drawn from one seed can land on the same point. Nudge
		// until each swatch has an identity the reader can copy separately;
		// the grey fallback only bounds a loop that cannot realistically run.
		for tries := 0; seen[hex]; tries++ {
			if tries >= 32 {
				hex = hslToHex(0, 0, 10+float64(index)*12)
				break
			}
			hue = wrap(hue + 7)
			light = 16 + float64((int(light)+5)%74)
			hex = hslToHex(hue, sat, light)
		}
		seen[hex] = true
		swatches = append(swatches, newSwatch(hex, hue, sat, light, roles[index]))
	}

	return Day{
		Date:     date,
		Weekday:  weekdayNames[parsed.Weekday()],
		Swatches: swatches,
	}, nil
}

// MonthOf returns every day's palette in the month that contains date, so the
// page can draw the whole month without asking one day at a time.
func MonthOf(date string) (Month, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		return Month{}, errors.New("日期格式应为 YYYY-MM-DD")
	}
	first := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.UTC)
	days := make([]Day, 0, 31)
	for day := first; day.Month() == first.Month(); day = day.AddDate(0, 0, 1) {
		palette, err := Daily(day.Format("2006-01-02"))
		if err != nil {
			return Month{}, err
		}
		days = append(days, palette)
	}
	return Month{Month: first.Format("2006-01"), Days: days}, nil
}

// Ramp returns the tint/shade scale around one colour. The picked colour is
// exactly the middle step, so a reader can see where their colour sits.
func Ramp(value string, count int) ([]Swatch, error) {
	hex, ok := NormalizeHex(value)
	if !ok {
		return nil, errors.New("颜色值应为 #RRGGBB")
	}
	if count == 0 {
		count = 9
	}
	if count < 3 || count > 11 || count%2 == 0 {
		return nil, errors.New("色阶数量应为 3 到 11 之间的奇数")
	}
	baseHue, baseSat, baseLight := hexToHSL(hex)
	middle := count / 2
	// The light end sits at #F5F5F5 rather than pure white so the steps have
	// room to differ; a colour picked above that point would have nowhere to
	// climb, so its light end opens up to white.
	tintEnd := 96.0
	if baseLight > tintEnd {
		tintEnd = 100
	}
	hues := make([]float64, count)
	sats := make([]float64, count)
	lights := make([]float64, count)
	hexes := make([]string, count)
	for index := 0; index < count; index++ {
		hue, sat, light := baseHue, baseSat, baseLight
		switch {
		case index < middle:
			step := float64(middle-index) / float64(middle)
			light = baseLight + (tintEnd-baseLight)*step
			sat = baseSat * (1 - 0.30*step)
		case index > middle:
			step := float64(index-middle) / float64(middle)
			light = baseLight - (baseLight-12)*step
			sat = baseSat * (1 - 0.20*step)
		}
		hues[index], sats[index], lights[index] = hue, sat, light
		hexes[index] = hslToHex(hue, sat, light)
		if index == middle {
			hexes[index] = hex
		}
	}
	// A colour near white (or black) leaves little room on one side, so two
	// steps can round to the same hex. Nudge the later one outward until it is
	// a colour of its own; the middle step is pinned to the picked colour and
	// is claimed first.
	seen := map[string]bool{hex: true}
	for index := 0; index < count; index++ {
		if index == middle {
			continue
		}
		direction := 1.0
		if index > middle {
			direction = -1
		}
		for tries := 0; seen[hexes[index]] && tries < 64; tries++ {
			lights[index] = clamp(lights[index]+direction*1.2, 0, 100)
			if lights[index] == 0 || lights[index] == 100 {
				direction = -direction
			}
			hexes[index] = hslToHex(hues[index], sats[index], lights[index])
		}
		seen[hexes[index]] = true
	}

	swatches := make([]Swatch, 0, count)
	for index := 0; index < count; index++ {
		swatches = append(swatches, newSwatch(hexes[index], hues[index], sats[index], lights[index], ""))
	}
	return swatches, nil
}

// Swatches formats colours already known to be valid (a stored palette). A
// caller that has not validated them yet must use NormalizeHex first.
func Swatches(hexes []string) ([]Swatch, error) {
	out := make([]Swatch, 0, len(hexes))
	for _, value := range hexes {
		hex, ok := NormalizeHex(value)
		if !ok {
			return nil, errors.New("保存的配色里有无效颜色")
		}
		hue, sat, light := hexToHSL(hex)
		out = append(out, newSwatch(hex, hue, sat, light, ""))
	}
	return out, nil
}

func newSwatch(hex string, hue, sat, light float64, role string) Swatch {
	r, g, b := hexToRGB(hex)
	return Swatch{
		Hex:  hex,
		RGB:  fmt.Sprintf("rgb(%d, %d, %d)", r, g, b),
		HSL:  fmt.Sprintf("hsl(%d, %d%%, %d%%)", int(math.Round(hue))%360, int(math.Round(sat)), int(math.Round(light))),
		Name: colorName(hue, sat, light),
		Role: role,
		Ink:  inkFor(hex),
	}
}

// NormalizeHex accepts #abc, abc, #AABBCC or aabbcc and returns #AABBCC.
func NormalizeHex(value string) (string, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(trimmed) == 3 {
		trimmed = string([]byte{
			trimmed[0], trimmed[0], trimmed[1], trimmed[1], trimmed[2], trimmed[2],
		})
	}
	if len(trimmed) != 6 {
		return "", false
	}
	for index := 0; index < len(trimmed); index++ {
		character := trimmed[index]
		if !(character >= '0' && character <= '9') &&
			!(character >= 'a' && character <= 'f') &&
			!(character >= 'A' && character <= 'F') {
			return "", false
		}
	}
	return "#" + strings.ToUpper(trimmed), true
}

var hueNames = [...]struct {
	limit float64
	name  string
}{
	{15, "赤"}, {40, "橙"}, {65, "黄"}, {95, "黄绿"}, {140, "绿"},
	{170, "青"}, {195, "青蓝"}, {225, "蓝"}, {255, "靛"}, {285, "紫"},
	{315, "紫红"}, {345, "品红"}, {361, "赤"},
}

func colorName(hue, sat, light float64) string {
	base := "灰"
	if sat >= 14 {
		for _, band := range hueNames {
			if hue < band.limit {
				base = band.name
				break
			}
		}
	}
	switch {
	case light < 32:
		return "深" + base
	case light < 48:
		return "暗" + base
	case light >= 72:
		return "浅" + base
	}
	return base
}

func inkFor(hex string) string {
	luminance := relativeLuminance(hex)
	onWhite := 1.05 / (luminance + 0.05)
	onBlack := (luminance + 0.05) / 0.05
	if onBlack > onWhite {
		return "#141416"
	}
	return "#FFFFFF"
}

func relativeLuminance(hex string) float64 {
	red, green, blue := hexToRGB(hex)
	channel := func(component int) float64 {
		value := float64(component) / 255
		if value <= 0.04045 {
			return value / 12.92
		}
		return math.Pow((value+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(red) + 0.7152*channel(green) + 0.0722*channel(blue)
}

func hexToRGB(hex string) (int, int, int) {
	value := strings.TrimPrefix(hex, "#")
	component := func(text string) int {
		parsed, _ := strconv.ParseUint(text, 16, 8)
		return int(parsed)
	}
	return component(value[0:2]), component(value[2:4]), component(value[4:6])
}

func hueOf(r, g, b float64) float64 {
	high := math.Max(r, math.Max(g, b))
	low := math.Min(r, math.Min(g, b))
	if high == low {
		return 0
	}
	delta := high - low
	var hue float64
	switch high {
	case r:
		hue = math.Mod((g-b)/delta, 6)
	case g:
		hue = (b-r)/delta + 2
	default:
		hue = (r-g)/delta + 4
	}
	return wrap(hue * 60)
}

func hexToHSL(hex string) (float64, float64, float64) {
	red, green, blue := hexToRGB(hex)
	r, g, b := float64(red)/255, float64(green)/255, float64(blue)/255
	high := math.Max(r, math.Max(g, b))
	low := math.Min(r, math.Min(g, b))
	light := (high + low) / 2
	if high == low {
		return 0, 0, light * 100
	}
	delta := high - low
	sat := delta / (1 - math.Abs(2*light-1))
	return hueOf(r, g, b), sat * 100, light * 100
}

func hslToHex(hue, sat, light float64) string {
	hue = wrap(hue)
	sat = clamp(sat, 0, 100) / 100
	light = clamp(light, 0, 100) / 100
	chroma := (1 - math.Abs(2*light-1)) * sat
	section := hue / 60
	secondary := chroma * (1 - math.Abs(math.Mod(section, 2)-1))
	var r, g, b float64
	switch {
	case section < 1:
		r, g, b = chroma, secondary, 0
	case section < 2:
		r, g, b = secondary, chroma, 0
	case section < 3:
		r, g, b = 0, chroma, secondary
	case section < 4:
		r, g, b = 0, secondary, chroma
	case section < 5:
		r, g, b = secondary, 0, chroma
	default:
		r, g, b = chroma, 0, secondary
	}
	match := light - chroma/2
	component := func(value float64) int {
		return int(math.Round(clamp(value+match, 0, 1) * 255))
	}
	return fmt.Sprintf("#%02X%02X%02X", component(r), component(g), component(b))
}

func wrap(hue float64) float64 {
	hue = math.Mod(hue, 360)
	if hue < 0 {
		hue += 360
	}
	return hue
}

func clamp(value, low, high float64) float64 {
	return math.Min(high, math.Max(low, value))
}
