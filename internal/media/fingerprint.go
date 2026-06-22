// Package media — media quality fingerprint extraction.
package media

import (
	"regexp"
	"strings"
)

// QualityTier ranks media quality from low to high.
type QualityTier int

const (
	QualityUnknown  QualityTier = 0
	QualitySD       QualityTier = 1 // ≤480p, DVD
	QualityHD       QualityTier = 2 // 720p
	QualityFullHD   QualityTier = 3 // 1080p
	Quality4K       QualityTier = 4 // 2160p / 4K
	Quality8K       QualityTier = 5 // 4320p / 8K
)

// MediaFingerprint captures the technical quality attributes extracted from
// a media filename.
type MediaFingerprint struct {
	Resolution QualityTier `json:"resolution"`
	Codec      string      `json:"codec"`   // x264, x265/HEVC, AV1, etc.
	HDR        string      `json:"hdr"`     // HDR10, DV, HDR10+, etc.
	Audio      string      `json:"audio"`   // AAC, DTS, TrueHD, Atmos, etc.
	Source     string      `json:"source"`  // BluRay, REMUX, WEB-DL, etc.
}

// Score returns a numeric quality score; higher = better.
// This can be used to compare two copies of the same title.
func (m MediaFingerprint) Score() int {
	score := int(m.Resolution) * 1000

	// Codec bonus
	switch strings.ToLower(m.Codec) {
	case "av1":
		score += 300
	case "hevc", "x265", "h265":
		score += 250
	case "x264", "h264", "avc":
		score += 100
	}

	// HDR bonus
	switch strings.ToLower(m.HDR) {
	case "dv", "dolbyvision", "dolby vision":
		score += 400
	case "hdr10+", "hdr10plus":
		score += 350
	case "hdr10", "hdr":
		score += 300
	case "hlg":
		score += 150
	}

	// Audio bonus
	audioLower := strings.ToLower(m.Audio)
	switch {
	case strings.Contains(audioLower, "atmos"):
		score += 500
	case strings.Contains(audioLower, "truehd"):
		score += 400
	case strings.Contains(audioLower, "dts-hd"), strings.Contains(audioLower, "dtshd"), strings.Contains(audioLower, "dts-x"), strings.Contains(audioLower, "dtsx"):
		score += 350
	case strings.Contains(audioLower, "dts"):
		score += 200
	case strings.Contains(audioLower, "flac"):
		score += 150
	case strings.Contains(audioLower, "dd+"), strings.Contains(audioLower, "ddp"), strings.Contains(audioLower, "eac3"):
		score += 120
	}

	// Source bonus
	sourceLower := strings.ToLower(m.Source)
	switch {
	case strings.Contains(sourceLower, "remux"):
		score += 500
	case strings.Contains(sourceLower, "bluray"), strings.Contains(sourceLower, "bdremux"):
		score += 300
	case strings.Contains(sourceLower, "web-dl"):
		score += 100
	case strings.Contains(sourceLower, "webrip"):
		score += 50
	case strings.Contains(sourceLower, "hdtv"):
		score += 30
	}

	return score
}

// IsEmpty returns true if no quality attributes were detected.
func (m MediaFingerprint) IsEmpty() bool {
	return m.Resolution == QualityUnknown &&
		m.Codec == "" &&
		m.HDR == "" &&
		m.Audio == "" &&
		m.Source == ""
}

// resolution patterns for detection.
var resolutionPatterns = []struct {
	re   *regexp.Regexp
	tier QualityTier
}{
	{regexp.MustCompile(`\b4320[Pp]\b`), Quality8K},
	{regexp.MustCompile(`\b8[Kk]\b`), Quality8K},
	{regexp.MustCompile(`\b2160[Pp]\b`), Quality4K},
	{regexp.MustCompile(`\b4[Kk]\b`), Quality4K},
	{regexp.MustCompile(`\bUHD\b`), Quality4K},
	{regexp.MustCompile(`\b1440[Pp]\b`), QualityFullHD}, // QHD → close to FHD
	{regexp.MustCompile(`\b1080[Pp]\b`), QualityFullHD},
	{regexp.MustCompile(`\b1080[Ii]\b`), QualityFullHD},
	{regexp.MustCompile(`\bFHD\b`), QualityFullHD},
	{regexp.MustCompile(`\b720[Pp]\b`), QualityHD},
	{regexp.MustCompile(`\bQHD\b`), QualityHD},
	{regexp.MustCompile(`\b576[Pp]\b`), QualitySD},
	{regexp.MustCompile(`\b480[Pp]\b`), QualitySD},
}

// ExtraceFingerprint analyzes a filename and extracts quality attributes.
func ExtraceFingerprint(filename string) MediaFingerprint {
	fp := MediaFingerprint{}
	upper := strings.ToUpper(filename)

	// Detect resolution
	for _, p := range resolutionPatterns {
		if p.re.MatchString(filename) || p.re.MatchString(upper) {
			fp.Resolution = p.tier
			break
		}
	}

	// Detect codec (first match only; the dominant codec)
	codecPatterns := []string{
		"AV1", "av1",
		"HEVC", "hevc", "H265", "h265", "X265", "x265",
		"AVC", "H264", "h264", "X264", "x264",
		"XviD", "xvid", "DivX", "divx",
		"VP9", "vp9",
	}
	for _, c := range codecPatterns {
		if strings.Contains(upper, strings.ToUpper(c)) {
			fp.Codec = c
			break
		}
	}

	// Detect HDR (highest tier first)
	hdrPatterns := []string{
		"DolbyVision", "Dolby Vision", "DoVi", "DV",
		"HDR10+", "HDR10Plus",
		"HDR10", "HDR",
		"HLG",
		"BT2020", "REC2020",
		"WCG",
		"10bit",
	}
	for _, h := range hdrPatterns {
		if strings.Contains(upper, strings.ToUpper(h)) {
			fp.HDR = h
			break
		}
	}

	// Detect audio (highest quality first)
	audioPatterns := []string{
		"Atmos",
		"TrueHD",
		"DTS-HD", "DTSHD", "DTS-X", "DTSX",
		"DTS",
		"FLAC",
		"EAC3", "DD+", "DDP",
		"AC3",
		"AAC5.1", "AAC2.0", "AAC",
		"LPCM", "PCM",
		"Opus",
		"MP3",
	}
	for _, a := range audioPatterns {
		if strings.Contains(upper, strings.ToUpper(a)) {
			fp.Audio = a
			break
		}
	}

	// Detect source (highest quality first)
	sourcePatterns := []string{
		"BDREMUX",
		"REMUX",
		"BluRay", "BLURAY", "bluray",
		"WEB-DL", "WEB DL", "WEBDL",
		"WEBRip", "WEBRIP", "WEB RIP",
		"HDTV",
		"DVDRip", "BDRip", "HDRip",
		"DVD",
	}
	for _, s := range sourcePatterns {
		if strings.Contains(upper, strings.ToUpper(s)) {
			fp.Source = s
			break
		}
	}

	return fp
}
