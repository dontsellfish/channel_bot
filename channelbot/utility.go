package channelbot

import (
	"encoding/json"
	"os"
)

func createDirectoryIfNotFound(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			err = os.Mkdir(path, os.ModePerm)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func deepCopyViaJsonSorryJesusChrist[T any](obj *T) *T {
	buff, err := json.Marshal(obj)
	if err != nil {
		panic(err) // unexpected behavior
	}
	var copied T
	err = json.Unmarshal(buff, &copied)
	if err != nil {
		panic(err) // much more unexpected behavior
	}
	return &copied
}

func contains(array []string, value string) bool {
	for _, el := range array {
		if el == value {
			return true
		}
	}
	return false
}

func containsInt(array []int64, value int64) bool {
	for _, el := range array {
		if el == value {
			return true
		}
	}
	return false
}
