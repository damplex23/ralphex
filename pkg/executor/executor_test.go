package executor

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/ralphex/pkg/executor/mocks"
	"github.com/umputun/ralphex/pkg/status"
)

func TestGeminiExecutor_Run_Success(t *testing.T) {
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello world <<<RALPHEX:ALL_TASKS_DONE>>>"}}`

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "Hello world <<<RALPHEX:ALL_TASKS_DONE>>>", result.Output)
	assert.Equal(t, "<<<RALPHEX:ALL_TASKS_DONE>>>", result.Signal)
}

func TestGeminiExecutor_Run_StartError(t *testing.T) {
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return nil, nil, errors.New("command not found")
		},
	}
	e := &GeminiExecutor{cmdRunner: mock}

	result := e.Run(context.Background(), "test prompt")

	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "command not found")
}

func TestGeminiExecutor_Run_WaitError_WithOutput(t *testing.T) {
	// non-zero exit with output but no signal should propagate error
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"partial output"}}`

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return errors.New("exit status 1") }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock}

	result := e.Run(context.Background(), "test prompt")

	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "gemini exited with error")
	assert.Equal(t, "partial output", result.Output)
}

func TestGeminiExecutor_Run_WaitError_WithOutputAndSignal(t *testing.T) {
	// non-zero exit with output AND signal should ignore exit code (useful work was done)
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"task done <<<RALPHEX:ALL_TASKS_DONE>>>"}}`

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return errors.New("exit status 1") }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "task done <<<RALPHEX:ALL_TASKS_DONE>>>", result.Output)
	assert.Equal(t, "<<<RALPHEX:ALL_TASKS_DONE>>>", result.Signal)
}

func TestGeminiExecutor_Run_WaitError_NoOutput(t *testing.T) {
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(""), func() error { return errors.New("exit status 1") }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock}

	result := e.Run(context.Background(), "test prompt")

	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "gemini exited with error")
}

func TestGeminiExecutor_Run_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(""), func() error { return context.Canceled }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock}

	result := e.Run(ctx, "test prompt")

	require.ErrorIs(t, result.Error, context.Canceled)
}

func TestGeminiExecutor_Run_WithOutputHandler(t *testing.T) {
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"chunk1"}}
{"type":"content_block_delta","delta":{"type":"text_delta","text":"chunk2"}}`

	var chunks []string
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner:     mock,
		OutputHandler: func(text string) { chunks = append(chunks, text) },
	}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "chunk1chunk2", result.Output)
	assert.Equal(t, []string{"chunk1", "chunk2"}, chunks)
}

func TestGeminiExecutor_parseStream(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantOutput string
		wantSignal string
	}{
		{
			name:       "content block delta",
			input:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello world"}}`,
			wantOutput: "Hello world",
			wantSignal: "",
		},
		{
			name: "multiple deltas",
			input: `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello "}}
{"type":"content_block_delta","delta":{"type":"text_delta","text":"world"}}`,
			wantOutput: "Hello world",
			wantSignal: "",
		},
		{
			name:       "completed signal",
			input:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Task done. <<<RALPHEX:ALL_TASKS_DONE>>>"}}`,
			wantOutput: "Task done. <<<RALPHEX:ALL_TASKS_DONE>>>",
			wantSignal: "<<<RALPHEX:ALL_TASKS_DONE>>>",
		},
		{
			name:       "failed signal",
			input:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Could not finish. <<<RALPHEX:TASK_FAILED>>>"}}`,
			wantOutput: "Could not finish. <<<RALPHEX:TASK_FAILED>>>",
			wantSignal: "<<<RALPHEX:TASK_FAILED>>>",
		},
		{
			name:       "review done signal",
			input:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Review complete. <<<RALPHEX:REVIEW_DONE>>>"}}`,
			wantOutput: "Review complete. <<<RALPHEX:REVIEW_DONE>>>",
			wantSignal: "<<<RALPHEX:REVIEW_DONE>>>",
		},
		{
			name:       "codex done signal",
			input:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Codex done. <<<RALPHEX:CODEX_REVIEW_DONE>>>"}}`,
			wantOutput: "Codex done. <<<RALPHEX:CODEX_REVIEW_DONE>>>",
			wantSignal: "<<<RALPHEX:CODEX_REVIEW_DONE>>>",
		},
		{
			name:       "plan ready signal",
			input:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Plan complete. <<<RALPHEX:PLAN_READY>>>"}}`,
			wantOutput: "Plan complete. <<<RALPHEX:PLAN_READY>>>",
			wantSignal: "<<<RALPHEX:PLAN_READY>>>",
		},
		{
			name:       "result type",
			input:      `{"type":"result","result":{"output":"Final output"}}`,
			wantOutput: "Final output",
			wantSignal: "",
		},
		{
			name:       "empty lines ignored",
			input:      "\n\n" + `{"type":"content_block_delta","delta":{"type":"text_delta","text":"text"}}` + "\n\n",
			wantOutput: "text",
			wantSignal: "",
		},
		{
			name:       "non-json lines printed as-is",
			input:      "not json\n" + `{"type":"content_block_delta","delta":{"type":"text_delta","text":"valid"}}`,
			wantOutput: "not json\nvalid",
			wantSignal: "",
		},
		{
			name:       "unknown event type",
			input:      `{"type":"unknown_type","data":"something"}`,
			wantOutput: "",
			wantSignal: "",
		},
		{
			name:       "assistant event type",
			input:      `{"type":"assistant","message":{"content":[{"type":"text","text":"assistant output"}]}}`,
			wantOutput: "assistant output",
			wantSignal: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &GeminiExecutor{}
			result := e.parseStream(context.Background(), strings.NewReader(tc.input), func() {})

			assert.Equal(t, tc.wantOutput, result.Output)
			assert.Equal(t, tc.wantSignal, result.Signal)
		})
	}
}

func TestGeminiExecutor_parseStream_withHandler(t *testing.T) {
	input := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"chunk1"}}
{"type":"content_block_delta","delta":{"type":"text_delta","text":"chunk2"}}`

	var chunks []string
	e := &GeminiExecutor{
		OutputHandler: func(text string) {
			chunks = append(chunks, text)
		},
	}

	result := e.parseStream(context.Background(), strings.NewReader(input), func() {})

	assert.Equal(t, "chunk1chunk2", result.Output)
	assert.Equal(t, []string{"chunk1", "chunk2"}, chunks)
}

func TestGeminiExecutor_parseStream_withDebug(t *testing.T) {
	// non-json lines should be printed as-is (with debug message)
	input := "not json\n" + `{"type":"content_block_delta","delta":{"type":"text_delta","text":"valid"}}`

	e := &GeminiExecutor{Debug: true}
	result := e.parseStream(context.Background(), strings.NewReader(input), func() {})

	assert.Equal(t, "not json\nvalid", result.Output)
}

func TestGeminiExecutor_extractText(t *testing.T) {
	e := &GeminiExecutor{}

	t.Run("assistant event with text", func(t *testing.T) {
		event := streamEvent{Type: "assistant", Message: []byte(`{"content":[{"type":"text","text":"assistant message"}]}`)}
		assert.Equal(t, "assistant message", e.extractText(&event))
	})

	t.Run("assistant event with multiple text blocks", func(t *testing.T) {
		event := streamEvent{Type: "assistant", Message: []byte(`{"content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}`)}
		assert.Equal(t, "firstsecond", e.extractText(&event))
	})

	t.Run("assistant event with empty content", func(t *testing.T) {
		event := streamEvent{Type: "assistant"}
		assert.Empty(t, e.extractText(&event))
	})

	t.Run("content block delta", func(t *testing.T) {
		event := streamEvent{Type: "content_block_delta", Delta: []byte(`{"type":"text_delta","text":"hello"}`)}
		assert.Equal(t, "hello", e.extractText(&event))
	})

	t.Run("non-text delta", func(t *testing.T) {
		event := streamEvent{Type: "content_block_delta", Delta: []byte(`{"type":"tool_use","text":"ignored"}`)}
		assert.Empty(t, e.extractText(&event))
	})

	t.Run("result with object", func(t *testing.T) {
		event := streamEvent{Type: "result"}
		event.Result = []byte(`{"output":"final"}`)
		assert.Equal(t, "final", e.extractText(&event))
	})

	t.Run("result with string skipped", func(t *testing.T) {
		// session summary format - content already streamed, should be skipped
		event := streamEvent{Type: "result"}
		event.Result = []byte(`"Task completed"`)
		assert.Empty(t, e.extractText(&event))
	})

	t.Run("message_stop with text content", func(t *testing.T) {
		event := streamEvent{Type: "message_stop", Message: []byte(`{"content":[{"type":"text","text":"final message"}]}`)}
		assert.Equal(t, "final message", e.extractText(&event))
	})

	t.Run("message_stop with non-text content", func(t *testing.T) {
		event := streamEvent{Type: "message_stop", Message: []byte(`{"content":[{"type":"tool_use","text":"ignored"}]}`)}
		assert.Empty(t, e.extractText(&event))
	})

	t.Run("message_stop with empty content", func(t *testing.T) {
		event := streamEvent{Type: "message_stop"}
		assert.Empty(t, e.extractText(&event))
	})

	t.Run("unknown type", func(t *testing.T) {
		event := streamEvent{Type: "ping"}
		assert.Empty(t, e.extractText(&event))
	})
}

func TestDetectSignal(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"some text", ""},
		{"task done " + status.Completed, status.Completed},
		{status.Failed + " error", status.Failed},
		{"review complete " + status.ReviewDone, status.ReviewDone},
		{status.CodexDone + " analysis done", status.CodexDone},
		{"plan complete " + status.PlanReady, status.PlanReady},
		{`I have inspected the codebase and confirmed all tasks are done.
The plan file shows every checkbox marked, tests pass locally, and the linter is clean.

<<<RALPHEX:ALL_TASKS_DONE>>>`, status.Completed},
		{`Round 1 review summary follows.

The implementation looks complete. Tests cover the new behavior.

<<<RALPHEX:REVIEW_DONE>>>

Additional thoughts: future work could explore caching.`, status.ReviewDone},
		{`External review iteration finished.
<<<RALPHEX:CODEX_REVIEW_DONE>>>
Note: a minor formatting preference was noted but not flagged.`, status.CodexDone},
		{`Attempted to run go test ./... but encountered a compilation error.

<<<RALPHEX:TASK_FAILED>>>`, status.Failed},
		{`Plan file written to docs/plans/20260514-feature.md.

<<<RALPHEX:PLAN_READY>>>`, status.PlanReady},
		{"no signal here", ""},
	}

	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			got := detectSignal(tc.text)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGeminiExecutor_Run_WithCustomCommand(t *testing.T) {
	var capturedCmd string
	var capturedArgs []string
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, name string, args ...string) (io.Reader, func() error, error) {
			capturedCmd = name
			capturedArgs = args
			return strings.NewReader(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner: mock,
		Command:   "my-gemini",
	}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "my-gemini", capturedCmd)
	// should still use default args
	assert.Contains(t, capturedArgs, "--yolo")
}

func TestGeminiExecutor_Run_WithCustomArgs(t *testing.T) {
	var capturedArgs []string
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, args ...string) (io.Reader, func() error, error) {
			capturedArgs = args
			return strings.NewReader(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner: mock,
		Args:      "--custom-arg --another-arg value",
	}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	// should use custom args plus --prompt "" (non-interactive mode flag, always appended)
	assert.Equal(t, []string{"--custom-arg", "--another-arg", "value", "--prompt", ""}, capturedArgs)
}

func TestGeminiExecutor_Run_WithExplicitEmptyArgs(t *testing.T) {
	var capturedArgs []string
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, args ...string) (io.Reader, func() error, error) {
			capturedArgs = args
			return strings.NewReader(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner: mock,
		ArgsSet:   true,
	}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, []string{"--prompt", ""}, capturedArgs)
}

func TestGeminiExecutor_Run_WithCustomCommandAndArgs(t *testing.T) {
	var capturedCmd string
	var capturedArgs []string
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, name string, args ...string) (io.Reader, func() error, error) {
			capturedCmd = name
			capturedArgs = args
			return strings.NewReader(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner: mock,
		Command:   "custom-gemini",
		Args:      `--skip-perms --prompt ""`,
	}

	result := e.Run(context.Background(), "the prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "custom-gemini", capturedCmd)
	assert.Equal(t, []string{"--skip-perms", "--prompt", ""}, capturedArgs)
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "simple args", input: "--flag1 --flag2 value", want: []string{"--flag1", "--flag2", "value"}},
		{name: "double quoted", input: `--flag "value with spaces"`, want: []string{"--flag", "value with spaces"}},
		{name: "single quoted", input: `--flag 'value with spaces'`, want: []string{"--flag", "value with spaces"}},
		{name: "empty string", input: "", want: nil},
		{name: "only spaces", input: "   ", want: nil},
		{name: "multiple spaces between", input: "arg1   arg2", want: []string{"arg1", "arg2"}},
		{name: "mixed quotes", input: `--a "b" --c 'd'`, want: []string{"--a", "b", "--c", "d"}},
		{name: "escaped quote", input: `--flag \"quoted\"`, want: []string{"--flag", `"quoted"`}},
		{name: "real gemini args", input: `--yolo --output-format stream-json --prompt ""`, want: []string{"--yolo", "--output-format", "stream-json", "--prompt", ""}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitArgs(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStripFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		flag string
		want []string
	}{
		{name: "removes flag and value", args: []string{"--prompt", "", "--model", "opus", "--prompt", ""}, flag: "--model", want: []string{"--prompt", "", "--prompt", ""}},
		{name: "flag not present", args: []string{"--prompt", "", "--prompt", ""}, flag: "--model", want: []string{"--prompt", "", "--prompt", ""}},
		{name: "flag at end with value", args: []string{"--prompt", "", "--model", "opus"}, flag: "--model", want: []string{"--prompt", ""}},
		{name: "empty args", args: []string{}, flag: "--model", want: []string{}},
		{name: "removes equals form", args: []string{"--prompt", "", "--model=opus", "--prompt", ""}, flag: "--model", want: []string{"--prompt", "", "--prompt", ""}},
		{name: "removes equals form at end", args: []string{"--prompt", "", "--model=opus"}, flag: "--model", want: []string{"--prompt", ""}},
		{name: "removes bare flag at end", args: []string{"--prompt", "", "--model"}, flag: "--model", want: []string{"--prompt", ""}},
		{name: "removes repeated occurrences", args: []string{"--model", "opus", "--prompt", "", "--model=sonnet"}, flag: "--model", want: []string{"--prompt", ""}},
		{name: "does not match prefix-only", args: []string{"--model-foo", "bar", "--prompt", ""}, flag: "--model", want: []string{"--model-foo", "bar", "--prompt", ""}},
		{name: "bare flag in middle preserves next flag", args: []string{"--prompt", "", "--model", "--prompt", ""}, flag: "--model", want: []string{"--prompt", "", "--prompt", ""}},
		{name: "bare flag preserves next flag with dash value", args: []string{"--model", "-x", "--prompt", ""}, flag: "--model", want: []string{"-x", "--prompt", ""}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripFlag(tc.args, tc.flag)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFilterEnv(t *testing.T) {
	tests := []struct {
		name   string
		env    []string
		remove []string
		want   []string
	}{
		{
			name:   "removes single key",
			env:    []string{"FOO=bar", "BAZ=qux", "GEMINI_API_KEY=secret"},
			remove: []string{"GEMINI_API_KEY"},
			want:   []string{"FOO=bar", "BAZ=qux"},
		},
		{
			name:   "removes multiple keys",
			env:    []string{"A=1", "B=2", "C=3"},
			remove: []string{"A", "C"},
			want:   []string{"B=2"},
		},
		{
			name:   "no match returns original",
			env:    []string{"FOO=bar", "BAZ=qux"},
			remove: []string{"NONEXISTENT"},
			want:   []string{"FOO=bar", "BAZ=qux"},
		},
		{
			name:   "empty env returns empty",
			env:    []string{},
			remove: []string{"FOO"},
			want:   []string{},
		},
		{
			name:   "partial key match not removed",
			env:    []string{"GEMINI_API_KEY_OLD=secret", "GEMINI_API_KEY=new"},
			remove: []string{"GEMINI_API_KEY"},
			want:   []string{"GEMINI_API_KEY_OLD=secret"},
		},
		{
			name:   "removes GEMINICODE and GEMINI_API_KEY together",
			env:    []string{"PATH=/usr/bin", "GEMINICODE=1", "GEMINI_API_KEY=secret", "HOME=/home/user"},
			remove: []string{"GEMINI_API_KEY", "GEMINICODE"},
			want:   []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterEnv(tc.env, tc.remove...)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGeminiChildEnv(t *testing.T) {
	tests := []struct {
		name           string
		env            []string
		preserveAPIKey bool
		want           []string
	}{
		{
			name:           "default strips both GEMINI_API_KEY and GEMINICODE",
			env:            []string{"PATH=/usr/bin", "GEMINICODE=1", "GEMINI_API_KEY=secret", "HOME=/home/user"},
			preserveAPIKey: false,
			want:           []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:           "preserve keeps GEMINI_API_KEY but still strips GEMINICODE",
			env:            []string{"PATH=/usr/bin", "GEMINICODE=1", "GEMINI_API_KEY=secret", "HOME=/home/user"},
			preserveAPIKey: true,
			want:           []string{"PATH=/usr/bin", "GEMINI_API_KEY=secret", "HOME=/home/user"},
		},
		{
			name:           "preserve with no api key in env keeps everything except GEMINICODE",
			env:            []string{"PATH=/usr/bin", "GEMINICODE=1", "HOME=/home/user"},
			preserveAPIKey: true,
			want:           []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:           "default with no api key in env still strips GEMINICODE",
			env:            []string{"PATH=/usr/bin", "GEMINICODE=1", "HOME=/home/user"},
			preserveAPIKey: false,
			want:           []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:           "preserve does not affect partial-match keys like GEMINI_API_KEY_OLD",
			env:            []string{"GEMINI_API_KEY_OLD=old", "GEMINI_API_KEY=new", "GEMINICODE=1"},
			preserveAPIKey: true,
			want:           []string{"GEMINI_API_KEY_OLD=old", "GEMINI_API_KEY=new"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := geminiChildEnv(tc.env, tc.preserveAPIKey)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGeminiExecutor_parseStream_largeLines(t *testing.T) {
	// test that lines of arbitrary length are handled without limit

	tests := []struct {
		name string
		size int
	}{
		{"100KB line", 100 * 1024},
		{"500KB line", 500 * 1024},
		{"1MB line", 1024 * 1024},
		{"2MB line", 2 * 1024 * 1024},
		{"65MB line", 65 * 1024 * 1024},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.size >= 65*1024*1024 && testing.Short() {
				t.Skip("skipping 65MB allocation in short mode")
			}
			// create a large text payload
			largeText := strings.Repeat("x", tc.size)
			jsonLine := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"` + largeText + `"}}`

			e := &GeminiExecutor{}
			result := e.parseStream(context.Background(), strings.NewReader(jsonLine), func() {})

			require.NoError(t, result.Error, "should handle %d byte line without error", tc.size)
			assert.Len(t, result.Output, tc.size, "output should contain full text")
		})
	}
}

func TestGeminiExecutor_parseStream_multipleLargeLines(t *testing.T) {
	// test multiple large lines in sequence (simulates parallel agent output)
	lineSize := 200 * 1024 // 200KB per line
	numLines := 5          // simulate 5 parallel agents

	lines := make([]string, 0, numLines)
	for i := range numLines {
		text := strings.Repeat(string(rune('a'+i)), lineSize)
		lines = append(lines, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"`+text+`"}}`)
	}
	input := strings.Join(lines, "\n")

	e := &GeminiExecutor{}
	result := e.parseStream(context.Background(), strings.NewReader(input), func() {})

	require.NoError(t, result.Error)
	assert.Len(t, result.Output, lineSize*numLines, "should contain all output from all lines")
}

func TestPatternMatchError_Error(t *testing.T) {
	err := &PatternMatchError{Pattern: "rate limit exceeded", HelpCmd: "gemini /help"}
	assert.Equal(t, `detected error pattern: "rate limit exceeded"`, err.Error())
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		patterns []string
		want     string
	}{
		{name: "no patterns", output: "some output", patterns: nil, want: ""},
		{name: "empty patterns slice", output: "some output", patterns: []string{}, want: ""},
		{name: "no match", output: "everything is fine", patterns: []string{"error", "failed"}, want: ""},
		{name: "exact match", output: "You've hit your limit", patterns: []string{"You've hit your limit"}, want: "You've hit your limit"},
		{name: "substring match", output: "Error: You've hit your limit today", patterns: []string{"hit your limit"}, want: "hit your limit"},
		{name: "case insensitive", output: "YOU'VE HIT YOUR LIMIT", patterns: []string{"you've hit your limit"}, want: "you've hit your limit"},
		{name: "mixed case match", output: "Rate Limit Exceeded", patterns: []string{"rate limit exceeded"}, want: "rate limit exceeded"},
		{name: "first pattern wins", output: "rate limit and quota exceeded", patterns: []string{"rate limit", "quota exceeded"}, want: "rate limit"},
		{name: "second pattern matches", output: "your quota exceeded the limit", patterns: []string{"rate limit", "quota exceeded"}, want: "quota exceeded"},
		{name: "empty pattern skipped", output: "some text", patterns: []string{"", "some"}, want: "some"},
		{name: "whitespace in pattern", output: "rate  limit", patterns: []string{"rate  limit"}, want: "rate  limit"},
		{name: "multiline output", output: "line1\nYou've hit your limit\nline3", patterns: []string{"hit your limit"}, want: "hit your limit"},
		{name: "api error 500", output: `API Error: 500 {"type":"error","error":{"type":"api_error","message":"Internal server error"}}`, patterns: []string{"API Error:"}, want: "API Error:"},
		{name: "not logged in", output: "Not logged in · Please run /login", patterns: []string{"Not logged in"}, want: "Not logged in"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchPattern(tc.output, tc.patterns)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGeminiExecutor_Run_ErrorPattern(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		patterns    []string
		wantError   bool
		wantPattern string
		wantHelpCmd string
		wantOutput  string
	}{
		{
			name:       "no patterns configured",
			output:     `{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit"}}`,
			patterns:   nil,
			wantError:  false,
			wantOutput: "You've hit your limit",
		},
		{
			name:       "pattern not matched",
			output:     `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Task completed successfully"}}`,
			patterns:   []string{"rate limit", "quota exceeded"},
			wantError:  false,
			wantOutput: "Task completed successfully",
		},
		{
			name:        "pattern matched",
			output:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Error: You've hit your limit for today"}}`,
			patterns:    []string{"hit your limit"},
			wantError:   true,
			wantPattern: "hit your limit",
			wantHelpCmd: "gemini /help",
			wantOutput:  "Error: You've hit your limit for today",
		},
		{
			name:        "case insensitive match",
			output:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"RATE LIMIT EXCEEDED"}}`,
			patterns:    []string{"rate limit exceeded"},
			wantError:   true,
			wantPattern: "rate limit exceeded",
			wantHelpCmd: "gemini /help",
			wantOutput:  "RATE LIMIT EXCEEDED",
		},
		{
			name:        "first matching pattern returned",
			output:      `{"type":"content_block_delta","delta":{"type":"text_delta","text":"rate limit and quota exceeded"}}`,
			patterns:    []string{"rate limit", "quota exceeded"},
			wantError:   true,
			wantPattern: "rate limit",
			wantHelpCmd: "gemini /help",
			wantOutput:  "rate limit and quota exceeded",
		},
		{
			name:        "not logged in detected as error",
			output:      "Not logged in \u00b7 Please run /login\n",
			patterns:    []string{"Not logged in"},
			wantError:   true,
			wantPattern: "Not logged in",
			wantHelpCmd: "gemini /help",
			wantOutput:  "Not logged in \u00b7 Please run /login\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mocks.CommandRunnerMock{
				RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
					return strings.NewReader(tc.output), func() error { return nil }, nil
				},
			}
			e := &GeminiExecutor{
				cmdRunner:     mock,
				ErrorPatterns: tc.patterns,
			}

			result := e.Run(context.Background(), "test prompt")

			assert.Equal(t, tc.wantOutput, result.Output)

			if tc.wantError {
				require.Error(t, result.Error)
				var patternErr *PatternMatchError
				require.ErrorAs(t, result.Error, &patternErr)
				assert.Equal(t, tc.wantPattern, patternErr.Pattern)
				assert.Equal(t, tc.wantHelpCmd, patternErr.HelpCmd)
			} else {
				require.NoError(t, result.Error)
			}
		})
	}
}

func TestGeminiExecutor_Run_WaitError_WithOutputAndErrorPattern(t *testing.T) {
	// non-zero exit + output matching error pattern → PatternMatchError takes precedence
	jsonStream := "Error: Gemini Code cannot be launched inside another Gemini Code session.\n"

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return errors.New("exit status 1") }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner:     mock,
		ErrorPatterns: []string{"cannot be launched inside another Gemini Code session"},
	}

	result := e.Run(context.Background(), "test prompt")

	require.Error(t, result.Error)
	var patternErr *PatternMatchError
	require.ErrorAs(t, result.Error, &patternErr)
	assert.Equal(t, "cannot be launched inside another Gemini Code session", patternErr.Pattern)
	assert.Contains(t, result.Output, "cannot be launched inside another Gemini Code session")
	assert.Empty(t, result.Signal)
}

func TestGeminiExecutor_Run_WaitError_WithSignalAndErrorPattern(t *testing.T) {
	// non-zero exit + output with signal + error pattern → PatternMatchError takes precedence (signal present skips exit error)
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit <<<RALPHEX:ALL_TASKS_DONE>>>"}}`

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return errors.New("exit status 1") }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner:     mock,
		ErrorPatterns: []string{"hit your limit"},
	}

	result := e.Run(context.Background(), "test prompt")

	require.Error(t, result.Error)
	var patternErr *PatternMatchError
	require.ErrorAs(t, result.Error, &patternErr)
	assert.Equal(t, "hit your limit", patternErr.Pattern)
	assert.Contains(t, result.Output, "You've hit your limit")
	assert.Equal(t, "<<<RALPHEX:ALL_TASKS_DONE>>>", result.Signal)
}

func TestGeminiExecutor_Run_ErrorPattern_WithSignal(t *testing.T) {
	// error pattern should still be detected even when output contains a signal
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit <<<RALPHEX:ALL_TASKS_DONE>>>"}}`

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner:     mock,
		ErrorPatterns: []string{"hit your limit"},
	}

	result := e.Run(context.Background(), "test prompt")

	// should have error due to pattern match
	require.Error(t, result.Error)
	var patternErr *PatternMatchError
	require.ErrorAs(t, result.Error, &patternErr)
	assert.Equal(t, "hit your limit", patternErr.Pattern)

	// should preserve output and signal
	assert.Contains(t, result.Output, "You've hit your limit")
	assert.Equal(t, "<<<RALPHEX:ALL_TASKS_DONE>>>", result.Signal)
}

func TestLimitPatternError_Error(t *testing.T) {
	err := &LimitPatternError{Pattern: "You've hit your limit", HelpCmd: "gemini /help"}
	assert.Equal(t, `detected limit pattern: "You've hit your limit"`, err.Error())
}

func TestRetryPatternError_Error(t *testing.T) {
	err := &RetryPatternError{Pattern: "FYA_TRANSIENT_TIMEOUT"}
	assert.Equal(t, `detected retry pattern: "FYA_TRANSIENT_TIMEOUT"`, err.Error())
}

func TestGeminiExecutor_Run_DetectsRetryPatternFromNonJSONLine(t *testing.T) {
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			out := "2026/06/02 13:18:04.138 [ERROR] run turn: turn canceled: " +
				"context deadline exceeded: FYA_TRANSIENT_TIMEOUT: gemini turn did not complete before fya turn timeout\n"
			return strings.NewReader(out), func() error { return errors.New("exit status 1") }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock, RetryPatterns: []string{"FYA_TRANSIENT_TIMEOUT"}}

	result := e.Run(context.Background(), "test prompt")

	var retryErr *RetryPatternError
	require.ErrorAs(t, result.Error, &retryErr)
	assert.Equal(t, "FYA_TRANSIENT_TIMEOUT", retryErr.Pattern)
	assert.Contains(t, result.Output, "FYA_TRANSIENT_TIMEOUT")
}

func TestGeminiExecutor_Run_RetryPatternTakesPriorityOverLimitAndError(t *testing.T) {
	// when recent text matches retry, limit, and error patterns at once, retry wins (highest priority)
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta",` +
		`"text":"FYA_TRANSIENT_TIMEOUT and You've hit your limit and API Error: 500"}}`
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{
		cmdRunner:     mock,
		RetryPatterns: []string{"FYA_TRANSIENT_TIMEOUT"},
		LimitPatterns: []string{"You've hit your limit"},
		ErrorPatterns: []string{"API Error:"},
	}

	result := e.Run(context.Background(), "test prompt")

	var retryErr *RetryPatternError
	require.ErrorAs(t, result.Error, &retryErr, "retry pattern must win over limit and error patterns")
	assert.Equal(t, "FYA_TRANSIENT_TIMEOUT", retryErr.Pattern)
}

func TestGeminiExecutor_Run_RetryPatternSkippedWhenSignalPresent(t *testing.T) {
	// a stray retry marker must not discard a completed run: when gemini emits a completion
	// signal, retry detection is skipped so the signal survives instead of forcing a re-run.
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta",` +
		`"text":"done FYA_TRANSIENT_TIMEOUT <<<RALPHEX:ALL_TASKS_DONE>>>"}}`
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock, RetryPatterns: []string{"FYA_TRANSIENT_TIMEOUT"}}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error, "retry pattern must not fire when a completion signal is present")
	assert.Equal(t, status.Completed, result.Signal, "completion signal must survive")
}

func TestGeminiExecutor_Run_IdleTimeoutDetectsRetryPattern(t *testing.T) {
	// when idle timeout fires after a transient retry marker, the retry pattern should be detected
	// instead of silently returning an idle timeout, so the phase retries the session.
	pr, pw := io.Pipe()

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(ctx context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			go func() {
				defer pw.Close()
				fmt.Fprintln(pw, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"FYA_TRANSIENT_TIMEOUT"}}`)
				<-ctx.Done()
			}()
			return pr, func() error {
				<-ctx.Done()
				return errors.New("signal: killed")
			}, nil
		},
	}

	e := &GeminiExecutor{
		cmdRunner:     mock,
		IdleTimeout:   100 * time.Millisecond,
		RetryPatterns: []string{"FYA_TRANSIENT_TIMEOUT"},
	}
	result := e.Run(context.Background(), "test prompt")

	var retryErr *RetryPatternError
	require.ErrorAs(t, result.Error, &retryErr, "should return RetryPatternError")
	assert.Equal(t, "FYA_TRANSIENT_TIMEOUT", retryErr.Pattern)
	assert.False(t, result.IdleTimedOut, "IdleTimedOut should not be set when pattern matched")
}

func TestGeminiExecutor_Run_IdleTimeoutFires(t *testing.T) {
	// idle timeout fires when no output comes after the first line
	pr, pw := io.Pipe()

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(ctx context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			go func() {
				defer pw.Close()
				// send one line then go silent, simulating a hang
				fmt.Fprintln(pw, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`)
				<-ctx.Done() // wait for idle timeout to cancel context and kill process
			}()
			return pr, func() error {
				<-ctx.Done()
				return errors.New("signal: killed")
			}, nil
		},
	}

	e := &GeminiExecutor{cmdRunner: mock, IdleTimeout: 100 * time.Millisecond}
	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "hello", result.Output)
	assert.True(t, result.IdleTimedOut, "IdleTimedOut should be set when idle timeout fires")
}

func TestGeminiExecutor_Run_IdleTimeoutNotFiredOnContinuousOutput(t *testing.T) {
	// continuous output keeps resetting the timer, so idle timeout never fires
	pr, pw := io.Pipe()

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			go func() {
				defer pw.Close()
				for range 5 {
					fmt.Fprintln(pw, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"x"}}`)
					time.Sleep(30 * time.Millisecond) // well within idle timeout
				}
			}()
			return pr, func() error { return nil }, nil
		},
	}

	e := &GeminiExecutor{cmdRunner: mock, IdleTimeout: 200 * time.Millisecond}
	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "xxxxx", result.Output)
}

func TestGeminiExecutor_Run_IdleTimeoutDisabledWhenZero(t *testing.T) {
	// default behavior: IdleTimeout=0 means no idle timeout, runs normally
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"output"}}`

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock} // IdleTimeout is zero (default)

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "output", result.Output)
	assert.Zero(t, e.IdleTimeout)
	assert.False(t, result.IdleTimedOut, "IdleTimedOut should be false when idle timeout is disabled")
}

func TestGeminiExecutor_Run_IdleTimeoutWithSessionTimeout(t *testing.T) {
	// when both session timeout and idle timeout are set, idle timeout fires first
	// if the session goes silent, even though session timeout is still alive
	pr, pw := io.Pipe()

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(ctx context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			go func() {
				defer pw.Close()
				fmt.Fprintln(pw, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`)
				<-ctx.Done()
			}()
			return pr, func() error {
				<-ctx.Done()
				return errors.New("signal: killed")
			}, nil
		},
	}

	// session timeout is 5s (long), idle timeout is 100ms (short) — idle fires first
	sessionCtx, sessionCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sessionCancel()

	e := &GeminiExecutor{cmdRunner: mock, IdleTimeout: 100 * time.Millisecond}
	result := e.Run(sessionCtx, "test prompt")

	require.NoError(t, result.Error, "idle timeout should not produce an error")
	assert.Equal(t, "hello", result.Output)
	require.NoError(t, sessionCtx.Err(), "session timeout context should still be alive")
	assert.True(t, result.IdleTimedOut, "IdleTimedOut should be set when idle timeout fires")
}

func TestGeminiExecutor_Run_IdleTimeoutDetectsLimitPattern(t *testing.T) {
	// when idle timeout fires after a rate-limit message, the limit pattern should be detected
	// instead of silently returning success. this ensures runWithLimitRetry can wait-and-retry.
	pr, pw := io.Pipe()

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(ctx context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			go func() {
				defer pw.Close()
				// print rate limit message then go silent
				fmt.Fprintln(pw, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit"}}`)
				<-ctx.Done()
			}()
			return pr, func() error {
				<-ctx.Done()
				return errors.New("signal: killed")
			}, nil
		},
	}

	e := &GeminiExecutor{
		cmdRunner:     mock,
		IdleTimeout:   100 * time.Millisecond,
		LimitPatterns: []string{"You've hit your limit"},
	}
	result := e.Run(context.Background(), "test prompt")

	var limitErr *LimitPatternError
	require.ErrorAs(t, result.Error, &limitErr, "should return LimitPatternError")
	assert.Equal(t, "You've hit your limit", limitErr.Pattern)
	assert.False(t, result.IdleTimedOut, "IdleTimedOut should not be set when pattern matched")
}

func TestGeminiExecutor_Run_IdleTimeoutDetectsErrorPattern(t *testing.T) {
	// when idle timeout fires after an error pattern message, the error pattern should be detected
	pr, pw := io.Pipe()

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(ctx context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			go func() {
				defer pw.Close()
				fmt.Fprintln(pw, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"API Error: something broke"}}`)
				<-ctx.Done()
			}()
			return pr, func() error {
				<-ctx.Done()
				return errors.New("signal: killed")
			}, nil
		},
	}

	e := &GeminiExecutor{
		cmdRunner:     mock,
		IdleTimeout:   100 * time.Millisecond,
		ErrorPatterns: []string{"API Error:"},
	}
	result := e.Run(context.Background(), "test prompt")

	var patternErr *PatternMatchError
	require.ErrorAs(t, result.Error, &patternErr, "should return PatternMatchError")
	assert.Equal(t, "API Error:", patternErr.Pattern)
	assert.False(t, result.IdleTimedOut, "IdleTimedOut should not be set when pattern matched")
}

func TestGeminiExecutor_Run_IdleTimeoutNotFiredResult(t *testing.T) {
	// verify IdleTimedOut is false on normal (non-idle-timeout) completion with idle timeout configured
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"done"}}`),
				func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock, IdleTimeout: 5 * time.Second}
	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)
	assert.Equal(t, "done", result.Output)
	assert.False(t, result.IdleTimedOut, "IdleTimedOut should be false on normal completion")
}

// test flags are registered so the test binary accepts --prompt "" and --yolo without erroring.
// GeminiExecutor.Run() always appends these to the command args; when the test
// binary is used as the subprocess command, these flags must be registered.
var _ = flag.String("prompt", "", "consumed by subprocess tests")
var _ = flag.Bool("yolo", false, "consumed by subprocess tests")
var _ = flag.String("output-format", "", "consumed by subprocess tests")

// TestHelperProcess is not a real test — it is used as a subprocess by TestExecGeminiRunner_StdinSet.
// It reads all of stdin and writes it to stdout, then exits.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	fmt.Print(string(data))
	os.Exit(0)
}

// TestHelperProcessStreamJSON is not a real test — used as a subprocess by
// TestGeminiExecutor_Run_RealRunner_StdinWired. Reads stdin and emits it as a
// stream-json content_block_delta event so GeminiExecutor.parseStream can parse it.
func TestHelperProcessStreamJSON(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_JSON") != "1" {
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	fmt.Printf(`{"type":"content_block_delta","delta":{"type":"text_delta","text":%q}}`, string(data))
	fmt.Println()
	fmt.Println(`{"type":"result","result":""}`)
	os.Exit(0)
}

func TestGeminiExecutor_Run_RealRunner_StdinWired(t *testing.T) {
	// verify the full wiring: GeminiExecutor.Run() with cmdRunner == nil constructs
	// execGeminiRunner{stdin: stdinReader} and the subprocess receives the prompt via stdin.
	// if the wiring is broken (e.g. execGeminiRunner{} without stdin), the subprocess reads
	// empty stdin and result.Output would be empty.
	t.Setenv("GO_WANT_HELPER_PROCESS_JSON", "1")
	exe, err := os.Executable()
	require.NoError(t, err)

	e := &GeminiExecutor{
		Command: exe,
		Args:    "-test.run=TestHelperProcessStreamJSON",
		// cmdRunner is nil — exercises the real execGeminiRunner construction path
	}

	result := e.Run(context.Background(), "hello stdin wiring")
	require.NoError(t, result.Error)
	assert.Contains(t, result.Output, "hello stdin wiring")
}

func TestExecGeminiRunner_StdinSet(t *testing.T) {
	// verify that when execGeminiRunner.stdin is set, it is piped to the child process's stdin.
	// uses the test binary re-invocation pattern: the subprocess runs TestHelperProcess which
	// echoes stdin to stdout, letting us confirm the pipe is connected.
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	exe, err := os.Executable()
	require.NoError(t, err)

	input := "hello from stdin"
	r := &execGeminiRunner{stdin: strings.NewReader(input)}

	output, wait, err := r.Run(context.Background(), exe, "-test.run=TestHelperProcess")
	require.NoError(t, err)

	data, err := io.ReadAll(output)
	require.NoError(t, err)
	require.NoError(t, wait())
	assert.Equal(t, input, string(data))
}

func TestGeminiExecutor_Run_NoPromptInArgs(t *testing.T) {
	// verify that args never include -p: prompt is always passed via stdin, not CLI arg.
	// also verify --prompt "" is present for non-interactive mode in both default and custom-args paths.
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`

	tests := []struct {
		name string
		args string
	}{
		{name: "default args", args: ""},
		{name: "custom args", args: "--yolo --output-format stream-json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedArgs []string
			e := &GeminiExecutor{
				Args: tc.args,
				cmdRunner: &mocks.CommandRunnerMock{
					RunFunc: func(_ context.Context, _ string, args ...string) (io.Reader, func() error, error) {
						capturedArgs = args
						return strings.NewReader(jsonStream), func() error { return nil }, nil
					},
				},
			}

			result := e.Run(context.Background(), "test prompt")

			require.NoError(t, result.Error)
			assert.NotContains(t, capturedArgs, "-p")
			assert.NotContains(t, capturedArgs, "test prompt")
			assert.Contains(t, capturedArgs, "--prompt", "non-interactive flag must be present")
		})
	}
}

func TestGeminiExecutor_Run_LimitPattern(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		limitPat    []string
		errorPat    []string
		wantLimit   bool
		wantError   bool
		wantPattern string
	}{
		{
			name:      "no limit patterns",
			output:    `{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit"}}`,
			limitPat:  nil,
			errorPat:  []string{"hit your limit"},
			wantLimit: false, wantError: true, wantPattern: "hit your limit",
		},
		{
			name:      "limit pattern matched",
			output:    `{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit"}}`,
			limitPat:  []string{"hit your limit"},
			errorPat:  nil,
			wantLimit: true, wantError: false, wantPattern: "hit your limit",
		},
		{
			name:      "limit takes precedence over error when both match",
			output:    `{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit"}}`,
			limitPat:  []string{"hit your limit"},
			errorPat:  []string{"hit your limit"},
			wantLimit: true, wantError: false, wantPattern: "hit your limit",
		},
		{
			name:      "error pattern when limit does not match",
			output:    `{"type":"content_block_delta","delta":{"type":"text_delta","text":"API Error: 500 internal"}}`,
			limitPat:  []string{"hit your limit"},
			errorPat:  []string{"API Error:"},
			wantLimit: false, wantError: true, wantPattern: "API Error:",
		},
		{
			name:      "no match at all",
			output:    `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Task completed"}}`,
			limitPat:  []string{"hit your limit"},
			errorPat:  []string{"API Error:"},
			wantLimit: false, wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mocks.CommandRunnerMock{
				RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
					return strings.NewReader(tc.output), func() error { return nil }, nil
				},
			}
			e := &GeminiExecutor{
				cmdRunner:     mock,
				LimitPatterns: tc.limitPat,
				ErrorPatterns: tc.errorPat,
			}

			result := e.Run(context.Background(), "test prompt")

			switch {
			case tc.wantLimit:
				require.Error(t, result.Error)
				var limitErr *LimitPatternError
				require.ErrorAs(t, result.Error, &limitErr)
				assert.Equal(t, tc.wantPattern, limitErr.Pattern)
				assert.Equal(t, "gemini /help", limitErr.HelpCmd)
			case tc.wantError:
				require.Error(t, result.Error)
				var patternErr *PatternMatchError
				require.ErrorAs(t, result.Error, &patternErr)
				assert.Equal(t, tc.wantPattern, patternErr.Pattern)
			default:
				require.NoError(t, result.Error)
			}
		})
	}
}

func TestGeminiExecutor_Run_PatternFalsePositive_InAnalysisText(t *testing.T) {
	// pattern appears in early output (analysis text) but is followed by many blocks of real work.
	// should NOT trigger pattern match because the pattern falls outside the recent blocks window.
	lines := make([]string, 0, recentBlockCount+2)
	// block 1: analysis text containing the pattern
	lines = append(lines, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"the error message says You've hit your limit when rate limited"}}`)
	// blocks 2-5: normal work output (pushes pattern out of the recentBlockCount window)
	for i := range recentBlockCount + 1 {
		lines = append(lines, fmt.Sprintf(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"normal work output block %d"}}`, i))
	}
	jsonStream := strings.Join(lines, "\n")

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock, LimitPatterns: []string{"You've hit your limit"}, ErrorPatterns: []string{"hit your limit"}}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error, "should not detect pattern in old analysis text")
	assert.Contains(t, result.Output, "You've hit your limit", "full output still has the text")
	assert.NotContains(t, result.RecentText, "You've hit your limit", "recent blocks should not contain old text")
}

func TestGeminiExecutor_Run_PatternInRecentBlock(t *testing.T) {
	// pattern in the last block (real rate limit) — should be detected
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"some work done"}}
{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit · resets 5pm"}}`

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock, LimitPatterns: []string{"You've hit your limit"}}

	result := e.Run(context.Background(), "test prompt")

	require.Error(t, result.Error)
	var limitErr *LimitPatternError
	require.ErrorAs(t, result.Error, &limitErr)
	assert.Equal(t, "You've hit your limit", limitErr.Pattern)
}

func TestGeminiExecutor_Run_PatternInSecondToLastBlock(t *testing.T) {
	// pattern in second-to-last block, one more short block after (e.g., reset info) — still in window
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"You've hit your limit"}}
{"type":"content_block_delta","delta":{"type":"text_delta","text":"resets at 5pm (Europe/Vilnius)"}}`

	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, _ ...string) (io.Reader, func() error, error) {
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}
	e := &GeminiExecutor{cmdRunner: mock, LimitPatterns: []string{"You've hit your limit"}}

	result := e.Run(context.Background(), "test prompt")

	require.Error(t, result.Error)
	var limitErr *LimitPatternError
	require.ErrorAs(t, result.Error, &limitErr)
	assert.Equal(t, "You've hit your limit", limitErr.Pattern)
}

func TestGeminiExecutor_Run_ModelFlag(t *testing.T) {
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`

	var capturedArgs []string
	mock := &mocks.CommandRunnerMock{
		RunFunc: func(_ context.Context, _ string, args ...string) (io.Reader, func() error, error) {
			capturedArgs = args
			return strings.NewReader(jsonStream), func() error { return nil }, nil
		},
	}

	t.Run("model set injects --model flag", func(t *testing.T) {
		e := &GeminiExecutor{Model: "sonnet", cmdRunner: mock}
		result := e.Run(context.Background(), "test")
		require.NoError(t, result.Error)
		assert.Contains(t, capturedArgs, "--model")
		assert.Contains(t, capturedArgs, "sonnet")
	})

	t.Run("model empty does not inject --model flag", func(t *testing.T) {
		e := &GeminiExecutor{cmdRunner: mock}
		result := e.Run(context.Background(), "test")
		require.NoError(t, result.Error)
		assert.NotContains(t, capturedArgs, "--model")
	})

	t.Run("model overrides existing --model in args", func(t *testing.T) {
		e := &GeminiExecutor{Args: `--prompt "" --model opus --output-format json`, Model: "sonnet", cmdRunner: mock}
		result := e.Run(context.Background(), "test")
		require.NoError(t, result.Error)
		assert.Contains(t, capturedArgs, "sonnet")
		assert.NotContains(t, capturedArgs, "opus", "old --model value should be stripped")
		// count --model occurrences — should be exactly one
		count := 0
		for _, a := range capturedArgs {
			if a == "--model" {
				count++
			}
		}
		assert.Equal(t, 1, count, "should have exactly one --model flag")
	})
}

func TestGeminiExecutor_Run_EffortFlag(t *testing.T) {
	jsonStream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`

	// newMock returns a fresh mock whose RunFunc writes captured args into
	// the provided slot. using a per-subtest slot avoids cross-test leakage.
	newMock := func(slot *[]string) *mocks.CommandRunnerMock {
		return &mocks.CommandRunnerMock{
			RunFunc: func(_ context.Context, _ string, args ...string) (io.Reader, func() error, error) {
				*slot = args
				return strings.NewReader(jsonStream), func() error { return nil }, nil
			},
		}
	}

	countFlag := func(args []string, flag string) int {
		n := 0
		for _, a := range args {
			if a == flag {
				n++
			}
		}
		return n
	}

	t.Run("effort set injects --effort flag", func(t *testing.T) {
		var capturedArgs []string
		e := &GeminiExecutor{Effort: "high", cmdRunner: newMock(&capturedArgs)}
		result := e.Run(context.Background(), "test")
		require.NoError(t, result.Error)
		assert.Contains(t, capturedArgs, "--effort")
		assert.Contains(t, capturedArgs, "high")
	})

	t.Run("effort empty does not inject --effort flag", func(t *testing.T) {
		var capturedArgs []string
		e := &GeminiExecutor{cmdRunner: newMock(&capturedArgs)}
		result := e.Run(context.Background(), "test")
		require.NoError(t, result.Error)
		assert.NotContains(t, capturedArgs, "--effort")
	})

	t.Run("model and effort together inject both flags", func(t *testing.T) {
		var capturedArgs []string
		e := &GeminiExecutor{Model: "opus", Effort: "medium", cmdRunner: newMock(&capturedArgs)}
		result := e.Run(context.Background(), "test")
		require.NoError(t, result.Error)
		assert.Contains(t, capturedArgs, "--model")
		assert.Contains(t, capturedArgs, "opus")
		assert.Contains(t, capturedArgs, "--effort")
		assert.Contains(t, capturedArgs, "medium")
	})

	t.Run("effort overrides existing --effort in args", func(t *testing.T) {
		var capturedArgs []string
		e := &GeminiExecutor{Args: `--prompt "" --effort low --output-format json`, Effort: "high", cmdRunner: newMock(&capturedArgs)}
		result := e.Run(context.Background(), "test")
		require.NoError(t, result.Error)
		assert.Contains(t, capturedArgs, "high")
		assert.NotContains(t, capturedArgs, "low", "old --effort value should be stripped")
		assert.Equal(t, 1, countFlag(capturedArgs, "--effort"), "should have exactly one --effort flag")
	})

	t.Run("effort overrides equals form in args", func(t *testing.T) {
		var capturedArgs []string
		e := &GeminiExecutor{Args: `--prompt "" --effort=low --output-format json`, Effort: "high", cmdRunner: newMock(&capturedArgs)}
		result := e.Run(context.Background(), "test")
		require.NoError(t, result.Error)
		assert.Contains(t, capturedArgs, "high")
		assert.NotContains(t, capturedArgs, "--effort=low", "equals form should be stripped")
		assert.Equal(t, 1, countFlag(capturedArgs, "--effort"), "should have exactly one --effort flag")
	})
}
