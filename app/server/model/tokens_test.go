package model

import (
	"plandex-server/types"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestGetMessagesTokenEstimate(t *testing.T) {
	tests := []struct {
		name     string
		messages []types.ExtendedChatMessage
		minTokens int
	}{
		{
			name:      "empty messages",
			messages:  []types.ExtendedChatMessage{},
			minTokens: 0,
		},
		{
			name: "single message with text",
			messages: []types.ExtendedChatMessage{
				{
					Role: openai.ChatMessageRoleUser,
					Content: []types.ExtendedChatMessagePart{
						{
							Type: openai.ChatMessagePartTypeText,
							Text: "Hello, world!",
						},
					},
				},
			},
			minTokens: TokensPerMessage + TokensPerName + TokensPerExtendedPart,
		},
		{
			name: "message with empty content",
			messages: []types.ExtendedChatMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: []types.ExtendedChatMessagePart{},
				},
			},
			minTokens: TokensPerMessage + TokensPerName,
		},
		{
			name: "multiple messages",
			messages: []types.ExtendedChatMessage{
				{
					Role: openai.ChatMessageRoleSystem,
					Content: []types.ExtendedChatMessagePart{
						{
							Type: openai.ChatMessagePartTypeText,
							Text: "You are a helpful assistant.",
						},
					},
				},
				{
					Role: openai.ChatMessageRoleUser,
					Content: []types.ExtendedChatMessagePart{
						{
							Type: openai.ChatMessagePartTypeText,
							Text: "What is 2 + 2?",
						},
					},
				},
				{
					Role: openai.ChatMessageRoleAssistant,
					Content: []types.ExtendedChatMessagePart{
						{
							Type: openai.ChatMessagePartTypeText,
							Text: "2 + 2 equals 4.",
						},
					},
				},
			},
			minTokens: 3 * (TokensPerMessage + TokensPerName + TokensPerExtendedPart),
		},
		{
			name: "message with multiple parts",
			messages: []types.ExtendedChatMessage{
				{
					Role: openai.ChatMessageRoleUser,
					Content: []types.ExtendedChatMessagePart{
						{
							Type: openai.ChatMessagePartTypeText,
							Text: "First part",
						},
						{
							Type: openai.ChatMessagePartTypeText,
							Text: "Second part",
						},
					},
				},
			},
			minTokens: TokensPerMessage + TokensPerName + 2*TokensPerExtendedPart,
		},
		{
			name: "message with image part (should be skipped)",
			messages: []types.ExtendedChatMessage{
				{
					Role: openai.ChatMessageRoleUser,
					Content: []types.ExtendedChatMessagePart{
						{
							Type: openai.ChatMessagePartTypeText,
							Text: "Look at this image",
						},
						{
							Type: openai.ChatMessagePartTypeImageURL,
							ImageURL: &openai.ChatMessageImageURL{
								URL: "https://example.com/image.png",
							},
						},
					},
				},
			},
			minTokens: TokensPerMessage + TokensPerName + TokensPerExtendedPart,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMessagesTokenEstimate(tt.messages...)
			if result < tt.minTokens {
				t.Errorf("GetMessagesTokenEstimate() = %v, want at least %v", result, tt.minTokens)
			}
		})
	}
}

func TestTokenConstants(t *testing.T) {
	t.Run("TokensPerMessage is 4", func(t *testing.T) {
		if TokensPerMessage != 4 {
			t.Errorf("TokensPerMessage = %d, want 4", TokensPerMessage)
		}
	})

	t.Run("TokensPerName is 1", func(t *testing.T) {
		if TokensPerName != 1 {
			t.Errorf("TokensPerName = %d, want 1", TokensPerName)
		}
	})

	t.Run("TokensPerRequest is 3", func(t *testing.T) {
		if TokensPerRequest != 3 {
			t.Errorf("TokensPerRequest = %d, want 3", TokensPerRequest)
		}
	})

	t.Run("TokensPerExtendedPart is 6", func(t *testing.T) {
		if TokensPerExtendedPart != 6 {
			t.Errorf("TokensPerExtendedPart = %d, want 6", TokensPerExtendedPart)
		}
	})
}

func TestGetMessagesTokenEstimateGrowsWithContent(t *testing.T) {
	shortMessage := types.ExtendedChatMessage{
		Role: openai.ChatMessageRoleUser,
		Content: []types.ExtendedChatMessagePart{
			{
				Type: openai.ChatMessagePartTypeText,
				Text: "Hi",
			},
		},
	}

	longMessage := types.ExtendedChatMessage{
		Role: openai.ChatMessageRoleUser,
		Content: []types.ExtendedChatMessagePart{
			{
				Type: openai.ChatMessagePartTypeText,
				Text: "This is a much longer message that contains many more words and should result in a higher token count estimate because there are more characters and words to process in this text content.",
			},
		},
	}

	shortTokens := GetMessagesTokenEstimate(shortMessage)
	longTokens := GetMessagesTokenEstimate(longMessage)

	if longTokens <= shortTokens {
		t.Errorf("longer message (%d tokens) should have more tokens than shorter message (%d tokens)", longTokens, shortTokens)
	}
}

func TestGetMessagesTokenEstimateMultipleMessagesAddUp(t *testing.T) {
	msg1 := types.ExtendedChatMessage{
		Role: openai.ChatMessageRoleUser,
		Content: []types.ExtendedChatMessagePart{
			{
				Type: openai.ChatMessagePartTypeText,
				Text: "First message",
			},
		},
	}

	msg2 := types.ExtendedChatMessage{
		Role: openai.ChatMessageRoleAssistant,
		Content: []types.ExtendedChatMessagePart{
			{
				Type: openai.ChatMessagePartTypeText,
				Text: "Second message",
			},
		},
	}

	singleTokens := GetMessagesTokenEstimate(msg1)
	bothTokens := GetMessagesTokenEstimate(msg1, msg2)

	if bothTokens <= singleTokens {
		t.Errorf("two messages (%d tokens) should have more tokens than one message (%d tokens)", bothTokens, singleTokens)
	}
}
