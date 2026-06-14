package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReviewSections_TypeAndLabelContract(t *testing.T) {
	tests := []struct {
		name          string
		section       Section
		wantType      SectionType
		wantIteration int
		wantLabel     string
		noCodex       bool
	}{
		{
			name:          "gemini first pass review",
			section:       NewGeminiReviewSection(0, ": all findings"),
			wantType:      SectionInternalReview,
			wantIteration: 0,
			wantLabel:     "gemini review 0: all findings",
		},
		{
			name:          "internal codex review",
			section:       NewInternalReviewSection(3, ": critical/major"),
			wantType:      SectionInternalReview,
			wantIteration: 3,
			wantLabel:     "review 3: critical/major",
			noCodex:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantType, tc.section.Type)
			assert.Equal(t, tc.wantIteration, tc.section.Iteration)
			assert.Equal(t, tc.wantLabel, tc.section.Label)
			if tc.noCodex {
				assert.NotContains(t, tc.section.Label, "codex")
			}
		})
	}
}
