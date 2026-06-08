package media

import (
	"regexp"
	"strings"
	"unicode"
)

// MediaInfo holds the result of media name normalization.
type MediaInfo struct {
	// Title is the cleaned media title (episode tag removed).
	Title string

	// Season (1-based) extracted from the name, or 0 if not an episode.
	Season int

	// Episode (1-based) extracted from the name, or 0 if not an episode.
	Episode int

	// EpisodeTag is the formatted tag like "S01E01", or empty for movies.
	EpisodeTag string

	// IsEpisode is true when a season/episode pattern was detected.
	IsEpisode bool
}

// qualityPatterns lists quality/resolution tags to remove, ordered longest
// first to avoid partial matches.
var qualityPatterns = []string{
	"2160P", "2160p",
	"1080P", "1080p",
	"720P", "720p",
	"4K",
	"HDR",
	"DV",
	"BluRay", "BLURAY", "bluray",
	"WEB-DL",
	"WEB DL", // after separator normalization
	"WEBRip", "WEBRIP", "webrip",
	"WEB RIP", // after separator normalization of WEBRip
	"x265", "X265",
	"H265", "h265",
	"AAC",
	"HEVC", "hevc",
	"HDTV",
	"REMUX",
	"Criterion", "criterion",
	"COMPLETE", "complete",
	"BLURAY", // caps-only variant
}

// patterns for season/episode detection in priority order.
var (
	// 第1季第1集 or 第01季第01集 — Chinese season+episode
	reChineseSeasonEp = regexp.MustCompile(`第(\d+)季第(\d+)集`)

	// 第01集 or 第1集 — Chinese episode only (assumes season 1)
	reChineseEp = regexp.MustCompile(`第(\d+)集`)

	// S01E01 or s01e01 or S1E1
	reStandardSE = regexp.MustCompile(`[Ss](\d+)[Ee](\d+)`)

	// EP01 or ep01 or Episode 01 (assumes season 1)
	reEP = regexp.MustCompile(`[Ee][Pp](\d+)`)

	// Season 1 Episode 2 style
	reSeasonEpisode = regexp.MustCompile(`[Ss]eason\s*(\d+)\s*[Ee]pisode\s*(\d+)`)

	// Year pattern: (2024) or [2024] or 2024 — used to separate year from title
	reYear = regexp.MustCompile(`[\(\[]?(\d{4})[\)\]]?`)
)

// Normalize cleans a media file name and extracts episode information.
// It handles both movie and episode file names, returning structured info.
//
// Examples:
//
//	"Avatar.2022.1080p.BluRay.x265.mkv" → {Title: "Avatar", IsEpisode: false}
//	"Breaking.Bad.S01E01.1080p.mkv"     → {Title: "Breaking Bad", Season: 1, Episode: 1, EpisodeTag: "S01E01"}
//	"庆余年.第01集.1080p.mp4"          → {Title: "庆余年", Season: 1, Episode: 1, EpisodeTag: "S01E01"}
func Normalize(filename string) MediaInfo {
	// Step 1: Remove file extension
	name := removeExtension(filename)

	// Step 2: Replace separators with spaces
	name = normalizeSeparators(name)

	// Step 3: Try to extract episode info. Do this BEFORE removing quality
	// tags so patterns like "S01E01" aren't destroyed.
	info := extractEpisodeInfo(name)

	// Step 4: Remove quality tags from the name
	cleaned := removeQualityTags(name)

	// Step 5: Remove episode patterns from the cleaned name
	cleaned = removeEpisodePatterns(cleaned)

	// Step 6: Remove standalone numbers that are common quality indicators
	cleaned = removeStandaloneQualities(cleaned)

	// Step 7: Remove year patterns from the end
	cleaned = removeYear(cleaned)

	// Step 8: Clean up whitespace
	cleaned = collapseSpaces(cleaned)
	cleaned = strings.TrimSpace(cleaned)

	info.Title = cleaned

	// If we found an episode tag, format it properly
	if info.IsEpisode {
		info.EpisodeTag = formatEpisodeTag(info.Season, info.Episode)
	}

	return info
}

// removeExtension strips the last file extension.
func removeExtension(name string) string {
	if idx := strings.LastIndex(name, "."); idx > 0 {
		// Only strip if the extension looks like a media format
		ext := strings.ToLower(name[idx+1:])
		switch ext {
		case "mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "m4v",
			"ts", "mts", "m2ts", "iso", "bdmv", "mpeg", "mpg", "3gp",
			"ogm", "ogv", "asf", "rm", "rmvb", "vob":
			name = name[:idx]
		}
	}
	return name
}

// normalizeSeparators replaces common separators with spaces.
func normalizeSeparators(name string) string {
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	return name
}

// extractEpisodeInfo checks the string for season/episode patterns.
func extractEpisodeInfo(name string) MediaInfo {
	// Try season+episode patterns first (most specific)

	// Season X Episode Y format
	if m := reSeasonEpisode.FindStringSubmatch(name); m != nil {
		return MediaInfo{
			Season:    parseInt(m[1]),
			Episode:   parseInt(m[2]),
			IsEpisode: true,
		}
	}

	// 第X季第Y集 format
	if m := reChineseSeasonEp.FindStringSubmatch(name); m != nil {
		return MediaInfo{
			Season:    parseInt(m[1]),
			Episode:   parseInt(m[2]),
			IsEpisode: true,
		}
	}

	// S01E01 format
	if m := reStandardSE.FindStringSubmatch(name); m != nil {
		return MediaInfo{
			Season:    parseInt(m[1]),
			Episode:   parseInt(m[2]),
			IsEpisode: true,
		}
	}

	// Episode-only patterns (assume season 1)

	// 第X集 format
	if m := reChineseEp.FindStringSubmatch(name); m != nil {
		return MediaInfo{
			Season:    1,
			Episode:   parseInt(m[1]),
			IsEpisode: true,
		}
	}

	// EP01 format
	if m := reEP.FindStringSubmatch(name); m != nil {
		return MediaInfo{
			Season:    1,
			Episode:   parseInt(m[1]),
			IsEpisode: true,
		}
	}

	return MediaInfo{}
}

// removeQualityTags strips known quality keywords from the name.
func removeQualityTags(name string) string {
	for _, q := range qualityPatterns {
		name = replaceCaseInsensitive(name, q, " ")
	}

	// Remove standalone "4K" or "4k" (the pattern above covers "4K")
	// Also handle "4k" lowercase variant
	name = replaceCaseInsensitive(name, "4k", " ")

	return name
}

// removeEpisodePatterns removes episode pattern text from the name.
func removeEpisodePatterns(name string) string {
	patterns := []*regexp.Regexp{
		reChineseSeasonEp,
		reChineseEp,
		reSeasonEpisode,
		reStandardSE,
		reEP,
	}
	for _, re := range patterns {
		name = re.ReplaceAllString(name, " ")
	}
	return name
}

// removeStandaloneQualities removes standalone numbers that look like
// resolution indicators (480, 720, 1080, 2160, etc.)
func removeStandaloneQualities(name string) string {
	re := regexp.MustCompile(`\b(480|720|1080|2160)\b`)
	return re.ReplaceAllString(name, " ")
}

// removeYear removes 4-digit year patterns, but only when they are not the
// sole remaining content (to preserve movie titles like "1917").
func removeYear(name string) string {
	locs := reYear.FindAllStringIndex(name, -1)
	if len(locs) == 0 {
		return name
	}

	// Count how many non-year tokens exist
	hasNonYear := false
	for _, part := range strings.Fields(name) {
		if !reYear.MatchString(part) {
			hasNonYear = true
			break
		}
	}

	// Remove the LAST occurrence only if there are ≥2 years OR other content exists
	if len(locs) >= 2 || hasNonYear {
		last := locs[len(locs)-1]
		name = name[:last[0]] + " " + name[last[1]:]
	}
	return name
}

// collapseSpaces reduces multiple whitespace characters to a single space.
func collapseSpaces(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	inSpace := false
	for _, r := range name {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return b.String()
}

// formatEpisodeTag produces a zero-padded tag like "S01E01".
func formatEpisodeTag(season, episode int) string {
	return formatTwoDigits("S", season) + formatTwoDigits("E", episode)
}

func formatTwoDigits(prefix string, n int) string {
	if n < 10 {
		return prefix + "0" + itoa(n)
	}
	return prefix + itoa(n)
}

// replaceCaseInsensitive replaces all case-insensitive occurrences of old
// with new in s.
func replaceCaseInsensitive(s, old, new string) string {
	lower := strings.ToLower(s)
	oldLower := strings.ToLower(old)

	var b strings.Builder
	last := 0
	for {
		i := strings.Index(lower[last:], oldLower)
		if i < 0 {
			break
		}
		pos := last + i
		b.WriteString(s[last:pos])
		b.WriteString(new)
		last = pos + len(old)
	}
	b.WriteString(s[last:])
	return b.String()
}

// parseInt converts a string to an integer. Returns 0 on failure.
func parseInt(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		} else {
			return 0
		}
	}
	return n
}

// itoa converts an int to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
