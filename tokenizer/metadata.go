package tokenizer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

/*
Metadata contains the small Hugging Face tokenizer-side contract used during
inference setup.
*/
type Metadata struct {
	ChatTemplate   string
	BOSToken       string
	EOSToken       string
	EOTToken       string
	ModelMaxLength int
	PadTokenID     int
}

/*
LoadMetadata resolves tokenizer_config.json and special_tokens_map.json for a
tokenizer source.
*/
func LoadMetadata(ctx context.Context, source Source) (*Metadata, error) {
	tokenizerConfig, err := loadTokenizerConfig(ctx, source)

	if err != nil {
		return nil, err
	}

	specialTokens, err := loadSpecialTokens(ctx, source)

	if err != nil {
		return nil, err
	}

	metadata := &Metadata{
		ChatTemplate: tokenizerConfig.ChatTemplate,
		BOSToken:     firstToken(tokenizerConfig.BOSToken, specialTokens.BOSToken),
		EOSToken:     firstToken(tokenizerConfig.EOSToken, specialTokens.EOSToken),
		EOTToken:     firstToken(tokenizerConfig.EOTToken, specialTokens.EOTToken),
	}

	if metadata.EOTToken == "" {
		metadata.EOTToken = metadata.EOSToken
	}

	metadata.ModelMaxLength = tokenizerConfig.ModelMaxLength
	metadata.PadTokenID = padTokenIDFromConfig(tokenizerConfig)

	return metadata, nil
}

/*
ApplyChatTemplate renders a single user message for an instruct model.
*/
func (metadata *Metadata) ApplyChatTemplate(text string) (string, error) {
	if metadata == nil || metadata.ChatTemplate == "" {
		return text, nil
	}

	return metadata.RenderChat(
		[]ChatMessage{{Role: "user", Content: text}},
		true,
	)
}

/*
ApplyChatContinuation renders a user turn to append to an existing chat token
stream.
*/
func (metadata *Metadata) ApplyChatContinuation(text string) (string, error) {
	if metadata == nil || metadata.ChatTemplate == "" {
		return text, nil
	}

	rendered, err := metadata.renderChat(
		[]ChatMessage{{Role: "user", Content: text}},
		true,
		false,
	)

	if err != nil {
		return "", err
	}

	return metadata.EOTToken + strings.TrimPrefix(rendered, metadata.BOSToken), nil
}

/*
RenderChat renders the common Hugging Face Llama-3 style chat template.
*/
func (metadata *Metadata) RenderChat(
	messages []ChatMessage,
	addGenerationPrompt bool,
) (string, error) {
	return metadata.renderChat(messages, addGenerationPrompt, true)
}

func (metadata *Metadata) renderChat(
	messages []ChatMessage,
	addGenerationPrompt bool,
	includeSystem bool,
) (string, error) {
	if metadata == nil || metadata.ChatTemplate == "" {
		return concatenateMessages(messages), nil
	}

	if !strings.Contains(metadata.ChatTemplate, "<|start_header_id|>") {
		return "", fmt.Errorf("tokenizer metadata: unsupported chat template")
	}

	var builder strings.Builder
	builder.WriteString(metadata.BOSToken)

	if includeSystem {
		builder.WriteString("<|start_header_id|>system<|end_header_id|>\n\n")
		builder.WriteString("Cutting Knowledge Date: December 2023\n")
		builder.WriteString("Today Date: 26 Jul 2024\n\n")
		builder.WriteString(metadata.EOTToken)
	}

	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.Content)

		if role == "" {
			return "", fmt.Errorf("tokenizer metadata: chat message role is required")
		}

		builder.WriteString("<|start_header_id|>")
		builder.WriteString(role)
		builder.WriteString("<|end_header_id|>\n\n")
		builder.WriteString(content)
		builder.WriteString(metadata.EOTToken)
	}

	if addGenerationPrompt {
		builder.WriteString("<|start_header_id|>assistant<|end_header_id|>\n\n")
	}

	return builder.String(), nil
}

/*
ChatMessage is one role/content entry in a chat prompt.
*/
type ChatMessage struct {
	Role    string
	Content string
}

type tokenizerConfigDocument struct {
	ChatTemplate   string                    `json:"chat_template"`
	BOSToken       tokenText                 `json:"bos_token"`
	EOSToken       tokenText                 `json:"eos_token"`
	EOTToken       tokenText                 `json:"eot_token"`
	PadToken       tokenText                 `json:"pad_token"`
	ModelMaxLength int                       `json:"model_max_length"`
	AddedTokens    map[string]addedTokenSpec `json:"added_tokens_decoder"`
}

type addedTokenSpec struct {
	Content string `json:"content"`
}

type specialTokensDocument struct {
	BOSToken tokenText `json:"bos_token"`
	EOSToken tokenText `json:"eos_token"`
	EOTToken tokenText `json:"eot_token"`
}

type tokenText string

func (token *tokenText) UnmarshalJSON(data []byte) error {
	var rawString string

	if err := json.Unmarshal(data, &rawString); err == nil {
		*token = tokenText(rawString)
		return nil
	}

	var rawObject struct {
		Content string `json:"content"`
	}

	if err := json.Unmarshal(data, &rawObject); err != nil {
		return err
	}

	*token = tokenText(rawObject.Content)
	return nil
}

func loadTokenizerConfig(ctx context.Context, source Source) (tokenizerConfigDocument, error) {
	path, err := metadataSource(source, "tokenizer_config.json").Resolve(ctx)

	if err != nil {
		return tokenizerConfigDocument{}, err
	}

	data, err := os.ReadFile(path)

	if err != nil {
		return tokenizerConfigDocument{}, err
	}

	var document tokenizerConfigDocument

	if err := json.Unmarshal(data, &document); err != nil {
		return tokenizerConfigDocument{}, err
	}

	return document, nil
}

func loadSpecialTokens(ctx context.Context, source Source) (specialTokensDocument, error) {
	path, err := metadataSource(source, "special_tokens_map.json").Resolve(ctx)

	if err != nil {
		return specialTokensDocument{}, nil
	}

	data, err := os.ReadFile(path)

	if err != nil {
		return specialTokensDocument{}, nil
	}

	var document specialTokensDocument

	if err := json.Unmarshal(data, &document); err != nil {
		return specialTokensDocument{}, err
	}

	return document, nil
}

func metadataSource(source Source, file string) Source {
	directory := filepath.Dir(source.File)

	if directory == "." {
		source.File = file
		return source
	}

	source.File = filepath.Join(directory, file)
	return source
}

func firstToken(tokens ...tokenText) string {
	for _, token := range tokens {
		text := string(token)

		if text != "" {
			return text
		}
	}

	return ""
}

func concatenateMessages(messages []ChatMessage) string {
	var builder strings.Builder

	for _, message := range messages {
		builder.WriteString(message.Content)
	}

	return builder.String()
}

func padTokenIDFromConfig(tokenizerConfig tokenizerConfigDocument) int {
	padToken := string(tokenizerConfig.PadToken)

	if padToken == "" {
		return 0
	}

	for rawID, entry := range tokenizerConfig.AddedTokens {
		if entry.Content != padToken {
			continue
		}

		tokenID := 0

		for _, character := range rawID {
			if character < '0' || character > '9' {
				return 0
			}

			tokenID = tokenID*10 + int(character-'0')
		}

		return tokenID
	}

	return 0
}
