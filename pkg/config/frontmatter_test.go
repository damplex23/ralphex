package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		opts  Options
		body  string
	}{
		{"no frontmatter", "just a prompt", Options{}, "just a prompt"},
		{"model only", "---\nmodel: flash-lite\n---\nthe prompt", Options{Model: "flash-lite"}, "the prompt"},
		{"agent only", "---\nagent: code-reviewer\n---\nthe prompt", Options{AgentType: "code-reviewer"}, "the prompt"},
		{"both fields", "---\nmodel: flash\nagent: code-reviewer\n---\nthe prompt", Options{Model: "flash", AgentType: "code-reviewer"}, "the prompt"},
		{"unclosed frontmatter", "---\nmodel: flash-lite\nno closing", Options{}, "---\nmodel: flash-lite\nno closing"},
		{"empty body after frontmatter", "---\nmodel: flash-lite\n---\n", Options{Model: "flash-lite"}, ""},
		{"unknown keys ignored", "---\nmodel: pro\nfoo: bar\n---\nbody", Options{Model: "pro"}, "body"},
		{"whitespace in values", "---\nmodel:  flash-lite  \nagent:  code-reviewer  \n---\nbody", Options{Model: "flash-lite", AgentType: "code-reviewer"}, "body"},
		{"malformed yaml", "---\n: :\n  bad:\n---\nbody", Options{}, "---\n: :\n  bad:\n---\nbody"},

		// closing delimiter must be on its own line
		{"closing delimiter not on own line", "---\nmodel: flash-lite\n---extra\nbody", Options{}, "---\nmodel: flash-lite\n---extra\nbody"},
		{"closing delimiter with trailing text", "---\nmodel: flash-lite\n--- body", Options{}, "---\nmodel: flash-lite\n--- body"},

		// empty and minimal frontmatter
		{"empty frontmatter block", "---\n---\nbody", Options{}, "---\n---\nbody"},
		{"frontmatter only no trailing newline", "---\nmodel: flash-lite\n---", Options{Model: "flash-lite"}, ""},

		// yaml edge cases
		// model normalization
		{"full model id normalized", "---\nmodel: gemini-flash-4-5-20250929\n---\nbody", Options{Model: "flash"}, "body"},
		{"full model id flash-lite normalized", "---\nmodel: gemini-flash-lite-4-5-20251001\n---\nbody", Options{Model: "flash-lite"}, "body"},
		{"full model id pro normalized", "---\nmodel: gemini-pro-4-6\n---\nbody", Options{Model: "pro"}, "body"},
		{"full model id pro-exp normalized", "---\nmodel: gemini-pro-exp-5\n---\nbody", Options{Model: "pro-exp"}, "body"},
		{"model keyword preserved", "---\nmodel: flash\n---\nbody", Options{Model: "flash"}, "body"},
		{"unknown model kept as-is", "---\nmodel: gpt-5\n---\nbody", Options{Model: "gpt-5"}, "body"},

		{"yaml type mismatch model number", "---\nmodel: 123\n---\nbody", Options{Model: "123"}, "body"},
		{"yaml null value", "---\nmodel: null\n---\nbody", Options{}, "body"},
		{"duplicate keys rejected", "---\nmodel: flash-lite\nmodel: pro\n---\nbody", Options{}, "---\nmodel: flash-lite\nmodel: pro\n---\nbody"},

		// body with dashes
		{"body contains triple dashes", "---\nmodel: flash-lite\n---\nsome text\n---\nmore text", Options{Model: "flash-lite"}, "some text\n---\nmore text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, body := parseOptions(tt.input)
			assert.Equal(t, tt.opts, opts)
			assert.Equal(t, tt.body, body)
		})
	}
}

func TestOptions_String(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"empty options", Options{}, "model=default, subagent=general-purpose"},
		{"model only", Options{Model: "flash-lite"}, "model=flash-lite, subagent=general-purpose"},
		{"agent only", Options{AgentType: "code-reviewer"}, "model=default, subagent=code-reviewer"},
		{"both fields", Options{Model: "pro", AgentType: "code-reviewer"}, "model=pro, subagent=code-reviewer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.opts.String())
		})
	}
}

func TestOptions_Validate(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		warnings []string
	}{
		{"empty options", Options{}, nil},
		{"valid model flash-lite", Options{Model: "flash-lite"}, nil},
		{"valid model flash", Options{Model: "flash"}, nil},
		{"valid model pro", Options{Model: "pro"}, nil},
		{"valid model pro-exp", Options{Model: "pro-exp"}, nil},
		{"unknown model", Options{Model: "gpt-5"}, []string{`unknown model "gpt-5", must be one of: flash-lite, flash, pro, pro-exp`}},
		{"agent type not validated", Options{AgentType: "anything-goes"}, nil},
		{"unknown model with agent", Options{Model: "bad", AgentType: "reviewer"}, []string{`unknown model "bad", must be one of: flash-lite, flash, pro, pro-exp`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.warnings, tt.opts.Validate())
		})
	}
}
