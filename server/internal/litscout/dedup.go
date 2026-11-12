package litscout

import (
	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// fuzzyTitleThreshold is the Indel ratio from which two titles count as the same, and only
// when the first authors also match.
const fuzzyTitleThreshold = 96

func firstAuthor(m paper.Metadata) string {
	if len(m.Authors) == 0 {
		return ""
	}
	return Normalize(m.Authors[0])
}

// Same reports whether two records describe one paper. Evidence is checked in order: DOI,
// source identifiers, normalised title with first author, and finally a fuzzy title that is only
// accepted together with an equal first author. A fuzzy title alone never merges two papers.
func Same(a, b paper.Metadata) bool {
	if a.DOI != "" && a.DOI == b.DOI {
		return true
	}
	if a.OpenAlexID != "" && a.OpenAlexID == b.OpenAlexID {
		return true
	}
	if a.ArxivID != "" && a.ArxivID == b.ArxivID {
		return true
	}
	if a.S2ID != "" && a.S2ID == b.S2ID {
		return true
	}
	ta, tb := Normalize(a.Title), Normalize(b.Title)
	fa, fb := firstAuthor(a), firstAuthor(b)
	authorsEqual := fa != "" && fb != "" && fa == fb
	if ta != "" && ta == tb && authorsEqual {
		return true
	}
	return authorsEqual && ta != "" && tb != "" && IndelRatio(ta, tb) >= fuzzyTitleThreshold
}

// Merge keeps the first record and fills only the fields it lacks from the second. The first
// record's metadata source stays; an abstract taken from the second keeps the second's source.
func Merge(a, b paper.Metadata) paper.Metadata {
	if a.DOI == "" {
		a.DOI = b.DOI
	}
	if a.OpenAlexID == "" {
		a.OpenAlexID = b.OpenAlexID
	}
	if a.ArxivID == "" {
		a.ArxivID = b.ArxivID
	}
	if a.S2ID == "" {
		a.S2ID = b.S2ID
	}
	if a.URL == "" {
		a.URL = b.URL
	}
	if a.PublicationDate == "" {
		a.PublicationDate = b.PublicationDate
	}
	if a.Type == "" {
		a.Type = b.Type
	}
	if a.OpenAccessURL == "" {
		a.OpenAccessURL = b.OpenAccessURL
	}
	if len(a.Authors) == 0 {
		a.Authors = b.Authors
	}
	if len(a.ReferencedWorks) == 0 {
		a.ReferencedWorks = b.ReferencedWorks
	}
	if a.Abstract == "" && b.Abstract != "" {
		a.Abstract = b.Abstract
		a.AbstractSource = b.AbstractSource
		if a.AbstractSource == "" {
			a.AbstractSource = b.MetadataSource
		}
	}
	return a
}

// assignID gives a paper its stable handle from the strongest identifier it has now.
func assignID(m paper.Metadata) string {
	switch {
	case m.DOI != "":
		return paper.NewID("doi:" + m.DOI)
	case m.OpenAlexID != "":
		return paper.NewID("oa:" + m.OpenAlexID)
	case m.ArxivID != "":
		return paper.NewID("arxiv:" + m.ArxivID)
	case m.S2ID != "":
		return paper.NewID("s2:" + m.S2ID)
	}
	return paper.NewID("t:" + Normalize(m.Title) + "|" + firstAuthor(m))
}

// Deduplicate merges incoming records into unique. Existing papers keep their ID even when a later
// record brings a better identifier.
func Deduplicate(unique, incoming []paper.Metadata) []paper.Metadata {
	for _, rec := range incoming {
		matched := false
		for i := range unique {
			if Same(unique[i], rec) {
				unique[i] = Merge(unique[i], rec)
				matched = true
				break
			}
		}
		if !matched {
			rec.ID = assignID(rec)
			unique = append(unique, rec)
		}
	}
	return unique
}
