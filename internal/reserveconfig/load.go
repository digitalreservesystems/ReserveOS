package reserveconfig

import (
	"encoding/json"
	"os"
)

func LoadJSON(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
