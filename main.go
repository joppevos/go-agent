// main.go

package main

import (
	"agent/tools"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

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
				text := markdownToANSI(block.Text)
				fmt.Printf("\u001b[93mClaude\u001b[0m: %s\n", text)
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

// Function to convert Markdown **bold** to terminal ANSI bold
func markdownToANSI(text string) string {
	re := regexp.MustCompile(`\*\*(.*?)\*\*`)
	return re.ReplaceAllString(text, "\u001b[1m$1\u001b[0m")
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
	You are a Chinese (HSK 1) conversation partner.

	STARTUP:
	- start conversation by retrieving known vocabularyfrom anki: get_anki_notes({"deck_name":"ChineseAgent"})
	- Do not question if we want to start a conversation. We always do a conversation. We dive straight in


	EXAMPLE RESPONSE FORMAT (STRICT - ONLY OUTPUT THESE 3 LINES):
	Pinyin: **nǐ hǎo**
	Chinese: 你好
	English: Hello

	Use this response format for all your responses. Always these 3 lines. Above is an example response. Please follow up with a question in Chinese.

	CONVERSATION RULES:
	- ONE short sentence per turn
	- Use traditional Chinese characters
	- NEVER use English except in translation and corrections
	- Introduce maximum one new word per exchange
	- Vary topics (food, directions, introductions)
	- Be engaging and encouraging
	- If user makes a mistake, include a short correction tip in brackets at the end of the English line
	- Never output additional Chinese text lines outside the 3-line format

	FLASHCARDS:
	- When user asks about a word, add it to Anki: add_flashcard({"front":"pinyin + characters", "back":"English + usage notes"})
	- Don't announce when you add cards

	IMPORTANT:
	- Never explain what you're doing
	- No multi-sentence responses
	- Keep exchanges natural and conversational
	- State clearly when conversation ends

	[Optional: One brief grammar tip at end if helpful]
`

	//go ui.RunSpinner()

	message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_5SonnetLatest,
		MaxTokens: int64(1024),
		Messages:  conversation,
		Tools:     anthropicTools,
		System: []anthropic.TextBlockParam{
			{Text: StartText},
		},
	})
	return message, err
}
