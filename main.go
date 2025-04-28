// main.go

package main

import (
	"agent/tools"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

func main() {
	client := anthropic.NewClient()

	scanner := bufio.NewScanner(os.Stdin)
	getUserMessage := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	tools := []tools.ToolDefinition{
		tools.ReadFileDefinition,
		tools.ListFilesDefinition,
		tools.AddFlashcardDefinition,
		tools.GetNotesDefinition,
	}
	agent := NewAgent(&client, getUserMessage, tools)
	err := agent.Run(context.TODO())
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}

func NewAgent(
	client *anthropic.Client,
	getUserMessage func() (string, bool),
	tools []tools.ToolDefinition) *Agent {
	return &Agent{
		client:         client,
		getUserMessage: getUserMessage,
		tools:          tools,
	}
}

type Agent struct {
	client         *anthropic.Client
	getUserMessage func() (string, bool)
	tools          []tools.ToolDefinition
}

func (a *Agent) Run(ctx context.Context) error {
	conversation := []anthropic.MessageParam{}

	//fmt.Println("(use 'ctrl-c' to quit)")

	readUserInput := true
	for {
		if readUserInput {
			fmt.Print("\u001b[94mYou\u001b[0m: ")
			userInput, ok := a.getUserMessage()
			if !ok {
				break
			}
			userMessage := anthropic.NewUserMessage(anthropic.NewTextBlock(userInput))
			conversation = append(conversation, userMessage)
		}

		message, err := a.runInference(ctx, conversation)

		if err != nil {
			return err
		}
		conversation = append(conversation, message.ToParam())
		toolResults := []anthropic.ContentBlockParamUnion{}
		for _, block := range message.Content {
			switch block := block.AsAny().(type) {
			case anthropic.TextBlock:
				// output pinyin part in bold
				text := block.Text
				openParenIndex := strings.Index(block.Text, "(")
				if openParenIndex > 0 {
					pinyin := text[:openParenIndex]
					rest := text[openParenIndex:]
					fmt.Printf("\u001b[93mClaude\u001b[0m: \033[1m%s\033[0m%s\n", pinyin, rest)
				} else {
					fmt.Printf("\u001b[93mClaude\u001b[0m: %s\n", block.Text)
				}
			case anthropic.ToolUseBlock:
				result := a.executeTool(block.ID, block.Name, block.Input)
				toolResults = append(toolResults, result)
			}
		}
		// no more tools
		if len(toolResults) == 0 {
			readUserInput = true
			continue
		}
		readUserInput = false
		conversation = append(conversation, anthropic.NewUserMessage(toolResults...))
	}

	return nil
}

// Find the right tool. Claude asks for tool, we map it to our function to run it
func (a *Agent) executeTool(id, name string, input json.RawMessage) anthropic.ContentBlockParamUnion {
	var toolDef tools.ToolDefinition
	var found bool
	for _, tool := range a.tools {
		if tool.Name == name {
			toolDef = tool
			found = true
			break
		}
	}
	if !found {
		// return error if we don't have the tool
		return anthropic.NewToolResultBlock(id, "tool not found", true)
	}
	fmt.Printf("\u001b[92mtool\u001b[0m: %s(%s)\n", name, input)
	response, err := toolDef.Function(input)
	if err != nil {
		return anthropic.NewToolResultBlock(id, err.Error(), true)
	}
	return anthropic.NewToolResultBlock(id, response, false)
}

func (a *Agent) runInference(ctx context.Context, conversation []anthropic.MessageParam) (*anthropic.Message, error) {
	anthropicTools := []anthropic.ToolUnionParam{}
	for _, tool := range a.tools {
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropic.String(tool.Description),
				InputSchema: tool.InputSchema,
			},
		})
	}

	StartText := `
	You are a chatbot designed to help me learn Chinese at the HSK 1 level.

	We will have short and practical conversations. I will write in pinyin. You will respond in this format:
	You are a chatbot designed to help me learn Chinese at the HSK 1 level.

	When starting a new conversation:

		First, retrieve all the vocabulary notes from Anki using:
		tool: get_notes({"deck_name":"ChineseAgent"})

		Then immediately begin a short, practical conversation in Chinese.
		Do not ask whether I want to practice. Always start the conversation directly.

	Response format:
		Reply on a single line: pinyin (Traditional Chinese) — English translation.
		Example: nǐ hǎo (你好) — hello
		Never split across multiple lines.

	Rules:

	During the conversation, do not use English except to translate words.

	Do not ask new questions in English. Ask and answer questions in pinyin + Chinese characters only.

	Take the initiative to start each conversation. Vary the topic (e.g., buying bubble tea, asking for directions, introducing yourself).

	Keep your answers short and practical.

	Try to introduce one new word per conversation. When you introduce a new word, you may briefly explain it in English.

	If the practice conversation is finished, tell me that it’s over.

	New words should be added to Anki to help me review later. The front should contain pinyin and tradition Chinese, the back english. Add extra information to the back to help learning.

	You may add a minimal grammar tip at the end if needed, in brackets (English). Keep it extremely short.

	Remember: stay in pinyin + Traditional Chinese during conversation; English is for translation and minimal notes only.
	
	You are funny to talk with and an interesting teacher that keeps the user engaged
`

	//go ui.RunSpinner()

	message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_5HaikuLatest,
		MaxTokens: int64(1024),
		Messages:  conversation,
		Tools:     anthropicTools,
		System: []anthropic.TextBlockParam{
			{Text: StartText},
		},
	})
	return message, err
}
