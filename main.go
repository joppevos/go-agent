// main.go

package main

import (
	"bufio"
	"context"
	"log"
	"path/filepath"
	"strings"

	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/atselvan/ankiconnect"
	"github.com/invopop/jsonschema"
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

	tools := []ToolDefinition{ReadFileDefinition, ListFilesDefinition, AddFlashcardDefinition}
	agent := NewAgent(&client, getUserMessage, tools)
	err := agent.Run(context.TODO())
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}

func NewAgent(
	client *anthropic.Client,
	getUserMessage func() (string, bool),
	tools []ToolDefinition) *Agent {
	return &Agent{
		client:         client,
		getUserMessage: getUserMessage,
		tools:          tools,
	}
}

type Agent struct {
	client         *anthropic.Client
	getUserMessage func() (string, bool)
	tools          []ToolDefinition
}

type ToolDefinition struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	InputSchema anthropic.ToolInputSchemaParam `json:"input_schema"`
	Function    func(input json.RawMessage) (string, error)
}

func (a *Agent) Run(ctx context.Context) error {
	conversation := []anthropic.MessageParam{}

	fmt.Println("Chat with Claude (use 'ctrl-c' to quit)")

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
				fmt.Printf("\u001b[93mClaude\u001b[0m: %s\n", block.Text)
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
	var toolDef ToolDefinition
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
	message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_5HaikuLatest,
		MaxTokens: int64(1024),
		Messages:  conversation,
		Tools:     anthropicTools,
		System: []anthropic.TextBlockParam{
			{Text: "Talk like a wise chinese man"},
		},
	})
	return message, err
}

// Defintion of the read filea. Input schema is the values the tool accept, Function is the function that will be executed
var ReadFileDefinition = ToolDefinition{
	Name:        "read_file",
	Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
	InputSchema: ReadFileInputSchema,
	Function:    ReadFile,
}

type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

var ReadFileInputSchema = GenerateSchema[ReadFileInput]()

var ListFilesDefinition = ToolDefinition{
	Name:        "list_files",
	Description: "List the files in the current directory. Use it to know what's inside a directory",
	InputSchema: ListFilesInputSchema,
	Function:    ListFiles,
}

func ListFiles(input json.RawMessage) (string, error) {
	listFilesInput := ListFilesInput{}
	err := json.Unmarshal(input, &listFilesInput)
	if err != nil {
		panic(err)
	}
	var files []string
	filepath.WalkDir(listFilesInput.Directory, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	return strings.Join(files, "\n"), nil
}

type ListFilesInput struct {
	Directory string `json:"directory" jsonschema_description:"The directory path"`
}

var ListFilesInputSchema = GenerateSchema[ListFilesInput]()

func ReadFile(input json.RawMessage) (string, error) {
	readFileInput := ReadFileInput{}
	err := json.Unmarshal(input, &readFileInput)
	if err != nil {
		panic(err)
	}

	content, err := os.ReadFile(readFileInput.Path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func GenerateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T

	schema := reflector.Reflect(v)

	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

type Anki struct {
	client *ankiconnect.Client
}

func NewAnki() *Anki {
	return &Anki{
		client: ankiconnect.NewClient(),
	}
}

var AddFlashcardDefinition = ToolDefinition{
	Name:        "add_flashcard",
	Description: "Add a flashcard to Anki. Front and back",
	InputSchema: AddFlashcardInputSchema,
	Function:    AddFlashcard,
}

func AddFlashcard(input json.RawMessage) (string, error) {
	addFlashcardInput := AddFlashcardInput{}
	err := json.Unmarshal(input, &addFlashcardInput)
	if err != nil {
		panic(err)
	}
	anki := NewAnki()
	restErr := anki.client.Ping()
	if restErr != nil {
		fmt.Println("unable to reach anki. make sure it's running")
		log.Fatal(restErr)
	}

	note := ankiconnect.Note{
		DeckName:  "New Deck",
		ModelName: "Basic",
		Fields: ankiconnect.Fields{
			"Front": addFlashcardInput.Front,
			"Back":  addFlashcardInput.Back,
		},
	}
	restErr = anki.client.Notes.Add(note)
	if restErr != nil {
		fmt.Println("unable to add note to anki. make sure it's running")
		log.Fatal(restErr)
	}
	return "Flashcard added successfully", nil
}

type AddFlashcardInput struct {
	Front string `json:"front" jsonschema_description:"The front of the flashcard"`
	Back  string `json:"back" jsonschema_description:"The back of the flashcard"`
}

var AddFlashcardInputSchema = GenerateSchema[AddFlashcardInput]()
