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

	// Year is the release year found in the name (1900-2029), or 0 if none.
	Year int
}

// qualityPatterns lists quality/resolution tags to remove, ordered longest
// first to avoid partial matches.
var qualityPatterns = []string{
	// Resolutions
	"4320P", "4320p",
	"2880P", "2880p",
	"2160P", "2160p",
	"1440P", "1440p",
	"1080P", "1080p",
	"1080I", "1080i",
	"720P", "720p",
	"576P", "576p",
	"480P", "480p",

	"8K",
	"4K",
	"UHD",
	"FHD",
	"QHD",

	// HDR / color / bit depth
	"HDR10+",
	"HDR10Plus",
	"HDR10",
	"HDR",
	"DolbyVision",
	"Dolby Vision",
	"DV",
	"DoVi",
	"SDR",
	"HLG",
	"BT2020",
	"REC2020",
	"WCG",
	"10bit",
	"8bit",

	// Source types
	"BluRay", "BLURAY", "bluray",
	"WEB-DL",
	"WEB DL",
	"WEBRip", "WEBRIP", "webrip",
	"WEB RIP",
	"WEBDL",
	"BDREMUX",
	"HDTV",
	"DVDRip",
	"BDRip",
	"HDRip",
	"BRRip",
	"REMUX",
	"Criterion", "criterion",
	"COMPLETE", "complete",
	"BLURAY",
	"R5",
	"DVD",

	// Streaming sources
	"Netflix",
	"NF",
	"Disney+",
	"DSNP",
	"AMZN",
	"Amazon",
	"ATVP",
	"AppleTV+",
	"HMAX",
	"MAX",
	"HULU",
	"iTunes",

	// Video codecs
	"x265", "X265",
	"H265", "h265",
	"x264", "X264",
	"H264", "h264",
	"HEVC", "hevc",
	"AVC",
	"XviD", "xvid",
	"DivX", "divx",
	"AV1", "av1",
	"VP9", "vp9",

	// Audio codecs
	"AAC2.0",
	"AAC5.1",
	"AAC",
	"DD+",
	"DDP",
	"AC3",
	"EAC3",
	"DTSHD",
	"DTS-HD",
	"DTS-X",
	"DTSX",
	"DTS",
	"TrueHD",
	"Atmos",
	"FLAC",
	"PCM",
	"LPCM",
	"Opus",
	"MP3",

	// Scene tags
	"PROPER",
	"REPACK",
	"RERIP",
	"INTERNAL",
	"EXTENDED",
	"THEATRICAL",
	"UNCUT",
	"UNRATED",
	"LIMITED",
	"HYBRID",
	"READNFO",

	// Chinese quality tags (common in Chinese media releases)
	"国英双音",
	"国粤双语",
	"特效字幕",
	"特效中字",
	"内封中字",
	"内封",
	"内嵌",
	"中字",
	"双字",
	"双语",
	"简体",
	"繁体",
	"简繁",
	"外挂字幕",
	"外挂",
	"原盘",
	"国语",
	"粤语",
	"国配",
	"港配",
	"台配",
	"DIY",
	"菜单修改",

	// Subtitle/audio language tags (English markers)
	"CHS",
	"CHT",
	"ENG",
	"JPN",
	"KOR",

	// Other common tags
	"MULTi",
	"DUBBED",
	"HC",
	"SUBBED",
}

// releaseYearRange defines the acceptable range for a release year anchor.
const (
	releaseYearMin = 1900
	releaseYearMax = 2029
)

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
// It uses a two-pass strategy:
//  1. Year-anchored pass: if a release year (1900-2029) is found, split the
//     filename at the year position. Everything after the year is discarded as
//     release metadata. The title zone before the year is cleaned of quality tags.
//     This handles complex scene filenames like:
//     "The.Amazing.Spider-Man.2012.PROPER.2160p.BluRay.REMUX.HEVC.DTS-HD.MA.TrueHD.7.1.Atmos-老K.mkv"
//     → "The Amazing Spider-Man"
//  2. Subtractive fallback: if no year is found, use the existing approach of
//     stripping known quality tags from the entire name.
//
// Examples:
//
//	"Avatar.2022.1080p.BluRay.x265.mkv" → {Title: "Avatar", IsEpisode: false}
//	"Breaking.Bad.S01E01.1080p.mkv"     → {Title: "Breaking Bad", Season: 1, Episode: 1, EpisodeTag: "S01E01"}
//	"庆余年.第01集.1080p.mp4"          → {Title: "庆余年", Season: 1, Episode: 1, EpisodeTag: "S01E01"}
func Normalize(filename string) MediaInfo {
	// Step 1: Remove file extension
	name := removeExtension(filename)

	// Step 2: Replace separators (., _, -) with spaces
	name = normalizeSeparators(name)

	// Step 3: Remove supplementary info in brackets 【】 [] （）
	// These often contain Chinese tags, quality info, format notes, etc.
	name = removeBrackets(name)

	// Step 5: Try to extract episode info. Do this BEFORE removing quality
	// tags so patterns like "S01E01" aren't destroyed.
	info := extractEpisodeInfo(name)

	// Extract release year from the name for downstream use
	info.Year, _ = findReleaseYear(name)

	// Step 6: Try year-anchored normalization
	cleaned := normalizeByYear(name)

	if cleaned == "" {
		// Fallback: no year found -- use subtractive tag-stripping approach
		cleaned = legacyNormalize(name)
	}

	// Step 7: Clean up whitespace
	cleaned = collapseSpaces(cleaned)
	cleaned = strings.TrimSpace(cleaned)

	info.Title = cleaned

	// If we found an episode tag, format it properly
	if info.IsEpisode {
		info.EpisodeTag = formatEpisodeTag(info.Season, info.Episode)
	}

	return info
}

// normalizeByYear tries to extract a clean title using the year as a structural
// delimiter. It returns the cleaned title zone, or "" if no year anchor is found.
//
// In scene-style filenames, the release year reliably separates the title
// (before) from release metadata (after). Everything after the year is discarded
// — no need to enumerate every possible tag.
func normalizeByYear(name string) string {
	_, pos := findReleaseYear(name)
	if pos <= 0 {
		return ""
	}

	// Split at year position: title zone is everything before the year
	titleZone := name[:pos]
	// Strip trailing delimiters between title and year marker
	titleZone = strings.TrimRight(titleZone, " ._-([（")

	// Clean quality tags from the title zone (some releases put tags before year)
	titleZone = removeQualityTags(titleZone)
	titleZone = removeEpisodePatterns(titleZone)
	titleZone = removeStandaloneQualities(titleZone)

	// Remove trailing year if present (e.g., "1917" from "1917.2019.1080p")
	titleZone = removeYear(titleZone)

	titleZone = collapseSpaces(titleZone)
	titleZone = strings.TrimSpace(titleZone)

	return titleZone
}

// legacyNormalize is the original subtractive approach: strip known quality tags
// from the entire name. Used as fallback when no release year is found.
func legacyNormalize(name string) string {
	name = removeQualityTags(name)
	name = removeEpisodePatterns(name)
	name = removeStandaloneQualities(name)
	name = removeReleaseGroup(name)
	name = removeYear(name)
	return name
}

// findReleaseYear finds the first 4-digit number in range 1900-2029 that is not
// a resolution indicator (480, 720, 1080, 2160). Returns the year value and its
// start position, or (0, -1) if none found.
func findReleaseYear(name string) (int, int) {
	// Scan for standalone 4-digit numbers (not part of longer numbers)
	// that are valid release years. This avoids issues with the year regex
	// including brackets in its match position.
	for i := 0; i <= len(name)-4; i++ {
		if isDigit(name[i]) && isDigit(name[i+1]) && isDigit(name[i+2]) && isDigit(name[i+3]) {
			if i+4 < len(name) && isDigit(name[i+4]) {
				continue
			}
			year := parseInt(name[i : i+4])
			if year >= releaseYearMin && year <= releaseYearMax && !isResolution(year) {
				return year, i
			}
		}
	}
	return 0, -1
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isResolution checks if a number is a known video resolution.
func isResolution(n int) bool {
	return n == 480 || n == 720 || n == 1080 || n == 2160
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

// removeBrackets removes content inside 【】 [] （） brackets, which typically
// contains supplementary metadata (Chinese tags, format notes, group names, etc.)
// that should not be part of the normalized title.
func removeBrackets(name string) string {
	re := regexp.MustCompile(`[【\[〖《][^】\]〗》]+[】\]〗》]`)
	return re.ReplaceAllString(name, " ")
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

	// Also handle "4k" lowercase variant (the pattern "4K" above covers uppercase)
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

// knownReleaseGroups are common release group suffixes stripped from filenames.
// These are NOT part of qualityPatterns — they're stripped separately by matching
// the trailing `-GroupName` pattern.
var knownReleaseGroups = []string{
	"CMRG", "EVO", "SPARKS", "FGT", "NTB", "FLUX",
	"CHD", "WiKi", "MTeam", "PTer", "OurBits", "FRDS", "BeiTai",
	"QHStudIo", "HiveWeb", "SCDI", "UBWEB", "ParkHD", "DreamHD",
	"VINEnc", "CtrlHD", "DECADES", "EBP", "iFT", "KINGS", "ViSION",
}

// removeReleaseGroup strips release group suffixes like "-FLUX" or "-NTb" from
// the end of the filename. Uses a two-pass approach:
//  1. Check against knownReleaseGroups list (case-insensitive suffix match)
//  2. If no known group matches, strip any trailing `-Word` that looks like a group
func removeReleaseGroup(name string) string {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)

	// Check hyphen-delimited groups (e.g., "Movie 1080p-FLUX")
	for _, g := range knownReleaseGroups {
		suffix := "-" + strings.ToLower(g)
		if strings.HasSuffix(lower, suffix) {
			name = strings.TrimSpace(name[:len(name)-len(suffix)])
			return name
		}
	}

	// Check space-delimited last word (after hyphen→space conversion)
	fields := strings.Fields(name)
	if len(fields) >= 2 {
		lastWord := strings.ToLower(fields[len(fields)-1])
		for _, g := range knownReleaseGroups {
			if lastWord == strings.ToLower(g) {
				name = strings.TrimSpace(name[:len(name)-len(fields[len(fields)-1])])
				return name
			}
		}
	}
	return name
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
