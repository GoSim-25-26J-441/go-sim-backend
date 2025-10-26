package export

import (
	"encoding/json"
	"os"
)

func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil { return err }
	return os.WriteFile(path, b, 0644)
}
