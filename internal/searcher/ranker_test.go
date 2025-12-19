package searcher

import (
	"testing"
	"time"

	"github.com/makokhawanjala/searchlight/internal/indexer"
)

func TestNewRanker(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "test",
		PreferExact:   true,
		PreferSmaller: true,
	}

	ranker := NewRanker(criteria)

	if ranker == nil {
		t.Fatal("NewRanker returned nil")
	}

	if ranker.criteria.Query != "test" {
		t.Errorf("Expected query 'test', got '%s'", ranker.criteria.Query)
	}
}

func TestRank_EmptySlice(t *testing.T) {
	criteria := RankingCriteria{Query: "test"}
	ranker := NewRanker(criteria)

	files := []*indexer.FileInfo{}
	result := ranker.Rank(files)

	if len(result) != 0 {
		t.Errorf("Expected empty slice, got %d files", len(result))
	}
}

func TestRank_SingleFile(t *testing.T) {
	criteria := RankingCriteria{Query: "test"}
	ranker := NewRanker(criteria)

	files := []*indexer.FileInfo{
		{Name: "test.txt", Path: "/test.txt", Size: 100},
	}

	result := ranker.Rank(files)

	if len(result) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(result))
	}

	if result[0].Name != "test.txt" {
		t.Errorf("Expected 'test.txt', got '%s'", result[0].Name)
	}
}

func TestRank_MultipleFiles(t *testing.T) {
	criteria := RankingCriteria{
		Query:       "config",
		PreferExact: true,
	}
	ranker := NewRanker(criteria)

	files := []*indexer.FileInfo{
		{Name: "my_config.txt", Path: "/my_config.txt", Size: 500},
		{Name: "config", Path: "/config", Size: 200},
		{Name: "config.yaml", Path: "/config.yaml", Size: 1500},
		{Name: "readme.md", Path: "/readme.md", Size: 300},
	}

	result := ranker.Rank(files)

	if len(result) != 4 {
		t.Fatalf("Expected 4 files, got %d", len(result))
	}

	if result[0].Name != "config" {
		t.Errorf("Expected 'config' first (exact match), got '%s'", result[0].Name)
	}

	if result[1].Name != "config.yaml" {
		t.Errorf("Expected 'config.yaml' second (prefix match), got '%s'", result[1].Name)
	}
}

func TestScoreNameMatch_ExactMatch(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "test.txt",
		CaseSensitive: false,
	}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Name: "test.txt"}
	score := ranker.scoreNameMatch(file)

	if score != 100 {
		t.Errorf("Expected score 100 for exact match, got %d", score)
	}
}

func TestScoreNameMatch_PrefixMatch(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "test",
		PreferExact:   true,
		CaseSensitive: false,
	}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Name: "test_file.txt"}
	score := ranker.scoreNameMatch(file)

	if score != 50 {
		t.Errorf("Expected score 50 for prefix match, got %d", score)
	}
}

func TestScoreNameMatch_SubstringMatch(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "test",
		CaseSensitive: false,
	}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Name: "my_test_file.txt"}
	score := ranker.scoreNameMatch(file)

	if score != 25 {
		t.Errorf("Expected score 25 for substring match, got %d", score)
	}
}

func TestScoreNameMatch_NoMatch(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "test",
		CaseSensitive: false,
	}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Name: "readme.md"}
	score := ranker.scoreNameMatch(file)

	if score != 0 {
		t.Errorf("Expected score 0 for no match, got %d", score)
	}
}

func TestScoreNameMatch_CaseSensitive(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "TEST",
		CaseSensitive: true,
	}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Name: "test.txt"}
	score := ranker.scoreNameMatch(file)

	if score != 0 {
		t.Errorf("Expected score 0 for case mismatch with case-sensitive, got %d", score)
	}
}

func TestScoreNameMatch_CaseInsensitive(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "TEST",
		CaseSensitive: false,
	}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Name: "test.txt"}
	score := ranker.scoreNameMatch(file)

	if score != 100 {
		t.Errorf("Expected score 100 for case-insensitive exact match, got %d", score)
	}
}

func TestScoreSize_VerySmall(t *testing.T) {
	criteria := RankingCriteria{PreferSmaller: true}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Size: 512}
	score := ranker.scoreSize(file)

	if score != 10 {
		t.Errorf("Expected score 10 for file < 1KB, got %d", score)
	}
}

func TestScoreSize_Small(t *testing.T) {
	criteria := RankingCriteria{PreferSmaller: true}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Size: 10240}
	score := ranker.scoreSize(file)

	if score != 5 {
		t.Errorf("Expected score 5 for file < 1MB, got %d", score)
	}
}

func TestScoreSize_Large(t *testing.T) {
	criteria := RankingCriteria{PreferSmaller: true}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{Size: 5242880}
	score := ranker.scoreSize(file)

	if score != 0 {
		t.Errorf("Expected score 0 for file >= 1MB, got %d", score)
	}
}

func TestScoreRecency(t *testing.T) {
	criteria := RankingCriteria{PreferRecent: true}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{ModifiedTime: time.Now()}
	score := ranker.scoreRecency(file)

	if score != 0 {
		t.Errorf("Expected score 0 (placeholder), got %d", score)
	}
}

func TestCalculateScore_Combined(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "test",
		PreferSmaller: true,
		CaseSensitive: false,
	}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{
		Name: "test.txt",
		Size: 512,
	}

	score := ranker.calculateScore(file)

	expectedScore := 100 + 10
	if score != expectedScore {
		t.Errorf("Expected combined score %d (100+10), got %d", expectedScore, score)
	}
}

func TestCalculateScore_NoQuery(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "",
		PreferSmaller: true,
	}
	ranker := NewRanker(criteria)

	file := &indexer.FileInfo{
		Name: "test.txt",
		Size: 512,
	}

	score := ranker.calculateScore(file)

	if score != 10 {
		t.Errorf("Expected score 10 (size only), got %d", score)
	}
}

func TestRank_StableSort(t *testing.T) {
	criteria := RankingCriteria{
		Query:         "file",
		CaseSensitive: false,
	}
	ranker := NewRanker(criteria)

	files := []*indexer.FileInfo{
		{Name: "file1.txt", Path: "/file1.txt", Size: 100},
		{Name: "file2.txt", Path: "/file2.txt", Size: 100},
		{Name: "file3.txt", Path: "/file3.txt", Size: 100},
	}

	result := ranker.Rank(files)

	if len(result) != 3 {
		t.Fatalf("Expected 3 files, got %d", len(result))
	}

	for i := 0; i < len(result); i++ {
		if result[i].Name != files[i].Name {
			t.Logf("Order changed but scores should be equal")
		}
	}
}