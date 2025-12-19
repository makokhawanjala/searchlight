package searcher

import (
	"strings"

	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// RankingCriteria defines how results should be ranked
type RankingCriteria struct {
	Query          string
	PreferExact    bool
	PreferRecent   bool
	PreferSmaller  bool
	CaseSensitive  bool
}

// Ranker ranks search results based on relevance
type Ranker struct {
	criteria RankingCriteria
}

// NewRanker creates a new Ranker with the given criteria
func NewRanker(criteria RankingCriteria) *Ranker {
	return &Ranker{
		criteria: criteria,
	}
}

// Rank sorts files by relevance score (highest first)
func (r *Ranker) Rank(files []*indexer.FileInfo) []*indexer.FileInfo {
	if len(files) == 0 {
		return files
	}

	scores := make([]struct {
		file  *indexer.FileInfo
		score int
	}, len(files))

	for i, file := range files {
		scores[i].file = file
		scores[i].score = r.calculateScore(file)
	}

	for i := 0; i < len(scores)-1; i++ {
		for j := 0; j < len(scores)-i-1; j++ {
			if scores[j].score < scores[j+1].score {
				scores[j], scores[j+1] = scores[j+1], scores[j]
			}
		}
	}

	ranked := make([]*indexer.FileInfo, len(files))
	for i, s := range scores {
		ranked[i] = s.file
	}

	return ranked
}

// calculateScore computes relevance score for a file
func (r *Ranker) calculateScore(file *indexer.FileInfo) int {
	score := 0

	if r.criteria.Query != "" {
		score += r.scoreNameMatch(file)
	}

	if r.criteria.PreferRecent {
		score += r.scoreRecency(file)
	}

	if r.criteria.PreferSmaller {
		score += r.scoreSize(file)
	}

	return score
}

// scoreNameMatch scores based on how well the filename matches the query
func (r *Ranker) scoreNameMatch(file *indexer.FileInfo) int {
	query := r.criteria.Query
	name := file.Name

	if !r.criteria.CaseSensitive {
		query = strings.ToLower(query)
		name = strings.ToLower(name)
	}

	if name == query {
		return 100
	}

	if r.criteria.PreferExact && strings.HasPrefix(name, query) {
		return 50
	}

	if strings.Contains(name, query) {
		return 25
	}

	return 0
}

// scoreRecency scores based on file modification time
func (r *Ranker) scoreRecency(file *indexer.FileInfo) int {
	return 0
}

// scoreSize scores based on file size (smaller files ranked higher)
func (r *Ranker) scoreSize(file *indexer.FileInfo) int {
	if file.Size < 1024 {
		return 10
	}
	if file.Size < 1024*1024 {
		return 5
	}
	return 0
}