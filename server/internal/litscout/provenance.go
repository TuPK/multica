package litscout

import (
	"github.com/multica-ai/multica/server/internal/litscout/paper"
	"github.com/multica-ai/multica/server/internal/litscout/source"
)

// ValidatePaper is the provenance check for one selected paper. A date, DOI or URL may be absent;
// none is ever invented.
func ValidatePaper(m paper.Metadata) []string {
	var errs []string
	if !source.Allowed[m.MetadataSource] {
		errs = append(errs, "INVALID_METADATA_SOURCE")
	}
	if m.Abstract != "" && !source.Allowed[m.AbstractSource] {
		errs = append(errs, "INVALID_ABSTRACT_SOURCE")
	}
	if m.Title == "" {
		errs = append(errs, "MISSING_TITLE")
	}
	if len(m.Authors) == 0 {
		errs = append(errs, "MISSING_AUTHORS")
	}
	if m.OpenAlexID == "" && m.DOI == "" && m.URL == "" && m.ArxivID == "" && m.S2ID == "" {
		errs = append(errs, "MISSING_RESOLVABLE_SOURCE_IDENTIFIER")
	}
	return errs
}
