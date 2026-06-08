package media

import (
	"testing"
)

// --- 中文电影 ---

func TestNormalize_ChineseMovie(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"让子弹飞.2010.1080p.BluRay.x265.mkv", "让子弹飞"},
		{"霸王别姬.1993.2160p.HDR.mkv", "霸王别姬"},
		{"无间道.2002.BluRay.1080p.mkv", "无间道"},
		{"大话西游之大圣娶亲.1995.HDR.4K.mkv", "大话西游之大圣娶亲"},
		{"功夫.2004.720p.WEB-DL.mkv", "功夫"},
		{"活着.1994.1080p.WEBRip.x265.mkv", "活着"},
		{"饮食男女.1994.BluRay.1080p.DV.mkv", "饮食男女"},
		{"一一.2000.Criterion.1080p.mkv", "一一"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if got.IsEpisode {
				t.Errorf("Normalize(%q).IsEpisode = true, want false", tt.input)
			}
		})
	}
}

// --- 英文电影 ---

func TestNormalize_EnglishMovie(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Avatar.2022.1080p.BluRay.x265.mkv", "Avatar"},
		{"The.Matrix.1999.4K.HDR.mkv", "The Matrix"},
		{"Inception.2010.BluRay.1080p.mkv", "Inception"},
		{"Interstellar.2014.2160p.HDR.DV.mkv", "Interstellar"},
		{"The.Godfather.1972.WEB-DL.1080p.mkv", "The Godfather"},
		{"Pulp.Fiction.1994.1080p.WEBRip.x265.mkv", "Pulp Fiction"},
		{"The.Dark.Knight.2008.BluRay.x265.AAC.mkv", "The Dark Knight"},
		{"Forrest Gump 1994 1080p WEB-DL.mkv", "Forrest Gump"},
		{"Fight Club 1999 BluRay 1080p.mkv", "Fight Club"},
		{"Schindlers.List.1993.4K.HDR.HEVC.mkv", "Schindlers List"},
		{"三体.2023.4K.HDR.mkv", "三体"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if got.IsEpisode {
				t.Errorf("Normalize(%q).IsEpisode = true, want false", tt.input)
			}
		})
	}
}

// --- 中文剧集 (S01E01 格式) ---

func TestNormalize_ChineseEpisode_SE(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantTag string
		season  int
		episode int
	}{
		{"庆余年.S01E01.1080p.mkv", "庆余年", "S01E01", 1, 1},
		{"庆余年.S01E02.1080p.mkv", "庆余年", "S01E02", 1, 2},
		{"琅琊榜.S02E01.4K.HDR.mkv", "琅琊榜", "S02E01", 2, 1},
		{"繁花.S01E01.2160p.mkv", "繁花", "S01E01", 1, 1},
		{"漫长的季节.S01E01.1080p.WEB-DL.mkv", "漫长的季节", "S01E01", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if !got.IsEpisode {
				t.Errorf("Normalize(%q).IsEpisode = false, want true", tt.input)
			}
			if got.EpisodeTag != tt.wantTag {
				t.Errorf("Normalize(%q).EpisodeTag = %q, want %q", tt.input, got.EpisodeTag, tt.wantTag)
			}
			if got.Season != tt.season {
				t.Errorf("Normalize(%q).Season = %d, want %d", tt.input, got.Season, tt.season)
			}
			if got.Episode != tt.episode {
				t.Errorf("Normalize(%q).Episode = %d, want %d", tt.input, got.Episode, tt.episode)
			}
		})
	}
}

// --- 英文剧集 (S01E01 格式) ---

func TestNormalize_EnglishEpisode_SE(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantTag string
		season  int
		episode int
	}{
		{"Breaking.Bad.S01E01.1080p.BluRay.mkv", "Breaking Bad", "S01E01", 1, 1},
		{"Game.of.Thrones.S01E01.4K.HDR.mkv", "Game of Thrones", "S01E01", 1, 1},
		{"Stranger.Things.S04E01.2160p.mkv", "Stranger Things", "S04E01", 4, 1},
		{"The.Last.of.Us.S01E01.1080p.WEB-DL.mkv", "The Last of Us", "S01E01", 1, 1},
		{"Friends.S10E01.1080p.WEBRip.mkv", "Friends", "S10E01", 10, 1},
		{"The.Crown.S05E01.BluRay.1080p.mkv", "The Crown", "S05E01", 5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if !got.IsEpisode {
				t.Errorf("Normalize(%q).IsEpisode = false, want true", tt.input)
			}
			if got.EpisodeTag != tt.wantTag {
				t.Errorf("Normalize(%q).EpisodeTag = %q, want %q", tt.input, got.EpisodeTag, tt.wantTag)
			}
			if got.Season != tt.season {
				t.Errorf("Normalize(%q).Season = %d, want %d", tt.input, got.Season, tt.season)
			}
			if got.Episode != tt.episode {
				t.Errorf("Normalize(%q).Episode = %d, want %d", tt.input, got.Episode, tt.episode)
			}
		})
	}
}

// --- EP 格式 ---

func TestNormalize_EPFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantTag string
		episode int
	}{
		{"Rick.and.Morty.EP01.1080p.mkv", "Rick and Morty", "S01E01", 1},
		{"Rick.and.Morty.EP02.1080p.mkv", "Rick and Morty", "S01E02", 2},
		{"Silicon.Valley.ep03.1080p.mkv", "Silicon Valley", "S01E03", 3},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if !got.IsEpisode {
				t.Errorf("Normalize(%q).IsEpisode = false, want true", tt.input)
			}
			if got.EpisodeTag != tt.wantTag {
				t.Errorf("Normalize(%q).EpisodeTag = %q, want %q", tt.input, got.EpisodeTag, tt.wantTag)
			}
			if got.Episode != tt.episode {
				t.Errorf("Normalize(%q).Episode = %d, want %d", tt.input, got.Episode, tt.episode)
			}
			if got.Season != 1 {
				t.Errorf("Normalize(%q).Season = %d, want 1", tt.input, got.Season)
			}
		})
	}
}

// --- 第X集 格式 ---

func TestNormalize_ChineseEpisodeOnly(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantTag string
		episode int
	}{
		{"庆余年.第01集.1080p.mp4", "庆余年", "S01E01", 1},
		{"庆余年.第02集.1080p.mp4", "庆余年", "S01E02", 2},
		{"繁花.第10集.4K.mp4", "繁花", "S01E10", 10},
		{"大明王朝1566.第01集.1080p.mkv", "大明王朝1566", "S01E01", 1},
		{"漫长的季节.第12集.1080p.mp4", "漫长的季节", "S01E12", 12},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if !got.IsEpisode {
				t.Errorf("Normalize(%q).IsEpisode = false, want true", tt.input)
			}
			if got.EpisodeTag != tt.wantTag {
				t.Errorf("Normalize(%q).EpisodeTag = %q, want %q", tt.input, got.EpisodeTag, tt.wantTag)
			}
			if got.Episode != tt.episode {
				t.Errorf("Normalize(%q).Episode = %d, want %d", tt.input, got.Episode, tt.episode)
			}
			if got.Season != 1 {
				t.Errorf("Normalize(%q).Season = %d, want 1", tt.input, got.Season)
			}
		})
	}
}

// --- 第X季第Y集 格式 ---

func TestNormalize_ChineseSeasonEpisode(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantTag string
		season  int
		episode int
	}{
		{"庆余年.第1季第1集.1080p.mp4", "庆余年", "S01E01", 1, 1},
		{"庆余年.第1季第2集.1080p.mp4", "庆余年", "S01E02", 1, 2},
		{"琅琊榜.第2季第1集.4K.mp4", "琅琊榜", "S02E01", 2, 1},
		{"大江大河.第3季第10集.1080p.mkv", "大江大河", "S03E10", 3, 10},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if !got.IsEpisode {
				t.Errorf("Normalize(%q).IsEpisode = false, want true", tt.input)
			}
			if got.EpisodeTag != tt.wantTag {
				t.Errorf("Normalize(%q).EpisodeTag = %q, want %q", tt.input, got.EpisodeTag, tt.wantTag)
			}
			if got.Season != tt.season {
				t.Errorf("Normalize(%q).Season = %d, want %d", tt.input, got.Season, tt.season)
			}
			if got.Episode != tt.episode {
				t.Errorf("Normalize(%q).Episode = %d, want %d", tt.input, got.Episode, tt.episode)
			}
		})
	}
}

// --- 分隔符统一 ---

func TestNormalize_Separators(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"The.Movie.Name.2020.1080p.mkv", "The Movie Name"},
		{"The_Movie_Name_2020_1080p.mkv", "The Movie Name"},
		{"The-Movie-Name-2020-1080p.mkv", "The Movie Name"},
		{"Mixed.Separators_Name-2020.mkv", "Mixed Separators Name"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
		})
	}
}

// --- 边缘情况 ---

func TestNormalize_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"movie.mp4", "movie"},
		{"Just A Movie.mkv", "Just A Movie"},
		{"", ""},
		{".hidden.mp4", "hidden"},
		{"a.b.c.d.e.f.mkv", "a b c d e f"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
		})
	}
}

// --- 大小写不敏感 ---

func TestNormalize_QualityCaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"movie.1080p.bluray.x265.mkv", "movie"},
		{"movie.1080P.BluRay.X265.mkv", "movie"},
		{"MOVIE.2160P.WEB-DL.mkv", "MOVIE"},
		{"MoViE.4k.hdr.dv.mkv", "MoViE"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
		})
	}
}

// --- S01E01 大小写 ---

func TestNormalize_EpisodeCaseInsensitive(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantTag string
	}{
		{"Show.s01e01.mkv", "Show", "S01E01"},
		{"Show.S01E01.mkv", "Show", "S01E01"},
		{"Show.S01e01.mkv", "Show", "S01E01"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if got.EpisodeTag != tt.wantTag {
				t.Errorf("Normalize(%q).EpisodeTag = %q, want %q", tt.input, got.EpisodeTag, tt.wantTag)
			}
		})
	}
}

// --- 准确率统计 ---

func TestNormalize_Accuracy(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		want     string
		isEp     bool
		wantTag  string
	}

	cases := []testCase{
		// 中文电影
		{"chinese-movie-1", "让子弹飞.2010.1080p.BluRay.x265.mkv", "让子弹飞", false, ""},
		{"chinese-movie-2", "霸王别姬.1993.2160p.HDR.mkv", "霸王别姬", false, ""},
		{"chinese-movie-3", "无间道.2002.BluRay.1080p.mkv", "无间道", false, ""},

		// 英文电影
		{"eng-movie-1", "Avatar.2022.1080p.BluRay.x265.mkv", "Avatar", false, ""},
		{"eng-movie-2", "The.Matrix.1999.4K.HDR.mkv", "The Matrix", false, ""},
		{"eng-movie-3", "Inception.2010.BluRay.1080p.mkv", "Inception", false, ""},
		{"eng-movie-4", "Interstellar.2014.2160p.HDR.DV.mkv", "Interstellar", false, ""},

		// 英文剧集 S01E01
		{"eng-ep-se-1", "Breaking.Bad.S01E01.1080p.BluRay.mkv", "Breaking Bad", true, "S01E01"},
		{"eng-ep-se-2", "Game.of.Thrones.S01E01.4K.HDR.mkv", "Game of Thrones", true, "S01E01"},
		{"eng-ep-se-3", "Stranger.Things.S04E01.2160p.mkv", "Stranger Things", true, "S04E01"},

		// 中文剧集 S01E01
		{"chinese-ep-se-1", "庆余年.S01E01.1080p.mkv", "庆余年", true, "S01E01"},
		{"chinese-ep-se-2", "琅琊榜.S02E01.4K.HDR.mkv", "琅琊榜", true, "S02E01"},

		// 中文剧集 第X集
		{"chinese-ep-ep-1", "庆余年.第01集.1080p.mp4", "庆余年", true, "S01E01"},
		{"chinese-ep-ep-2", "繁花.第10集.4K.mp4", "繁花", true, "S01E10"},

		// 中文剧集 第X季第X集
		{"chinese-ep-se-3", "庆余年.第1季第1集.1080p.mp4", "庆余年", true, "S01E01"},

		// EP格式
		{"ep-format-1", "Rick.and.Morty.EP01.1080p.mkv", "Rick and Morty", true, "S01E01"},

		// 特殊字符
		{"special-1", "三体.2023.4K.HDR.mkv", "三体", false, ""},

		// 分隔符统一
		{"separator-1", "The_Last_of_Us_S01E01_1080p.mkv", "The Last of Us", true, "S01E01"},
		{"separator-2", "The-Last-of-Us-S01E01-1080p.mkv", "The Last of Us", true, "S01E01"},
	}

	correct := 0
	for _, tc := range cases {
		got := Normalize(tc.input)

		titleMatch := got.Title == tc.want
		epMatch := got.IsEpisode == tc.isEp
		tagMatch := true
		if tc.isEp {
			tagMatch = got.EpisodeTag == tc.wantTag
		}

		if titleMatch && epMatch && tagMatch {
			correct++
		} else {
			t.Errorf("FAIL [%s]: Normalize(%q) = {Title:%q IsEpisode:%v Tag:%q}, want {Title:%q IsEpisode:%v Tag:%q}",
				tc.name, tc.input, got.Title, got.IsEpisode, got.EpisodeTag,
				tc.want, tc.isEp, tc.wantTag)
		}
	}

	total := len(cases)
	accuracy := float64(correct) / float64(total) * 100
	t.Logf("Accuracy: %d/%d = %.1f%%", correct, total, accuracy)

	if accuracy < 90 {
		t.Errorf("accuracy %.1f%% is below 90%% threshold", accuracy)
	}
}

// --- Helper tests ---

func TestRemoveExtension(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"movie.mp4", "movie"},
		{"show.s01e01.mkv", "show.s01e01"},
		{"noext", "noext"},
		{"file.txt", "file.txt"},               // not a media extension
		{"archive.tar.gz", "archive.tar.gz"},     // not a media extension
		{".hidden.mp4", ".hidden"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := removeExtension(tt.input)
			if got != tt.want {
				t.Errorf("removeExtension(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"1", 1},
		{"10", 10},
		{"99", 99},
		{"001", 1},
		{"abc", 0},
		{"", 0},
		{"12abc", 0},
	}
	for _, tt := range tests {
		got := parseInt(tt.input)
		if got != tt.want {
			t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestFormatEpisodeTag(t *testing.T) {
	tests := []struct {
		season  int
		episode int
		want    string
	}{
		{1, 1, "S01E01"},
		{1, 10, "S01E10"},
		{10, 1, "S10E01"},
		{12, 24, "S12E24"},
	}
	for _, tt := range tests {
		got := formatEpisodeTag(tt.season, tt.episode)
		if got != tt.want {
			t.Errorf("formatEpisodeTag(%d, %d) = %q, want %q", tt.season, tt.episode, got, tt.want)
		}
	}
}

func TestCollapseSpaces(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello    world", "hello world"},
		{"  spaced  ", " spaced "},
		{"normal", "normal"},
		{"a   b   c", "a b c"},
	}
	for _, tt := range tests {
		got := collapseSpaces(tt.input)
		if got != tt.want {
			t.Errorf("collapseSpaces(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestReplaceCaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		s    string
		old  string
		new  string
		want string
	}{
		{"lower", "hello world", "world", "there", "hello there"},
		{"case", "HELLO World", "world", "THERE", "HELLO THERE"},
		{"mixed", "aAa", "aa", "b", "ba"},
		{"notfound", "hello", "xyz", "", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceCaseInsensitive(tt.s, tt.old, tt.new)
			if got != tt.want {
				t.Errorf("replaceCaseInsensitive(%q, %q, %q) = %q, want %q",
					tt.s, tt.old, tt.new, got, tt.want)
			}
		})
	}
}

// --- 更多剧集组合测试 ---

func TestNormalize_VariousEpisodeFormats(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantTag string
	}{
		// Season + Episode text format
		{"Show Season 1 Episode 1.mkv", "Show", "S01E01"},
		// s01e01 with dots
		{"Show s01e01.mkv", "Show", "S01E01"},
		// chinese episode with zero
		{"节目.第05集.mp4", "节目", "S01E05"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if got.EpisodeTag != tt.wantTag {
				t.Errorf("Normalize(%q).EpisodeTag = %q, want %q", tt.input, got.EpisodeTag, tt.wantTag)
			}
		})
	}
}

// --- 含有年份数字的电影（确保年份不会错误触发剧集检测） ---

func TestNormalize_MovieWithYear(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1917.2019.1080p.mkv", "1917"},
		{"2022.2023.BluRay.mkv", "2022"},
		{"The.Thing.1982.4K.mkv", "The Thing"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got.Title != tt.want {
				t.Errorf("Normalize(%q).Title = %q, want %q", tt.input, got.Title, tt.want)
			}
			if got.IsEpisode {
				t.Errorf("Normalize(%q).IsEpisode = true, want false", tt.input)
			}
		})
	}
}
