package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

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
