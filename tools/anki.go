package tools

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/atselvan/ankiconnect"
)

var DeckName = "ChineseAgent"

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

	note := ankiconnect.Note{
		DeckName:  DeckName,
		ModelName: "Basic",
		Fields: ankiconnect.Fields{
			"Front": addFlashcardInput.Front,
			"Back":  addFlashcardInput.Back,
		},
	}
	restErr := anki.client.Notes.Add(note)
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

func GetNotes(input json.RawMessage) (string, error) {
	anki := NewAnki()

	// Get the Note Ids of cards due today
	nodeIds, restErr := anki.client.Cards.Get("deck:" + DeckName)

	if restErr != nil {
		log.Fatal(restErr)
	}
	var contextCards string
	for _, node := range *nodeIds {
		front := node.Fields["Front"].Value
		back := node.Fields["Back"].Value
		contextCards += front + " - " + back + "\n"
	}
	return contextCards, nil
}

type GetNotesInput struct {
	DeckName string `json:"deck_name" jsonschema_description:"The name of the deck to retrieve notes from. default: ChineseAgent"`
}

var GetNotesInputSchema = GenerateSchema[GetNotesInput]()

// Retrieves the full vocabulary context from the user's Anki notes.
var GetNotesDefinition = ToolDefinition{
	Name:        "get_anki_notes",
	Description: "Retrieve all vocabulary notes from the user's Anki collection. Use this to better understand the words the user is familiar with and to enrich the conversation with more personalized, context-aware responses.",
	InputSchema: GetNotesInputSchema,
	Function:    GetNotes,
}
