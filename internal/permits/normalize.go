package permits

import (
	"strconv"
	"strings"
	"unicode"
)

var suffixes = map[string]string{
	"BOULEVARD": "BLVD", "BLVD": "BLVD", "AVENUE": "AVE", "AV": "AVE", "AVE": "AVE",
	"ROAD": "RD", "RD": "RD", "STREET": "ST", "ST": "ST", "DRIVE": "DR", "DR": "DR",
	"LANE": "LN", "LN": "LN", "PARKWAY": "PKWY", "PKWY": "PKWY", "HIGHWAY": "HWY", "HWY": "HWY",
	"COURT": "CT", "CT": "CT", "PLACE": "PL", "PL": "PL", "CIRCLE": "CIR", "CIR": "CIR",
	"TRAIL": "TRL", "TRL": "TRL", "BL": "BLVD", "LP": "LOOP", "CI": "CIR", "WY": "WAY", "WAY": "WAY", "LOOP": "LOOP", "TERRACE": "TER", "TER": "TER",
}

var directions = map[string]string{"N": "N", "NORTH": "N", "S": "S", "SOUTH": "S", "E": "E", "EAST": "E", "W": "W", "WEST": "W", "NE": "NE", "NW": "NW", "SE": "SE", "SW": "SW"}

// "5 TH" -> "5TH"
var ordinalTail = map[string]bool{"ST": true, "ND": true, "RD": true, "TH": true}

// Street squashes the city's many spellings into one (E Speedway Blvd, EAST SPEEDWAY BOULEVARD,
// E. SPEEDWAY) so the same street from different layers ends up in the same place
func Street(dir, name string) string {
	clean := func(s string) []string {
		s = strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToUpper(r)
			}
			return ' '
		}, s)
		return strings.Fields(s)
	}
	toks := clean(name)
	if len(toks) == 0 {
		return ""
	}
	d := directions[strings.ToUpper(strings.TrimSpace(strings.Trim(dir, ". ")))]
	if len(toks) >= 2 {
		if td, ok := directions[toks[0]]; ok {
			if _, isSuffix := suffixes[toks[1]]; !isSuffix {
				if d == "" {
					d = td
				}
				if td == d {
					toks = toks[1:]
				}
			}
		}
	}
	var merged []string
	for i := 0; i < len(toks); i++ {
		if _, err := strconv.Atoi(toks[i]); err == nil && i+2 < len(toks) && ordinalTail[toks[i+1]] {
			merged = append(merged, toks[i]+toks[i+1])
			i++
			continue
		}
		merged = append(merged, toks[i])
	}
	if n := len(merged); n > 1 {
		if s, ok := suffixes[merged[n-1]]; ok {
			merged[n-1] = s
		}
	}
	out := strings.Join(merged, " ")
	if d != "" {
		out = d + " " + out
	}
	return out
}

// BlockOf returns the hundred-block of a street number ("2345 E" -> 2300), or -1
func BlockOf(streetNo string) int {
	s := strings.TrimSpace(streetNo)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return -1
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil || n <= 0 {
		return -1
	}
	return n / 100 * 100
}

func Category(parts ...string) string {
	s := strings.ToUpper(strings.Join(parts, " "))
	switch {
	case strings.Contains(s, "WIRELESS"), strings.Contains(s, "TR-SWF"): // equipment on poles, not street work
		return "Small wireless"
	case strings.Contains(s, "COMBO"):
		return "Combo"
	case strings.Contains(s, "EXCAV"):
		return "Excavation"
	case strings.Contains(s, "BARRICADE"):
		return "Barricade"
	case strings.Contains(s, "CIVIC"), strings.Contains(s, "EVENT"):
		return "Civic event"
	// the city's newer permit system (TR-... sub-types) has its own vocabulary
	case strings.Contains(s, "TR-UTL"), strings.Contains(s, "- UTILITY"), strings.Contains(s, "TR-FBR"), strings.Contains(s, "FIBER"):
		return "Utility"
	case strings.Contains(s, "TR-ANN"), strings.Contains(s, "- ANNUAL"):
		return "Annual"
	case strings.Contains(s, "TR-ROW"), strings.Contains(s, "RIGHT-OF-WAY (ROW)"):
		return "Right-of-way"
	}
	return "Other"
}
