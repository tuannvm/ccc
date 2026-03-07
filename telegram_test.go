package main

import (
	"testing"
)

func TestMarkdownToTelegramV2_Bold(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple bold",
			input:    "**Bold text**",
			expected: "*Bold text*",
		},
		{
			name:     "bold with spaces",
			input:    "**Bold with spaces**",
			expected: "*Bold with spaces*",
		},
		{
			name:     "bold in middle of text",
			input:    "This is **bold** text",
			expected: `This is *bold* text`,
		},
		{
			name:     "multiple bold sections",
			input:    "**Bold1** and **Bold2**",
			expected: "*Bold1* and *Bold2*",
		},
		{
			name:     "bold with special chars",
			input:    "**file_name.txt**",
			expected: "*file\\_name\\.txt*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_Italic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple italic",
			input:    "*Italic text*",
			expected: "_Italic text_",
		},
		{
			name:     "italic in middle",
			input:    "This is *italic* text",
			expected: `This is _italic_ text`,
		},
		{
			name:     "italic with special chars",
			input:    "*file_name.txt*",
			expected: "_file\\_name\\.txt_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_Code(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "inline code",
			input:    "`code`",
			expected: "`code`",
		},
		{
			name:     "code with special chars",
			input:    "`file_name.txt`",
			expected: "`file_name.txt`",
		},
		{
			name:     "code block",
			input:    "```\ncode here\n```",
			expected: "```\ncode here\n```",
		},
		{
			name:     "code in text",
			input:    "Use `npm install` to install",
			expected: `Use ` + "`npm install`" + ` to install`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_CodeWithBackslashes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "inline code with backslash",
			input:    "`C:\\Users`",
			expected: "`C:\\\\Users`",
		},
		{
			name:     "code block with backslashes",
			input:    "```\nC:\\Users\\tmp\n```",
			expected: "```\nC:\\\\Users\\\\tmp\n```",
		},
		{
			name:     "code block with backtick inside",
			input:    "```\nconst s = `hello`\n```",
			expected: "```\nconst s = \\`hello\\`\n```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_Links(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple link",
			input:    "[Link](https://example.com)",
			expected: "[Link](https://example.com)",
		},
		{
			name:     "link with special chars in text",
			input:    "[Click_here](url)",
			expected: "[Click\\_here](url)",
		},
		{
			name:     "link in text",
			input:    "Visit [example](https://example.com) for more",
			expected: `Visit [example](https://example.com) for more`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_Headings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "h2 heading",
			input:    "## Heading",
			expected: "*Heading*",
		},
		{
			name:     "h3 heading",
			input:    "### Heading",
			expected: "*Heading*",
		},
		{
			name:     "heading with text",
			input:    "## Heading\n\nSome text",
			expected: "*Heading*\n\nSome text",
		},
		{
			name:     "heading with special chars",
			input:    "## Heading_with_underscore",
			expected: "*Heading\\_with\\_underscore*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_NoHeadingForHashtags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		notWant  string // Should NOT contain this
	}{
		{
			name:    "hashtag at line start",
			input:   "#hashtag",
			notWant: "*hashtag*", // Should not be converted to bold
		},
		{
			name:    "identifier at line start",
			input:   "#123",
			notWant: "*123*", // Should not be converted to bold
		},
		{
			name:    "valid heading",
			input:   "# Heading",
			notWant: "# Heading", // Should be converted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if tt.name == "valid heading" {
				// Valid heading SHOULD be converted to bold
				if !contains(result, "*Heading*") {
					t.Errorf("Valid heading should be converted, got: %s", result)
				}
			} else {
				// Hashtags and identifiers should NOT be converted
				if contains(result, tt.notWant) {
					t.Errorf("%s should not be treated as heading, got: %s", tt.name, result)
				}
			}
		})
	}
}

func TestMarkdownToTelegramV2_Lists(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "unordered list",
			input:    "- Item",
			expected: "\\- Item",
		},
		{
			name:     "ordered list",
			input:    "1. Item",
			expected: "1\\. Item",
		},
		{
			name:     "multiple items",
			input:    "- Item 1\n- Item 2",
			expected: "\\- Item 1\n\\- Item 2",
		},
		{
			name:     "ordered with special chars",
			input:    "1. Item_with_underscore",
			expected: "1\\. Item\\_with\\_underscore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_Blockquotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple blockquote",
			input:    "> Quote text",
			expected: "Quote text\n",
		},
		{
			name:     "blockquote with special chars",
			input:    "> file_name.txt",
			expected: "file\\_name\\.txt\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_MixedFormatting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bold and italic",
			input:    "**Bold** and *italic*",
			expected: "*Bold* and _italic_",
		},
		{
			name:     "bold with code",
			input:    "**Bold** with `code`",
			expected: "*Bold* with `code`",
		},
		{
			name:     "complex formatting",
			input:    "**Bold** text with *italic* and `code`",
			expected: "*Bold* text with _italic_ and `code`",
		},
		{
			name:     "link with bold",
			input:    "[**Bold Link**](url)",
			expected: "[*Bold Link*](url)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "underscores",
			input:    "file_name.txt",
			expected: "file\\_name\\.txt",
		},
		{
			name:     "exclamation",
			input:    "Error!",
			expected: "Error\\!",
		},
		{
			name:     "dots",
			input:    "1. Item",
			expected: "1\\. Item",
		},
		{
			name:     "minus",
			input:    "a-b",
			expected: "a\\-b",
		},
		{
			name:     "special chars in text",
			input:    "Error: file_not_found.txt! Check /var/log",
			expected: "Error: file\\_not\\_found\\.txt\\! Check /var/log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_RealWorld(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "soloclaw example",
			input:    "**Bold text** and *italic text*",
			expected: "*Bold text* and _italic text_",
		},
		{
			name:     "code block example",
			input:    "Run `npm install` then `npm test`",
			expected: "Run `npm install` then `npm test`",
		},
		{
			name:     "heading example",
			input:    "## Heading Level 2\n\nSome text",
			expected: "*Heading Level 2*\n\nSome text",
		},
		{
			name:     "list example",
			input:    "- List item 1\n- List item 2",
			expected: "\\- List item 1\n\\- List item 2",
		},
		{
			name:     "mixed example",
			input:    "**Bold** and `code` and [link](url)",
			expected: "*Bold* and `code` and [link](url)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text",
			input:    "Just plain text",
			expected: "Just plain text",
		},
		{
			name:     "newlines",
			input:    "Line 1\nLine 2",
			expected: "Line 1\nLine 2",
		},
		{
			name:     "unclosed bold",
			input:    "**bold",
			expected: "\\*\\*bold",
		},
		{
			name:     "unclosed code",
			input:    "`code",
			expected: "\\`code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_HTMLTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple div tag",
			input:    "<div>content</div>",
			expected: "\\<div\\>content\\</div\\>",
		},
		{
			name:     "details tag",
			input:    "<details>Click to expand</details>",
			expected: "\\<details\\>Click to expand\\</details\\>",
		},
		{
			name:     "self-closing br tag",
			input:    "Line 1<br>Line 2",
			expected: "Line 1\\<br\\>Line 2",
		},
		{
			name:     "multiple tags",
			input:    "<p>Paragraph</p> and <div>Div</div>",
			expected: "\\<p\\>Paragraph\\</p\\> and \\<div\\>Div\\</div\\>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if result != tt.expected {
				t.Errorf("MarkdownToTelegramV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToTelegramV2_ComparisonOperators(t *testing.T) {
	// Test that comparison operators are NOT treated as HTML tags
	// Note: < is NOT a special char in MarkdownV2, but > IS
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			name:  "less than comparison",
			input: "if (a < b)",
			check: func(result string) bool {
				// < is NOT a special char, so should NOT be escaped
				// but ( should be escaped
				return result == "if \\(a < b\\)"
			},
		},
		{
			name:  "greater than comparison",
			input: "if (a > b)",
			check: func(result string) bool {
				// > IS a special character and should be escaped
				return result == "if \\(a \\> b\\)"
			},
		},
		{
			name:  "both comparisons",
			input:  "1 < 2 > 1",
			check: func(result string) bool {
				// < should NOT be escaped, > should be escaped
				return result == "1 < 2 \\> 1"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MarkdownToTelegramV2(tt.input)
			if !tt.check(result) {
				t.Errorf("Check failed for input %q, got: %s", tt.input, result)
			}
		})
	}
}
